#include <QtTest/QTest>

#include <memory>
#include <optional>
#include <utility>
#include <variant>
#include <vector>

#include "data/LocalSchema.h"
#include "sqlite3.h"
#include "support/TemporarySqliteDatabase.h"

class LocalSchemaTest final : public QObject {
  Q_OBJECT

private slots:
  void createsSettingsSchemaAndRecordsMigration();
  void enforcesSettingsIntegrity();
  void createsAccountMetadataSchemaAndEnforcesIntegrity();
  void createsTaskListSchemaAndEnforcesIntegrity();
  void createsTaskAndSubtaskSchemaAndEnforcesIntegrity();
  void createsCalendarSchemaAndEnforcesIntegrity();
  void createsCalendarEventSchemaAndEnforcesIntegrity();
  void createsTaskBackedNoteProjectionAndIndexes();
  void createsPendingMutationSchemaAndEnforcesLifecycle();
  void createsSyncCheckpointSchemaAndEnforcesIntegrity();
  void createsFtsIndexesTriggersAndBackfillsExistingData();
};

namespace {

[[nodiscard]] std::unique_ptr<hcb::test::TemporarySqliteDatabase> createDatabase() {
  hcb::test::TemporarySqliteDatabaseResult result = hcb::test::TemporarySqliteDatabase::create();
  if (std::holds_alternative<hcb::AppError>(result)) {
    return nullptr;
  }
  return std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result));
}

[[nodiscard]] std::optional<hcb::SqliteConnection>
openConnection(const hcb::test::TemporarySqliteDatabase& database) {
  hcb::SqliteConnectionResult result = database.open(hcb::SqliteOpenMode::ReadWriteCreate);
  if (std::holds_alternative<hcb::AppError>(result)) {
    return std::nullopt;
  }
  return std::move(std::get<hcb::SqliteConnection>(result));
}

[[nodiscard]] int scalar(sqlite3* handle, const char* sql) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    return -1;
  }
  const int stepResult = sqlite3_step(statement);
  const int value = sqlite3_column_int(statement, 0);
  const int finalizeResult = sqlite3_finalize(statement);
  return stepResult == SQLITE_ROW && finalizeResult == SQLITE_OK ? value : -1;
}

[[nodiscard]] int execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  return result;
}

[[nodiscard]] std::optional<QString> scalarText(sqlite3* handle, const char* sql) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    return std::nullopt;
  }
  const int stepResult = sqlite3_step(statement);
  const char* const value = reinterpret_cast<const char*>(sqlite3_column_text(statement, 0));
  const int valueSize = sqlite3_column_bytes(statement, 0);
  std::optional<QString> result = stepResult == SQLITE_ROW && value != nullptr
                                      ? std::optional<QString>(QString::fromUtf8(value, valueSize))
                                      : std::nullopt;
  const int finalizeResult = sqlite3_finalize(statement);
  if (!result.has_value() || finalizeResult != SQLITE_OK) {
    return std::nullopt;
  }
  return result;
}

} // namespace

