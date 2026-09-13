#include <QDateTime>
#include <QDir>
#include <QFileInfo>
#include <QJsonArray>
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

#include "core/CalendarMutationService.h"
#include "core/CalendarEventBulkMutationService.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

using namespace std::chrono_literals;

namespace {

class FixedClock final : public hcb::Clock {
public:
  explicit FixedClock(hcb::WallTimePoint wallTime) : wallTime_(wallTime) {}
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

private:
  hcb::WallTimePoint wallTime_;
};

struct EventSnapshot final {
  QString remoteId;
  QString calendarId;
  QString title;
  std::optional<QString> description;
  std::optional<QString> location;
  QString startAt;
  std::optional<QString> startTimeZone;
  QString endAt;
  std::optional<QString> endTimeZone;
  std::optional<QString> colorId;
  std::optional<QString> transparency;
  std::optional<QString> visibility;
  bool allDay;
  std::optional<QString> deletedAt;
  QString updatedAt;
};

struct PendingMutationSnapshot final {
  QString id;
  QString operation;
  QJsonObject payload;
};

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("calendar mutation service request timed out");
  }
  return future.get();
}

void verifyReady(hcb::CalendarMutationService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

void execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  const QString message = errorMessage == nullptr ? QString() : QString::fromUtf8(errorMessage);
  sqlite3_free(errorMessage);
  QVERIFY2(result == SQLITE_OK, qPrintable(message));
}

void seed(hcb::SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z'), "
          "('account-b', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(
      handle,
      "INSERT INTO local_calendars (id, account_id, remote_id, title, updated_at, deleted_at) "
      "VALUES "
      "('calendar-work', 'account-a', 'work', 'Work', '2026-07-25T00:00:00Z', NULL), "
      "('calendar-other', 'account-a', 'other', 'Other', '2026-07-25T00:00:00Z', NULL), "
      "('calendar-cross-account', 'account-b', 'cross', 'Cross', '2026-07-25T00:00:00Z', NULL), "
      "('calendar-deleted', 'account-a', 'deleted', 'Deleted', '2026-07-25T00:00:00Z', "
      "'2026-07-25T01:00:00Z')");
}

[[nodiscard]] std::optional<QString> optionalText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int byteCount = sqlite3_column_bytes(statement, index);
  return value == nullptr || byteCount < 0
             ? std::nullopt
             : std::optional<QString>(QString::fromUtf8(value, byteCount));
}

[[nodiscard]] std::optional<EventSnapshot> readEvent(sqlite3* handle, const QString& eventId) {
  constexpr char sql[] = R"(
SELECT remote_id, calendar_id, title, description, location, start_at, start_time_zone, end_at,
       end_time_zone, color_id, transparency, visibility, is_all_day, deleted_at, updated_at
FROM local_calendar_events
WHERE id = ?1
)";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const QByteArray eventIdUtf8 = eventId.toUtf8();
  if (sqlite3_bind_text(statement,
                        1,
                        eventIdUtf8.constData(),
                        static_cast<int>(eventIdUtf8.size()),
                        SQLITE_TRANSIENT) != SQLITE_OK ||
      sqlite3_step(statement) != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const EventSnapshot snapshot{.remoteId = optionalText(statement, 0).value_or(QString()),
                               .calendarId = optionalText(statement, 1).value_or(QString()),
                               .title = optionalText(statement, 2).value_or(QString()),
                               .description = optionalText(statement, 3),
                               .location = optionalText(statement, 4),
                               .startAt = optionalText(statement, 5).value_or(QString()),
                               .startTimeZone = optionalText(statement, 6),
                               .endAt = optionalText(statement, 7).value_or(QString()),
                               .endTimeZone = optionalText(statement, 8),
                               .colorId = optionalText(statement, 9),
                               .transparency = optionalText(statement, 10),
                               .visibility = optionalText(statement, 11),
                               .allDay = sqlite3_column_int(statement, 12) == 1,
                               .deletedAt = optionalText(statement, 13),
                               .updatedAt = optionalText(statement, 14).value_or(QString())};
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK ? std::optional<EventSnapshot>(snapshot) : std::nullopt;
}

[[nodiscard]] QList<PendingMutationSnapshot>
readPendingEventMutations(sqlite3* handle, const QString& eventId) {
  constexpr char sql[] = R"(
SELECT id, operation, payload_json
FROM local_pending_mutations
WHERE resource_type = 'event' AND resource_id = ?1 AND status IN ('pending', 'failed')
ORDER BY created_at ASC, id ASC
)";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    sqlite3_finalize(statement);
    return {};
  }
  const QByteArray eventIdUtf8 = eventId.toUtf8();
  if (sqlite3_bind_text(statement,
                        1,
                        eventIdUtf8.constData(),
                        static_cast<int>(eventIdUtf8.size()),
                        SQLITE_TRANSIENT) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return {};
  }
  QList<PendingMutationSnapshot> mutations;
  int stepResult = SQLITE_ROW;
  while ((stepResult = sqlite3_step(statement)) == SQLITE_ROW) {
    const std::optional<QString> id = optionalText(statement, 0);
    const std::optional<QString> operation = optionalText(statement, 1);
    const std::optional<QString> payloadJson = optionalText(statement, 2);
    QJsonParseError parseError;
    const QJsonDocument payload = payloadJson.has_value()
                                      ? QJsonDocument::fromJson(payloadJson->toUtf8(), &parseError)
                                      : QJsonDocument();
    if (!id.has_value() || !operation.has_value() || !payloadJson.has_value() ||
        parseError.error != QJsonParseError::NoError || !payload.isObject()) {
      sqlite3_finalize(statement);
      return {};
    }
    mutations.append({.id = *id, .operation = *operation, .payload = payload.object()});
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return stepResult == SQLITE_DONE && finalizeResult == SQLITE_OK ? mutations
                                                                    : QList<PendingMutationSnapshot>{};
}

