#include "core/TaskListMutationService.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

#include <QDateTime>
#include <QDir>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTemporaryDir>
#include <QTimeZone>
#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

using namespace std::chrono_literals;

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

struct TaskListSnapshot final {
  QString remoteId;
  QString title;
  std::optional<QString> etag;
  std::optional<QString> deletedAt;
};

struct PendingMutationSnapshot final {
  QString operation;
  QString status;
  QJsonObject payload;
};

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("task-list mutation service request timed out");
  }
  return future.get();
}

void verifyReady(hcb::TaskListMutationService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

void execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  const QString message = errorMessage == nullptr ? QString() : QString::fromUtf8(errorMessage);
  sqlite3_free(errorMessage);
  QVERIFY2(result == SQLITE_OK, qPrintable(message));
}

[[nodiscard]] std::optional<QString> optionalText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int size = sqlite3_column_bytes(statement, index);
  return value == nullptr || size < 0 ? std::nullopt
                                      : std::optional<QString>(QString::fromUtf8(value, size));
}

[[nodiscard]] std::optional<TaskListSnapshot> readTaskList(sqlite3* handle,
                                                           const QString& taskListId) {
  constexpr char sql[] = R"(
SELECT remote_id, title, etag, deleted_at
FROM local_task_lists
WHERE id = ?1
)";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
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
  if (sqlite3_step(statement) != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const std::optional<QString> remoteId = optionalText(statement, 0);
  const std::optional<QString> title = optionalText(statement, 1);
  const std::optional<QString> etag = optionalText(statement, 2);
  const std::optional<QString> deletedAt = optionalText(statement, 3);
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK && remoteId.has_value() && title.has_value()
             ? std::optional<TaskListSnapshot>(TaskListSnapshot{
                   .remoteId = *remoteId, .title = *title, .etag = etag, .deletedAt = deletedAt})
             : std::nullopt;
}

[[nodiscard]] std::optional<PendingMutationSnapshot>
readMutation(sqlite3* handle, const QString& resourceType, const QString& resourceId) {
  constexpr char sql[] = R"(
SELECT operation, status, payload_json
FROM local_pending_mutations
WHERE resource_type = ?1 AND resource_id = ?2
ORDER BY CASE WHEN status IN ('pending', 'failed', 'applying') THEN 0 ELSE 1 END,
         created_at DESC, id DESC
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const QByteArray resourceTypeUtf8 = resourceType.toUtf8();
  const QByteArray resourceIdUtf8 = resourceId.toUtf8();
  if (sqlite3_bind_text(statement,
                        1,
                        resourceTypeUtf8.constData(),
                        static_cast<int>(resourceTypeUtf8.size()),
                        SQLITE_TRANSIENT) != SQLITE_OK ||
      sqlite3_bind_text(statement,
                        2,
                        resourceIdUtf8.constData(),
                        static_cast<int>(resourceIdUtf8.size()),
                        SQLITE_TRANSIENT) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  if (sqlite3_step(statement) != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const std::optional<QString> operation = optionalText(statement, 0);
  const std::optional<QString> status = optionalText(statement, 1);
  const std::optional<QString> payloadJson = optionalText(statement, 2);
  const QJsonDocument payload =
      payloadJson.has_value() ? QJsonDocument::fromJson(payloadJson->toUtf8()) : QJsonDocument();
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK && operation.has_value() && status.has_value() &&
                 payload.isObject()
             ? std::optional<PendingMutationSnapshot>(PendingMutationSnapshot{
                   .operation = *operation, .status = *status, .payload = payload.object()})
             : std::nullopt;
}

[[nodiscard]] std::optional<QString> taskDeletedAt(sqlite3* handle, const QString& taskId) {
  constexpr char sql[] = "SELECT deleted_at FROM local_tasks WHERE id = ?1";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
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
  const std::optional<QString> deletedAt =
      stepResult == SQLITE_ROW ? optionalText(statement, 0) : std::nullopt;
  const int finalizeResult = sqlite3_finalize(statement);
  return stepResult == SQLITE_ROW && finalizeResult == SQLITE_OK ? deletedAt : std::nullopt;
}

[[nodiscard]] std::optional<bool> taskListSelected(sqlite3* handle, const QString& taskListId) {
  constexpr char sql[] = "SELECT is_selected FROM local_task_lists WHERE id = ?1";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
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
  const int selected = stepResult == SQLITE_ROW ? sqlite3_column_int(statement, 0) : -1;
  const int finalizeResult = sqlite3_finalize(statement);
  return stepResult == SQLITE_ROW && finalizeResult == SQLITE_OK && (selected == 0 || selected == 1)
             ? std::optional<bool>(selected == 1)
             : std::nullopt;
}

void seedAccount(sqlite3* handle) {
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
}

} // namespace

class TaskListMutationServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void queuesReconcilesAndDeletesTaskList();
  void updatesLocalSelectionWithoutQueueingGoogleMutation();
  void rejectsDeletionWhileTaskMutationIsApplying();
};