void LocalSchemaTest::createsSettingsSchemaAndRecordsMigration() {
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

  hcb::SqliteMigrationRunResultOrError firstResult = hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(firstResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(firstResult)) {
    return;
  }
  const hcb::SqliteMigrationRunResult first =
      std::get<hcb::SqliteMigrationRunResult>(std::move(firstResult));
  QCOMPARE(first.version, 25);
  QCOMPARE(first.appliedVersions,
           std::vector<int>(
               {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25}));
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' "
                  "AND name = 'local_settings'"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 1 "
                  "AND name = 'create local settings' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 2 "
                  "AND name = 'create local accounts' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 3 "
                  "AND name = 'create local task lists' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 4 "
                  "AND name = 'create local tasks' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' "
                  "AND name = 'local_mutation_telemetry'"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 5 "
                  "AND name = 'create local calendars' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 6 "
                  "AND name = 'create local calendar events' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 7 "
                  "AND name = 'create local note projection' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 8 "
                  "AND name = 'create local pending mutations' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 9 "
                  "AND name = 'create local sync checkpoints' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 10 "
                  "AND name = 'create local FTS indexes' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 11 "
                  "AND name = 'create local undo entries' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 12 "
                  "AND name = 'create local sync conflicts' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 13 "
                  "AND name = 'create local note FTS index' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 14 "
                  "AND name = 'add calendar event type' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 15 "
                  "AND name = 'expand calendar event metadata' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 16 "
                  "AND name = 'project all undated tasks as notes' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 17 "
                  "AND name = 'add task recurrence claims' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 19 "
                  "AND name = 'add calendar instance cache' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM local_schema_migrations WHERE version = 20 "
                  "AND name = 'store full calendar recurrence lines' AND length(checksum) = 64"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' "
                  "AND name = 'local_task_recurrence_claims'"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM pragma_table_info('local_tasks') "
                  "WHERE name = 'is_assigned'"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM pragma_table_info('local_tasks') "
                  "WHERE name = 'recurrence_diagnostic'"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' "
                  "AND name = 'local_calendar_event_recurrences'"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM pragma_table_info('local_calendars') "
                  "WHERE name = 'default_reminders_json'"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' "
                  "AND name = 'local_reminder_state'"),
           1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM pragma_table_info('local_calendars') "
                  "WHERE name = 'color_id'"),
           1);
  const std::optional<QString> settingsSchema = scalarText(
      connection->nativeHandle(), "SELECT sql FROM sqlite_master WHERE name = 'local_settings'");
  QVERIFY(settingsSchema.has_value());
  if (!settingsSchema.has_value()) {
    return;
  }
  QVERIFY(settingsSchema->contains(QStringLiteral("STRICT, WITHOUT ROWID")));

  hcb::SqliteMigrationRunResultOrError secondResult = hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(secondResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(secondResult)) {
    return;
  }
  const hcb::SqliteMigrationRunResult second =
      std::get<hcb::SqliteMigrationRunResult>(std::move(secondResult));
  QCOMPARE(second.version, 25);
  QVERIFY(second.appliedVersions.empty());
}

void LocalSchemaTest::enforcesSettingsIntegrity() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_settings (scope, key, value_json, updated_at) "
                   "VALUES ('appearance', 'theme', '\"system\"', '2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_settings (scope, key, value_json, updated_at) "
                   "VALUES ('appearance', 'theme', '\"dark\"', '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_settings (scope, key, value_json, updated_at) "
                   "VALUES ('appearance', 'bad-json', 'not-json', '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_settings (scope, key, value_json, updated_at) "
                   "VALUES (' ', 'empty-scope', 'true', '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_settings (scope, key, value_json, updated_at) "
                   "VALUES ('appearance', 'empty-time', 'true', ' ')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_settings"), 1);
}

void LocalSchemaTest::createsAccountMetadataSchemaAndEnforcesIntegrity() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, provider_account_id, email, "
                   "connection_state, granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'google-1', 'person@example.com', "
                   "'connected', '[\"openid\"]', '[]', '2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, provider_account_id, "
                   "connection_state, granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-2', 'google', 'google-1', 'signed_out', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-3', 'google', 'unknown', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-4', 'google', 'signed_out', '{}', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_accounts"), 1);
  const std::optional<QString> accountSchema =
      scalarText(handle, "SELECT sql FROM sqlite_master WHERE name = 'local_accounts'");
  QVERIFY(accountSchema.has_value());
  if (!accountSchema.has_value()) {
    return;
  }
  QVERIFY(accountSchema->contains(QStringLiteral("STRICT, WITHOUT ROWID")));
  QVERIFY(!accountSchema->contains(QStringLiteral("token"), Qt::CaseInsensitive));
}