[[nodiscard]] std::int64_t pendingEventMutationCount(sqlite3* handle) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle,
                         "SELECT COUNT(*) FROM local_pending_mutations WHERE resource_type = 'event'",
                         -1,
                         SQLITE_PREPARE_PERSISTENT,
                         &statement,
                         nullptr) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return -1;
  }
  const int stepResult = sqlite3_step(statement);
  const std::int64_t count = sqlite3_column_int64(statement, 0);
  const int finalizeResult = sqlite3_finalize(statement);
  return stepResult == SQLITE_ROW && finalizeResult == SQLITE_OK ? count : -1;
}

} // namespace

class CalendarMutationServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void createsUpdatesMovesAndDeletesEvents();
  void createsBatchAtomically();
  void createsRichGoogleEventsAndCarriesDeliveryPolicy();
  void preservesAdvancedGoogleRecurrenceLines();
  void journalsRemoteUpdatesMovesAndCreateReconciliation();
  void rejectsInvalidAndUnavailableMutations();
  void scopesRecurringSeriesWithoutUnsafeLegacyMutations();
  void bulkClassifiesAndQueuesEligibleEvents();
  void bulkReplacesLiteralEventTextWithPreviewAndScope();
};

void CalendarMutationServiceTest::createsUpdatesMovesAndDeletesEvents() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  const QString expectedTimestamp =
      QDateTime::fromMSecsSinceEpoch(1'753'408'000'123, QTimeZone::UTC).toString(Qt::ISODateWithMs);
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);

  std::future<hcb::CalendarEventMutationResult> create = service.create(
      hcb::CalendarEventCreateInput{.calendarId = QStringLiteral("calendar-work"),
                                    .title = QStringLiteral(" Planning "),
                                    .startAt = QStringLiteral("2026-07-26T09:30:00+08:00"),
                                    .endAt = QStringLiteral("2026-07-26T10:30:00+08:00"),
                                    .description = QStringLiteral("Quarterly review"),
                                    .location = QStringLiteral("Room 5"),
                                    .startTimeZone = QStringLiteral("Asia/Singapore"),
                                    .endTimeZone = QStringLiteral("Asia/Singapore"),
                                    .colorId = QStringLiteral("4")});
  const hcb::CalendarEventMutationResult createResult = awaitResult(create);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(createResult));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(createResult)) {
    return;
  }
  const hcb::CalendarEventMutationReceipt receipt =
      std::get<hcb::CalendarEventMutationReceipt>(createResult);
  QCOMPARE(receipt.updatedAt, expectedTimestamp);
  QVERIFY(receipt.eventId.startsWith(QStringLiteral("event:")));
  const std::optional<EventSnapshot> created = readEvent(handle, receipt.eventId);
  QVERIFY(created.has_value());
  if (!created.has_value()) {
    return;
  }
  QCOMPARE(created->remoteId, QStringLiteral("pending:") + receipt.eventId.mid(6));
  QCOMPARE(created->title, QStringLiteral("Planning"));
  QCOMPARE(created->startAt, QStringLiteral("2026-07-26T01:30:00.000Z"));
  QCOMPARE(created->endAt, QStringLiteral("2026-07-26T02:30:00.000Z"));
  QCOMPARE(created->updatedAt, expectedTimestamp);
  const QList<PendingMutationSnapshot> createMutations =
      readPendingEventMutations(handle, receipt.eventId);
  QCOMPARE(createMutations.size(), 1);
  if (createMutations.size() != 1) {
    return;
  }
  const QJsonObject createPayload = createMutations.constFirst().payload.value(QStringLiteral("event")).toObject();
  QCOMPARE(createPayload.value(QStringLiteral("attendees")).toArray(), QJsonArray());
  QCOMPARE(createPayload.value(QStringLiteral("reminders")).toObject()
               .value(QStringLiteral("useDefault"))
               .toBool(),
           true);
  QCOMPARE(createPayload.value(QStringLiteral("recurrence")).toArray(), QJsonArray());

  const std::optional<std::optional<QString>> clearText{std::optional<QString>{}};
  std::future<hcb::CalendarEventMutationResult> update = service.update(
      hcb::CalendarEventUpdateInput{.eventId = receipt.eventId,
                                    .calendarId = QStringLiteral("calendar-other"),
                                    .title = QStringLiteral(" Review "),
                                    .description = clearText,
                                    .location = clearText,
                                    .startAt = QStringLiteral("2026-07-26T02:00:00Z"),
                                    .allDay = true,
                                    .startTimeZone = clearText,
                                    .endTimeZone = clearText,
                                    .colorId = std::optional<std::optional<QString>>(
                                        std::optional<QString>{})});
  const hcb::CalendarEventMutationResult updateResult = awaitResult(update);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(updateResult));
  const std::optional<EventSnapshot> updated = readEvent(handle, receipt.eventId);
  QVERIFY(updated.has_value());
  if (!updated.has_value()) {
    return;
  }
  QCOMPARE(updated->calendarId, QStringLiteral("calendar-other"));
  QCOMPARE(updated->title, QStringLiteral("Review"));
  QVERIFY(!updated->description.has_value());
  QVERIFY(!updated->location.has_value());
  QCOMPARE(updated->startAt, QStringLiteral("2026-07-26T02:00:00.000Z"));
  QCOMPARE(updated->endAt, QStringLiteral("2026-07-26T02:30:00.000Z"));
  QVERIFY(updated->allDay);
  QVERIFY(!updated->startTimeZone.has_value());
  QVERIFY(!updated->colorId.has_value());

  std::future<hcb::CalendarEventMutationResult> remove = service.remove(receipt.eventId);
  const hcb::CalendarEventMutationResult removeResult = awaitResult(remove);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(removeResult));
  const std::optional<EventSnapshot> removed = readEvent(handle, receipt.eventId);
  QVERIFY(removed.has_value());
  if (!removed.has_value()) {
    return;
  }
  QCOMPARE(removed->deletedAt, std::optional<QString>(expectedTimestamp));
}