void TaskListMutationServiceTest::queuesReconcilesAndDeletesTaskList() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::TaskListMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  seedAccount(handle);

  std::future<hcb::TaskListMutationResult> created = service.create(
      {.accountId = QStringLiteral("account-a"), .title = QStringLiteral(" Inbox ")});
  const hcb::TaskListMutationResult createdResult = awaitResult(created);
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(createdResult));
  if (!std::holds_alternative<hcb::TaskListMutationReceipt>(createdResult)) {
    return;
  }
  const QString taskListId = std::get<hcb::TaskListMutationReceipt>(createdResult).taskListId;
  const std::optional<TaskListSnapshot> createdList = readTaskList(handle, taskListId);
  QVERIFY(createdList.has_value());
  if (!createdList.has_value()) {
    return;
  }
  QCOMPARE(createdList->title, QStringLiteral("Inbox"));
  QVERIFY(createdList->remoteId.startsWith(QStringLiteral("pending:")));
  const std::optional<PendingMutationSnapshot> createMutation =
      readMutation(handle, QStringLiteral("task_list"), taskListId);
  QVERIFY(createMutation.has_value());
  if (!createMutation.has_value()) {
    return;
  }
  QCOMPARE(createMutation->operation, QStringLiteral("task_list.create"));

  std::future<hcb::TaskListMutationResult> renamedWhilePending =
      service.update({.taskListId = taskListId, .title = QStringLiteral(" Work ")});
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(awaitResult(renamedWhilePending)));
  const std::optional<PendingMutationSnapshot> coalescedCreate =
      readMutation(handle, QStringLiteral("task_list"), taskListId);
  QVERIFY(coalescedCreate.has_value());
  if (!coalescedCreate.has_value()) {
    return;
  }
  QCOMPARE(coalescedCreate->operation, QStringLiteral("task_list.create"));
  QCOMPARE(coalescedCreate->payload.value(QStringLiteral("taskList"))
               .toObject()
               .value(QStringLiteral("title"))
               .toString(),
           QStringLiteral("Work"));

  std::future<hcb::TaskListMutationResult> reconciled =
      service.reconcileGoogleTaskList({.localTaskListId = taskListId,
                                       .remoteTaskListId = QStringLiteral("remote-list"),
                                       .remoteEtag = QStringLiteral("etag-created")});
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(awaitResult(reconciled)));
  const QByteArray taskListIdUtf8 = taskListId.toUtf8();
  sqlite3_stmt* appliedStatement = nullptr;
  QVERIFY(sqlite3_prepare_v3(handle,
                             "UPDATE local_pending_mutations SET status = 'applied', "
                             "applied_at = '2026-07-25T00:00:00Z' "
                             "WHERE resource_type = 'task_list' AND resource_id = ?1",
                             -1,
                             SQLITE_PREPARE_PERSISTENT,
                             &appliedStatement,
                             nullptr) == SQLITE_OK);
  QVERIFY(sqlite3_bind_text(appliedStatement,
                            1,
                            taskListIdUtf8.constData(),
                            static_cast<int>(taskListIdUtf8.size()),
                            SQLITE_TRANSIENT) == SQLITE_OK);
  QCOMPARE(sqlite3_step(appliedStatement), SQLITE_DONE);
  QCOMPARE(sqlite3_finalize(appliedStatement), SQLITE_OK);

  std::future<hcb::TaskListMutationResult> renamed =
      service.update({.taskListId = taskListId, .title = QStringLiteral(" Projects ")});
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(awaitResult(renamed)));
  const std::optional<PendingMutationSnapshot> updateMutation =
      readMutation(handle, QStringLiteral("task_list"), taskListId);
  QVERIFY(updateMutation.has_value());
  if (!updateMutation.has_value()) {
    return;
  }
  QCOMPARE(updateMutation->operation, QStringLiteral("task_list.update"));
  QCOMPARE(updateMutation->payload.value(QStringLiteral("remoteTaskListId")).toString(),
           QStringLiteral("remote-list"));
  QCOMPARE(updateMutation->payload.value(QStringLiteral("_hcbSync"))
               .toObject()
               .value(QStringLiteral("etag"))
               .toString(),
           QStringLiteral("etag-created"));

  sqlite3_stmt* taskStatement = nullptr;
  QVERIFY(sqlite3_prepare_v3(
              handle,
              "INSERT INTO local_tasks (id, task_list_id, remote_id, title, state, updated_at) "
              "VALUES ('task-a', ?1, 'pending:task-a', 'Task', 'active', '2026-07-25T00:00:00Z')",
              -1,
              SQLITE_PREPARE_PERSISTENT,
              &taskStatement,
              nullptr) == SQLITE_OK);
  QVERIFY(sqlite3_bind_text(taskStatement,
                            1,
                            taskListIdUtf8.constData(),
                            static_cast<int>(taskListIdUtf8.size()),
                            SQLITE_TRANSIENT) == SQLITE_OK);
  QCOMPARE(sqlite3_step(taskStatement), SQLITE_DONE);
  QCOMPARE(sqlite3_finalize(taskStatement), SQLITE_OK);
  execute(handle,
          "INSERT INTO local_pending_mutations (id, resource_type, resource_id, operation, "
          "payload_json, status, attempt_count, created_at, updated_at) VALUES "
          "('mutation:task-a', 'task', 'task-a', 'task.create', '{}', 'pending', 0, "
          "'2026-07-25T00:00:00Z', '2026-07-25T00:00:00Z')");

  std::future<hcb::TaskListMutationResult> removed = service.remove(taskListId);
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(awaitResult(removed)));
  const std::optional<TaskListSnapshot> deletedList = readTaskList(handle, taskListId);
  QVERIFY(deletedList.has_value());
  if (!deletedList.has_value()) {
    return;
  }
  QVERIFY(deletedList->deletedAt.has_value());
  QVERIFY(taskDeletedAt(handle, QStringLiteral("task-a")).has_value());
  const std::optional<PendingMutationSnapshot> cancelledTaskMutation =
      readMutation(handle, QStringLiteral("task"), QStringLiteral("task-a"));
  QVERIFY(cancelledTaskMutation.has_value());
  if (!cancelledTaskMutation.has_value()) {
    return;
  }
  QCOMPARE(cancelledTaskMutation->status, QStringLiteral("cancelled"));
  const std::optional<PendingMutationSnapshot> deleteMutation =
      readMutation(handle, QStringLiteral("task_list"), taskListId);
  QVERIFY(deleteMutation.has_value());
  if (!deleteMutation.has_value()) {
    return;
  }
  QCOMPARE(deleteMutation->operation, QStringLiteral("task_list.delete"));
  QCOMPARE(deleteMutation->payload.value(QStringLiteral("remoteTaskListId")).toString(),
           QStringLiteral("remote-list"));
}

