#include <QtTest/QTest>

#include "core/Clock.h"
#include "core/GoogleHttpClient.h"
#include "core/GoogleSyncConflictResolver.h"
#include "core/OptimisticMutationCoordinator.h"
#include "core/SyncConflictStore.h"
#include "core/TaskRecurrenceMarker.h"
#include "support/MockNetworkAccessManager.h"
#include "support/TemporarySqliteDatabase.h"

#include <QCoreApplication>
#include <QEventLoop>
#include <QJsonDocument>
#include <QJsonObject>

#include <chrono>
#include <future>
#include <memory>
#include <optional>
#include <thread>
#include <utility>
#include <variant>

using namespace std::chrono_literals;

class GoogleSyncConflictResolverTest final : public QObject {
  Q_OBJECT

private slots:
  void preferGoogleCancelsAndRecordsResolution();
  void preferHcbRebasesWithFreshRemoteState();
  void preferHcbRetainsManagedRecurrenceMarkerOnConflict();
  void rebaseCalendarEventWithFreshRemoteState();
  void askEachTimeLeavesAResolvableConflict();
  void rejectsMalformedGoogleTaskResponse();
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
    qFatal("sync conflict resolver request timed out");
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

void verifyReady(const std::shared_future<hcb::SqliteWriteResult>& ready) {
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

[[nodiscard]] hcb::PendingMutation enqueueAndClaim(hcb::OptimisticMutationCoordinator& coordinator) {
  QJsonObject task{{QStringLiteral("title"), QStringLiteral("Local")},
                   {QStringLiteral("notes"), QStringLiteral("Base notes")},
                   {QStringLiteral("status"), QStringLiteral("needsAction")}};
  QJsonObject payload{{QStringLiteral("taskListId"), QStringLiteral("task-list")},
                      {QStringLiteral("remoteTaskId"), QStringLiteral("task-remote")},
                      {QStringLiteral("task"), task}};
  std::future<hcb::PendingMutationResult> enqueued = coordinator.enqueue(
      {.resource = hcb::PendingMutationResource::Task,
       .resourceId = QStringLiteral("task-local"),
       .operation = QStringLiteral("task.update"),
       .payload = std::move(payload),
       .baseSnapshot = {{QStringLiteral("title"), QStringLiteral("Base")},
                        {QStringLiteral("notes"), QStringLiteral("Base notes")},
                        {QStringLiteral("status"), QStringLiteral("needsAction")},
                        {QStringLiteral("due"), QJsonValue::Null}},
       .remoteEtag = QStringLiteral("etag-stale")});
  const hcb::PendingMutationResult enqueuedResult = awaitResult(enqueued);
  if (!std::holds_alternative<hcb::PendingMutation>(enqueuedResult)) {
    qFatal("sync conflict mutation enqueue failed");
  }
  const hcb::PendingMutation pending = std::get<hcb::PendingMutation>(enqueuedResult);
  std::future<hcb::PendingMutationResult> claimed = coordinator.claim(pending.id, 30s);
  const hcb::PendingMutationResult claimedResult = awaitResult(claimed);
  if (!std::holds_alternative<hcb::PendingMutation>(claimedResult)) {
    qFatal("sync conflict mutation claim failed");
  }
  return std::get<hcb::PendingMutation>(claimedResult);
}

[[nodiscard]] hcb::PendingMutation
enqueueAndClaimEvent(hcb::OptimisticMutationCoordinator& coordinator) {
  const QJsonObject start{{QStringLiteral("dateTime"), QStringLiteral("2025-07-25T10:00:00.000Z")}};
  const QJsonObject end{{QStringLiteral("dateTime"), QStringLiteral("2025-07-25T11:00:00.000Z")}};
  const QJsonObject event{{QStringLiteral("summary"), QStringLiteral("Local")},
                          {QStringLiteral("description"), QStringLiteral("Base description")},
                          {QStringLiteral("start"), start},
                          {QStringLiteral("end"), end}};
  std::future<hcb::PendingMutationResult> enqueued = coordinator.enqueue(
      {.resource = hcb::PendingMutationResource::Event,
       .resourceId = QStringLiteral("event-local"),
       .operation = QStringLiteral("event.update"),
       .payload = {{QStringLiteral("calendarId"), QStringLiteral("calendar-remote")},
                   {QStringLiteral("remoteEventId"), QStringLiteral("event-remote")},
                   {QStringLiteral("localEventId"), QStringLiteral("event-local")},
                   {QStringLiteral("event"), event}},
       .baseSnapshot = {{QStringLiteral("summary"), QStringLiteral("Base")},
                        {QStringLiteral("description"), QStringLiteral("Base description")},
                        {QStringLiteral("location"), QJsonValue::Null},
                        {QStringLiteral("start"), start},
                        {QStringLiteral("end"), end}},
       .remoteEtag = QStringLiteral("event-etag-stale")});
  const hcb::PendingMutationResult enqueuedResult = awaitResult(enqueued);
  if (!std::holds_alternative<hcb::PendingMutation>(enqueuedResult)) {
    qFatal("calendar conflict mutation enqueue failed");
  }
  const hcb::PendingMutation pending = std::get<hcb::PendingMutation>(enqueuedResult);
  std::future<hcb::PendingMutationResult> claimed = coordinator.claim(pending.id, 30s);
  const hcb::PendingMutationResult claimedResult = awaitResult(claimed);
  if (!std::holds_alternative<hcb::PendingMutation>(claimedResult)) {
    qFatal("calendar conflict mutation claim failed");
  }
  return std::get<hcb::PendingMutation>(claimedResult);
}

[[nodiscard]] QByteArray remoteTask(QString title,
                                    QString notes = QStringLiteral("Base notes")) {
  return QJsonDocument(QJsonObject{{QStringLiteral("id"), QStringLiteral("task-remote")},
                                   {QStringLiteral("etag"), QStringLiteral("etag-remote")},
                                   {QStringLiteral("title"), std::move(title)},
                                   {QStringLiteral("notes"), std::move(notes)},
                                   {QStringLiteral("status"), QStringLiteral("needsAction")}})
      .toJson(QJsonDocument::Compact);
}

[[nodiscard]] QByteArray remoteEvent(QString summary) {
  return QJsonDocument(
             QJsonObject{{QStringLiteral("id"), QStringLiteral("event-remote")},
                         {QStringLiteral("etag"), QStringLiteral("event-etag-remote")},
                         {QStringLiteral("summary"), std::move(summary)},
                         {QStringLiteral("description"), QStringLiteral("Base description")},
                         {QStringLiteral("start"),
                          QJsonObject{{QStringLiteral("dateTime"),
                                       QStringLiteral("2025-07-25T10:00:00Z")}}},
                         {QStringLiteral("end"),
                          QJsonObject{{QStringLiteral("dateTime"),
                                       QStringLiteral("2025-07-25T11:00:00Z")}}}})
      .toJson(QJsonDocument::Compact);
}

[[nodiscard]] hcb::GoogleSyncConflictResult resolveInWorker(
    hcb::GoogleSyncConflictResolver& resolver, hcb::PendingMutation mutation) {
  std::future<hcb::GoogleSyncConflictResult> future = std::async(
      std::launch::async,
      [&resolver, mutation = std::move(mutation)]() mutable {
        return resolver.handle(std::move(mutation),
                               QStringLiteral("precondition_failed"),
                               QStringLiteral("Google resource changed"),
                               QStringLiteral("access-token"));
      });
  return awaitResult(future);
}

[[nodiscard]] hcb::PendingMutation find(hcb::OptimisticMutationCoordinator& coordinator,
                                        const QString& id) {
  std::future<hcb::PendingMutationLookupResult> future = coordinator.find(id);
  const hcb::PendingMutationLookupResult result = awaitResult(future);
  if (!std::holds_alternative<std::optional<hcb::PendingMutation>>(result) ||
      !std::get<std::optional<hcb::PendingMutation>>(result).has_value()) {
    qFatal("sync conflict mutation lookup failed");
  }
  return *std::get<std::optional<hcb::PendingMutation>>(result);
}

} // namespace

void GoogleSyncConflictResolverTest::preferGoogleCancelsAndRecordsResolution() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator mutations(database->databasePath(), clock);
  hcb::SyncConflictStore conflicts(database->databasePath(), clock);
  verifyReady(mutations.ready());
  verifyReady(conflicts.ready());
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 200, .body = remoteTask(QStringLiteral("Remote"))});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleSyncConflictResolver resolver(mutations, conflicts, http);
  const hcb::PendingMutation claimed = enqueueAndClaim(mutations);
  const hcb::GoogleSyncConflictResult result = resolveInWorker(resolver, claimed);
  QVERIFY(std::holds_alternative<hcb::GoogleSyncConflictOutcome>(result));
  QCOMPARE(std::get<hcb::GoogleSyncConflictOutcome>(result),
           hcb::GoogleSyncConflictOutcome::KeptRemote);
  QCOMPARE(find(mutations, claimed.id).status, hcb::PendingMutationStatus::Cancelled);
  std::future<hcb::SyncConflictListResult> listed = conflicts.listUnresolved();
  const hcb::SyncConflictListResult unresolved = awaitResult(listed);
  QVERIFY(std::holds_alternative<QList<hcb::SyncConflict>>(unresolved));
  QVERIFY(std::get<QList<hcb::SyncConflict>>(unresolved).isEmpty());
  QCOMPARE(manager.requests().size(), 1);
  QCOMPARE(manager.requests().constFirst().request.url().path(),
           QStringLiteral("/tasks/v1/lists/task-list/tasks/task-remote"));
}

