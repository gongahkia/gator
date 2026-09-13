#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <optional>
#include <utility>
#include <variant>

#include "data/SqliteConnection.h"
#include "sqlite3.h"

class SqliteConnectionTest final : public QObject {
  Q_OBJECT

private slots:
  void appliesProductionPragmas();
  void opensReadWriteConnectionAndCreatesDatabase();
  void opensExistingDatabaseReadOnly();
  void reportsDatabaseErrorForMissingReadOnlyFile();
  void rejectsSymbolicLink();
};

namespace {

void verifyProductionPragmas(sqlite3* handle) {
  sqlite3_stmt* statement = nullptr;
  QCOMPARE(sqlite3_prepare_v2(handle, "PRAGMA foreign_keys", -1, &statement, nullptr), SQLITE_OK);
  QCOMPARE(sqlite3_step(statement), SQLITE_ROW);
  QCOMPARE(sqlite3_column_int64(statement, 0), 1);
  QCOMPARE(sqlite3_finalize(statement), SQLITE_OK);

  QCOMPARE(sqlite3_prepare_v2(handle, "PRAGMA journal_mode", -1, &statement, nullptr), SQLITE_OK);
  QCOMPARE(sqlite3_step(statement), SQLITE_ROW);
  QCOMPARE(QString::fromUtf8(reinterpret_cast<const char*>(sqlite3_column_text(statement, 0))),
           QStringLiteral("wal"));
  QCOMPARE(sqlite3_finalize(statement), SQLITE_OK);

  QCOMPARE(sqlite3_prepare_v2(handle, "PRAGMA synchronous", -1, &statement, nullptr), SQLITE_OK);
  QCOMPARE(sqlite3_step(statement), SQLITE_ROW);
  QCOMPARE(sqlite3_column_int64(statement, 0), 2);
  QCOMPARE(sqlite3_finalize(statement), SQLITE_OK);

  QCOMPARE(sqlite3_prepare_v2(handle, "PRAGMA temp_store", -1, &statement, nullptr), SQLITE_OK);
  QCOMPARE(sqlite3_step(statement), SQLITE_ROW);
  QCOMPARE(sqlite3_column_int64(statement, 0), 2);
  QCOMPARE(sqlite3_finalize(statement), SQLITE_OK);

  QCOMPARE(sqlite3_prepare_v2(handle, "PRAGMA cache_size", -1, &statement, nullptr), SQLITE_OK);
  QCOMPARE(sqlite3_step(statement), SQLITE_ROW);
  QCOMPARE(sqlite3_column_int64(statement, 0), -65'536);
  QCOMPARE(sqlite3_finalize(statement), SQLITE_OK);

  QCOMPARE(sqlite3_prepare_v2(handle, "PRAGMA mmap_size", -1, &statement, nullptr), SQLITE_OK);
  QCOMPARE(sqlite3_step(statement), SQLITE_ROW);
  QCOMPARE(sqlite3_column_int64(statement, 0), 268'435'456);
  QCOMPARE(sqlite3_finalize(statement), SQLITE_OK);

  QCOMPARE(sqlite3_prepare_v2(handle, "PRAGMA busy_timeout", -1, &statement, nullptr), SQLITE_OK);
  QCOMPARE(sqlite3_step(statement), SQLITE_ROW);
  QCOMPARE(sqlite3_column_int64(statement, 0), 30'000);
  QCOMPARE(sqlite3_finalize(statement), SQLITE_OK);
}

} // namespace

void SqliteConnectionTest::appliesProductionPragmas() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath =
      hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                      .filePath(QStringLiteral("hot-cross-buns.sqlite")));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  {
    hcb::SqliteConnectionResult writable =
        hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
    QVERIFY(std::holds_alternative<hcb::SqliteConnection>(writable));
    if (!std::holds_alternative<hcb::SqliteConnection>(writable)) {
      return;
    }
    hcb::SqliteConnection writeConnection = std::move(std::get<hcb::SqliteConnection>(writable));
    verifyProductionPragmas(writeConnection.nativeHandle());
  }

