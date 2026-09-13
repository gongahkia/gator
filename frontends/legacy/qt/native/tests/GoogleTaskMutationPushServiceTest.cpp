#include <QtTest/QTest>

#include "core/Clock.h"
#include "core/GoogleHttpClient.h"
#include "core/GoogleTaskMutationPushService.h"
#include "core/MutationTelemetryStore.h"
#include "core/OptimisticMutationCoordinator.h"
#include "core/SyncBackoffPolicy.h"
#include "core/TaskListMutationService.h"
#include "core/TaskMutationService.h"
#include "data/SqliteConnection.h"
#include "support/MockNetworkAccessManager.h"
#include "support/TemporarySqliteDatabase.h"
#include "sqlite3.h"

#include <QDateTime>
#include <QCoreApplication>
#include <QEventLoop>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTimeZone>
#include <QUrlQuery>

#include <algorithm>
#include <chrono>
#include <future>
#include <memory>
#include <optional>
#include <thread>
#include <utility>
#include <variant>

using namespace std::chrono_literals;

class GoogleTaskMutationPushServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void pushesCreateUpdateAndDeleteMutations();
  void batchesIndependentTaskCreatesUpdatesMovesAndDeletes();
  void keepsPartialTaskBatchFailuresRetryable();
  void pushesTaskReorderMove();
  void recreatesThenDeletesCrossListTaskMove();
  void reconcilesCreatedTaskRemoteIdentity();
  void reconcilesChangeQueuedDuringCreate();
  void resolvesParentReferenceBeforeCreatingChild();
  void pushesAndReconcilesTaskListMutations();
  void pushesNewTaskListBeforeTasksAssignedToIt();
  void defersTaskMutationUntilTaskListHasRemoteId();
  void doesNotRetryUnknownCreateOutcome();
  void recordsPermanentAndRetriableFailures();
};

namespace {

class FixedClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override {
    return hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}};
  }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }
};

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  const auto deadline = std::chrono::steady_clock::now() + 1s;
  while (future.wait_for(0ms) != std::future_status::ready &&
         std::chrono::steady_clock::now() < deadline) {
    QCoreApplication::processEvents(QEventLoop::AllEvents, 10);
    std::this_thread::sleep_for(1ms);
  }
  if (future.wait_for(0ms) != std::future_status::ready) {
    qFatal("task mutation push request timed out");
  }
  return future.get();
}

[[nodiscard]] std::unique_ptr<hcb::test::TemporarySqliteDatabase> createDatabase() {
  hcb::test::TemporarySqliteDatabaseResult result = hcb::test::TemporarySqliteDatabase::create();
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result)) {
    return nullptr;
  }
  return std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result));
}