void LocalSchemaTest::createsTaskListSchemaAndEnforcesIntegrity() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'connected', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('list-1', 'account-1', 'google-list-1', 'Inbox', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('list-2', 'account-1', 'google-list-1', 'Duplicate', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_task_lists (id, account_id, remote_id, title, is_selected, "
                   "updated_at) VALUES ('list-3', 'account-1', 'google-list-3', 'Bad selection', "
                   "2, '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('list-4', 'missing-account', 'google-list-4', 'Orphan', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_task_lists"), 1);
  const std::optional<QString> taskListSchema =
      scalarText(handle, "SELECT sql FROM sqlite_master WHERE name = 'local_task_lists'");
  QVERIFY(taskListSchema.has_value());
  if (!taskListSchema.has_value()) {
    return;
  }
  QVERIFY(taskListSchema->contains(QStringLiteral("STRICT, WITHOUT ROWID")));
}

void LocalSchemaTest::createsTaskAndSubtaskSchemaAndEnforcesIntegrity() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'connected', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('list-1', 'account-1', 'google-list-1', 'Inbox', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('list-2', 'account-1', 'google-list-2', 'Later', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_tasks (id, task_list_id, remote_id, title, updated_at) "
                   "VALUES ('task-1', 'list-1', 'google-task-1', 'Parent', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_tasks (id, task_list_id, remote_id, parent_task_id, title, "
                   "updated_at) VALUES ('task-2', 'list-1', 'google-task-2', 'task-1', 'Child', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_tasks (id, task_list_id, remote_id, title, updated_at) "
                   "VALUES ('task-3', 'list-1', 'google-task-1', 'Duplicate', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_tasks (id, task_list_id, remote_id, parent_task_id, title, "
              "updated_at) VALUES ('task-4', 'list-2', 'google-task-4', 'task-1', 'Cross list', "
              "'2026-07-24T00:00:00Z')"),
      SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle, "UPDATE local_tasks SET parent_task_id = 'task-2' WHERE id = 'task-1'"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_tasks (id, task_list_id, remote_id, title, tags_json, "
                   "updated_at) VALUES ('task-5', 'list-1', 'google-task-5', 'Bad tags', '{}', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_tasks"), 2);
  const std::optional<QString> taskSchema =
      scalarText(handle, "SELECT sql FROM sqlite_master WHERE name = 'local_tasks'");
  QVERIFY(taskSchema.has_value());
  if (!taskSchema.has_value()) {
    return;
  }
  QVERIFY(taskSchema->contains(QStringLiteral("STRICT, WITHOUT ROWID")));
}