  hcb::SqliteConnectionResult readOnly =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadOnly);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(readOnly));
  if (!std::holds_alternative<hcb::SqliteConnection>(readOnly)) {
    return;
  }
  hcb::SqliteConnection readConnection = std::move(std::get<hcb::SqliteConnection>(readOnly));
  verifyProductionPragmas(readConnection.nativeHandle());
}

void SqliteConnectionTest::opensReadWriteConnectionAndCreatesDatabase() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath =
      hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                      .filePath(QStringLiteral("hot-cross-buns.sqlite")));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  hcb::SqliteConnectionResult result =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(result));
  if (!std::holds_alternative<hcb::SqliteConnection>(result)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(result));
  QVERIFY(connection.nativeHandle() != nullptr);
  QCOMPARE(sqlite3_db_readonly(connection.nativeHandle(), "main"), 0);
  QCOMPARE(sqlite3_exec(connection.nativeHandle(),
                        "CREATE TABLE task (id INTEGER PRIMARY KEY)",
                        nullptr,
                        nullptr,
                        nullptr),
           SQLITE_OK);
  QVERIFY(QFileInfo::exists(databasePath->nativePath()));
}

void SqliteConnectionTest::opensExistingDatabaseReadOnly() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath =
      hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                      .filePath(QStringLiteral("hot-cross-buns.sqlite")));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  {
    hcb::SqliteConnectionResult writable =
        hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
    QVERIFY(std::holds_alternative<hcb::SqliteConnection>(writable));
  }

  hcb::SqliteConnectionResult readOnly =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadOnly);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(readOnly));
  if (!std::holds_alternative<hcb::SqliteConnection>(readOnly)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(readOnly));
  QCOMPARE(sqlite3_db_readonly(connection.nativeHandle(), "main"), 1);
}

void SqliteConnectionTest::reportsDatabaseErrorForMissingReadOnlyFile() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath =
      hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                      .filePath(QStringLiteral("missing.sqlite")));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  const hcb::SqliteConnectionResult result =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadOnly);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  if (!std::holds_alternative<hcb::AppError>(result)) {
    return;
  }
  const hcb::AppError& error = std::get<hcb::AppError>(result);
  QCOMPARE(error.code(), hcb::AppErrorCode::Database);
  QVERIFY(!QFileInfo::exists(databasePath->nativePath()));
}

void SqliteConnectionTest::rejectsSymbolicLink() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const QString canonicalDirectory = QFileInfo(temporaryDirectory.path()).canonicalFilePath();
  const std::optional<hcb::FilePath> targetPath = hcb::FilePath::fromAbsolute(
      QDir(canonicalDirectory).filePath(QStringLiteral("target.sqlite")));
  const std::optional<hcb::FilePath> linkPath =
      hcb::FilePath::fromAbsolute(QDir(canonicalDirectory).filePath(QStringLiteral("link.sqlite")));
  QVERIFY(targetPath.has_value());
  QVERIFY(linkPath.has_value());
  if (!targetPath.has_value() || !linkPath.has_value()) {
    return;
  }

  {
    hcb::SqliteConnectionResult writable =
        hcb::SqliteConnectionFactory::open(*targetPath, hcb::SqliteOpenMode::ReadWriteCreate);
    QVERIFY(std::holds_alternative<hcb::SqliteConnection>(writable));
  }
  QVERIFY(QFile::link(targetPath->nativePath(), linkPath->nativePath()));

  const hcb::SqliteConnectionResult result =
      hcb::SqliteConnectionFactory::open(*linkPath, hcb::SqliteOpenMode::ReadOnly);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  if (!std::holds_alternative<hcb::AppError>(result)) {
    return;
  }
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Database);
}

QTEST_GUILESS_MAIN(SqliteConnectionTest)

#include "SqliteConnectionTest.moc"