void verifyReady(hcb::OptimisticMutationCoordinator& coordinator) {
  const std::shared_future<hcb::SqliteWriteResult> ready = coordinator.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

void verifyReady(hcb::MutationTelemetryStore& store) {
  const std::shared_future<hcb::SqliteWriteResult> ready = store.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

void verifyReady(hcb::TaskMutationService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

void verifyReady(hcb::TaskListMutationService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

[[nodiscard]] hcb::PendingMutation enqueue(hcb::OptimisticMutationCoordinator& coordinator,
                                           QString operation,
                                           QJsonObject payload,
                                           QString resourceId = QStringLiteral("task-local")) {
  std::future<hcb::PendingMutationResult> future =
      coordinator.enqueue({.resource = hcb::PendingMutationResource::Task,
                           .resourceId = std::move(resourceId),
                           .operation = std::move(operation),
                           .payload = std::move(payload)});
  const hcb::PendingMutationResult result = awaitResult(future);
  if (!std::holds_alternative<hcb::PendingMutation>(result)) {
    qFatal("task mutation enqueue failed");
  }
  return std::get<hcb::PendingMutation>(result);
}

[[nodiscard]] hcb::PendingMutation find(hcb::OptimisticMutationCoordinator& coordinator,
                                        const QString& mutationId) {
  std::future<hcb::PendingMutationLookupResult> future = coordinator.find(mutationId);
  const hcb::PendingMutationLookupResult result = awaitResult(future);
  if (!std::holds_alternative<std::optional<hcb::PendingMutation>>(result) ||
      !std::get<std::optional<hcb::PendingMutation>>(result).has_value()) {
    qFatal("task mutation was not found");
  }
  return *std::get<std::optional<hcb::PendingMutation>>(result);
}

[[nodiscard]] hcb::GoogleTaskMutationPushResult push(hcb::GoogleTaskMutationPushService& service) {
  std::future<hcb::GoogleTaskMutationPushResultOrError> future =
      service.pushDue(QStringLiteral("access-token"));
  const hcb::GoogleTaskMutationPushResultOrError result = awaitResult(future);
  if (!std::holds_alternative<hcb::GoogleTaskMutationPushResult>(result)) {
    qFatal("task mutation push failed");
  }
  return std::get<hcb::GoogleTaskMutationPushResult>(result);
}

void execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  const QString message = errorMessage == nullptr ? QString() : QString::fromUtf8(errorMessage);
  sqlite3_free(errorMessage);
  if (result != SQLITE_OK) {
    qFatal("%s", qPrintable(message));
  }
}

[[nodiscard]] std::optional<QString>
readTaskColumn(sqlite3* handle, const QString& taskId, const char* column) {
  const QString sql =
      QStringLiteral("SELECT %1 FROM local_tasks WHERE id = ?1").arg(QString::fromLatin1(column));
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(
          handle, sql.toUtf8().constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const QByteArray taskIdUtf8 = taskId.toUtf8();
  if (sqlite3_bind_text(statement,
                        1,
                        taskIdUtf8.constData(),
                        static_cast<int>(taskIdUtf8.size()),
                        SQLITE_TRANSIENT) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const int stepResult = sqlite3_step(statement);
  const auto* text = stepResult == SQLITE_ROW
                         ? reinterpret_cast<const char*>(sqlite3_column_text(statement, 0))
                         : nullptr;
  const int size = stepResult == SQLITE_ROW ? sqlite3_column_bytes(statement, 0) : 0;
  const std::optional<QString> value = stepResult == SQLITE_ROW && text != nullptr && size >= 0
                                           ? std::optional<QString>(QString::fromUtf8(text, size))
                                           : std::nullopt;
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK ? value : std::nullopt;
}

[[nodiscard]] std::optional<QString>
readTaskListColumn(sqlite3* handle, const QString& taskListId, const char* column) {
  const QString sql = QStringLiteral("SELECT %1 FROM local_task_lists WHERE id = ?1")
                          .arg(QString::fromLatin1(column));
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(
          handle, sql.toUtf8().constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const QByteArray taskListIdUtf8 = taskListId.toUtf8();
  if (sqlite3_bind_text(statement,
                        1,
                        taskListIdUtf8.constData(),
                        static_cast<int>(taskListIdUtf8.size()),
                        SQLITE_TRANSIENT) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const int stepResult = sqlite3_step(statement);
  const auto* text = stepResult == SQLITE_ROW
                         ? reinterpret_cast<const char*>(sqlite3_column_text(statement, 0))
                         : nullptr;
  const int size = stepResult == SQLITE_ROW ? sqlite3_column_bytes(statement, 0) : 0;
  const std::optional<QString> value = stepResult == SQLITE_ROW && text != nullptr && size >= 0
                                           ? std::optional<QString>(QString::fromUtf8(text, size))
                                           : std::nullopt;
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK ? value : std::nullopt;
}

} // namespace

void GoogleTaskMutationPushServiceTest::pushesCreateUpdateAndDeleteMutations() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  const hcb::PendingMutation created = enqueue(
      coordinator,
      QStringLiteral("task.create"),
      {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
       {QStringLiteral("parentTaskId"), QStringLiteral("parent-1")},
       {QStringLiteral("previousTaskId"), QStringLiteral("previous-1")},
       {QStringLiteral("task"),
        QJsonObject{{QStringLiteral("title"), QStringLiteral(" Write/report ")},
                    {QStringLiteral("notes"), QStringLiteral("draft")},
                    {QStringLiteral("status"), QStringLiteral("needsAction")},
                    {QStringLiteral("due"), QStringLiteral("2026-08-01T18:30:00+08:00")}}}});
  const hcb::PendingMutation updated =
      enqueue(coordinator,
              QStringLiteral("task.update"),
              {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
               {QStringLiteral("remoteTaskId"), QStringLiteral("remote-1")},
               {QStringLiteral("etag"), QStringLiteral("etag-1")},
               {QStringLiteral("task"),
                QJsonObject{{QStringLiteral("status"), QStringLiteral("completed")},
                            {QStringLiteral("due"), QJsonValue::Null}}}});
  const hcb::PendingMutation removed =
      enqueue(coordinator,
              QStringLiteral("task.delete"),
              {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
               {QStringLiteral("remoteTaskId"), QStringLiteral("remote-1")},
               {QStringLiteral("etag"), QStringLiteral("etag-1")}});
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{}")});
  manager.enqueue({.body = QByteArray("{}")});
  manager.enqueue({.body = QByteArray()});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});

  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 3);
  QCOMPARE(result.failed, 0);
  QCOMPARE(manager.requests().size(), 3);
  const hcb::test::CapturedNetworkRequest* createRequest = nullptr;
  const hcb::test::CapturedNetworkRequest* updateRequest = nullptr;
  const hcb::test::CapturedNetworkRequest* deleteRequest = nullptr;
  for (const hcb::test::CapturedNetworkRequest& request : manager.requests()) {
    if (request.operation == QNetworkAccessManager::PostOperation) {
      createRequest = &request;
    } else if (request.operation == QNetworkAccessManager::CustomOperation) {
      updateRequest = &request;
    } else if (request.operation == QNetworkAccessManager::DeleteOperation) {
      deleteRequest = &request;
    }
  }
  QVERIFY(createRequest != nullptr);
  QVERIFY(updateRequest != nullptr);
  QVERIFY(deleteRequest != nullptr);
  if (createRequest == nullptr || updateRequest == nullptr || deleteRequest == nullptr) {
    return;
  }
  QCOMPARE(createRequest->request.url().path(), QStringLiteral("/tasks/v1/lists/list-1/tasks"));
  const QUrlQuery createQuery(createRequest->request.url());
  QCOMPARE(createQuery.queryItemValue(QStringLiteral("parent")), QStringLiteral("parent-1"));
  QCOMPARE(createQuery.queryItemValue(QStringLiteral("previous")), QStringLiteral("previous-1"));
  const QJsonObject createBody = QJsonDocument::fromJson(createRequest->body).object();
  QCOMPARE(createBody.value(QStringLiteral("title")).toString(), QStringLiteral("Write/report"));
  QCOMPARE(createBody.value(QStringLiteral("due")).toString(),
           QStringLiteral("2026-08-01T00:00:00.000Z"));
  QCOMPARE(updateRequest->request.url().path(),
           QStringLiteral("/tasks/v1/lists/list-1/tasks/remote-1"));
  QCOMPARE(updateRequest->request.rawHeader("If-Match"), QByteArray("etag-1"));
  const QJsonObject updateBody = QJsonDocument::fromJson(updateRequest->body).object();
  QCOMPARE(updateBody.value(QStringLiteral("status")).toString(), QStringLiteral("completed"));
  QVERIFY(updateBody.value(QStringLiteral("due")).isNull());
  QCOMPARE(deleteRequest->request.url().path(),
           QStringLiteral("/tasks/v1/lists/list-1/tasks/remote-1"));
  QCOMPARE(deleteRequest->request.rawHeader("If-Match"), QByteArray("etag-1"));
  QCOMPARE(find(coordinator, created.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, updated.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, removed.id).status, hcb::PendingMutationStatus::Applied);
}

void GoogleTaskMutationPushServiceTest::batchesIndependentTaskCreatesUpdatesMovesAndDeletes() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  const hcb::PendingMutation created = enqueue(
      coordinator,
      QStringLiteral("task.create"),
      {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
       {QStringLiteral("task"), QJsonObject{{QStringLiteral("title"), QStringLiteral("Create")}}}},
      QStringLiteral("task-created"));
  const hcb::PendingMutation updated = enqueue(
      coordinator,
      QStringLiteral("task.update"),
      {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
       {QStringLiteral("remoteTaskId"), QStringLiteral("task-update")},
       {QStringLiteral("etag"), QStringLiteral("etag-update")},
       {QStringLiteral("task"), QJsonObject{{QStringLiteral("title"), QStringLiteral("Update")}}}},
      QStringLiteral("task-updated"));
  const hcb::PendingMutation moved =
      enqueue(coordinator,
              QStringLiteral("task.move"),
              {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
               {QStringLiteral("remoteTaskId"), QStringLiteral("task-move")},
               {QStringLiteral("parentTaskId"), QStringLiteral("task-parent")}},
              QStringLiteral("task-moved"));
  const hcb::PendingMutation removed =
      enqueue(coordinator,
              QStringLiteral("task.delete"),
              {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
               {QStringLiteral("remoteTaskId"), QStringLiteral("task-delete")},
               {QStringLiteral("etag"), QStringLiteral("etag-delete")}},
              QStringLiteral("task-deleted"));
  const QByteArray batchResponse =
      QByteArrayLiteral("--batch_response\r\n"
                        "Content-Type: application/http\r\n"
                        "Content-ID: <response-item-3>\r\n\r\n"
                        "HTTP/1.1 204 No Content\r\nETag: \"etag-delete-new\"\r\n\r\n"
                        "--batch_response\r\n"
                        "Content-Type: application/http\r\n"
                        "Content-ID: <response-item-2>\r\n\r\n"
                        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{}\r\n"
                        "--batch_response\r\n"
                        "Content-Type: application/http\r\n"
                        "Content-ID: <response-item-1>\r\n\r\n"
                        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{}\r\n"
                        "--batch_response\r\n"
                        "Content-Type: application/http\r\n"
                        "Content-ID: <response-item-0>\r\n\r\n"
                        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{}\r\n"
                        "--batch_response--\r\n");
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = batchResponse});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});

  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 4);
  QCOMPARE(result.failed, 0);
  QCOMPARE(manager.requests().size(), 1);
  if (manager.requests().size() != 1) {
    return;
  }
  const hcb::test::CapturedNetworkRequest& request = manager.requests().constFirst();
  QCOMPARE(request.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(request.request.url().path(), QStringLiteral("/batch/tasks/v1"));
  QVERIFY(request.request.rawHeader("Content-Type").startsWith("multipart/mixed; boundary=hcb_"));
  QCOMPARE(request.request.rawHeader("Accept"), QByteArray("multipart/mixed"));
  QVERIFY(request.body.contains("POST /tasks/v1/lists/list-1/tasks HTTP/1.1"));
  QVERIFY(request.body.contains("PATCH /tasks/v1/lists/list-1/tasks/task-update HTTP/1.1"));
  QVERIFY(request.body.contains(
      "POST /tasks/v1/lists/list-1/tasks/task-move/move?parent=task-parent HTTP/1.1"));
  QVERIFY(request.body.contains("DELETE /tasks/v1/lists/list-1/tasks/task-delete HTTP/1.1"));
  QVERIFY(request.body.contains("If-Match: etag-update"));
  QVERIFY(request.body.contains("If-Match: etag-delete"));
  QCOMPARE(find(coordinator, created.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, updated.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, moved.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, removed.id).status, hcb::PendingMutationStatus::Applied);
}

