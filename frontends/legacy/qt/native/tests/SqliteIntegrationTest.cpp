#include <memory>

#include <QtTest/QTest>

#include "sqlite3.h"

class SqliteIntegrationTest final : public QObject {
  Q_OBJECT

private slots:
  void opensPinnedAmalgamation();
};

void SqliteIntegrationTest::opensPinnedAmalgamation() {
  QCOMPARE(sqlite3_libversion_number(), 3053003);
  QCOMPARE(sqlite3_threadsafe(), 1);
  QCOMPARE(
      QString::fromUtf8(sqlite3_sourceid()),
      QStringLiteral(
          "2026-06-26 20:14:12 d4c0e51e4aeb96955b99185ab9cde75c339e2c29c3f3f12428d364a10d782c62"));

  sqlite3* database = nullptr;
  QCOMPARE(
      sqlite3_open_v2(":memory:", &database, SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE, nullptr),
      SQLITE_OK);
  const std::unique_ptr<sqlite3, decltype(&sqlite3_close_v2)> close_database(database,
                                                                             &sqlite3_close_v2);

  sqlite3_stmt* statement = nullptr;
  QCOMPARE(sqlite3_prepare_v2(database, "SELECT sqlite_version()", -1, &statement, nullptr),
           SQLITE_OK);
  const std::unique_ptr<sqlite3_stmt, decltype(&sqlite3_finalize)> finalize_statement(
      statement, &sqlite3_finalize);
  QCOMPARE(sqlite3_step(statement), SQLITE_ROW);
  QCOMPARE(QString::fromUtf8(reinterpret_cast<const char*>(sqlite3_column_text(statement, 0))),
           QStringLiteral("3.53.3"));
}

QTEST_GUILESS_MAIN(SqliteIntegrationTest)

#include "SqliteIntegrationTest.moc"