void TaskListMutationServiceTest::updatesLocalSelectionWithoutQueueingGoogleMutation() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::TaskListMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  seedAccount(handle);
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, is_selected, "
          "updated_at) VALUES ('list-a', 'account-a', 'remote-list', 'Tasks', 1, "
          "'2026-07-25T00:00:00Z')");

  std::future<hcb::TaskListMutationResult> deselected =
      service.setSelected({.taskListId = QStringLiteral("list-a"), .selected = false});
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(awaitResult(deselected)));
  QCOMPARE(taskListSelected(handle, QStringLiteral("list-a")), std::optional<bool>(false));
  QVERIFY(!readMutation(handle, QStringLiteral("task_list"), QStringLiteral("list-a")).has_value());

  std::future<hcb::TaskListMutationResult> selected =
      service.setSelected({.taskListId = QStringLiteral("list-a"), .selected = true});
  QVERIFY(std::holds_alternative<hcb::TaskListMutationReceipt>(awaitResult(selected)));
  QCOMPARE(taskListSelected(handle, QStringLiteral("list-a")), std::optional<bool>(true));
}

void TaskListMutationServiceTest::rejectsDeletionWhileTaskMutationIsApplying() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::TaskListMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  seedAccount(handle);
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-a', 'account-a', 'remote-list', 'Tasks', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_tasks (id, task_list_id, remote_id, title, state, updated_at) VALUES "
          "('task-a', 'list-a', 'remote-task', 'Task', 'active', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_pending_mutations (id, resource_type, resource_id, operation, "
          "payload_json, status, attempt_count, lease_id, lease_expires_at, created_at, "
          "updated_at) VALUES "
          "('mutation:task-a', 'task', 'task-a', 'task.update', '{}', 'applying', 1, "
          "'lease-a', '2026-07-25T00:01:00Z', '2026-07-25T00:00:00Z', "
          "'2026-07-25T00:00:00Z')");
  std::future<hcb::TaskListMutationResult> removed = service.remove(QStringLiteral("list-a"));
  const hcb::TaskListMutationResult result = awaitResult(removed);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
  const std::optional<TaskListSnapshot> taskList = readTaskList(handle, QStringLiteral("list-a"));
  QVERIFY(taskList.has_value());
  if (!taskList.has_value()) {
    return;
  }
  QVERIFY(!taskList->deletedAt.has_value());
}

QTEST_GUILESS_MAIN(TaskListMutationServiceTest)

#include "TaskListMutationServiceTest.moc"
