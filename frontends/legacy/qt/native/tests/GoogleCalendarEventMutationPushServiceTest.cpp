#include <QtTest/QTest>

#include "core/Clock.h"
#include "core/CalendarMutationService.h"
#include "core/GoogleCalendarEventMutationPushService.h"
#include "core/GoogleHttpClient.h"
#include "core/OptimisticMutationCoordinator.h"
#include "core/SyncBackoffPolicy.h"
#include "support/MockNetworkAccessManager.h"
#include "support/TemporarySqliteDatabase.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

#include <QCoreApplication>
#include <QEventLoop>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QUrlQuery>

#include <chrono>
#include <future>
#include <memory>
#include <optional>
#include <thread>
#include <utility>
#include <variant>

using namespace std::chrono_literals;

class GoogleCalendarEventMutationPushServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void pushesCreateUpdateAndDeleteMutations();
  void batchesIndependentEventWritesWithPerItemResults();
  void batchesIndependentEventCreatesAndMoves();
  void pushesMoveBeforeDependentPatch();
  void resolvesGeneratedInstanceBeforeApplyingScopedUpdate();
  void reconcilesCreatedEventIdentity();
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
    qFatal("event mutation push request timed out");
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

void verifyReady(hcb::CalendarMutationService& service) {
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

[[nodiscard]] std::optional<QString>
readEventRemoteId(sqlite3* handle, const QString& eventId) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle,
                         "SELECT remote_id FROM local_calendar_events WHERE id = ?1",
                         -1,
                         SQLITE_PREPARE_PERSISTENT,
                         &statement,
                         nullptr) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const QByteArray eventIdUtf8 = eventId.toUtf8();
  if (sqlite3_bind_text(statement,
                        1,
                        eventIdUtf8.constData(),
                        static_cast<int>(eventIdUtf8.size()),
                        SQLITE_TRANSIENT) != SQLITE_OK ||
      sqlite3_step(statement) != SQLITE_ROW || sqlite3_column_type(statement, 0) != SQLITE_TEXT) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const auto* raw = reinterpret_cast<const char*>(sqlite3_column_text(statement, 0));
  const int size = sqlite3_column_bytes(statement, 0);
  const std::optional<QString> result =
      raw == nullptr || size < 0 ? std::nullopt
                                 : std::optional<QString>(QString::fromUtf8(raw, size));
  return sqlite3_finalize(statement) == SQLITE_OK ? result : std::nullopt;
}

[[nodiscard]] hcb::PendingMutation enqueue(hcb::OptimisticMutationCoordinator& coordinator,
                                           QString operation,
                                           QJsonObject payload,
                                           QString resourceId = QStringLiteral("event-local")) {
  std::future<hcb::PendingMutationResult> future =
      coordinator.enqueue({.resource = hcb::PendingMutationResource::Event,
                           .resourceId = std::move(resourceId),
                           .operation = std::move(operation),
                           .payload = std::move(payload)});
  const hcb::PendingMutationResult result = awaitResult(future);
  if (!std::holds_alternative<hcb::PendingMutation>(result)) {
    qFatal("event mutation enqueue failed");
  }
  return std::get<hcb::PendingMutation>(result);
}

[[nodiscard]] hcb::PendingMutation find(hcb::OptimisticMutationCoordinator& coordinator,
                                        const QString& mutationId) {
  std::future<hcb::PendingMutationLookupResult> future = coordinator.find(mutationId);
  const hcb::PendingMutationLookupResult result = awaitResult(future);
  if (!std::holds_alternative<std::optional<hcb::PendingMutation>>(result) ||
      !std::get<std::optional<hcb::PendingMutation>>(result).has_value()) {
    qFatal("event mutation was not found");
  }
  return *std::get<std::optional<hcb::PendingMutation>>(result);
}

[[nodiscard]] hcb::GoogleCalendarEventMutationPushResult
push(hcb::GoogleCalendarEventMutationPushService& service) {
  std::future<hcb::GoogleCalendarEventMutationPushResultOrError> future =
      service.pushDue(QStringLiteral("access-token"));
  const hcb::GoogleCalendarEventMutationPushResultOrError result = awaitResult(future);
  if (!std::holds_alternative<hcb::GoogleCalendarEventMutationPushResult>(result)) {
    qFatal("event mutation push failed");
  }
  return std::get<hcb::GoogleCalendarEventMutationPushResult>(result);
}

} // namespace