void LocalSchemaTest::createsCalendarSchemaAndEnforcesIntegrity() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'connected', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendars (id, account_id, remote_id, title, "
                   "background_color, foreground_color, access_role, is_primary, updated_at) "
                   "VALUES ('calendar-1', 'account-1', 'google-calendar-1', 'Primary', "
                   "'#112233', '#ffffff', 'owner', 1, '2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendars (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('calendar-2', 'account-1', 'google-calendar-1', 'Duplicate', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendars (id, account_id, remote_id, title, is_primary, "
                   "updated_at) VALUES ('calendar-3', 'account-1', 'google-calendar-3', 'Second', "
                   "1, '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_calendars (id, account_id, remote_id, title, background_color, "
              "updated_at) VALUES ('calendar-4', 'account-1', 'google-calendar-4', 'Bad color', "
              "'#bad', '2026-07-24T00:00:00Z')"),
      SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendars (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('calendar-5', 'missing-account', 'google-calendar-5', 'Orphan', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_calendars"), 1);
  const std::optional<QString> calendarSchema =
      scalarText(handle, "SELECT sql FROM sqlite_master WHERE name = 'local_calendars'");
  QVERIFY(calendarSchema.has_value());
  if (!calendarSchema.has_value()) {
    return;
  }
  QVERIFY(calendarSchema->contains(QStringLiteral("STRICT, WITHOUT ROWID")));
}

void LocalSchemaTest::createsCalendarEventSchemaAndEnforcesIntegrity() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'connected', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendars (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('calendar-1', 'account-1', 'google-calendar-1', 'Primary', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendar_events (id, calendar_id, remote_id, "
                   "recurring_remote_id, original_start_at, title, start_at, end_at, "
                   "attendee_emails_json, attendee_details_json, reminders_json, updated_at) "
                   "VALUES ('event-1', 'calendar-1', 'google-event-1', 'google-series-1', "
                   "'2026-07-24T09:00:00Z', 'Planning', '2026-07-24T09:00:00Z', "
                   "'2026-07-24T10:00:00Z', '[\"person@example.com\"]', "
                   "'[{\"email\":\"person@example.com\"}]', "
                   "'[{\"method\":\"popup\",\"minutes\":15}]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendar_events (id, calendar_id, remote_id, title, "
                   "start_at, end_at, updated_at) VALUES ('event-2', 'calendar-1', "
                   "'google-event-1', 'Duplicate', '2026-07-24T11:00:00Z', "
                   "'2026-07-24T12:00:00Z', '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendar_events (id, calendar_id, title, start_at, end_at, "
                   "updated_at) VALUES ('event-pending', 'calendar-1', 'Offline create', "
                   "'2026-07-24T11:00:00Z', '2026-07-24T12:00:00Z', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendar_events (id, calendar_id, remote_id, status, title, "
                   "start_at, end_at, updated_at) VALUES ('event-cancelled', 'calendar-1', "
                   "'google-event-cancelled', 'cancelled', 'Cancelled tombstone', "
                   "'2026-07-24T00:00:00Z', '2026-07-24T00:00:00Z', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendar_events (id, calendar_id, remote_id, title, "
                   "start_at, end_at, updated_at) VALUES ('event-3', 'calendar-1', "
                   "'google-event-3', 'Invalid range', '2026-07-24T12:00:00Z', "
                   "'2026-07-24T11:00:00Z', '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendar_events (id, calendar_id, remote_id, title, "
                   "start_at, end_at, attendee_details_json, updated_at) VALUES ('event-4', "
                   "'calendar-1', 'google-event-4', 'Bad attendees', '2026-07-24T13:00:00Z', "
                   "'2026-07-24T14:00:00Z', '{}', '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendar_events (id, calendar_id, remote_id, title, "
                   "start_at, end_at, updated_at) VALUES ('event-5', 'missing-calendar', "
                   "'google-event-5', 'Orphan', '2026-07-24T13:00:00Z', "
                   "'2026-07-24T14:00:00Z', '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_calendar_events"), 3);
  const std::optional<QString> eventSchema =
      scalarText(handle, "SELECT sql FROM sqlite_master WHERE name = 'local_calendar_events'");
  QVERIFY(eventSchema.has_value());
  if (!eventSchema.has_value()) {
    return;
  }
  QVERIFY(eventSchema->contains(QStringLiteral("STRICT, WITHOUT ROWID")));
}

void LocalSchemaTest::createsTaskBackedNoteProjectionAndIndexes() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'connected', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('list-1', 'account-1', 'google-list-1', 'Notes', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_tasks (id, task_list_id, remote_id, title, notes, updated_at) "
              "VALUES ('note-1', 'list-1', 'google-note-1', 'Project note', 'Body', "
              "'2026-07-24T00:00:00Z')"),
      SQLITE_OK);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_tasks (id, task_list_id, remote_id, title, due_at, updated_at) "
              "VALUES ('task-due', 'list-1', 'google-task-due', 'Task', '2026-07-25T00:00:00Z', "
              "'2026-07-24T00:00:00Z')"),
      SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_tasks (id, task_list_id, remote_id, parent_task_id, title, "
                   "updated_at) VALUES ('task-child', 'list-1', 'google-task-child', 'note-1', "
                   "'Child', '2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_tasks (id, task_list_id, remote_id, title, state, updated_at) "
              "VALUES ('task-completed', 'list-1', 'google-task-completed', 'Completed', "
              "'completed', '2026-07-24T00:00:00Z')"),
      SQLITE_OK);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_tasks (id, task_list_id, remote_id, title, is_hidden, updated_at) "
              "VALUES ('task-hidden', 'list-1', 'google-task-hidden', 'Hidden', 1, "
              "'2026-07-24T00:00:00Z')"),
      SQLITE_OK);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_note_projections"), 3);
  QCOMPARE(scalarText(handle, "SELECT body FROM local_note_projections WHERE id = 'note-1'"),
           std::optional<QString>(QStringLiteral("Body")));
  QCOMPARE(scalar(handle,
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'view' "
                  "AND name = 'local_note_projections'"),
           1);
  QCOMPARE(scalar(handle,
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' "
                  "AND name = 'local_note_projections'"),
           0);
  QCOMPARE(scalar(handle,
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' "
                  "AND name = 'local_tasks_active_note_recency'"),
           1);
  QCOMPARE(scalar(handle,
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' "
                  "AND name = 'local_notes_fts'"),
           1);
  QCOMPARE(scalarText(handle,
                      "SELECT note_id FROM local_notes_fts "
                      "WHERE local_notes_fts MATCH 'project*'"),
           std::optional<QString>(QStringLiteral("note-1")));
  QCOMPARE(scalarText(handle,
                      "SELECT note_id FROM local_notes_fts "
                      "WHERE local_notes_fts MATCH 'child*'"),
           std::optional<QString>(QStringLiteral("task-child")));
  QCOMPARE(scalarText(handle,
                      "SELECT note_id FROM local_notes_fts "
                      "WHERE local_notes_fts MATCH 'completed*'"),
           std::optional<QString>(QStringLiteral("task-completed")));
  QCOMPARE(
      execute(handle, "UPDATE local_tasks SET due_at = '2026-07-25T00:00:00Z' WHERE id = 'note-1'"),
      SQLITE_OK);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_notes_fts"), 2);
  QCOMPARE(
      execute(handle,
              "UPDATE local_tasks SET due_at = NULL, notes = 'Revised body' WHERE id = 'note-1'"),
      SQLITE_OK);
  QCOMPARE(scalarText(handle,
                      "SELECT note_id FROM local_notes_fts "
                      "WHERE local_notes_fts MATCH 'revis*'"),
           std::optional<QString>(QStringLiteral("note-1")));
  QCOMPARE(
      execute(
          handle,
          "UPDATE local_task_lists SET deleted_at = '2026-07-25T00:00:00Z' WHERE id = 'list-1'"),
      SQLITE_OK);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_notes_fts"), 0);
  QCOMPARE(execute(handle, "UPDATE local_task_lists SET deleted_at = NULL WHERE id = 'list-1'"),
           SQLITE_OK);
  QCOMPARE(scalarText(handle,
                      "SELECT note_id FROM local_notes_fts "
                      "WHERE local_notes_fts MATCH 'revis*'"),
           std::optional<QString>(QStringLiteral("note-1")));
}