void CalendarMutationServiceTest::createsBatchAtomically() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);

  std::future<hcb::CalendarEventBatchMutationResult> rejected = service.createBatch(
      {{.calendarId = QStringLiteral("calendar-work"),
        .title = QStringLiteral("first"),
        .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-26T10:00:00.000Z")},
       {.calendarId = QStringLiteral("calendar-deleted"),
        .title = QStringLiteral("second"),
        .startAt = QStringLiteral("2026-07-26T11:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-26T12:00:00.000Z")}});
  QVERIFY(std::holds_alternative<hcb::AppError>(awaitResult(rejected)));
  QCOMPARE(pendingEventMutationCount(handle), std::int64_t{0});

  std::future<hcb::CalendarEventBatchMutationResult> created = service.createBatch(
      {{.calendarId = QStringLiteral("calendar-work"),
        .title = QStringLiteral("first"),
        .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-26T10:00:00.000Z")},
       {.calendarId = QStringLiteral("calendar-work"),
        .title = QStringLiteral("second"),
        .startAt = QStringLiteral("2026-07-26T11:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-26T12:00:00.000Z")}});
  const hcb::CalendarEventBatchMutationResult result = awaitResult(created);
  QVERIFY(std::holds_alternative<QList<hcb::CalendarEventMutationReceipt>>(result));
  QCOMPARE(std::get<QList<hcb::CalendarEventMutationReceipt>>(result).size(), qsizetype{2});
  QCOMPARE(pendingEventMutationCount(handle), std::int64_t{2});
}

void CalendarMutationServiceTest::createsRichGoogleEventsAndCarriesDeliveryPolicy() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle, "UPDATE local_calendars SET is_primary = 1 WHERE id = 'calendar-work'");

  std::future<hcb::CalendarEventMutationResult> create = service.create(
      {.calendarId = QStringLiteral("calendar-work"),
       .title = QStringLiteral("Focus"),
       .startAt = QStringLiteral("2026-07-26T09:30:00+08:00"),
       .endAt = QStringLiteral("2026-07-26T10:30:00+08:00"),
       .richMetadata = {.createGoogleMeet = true,
                        .attachmentsJson = QStringLiteral(
                            "[{\"fileUrl\":\"https://drive.google.com/open?id=file-1\",\"title\":\"Spec\"}]"),
                        .guestPermissionsJson = QStringLiteral("{\"guestsCanModify\":true}"),
                        .eventType = QStringLiteral("focusTime"),
                        .statusPropertiesJson = QStringLiteral(
                            "{\"focusTimeProperties\":{\"autoDeclineMode\":\"declineNone\",\"chatStatus\":\"available\"}}"),
                        .sendUpdates = QStringLiteral("externalOnly")}});
  const hcb::CalendarEventMutationResult result = awaitResult(create);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(result));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(result)) {
    return;
  }
  const QString eventId = std::get<hcb::CalendarEventMutationReceipt>(result).eventId;
  std::future<hcb::CalendarEventMutationSnapshotResult> inspection = service.inspect({eventId});
  const hcb::CalendarEventMutationSnapshotResult inspectionResult = awaitResult(inspection);
  QVERIFY(std::holds_alternative<QList<hcb::CalendarEventMutationSnapshot>>(inspectionResult));
  if (!std::holds_alternative<QList<hcb::CalendarEventMutationSnapshot>>(inspectionResult)) {
    return;
  }
  const QList<hcb::CalendarEventMutationSnapshot>& snapshots =
      std::get<QList<hcb::CalendarEventMutationSnapshot>>(inspectionResult);
  QCOMPARE(snapshots.size(), 1);
  if (snapshots.size() != 1) {
    return;
  }
  QCOMPARE(snapshots.constFirst().eventType, std::optional<QString>(QStringLiteral("focusTime")));
  QVERIFY(snapshots.constFirst().conferenceJson.has_value());
  QCOMPARE(QJsonDocument::fromJson(snapshots.constFirst().attachmentsJson.toUtf8()).array().size(), 1);
  QCOMPARE(QJsonDocument::fromJson(snapshots.constFirst().guestPermissionsJson.toUtf8())
               .object()
               .value(QStringLiteral("guestsCanModify"))
               .toBool(),
           true);

  const QList<PendingMutationSnapshot> mutations = readPendingEventMutations(handle, eventId);
  QCOMPARE(mutations.size(), 1);
  if (mutations.size() != 1) {
    return;
  }
  const QJsonObject payload = mutations.constFirst().payload;
  QCOMPARE(payload.value(QStringLiteral("sendUpdates")).toString(), QStringLiteral("externalOnly"));
  const QJsonObject event = payload.value(QStringLiteral("event")).toObject();
  QCOMPARE(event.value(QStringLiteral("eventType")).toString(), QStringLiteral("focusTime"));
  QVERIFY(event.value(QStringLiteral("conferenceData")).toObject()
              .value(QStringLiteral("createRequest"))
              .isObject());
  QCOMPARE(event.value(QStringLiteral("attachments")).toArray().size(), 1);
  QVERIFY(event.value(QStringLiteral("guestsCanModify")).toBool());
  QCOMPARE(event.value(QStringLiteral("focusTimeProperties")).toObject()
               .value(QStringLiteral("chatStatus"))
               .toString(),
           QStringLiteral("available"));
}