void GoogleTaskMutationPushServiceTest::keepsPartialTaskBatchFailuresRetryable() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) return;
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  hcb::MutationTelemetryStore telemetry(database->databasePath(), clock);
  verifyReady(telemetry);
  const hcb::PendingMutation first = enqueue(
      coordinator,
      QStringLiteral("task.update"),
      {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
       {QStringLiteral("remoteTaskId"), QStringLiteral("remote-one")},
       {QStringLiteral("task"), QJsonObject{{QStringLiteral("title"), QStringLiteral("One")}}}},
      QStringLiteral("task-one"));
  const hcb::PendingMutation second = enqueue(
      coordinator,
      QStringLiteral("task.update"),
      {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
       {QStringLiteral("remoteTaskId"), QStringLiteral("remote-two")},
       {QStringLiteral("task"), QJsonObject{{QStringLiteral("title"), QStringLiteral("Two")}}}},
      QStringLiteral("task-two"));
  const QByteArray batchResponse =
      QByteArrayLiteral("--batch_response\r\n"
                        "Content-Type: application/http\r\n"
                        "Content-ID: <response-item-1>\r\n\r\n"
                        "HTTP/1.1 503 Service Unavailable\r\n"
                        "Content-Type: application/json\r\n"
                        "Retry-After: 3\r\n\r\n"
                        "{\"error\":{\"message\":\"busy\"}}\r\n"
                        "--batch_response\r\n"
                        "Content-Type: application/http\r\n"
                        "Content-ID: <response-item-0>\r\n\r\n"
                        "HTTP/1.1 200 OK\r\n"
                        "Content-Type: application/json\r\n\r\n"
                        "{}\r\n"
                        "--batch_response--\r\n");
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = batchResponse});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{}, nullptr, nullptr, nullptr, &telemetry);

  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 1);
  QCOMPARE(result.failed, 1);
  const hcb::PendingMutation firstResult = find(coordinator, first.id);
  const hcb::PendingMutation secondResult = find(coordinator, second.id);
  const hcb::PendingMutation& retriable =
      firstResult.status == hcb::PendingMutationStatus::Failed ? firstResult : secondResult;
  const hcb::PendingMutation& applied =
      firstResult.status == hcb::PendingMutationStatus::Applied ? firstResult : secondResult;
  QCOMPARE(applied.status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(retriable.status, hcb::PendingMutationStatus::Failed);
  QCOMPARE(retriable.lastErrorCode, std::optional<QString>(QStringLiteral("server")));
  QVERIFY(retriable.nextRetryAt.has_value());
  const hcb::MutationTelemetryReadResult telemetryResult = telemetry.recent().get();
  QVERIFY(std::holds_alternative<QList<hcb::MutationTelemetryRecord>>(telemetryResult));
  if (!std::holds_alternative<QList<hcb::MutationTelemetryRecord>>(telemetryResult)) {
    return;
  }
  const QList<hcb::MutationTelemetryRecord> records =
      std::get<QList<hcb::MutationTelemetryRecord>>(telemetryResult);
  QCOMPARE(records.size(), 2);
  const auto failed = std::find_if(records.cbegin(), records.cend(), [](const auto& record) {
    return record.phase == hcb::MutationTelemetryPhase::RemoteFailed;
  });
  QVERIFY(failed != records.cend());
  if (failed == records.cend()) {
    return;
  }
  QCOMPARE(failed->errorCode, std::optional<QString>(QStringLiteral("server")));
  QCOMPARE(failed->rollbackReason, std::optional<QString>(QStringLiteral("keep_retry")));
}