void GoogleCalendarEventMutationPushServiceTest::pushesCreateUpdateAndDeleteMutations() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  const QJsonObject reminderSettings{
      {QStringLiteral("useDefault"), false},
      {QStringLiteral("overrides"),
       QJsonArray{QJsonObject{{QStringLiteral("method"), QStringLiteral("popup")},
                              {QStringLiteral("minutes"), 10}}}}};
  const QJsonObject createdEvent{
      {QStringLiteral("summary"), QStringLiteral(" Planning/Q3 ")},
      {QStringLiteral("description"), QStringLiteral("draft")},
      {QStringLiteral("location"), QStringLiteral("HQ")},
      {QStringLiteral("start"),
       QJsonObject{{QStringLiteral("dateTime"), QStringLiteral("2026-08-01T09:00:00")},
                   {QStringLiteral("timeZone"), QStringLiteral("Asia/Singapore")}}},
      {QStringLiteral("end"),
       QJsonObject{{QStringLiteral("dateTime"), QStringLiteral("2026-08-01T10:00:00")},
                   {QStringLiteral("timeZone"), QStringLiteral("Asia/Singapore")}}},
      {QStringLiteral("attendees"),
       QJsonArray{QJsonObject{{QStringLiteral("email"), QStringLiteral("guest@example.com")},
                              {QStringLiteral("responseStatus"), QStringLiteral("needsAction")}}}},
      {QStringLiteral("reminders"), reminderSettings},
      {QStringLiteral("recurrence"),
       QJsonArray{QStringLiteral("RDATE;TZID=Asia/Singapore:20261225T093000")}},
      {QStringLiteral("conferenceData"),
       QJsonObject{{QStringLiteral("createRequest"),
                    QJsonObject{{QStringLiteral("requestId"), QStringLiteral("request-1")},
                                {QStringLiteral("conferenceSolutionKey"),
                                 QJsonObject{{QStringLiteral("type"),
                                              QStringLiteral("hangoutsMeet")}}}}}}},
      {QStringLiteral("attachments"),
       QJsonArray{QJsonObject{{QStringLiteral("fileUrl"),
                              QStringLiteral("https://drive.google.com/open?id=file-1")},
                             {QStringLiteral("title"), QStringLiteral("Spec")}}}},
      {QStringLiteral("guestsCanModify"), true}};
  const hcb::PendingMutation created = enqueue(
      coordinator,
      QStringLiteral("event.create"),
      {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
       {QStringLiteral("sendUpdates"), QStringLiteral("externalOnly")},
       {QStringLiteral("event"), createdEvent}});
  const hcb::PendingMutation updated =
      enqueue(coordinator,
              QStringLiteral("event.update"),
              {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
               {QStringLiteral("remoteEventId"), QStringLiteral("remote-1")},
               {QStringLiteral("etag"), QStringLiteral("etag-1")},
               {QStringLiteral("sendUpdates"), QStringLiteral("none")},
               {QStringLiteral("event"),
                QJsonObject{{QStringLiteral("description"), QJsonValue::Null},
                            {QStringLiteral("location"), QStringLiteral("Remote")},
                            {QStringLiteral("colorId"), QJsonValue::Null},
                            {QStringLiteral("attendees"),
                             QJsonArray{QJsonObject{{QStringLiteral("email"),
                                                    QStringLiteral("me@example.com")},
                                                   {QStringLiteral("responseStatus"),
                                                    QStringLiteral("accepted")}}}},
                            {QStringLiteral("attendeesOmitted"), true}}}});
  const hcb::PendingMutation removed =
      enqueue(coordinator,
              QStringLiteral("event.delete"),
              {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
               {QStringLiteral("remoteEventId"), QStringLiteral("remote-1")},
               {QStringLiteral("etag"), QStringLiteral("etag-1")}});
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{}")});
  manager.enqueue({.body = QByteArray("{}")});
  manager.enqueue({.body = QByteArray()});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});

  const hcb::GoogleCalendarEventMutationPushResult result = push(service);
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
  QCOMPARE(createRequest->request.url().path(),
           QStringLiteral("/calendar/v3/calendars/calendar-1/events"));
  const QJsonObject createBody = QJsonDocument::fromJson(createRequest->body).object();
  QCOMPARE(createBody.value(QStringLiteral("summary")).toString(), QStringLiteral("Planning/Q3"));
  QCOMPARE(createBody.value(QStringLiteral("start"))
               .toObject()
               .value(QStringLiteral("dateTime"))
               .toString(),
           QStringLiteral("2026-08-01T01:00:00.000Z"));
  QCOMPARE(QUrlQuery(createRequest->request.url()).queryItemValue(QStringLiteral("sendUpdates")),
           QStringLiteral("externalOnly"));
  QCOMPARE(QUrlQuery(createRequest->request.url()).queryItemValue(QStringLiteral("conferenceDataVersion")),
           QStringLiteral("1"));
  QCOMPARE(QUrlQuery(createRequest->request.url()).queryItemValue(QStringLiteral("supportsAttachments")),
           QStringLiteral("true"));
  QCOMPARE(createBody.value(QStringLiteral("attendees")).toArray().at(0)
               .toObject()
               .value(QStringLiteral("email"))
               .toString(),
           QStringLiteral("guest@example.com"));
  QCOMPARE(createBody.value(QStringLiteral("reminders")).toObject()
               .value(QStringLiteral("overrides"))
               .toArray()
               .at(0)
               .toObject()
               .value(QStringLiteral("minutes"))
               .toInteger(),
           10);
  QCOMPARE(createBody.value(QStringLiteral("recurrence")).toArray(),
           createdEvent.value(QStringLiteral("recurrence")).toArray());
  QCOMPARE(createBody.value(QStringLiteral("conferenceData")).toObject()
               .value(QStringLiteral("createRequest"))
               .toObject()
               .value(QStringLiteral("requestId"))
               .toString(),
           QStringLiteral("request-1"));
  QCOMPARE(createBody.value(QStringLiteral("attachments")).toArray().size(), 1);
  QVERIFY(createBody.value(QStringLiteral("guestsCanModify")).toBool());
  QCOMPARE(updateRequest->request.url().path(),
           QStringLiteral("/calendar/v3/calendars/calendar-1/events/remote-1"));
  QCOMPARE(updateRequest->request.rawHeader("If-Match"), QByteArray("etag-1"));
  const QJsonObject updateBody = QJsonDocument::fromJson(updateRequest->body).object();
  QVERIFY(updateBody.value(QStringLiteral("description")).isNull());
  QVERIFY(updateBody.value(QStringLiteral("colorId")).isNull());
  QCOMPARE(updateBody.value(QStringLiteral("location")).toString(), QStringLiteral("Remote"));
  QVERIFY(updateBody.value(QStringLiteral("attendeesOmitted")).toBool());
  QCOMPARE(updateBody.value(QStringLiteral("attendees")).toArray().size(), 1);
  QCOMPARE(QUrlQuery(updateRequest->request.url()).queryItemValue(QStringLiteral("sendUpdates")),
           QStringLiteral("none"));
  QCOMPARE(deleteRequest->request.url().path(),
           QStringLiteral("/calendar/v3/calendars/calendar-1/events/remote-1"));
  QCOMPARE(deleteRequest->request.rawHeader("If-Match"), QByteArray("etag-1"));
  QCOMPARE(find(coordinator, created.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, updated.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, removed.id).status, hcb::PendingMutationStatus::Applied);
}

