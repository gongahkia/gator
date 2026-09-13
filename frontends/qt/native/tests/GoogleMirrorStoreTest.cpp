#include <QDir>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonObject>
#include <QStringList>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/GoogleMirrorStore.h"
#include "core/TaskRecurrenceMarker.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

using namespace std::chrono_literals;

class GoogleMirrorStoreTest final : public QObject {
  Q_OBJECT

private slots:
  void atomicallyReplacesTaskAndCalendarSnapshots();
  void preservesQueuedApplyingAndRetryableRowsDuringPull();
  void appliesDeltasWithoutDeletingUnreturnedRows();
  void recordsManagedRecurrencePullDiagnostics();
  void cachesResolvedInstancesAndInvalidatesChangedSeries();
  void storesFullGoogleRecurrenceOutsideLegacyColumn();
};

namespace {

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("google mirror request timed out");
  }
  return future.get();
}

void verifyReady(hcb::GoogleMirrorStore& store) {
  const std::shared_future<hcb::SqliteWriteResult> ready = store.ready();
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

[[nodiscard]] int count(sqlite3* handle, const char* sql) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    qFatal("SQLite count preparation failed");
  }
  if (sqlite3_step(statement) != SQLITE_ROW) {
    sqlite3_finalize(statement);
    qFatal("SQLite count query failed");
  }
  const int value = sqlite3_column_int(statement, 0);
  if (sqlite3_finalize(statement) != SQLITE_OK) {
    qFatal("SQLite count finalization failed");
  }
  return value;
}

[[nodiscard]] QString text(sqlite3* handle, const char* sql) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    qFatal("SQLite text preparation failed");
  }
  if (sqlite3_step(statement) != SQLITE_ROW) {
    sqlite3_finalize(statement);
    qFatal("SQLite text query failed");
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, 0));
  const int size = sqlite3_column_bytes(statement, 0);
  const QString result = value == nullptr ? QString() : QString::fromUtf8(value, size);
  if (sqlite3_finalize(statement) != SQLITE_OK) {
    qFatal("SQLite text finalization failed");
  }
  return result;
}

} // namespace