void GoogleTaskMutationPushServiceTest::pushesTaskReorderMove() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  const hcb::PendingMutation moved = enqueue(
      coordinator,
      QStringLiteral("task.move"),
      {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
       {QStringLiteral("remoteTaskId"), QStringLiteral("remote-task")},
       {QStringLiteral("parentTaskId"), QStringLiteral("remote-parent")},
       {QStringLiteral("previousTaskId"), QStringLiteral("remote-previous")}});
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});

  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 1);
  QCOMPARE(result.failed, 0);
  QCOMPARE(manager.requests().size(), 1);
  const hcb::test::CapturedNetworkRequest& request = manager.requests().constFirst();
  QCOMPARE(request.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(request.request.url().path(),
           QStringLiteral("/tasks/v1/lists/list-1/tasks/remote-task/move"));
  const QUrlQuery query(request.request.url());
  QCOMPARE(query.queryItemValue(QStringLiteral("parent")), QStringLiteral("remote-parent"));
  QCOMPARE(query.queryItemValue(QStringLiteral("previous")), QStringLiteral("remote-previous"));
  QVERIFY(request.body.isEmpty());
  QCOMPARE(find(coordinator, moved.id).status, hcb::PendingMutationStatus::Applied);
}

void GoogleTaskMutationPushServiceTest::recreatesThenDeletesCrossListTaskMove() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(database->databasePath(), clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult = hcb::SqliteConnectionFactory::open(
      database->databasePath(), hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-source', 'account-a', 'source-list', 'Source', '2026-07-25T00:00:00Z'), "
          "('list-destination', 'account-a', 'destination-list', 'Destination', "
          "'2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_tasks (id, task_list_id, remote_id, etag, title, state, updated_at) "
          "VALUES ('task-source', 'list-source', 'source-task', 'source-etag', 'Move me', "
          "'active', '2026-07-25T00:00:00Z')");
  std::future<hcb::TaskMutationResult> moved =
      taskMutations.moveToTaskList(QStringLiteral("task-source"),
                                   QStringLiteral("list-destination"));
  const hcb::TaskMutationResult movedResult = awaitResult(moved);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(movedResult));
  if (!std::holds_alternative<hcb::TaskMutationReceipt>(movedResult)) {
    return;
  }
  const QString replacementId = std::get<hcb::TaskMutationReceipt>(movedResult).taskId;

  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(R"({"id":"destination-task","etag":"destination-etag"})")});
  manager.enqueue({.body = QByteArray()});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{}, &taskMutations);

  const hcb::GoogleTaskMutationPushResult pushed = push(service);
  QCOMPARE(pushed.applied, 2);
  QCOMPARE(pushed.failed, 0);
  QCOMPARE(manager.requests().size(), 2);
  const hcb::test::CapturedNetworkRequest& createRequest = manager.requests().at(0);
  QCOMPARE(createRequest.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(createRequest.request.url().path(),
           QStringLiteral("/tasks/v1/lists/destination-list/tasks"));
  QCOMPARE(QJsonDocument::fromJson(createRequest.body)
               .object()
               .value(QStringLiteral("title"))
               .toString(),
           QStringLiteral("Move me"));
  const hcb::test::CapturedNetworkRequest& deleteRequest = manager.requests().at(1);
  QCOMPARE(deleteRequest.operation, QNetworkAccessManager::DeleteOperation);
  QCOMPARE(deleteRequest.request.url().path(),
           QStringLiteral("/tasks/v1/lists/source-list/tasks/source-task"));
  QCOMPARE(deleteRequest.request.rawHeader("If-Match"), QByteArray("source-etag"));
  QCOMPARE(readTaskColumn(handle, replacementId, "remote_id"),
           std::optional<QString>(QStringLiteral("destination-task")));
  QCOMPARE(readTaskColumn(handle, replacementId, "etag"),
           std::optional<QString>(QStringLiteral("destination-etag")));
}

