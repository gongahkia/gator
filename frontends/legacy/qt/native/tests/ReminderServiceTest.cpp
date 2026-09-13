#include <QDateTime>
#include <QTime>
#include <QTimeZone>
#include <QtTest/QTest>

#include <chrono>
#include <memory>
#include <optional>
#include <utility>
#include <variant>

#include "app/NativeReminderNotifier.h"
#include "core/ReminderService.h"
#include "data/LocalSchema.h"
#include "sqlite3.h"
#include "support/TemporarySqliteDatabase.h"

namespace {

class FixedClock final : public hcb::Clock {
public:
  explicit FixedClock(const QDateTime& dateTime)
      : wallTime_(std::chrono::milliseconds(dateTime.toMSecsSinceEpoch())) {}

  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

private:
  hcb::WallTimePoint wallTime_;
};

[[nodiscard]] std::unique_ptr<hcb::test::TemporarySqliteDatabase> createDatabase() {
  hcb::test::TemporarySqliteDatabaseResult result = hcb::test::TemporarySqliteDatabase::create();
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result)) {
    return nullptr;
  }
  return std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result));
}

[[nodiscard]] std::optional<hcb::SqliteConnection>
openConnection(const hcb::test::TemporarySqliteDatabase& database) {
  hcb::SqliteConnectionResult result = database.open(hcb::SqliteOpenMode::ReadWriteCreate);
  if (!std::holds_alternative<hcb::SqliteConnection>(result)) {
    return std::nullopt;
  }
  return std::move(std::get<hcb::SqliteConnection>(result));
}

void execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  const QString message = errorMessage == nullptr ? QString() : QString::fromUtf8(errorMessage);
  sqlite3_free(errorMessage);
  QVERIFY2(result == SQLITE_OK, qPrintable(message));
}

[[nodiscard]] std::optional<QString> scalarText(sqlite3* handle, const char* sql) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    return std::nullopt;
  }
  const int stepped = sqlite3_step(statement);
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, 0));
  const int size = sqlite3_column_bytes(statement, 0);
  const std::optional<QString> result =
      stepped == SQLITE_ROW && value != nullptr && size >= 0
          ? std::optional<QString>(QString::fromUtf8(value, size))
          : std::nullopt;
  return sqlite3_finalize(statement) == SQLITE_OK ? result : std::nullopt;
}

void seed(sqlite3* handle) {
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-1', 'google', 'connected', '[]', '[]', '2026-07-30T12:00:00.000Z')");
  execute(handle,
          "INSERT INTO local_calendars (id, account_id, remote_id, title, time_zone, "
          "default_reminders_json, updated_at) VALUES "
          "('calendar-1', 'account-1', 'calendar-1', 'Work', 'Asia/Singapore', '[]', "
          "'2026-07-30T12:00:00.000Z')");
  execute(handle,
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, title, start_at, "
          "end_at, is_all_day, reminders_json, reminders_use_default, updated_at) VALUES "
          "('event-1', 'calendar-1', 'event-1', 'All day', '2026-08-01T00:00:00.000Z', "
          "'2026-08-02T00:00:00.000Z', 1, '[{\"method\":\"popup\",\"minutes\":30}]', "
          "0, '2026-07-30T12:00:00.000Z')");
}

} // namespace

class ReminderServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void schedulesAllDayReminderInCalendarTimeZone();
  void persistsSnoozeAndDismissal();
};

void ReminderServiceTest::schedulesAllDayReminderInCalendarTimeZone() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*database);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(
      hcb::LocalSchema::initialize(*connection)));
  seed(connection->nativeHandle());

  FixedClock clock(QDateTime(QDate(2026, 7, 30), QTime(12, 0), QTimeZone::UTC));
  hcb::NativeReminderNotifier notifier;
  hcb::ReminderService service(database->databasePath(), clock, notifier);
  service.refresh();

  const std::optional<QString> triggerAt = scalarText(
      connection->nativeHandle(), "SELECT trigger_at FROM local_reminder_state WHERE event_id = 'event-1'");
  QVERIFY(triggerAt.has_value());
  if (!triggerAt.has_value()) {
    return;
  }
  QCOMPARE(*triggerAt, QStringLiteral("2026-07-31T15:30:00.000Z"));
}

void ReminderServiceTest::persistsSnoozeAndDismissal() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*database);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(
      hcb::LocalSchema::initialize(*connection)));
  seed(connection->nativeHandle());

  FixedClock clock(QDateTime(QDate(2026, 7, 30), QTime(12, 0), QTimeZone::UTC));
  hcb::NativeReminderNotifier notifier;
  hcb::ReminderService service(database->databasePath(), clock, notifier);
  service.refresh();
  const std::optional<QString> identifier = scalarText(
      connection->nativeHandle(), "SELECT identifier FROM local_reminder_state WHERE event_id = 'event-1'");
  QVERIFY(identifier.has_value());
  if (!identifier.has_value()) {
    return;
  }

  service.snooze(*identifier, 10);
  const std::optional<QString> snoozedUntil = scalarText(
      connection->nativeHandle(), "SELECT snoozed_until FROM local_reminder_state WHERE event_id = 'event-1'");
  QVERIFY(snoozedUntil.has_value());
  if (!snoozedUntil.has_value()) {
    return;
  }
  QCOMPARE(*snoozedUntil, QStringLiteral("2026-07-30T12:10:00.000Z"));

  service.dismiss(*identifier);
  const std::optional<QString> dismissedAt = scalarText(
      connection->nativeHandle(), "SELECT dismissed_at FROM local_reminder_state WHERE event_id = 'event-1'");
  QVERIFY(dismissedAt.has_value());
  if (!dismissedAt.has_value()) {
    return;
  }
  QCOMPARE(*dismissedAt, QStringLiteral("2026-07-30T12:00:00.000Z"));
}

QTEST_GUILESS_MAIN(ReminderServiceTest)

#include "ReminderServiceTest.moc"