void GoogleMirrorStoreTest::atomicallyReplacesTaskAndCalendarSnapshots() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::SystemClock clock;
  hcb::GoogleMirrorStore store(*databasePath, clock);
  verifyReady(store);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
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
          "('google', 'google', 'connected', '[]', '[]', '2026-07-26T00:00:00Z')");

  std::future<hcb::GoogleMirrorWriteResult> taskWrite =
      store.replaceTasks(QStringLiteral("google"),
                         {{.id = QStringLiteral("inbox"),
                           .title = QStringLiteral("Inbox"),
                           .updatedAt = QStringLiteral("2026-07-26T00:00:00Z")}},
                         {{.id = QStringLiteral("parent"),
                           .taskListId = QStringLiteral("inbox"),
                           .title = QStringLiteral("Parent"),
                           .status = hcb::GoogleTaskStatus::NeedsAction,
                           .isAssigned = true},
                          {.id = QStringLiteral("child"),
                           .taskListId = QStringLiteral("inbox"),
                           .parentId = QStringLiteral("parent"),
                           .title = QStringLiteral("Child"),
                           .status = hcb::GoogleTaskStatus::Completed}});
  const hcb::GoogleMirrorWriteResult taskResult = awaitResult(taskWrite);
  QVERIFY(std::holds_alternative<std::monostate>(taskResult));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_task_lists WHERE deleted_at IS NULL"), 1);
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_tasks WHERE deleted_at IS NULL"), 2);
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_tasks WHERE is_assigned = 1"), 1);
  QCOMPARE(count(handle,
                 "SELECT COUNT(*) FROM local_tasks WHERE parent_task_id IS NOT NULL "
                 "AND state = 'completed'"),
           1);

  std::future<hcb::GoogleMirrorWriteResult> calendarWrite =
      store.replaceCalendars(QStringLiteral("google"),
                             {{.id = QStringLiteral("primary"),
                               .title = QStringLiteral("Primary"),
                               .timeZone = QStringLiteral("UTC"),
                               .colorId = QStringLiteral("7"),
                               .accessRole = hcb::GoogleCalendarAccessRole::Owner,
                               .selected = true,
                               .hidden = false,
                               .primary = true,
                               .defaultReminders = QJsonArray{
                                   QJsonObject{{QStringLiteral("method"), QStringLiteral("popup")},
                                               {QStringLiteral("minutes"), 30}}}}},
                             {{.id = QStringLiteral("event"),
                               .calendarId = QStringLiteral("primary"),
                               .status = hcb::GoogleCalendarEventStatus::Confirmed,
                               .title = QStringLiteral("Planning"),
                               .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
                               .endAt = QStringLiteral("2026-07-26T10:00:00.000Z"),
                               .allDay = false,
                               .visibility = QStringLiteral("confidential"),
                               .attendees = QJsonArray{QJsonObject{{QStringLiteral("email"),
                                                                    QStringLiteral("guest@example.com")},
                                                        {QStringLiteral("displayName"),
                                                         QStringLiteral("Guest")}}},
                               .reminders = QJsonObject{
                                   {QStringLiteral("useDefault"), false},
                                   {QStringLiteral("overrides"),
                                    QJsonArray{QJsonObject{{QStringLiteral("method"),
                                                             QStringLiteral("popup")},
                                                          {QStringLiteral("minutes"), 10}}}}},
                               .conferenceData = QJsonObject{
                                   {QStringLiteral("entryPoints"),
                                    QJsonArray{QJsonObject{{QStringLiteral("entryPointType"),
                                                           QStringLiteral("video")},
                                                          {QStringLiteral("uri"),
                                                           QStringLiteral("https://meet.google.com/abc")}}}}},
                               .attachments = QJsonArray{QJsonObject{
                                   {QStringLiteral("fileUrl"),
                                    QStringLiteral("https://drive.google.com/open?id=file-1")},
                                   {QStringLiteral("title"), QStringLiteral("Spec")}}},
                               .guestPermissions = QJsonObject{
                                   {QStringLiteral("guestsCanModify"), true}},
                               .statusProperties = QJsonObject{
                                   {QStringLiteral("focusTimeProperties"),
                                    QJsonObject{{QStringLiteral("chatStatus"),
                                                 QStringLiteral("available")}}}}}});
  const hcb::GoogleMirrorWriteResult calendarResult = awaitResult(calendarWrite);
  QVERIFY(std::holds_alternative<std::monostate>(calendarResult));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendars WHERE deleted_at IS NULL"), 1);
  QCOMPARE(text(handle, "SELECT color_id FROM local_calendars"), QStringLiteral("7"));
  QCOMPARE(count(handle,
                 "SELECT COUNT(*) FROM local_calendars "
                 "WHERE is_selected = 1 AND is_hidden = 0"),
           1);
  QCOMPARE(text(handle, "SELECT default_reminders_json FROM local_calendars"),
           QStringLiteral("[{\"method\":\"popup\",\"minutes\":30}]"));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendar_events WHERE deleted_at IS NULL"), 1);
  QCOMPARE(text(handle, "SELECT visibility FROM local_calendar_events"),
           QStringLiteral("confidential"));
  QCOMPARE(text(handle, "SELECT attendee_emails_json FROM local_calendar_events"),
           QStringLiteral("[\"guest@example.com\"]"));
  QCOMPARE(text(handle, "SELECT attendee_details_json FROM local_calendar_events"),
           QStringLiteral("[{\"displayName\":\"Guest\",\"email\":\"guest@example.com\"}]"));
  QCOMPARE(text(handle, "SELECT reminders_json FROM local_calendar_events"),
           QStringLiteral("[{\"method\":\"popup\",\"minutes\":10}]"));
  QCOMPARE(text(handle, "SELECT conference_json FROM local_calendar_events"),
           QStringLiteral("{\"entryPoints\":[{\"entryPointType\":\"video\",\"uri\":\"https://meet.google.com/abc\"}]}"));
  QCOMPARE(text(handle, "SELECT attachments_json FROM local_calendar_events"),
           QStringLiteral("[{\"fileUrl\":\"https://drive.google.com/open?id=file-1\",\"title\":\"Spec\"}]"));
  QCOMPARE(text(handle, "SELECT guest_permissions_json FROM local_calendar_events"),
           QStringLiteral("{\"guestsCanModify\":true}"));
  QCOMPARE(text(handle, "SELECT status_properties_json FROM local_calendar_events"),
           QStringLiteral("{\"focusTimeProperties\":{\"chatStatus\":\"available\"}}"));
  QCOMPARE(count(handle, "SELECT reminders_use_default FROM local_calendar_events"), 0);

  std::future<hcb::GoogleMirrorWriteResult> emptyTasks =
      store.replaceTasks(QStringLiteral("google"), {}, {});
  const hcb::GoogleMirrorWriteResult emptyTaskResult = awaitResult(emptyTasks);
  QVERIFY(std::holds_alternative<std::monostate>(emptyTaskResult));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_task_lists WHERE deleted_at IS NULL"), 0);
}

