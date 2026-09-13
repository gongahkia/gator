#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <optional>
#include <stdexcept>
#include <utility>
#include <variant>
#include <vector>

#include "data/SqliteConnection.h"
#include "data/SqliteMigrationRunner.h"
#include "sqlite3.h"

class SqliteMigrationRunnerTest final : public QObject {
  Q_OBJECT

private slots:
  void appliesAndSkipsRecordedMigrations();
  void rollsBackFailedMigrationWithoutRecordingIt();
  void rejectsInvalidMigrationChecksumBeforeCreatingMetadata();
  void rejectsModifiedMigrationChecksum();
  void acceptsDocumentedLegacyChecksum();
  void upgradesLegacyMigrationHistory();
  void rejectsMismatchedLegacyHistoryAtomically();
  void rejectsMalformedOrDiscontinuousHistory();
};

namespace {

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

[[nodiscard]] std::optional<hcb::SqliteConnection>
openConnection(const hcb::FilePath& databasePath) {
  hcb::SqliteConnectionResult result =
      hcb::SqliteConnectionFactory::open(databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  if (std::holds_alternative<hcb::AppError>(result)) {
    return std::nullopt;
  }
  return std::move(std::get<hcb::SqliteConnection>(result));
}

[[nodiscard]] std::optional<hcb::AppError> execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  if (result != SQLITE_OK) {
    return hcb::AppError(hcb::AppErrorCode::Database,
                         QStringLiteral("SQLite migration test command failed (%1)").arg(result));
  }
  return std::nullopt;
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
  if (stepResult != SQLITE_ROW || finalizeResult != SQLITE_OK) {
    return -1;
  }
  return value;
}

[[nodiscard]] std::optional<QString> scalarText(sqlite3* handle, const char* sql) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    return std::nullopt;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const char* const value = reinterpret_cast<const char*>(sqlite3_column_text(statement, 0));
  const int valueSize = sqlite3_column_bytes(statement, 0);
  std::optional<QString> result =
      value == nullptr ? std::nullopt : std::optional<QString>(QString::fromUtf8(value, valueSize));
  const int finalizeResult = sqlite3_finalize(statement);
  if (!result.has_value() || finalizeResult != SQLITE_OK) {
    return std::nullopt;
  }
  return result;
}

[[nodiscard]] QString checksum(QChar value) { return QString(64, value); }

[[nodiscard]] std::vector<hcb::SqliteMigration> migrations() {
  return {
      {1,
       QStringLiteral("create entries"),
       checksum(QLatin1Char('a')),
       [](hcb::SqliteConnection& connection) {
         return execute(
             connection.nativeHandle(),
             "CREATE TABLE entries (value INTEGER NOT NULL); INSERT INTO entries VALUES (1)");
       }},
      {2,
       QStringLiteral("add entry"),
       checksum(QLatin1Char('b')),
       [](hcb::SqliteConnection& connection) {
         return execute(connection.nativeHandle(), "INSERT INTO entries VALUES (2)");
       }},
  };
}

} // namespace

void SqliteMigrationRunnerTest::appliesAndSkipsRecordedMigrations() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  const std::vector<hcb::SqliteMigration> catalogue = migrations();

  hcb::SqliteMigrationRunResultOrError firstResult =
      hcb::SqliteMigrationRunner::run(*connection, catalogue);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(firstResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(firstResult)) {
    return;
  }
  const hcb::SqliteMigrationRunResult first =
      std::get<hcb::SqliteMigrationRunResult>(std::move(firstResult));
  QCOMPARE(first.version, 2);
  QCOMPARE(first.appliedVersions, std::vector<int>({1, 2}));
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM entries"), 2);
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM local_schema_migrations"), 2);
  const std::optional<QString> recordedChecksum = scalarText(
      connection->nativeHandle(), "SELECT checksum FROM local_schema_migrations WHERE version = 1");
  QVERIFY(recordedChecksum.has_value());
  if (!recordedChecksum.has_value()) {
    return;
  }
  QCOMPARE(*recordedChecksum, checksum(QLatin1Char('a')));

  hcb::SqliteMigrationRunResultOrError secondResult =
      hcb::SqliteMigrationRunner::run(*connection, catalogue);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(secondResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(secondResult)) {
    return;
  }
  const hcb::SqliteMigrationRunResult second =
      std::get<hcb::SqliteMigrationRunResult>(std::move(secondResult));
  QVERIFY(second.appliedVersions.empty());
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM entries"), 2);
}

void SqliteMigrationRunnerTest::rollsBackFailedMigrationWithoutRecordingIt() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  std::vector<hcb::SqliteMigration> catalogue = migrations();
  catalogue.push_back(
      {3,
       QStringLiteral("fail after write"),
       checksum(QLatin1Char('c')),
       [](hcb::SqliteConnection& migrationConnection) {
         if (const std::optional<hcb::AppError> error =
                 execute(migrationConnection.nativeHandle(), "INSERT INTO entries VALUES (3)");
             error.has_value()) {
           return error;
         }
         return std::optional<hcb::AppError>(
             hcb::AppError(hcb::AppErrorCode::Database, QStringLiteral("migration failure")));
       }});

  const hcb::SqliteMigrationRunResultOrError result =
      hcb::SqliteMigrationRunner::run(*connection, catalogue);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM entries"), 2);
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM local_schema_migrations"), 2);
}