void GoogleSyncConflictResolverTest::preferHcbRebasesWithFreshRemoteState() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator mutations(database->databasePath(), clock);
  hcb::SyncConflictStore conflicts(database->databasePath(), clock);
  verifyReady(mutations.ready());
  verifyReady(conflicts.ready());
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 200, .body = remoteTask(QStringLiteral("Remote"))});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleSyncConflictResolver resolver(mutations, conflicts, http);
  resolver.setPolicy(hcb::SyncConflictPolicy::PreferHcb);
  const hcb::PendingMutation claimed = enqueueAndClaim(mutations);
  const hcb::GoogleSyncConflictResult result = resolveInWorker(resolver, claimed);
  QVERIFY(std::holds_alternative<hcb::GoogleSyncConflictOutcome>(result));
  QCOMPARE(std::get<hcb::GoogleSyncConflictOutcome>(result),
           hcb::GoogleSyncConflictOutcome::ReappliedLocal);
  const hcb::PendingMutation rebased = find(mutations, claimed.id);
  QCOMPARE(rebased.status, hcb::PendingMutationStatus::Pending);
  QCOMPARE(rebased.remoteEtag, std::optional<QString>(QStringLiteral("etag-remote")));
  QCOMPARE(rebased.baseSnapshot.value(QStringLiteral("title")), QJsonValue(QStringLiteral("Remote")));
  QCOMPARE(rebased.payload.value(QStringLiteral("task")).toObject().value(QStringLiteral("title")),
           QJsonValue(QStringLiteral("Local")));
}