void GoogleMirrorStoreTest::cachesResolvedInstancesAndInvalidatesChangedSeries() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::SystemClock clock;
  hcb::GoogleMirrorStore store(*databasePath, clock);
  verifyReady(store);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
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
          "('google', 'google', 'connected', '[]', '[]', '2026-07-26T00:00:00Z')");
  std::future<hcb::GoogleMirrorWriteResult> initial =
      store.replaceCalendars(QStringLiteral("google"),
                             {{.id = QStringLiteral("primary"),
                               .title = QStringLiteral("Primary"),
                               .accessRole = hcb::GoogleCalendarAccessRole::Owner}},
                             {{.id = QStringLiteral("series"),
                               .calendarId = QStringLiteral("primary"),
                               .status = hcb::GoogleCalendarEventStatus::Confirmed,
                               .title = QStringLiteral("Daily"),
                               .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
                               .endAt = QStringLiteral("2026-07-26T10:00:00.000Z"),
                               .allDay = false,
                               .recurrence = {QStringLiteral("RRULE:FREQ=DAILY")}}});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(initial)));
  std::future<hcb::GoogleMirrorWriteResult> cached = store.cacheCalendarInstances(
      QStringLiteral("google"),
      QStringLiteral("primary"),
      QStringLiteral("series"),
      QStringLiteral("2026-07-27T00:00:00.000Z"),
      QStringLiteral("2026-08-03T00:00:00.000Z"),
      {{.id = QStringLiteral("series_20260727T090000Z"),
        .calendarId = QStringLiteral("primary"),
        .status = hcb::GoogleCalendarEventStatus::Confirmed,
        .title = QStringLiteral("Daily"),
        .startAt = QStringLiteral("2026-07-27T09:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-27T10:00:00.000Z"),
        .allDay = false,
        .recurringEventId = QStringLiteral("series"),
        .originalStartAt = QStringLiteral("2026-07-27T09:00:00.000Z")}});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(cached)));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendar_instance_coverage"), 1);
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendar_events WHERE is_instance_cache = 1"),
           1);
  std::future<hcb::GoogleMirrorWriteResult> changed = store.mergeCalendarEvents(
      QStringLiteral("google"),
      QStringLiteral("primary"),
      {{.id = QStringLiteral("series"),
        .calendarId = QStringLiteral("primary"),
        .status = hcb::GoogleCalendarEventStatus::Confirmed,
        .title = QStringLiteral("Changed daily"),
        .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-26T10:00:00.000Z"),
        .allDay = false,
        .recurrence = {QStringLiteral("RRULE:FREQ=DAILY;INTERVAL=2")}}},
      false);
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(changed)));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendar_instance_coverage"), 0);
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendar_events WHERE is_instance_cache = 1"),
           0);
}