void CalendarMutationServiceTest::preservesAdvancedGoogleRecurrenceLines() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  const QString recurrence = QStringLiteral(
      "RRULE:FREQ=HOURLY;INTERVAL=2;BYSECOND=0,30\n"
      "EXRULE:FREQ=DAILY;BYHOUR=3\n"
      "RDATE;VALUE=DATE:20261225\n"
      "EXDATE;TZID=Asia/Singapore:20260726T093000");
  std::future<hcb::CalendarEventMutationResult> create = service.create(
      {.calendarId = QStringLiteral("calendar-work"),
       .title = QStringLiteral("Advanced recurrence"),
       .startAt = QStringLiteral("2026-07-26T09:30:00+08:00"),
       .endAt = QStringLiteral("2026-07-26T10:30:00+08:00"),
       .recurrenceRule = recurrence});
  const hcb::CalendarEventMutationResult createResult = awaitResult(create);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(createResult));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(createResult)) {
    return;
  }
  const hcb::CalendarEventMutationReceipt receipt =
      std::get<hcb::CalendarEventMutationReceipt>(createResult);
  const QList<PendingMutationSnapshot> mutations =
      readPendingEventMutations(connection.nativeHandle(), receipt.eventId);
  QCOMPARE(mutations.size(), 1);
  if (mutations.size() != 1) {
    return;
  }
  const QJsonArray expected{
      QStringLiteral("RRULE:FREQ=HOURLY;INTERVAL=2;BYSECOND=0,30"),
      QStringLiteral("EXRULE:FREQ=DAILY;BYHOUR=3"),
      QStringLiteral("RDATE;VALUE=DATE:20261225"),
      QStringLiteral("EXDATE;TZID=Asia/Singapore:20260726T093000")};
  QCOMPARE(mutations.constFirst().payload.value(QStringLiteral("event")).toObject()
               .value(QStringLiteral("recurrence"))
               .toArray(),
           expected);

  std::future<hcb::CalendarEventMutationResult> rdateOnly = service.create(
      {.calendarId = QStringLiteral("calendar-work"),
       .title = QStringLiteral("RDATE-only recurrence"),
       .startAt = QStringLiteral("2026-12-25T09:30:00+08:00"),
       .endAt = QStringLiteral("2026-12-25T10:30:00+08:00"),
       .recurrenceRule = QStringLiteral("RDATE;TZID=Asia/Singapore:20261225T093000")});
  const hcb::CalendarEventMutationResult rdateOnlyResult = awaitResult(rdateOnly);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(rdateOnlyResult));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(rdateOnlyResult)) {
    return;
  }
  const hcb::CalendarEventMutationReceipt rdateOnlyReceipt =
      std::get<hcb::CalendarEventMutationReceipt>(rdateOnlyResult);
  const QList<PendingMutationSnapshot> rdateOnlyMutations =
      readPendingEventMutations(connection.nativeHandle(), rdateOnlyReceipt.eventId);
  QCOMPARE(rdateOnlyMutations.size(), 1);
  if (rdateOnlyMutations.size() != 1) {
    return;
  }
  QCOMPARE(rdateOnlyMutations.constFirst().payload.value(QStringLiteral("event")).toObject()
               .value(QStringLiteral("recurrence"))
               .toArray(),
           QJsonArray{QStringLiteral("RDATE;TZID=Asia/Singapore:20261225T093000")});
}

void CalendarMutationServiceTest::journalsRemoteUpdatesMovesAndCreateReconciliation() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, status, title, "
          "description, location, start_at, end_at, is_all_day, etag, created_at, updated_at) "
          "VALUES ('event-remote', 'calendar-work', 'remote-event', 'confirmed', 'Remote', "
          "'old description', 'Old room', '2026-07-26T01:00:00.000Z', "
          "'2026-07-26T02:00:00.000Z', 0, 'etag-old', '2026-07-25T00:00:00Z', "
          "'2026-07-25T00:00:00Z')");

  std::future<hcb::CalendarEventMutationResult> update = service.update(
      {.eventId = QStringLiteral("event-remote"), .title = QStringLiteral("Local title")});
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(awaitResult(update)));
  QList<PendingMutationSnapshot> mutations =
      readPendingEventMutations(handle, QStringLiteral("event-remote"));
  QCOMPARE(mutations.size(), 1);
  if (mutations.size() != 1) {
    return;
  }
  QCOMPARE(mutations.constFirst().operation, QStringLiteral("event.update"));
  QCOMPARE(mutations.constFirst().payload.value(QStringLiteral("calendarId")).toString(),
           QStringLiteral("work"));
  QCOMPARE(mutations.constFirst()
               .payload.value(QStringLiteral("event"))
               .toObject()
               .value(QStringLiteral("summary"))
               .toString(),
           QStringLiteral("Local title"));
  QVERIFY(!mutations.constFirst().payload.value(QStringLiteral("event"))
                .toObject()
                .contains(QStringLiteral("attendees")));
  QVERIFY(!mutations.constFirst().payload.value(QStringLiteral("event"))
                .toObject()
                .contains(QStringLiteral("reminders")));
  const QJsonObject metadata =
      mutations.constFirst().payload.value(QStringLiteral("_hcbSync")).toObject();
  QCOMPARE(metadata.value(QStringLiteral("etag")).toString(), QStringLiteral("etag-old"));
  QCOMPARE(metadata.value(QStringLiteral("base")).toObject().value(QStringLiteral("summary")).toString(),
           QStringLiteral("Remote"));

  std::future<hcb::CalendarEventMutationResult> metadataUpdate = service.update(
      {.eventId = QStringLiteral("event-remote"),
       .attendeeEmails = QList<QString>{QStringLiteral("guest@example.com")},
       .reminders = hcb::CalendarEventReminderSettings{
           .useDefault = false,
           .overrides = {{.method = QStringLiteral("popup"), .minutes = 10}}}});
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(awaitResult(metadataUpdate)));
  mutations = readPendingEventMutations(handle, QStringLiteral("event-remote"));
  QCOMPARE(mutations.size(), 1);
  if (mutations.size() != 1) {
    return;
  }
  const QJsonObject metadataPatch = mutations.constFirst().payload.value(QStringLiteral("event")).toObject();
  QCOMPARE(metadataPatch.value(QStringLiteral("attendees")).toArray().at(0)
               .toObject()
               .value(QStringLiteral("email"))
               .toString(),
           QStringLiteral("guest@example.com"));
  QCOMPARE(metadataPatch.value(QStringLiteral("reminders")).toObject()
               .value(QStringLiteral("overrides"))
               .toArray()
               .at(0)
               .toObject()
               .value(QStringLiteral("minutes"))
               .toInteger(),
           10);

  std::future<hcb::CalendarEventMutationResult> noOp = service.update(
      {.eventId = QStringLiteral("event-remote"), .title = QStringLiteral("Local title")});
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(awaitResult(noOp)));
  QCOMPARE(readPendingEventMutations(handle, QStringLiteral("event-remote")).size(), 1);

  std::future<hcb::CalendarEventMutationResult> move = service.update(
      {.eventId = QStringLiteral("event-remote"), .calendarId = QStringLiteral("calendar-other")});
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(awaitResult(move)));
  mutations = readPendingEventMutations(handle, QStringLiteral("event-remote"));
  QCOMPARE(mutations.size(), 2);
  if (mutations.size() != 2) {
    return;
  }
  const PendingMutationSnapshot* moveMutation = nullptr;
  const PendingMutationSnapshot* followUpMutation = nullptr;
  for (const PendingMutationSnapshot& mutation : mutations) {
    if (mutation.operation == QStringLiteral("event.move")) {
      moveMutation = &mutation;
    } else if (mutation.operation == QStringLiteral("event.update")) {
      followUpMutation = &mutation;
    }
  }
  QVERIFY(moveMutation != nullptr);
  QVERIFY(followUpMutation != nullptr);
  if (moveMutation == nullptr || followUpMutation == nullptr) {
    return;
  }
  QCOMPARE(moveMutation->payload.value(QStringLiteral("sourceCalendarId")).toString(),
           QStringLiteral("work"));
  QCOMPARE(moveMutation->payload.value(QStringLiteral("destinationCalendarId")).toString(),
           QStringLiteral("other"));
  QCOMPARE(followUpMutation->payload.value(QStringLiteral("dependsOnMutationId")).toString(),
           moveMutation->id);

  std::future<hcb::CalendarEventMutationResult> created = service.create(
      {.calendarId = QStringLiteral("calendar-work"),
       .title = QStringLiteral("New event"),
       .startAt = QStringLiteral("2026-08-01T09:00:00Z"),
       .endAt = QStringLiteral("2026-08-01T10:00:00Z")});
  const hcb::CalendarEventMutationResult createdResult = awaitResult(created);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(createdResult));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(createdResult)) {
    return;
  }
  const QString createdId = std::get<hcb::CalendarEventMutationReceipt>(createdResult).eventId;
  std::future<hcb::CalendarEventMutationResult> reconciled = service.reconcileGoogleEvent(
      {.localEventId = createdId,
       .remoteEventId = QStringLiteral("remote-created"),
       .remoteEtag = QStringLiteral("etag-created")});
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(awaitResult(reconciled)));
  const std::optional<EventSnapshot> createdSnapshot = readEvent(handle, createdId);
  QVERIFY(createdSnapshot.has_value());
  if (!createdSnapshot.has_value()) {
    return;
  }
  QCOMPARE(createdSnapshot->remoteId, QStringLiteral("remote-created"));
  std::future<hcb::CalendarEventMutationResult> afterCreateUpdate = service.update(
      {.eventId = createdId, .location = std::optional<std::optional<QString>>(QStringLiteral("HQ"))});
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(awaitResult(afterCreateUpdate)));
  const QList<PendingMutationSnapshot> createdMutations =
      readPendingEventMutations(handle, createdId);
  QCOMPARE(createdMutations.size(), 1);
  if (createdMutations.size() != 1) {
    return;
  }
  QCOMPARE(createdMutations.constFirst().operation, QStringLiteral("event.create"));
  QCOMPARE(createdMutations.constFirst().payload.value(QStringLiteral("_hcbSync"))
               .toObject()
               .value(QStringLiteral("etag"))
               .toString(),
           QStringLiteral("etag-created"));
}