void GoogleSyncConflictResolverTest::preferHcbRetainsManagedRecurrenceMarkerOnConflict() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  const hcb::TaskRecurrenceMarker marker{
      .seriesId = QStringLiteral("76e1c650-8f6c-447f-a5ed-fb25eebb9ea9"),
      .occurrenceId = QStringLiteral("76e1c650-8f6c-447f-a5ed-fb25eebb9ea9:0"),
      .frequency = hcb::TaskRecurrenceFrequency::Weekly,
      .anchorDate = QStringLiteral("2026-07-26"),
      .timeZone = QStringLiteral("Asia/Singapore"),
      .templateTitle = QStringLiteral("Local"),
      .templateDueDate = QStringLiteral("2026-07-26"),
      .templatePriority = QStringLiteral("medium")};
  const hcb::TaskRecurrenceSerializationResult serialized =
      hcb::serializeTaskRecurrenceNotes(QStringLiteral("Local notes"), marker);
  QVERIFY(!serialized.error.has_value());
  if (serialized.error.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator mutations(database->databasePath(), clock);
  hcb::SyncConflictStore conflicts(database->databasePath(), clock);
  verifyReady(mutations.ready());
  verifyReady(conflicts.ready());
  const QJsonObject task{{QStringLiteral("title"), QStringLiteral("Local")},
                         {QStringLiteral("notes"), serialized.notes},
                         {QStringLiteral("status"), QStringLiteral("needsAction")}};
  std::future<hcb::PendingMutationResult> enqueued = mutations.enqueue(
      {.resource = hcb::PendingMutationResource::Task,
       .resourceId = QStringLiteral("task-local"),
       .operation = QStringLiteral("task.update"),
       .payload = {{QStringLiteral("taskListId"), QStringLiteral("task-list")},
                   {QStringLiteral("remoteTaskId"), QStringLiteral("task-remote")},
                   {QStringLiteral("task"), task}},
       .baseSnapshot = {{QStringLiteral("title"), QStringLiteral("Base")},
                        {QStringLiteral("notes"), QStringLiteral("Base notes")},
                        {QStringLiteral("status"), QStringLiteral("needsAction")},
                        {QStringLiteral("due"), QJsonValue::Null}},
       .remoteEtag = QStringLiteral("etag-stale")});
  const hcb::PendingMutationResult enqueuedResult = awaitResult(enqueued);
  QVERIFY(std::holds_alternative<hcb::PendingMutation>(enqueuedResult));
  if (!std::holds_alternative<hcb::PendingMutation>(enqueuedResult)) {
    return;
  }
  const hcb::PendingMutation pending = std::get<hcb::PendingMutation>(enqueuedResult);
  std::future<hcb::PendingMutationResult> claimed = mutations.claim(pending.id, 30s);
  const hcb::PendingMutationResult claimedResult = awaitResult(claimed);
  QVERIFY(std::holds_alternative<hcb::PendingMutation>(claimedResult));
  if (!std::holds_alternative<hcb::PendingMutation>(claimedResult)) {
    return;
  }
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 200,
                   .body = remoteTask(QStringLiteral("Remote"), QStringLiteral("Remote notes"))});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleSyncConflictResolver resolver(mutations, conflicts, http);
  resolver.setPolicy(hcb::SyncConflictPolicy::PreferHcb);
  const hcb::GoogleSyncConflictResult result =
      resolveInWorker(resolver, std::get<hcb::PendingMutation>(claimedResult));
  QVERIFY(std::holds_alternative<hcb::GoogleSyncConflictOutcome>(result));
  QCOMPARE(std::get<hcb::GoogleSyncConflictOutcome>(result),
           hcb::GoogleSyncConflictOutcome::ReappliedLocal);
  const hcb::PendingMutation rebased = find(mutations, pending.id);
  QCOMPARE(rebased.remoteEtag, std::optional<QString>(QStringLiteral("etag-remote")));
  QCOMPARE(rebased.baseSnapshot.value(QStringLiteral("notes")), QJsonValue(QStringLiteral("Remote notes")));
  const hcb::TaskRecurrenceNotes notes = hcb::parseTaskRecurrenceNotes(
      rebased.payload.value(QStringLiteral("task")).toObject().value(QStringLiteral("notes")).toString());
  QCOMPARE(notes.state, hcb::TaskRecurrenceNotesState::Managed);
  QVERIFY(notes.marker.has_value());
  if (notes.marker.has_value()) {
    QCOMPARE(notes.marker->seriesId, marker.seriesId);
    QCOMPARE(notes.marker->occurrenceId, marker.occurrenceId);
  }
}