void GoogleMirrorStoreTest::storesFullGoogleRecurrenceOutsideLegacyColumn() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::SystemClock clock;
  hcb::GoogleMirrorStore store(*databasePath, clock);
  verifyReady(store);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
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
          "('google', 'google', 'connected', '[]', '[]', '2026-07-26T00:00:00Z')");
  QStringList dates;
  dates.reserve(100);
  for (int index = 0; index < 100; ++index) {
    dates.append(QStringLiteral("20260726T090000Z"));
  }
  const QString rdate = QStringLiteral("RDATE:") + dates.join(u',');
  const QList<QString> recurrence{QStringLiteral("RRULE:FREQ=DAILY"), rdate, rdate, rdate};
  QVERIFY(recurrence.join(u'\n').size() > 4'096);
  std::future<hcb::GoogleMirrorWriteResult> write = store.replaceCalendars(
      QStringLiteral("google"),
      {{.id = QStringLiteral("primary"),
        .title = QStringLiteral("Primary"),
        .accessRole = hcb::GoogleCalendarAccessRole::Owner}},
      {{.id = QStringLiteral("series"),
        .calendarId = QStringLiteral("primary"),
        .status = hcb::GoogleCalendarEventStatus::Confirmed,
        .title = QStringLiteral("Long recurrence"),
        .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-26T10:00:00.000Z"),
        .allDay = false,
        .recurrence = recurrence}});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(write)));
  QCOMPARE(count(handle,
                 "SELECT COUNT(*) FROM local_calendar_events WHERE remote_id = 'series' "
                 "AND recurrence_rule IS NULL"),
           1);
  QCOMPARE(text(handle,
                "SELECT recurrences.recurrence_rule FROM local_calendar_event_recurrences AS recurrences "
                "INNER JOIN local_calendar_events AS events ON events.id = recurrences.event_id "
                "WHERE events.remote_id = 'series'"),
           recurrence.join(u'\n'));
}

void GoogleMirrorStoreTest::preservesQueuedApplyingAndRetryableRowsDuringPull() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::SystemClock clock;
  hcb::GoogleMirrorStore store(*databasePath, clock);
  verifyReady(store);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
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
          "('google', 'google', 'connected', '[]', '[]', '2026-07-26T00:00:00Z')");
  std::future<hcb::GoogleMirrorWriteResult> tasks =
      store.replaceTasks(QStringLiteral("google"),
                         {{.id = QStringLiteral("inbox"), .title = QStringLiteral("Inbox")}},
                         {{.id = QStringLiteral("task"),
                           .taskListId = QStringLiteral("inbox"),
                           .title = QStringLiteral("Local title"),
                           .status = hcb::GoogleTaskStatus::NeedsAction}});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(tasks)));
  std::future<hcb::GoogleMirrorWriteResult> calendars =
      store.replaceCalendars(QStringLiteral("google"),
                             {{.id = QStringLiteral("primary"),
                               .title = QStringLiteral("Primary"),
                               .accessRole = hcb::GoogleCalendarAccessRole::Owner}},
                             {{.id = QStringLiteral("event"),
                               .calendarId = QStringLiteral("primary"),
                               .status = hcb::GoogleCalendarEventStatus::Confirmed,
                               .title = QStringLiteral("Local event"),
                               .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
                               .endAt = QStringLiteral("2026-07-26T10:00:00.000Z")}});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(calendars)));
  execute(handle,
          "INSERT INTO local_pending_mutations "
          "(id, account_id, resource_type, resource_id, operation, payload_json, status, "
          "attempt_count, next_retry_at, created_at, updated_at) "
          "SELECT 'task-mutation', 'google', 'task', id, 'update', '{}', 'failed', 1, "
          "'2026-07-26T01:00:00Z', '2026-07-26T00:00:00Z', '2026-07-26T00:00:00Z' "
          "FROM local_tasks LIMIT 1");
  execute(handle,
          "INSERT INTO local_pending_mutations "
          "(id, account_id, resource_type, resource_id, operation, payload_json, status, "
          "attempt_count, lease_id, lease_expires_at, next_retry_at, created_at, updated_at) "
          "SELECT 'event-mutation', 'google', 'event', id, 'update', '{}', 'failed', 1, "
          "NULL, NULL, '2026-07-26T01:00:00Z', '2026-07-26T00:00:00Z', "
          "'2026-07-26T00:00:00Z' "
          "FROM local_calendar_events LIMIT 1");
  std::future<hcb::GoogleMirrorWriteResult> refreshedTasks =
      store.replaceTasks(QStringLiteral("google"), {}, {});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(refreshedTasks)));
  std::future<hcb::GoogleMirrorWriteResult> refreshedCalendars =
      store.replaceCalendars(QStringLiteral("google"), {}, {});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(refreshedCalendars)));
  QCOMPARE(text(handle, "SELECT title FROM local_tasks WHERE deleted_at IS NULL"),
           QStringLiteral("Local title"));
  QCOMPARE(text(handle, "SELECT title FROM local_calendar_events WHERE deleted_at IS NULL"),
           QStringLiteral("Local event"));
  std::future<hcb::GoogleMirrorWriteResult> calendarDeletion = store.mergeCalendars(
      QStringLiteral("google"),
      {{.id = QStringLiteral("primary"),
        .title = QStringLiteral("Deleted remotely"),
        .accessRole = hcb::GoogleCalendarAccessRole::Owner,
        .deleted = true}},
      false);
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(calendarDeletion)));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendars WHERE deleted_at IS NULL"), 1);
}