void LocalSchemaTest::createsPendingMutationSchemaAndEnforcesLifecycle() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'connected', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_pending_mutations (id, account_id, resource_type, "
                   "resource_id, operation, payload_json, updated_at) VALUES ('mutation-1', "
                   "'account-1', 'task', 'task-1', 'task.create', '{\"title\":\"Write\"}', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_pending_mutations (id, resource_type, resource_id, operation, "
              "payload_json, updated_at) VALUES ('mutation-2', 'unknown', 'task-2', "
              "'task.create', '{}', '2026-07-24T00:00:00Z')"),
      SQLITE_CONSTRAINT);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_pending_mutations (id, resource_type, resource_id, operation, "
              "payload_json, updated_at) VALUES ('mutation-3', 'task', 'task-3', "
              "'task.create', 'not-json', '2026-07-24T00:00:00Z')"),
      SQLITE_CONSTRAINT);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_pending_mutations (id, resource_type, resource_id, operation, "
              "payload_json, status, updated_at) VALUES ('mutation-4', 'task', 'task-4', "
              "'task.create', '{}', 'applying', '2026-07-24T00:00:00Z')"),
      SQLITE_CONSTRAINT);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_pending_mutations (id, resource_type, resource_id, operation, "
              "payload_json, status, lease_id, lease_expires_at, updated_at) VALUES "
              "('mutation-5', 'event', 'event-1', 'calendar.events.create', '{}', "
              "'applying', 'worker-1', '2026-07-24T00:05:00Z', "
              "'2026-07-24T00:00:00Z')"),
      SQLITE_OK);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_pending_mutations (id, resource_type, resource_id, operation, "
              "payload_json, status, updated_at) VALUES ('mutation-6', 'task', 'task-6', "
              "'task.create', '{}', 'applied', '2026-07-24T00:00:00Z')"),
      SQLITE_CONSTRAINT);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_pending_mutations (id, resource_type, resource_id, operation, "
              "payload_json, status, applied_at, updated_at) VALUES ('mutation-7', 'task_list', "
              "'list-1', 'task_list.create', '{}', 'applied', '2026-07-24T00:00:00Z', "
              "'2026-07-24T00:00:00Z')"),
      SQLITE_OK);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_pending_mutations (id, account_id, resource_type, resource_id, "
              "operation, payload_json, updated_at) VALUES ('mutation-8', 'missing-account', "
              "'task', 'task-8', 'task.create', '{}', '2026-07-24T00:00:00Z')"),
      SQLITE_CONSTRAINT);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_pending_mutations"), 3);
  const std::optional<QString> mutationSchema =
      scalarText(handle, "SELECT sql FROM sqlite_master WHERE name = 'local_pending_mutations'");
  QVERIFY(mutationSchema.has_value());
  if (!mutationSchema.has_value()) {
    return;
  }
  QVERIFY(mutationSchema->contains(QStringLiteral("STRICT, WITHOUT ROWID")));
  QCOMPARE(scalar(handle,
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' "
                  "AND name = 'local_pending_mutations_due'"),
           1);
  QCOMPARE(scalar(handle,
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' "
                  "AND name = 'local_pending_mutations_expired_lease'"),
           1);
}

