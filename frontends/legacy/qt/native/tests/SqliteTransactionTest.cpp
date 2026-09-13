#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <optional>
#include <utility>
#include <variant>

#include "data/SqliteConnection.h"
#include "data/SqliteTransaction.h"
#include "sqlite3.h"

class SqliteTransactionTest final : public QObject {
  Q_OBJECT

private slots:
  void acquiresWriteLockImmediately();
  void commitsExplicitly();
  void rollsBackExplicitly();
  void rollsBackOnDestruction();
  void rejectsNestedTransactions();
};

namespace {

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

[[nodiscard]] std::optional<hcb::AppError> execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  if (result != SQLITE_OK) {
    return hcb::AppError(hcb::AppErrorCode::Database,
                         QStringLiteral("SQLite test setup failed (%1)").arg(result));
  }
  return std::nullopt;
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

[[nodiscard]] int rowCount(sqlite3* handle) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v2(handle, "SELECT COUNT(*) FROM transactions", -1, &statement, nullptr) !=
      SQLITE_OK) {
    return -1;
  }
  const int stepResult = sqlite3_step(statement);
  const int count = sqlite3_column_int(statement, 0);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_ROW || finalizeResult != SQLITE_OK) {
    return -1;
  }
  return count;
}

[[nodiscard]] std::optional<hcb::SqliteConnection>
preparedConnection(const hcb::FilePath& databasePath) {
  std::optional<hcb::SqliteConnection> connection = openConnection(databasePath);
  if (!connection.has_value() ||
      execute(connection->nativeHandle(), "CREATE TABLE transactions (id INTEGER PRIMARY KEY)")
          .has_value()) {
    return std::nullopt;
  }
  return connection;
}

} // namespace

void SqliteTransactionTest::acquiresWriteLockImmediately() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  std::optional<hcb::SqliteConnection> firstConnection = preparedConnection(*databasePath);
  std::optional<hcb::SqliteConnection> secondConnection = openConnection(*databasePath);
  QVERIFY(firstConnection.has_value());
  QVERIFY(secondConnection.has_value());
  if (!firstConnection.has_value() || !secondConnection.has_value()) {
    return;
  }

  hcb::SqliteTransactionResult firstResult = hcb::SqliteTransaction::begin(*firstConnection);
  QVERIFY(std::holds_alternative<hcb::SqliteTransaction>(firstResult));
  if (!std::holds_alternative<hcb::SqliteTransaction>(firstResult)) {
    return;
  }
  hcb::SqliteTransaction first = std::move(std::get<hcb::SqliteTransaction>(firstResult));
  QCOMPARE(sqlite3_busy_timeout(secondConnection->nativeHandle(), 0), SQLITE_OK);
  QCOMPARE(
      sqlite3_exec(secondConnection->nativeHandle(), "BEGIN IMMEDIATE", nullptr, nullptr, nullptr),
      SQLITE_BUSY);
  QVERIFY(!first.rollback().has_value());
}

void SqliteTransactionTest::commitsExplicitly() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  std::optional<hcb::SqliteConnection> connection = preparedConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  hcb::SqliteTransactionResult result = hcb::SqliteTransaction::begin(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteTransaction>(result));
  if (!std::holds_alternative<hcb::SqliteTransaction>(result)) {
    return;
  }
  hcb::SqliteTransaction transaction = std::move(std::get<hcb::SqliteTransaction>(result));
  QVERIFY(!execute(connection->nativeHandle(), "INSERT INTO transactions VALUES (1)").has_value());
  QVERIFY(!transaction.commit().has_value());
  QVERIFY(!transaction.active());
  QCOMPARE(rowCount(connection->nativeHandle()), 1);
}

void SqliteTransactionTest::rollsBackExplicitly() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  std::optional<hcb::SqliteConnection> connection = preparedConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  hcb::SqliteTransactionResult result = hcb::SqliteTransaction::begin(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteTransaction>(result));
  if (!std::holds_alternative<hcb::SqliteTransaction>(result)) {
    return;
  }
  hcb::SqliteTransaction transaction = std::move(std::get<hcb::SqliteTransaction>(result));
  QVERIFY(!execute(connection->nativeHandle(), "INSERT INTO transactions VALUES (1)").has_value());
  QVERIFY(!transaction.rollback().has_value());
  QVERIFY(!transaction.active());
  QCOMPARE(rowCount(connection->nativeHandle()), 0);
}

void SqliteTransactionTest::rollsBackOnDestruction() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  std::optional<hcb::SqliteConnection> connection = preparedConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  {
    hcb::SqliteTransactionResult result = hcb::SqliteTransaction::begin(*connection);
    QVERIFY(std::holds_alternative<hcb::SqliteTransaction>(result));
    if (!std::holds_alternative<hcb::SqliteTransaction>(result)) {
      return;
    }
    hcb::SqliteTransaction transaction = std::move(std::get<hcb::SqliteTransaction>(result));
    QVERIFY(
        !execute(connection->nativeHandle(), "INSERT INTO transactions VALUES (1)").has_value());
  }
  QCOMPARE(rowCount(connection->nativeHandle()), 0);
}

void SqliteTransactionTest::rejectsNestedTransactions() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  std::optional<hcb::SqliteConnection> connection = preparedConnection(*databasePath);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  hcb::SqliteTransactionResult firstResult = hcb::SqliteTransaction::begin(*connection);
  QVERIFY(std::holds_alternative<hcb::SqliteTransaction>(firstResult));
  if (!std::holds_alternative<hcb::SqliteTransaction>(firstResult)) {
    return;
  }
  hcb::SqliteTransaction first = std::move(std::get<hcb::SqliteTransaction>(firstResult));
  const hcb::SqliteTransactionResult secondResult = hcb::SqliteTransaction::begin(*connection);
  QVERIFY(std::holds_alternative<hcb::AppError>(secondResult));
  QVERIFY(first.active());
  QVERIFY(!first.rollback().has_value());
}

QTEST_GUILESS_MAIN(SqliteTransactionTest)

#include "SqliteTransactionTest.moc"