void GoogleMirrorStoreTest::appliesDeltasWithoutDeletingUnreturnedRows() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::SystemClock clock;
  hcb::GoogleMirrorStore store(*databasePath, clock);
  verifyReady(store);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
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
          "('google', 'google', 'connected', '[]', '[]', '2026-07-26T00:00:00Z')");
  std::future<hcb::GoogleMirrorWriteResult> initialTasks =
      store.replaceTasks(QStringLiteral("google"),
                         {{.id = QStringLiteral("inbox"), .title = QStringLiteral("Inbox")}},
                         {{.id = QStringLiteral("changed"),
                           .taskListId = QStringLiteral("inbox"),
                           .title = QStringLiteral("Before"),
                           .status = hcb::GoogleTaskStatus::NeedsAction},
                          {.id = QStringLiteral("untouched"),
                           .taskListId = QStringLiteral("inbox"),
                           .title = QStringLiteral("Untouched"),
                           .status = hcb::GoogleTaskStatus::NeedsAction},
                          {.id = QStringLiteral("deleted"),
                           .taskListId = QStringLiteral("inbox"),
                           .title = QStringLiteral("Deleted"),
                           .status = hcb::GoogleTaskStatus::NeedsAction}});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(initialTasks)));
  std::future<hcb::GoogleMirrorWriteResult> taskDelta =
      store.mergeTasks(QStringLiteral("google"),
                       QStringLiteral("inbox"),
                       {{.id = QStringLiteral("changed"),
                         .taskListId = QStringLiteral("inbox"),
                         .title = QStringLiteral("After"),
                         .status = hcb::GoogleTaskStatus::NeedsAction},
                        {.id = QStringLiteral("deleted"),
                         .taskListId = QStringLiteral("inbox"),
                         .title = QStringLiteral("Deleted"),
                         .status = hcb::GoogleTaskStatus::NeedsAction,
                         .deleted = true}},
                       false);
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(taskDelta)));
  QCOMPARE(text(handle, "SELECT title FROM local_tasks WHERE remote_id = 'changed'"),
           QStringLiteral("After"));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_tasks WHERE remote_id = 'untouched' "
                        "AND deleted_at IS NULL"),
           1);
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_tasks WHERE remote_id = 'deleted' "
                        "AND deleted_at IS NOT NULL"),
           1);

  std::future<hcb::GoogleMirrorWriteResult> initialCalendars =
      store.replaceCalendars(QStringLiteral("google"),
                             {{.id = QStringLiteral("primary"),
                               .title = QStringLiteral("Primary"),
                               .accessRole = hcb::GoogleCalendarAccessRole::Owner}},
                             {{.id = QStringLiteral("changed-event"),
                               .calendarId = QStringLiteral("primary"),
                               .status = hcb::GoogleCalendarEventStatus::Confirmed,
                               .title = QStringLiteral("Before"),
                               .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
                               .endAt = QStringLiteral("2026-07-26T10:00:00.000Z")},
                              {.id = QStringLiteral("untouched-event"),
                               .calendarId = QStringLiteral("primary"),
                               .status = hcb::GoogleCalendarEventStatus::Confirmed,
                               .title = QStringLiteral("Untouched"),
                               .startAt = QStringLiteral("2026-07-26T11:00:00.000Z"),
                               .endAt = QStringLiteral("2026-07-26T12:00:00.000Z")}});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(initialCalendars)));
  std::future<hcb::GoogleMirrorWriteResult> eventDelta =
      store.mergeCalendarEvents(QStringLiteral("google"),
                                QStringLiteral("primary"),
                                {{.id = QStringLiteral("changed-event"),
                                  .calendarId = QStringLiteral("primary"),
                                  .status = hcb::GoogleCalendarEventStatus::Confirmed,
                                  .title = QStringLiteral("After"),
                                  .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
                                  .endAt = QStringLiteral("2026-07-26T10:00:00.000Z")}},
                                false);
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(eventDelta)));
  QCOMPARE(text(handle, "SELECT title FROM local_calendar_events WHERE remote_id = 'changed-event'"),
           QStringLiteral("After"));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendar_events WHERE remote_id = "
                        "'untouched-event' AND deleted_at IS NULL"),
           1);
}