void GoogleCalendarEventMutationPushServiceTest::resolvesGeneratedInstanceBeforeApplyingScopedUpdate() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  const hcb::PendingMutation mutation = enqueue(
      coordinator,
      QStringLiteral("event.instance.update"),
      {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
       {QStringLiteral("localCalendarId"), QStringLiteral("calendar-local")},
       {QStringLiteral("localEventId"), QStringLiteral("event-local-instance")},
       {QStringLiteral("recurringRemoteId"), QStringLiteral("series-remote")},
       {QStringLiteral("originalStartAt"), QStringLiteral("2026-08-02T09:00:00.000Z")},
       {QStringLiteral("event"),
        QJsonObject{{QStringLiteral("description"), QStringLiteral("Changed instance")}}}},
      QStringLiteral("event-local-instance"));
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(
                      R"({"items":[{"id":"instance-remote","etag":"instance-etag","status":"confirmed","recurringEventId":"series-remote","originalStartTime":{"dateTime":"2026-08-02T09:00:00.000Z"}}]})")});
  manager.enqueue({.body = QByteArray(R"({"id":"instance-remote","etag":"new-instance-etag"})")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});

  const hcb::GoogleCalendarEventMutationPushResult result = push(service);
  QCOMPARE(result.applied, 1);
  QCOMPARE(result.failed, 0);
  QCOMPARE(manager.requests().size(), 2);
  if (manager.requests().size() != 2) {
    return;
  }
  const hcb::test::CapturedNetworkRequest& lookup = manager.requests().at(0);
  const hcb::test::CapturedNetworkRequest& patch = manager.requests().at(1);
  QCOMPARE(lookup.operation, QNetworkAccessManager::GetOperation);
  QCOMPARE(lookup.request.url().path(),
           QStringLiteral("/calendar/v3/calendars/calendar-1/events/series-remote/instances"));
  QCOMPARE(QUrlQuery(lookup.request.url()).queryItemValue(QStringLiteral("originalStart")),
           QStringLiteral("2026-08-02T09:00:00.000Z"));
  QCOMPARE(patch.operation, QNetworkAccessManager::CustomOperation);
  QCOMPARE(patch.request.url().path(),
           QStringLiteral("/calendar/v3/calendars/calendar-1/events/instance-remote"));
  QCOMPARE(QString::fromUtf8(patch.request.rawHeader("If-Match")), QStringLiteral("instance-etag"));
  QCOMPARE(QJsonDocument::fromJson(patch.body).object().value(QStringLiteral("description")).toString(),
           QStringLiteral("Changed instance"));
  QCOMPARE(find(coordinator, mutation.id).status, hcb::PendingMutationStatus::Applied);
}