void SqliteMigrationRunnerTest::rejectsInvalidMigrationChecksumBeforeCreatingMetadata() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  const std::vector<hcb::SqliteMigration> invalid = {
      {1,
       QStringLiteral("invalid checksum"),
       QStringLiteral("not-a-sha256-checksum"),
       [](hcb::SqliteConnection&) { return std::nullopt; }},
  };

  const hcb::SqliteMigrationRunResultOrError result =
      hcb::SqliteMigrationRunner::run(*connection, invalid);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM sqlite_master WHERE name = 'local_schema_migrations'"),
           0);
}

void SqliteMigrationRunnerTest::rejectsModifiedMigrationChecksum() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  const std::vector<hcb::SqliteMigration> catalogue = migrations();
  const hcb::SqliteMigrationRunResultOrError initial =
      hcb::SqliteMigrationRunner::run(*connection, catalogue);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(initial));
  QVERIFY(!execute(connection->nativeHandle(),
                   "UPDATE local_schema_migrations SET checksum = "
                   "'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff' "
                   "WHERE version = 1")
               .has_value());

  const hcb::SqliteMigrationRunResultOrError result =
      hcb::SqliteMigrationRunner::run(*connection, catalogue);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM entries"), 2);
}

void SqliteMigrationRunnerTest::acceptsDocumentedLegacyChecksum() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  std::vector<hcb::SqliteMigration> catalogue = migrations();
  catalogue[0].acceptedLegacyChecksum = checksum(QLatin1Char('c'));
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(
      hcb::SqliteMigrationRunner::run(*connection, catalogue)));
  QVERIFY(!execute(connection->nativeHandle(),
                   "UPDATE local_schema_migrations SET checksum = "
                   "'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc' "
                   "WHERE version = 1")
               .has_value());

  const hcb::SqliteMigrationRunResultOrError result =
      hcb::SqliteMigrationRunner::run(*connection, catalogue);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(result));
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM entries"), 2);
}

void SqliteMigrationRunnerTest::upgradesLegacyMigrationHistory() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  QVERIFY(!execute(connection->nativeHandle(),
                   "CREATE TABLE entries (value INTEGER NOT NULL);"
                   "INSERT INTO entries VALUES (1);"
                   "CREATE TABLE local_schema_migrations ("
                   "version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);"
                   "INSERT INTO local_schema_migrations VALUES (1, 'create entries', "
                   "'2026-01-01T00:00:00Z')")
               .has_value());

  hcb::SqliteMigrationRunResultOrError result =
      hcb::SqliteMigrationRunner::run(*connection, migrations());
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(result));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(result)) {
    return;
  }
  const hcb::SqliteMigrationRunResult applied =
      std::get<hcb::SqliteMigrationRunResult>(std::move(result));
  QCOMPARE(applied.appliedVersions, std::vector<int>({2}));
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM entries"), 2);
  const std::optional<QString> legacyChecksum = scalarText(
      connection->nativeHandle(), "SELECT checksum FROM local_schema_migrations WHERE version = 1");
  QVERIFY(legacyChecksum.has_value());
  if (!legacyChecksum.has_value()) {
    return;
  }
  QCOMPARE(*legacyChecksum, checksum(QLatin1Char('a')));
}

void SqliteMigrationRunnerTest::rejectsMismatchedLegacyHistoryAtomically() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  QVERIFY(!execute(connection->nativeHandle(),
                   "CREATE TABLE entries (value INTEGER NOT NULL);"
                   "INSERT INTO entries VALUES (1);"
                   "CREATE TABLE local_schema_migrations ("
                   "version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);"
                   "INSERT INTO local_schema_migrations VALUES (1, 'unexpected name', "
                   "'2026-01-01T00:00:00Z')")
               .has_value());

  const hcb::SqliteMigrationRunResultOrError result =
      hcb::SqliteMigrationRunner::run(*connection, migrations());
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM entries"), 1);
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM pragma_table_info('local_schema_migrations') "
                  "WHERE name = 'checksum'"),
           0);
}

void SqliteMigrationRunnerTest::rejectsMalformedOrDiscontinuousHistory() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }

  const std::vector<hcb::SqliteMigration> malformed = {
      {2,
       QStringLiteral("later"),
       checksum(QLatin1Char('a')),
       [](hcb::SqliteConnection&) { return std::nullopt; }},
      {1,
       QStringLiteral("earlier"),
       checksum(QLatin1Char('b')),
       [](hcb::SqliteConnection&) { return std::nullopt; }},
  };
  const hcb::SqliteMigrationRunResultOrError malformedResult =
      hcb::SqliteMigrationRunner::run(*connection, malformed);
  QVERIFY(std::holds_alternative<hcb::AppError>(malformedResult));
  QCOMPARE(scalar(connection->nativeHandle(),
                  "SELECT COUNT(*) FROM sqlite_master WHERE name = 'local_schema_migrations'"),
           0);

  const std::vector<hcb::SqliteMigration> catalogue = migrations();
  hcb::SqliteMigrationRunResultOrError appliedResult =
      hcb::SqliteMigrationRunner::run(*connection, catalogue);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(appliedResult));
  QVERIFY(
      !execute(connection->nativeHandle(), "DELETE FROM local_schema_migrations WHERE version = 1")
           .has_value());
  const hcb::SqliteMigrationRunResultOrError discontinuousResult =
      hcb::SqliteMigrationRunner::run(*connection, catalogue);
  QVERIFY(std::holds_alternative<hcb::AppError>(discontinuousResult));
  QCOMPARE(scalar(connection->nativeHandle(), "SELECT COUNT(*) FROM entries"), 2);
}

QTEST_GUILESS_MAIN(SqliteMigrationRunnerTest)

#include "SqliteMigrationRunnerTest.moc"