void GoogleTaskMutationPushServiceTest::reconcilesCreatedTaskRemoteIdentity() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(database->databasePath(), clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult = hcb::SqliteConnectionFactory::open(
      database->databasePath(), hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-a', 'account-a', 'remote-list', 'Tasks', '2026-07-25T00:00:00Z')");
  std::future<hcb::TaskMutationResult> created = taskMutations.create(
      {.taskListId = QStringLiteral("list-a"), .title = QStringLiteral("Write release notes")});
  const hcb::TaskMutationResult createdResult = awaitResult(created);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(createdResult));
  if (!std::holds_alternative<hcb::TaskMutationReceipt>(createdResult)) {
    return;
  }
  const QString localTaskId = std::get<hcb::TaskMutationReceipt>(createdResult).taskId;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(R"({"id":"remote-task","etag":"etag-created"})")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{}, &taskMutations);

  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 1);
  QCOMPARE(result.failed, 0);
  QCOMPARE(manager.requests().size(), 1);
  QCOMPARE(manager.requests().constFirst().request.url().path(),
           QStringLiteral("/tasks/v1/lists/remote-list/tasks"));
  QCOMPARE(readTaskColumn(handle, localTaskId, "remote_id"),
           std::optional<QString>(QStringLiteral("remote-task")));
  QCOMPARE(readTaskColumn(handle, localTaskId, "etag"),
           std::optional<QString>(QStringLiteral("etag-created")));
  hcb::PendingMutationListResult pendingResult = coordinator.listDue().get();
  QVERIFY(std::holds_alternative<QList<hcb::PendingMutation>>(pendingResult));
  QCOMPARE(std::get<QList<hcb::PendingMutation>>(pendingResult).size(), 0);
}

