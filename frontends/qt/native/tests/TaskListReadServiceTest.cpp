#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/TaskListReadService.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

using namespace std::chrono_literals;

class TaskListReadServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void readsOrderedFilteredTaskListPages();
  void rejectsInvalidReadRequests();
};

namespace {

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("task-list read service request timed out");
  }
  return future.get();
}

void verifyReady(hcb::TaskListReadService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
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

void seed(hcb::SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-24T00:00:00Z'), "
          "('account-b', 'google', 'connected', '[]', '[]', '2026-07-24T00:00:00Z')");
  execute(
      handle,
      "INSERT INTO local_task_lists (id, account_id, remote_id, title, sort_order, is_selected, "
      "updated_at, deleted_at) VALUES "
      "('list-inbox', 'account-a', 'inbox', 'Inbox', 0, 1, '2026-07-24T00:00:00Z', NULL), "
      "('list-alpha', 'account-a', 'alpha', 'alpha', 1, 0, '2026-07-24T00:00:00Z', NULL), "
      "('list-zeta', 'account-b', 'zeta', 'Zeta', 1, 1, '2026-07-24T00:00:00Z', NULL), "
      "('list-deleted', 'account-a', 'deleted', 'Deleted', 2, 1, '2026-07-24T00:00:00Z', "
      "'2026-07-24T01:00:00Z')");
  execute(
      handle,
      "INSERT INTO local_tasks (id, task_list_id, remote_id, title, state, is_hidden, updated_at, "
      "deleted_at) VALUES "
      "('task-active', 'list-inbox', 'active', 'Active', 'active', 0, '2026-07-24T00:00:00Z', "
      "NULL), "
      "('task-completed', 'list-inbox', 'completed', 'Completed', 'completed', 0, "
      "'2026-07-24T00:00:00Z', NULL), "
      "('task-hidden', 'list-inbox', 'hidden', 'Hidden', 'active', 1, '2026-07-24T00:00:00Z', "
      "NULL), "
      "('task-deleted', 'list-inbox', 'deleted', 'Deleted', 'active', 0, "
      "'2026-07-24T00:00:00Z', '2026-07-24T01:00:00Z')");
}

} // namespace

void TaskListReadServiceTest::readsOrderedFilteredTaskListPages() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::TaskListReadService service(*databasePath);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);

  std::future<hcb::TaskListPageResult> firstPage =
      service.list(hcb::TaskListReadRequest{.limit = 2});
  const hcb::TaskListPageResult firstResult = awaitResult(firstPage);
  QVERIFY(std::holds_alternative<hcb::TaskListPage>(firstResult));
  const hcb::TaskListPage& page = std::get<hcb::TaskListPage>(firstResult);
  QCOMPARE(page.totalKnown, 3);
  QCOMPARE(page.items.size(), 2);
  QCOMPARE(page.items.at(0).id, QStringLiteral("list-inbox"));
  QCOMPARE(page.items.at(0).taskCount, 3);
  QCOMPARE(page.items.at(0).activeTaskCount, 1);
  QCOMPARE(page.items.at(1).id, QStringLiteral("list-alpha"));
  QCOMPARE(page.nextOffset, std::optional<std::int64_t>(2));

  std::future<hcb::TaskListPageResult> selected = service.list(
      hcb::TaskListReadRequest{.accountId = QStringLiteral("account-a"), .selectedOnly = true});
  const hcb::TaskListPageResult selectedResult = awaitResult(selected);
  QVERIFY(std::holds_alternative<hcb::TaskListPage>(selectedResult));
  const hcb::TaskListPage& selectedPage = std::get<hcb::TaskListPage>(selectedResult);
  QCOMPARE(selectedPage.totalKnown, 1);
  QCOMPARE(selectedPage.items.size(), 1);
  QCOMPARE(selectedPage.items.at(0).id, QStringLiteral("list-inbox"));

  std::future<hcb::TaskListLookupResult> found = service.find(QStringLiteral("list-zeta"));
  const hcb::TaskListLookupResult foundResult = awaitResult(found);
  QVERIFY(std::holds_alternative<std::optional<hcb::TaskListSummary>>(foundResult));
  const std::optional<hcb::TaskListSummary>& foundList =
      std::get<std::optional<hcb::TaskListSummary>>(foundResult);
  QVERIFY(foundList.has_value());
  if (!foundList.has_value()) {
    return;
  }
  QCOMPARE(foundList->accountId, QStringLiteral("account-b"));

  std::future<hcb::TaskListLookupResult> deleted = service.find(QStringLiteral("list-deleted"));
  const hcb::TaskListLookupResult deletedResult = awaitResult(deleted);
  QVERIFY(std::holds_alternative<std::optional<hcb::TaskListSummary>>(deletedResult));
  QVERIFY(!std::get<std::optional<hcb::TaskListSummary>>(deletedResult).has_value());
}

void TaskListReadServiceTest::rejectsInvalidReadRequests() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::TaskListReadService service(*databasePath);
  verifyReady(service);

  std::future<hcb::TaskListPageResult> invalidPage =
      service.list(hcb::TaskListReadRequest{.limit = 101});
  const hcb::TaskListPageResult invalidPageResult = awaitResult(invalidPage);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidPageResult));
  QCOMPARE(std::get<hcb::AppError>(invalidPageResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::TaskListLookupResult> invalidId = service.find(QStringLiteral(" list-inbox"));
  const hcb::TaskListLookupResult invalidIdResult = awaitResult(invalidId);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidIdResult));
  QCOMPARE(std::get<hcb::AppError>(invalidIdResult).code(), hcb::AppErrorCode::Validation);
}

QTEST_GUILESS_MAIN(TaskListReadServiceTest)

#include "TaskListReadServiceTest.moc"