void GoogleMirrorStoreTest::recordsManagedRecurrencePullDiagnostics() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::SystemClock clock;
  hcb::GoogleMirrorStore store(*databasePath, clock);
  verifyReady(store);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
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
          "('google', 'google', 'connected', '[]', '[]', '2026-07-26T00:00:00Z')");
  const hcb::TaskRecurrenceMarker marker{
      .seriesId = QStringLiteral("8317b490-3a8f-4b09-a32a-a4ca7e2a7c22"),
      .occurrenceId = QStringLiteral("8317b490-3a8f-4b09-a32a-a4ca7e2a7c22:0"),
      .frequency = hcb::TaskRecurrenceFrequency::Weekly,
      .anchorDate = QStringLiteral("2026-07-26"),
      .timeZone = QStringLiteral("UTC"),
      .templateTitle = QStringLiteral("Recurring"),
      .templateDueDate = QStringLiteral("2026-07-26"),
      .templatePriority = QStringLiteral("none")};
  const hcb::TaskRecurrenceSerializationResult serialized =
      hcb::serializeTaskRecurrenceNotes(QStringLiteral("Body"), marker);
  QVERIFY(!serialized.error.has_value());
  if (serialized.error.has_value()) {
    return;
  }
  std::future<hcb::GoogleMirrorWriteResult> initial = store.replaceTasks(
      QStringLiteral("google"),
      {{.id = QStringLiteral("inbox"), .title = QStringLiteral("Inbox")}},
      {{.id = QStringLiteral("recurring"),
        .taskListId = QStringLiteral("inbox"),
        .title = QStringLiteral("Recurring"),
        .notes = serialized.notes,
        .status = hcb::GoogleTaskStatus::NeedsAction}});
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(initial)));

  hcb::TaskRecurrenceMarker changedMarker = marker;
  changedMarker.interval = 2;
  const hcb::TaskRecurrenceSerializationResult changed =
      hcb::serializeTaskRecurrenceNotes(QStringLiteral("Body"), changedMarker);
  QVERIFY(!changed.error.has_value());
  if (changed.error.has_value()) {
    return;
  }
  std::future<hcb::GoogleMirrorWriteResult> changedPull = store.mergeTasks(
      QStringLiteral("google"),
      QStringLiteral("inbox"),
      {{.id = QStringLiteral("recurring"),
        .taskListId = QStringLiteral("inbox"),
        .title = QStringLiteral("Recurring"),
        .notes = changed.notes,
        .status = hcb::GoogleTaskStatus::NeedsAction}},
      false);
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(changedPull)));
  QCOMPARE(text(handle, "SELECT recurrence_diagnostic FROM local_tasks WHERE remote_id = 'recurring'"),
           QStringLiteral("Managed recurrence marker changed in Google Tasks"));

  std::future<hcb::GoogleMirrorWriteResult> removedPull = store.mergeTasks(
      QStringLiteral("google"),
      QStringLiteral("inbox"),
      {{.id = QStringLiteral("recurring"),
        .taskListId = QStringLiteral("inbox"),
        .title = QStringLiteral("Recurring"),
        .notes = QStringLiteral("Body"),
        .status = hcb::GoogleTaskStatus::NeedsAction}},
      false);
  QVERIFY(std::holds_alternative<std::monostate>(awaitResult(removedPull)));
  QCOMPARE(text(handle, "SELECT recurrence_diagnostic FROM local_tasks WHERE remote_id = 'recurring'"),
           QStringLiteral("Managed recurrence marker was removed in Google Tasks"));
}

QTEST_GUILESS_MAIN(GoogleMirrorStoreTest)

#include "GoogleMirrorStoreTest.moc"