void GoogleTaskMutationPushServiceTest::reconcilesChangeQueuedDuringCreate() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(database->databasePath(), clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult = hcb::SqliteConnectionFactory::open(
      database->databasePath(), hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-a', 'account-a', 'remote-list', 'Tasks', '2026-07-25T00:00:00Z')");
  std::future<hcb::TaskMutationResult> created = taskMutations.create(
      {.taskListId = QStringLiteral("list-a"), .title = QStringLiteral("Initial title")});
  const hcb::TaskMutationResult createdResult = awaitResult(created);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(createdResult));
  if (!std::holds_alternative<hcb::TaskMutationReceipt>(createdResult)) {
    return;
  }
  const QString localTaskId = std::get<hcb::TaskMutationReceipt>(createdResult).taskId;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  hcb::PendingMutationListResult dueResult = coordinator.listDue().get();
  QVERIFY(std::holds_alternative<QList<hcb::PendingMutation>>(dueResult));
  if (!std::holds_alternative<QList<hcb::PendingMutation>>(dueResult)) {
    return;
  }
  const QList<hcb::PendingMutation>& due = std::get<QList<hcb::PendingMutation>>(dueResult);
  QCOMPARE(due.size(), 1);
  if (due.size() != 1) {
    return;
  }
  hcb::PendingMutationResult claimedResult = coordinator.claim(due.constFirst().id, 5min).get();
  QVERIFY(std::holds_alternative<hcb::PendingMutation>(claimedResult));
  if (!std::holds_alternative<hcb::PendingMutation>(claimedResult)) {
    return;
  }
  const hcb::PendingMutation claimed = std::get<hcb::PendingMutation>(claimedResult);
  QVERIFY(claimed.leaseId.has_value());
  if (!claimed.leaseId.has_value()) {
    return;
  }
  std::future<hcb::TaskMutationResult> update =
      taskMutations.update({.taskId = localTaskId, .title = QStringLiteral("Updated title")});
  const hcb::TaskMutationResult updateResult = awaitResult(update);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(updateResult));
  std::future<hcb::TaskMutationResult> reconciled =
      taskMutations.reconcileGoogleTask({.localTaskId = localTaskId,
                                         .remoteTaskId = QStringLiteral("remote-task"),
                                         .remoteEtag = QStringLiteral("etag-created")});
  const hcb::TaskMutationResult reconciledResult = awaitResult(reconciled);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(reconciledResult));
  hcb::PendingMutationResult appliedResult =
      coordinator.markApplied(claimed.id, *claimed.leaseId).get();
  QVERIFY(std::holds_alternative<hcb::PendingMutation>(appliedResult));

  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(R"({"id":"remote-task","etag":"etag-updated"})")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{}, &taskMutations);
  const hcb::GoogleTaskMutationPushResult pushed = push(service);
  QCOMPARE(pushed.applied, 1);
  QCOMPARE(pushed.failed, 0);
  QCOMPARE(manager.requests().size(), 1);
  QCOMPARE(manager.requests().constFirst().request.url().path(),
           QStringLiteral("/tasks/v1/lists/remote-list/tasks/remote-task"));
  QCOMPARE(manager.requests().constFirst().request.rawHeader("If-Match"),
           QByteArray("etag-created"));
  QCOMPARE(readTaskColumn(handle, localTaskId, "etag"),
           std::optional<QString>(QStringLiteral("etag-updated")));
}

void GoogleTaskMutationPushServiceTest::resolvesParentReferenceBeforeCreatingChild() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(database->databasePath(), clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult = hcb::SqliteConnectionFactory::open(
      database->databasePath(), hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-a', 'account-a', 'remote-list', 'Tasks', '2026-07-25T00:00:00Z')");
  std::future<hcb::TaskMutationResult> parent = taskMutations.create(
      {.taskListId = QStringLiteral("list-a"), .title = QStringLiteral("Parent")});
  const hcb::TaskMutationResult parentResult = awaitResult(parent);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(parentResult));
  if (!std::holds_alternative<hcb::TaskMutationReceipt>(parentResult)) {
    return;
  }
  const QString parentTaskId = std::get<hcb::TaskMutationReceipt>(parentResult).taskId;
  std::future<hcb::TaskMutationResult> child =
      taskMutations.create({.taskListId = QStringLiteral("list-a"),
                            .parentTaskId = parentTaskId,
                            .title = QStringLiteral("Child")});
  const hcb::TaskMutationResult childResult = awaitResult(child);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(childResult));

  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  hcb::PendingMutationListResult dueResult = coordinator.listDue().get();
  QVERIFY(std::holds_alternative<QList<hcb::PendingMutation>>(dueResult));
  if (!std::holds_alternative<QList<hcb::PendingMutation>>(dueResult)) {
    return;
  }
  const QList<hcb::PendingMutation>& due = std::get<QList<hcb::PendingMutation>>(dueResult);
  const auto parentMutation =
      std::find_if(due.cbegin(), due.cend(), [&parentTaskId](const hcb::PendingMutation& mutation) {
        return mutation.resourceId == parentTaskId;
      });
  QVERIFY(parentMutation != due.cend());
  if (parentMutation == due.cend()) {
    return;
  }
  hcb::PendingMutationResult claimedResult = coordinator.claim(parentMutation->id, 5min).get();
  QVERIFY(std::holds_alternative<hcb::PendingMutation>(claimedResult));
  if (!std::holds_alternative<hcb::PendingMutation>(claimedResult)) {
    return;
  }
  const hcb::PendingMutation claimed = std::get<hcb::PendingMutation>(claimedResult);
  QVERIFY(claimed.leaseId.has_value());
  if (!claimed.leaseId.has_value()) {
    return;
  }
  std::future<hcb::TaskMutationResult> reconciled =
      taskMutations.reconcileGoogleTask({.localTaskId = parentTaskId,
                                         .remoteTaskId = QStringLiteral("remote-parent"),
                                         .remoteEtag = QStringLiteral("etag-parent")});
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(awaitResult(reconciled)));
  QVERIFY(std::holds_alternative<hcb::PendingMutation>(
      coordinator.markApplied(claimed.id, *claimed.leaseId).get()));

  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(R"({"id":"remote-child","etag":"etag-child"})")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{}, &taskMutations);
  const hcb::GoogleTaskMutationPushResult pushed = push(service);
  QCOMPARE(pushed.applied, 1);
  QCOMPARE(pushed.failed, 0);
  QCOMPARE(manager.requests().size(), 1);
  const QUrlQuery query(manager.requests().constFirst().request.url());
  QCOMPARE(query.queryItemValue(QStringLiteral("parent")), QStringLiteral("remote-parent"));
}

