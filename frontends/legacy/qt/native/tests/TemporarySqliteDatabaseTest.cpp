#include <QFileInfo>
#include <QtTest/QTest>

#include <memory>
#include <utility>
#include <variant>

#include "sqlite3.h"
#include "support/TemporarySqliteDatabase.h"

class TemporarySqliteDatabaseTest final : public QObject {
  Q_OBJECT

private slots:
  void createsDatabaseInTemporaryRootAndCleansItUp();
  void opensReadOnlyAfterWriterCloses();
};

void TemporarySqliteDatabaseTest::createsDatabaseInTemporaryRootAndCleansItUp() {
  hcb::test::TemporarySqliteDatabaseResult databaseResult =
      hcb::test::TemporarySqliteDatabase::create();
  QVERIFY(
      std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(databaseResult));
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(
          databaseResult)) {
    return;
  }
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database =
      std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(databaseResult));
  const QString rootPath = database->rootPath();
  QVERIFY(QFileInfo::exists(rootPath));
  QVERIFY(database->databasePath().nativePath().startsWith(rootPath));

  {
    hcb::SqliteConnectionResult connectionResult =
        database->open(hcb::SqliteOpenMode::ReadWriteCreate);
    QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
    if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
      return;
    }
    hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
    QCOMPARE(sqlite3_exec(connection.nativeHandle(),
                          "CREATE TABLE entries (id INTEGER PRIMARY KEY)",
                          nullptr,
                          nullptr,
                          nullptr),
             SQLITE_OK);
    QVERIFY(QFileInfo::exists(database->databasePath().nativePath()));
  }

  database.reset();
  QVERIFY(!QFileInfo::exists(rootPath));
}

void TemporarySqliteDatabaseTest::opensReadOnlyAfterWriterCloses() {
  hcb::test::TemporarySqliteDatabaseResult databaseResult =
      hcb::test::TemporarySqliteDatabase::create();
  QVERIFY(
      std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(databaseResult));
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(
          databaseResult)) {
    return;
  }
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database =
      std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(databaseResult));

  {
    hcb::SqliteConnectionResult writerResult = database->open(hcb::SqliteOpenMode::ReadWriteCreate);
    QVERIFY(std::holds_alternative<hcb::SqliteConnection>(writerResult));
  }
  hcb::SqliteConnectionResult readerResult = database->open(hcb::SqliteOpenMode::ReadOnly);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(readerResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(readerResult)) {
    return;
  }
  hcb::SqliteConnection reader = std::move(std::get<hcb::SqliteConnection>(readerResult));
  QCOMPARE(sqlite3_db_readonly(reader.nativeHandle(), "main"), 1);
}

QTEST_GUILESS_MAIN(TemporarySqliteDatabaseTest)

#include "TemporarySqliteDatabaseTest.moc"
