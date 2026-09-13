#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <optional>
#include <utility>
#include <variant>

#include "data/SqliteConnection.h"
#include "data/SqliteStatementCache.h"
#include "sqlite3.h"

class SqliteStatementCacheTest final : public QObject {
  Q_OBJECT

private slots:
  void reusesStatementsAndClearsBindings();
  void isolatesConcurrentLeases();
  void rejectsInvalidOrMultipleStatements();
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

[[nodiscard]] std::size_t statementCount(sqlite3* connection) {
  std::size_t count = 0;
  for (sqlite3_stmt* statement = sqlite3_next_stmt(connection, nullptr); statement != nullptr;
       statement = sqlite3_next_stmt(connection, statement)) {
    ++count;
  }
  return count;
}

} // namespace

void SqliteStatementCacheTest::reusesStatementsAndClearsBindings() {
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

  {
    hcb::SqliteStatementCache cache(*connection, 1);
    hcb::SqlitePreparedStatementResult firstResult =
        cache.acquire(QStringLiteral("SELECT ?1 IS NULL"));
    QVERIFY(std::holds_alternative<hcb::SqlitePreparedStatement>(firstResult));
    if (!std::holds_alternative<hcb::SqlitePreparedStatement>(firstResult)) {
      return;
    }
    hcb::SqlitePreparedStatement first =
        std::move(std::get<hcb::SqlitePreparedStatement>(firstResult));
    sqlite3_stmt* const firstHandle = first.nativeHandle();
    QCOMPARE(sqlite3_bind_int(firstHandle, 1, 42), SQLITE_OK);
    QCOMPARE(sqlite3_step(firstHandle), SQLITE_ROW);
    QCOMPARE(sqlite3_column_int(firstHandle, 0), 0);
    QVERIFY(!first.release().has_value());

    hcb::SqlitePreparedStatementResult secondResult =
        cache.acquire(QStringLiteral("SELECT ?1 IS NULL"));
    QVERIFY(std::holds_alternative<hcb::SqlitePreparedStatement>(secondResult));
    if (!std::holds_alternative<hcb::SqlitePreparedStatement>(secondResult)) {
      return;
    }
    hcb::SqlitePreparedStatement second =
        std::move(std::get<hcb::SqlitePreparedStatement>(secondResult));
    QCOMPARE(second.nativeHandle(), firstHandle);
    QCOMPARE(sqlite3_step(second.nativeHandle()), SQLITE_ROW);
    QCOMPARE(sqlite3_column_int(second.nativeHandle(), 0), 1);
    QVERIFY(!second.release().has_value());
    QCOMPARE(statementCount(connection->nativeHandle()), static_cast<std::size_t>(1));
  }
  QCOMPARE(statementCount(connection->nativeHandle()), static_cast<std::size_t>(0));
}

void SqliteStatementCacheTest::isolatesConcurrentLeases() {
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

  hcb::SqliteStatementCache cache(*connection, 1);
  hcb::SqlitePreparedStatementResult firstResult = cache.acquire(QStringLiteral("SELECT 1"));
  QVERIFY(std::holds_alternative<hcb::SqlitePreparedStatement>(firstResult));
  if (!std::holds_alternative<hcb::SqlitePreparedStatement>(firstResult)) {
    return;
  }
  hcb::SqlitePreparedStatement first =
      std::move(std::get<hcb::SqlitePreparedStatement>(firstResult));
  hcb::SqlitePreparedStatementResult secondResult = cache.acquire(QStringLiteral("SELECT 1"));
  QVERIFY(std::holds_alternative<hcb::SqlitePreparedStatement>(secondResult));
  if (!std::holds_alternative<hcb::SqlitePreparedStatement>(secondResult)) {
    return;
  }
  hcb::SqlitePreparedStatement second =
      std::move(std::get<hcb::SqlitePreparedStatement>(secondResult));
  QVERIFY(first.nativeHandle() != second.nativeHandle());
  QCOMPARE(statementCount(connection->nativeHandle()), static_cast<std::size_t>(2));
  QVERIFY(!first.release().has_value());
  QVERIFY(!second.release().has_value());
  QCOMPARE(statementCount(connection->nativeHandle()), static_cast<std::size_t>(1));
}

void SqliteStatementCacheTest::rejectsInvalidOrMultipleStatements() {
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

  hcb::SqliteStatementCache cache(*connection, 1);
  const hcb::SqlitePreparedStatementResult emptyResult = cache.acquire(QString());
  QVERIFY(std::holds_alternative<hcb::AppError>(emptyResult));
  const hcb::SqlitePreparedStatementResult multipleResult =
      cache.acquire(QStringLiteral("SELECT 1; SELECT 2"));
  QVERIFY(std::holds_alternative<hcb::AppError>(multipleResult));
  QCOMPARE(statementCount(connection->nativeHandle()), static_cast<std::size_t>(0));
}

QTEST_GUILESS_MAIN(SqliteStatementCacheTest)

#include "SqliteStatementCacheTest.moc"