void CalendarMutationServiceTest::rejectsInvalidAndUnavailableMutations() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{});
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);

  std::future<hcb::CalendarEventMutationResult> unavailableCreate = service.create(
      hcb::CalendarEventCreateInput{.calendarId = QStringLiteral("calendar-deleted"),
                                    .title = QStringLiteral("Unavailable"),
                                    .startAt = QStringLiteral("2026-07-26T09:00:00Z"),
                                    .endAt = QStringLiteral("2026-07-26T10:00:00Z")});
  const hcb::CalendarEventMutationResult unavailableCreateResult = awaitResult(unavailableCreate);
  QVERIFY(std::holds_alternative<hcb::AppError>(unavailableCreateResult));
  QCOMPARE(std::get<hcb::AppError>(unavailableCreateResult).code(), hcb::AppErrorCode::Validation);

  execute(connection.nativeHandle(),
          "UPDATE local_calendars SET access_role = 'reader' WHERE id = 'calendar-other'");
  std::future<hcb::CalendarEventMutationResult> readOnlyCreate = service.create(
      hcb::CalendarEventCreateInput{.calendarId = QStringLiteral("calendar-other"),
                                    .title = QStringLiteral("Read-only"),
                                    .startAt = QStringLiteral("2026-07-26T09:00:00Z"),
                                    .endAt = QStringLiteral("2026-07-26T10:00:00Z")});
  const hcb::CalendarEventMutationResult readOnlyCreateResult = awaitResult(readOnlyCreate);
  QVERIFY(std::holds_alternative<hcb::AppError>(readOnlyCreateResult));
  QCOMPARE(std::get<hcb::AppError>(readOnlyCreateResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarEventMutationResult> invalidCreate = service.create(
      hcb::CalendarEventCreateInput{.calendarId = QStringLiteral("calendar-work"),
                                    .title = QStringLiteral("Invalid"),
                                    .startAt = QStringLiteral("2026-07-26T10:00:00Z"),
                                    .endAt = QStringLiteral("2026-07-26T09:00:00Z")});
  const hcb::CalendarEventMutationResult invalidCreateResult = awaitResult(invalidCreate);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidCreateResult));
  QCOMPARE(std::get<hcb::AppError>(invalidCreateResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarEventMutationResult> invalidAttendee = service.create(
      hcb::CalendarEventCreateInput{.calendarId = QStringLiteral("calendar-work"),
                                    .title = QStringLiteral("Invalid attendee"),
                                    .startAt = QStringLiteral("2026-07-26T09:00:00Z"),
                                    .endAt = QStringLiteral("2026-07-26T10:00:00Z"),
                                    .attendeeEmails = {QStringLiteral("not-an-email")}});
  const hcb::CalendarEventMutationResult invalidAttendeeResult = awaitResult(invalidAttendee);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidAttendeeResult));
  QCOMPARE(std::get<hcb::AppError>(invalidAttendeeResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarEventMutationResult> invalidReminder = service.create(
      hcb::CalendarEventCreateInput{
          .calendarId = QStringLiteral("calendar-work"),
          .title = QStringLiteral("Invalid reminder"),
          .startAt = QStringLiteral("2026-07-26T09:00:00Z"),
          .endAt = QStringLiteral("2026-07-26T10:00:00Z"),
          .reminders = {.useDefault = false,
                        .overrides = {{.method = QStringLiteral("sms"), .minutes = 10}}}});
  const hcb::CalendarEventMutationResult invalidReminderResult = awaitResult(invalidReminder);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidReminderResult));
  QCOMPARE(std::get<hcb::AppError>(invalidReminderResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarEventMutationResult> create = service.create(
      hcb::CalendarEventCreateInput{.calendarId = QStringLiteral("calendar-work"),
                                    .title = QStringLiteral("Existing"),
                                    .startAt = QStringLiteral("2026-07-26T09:00:00Z"),
                                    .endAt = QStringLiteral("2026-07-26T10:00:00Z")});
  const hcb::CalendarEventMutationResult createResult = awaitResult(create);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(createResult));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(createResult)) {
    return;
  }
  const QString eventId = std::get<hcb::CalendarEventMutationReceipt>(createResult).eventId;

  std::future<hcb::CalendarEventMutationResult> invalidRange =
      service.update(hcb::CalendarEventUpdateInput{
          .eventId = eventId, .endAt = QStringLiteral("2026-07-26T08:00:00Z")});
  const hcb::CalendarEventMutationResult invalidRangeResult = awaitResult(invalidRange);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidRangeResult));
  QCOMPARE(std::get<hcb::AppError>(invalidRangeResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarEventMutationResult> crossAccountMove =
      service.update(hcb::CalendarEventUpdateInput{
          .eventId = eventId, .calendarId = QStringLiteral("calendar-cross-account")});
  const hcb::CalendarEventMutationResult crossAccountMoveResult = awaitResult(crossAccountMove);
  QVERIFY(std::holds_alternative<hcb::AppError>(crossAccountMoveResult));
  QCOMPARE(std::get<hcb::AppError>(crossAccountMoveResult).code(), hcb::AppErrorCode::Validation);
}

void CalendarMutationServiceTest::scopesRecurringSeriesWithoutUnsafeLegacyMutations() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  execute(connection.nativeHandle(),
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, status, title, start_at, "
          "end_at, is_all_day, recurrence_rule, etag, updated_at, deleted_at) VALUES "
          "('event-series', 'calendar-work', 'remote-series', 'confirmed', 'Series', "
          "'2026-07-25T09:00:00.000Z', '2026-07-25T10:00:00.000Z', 0, "
          "'RRULE:FREQ=DAILY;COUNT=5', 'etag-series', '2026-07-25T00:00:00Z', NULL)");

  std::future<hcb::CalendarEventMutationResult> legacy = service.update(
      {.eventId = QStringLiteral("event-series"), .title = QStringLiteral("Unsafe")});
  const hcb::CalendarEventMutationResult legacyResult = awaitResult(legacy);
  QVERIFY(std::holds_alternative<hcb::AppError>(legacyResult));

  std::future<hcb::CalendarEventMutationResult> exception = service.updateScoped(
      {.update = {.eventId = QStringLiteral("event-series:instance:20260726T090000Z"),
                  .title = QStringLiteral("Changed instance")},
       .scope = hcb::CalendarEventRecurrenceScope::ThisInstance});
  const hcb::CalendarEventMutationResult exceptionResult = awaitResult(exception);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(exceptionResult));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(exceptionResult)) {
    return;
  }
  const QString exceptionId = std::get<hcb::CalendarEventMutationReceipt>(exceptionResult).eventId;
  std::future<hcb::CalendarEventMutationSnapshotResult> exceptionInspectFuture =
      service.inspect({exceptionId});
  const hcb::CalendarEventMutationSnapshotResult exceptionInspect = awaitResult(exceptionInspectFuture);
  QVERIFY(std::holds_alternative<QList<hcb::CalendarEventMutationSnapshot>>(exceptionInspect));
  if (!std::holds_alternative<QList<hcb::CalendarEventMutationSnapshot>>(exceptionInspect)) {
    return;
  }
  const QList<hcb::CalendarEventMutationSnapshot>& exceptionSnapshots =
      std::get<QList<hcb::CalendarEventMutationSnapshot>>(exceptionInspect);
  QCOMPARE(exceptionSnapshots.size(), 1);
  QCOMPARE(exceptionSnapshots.constFirst().recurringRemoteId,
           std::optional<QString>(QStringLiteral("remote-series")));
  QCOMPARE(exceptionSnapshots.constFirst().originalStartAt,
           std::optional<QString>(QStringLiteral("2026-07-26T09:00:00.000Z")));
  const QList<PendingMutationSnapshot> exceptionMutations =
      readPendingEventMutations(connection.nativeHandle(), exceptionId);
  QCOMPARE(exceptionMutations.size(), 1);
  if (exceptionMutations.size() != 1) {
    return;
  }
  QCOMPARE(exceptionMutations.constFirst().operation, QStringLiteral("event.instance.update"));
  QCOMPARE(exceptionMutations.constFirst().payload.value(QStringLiteral("recurringRemoteId")).toString(),
           QStringLiteral("remote-series"));
  QCOMPARE(exceptionMutations.constFirst().payload.value(QStringLiteral("originalStartAt")).toString(),
           QStringLiteral("2026-07-26T09:00:00.000Z"));

  std::future<hcb::CalendarEventMutationResult> cancelException = service.removeScoped(
      {.eventId = exceptionId, .scope = hcb::CalendarEventRecurrenceScope::ThisInstance});
  const hcb::CalendarEventMutationResult cancelExceptionResult = awaitResult(cancelException);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(cancelExceptionResult));
  const QList<PendingMutationSnapshot> cancelledExceptionMutations =
      readPendingEventMutations(connection.nativeHandle(), exceptionId);
  QCOMPARE(cancelledExceptionMutations.size(), 1);
  if (cancelledExceptionMutations.size() != 1) {
    return;
  }
  QCOMPARE(cancelledExceptionMutations.constFirst().operation, QStringLiteral("event.instance.delete"));

  std::future<hcb::CalendarEventMutationResult> split = service.updateScoped(
      {.update = {.eventId = QStringLiteral("event-series:instance:20260727T090000Z"),
                  .title = QStringLiteral("Tail")},
       .scope = hcb::CalendarEventRecurrenceScope::ThisAndFollowing});
  const hcb::CalendarEventMutationResult splitResult = awaitResult(split);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(splitResult));
  if (!std::holds_alternative<hcb::CalendarEventMutationReceipt>(splitResult)) {
    return;
  }
  std::future<hcb::CalendarEventMutationSnapshotResult> inspectFuture =
      service.inspect({QStringLiteral("event-series")});
  const hcb::CalendarEventMutationSnapshotResult inspectResult = awaitResult(inspectFuture);
  QVERIFY(std::holds_alternative<QList<hcb::CalendarEventMutationSnapshot>>(inspectResult));
  if (!std::holds_alternative<QList<hcb::CalendarEventMutationSnapshot>>(inspectResult)) {
    return;
  }
  const QList<hcb::CalendarEventMutationSnapshot>& snapshots =
      std::get<QList<hcb::CalendarEventMutationSnapshot>>(inspectResult);
  QCOMPARE(snapshots.size(), 1);
  QCOMPARE(snapshots.constFirst().recurrenceRule,
           std::optional<QString>(QStringLiteral("RRULE:FREQ=DAILY;UNTIL=20260727T085959Z")));
  const QList<PendingMutationSnapshot> masterMutations =
      readPendingEventMutations(connection.nativeHandle(), QStringLiteral("event-series"));
  QCOMPARE(masterMutations.size(), 1);
  if (masterMutations.size() != 1) {
    return;
  }
  QCOMPARE(masterMutations.constFirst().operation, QStringLiteral("event.update"));
  QCOMPARE(masterMutations.constFirst().payload.value(QStringLiteral("event"))
               .toObject()
               .value(QStringLiteral("recurrence"))
               .toArray()
               .at(0)
               .toString(),
           QStringLiteral("RRULE:FREQ=DAILY;UNTIL=20260727T085959Z"));

  std::future<hcb::CalendarEventMutationResult> virtualInstance = service.removeScoped(
      {.eventId = QStringLiteral("event-series:instance:20260728T090000Z"),
       .scope = hcb::CalendarEventRecurrenceScope::ThisInstance});
  const hcb::CalendarEventMutationResult virtualInstanceResult = awaitResult(virtualInstance);
  QVERIFY(std::holds_alternative<hcb::AppError>(virtualInstanceResult));
}