void GoogleCalendarEventMutationPushServiceTest::batchesIndependentEventWritesWithPerItemResults() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  verifyReady(coordinator);
  const hcb::PendingMutation first = enqueue(
      coordinator,
      QStringLiteral("event.update"),
      {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
       {QStringLiteral("remoteEventId"), QStringLiteral("remote-1")},
       {QStringLiteral("etag"), QStringLiteral("etag-1")},
       {QStringLiteral("event"), QJsonObject{{QStringLiteral("summary"), QStringLiteral("One")}}}},
      QStringLiteral("event-one"));
  const hcb::PendingMutation second = enqueue(
      coordinator,
      QStringLiteral("event.update"),
      {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
       {QStringLiteral("remoteEventId"), QStringLiteral("remote-2")},
       {QStringLiteral("etag"), QStringLiteral("etag-2")},
       {QStringLiteral("event"), QJsonObject{{QStringLiteral("summary"), QStringLiteral("Two")}}}},
      QStringLiteral("event-two"));
  const hcb::PendingMutation third = enqueue(
      coordinator,
      QStringLiteral("event.update"),
      {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
       {QStringLiteral("remoteEventId"), QStringLiteral("remote-3")},
       {QStringLiteral("etag"), QStringLiteral("etag-3")},
       {QStringLiteral("event"),
        QJsonObject{{QStringLiteral("summary"), QStringLiteral("Three")}}}},
      QStringLiteral("event-three"));
  std::future<hcb::PendingMutationListResult> dueFuture = coordinator.listDue(100);
  const hcb::PendingMutationListResult dueResult = awaitResult(dueFuture);
  QVERIFY(std::holds_alternative<QList<hcb::PendingMutation>>(dueResult));
  if (!std::holds_alternative<QList<hcb::PendingMutation>>(dueResult)) {
    return;
  }
  const QList<hcb::PendingMutation>& due = std::get<QList<hcb::PendingMutation>>(dueResult);
  QCOMPARE(due.size(), 3);
  const QByteArray batchResponse = QByteArrayLiteral(
      "--batch_response\r\n"
      "Content-Type: application/http\r\n"
      "Content-ID: <response-item-2>\r\n\r\n"
      "HTTP/1.1 412 Precondition Failed\r\n"
      "Content-Type: application/json\r\n\r\n"
      "{\"error\":{\"message\":\"stale\"}}\r\n"
      "--batch_response\r\n"
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
  hcb::GoogleCalendarEventMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});

  const hcb::GoogleCalendarEventMutationPushResult result = push(service);
  QCOMPARE(result.applied, 1);
  QCOMPARE(result.failed, 2);
  QCOMPARE(manager.requests().size(), 1);
  const hcb::test::CapturedNetworkRequest& request = manager.requests().constFirst();
  QCOMPARE(request.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(request.request.url().path(), QStringLiteral("/batch/calendar/v3"));
  QVERIFY(request.request.rawHeader("Content-Type").startsWith("multipart/mixed; boundary=hcb_"));
  QCOMPARE(request.request.rawHeader("Accept"), QByteArray("multipart/mixed"));
  QVERIFY(request.body.contains(
      "PATCH /calendar/v3/calendars/calendar-1/events/remote-1?sendUpdates=all HTTP/1.1"));
  QVERIFY(request.body.contains(
      "PATCH /calendar/v3/calendars/calendar-1/events/remote-2?sendUpdates=all HTTP/1.1"));
  QVERIFY(request.body.contains(
      "PATCH /calendar/v3/calendars/calendar-1/events/remote-3?sendUpdates=all HTTP/1.1"));
  QVERIFY(request.body.contains("If-Match: etag-1"));
  QVERIFY(request.body.contains("If-Match: etag-2"));
  const hcb::PendingMutation applied = find(coordinator, due.at(0).id);
  const hcb::PendingMutation retriable = find(coordinator, due.at(1).id);
  const hcb::PendingMutation conflicted = find(coordinator, due.at(2).id);
  QCOMPARE(applied.status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(retriable.status, hcb::PendingMutationStatus::Failed);
  QCOMPARE(retriable.lastErrorCode, std::optional<QString>(QStringLiteral("server")));
  QCOMPARE(retriable.nextRetryAt,
           std::optional<QString>(QStringLiteral("2025-07-25T01:46:43.123Z")));
  QCOMPARE(conflicted.status, hcb::PendingMutationStatus::Failed);
  QCOMPARE(conflicted.lastErrorCode,
           std::optional<QString>(QStringLiteral("precondition_failed")));
  QVERIFY(!conflicted.nextRetryAt.has_value());
  Q_UNUSED(first);
  Q_UNUSED(second);
  Q_UNUSED(third);
}

void GoogleCalendarEventMutationPushServiceTest::batchesIndependentEventCreatesAndMoves() {
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
      QStringLiteral("event.create"),
      {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
       {QStringLiteral("localEventId"), QStringLiteral("event-created")},
       {QStringLiteral("event"),
        QJsonObject{
            {QStringLiteral("summary"), QStringLiteral("Created")},
            {QStringLiteral("start"),
             QJsonObject{{QStringLiteral("dateTime"), QStringLiteral("2026-08-01T09:00:00Z")}}},
            {QStringLiteral("end"),
             QJsonObject{{QStringLiteral("dateTime"), QStringLiteral("2026-08-01T10:00:00Z")}}}}}},
      QStringLiteral("event-created"));
  const hcb::PendingMutation moved =
      enqueue(coordinator,
              QStringLiteral("event.move"),
              {{QStringLiteral("calendarId"), QStringLiteral("calendar-2")},
               {QStringLiteral("localEventId"), QStringLiteral("event-moved")},
               {QStringLiteral("sourceCalendarId"), QStringLiteral("calendar-1")},
               {QStringLiteral("destinationCalendarId"), QStringLiteral("calendar-2")},
               {QStringLiteral("remoteEventId"), QStringLiteral("remote-event")}},
              QStringLiteral("event-moved"));
  const QByteArray batchResponse = QByteArrayLiteral(
      "--batch_response\r\nContent-Type: application/http\r\nContent-ID: <response-item-0>\r\n\r\n"
      "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"id\":\"created-remote\"}\r\n"
      "--batch_response\r\nContent-Type: application/http\r\nContent-ID: <response-item-1>\r\n\r\n"
      "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"id\":\"moved-remote\"}\r\n"
      "--batch_response--\r\n");
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = batchResponse});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});

  const hcb::GoogleCalendarEventMutationPushResult result = push(service);
  QCOMPARE(result.applied, 2);
  QCOMPARE(result.failed, 0);
  QCOMPARE(manager.requests().size(), 1);
  const QByteArray body = manager.requests().constFirst().body;
  QVERIFY(body.contains(
      "POST /calendar/v3/calendars/calendar-1/events?sendUpdates=all HTTP/1.1"));
  QVERIFY(body.contains(
      "POST /calendar/v3/calendars/calendar-1/events/remote-event/move?destination=calendar-2 "
      "HTTP/1.1"));
  QCOMPARE(find(coordinator, created.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, moved.id).status, hcb::PendingMutationStatus::Applied);
}