void LocalSchemaTest::createsSyncCheckpointSchemaAndEnforcesIntegrity() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'connected', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_sync_checkpoints (id, account_id, resource_type, "
                   "resource_id, checkpoint_type, checkpoint_value, metadata_json, "
                   "last_successful_sync_at, updated_at) VALUES ('checkpoint-1', 'account-1', "
                   "'calendar', 'primary', 'sync_token', 'google-sync-token', "
                   "'{\"fullResync\":false}', '2026-07-24T00:00:00Z', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_sync_checkpoints (id, account_id, resource_type, "
                   "resource_id, checkpoint_type, checkpoint_value, last_successful_sync_at, "
                   "updated_at) VALUES ('checkpoint-2', 'account-1', 'calendar', 'primary', "
                   "'sync_token', 'duplicate', '2026-07-24T00:00:00Z', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_sync_checkpoints (id, account_id, resource_type, "
                   "resource_id, checkpoint_type, checkpoint_value, last_successful_sync_at, "
                   "updated_at) VALUES ('checkpoint-3', 'account-1', 'unknown', 'resource-3', "
                   "'sync_token', 'value', '2026-07-24T00:00:00Z', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_sync_checkpoints (id, account_id, resource_type, "
                   "resource_id, checkpoint_type, checkpoint_value, metadata_json, "
                   "last_successful_sync_at, updated_at) VALUES ('checkpoint-4', 'account-1', "
                   "'task_list', 'list-1', 'watermark:v3-split-completed', 'value', '[]', "
                   "'2026-07-24T00:00:00Z', '2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_sync_checkpoints (id, account_id, resource_type, "
                   "resource_id, checkpoint_type, checkpoint_value, last_successful_sync_at, "
                   "updated_at) VALUES ('checkpoint-5', 'missing-account', 'task_list', 'list-5', "
                   "'watermark', 'value', '2026-07-24T00:00:00Z', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_CONSTRAINT);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_sync_checkpoints"), 1);
  const std::optional<QString> checkpointSchema =
      scalarText(handle, "SELECT sql FROM sqlite_master WHERE name = 'local_sync_checkpoints'");
  QVERIFY(checkpointSchema.has_value());
  if (!checkpointSchema.has_value()) {
    return;
  }
  QVERIFY(checkpointSchema->contains(QStringLiteral("STRICT, WITHOUT ROWID")));
  QCOMPARE(scalar(handle,
                  "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' "
                  "AND name = 'local_sync_checkpoints_account_recency'"),
           1);
}

