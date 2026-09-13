#include <QDir>
#include <QFileInfo>
#include <QJsonObject>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <chrono>
#include <cstdint>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/CalendarMutationService.h"
#include "core/OptimisticMutationCoordinator.h"
#include "core/TaskMutationService.h"
#include "core/UndoRecoveryPolicy.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

using namespace std::chrono_literals;

namespace {

class FixedClock final : public hcb::Clock {
public:
  explicit FixedClock(hcb::WallTimePoint wallTime) : wallTime_(wallTime) {}

  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

private:
  hcb::WallTimePoint wallTime_;
};

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("domain service contract request timed out");
  }
  return future.get();
}

void execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  const QString message = errorMessage == nullptr ? QString() : QString::fromUtf8(errorMessage);
  sqlite3_free(errorMessage);
  QVERIFY2(result == SQLITE_OK, qPrintable(message));
}

void verifyReady(const std::shared_future<hcb::SqliteWriteResult>& ready) {
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

void seed(hcb::SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-a', 'account-a', 'inbox', 'Inbox', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_calendars (id, account_id, remote_id, title, updated_at) VALUES "
          "('calendar-a', 'account-a', 'primary', 'Primary', '2026-07-25T00:00:00Z')");
}

[[nodiscard]] std::int64_t count(sqlite3* handle, const char* sql) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    sqlite3_finalize(statement);
    return -1;
  }
  const int stepResult = sqlite3_step(statement);
  const std::int64_t value = sqlite3_column_int64(statement, 0);
  const int finalizeResult = sqlite3_finalize(statement);
  return stepResult == SQLITE_ROW && finalizeResult == SQLITE_OK ? value : -1;
}

} // namespace

class DomainServiceContractTest final : public QObject {
  Q_OBJECT

private slots:
  void sharesOneInitializedDatabaseAcrossPublicServices();
};

void DomainServiceContractTest::sharesOneInitializedDatabaseAcrossPublicServices() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::TaskMutationService tasks(*databasePath, clock);
  verifyReady(tasks.ready());
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);

  hcb::CalendarMutationService calendar(*databasePath, clock);
  hcb::OptimisticMutationCoordinator coordinator(*databasePath, clock);
  hcb::UndoRecoveryPolicy undo(*databasePath, clock, QStringLiteral("contract-session"));
  verifyReady(calendar.ready());
  verifyReady(coordinator.ready());
  verifyReady(undo.ready());

  std::future<hcb::TaskMutationResult> createTask = tasks.create(hcb::TaskCreateInput{
      .taskListId = QStringLiteral("list-a"), .title = QStringLiteral("Contract task")});
  const hcb::TaskMutationResult taskResult = awaitResult(createTask);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(taskResult));

  std::future<hcb::CalendarEventMutationResult> createEvent = calendar.create(
      hcb::CalendarEventCreateInput{.calendarId = QStringLiteral("calendar-a"),
                                    .title = QStringLiteral("Contract event"),
                                    .startAt = QStringLiteral("2026-07-26T09:00:00.000Z"),
                                    .endAt = QStringLiteral("2026-07-26T10:00:00.000Z")});
  const hcb::CalendarEventMutationResult eventResult = awaitResult(createEvent);
  QVERIFY(std::holds_alternative<hcb::CalendarEventMutationReceipt>(eventResult));

  std::future<hcb::PendingMutationResult> enqueue =
      coordinator.enqueue(hcb::OptimisticMutationInput{
          .accountId = QStringLiteral("account-a"),
          .resource = hcb::PendingMutationResource::Task,
          .resourceId = QStringLiteral("contract-resource"),
          .operation = QStringLiteral("task.update"),
          .payload = QJsonObject{{QStringLiteral("title"), QStringLiteral("Contract task")}}});
  const hcb::PendingMutationResult pendingResult = awaitResult(enqueue);
  QVERIFY(std::holds_alternative<hcb::PendingMutation>(pendingResult));

  std::future<std::optional<hcb::AppError>> record = undo.record(hcb::UndoChangeInput{
      .actionKind = QStringLiteral("task.update"),
      .label = QStringLiteral("Contract edit"),
      .resource = hcb::UndoResourceKind::Task,
      .resourceId = QStringLiteral("contract-resource"),
      .before = QJsonObject{{QStringLiteral("title"), QStringLiteral("Before")}},
      .after = QJsonObject{{QStringLiteral("title"), QStringLiteral("After")}}});
  const std::optional<hcb::AppError> recordResult = awaitResult(record);
  QVERIFY2(!recordResult.has_value(),
           qPrintable(recordResult.has_value() ? recordResult->message() : QString()));

  std::future<hcb::UndoStatusResult> status = undo.status();
  const hcb::UndoStatusResult statusResult = awaitResult(status);
  QVERIFY(std::holds_alternative<hcb::UndoStatus>(statusResult));
  if (std::holds_alternative<hcb::UndoStatus>(statusResult)) {
    QCOMPARE(std::get<hcb::UndoStatus>(statusResult).undoLabel,
             std::optional<QString>(QStringLiteral("Contract edit")));
  }

  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_tasks WHERE deleted_at IS NULL"), 1);
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendar_events WHERE deleted_at IS NULL"), 1);
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_pending_mutations"), 3);
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_undo_entries"), 1);
}

QTEST_GUILESS_MAIN(DomainServiceContractTest)

#include "DomainServiceContractTest.moc"