void GoogleCalendarEventMutationPushServiceTest::pushesMoveBeforeDependentPatch() {
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
      QStringLiteral("event.move"),
      {{QStringLiteral("calendarId"), QStringLiteral("destination-calendar")},
       {QStringLiteral("sourceCalendarId"), QStringLiteral("source-calendar")},
       {QStringLiteral("destinationCalendarId"), QStringLiteral("destination-calendar")},
       {QStringLiteral("remoteEventId"), QStringLiteral("remote-event")},
       {QStringLiteral("etag"), QStringLiteral("etag-move")}});
  const hcb::PendingMutation updated = enqueue(
      coordinator,
      QStringLiteral("event.update"),
      {{QStringLiteral("calendarId"), QStringLiteral("destination-calendar")},
       {QStringLiteral("remoteEventId"), QStringLiteral("remote-event")},
       {QStringLiteral("etag"), QStringLiteral("etag-after-move")},
       {QStringLiteral("dependsOnMutationId"), moved.id},
       {QStringLiteral("event"),
        QJsonObject{{QStringLiteral("summary"), QStringLiteral("Moved event")}}}});
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{}")});
  manager.enqueue({.body = QByteArray("{}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{});

  const hcb::GoogleCalendarEventMutationPushResult result = push(service);
  QCOMPARE(result.applied, 2);
  QCOMPARE(result.failed, 0);
  QCOMPARE(manager.requests().size(), 2);
  if (manager.requests().size() != 2) {
    return;
  }
  const hcb::test::CapturedNetworkRequest& moveRequest = manager.requests().at(0);
  QCOMPARE(moveRequest.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(moveRequest.request.url().path(),
           QStringLiteral("/calendar/v3/calendars/source-calendar/events/remote-event/move"));
  QCOMPARE(QUrlQuery(moveRequest.request.url()).queryItemValue(QStringLiteral("destination")),
           QStringLiteral("destination-calendar"));
  QCOMPARE(moveRequest.request.rawHeader("If-Match"), QByteArray("etag-move"));
  QVERIFY(moveRequest.body.isEmpty());
  const hcb::test::CapturedNetworkRequest& updateRequest = manager.requests().at(1);
  QCOMPARE(updateRequest.operation, QNetworkAccessManager::CustomOperation);
  QCOMPARE(updateRequest.request.url().path(),
           QStringLiteral("/calendar/v3/calendars/destination-calendar/events/remote-event"));
  QCOMPARE(updateRequest.request.rawHeader("If-Match"), QByteArray("etag-after-move"));
  QCOMPARE(find(coordinator, moved.id).status, hcb::PendingMutationStatus::Applied);
  QCOMPARE(find(coordinator, updated.id).status, hcb::PendingMutationStatus::Applied);
}

void GoogleCalendarEventMutationPushServiceTest::reconcilesCreatedEventIdentity() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator coordinator(database->databasePath(), clock);
  hcb::CalendarMutationService calendarMutations(database->databasePath(), clock);
  verifyReady(coordinator);
  verifyReady(calendarMutations);
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
          "missing_scopes_json, updated_at) VALUES ('account-a', 'google', 'connected', '[]', "
          "'[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_calendars (id, account_id, remote_id, title, updated_at) VALUES "
          "('calendar-local', 'account-a', 'calendar-remote', 'Calendar', "
          "'2026-07-25T00:00:00Z')");
  std::future<hcb::CalendarEventMutationResult> created = calendarMutations.create(
      {.calendarId = QStringLiteral("calendar-local"),
       .title = QStringLiteral("Created"),
       .startAt = QStringLiteral("2026-08-01T09:00:00Z"),
       .endAt = QStringLiteral("2026-08-01T10:00:00Z")});
  const hcb::CalendarEventMutationResult createdResult = awaitResult(created);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(createdResult));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(createdResult)) {
    return;
  }
  const QString eventId = std::get<hcb::CalendarEventMutationReceipt>(createdResult).eventId;
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(R"({"id":"remote-created","etag":"etag-created"})")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventMutationPushService service(
      coordinator, httpClient, clock, hcb::SyncBackoffPolicy{}, &calendarMutations);

  const hcb::GoogleCalendarEventMutationPushResult result = push(service);
  QCOMPARE(result.applied, 1);
  QCOMPARE(result.failed, 0);
  QCOMPARE(readEventRemoteId(handle, eventId), std::optional<QString>(QStringLiteral("remote-created")));
}