void GoogleTaskMutationPushServiceTest::pushesAndReconcilesTaskListMutations() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::TaskListMutationService taskListMutations(database->databasePath(), clock);
  verifyReady(taskListMutations);
  hcb::SqliteConnectionResult connectionResult = hcb::SqliteConnectionFactory::open(
      database->databasePath(), hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  std::future<hcb::TaskListMutationResult> created =
      taskListMutations.create({.accountId = QStringLiteral("account-a"),
                                .title = QStringLiteral(" Work ")});
  const hcb::TaskListMutationResult createdResult = awaitResult(created);
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(createdResult));
  if (!std::holds_alternative<hcb::TaskListMutationReceipt>(createdResult)) {
    return;
  }
  const QString taskListId = std::get<hcb::TaskListMutationReceipt>(createdResult).taskListId;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(R"({"id":"remote-list","etag":"etag-created"})")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{}, nullptr, &taskListMutations);

  const hcb::GoogleTaskMutationPushResult createdPush = push(service);
  QCOMPARE(createdPush.applied, 1);
  QCOMPARE(createdPush.failed, 0);
  QCOMPARE(manager.requests().size(), 1);
  QCOMPARE(manager.requests().constFirst().request.url().path(),
           QStringLiteral("/tasks/v1/users/@me/lists"));
  const QJsonObject createBody = QJsonDocument::fromJson(manager.requests().constFirst().body).object();
  QCOMPARE(createBody.value(QStringLiteral("title")).toString(), QStringLiteral("Work"));
  QCOMPARE(readTaskListColumn(handle, taskListId, "remote_id"),
           std::optional<QString>(QStringLiteral("remote-list")));

  std::future<hcb::TaskListMutationResult> updated =
      taskListMutations.update({.taskListId = taskListId, .title = QStringLiteral("Projects")});
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(awaitResult(updated)));
  manager.enqueue({.body = QByteArray(R"({"id":"remote-list","etag":"etag-updated"})")});
  const hcb::GoogleTaskMutationPushResult updatedPush = push(service);
  QCOMPARE(updatedPush.applied, 1);
  QCOMPARE(updatedPush.failed, 0);
  QCOMPARE(manager.requests().size(), 2);
  const hcb::test::CapturedNetworkRequest& updateRequest = manager.requests().at(1);
  QCOMPARE(updateRequest.request.url().path(),
           QStringLiteral("/tasks/v1/users/@me/lists/remote-list"));
  QCOMPARE(updateRequest.request.rawHeader("If-Match"), QByteArray("etag-created"));
  QCOMPARE(QJsonDocument::fromJson(updateRequest.body)
               .object()
               .value(QStringLiteral("title"))
               .toString(),
           QStringLiteral("Projects"));
  QCOMPARE(readTaskListColumn(handle, taskListId, "etag"),
           std::optional<QString>(QStringLiteral("etag-updated")));

  std::future<hcb::TaskListMutationResult> removed = taskListMutations.remove(taskListId);
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(awaitResult(removed)));
  manager.enqueue({.body = QByteArray()});
  const hcb::GoogleTaskMutationPushResult removedPush = push(service);
  QCOMPARE(removedPush.applied, 1);
  QCOMPARE(removedPush.failed, 0);
  QCOMPARE(manager.requests().size(), 3);
  const hcb::test::CapturedNetworkRequest& deleteRequest = manager.requests().at(2);
  QCOMPARE(deleteRequest.operation, QNetworkAccessManager::DeleteOperation);
  QCOMPARE(deleteRequest.request.url().path(),
           QStringLiteral("/tasks/v1/users/@me/lists/remote-list"));
  QCOMPARE(deleteRequest.request.rawHeader("If-Match"), QByteArray("etag-updated"));
}

void GoogleTaskMutationPushServiceTest::pushesNewTaskListBeforeTasksAssignedToIt() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::TaskListMutationService taskListMutations(database->databasePath(), clock);
  verifyReady(taskListMutations);
  hcb::TaskMutationService taskMutations(database->databasePath(), clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult = hcb::SqliteConnectionFactory::open(
      database->databasePath(), hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  std::future<hcb::TaskListMutationResult> createdList =
      taskListMutations.create({.accountId = QStringLiteral("account-a"),
                                .title = QStringLiteral("Projects")});
  const hcb::TaskListMutationResult createdListResult = awaitResult(createdList);
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(createdListResult));
  if (!std::holds_alternative<hcb::TaskListMutationReceipt>(createdListResult)) {
    return;
  }
  const QString taskListId = std::get<hcb::TaskListMutationReceipt>(createdListResult).taskListId;
  std::future<hcb::TaskMutationResult> createdTask =
      taskMutations.create({.taskListId = taskListId, .title = QStringLiteral("Ship")});
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(awaitResult(createdTask)));
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(R"({"id":"remote-list","etag":"etag-list"})")});
  manager.enqueue({.body = QByteArray(R"({"id":"remote-task","etag":"etag-task"})")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(coordinator,
                                              httpClient,
                                              clock,
                                              hcb::SyncBackoffPolicy{},
                                              &taskMutations,
                                              &taskListMutations);

  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 2);
  QCOMPARE(result.failed, 0);
  QCOMPARE(result.skipped, 0);
  QCOMPARE(manager.requests().size(), 2);
  QCOMPARE(manager.requests().at(0).request.url().path(),
           QStringLiteral("/tasks/v1/users/@me/lists"));
  QCOMPARE(manager.requests().at(1).request.url().path(),
           QStringLiteral("/tasks/v1/lists/remote-list/tasks"));
}