void GoogleSyncConflictResolverTest::rebaseCalendarEventWithFreshRemoteState() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator mutations(database->databasePath(), clock);
  hcb::SyncConflictStore conflicts(database->databasePath(), clock);
  verifyReady(mutations.ready());
  verifyReady(conflicts.ready());
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 200, .body = remoteEvent(QStringLiteral("Remote"))});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleSyncConflictResolver resolver(mutations, conflicts, http);
  resolver.setPolicy(hcb::SyncConflictPolicy::PreferHcb);
  const hcb::PendingMutation claimed = enqueueAndClaimEvent(mutations);
  const hcb::GoogleSyncConflictResult result = resolveInWorker(resolver, claimed);
  QVERIFY(std::holds_alternative<hcb::GoogleSyncConflictOutcome>(result));
  QCOMPARE(std::get<hcb::GoogleSyncConflictOutcome>(result),
           hcb::GoogleSyncConflictOutcome::ReappliedLocal);
  const hcb::PendingMutation rebased = find(mutations, claimed.id);
  QCOMPARE(rebased.status, hcb::PendingMutationStatus::Pending);
  QCOMPARE(rebased.remoteEtag, std::optional<QString>(QStringLiteral("event-etag-remote")));
  QCOMPARE(rebased.payload.value(QStringLiteral("event")).toObject().value(QStringLiteral("summary")),
           QJsonValue(QStringLiteral("Local")));
  QCOMPARE(manager.requests().size(), 1);
  QCOMPARE(manager.requests().constFirst().request.url().path(),
           QStringLiteral("/calendar/v3/calendars/calendar-remote/events/event-remote"));
}