void GoogleCalendarEventMutationPushServiceTest::recordsPermanentAndRetriableFailures() {
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
      QStringLiteral("event.create"),
      {{QStringLiteral("event"), QJsonObject{{QStringLiteral("summary"), QStringLiteral("x")}}}});
  const hcb::PendingMutation unavailable = enqueue(
      coordinator,
      QStringLiteral("event.update"),
      {{QStringLiteral("calendarId"), QStringLiteral("calendar-1")},
       {QStringLiteral("remoteEventId"), QStringLiteral("remote-1")},
       {QStringLiteral("event"), QJsonObject{{QStringLiteral("summary"), QStringLiteral("x")}}}});
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 503,
                   .body = QByteArray("{}"),
                   .error = QNetworkReply::UnknownServerError,
                   .headers = {{QByteArray("Retry-After"), QByteArray("3")}}});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventMutationPushService service(
      coordinator,
      httpClient,
      clock,
      hcb::SyncBackoffPolicy({.baseDelayMilliseconds = 1'000,
                              .maximumDelayMilliseconds = 10'000,
                              .jitterMilliseconds = 0,
                              .maximumAttempts = 6,
                              .random = [] { return 0.0; }}));

  const hcb::GoogleCalendarEventMutationPushResult result = push(service);
  QCOMPARE(result.applied, 0);
  QCOMPARE(result.failed, 2);
  QCOMPARE(manager.requests().size(), 1);
  const hcb::PendingMutation invalidResult = find(coordinator, invalid.id);
  QCOMPARE(invalidResult.status, hcb::PendingMutationStatus::Failed);
  QCOMPARE(invalidResult.lastErrorCode, std::optional<QString>(QStringLiteral("invalid_payload")));
  QVERIFY(!invalidResult.nextRetryAt.has_value());
  const hcb::PendingMutation unavailableResult = find(coordinator, unavailable.id);
  QCOMPARE(unavailableResult.status, hcb::PendingMutationStatus::Failed);
  QCOMPARE(unavailableResult.lastErrorCode, std::optional<QString>(QStringLiteral("server")));
  QCOMPARE(unavailableResult.nextRetryAt,
           std::optional<QString>(QStringLiteral("2025-07-25T01:46:43.123Z")));
}

QTEST_GUILESS_MAIN(GoogleCalendarEventMutationPushServiceTest)

#include "GoogleCalendarEventMutationPushServiceTest.moc"