void LocalSchemaTest::createsFtsIndexesTriggersAndBackfillsExistingData() {
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
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }

  sqlite3* const handle = connection->nativeHandle();
  QCOMPARE(execute(handle,
                   "INSERT INTO local_accounts (id, provider, connection_state, "
                   "granted_scopes_json, missing_scopes_json, updated_at) "
                   "VALUES ('account-1', 'google', 'connected', '[]', '[]', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('list-1', 'account-1', 'google-list-1', 'Project work', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(
      execute(handle,
              "INSERT INTO local_tasks (id, task_list_id, remote_id, title, notes, updated_at) "
              "VALUES ('task-1', 'list-1', 'google-task-1', 'Ship search', "
              "'Initial investigation', '2026-07-24T00:00:00Z')"),
      SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendars (id, account_id, remote_id, title, updated_at) "
                   "VALUES ('calendar-1', 'account-1', 'primary', 'Engineering', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(execute(handle,
                   "INSERT INTO local_calendar_events (id, calendar_id, remote_id, title, "
                   "description, location, start_at, end_at, updated_at) VALUES ('event-1', "
                   "'calendar-1', 'google-event-1', 'Search review', 'Initial design review', "
                   "'Room 5', '2026-07-24T10:00:00Z', '2026-07-24T11:00:00Z', "
                   "'2026-07-24T00:00:00Z')"),
           SQLITE_OK);
  QCOMPARE(scalarText(handle,
                      "SELECT task_list_id FROM local_task_lists_fts "
                      "WHERE local_task_lists_fts MATCH 'proj*'"),
           std::optional<QString>(QStringLiteral("list-1")));
  QCOMPARE(scalarText(handle,
                      "SELECT task_id FROM local_tasks_fts WHERE local_tasks_fts MATCH 'invest*'"),
           std::optional<QString>(QStringLiteral("task-1")));
  QCOMPARE(scalarText(handle,
                      "SELECT calendar_id FROM local_calendars_fts "
                      "WHERE local_calendars_fts MATCH 'engine*'"),
           std::optional<QString>(QStringLiteral("calendar-1")));
  QCOMPARE(scalarText(handle,
                      "SELECT calendar_event_id FROM local_calendar_events_fts "
                      "WHERE local_calendar_events_fts MATCH 'room*'"),
           std::optional<QString>(QStringLiteral("event-1")));

  QCOMPARE(
      execute(handle, "UPDATE local_tasks SET notes = 'Revised investigation' WHERE id = 'task-1'"),
      SQLITE_OK);
  QCOMPARE(
      scalar(handle, "SELECT COUNT(*) FROM local_tasks_fts WHERE local_tasks_fts MATCH 'initial*'"),
      0);
  QCOMPARE(scalarText(handle,
                      "SELECT task_id FROM local_tasks_fts WHERE local_tasks_fts MATCH 'revis*'"),
           std::optional<QString>(QStringLiteral("task-1")));
  QCOMPARE(
      execute(handle, "UPDATE local_calendar_events SET location = 'Room 7' WHERE id = 'event-1'"),
      SQLITE_OK);
  QCOMPARE(scalar(handle,
                  "SELECT COUNT(*) FROM local_calendar_events_fts "
                  "WHERE local_calendar_events_fts MATCH 'room 5'"),
           0);
  QCOMPARE(scalarText(handle,
                      "SELECT calendar_event_id FROM local_calendar_events_fts "
                      "WHERE local_calendar_events_fts MATCH 'room 7'"),
           std::optional<QString>(QStringLiteral("event-1")));
  QCOMPARE(execute(handle, "DELETE FROM local_tasks WHERE id = 'task-1'"), SQLITE_OK);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_tasks_fts"), 0);

  QCOMPARE(
      execute(
          handle,
          "DROP TRIGGER local_task_lists_fts_insert; "
          "DROP TRIGGER local_task_lists_fts_delete; "
          "DROP TRIGGER local_task_lists_fts_update; "
          "DROP TRIGGER local_tasks_fts_insert; "
          "DROP TRIGGER local_tasks_fts_delete; "
          "DROP TRIGGER local_tasks_fts_update; "
          "DROP TRIGGER local_calendars_fts_insert; "
          "DROP TRIGGER local_calendars_fts_delete; "
          "DROP TRIGGER local_calendars_fts_update; "
          "DROP TRIGGER local_calendar_events_fts_insert; "
          "DROP TRIGGER local_calendar_events_fts_delete; "
          "DROP TRIGGER local_calendar_events_fts_update; "
          "DROP TRIGGER local_notes_fts_task_insert; "
          "DROP TRIGGER local_notes_fts_task_delete; "
          "DROP TRIGGER local_notes_fts_task_update; "
          "DROP TRIGGER local_notes_fts_task_list_update; "
          "DROP TABLE local_task_lists_fts; "
          "DROP TABLE local_tasks_fts; "
          "DROP TABLE local_calendars_fts; "
          "DROP TABLE local_calendar_events_fts; "
          "DROP TABLE local_notes_fts; "
          "DROP TABLE local_undo_entries; "
          "DROP TABLE local_sync_conflicts; "
          "DROP INDEX local_task_recurrence_claims_successor; "
          "DROP TABLE local_task_recurrence_claims; "
          "DROP INDEX local_calendar_events_instance_cache; "
          "DROP TABLE local_calendar_instance_coverage; "
          "DROP TABLE local_calendar_event_recurrences; "
          "DROP TABLE local_reminder_state; "
          "DROP TABLE local_mutation_telemetry; "
          "ALTER TABLE local_tasks DROP COLUMN is_assigned; "
          "ALTER TABLE local_tasks DROP COLUMN recurrence_diagnostic; "
          "ALTER TABLE local_calendar_events DROP COLUMN event_type; "
          "ALTER TABLE local_calendar_events DROP COLUMN is_instance_cache; "
          "ALTER TABLE local_calendars DROP COLUMN default_reminders_json; "
          "ALTER TABLE local_calendars DROP COLUMN color_id; "
          "DELETE FROM local_schema_migrations WHERE version IN (10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25)"),
      SQLITE_OK);
  hcb::SqliteMigrationRunResultOrError backfillResult = hcb::LocalSchema::initialize(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(backfillResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(backfillResult)) {
    return;
  }
  const hcb::SqliteMigrationRunResult backfill =
      std::get<hcb::SqliteMigrationRunResult>(std::move(backfillResult));
  QCOMPARE(backfill.appliedVersions,
           std::vector<int>({10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25}));
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_task_lists_fts"), 1);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_tasks_fts"), 0);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_calendars_fts"), 1);
  QCOMPARE(scalar(handle, "SELECT COUNT(*) FROM local_calendar_events_fts"), 1);
  QCOMPARE(scalarText(handle,
                      "SELECT calendar_event_id FROM local_calendar_events_fts "
                      "WHERE local_calendar_events_fts MATCH 'room 7'"),
           std::optional<QString>(QStringLiteral("event-1")));
}

QTEST_GUILESS_MAIN(LocalSchemaTest)

#include "LocalSchemaTest.moc"
