#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <variant>

#include "core/ImportMutationService.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

using namespace std::chrono_literals;

namespace {

class FixedClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override {
    return hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}};
  }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }
};

[[nodiscard]] std::optional<hcb::FilePath> databasePathFor(const QTemporaryDir& directory) {
  return hcb::FilePath::fromAbsolute(
      QDir(QFileInfo(directory.path()).canonicalFilePath())
          .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

void execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  const QString message = errorMessage == nullptr ? QString() : QString::fromUtf8(errorMessage);
  sqlite3_free(errorMessage);
  QVERIFY2(result == SQLITE_OK, qPrintable(message));
}

[[nodiscard]] std::int64_t count(sqlite3* handle, const char* table) {
  const QString sql = QStringLiteral("SELECT COUNT(*) FROM %1").arg(QString::fromUtf8(table));
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v2(handle, sql.toUtf8().constData(), -1, &statement, nullptr) != SQLITE_OK) {
    return -1;
  }
  const std::int64_t result =
      sqlite3_step(statement) == SQLITE_ROW ? sqlite3_column_int64(statement, 0) : -1;
  sqlite3_finalize(statement);
  return result;
}

template <typename Result> [[nodiscard]] Result await(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("import mutation timed out");
  }
  return future.get();
}

} // namespace

class ImportMutationServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void createsTasksAndEventsInOneTransaction();
};

void ImportMutationServiceTest::createsTasksAndEventsInOneTransaction() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const std::optional<hcb::FilePath> path = databasePathFor(directory);
  QVERIFY(path.has_value());
  if (!path.has_value()) return;
  FixedClock clock;
  hcb::ImportMutationService service(*path, clock);
  QCOMPARE(service.ready().wait_for(2s), std::future_status::ready);
  QVERIFY(!service.ready().get().has_value());
  hcb::SqliteConnectionResult opened =
      hcb::SqliteConnectionFactory::open(*path, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(opened));
  if (!std::holds_alternative<hcb::SqliteConnection>(opened)) return;
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(opened));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-a', 'account-a', 'remote-list', 'Inbox', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_calendars (id, account_id, remote_id, title, access_role, updated_at) "
          "VALUES ('calendar-a', 'account-a', 'remote-calendar', 'Work', 'owner', "
          "'2026-07-25T00:00:00Z')");

  std::future<hcb::ImportMutationResult> rejected = service.create(
      {{.taskListId = QStringLiteral("list-a"), .title = QStringLiteral("Task")}},
      {{.calendarId = QStringLiteral("missing-calendar"),
        .title = QStringLiteral("Event"),
        .startAt = QStringLiteral("2026-07-29T01:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-29T02:00:00.000Z")}});
  QVERIFY(std::holds_alternative<hcb::AppError>(await(rejected)));
  QCOMPARE(count(handle, "local_tasks"), std::int64_t{0});
  QCOMPARE(count(handle, "local_calendar_events"), std::int64_t{0});
  QCOMPARE(count(handle, "local_pending_mutations"), std::int64_t{0});

  std::future<hcb::ImportMutationResult> created = service.create(
      {{.taskListId = QStringLiteral("list-a"), .title = QStringLiteral("Task")}},
      {{.calendarId = QStringLiteral("calendar-a"),
        .title = QStringLiteral("Event"),
        .startAt = QStringLiteral("2026-07-29T01:00:00.000Z"),
        .endAt = QStringLiteral("2026-07-29T02:00:00.000Z")}});
  const hcb::ImportMutationResult result = await(created);
  QVERIFY(std::holds_alternative<hcb::ImportMutationReceipt>(result));
  if (!std::holds_alternative<hcb::ImportMutationReceipt>(result)) return;
  QCOMPARE(std::get<hcb::ImportMutationReceipt>(result).taskCount, qsizetype{1});
  QCOMPARE(std::get<hcb::ImportMutationReceipt>(result).eventCount, qsizetype{1});
  QCOMPARE(count(handle, "local_tasks"), std::int64_t{1});
  QCOMPARE(count(handle, "local_calendar_events"), std::int64_t{1});
  QCOMPARE(count(handle, "local_pending_mutations"), std::int64_t{2});
}

QTEST_GUILESS_MAIN(ImportMutationServiceTest)

#include "ImportMutationServiceTest.moc"