void GoogleTaskMutationPushServiceTest::defersTaskMutationUntilTaskListHasRemoteId() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(database->databasePath(), clock);
  verifyReady(taskMutations);
  hcb::TaskListMutationService taskListMutations(database->databasePath(), clock);
  verifyReady(taskListMutations);
  hcb::SqliteConnectionResult connectionResult = hcb::SqliteConnectionFactory::open(
      database->databasePath(), hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-a', 'account-a', 'pending:list-a', 'Tasks', '2026-07-25T00:00:00Z')");
  std::future<hcb::TaskMutationResult> created = taskMutations.create(
      {.taskListId = QStringLiteral("list-a"), .title = QStringLiteral("Deferred")});
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(awaitResult(created)));
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  hcb::test::MockNetworkAccessManager manager;
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(coordinator,
                                              httpClient,
                                              clock,
                                              hcb::SyncBackoffPolicy{},
                                              &taskMutations,
                                              &taskListMutations);

  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 0);
  QCOMPARE(result.failed, 0);
  QCOMPARE(result.skipped, 1);
  QCOMPARE(manager.requests().size(), 0);
  std::future<hcb::PendingMutationListResult> due = coordinator.listDue();
  const hcb::PendingMutationListResult dueResult = awaitResult(due);
  QVERIFY(std::holds_alternative<QList<hcb::PendingMutation>>(dueResult));
  QCOMPARE(std::get<QList<hcb::PendingMutation>>(dueResult).size(), 0);
}

void GoogleTaskMutationPushServiceTest::doesNotRetryUnknownCreateOutcome() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  const hcb::PendingMutation created = enqueue(
      coordinator,
      QStringLiteral("task.create"),
      {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
       {QStringLiteral("task"), QJsonObject{{QStringLiteral("title"), QStringLiteral("x")}}}});
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 503, .body = QByteArray("{}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});
  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 0);
  QCOMPARE(result.failed, 1);
  QCOMPARE(manager.requests().size(), 1);
  const hcb::PendingMutation failed = find(coordinator, created.id);
  QCOMPARE(failed.status, hcb::PendingMutationStatus::Failed);
  QCOMPARE(failed.lastErrorCode, std::optional<QString>(QStringLiteral("create_outcome_unknown")));
  QVERIFY(!failed.nextRetryAt.has_value());
}

void GoogleTaskMutationPushServiceTest::recordsPermanentAndRetriableFailures() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  const hcb::PendingMutation invalid = enqueue(
      coordinator,
      QStringLiteral("task.create"),
      {{QStringLiteral("task"), QJsonObject{{QStringLiteral("title"), QStringLiteral("x")}}}});
  const hcb::PendingMutation rateLimited = enqueue(
      coordinator,
      QStringLiteral("task.update"),
      {{QStringLiteral("taskListId"), QStringLiteral("list-1")},
       {QStringLiteral("remoteTaskId"), QStringLiteral("remote-1")},
       {QStringLiteral("task"), QJsonObject{{QStringLiteral("title"), QStringLiteral("x")}}}});
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 503,
                   .body = QByteArray("{}"),
                   .error = QNetworkReply::UnknownServerError,
                   .headers = {{QByteArray("Retry-After"), QByteArray("3")}}});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskMutationPushService service(
      coordinator,
      httpClient,
      clock,
      hcb::SyncBackoffPolicy({.baseDelayMilliseconds = 1'000,
                              .maximumDelayMilliseconds = 10'000,
                              .jitterMilliseconds = 0,
                              .maximumAttempts = 6,
                              .random = [] { return 0.0; }}));

  const hcb::GoogleTaskMutationPushResult result = push(service);
  QCOMPARE(result.applied, 0);
  QCOMPARE(result.failed, 2);
  QCOMPARE(manager.requests().size(), 1);
  const hcb::PendingMutation invalidResult = find(coordinator, invalid.id);
  QCOMPARE(invalidResult.status, hcb::PendingMutationStatus::Failed);
  QCOMPARE(invalidResult.lastErrorCode, std::optional<QString>(QStringLiteral("invalid_payload")));
  QVERIFY(!invalidResult.nextRetryAt.has_value());
  const hcb::PendingMutation rateLimitedResult = find(coordinator, rateLimited.id);
  QCOMPARE(rateLimitedResult.status, hcb::PendingMutationStatus::Failed);
  QCOMPARE(rateLimitedResult.lastErrorCode, std::optional<QString>(QStringLiteral("server")));
  QCOMPARE(rateLimitedResult.nextRetryAt,
           std::optional<QString>(QStringLiteral("2025-07-25T01:46:43.123Z")));
}

QTEST_GUILESS_MAIN(GoogleTaskMutationPushServiceTest)

#include "GoogleTaskMutationPushServiceTest.moc"