void CalendarMutationServiceTest::bulkClassifiesAndQueuesEligibleEvents() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "UPDATE local_calendars SET access_role = 'owner' WHERE id = 'calendar-work'; "
          "UPDATE local_calendars SET access_role = 'reader' WHERE id = 'calendar-other'; "
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, status, title, start_at, "
          "end_at, is_all_day, recurrence_rule, etag, updated_at, event_type) VALUES "
          "('event-editable', 'calendar-work', 'remote-editable', 'confirmed', 'Editable', "
          "'2026-07-26T01:00:00.000Z', '2026-07-26T02:00:00.000Z', 0, NULL, "
          "'etag-editable', '2026-07-25T00:00:00Z', NULL), "
          "('event-recurring', 'calendar-work', 'remote-recurring', 'confirmed', 'Recurring', "
          "'2026-07-27T01:00:00.000Z', '2026-07-27T02:00:00.000Z', 0, 'RRULE:FREQ=DAILY', "
          "'etag-recurring', '2026-07-25T00:00:00Z', NULL), "
          "('event-read-only', 'calendar-other', 'remote-read-only', 'confirmed', 'Read only', "
          "'2026-07-28T01:00:00.000Z', '2026-07-28T02:00:00.000Z', 0, NULL, "
          "'etag-read-only', '2026-07-25T00:00:00Z', NULL), "
          "('event-immutable', 'calendar-work', 'remote-immutable', 'confirmed', 'Immutable', "
          "'2026-07-29T01:00:00.000Z', '2026-07-29T02:00:00.000Z', 0, NULL, "
          "'etag-immutable', '2026-07-25T00:00:00Z', 'focusTime')");

  hcb::CalendarEventBulkMutationService bulk(service);
  std::future<hcb::CalendarEventBulkMutationResult> availability = bulk.execute(
      {.action = hcb::CalendarEventBulkAction::SetAvailability,
       .eventIds = {QStringLiteral("event-editable"),
                    QStringLiteral("event-recurring"),
                    QStringLiteral("event-read-only"),
                    QStringLiteral("event-immutable")},
       .available = true});
  const hcb::CalendarEventBulkMutationResult availabilityResult = awaitResult(availability);
  QVERIFY(std::holds_alternative<hcb::CalendarEventBulkMutationSummary>(availabilityResult));
  if (!std::holds_alternative<hcb::CalendarEventBulkMutationSummary>(availabilityResult)) {
    return;
  }
  const hcb::CalendarEventBulkMutationSummary availabilitySummary =
      std::get<hcb::CalendarEventBulkMutationSummary>(availabilityResult);
  QCOMPARE(availabilitySummary.requested, 4);
  QCOMPARE(availabilitySummary.eligible, 1);
  QCOMPARE(availabilitySummary.queued, 1);
  QCOMPARE(availabilitySummary.skipped, 3);
  const std::optional<EventSnapshot> editable = readEvent(handle, QStringLiteral("event-editable"));
  QVERIFY(editable.has_value());
  if (!editable.has_value()) {
    return;
  }
  QCOMPARE(editable->transparency, std::optional<QString>(QStringLiteral("transparent")));

  std::future<hcb::CalendarEventMutationResult> immutableUpdate = service.update(
      {.eventId = QStringLiteral("event-immutable"), .title = QStringLiteral("Changed")});
  const hcb::CalendarEventMutationResult immutableUpdateResult = awaitResult(immutableUpdate);
  QVERIFY(std::holds_alternative<hcb::AppError>(immutableUpdateResult));
  QCOMPARE(std::get<hcb::AppError>(immutableUpdateResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarEventBulkMutationResult> shift = bulk.execute(
      {.action = hcb::CalendarEventBulkAction::ShiftTime,
       .eventIds = {QStringLiteral("event-editable")},
       .shiftMinutes = 60});
  const hcb::CalendarEventBulkMutationResult shiftResult = awaitResult(shift);
  QVERIFY(std::holds_alternative<hcb::CalendarEventBulkMutationSummary>(shiftResult));
  if (!std::holds_alternative<hcb::CalendarEventBulkMutationSummary>(shiftResult)) {
    return;
  }
  QCOMPARE(std::get<hcb::CalendarEventBulkMutationSummary>(shiftResult).queued, 1);
  const std::optional<EventSnapshot> shifted = readEvent(handle, QStringLiteral("event-editable"));
  QVERIFY(shifted.has_value());
  if (!shifted.has_value()) {
    return;
  }
  QCOMPARE(shifted->startAt, QStringLiteral("2026-07-26T02:00:00.000Z"));
  QCOMPARE(shifted->endAt, QStringLiteral("2026-07-26T03:00:00.000Z"));
}

void CalendarMutationServiceTest::bulkReplacesLiteralEventTextWithPreviewAndScope() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::CalendarMutationService service(*databasePath, clock);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "UPDATE local_calendars SET access_role = 'owner' WHERE id = 'calendar-work'; "
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, status, title, description, "
          "location, start_at, end_at, is_all_day, recurrence_rule, etag, updated_at) VALUES "
          "('event-replace', 'calendar-work', 'remote-replace', 'confirmed', 'Alpha Alpha', "
          "'Alpha description', 'Alpha room', '2026-07-26T01:00:00.000Z', "
          "'2026-07-26T02:00:00.000Z', 0, NULL, 'etag-replace', '2026-07-25T00:00:00Z'), "
          "('event-replace-recurring', 'calendar-work', 'remote-replace-recurring', 'confirmed', "
          "'Alpha recurring', NULL, NULL, '2026-07-27T01:00:00.000Z', "
          "'2026-07-27T02:00:00.000Z', 0, 'RRULE:FREQ=DAILY', 'etag-recurring', "
          "'2026-07-25T00:00:00Z')");

  hcb::CalendarEventBulkMutationService bulk(service);
  std::future<hcb::CalendarEventBulkMutationResult> preview = bulk.execute(
      {.action = hcb::CalendarEventBulkAction::ReplaceText,
       .eventIds = {QStringLiteral("event-replace"), QStringLiteral("event-replace-recurring")},
       .findText = QStringLiteral("Alpha"),
       .replaceText = QStringLiteral("Beta"),
       .textFields = static_cast<std::uint8_t>(hcb::CalendarEventBulkTextField::Title) |
                     static_cast<std::uint8_t>(hcb::CalendarEventBulkTextField::Description) |
                     static_cast<std::uint8_t>(hcb::CalendarEventBulkTextField::Location),
       .recurrenceScope = 0,
       .previewOnly = true});
  const hcb::CalendarEventBulkMutationResult previewResult = awaitResult(preview);
  QVERIFY(std::holds_alternative<hcb::CalendarEventBulkMutationSummary>(previewResult));
  if (!std::holds_alternative<hcb::CalendarEventBulkMutationSummary>(previewResult)) {
    return;
  }
  QCOMPARE(std::get<hcb::CalendarEventBulkMutationSummary>(previewResult).eligible, 1);
  QCOMPARE(std::get<hcb::CalendarEventBulkMutationSummary>(previewResult).queued, 0);
  QCOMPARE(std::get<hcb::CalendarEventBulkMutationSummary>(previewResult).skipped, 1);

  std::future<hcb::CalendarEventBulkMutationResult> replace = bulk.execute(
      {.action = hcb::CalendarEventBulkAction::ReplaceText,
       .eventIds = {QStringLiteral("event-replace")},
       .findText = QStringLiteral("Alpha"),
       .replaceText = QStringLiteral("Beta"),
       .textFields = static_cast<std::uint8_t>(hcb::CalendarEventBulkTextField::Title) |
                     static_cast<std::uint8_t>(hcb::CalendarEventBulkTextField::Description) |
                     static_cast<std::uint8_t>(hcb::CalendarEventBulkTextField::Location),
       .recurrenceScope = 0});
  const hcb::CalendarEventBulkMutationResult replaceResult = awaitResult(replace);
  QVERIFY(std::holds_alternative<hcb::CalendarEventBulkMutationSummary>(replaceResult));
  if (!std::holds_alternative<hcb::CalendarEventBulkMutationSummary>(replaceResult)) {
    return;
  }
  QCOMPARE(std::get<hcb::CalendarEventBulkMutationSummary>(replaceResult).queued, 1);
  const std::optional<EventSnapshot> event = readEvent(handle, QStringLiteral("event-replace"));
  QVERIFY(event.has_value());
  if (!event.has_value()) {
    return;
  }
  QCOMPARE(event->title, QStringLiteral("Beta Beta"));
  QCOMPARE(event->description, std::optional<QString>(QStringLiteral("Beta description")));
  QCOMPARE(event->location, std::optional<QString>(QStringLiteral("Beta room")));
}

QTEST_GUILESS_MAIN(CalendarMutationServiceTest)

#include "CalendarMutationServiceTest.moc"