void GoogleSyncConflictResolverTest::askEachTimeLeavesAResolvableConflict() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator mutations(database->databasePath(), clock);
  hcb::SyncConflictStore conflicts(database->databasePath(), clock);
  verifyReady(mutations.ready());
  verifyReady(conflicts.ready());
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 200, .body = remoteTask(QStringLiteral("Remote"))});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleSyncConflictResolver resolver(mutations, conflicts, http);
  resolver.setPolicy(hcb::SyncConflictPolicy::AskEachTime);
  const hcb::PendingMutation claimed = enqueueAndClaim(mutations);
  const hcb::GoogleSyncConflictResult result = resolveInWorker(resolver, claimed);
  QVERIFY(std::holds_alternative<hcb::GoogleSyncConflictOutcome>(result));
  QCOMPARE(std::get<hcb::GoogleSyncConflictOutcome>(result),
           hcb::GoogleSyncConflictOutcome::AwaitingUser);
  QCOMPARE(find(mutations, claimed.id).status, hcb::PendingMutationStatus::Failed);
  std::future<hcb::SyncConflictListResult> listed = conflicts.listUnresolved();
  const hcb::SyncConflictListResult unresolved = awaitResult(listed);
  QVERIFY(std::holds_alternative<QList<hcb::SyncConflict>>(unresolved));
  if (!std::holds_alternative<QList<hcb::SyncConflict>>(unresolved) ||
      std::get<QList<hcb::SyncConflict>>(unresolved).isEmpty()) {
    return;
  }
  const QString conflictId = std::get<QList<hcb::SyncConflict>>(unresolved).constFirst().id;
  std::future<std::optional<hcb::AppError>> resolution =
      resolver.resolve(conflictId, hcb::SyncConflictResolution::KeepRemote);
  QVERIFY(!awaitResult(resolution).has_value());
  QCOMPARE(find(mutations, claimed.id).status, hcb::PendingMutationStatus::Cancelled);
}

void GoogleSyncConflictResolverTest::rejectsMalformedGoogleTaskResponse() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::OptimisticMutationCoordinator mutations(database->databasePath(), clock);
  hcb::SyncConflictStore conflicts(database->databasePath(), clock);
  verifyReady(mutations.ready());
  verifyReady(conflicts.ready());
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 200,
                   .body = QByteArray(
                       R"({"id":"task-remote","etag":"etag-remote","title":"Remote","notes":7})")});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleSyncConflictResolver resolver(mutations, conflicts, http);
  const hcb::PendingMutation claimed = enqueueAndClaim(mutations);
  const hcb::GoogleSyncConflictResult result = resolveInWorker(resolver, claimed);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
  QCOMPARE(find(mutations, claimed.id).status, hcb::PendingMutationStatus::Applying);
  std::future<hcb::SyncConflictListResult> listed = conflicts.listUnresolved();
  const hcb::SyncConflictListResult unresolved = awaitResult(listed);
  QVERIFY(std::holds_alternative<QList<hcb::SyncConflict>>(unresolved));
  QVERIFY(std::get<QList<hcb::SyncConflict>>(unresolved).isEmpty());
}

QTEST_MAIN(GoogleSyncConflictResolverTest)

#include "GoogleSyncConflictResolverTest.moc"
