#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include "core/TaskBulkMutationService.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

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

template <typename Result> [[nodiscard]] Result await(std::future<Result>& future) {
  if (future.wait_for(5s) != std::future_status::ready) {
    qFatal("bulk task mutation request timed out");
  }
  return future.get();
}

[[nodiscard]] std::optional<hcb::FilePath> databasePathFor(const QTemporaryDir& directory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(directory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
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
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
          "('list-active', 'account-a', 'active', 'Active', '2026-07-25T00:00:00Z'), "
          "('list-other', 'account-a', 'other', 'Other', '2026-07-25T00:00:00Z')");
}

void verifyReady(hcb::TaskMutationService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

[[nodiscard]] QString create(hcb::TaskMutationService& service,
                             const QString& taskListId,
                             const QString& title,
                             std::optional<QString> parentTaskId = {}) {
  std::future<hcb::TaskMutationResult> future = service.create(
      {.taskListId = taskListId, .parentTaskId = std::move(parentTaskId), .title = title});
  const hcb::TaskMutationResult result = await(future);
  if (!std::holds_alternative<hcb::TaskMutationReceipt>(result)) {
    qFatal("task create failed");
  }
  return std::get<hcb::TaskMutationReceipt>(result).taskId;
}

[[nodiscard]] hcb::TaskBulkMutationSummary execute(hcb::TaskBulkMutationService& service,
                                                    hcb::TaskBulkMutationInput input) {
  std::future<hcb::TaskBulkMutationResult> future = service.execute(std::move(input));
  const hcb::TaskBulkMutationResult result = await(future);
  if (!std::holds_alternative<hcb::TaskBulkMutationSummary>(result)) {
    qFatal("%s", qPrintable(std::get<hcb::AppError>(result).message()));
  }
  return std::get<hcb::TaskBulkMutationSummary>(result);
}

} // namespace

class TaskBulkMutationServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void queuesEligibleItemsAndSeparatesSkips();
  void avoidsOrderDependentHierarchyMoves();
  void handlesLargeSelectionsWithIndependentMutations();
  void reparentsMaximumSelectionWithCompatibleParent();
  void replacesLiteralTaskTextWithPreview();
};

void TaskBulkMutationServiceTest::queuesEligibleItemsAndSeparatesSkips() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(directory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(*databasePath, clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  const QString active = create(taskMutations, QStringLiteral("list-active"), QStringLiteral("Active"));
  const QString completed =
      create(taskMutations, QStringLiteral("list-active"), QStringLiteral("Completed"));
  std::future<hcb::TaskMutationResult> complete = taskMutations.setCompleted(completed, true);
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(await(complete)));

  hcb::TaskBulkMutationService bulk(taskMutations);
  const hcb::TaskBulkMutationSummary summary = execute(
      bulk,
      {.action = hcb::TaskBulkAction::Complete,
       .taskIds = {active, completed, QStringLiteral("task-missing")}});
  QCOMPARE(summary.requested, 3);
  QCOMPARE(summary.eligible, 1);
  QCOMPARE(summary.queued, 1);
  QCOMPARE(summary.applied, 0);
  QCOMPARE(summary.conflicted, 0);
  QCOMPARE(summary.failed, 0);
  QCOMPARE(summary.skipped, 2);
  QCOMPARE(summary.items.size(), 3);

  std::future<hcb::TaskMutationSnapshotResult> inspected =
      taskMutations.inspect({active, completed});
  const hcb::TaskMutationSnapshotResult snapshots = await(inspected);
  QVERIFY(std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(snapshots));
  if (!std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(snapshots)) {
    return;
  }
  const QList<hcb::TaskMutationSnapshot>& rows =
      std::get<QList<hcb::TaskMutationSnapshot>>(snapshots);
  QCOMPARE(rows.size(), 2);
  QVERIFY(rows.at(0).completed);
  QVERIFY(rows.at(1).completed);
}

void TaskBulkMutationServiceTest::avoidsOrderDependentHierarchyMoves() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(directory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(*databasePath, clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  const QString parent = create(taskMutations, QStringLiteral("list-active"), QStringLiteral("Parent"));
  const QString child = create(taskMutations,
                               QStringLiteral("list-active"),
                               QStringLiteral("Child"),
                               parent);

  hcb::TaskBulkMutationService bulk(taskMutations);
  const hcb::TaskBulkMutationSummary summary = execute(
      bulk,
      {.action = hcb::TaskBulkAction::MoveToList,
       .taskIds = {parent, child},
       .taskListId = QStringLiteral("list-other")});
  QCOMPARE(summary.eligible, 1);
  QCOMPARE(summary.queued, 1);
  QCOMPARE(summary.skipped, 1);

  std::future<hcb::TaskMutationSnapshotResult> inspected =
      taskMutations.inspect({parent, child});
  const hcb::TaskMutationSnapshotResult snapshots = await(inspected);
  QVERIFY(std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(snapshots));
  if (!std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(snapshots)) {
    return;
  }
  const QList<hcb::TaskMutationSnapshot>& rows =
      std::get<QList<hcb::TaskMutationSnapshot>>(snapshots);
  QCOMPARE(rows.at(0).taskListId, QStringLiteral("list-active"));
  QCOMPARE(rows.at(1).taskListId, QStringLiteral("list-other"));
}

void TaskBulkMutationServiceTest::handlesLargeSelectionsWithIndependentMutations() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(directory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(*databasePath, clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  QList<QString> taskIds;
  constexpr int taskCount = 96;
  taskIds.reserve(taskCount);
  for (int index = 0; index < taskCount; ++index) {
    taskIds.append(create(taskMutations,
                          QStringLiteral("list-active"),
                          QStringLiteral("Task %1").arg(index)));
  }

  hcb::TaskBulkMutationService bulk(taskMutations);
  const hcb::TaskBulkMutationSummary summary = execute(
      bulk,
      {.action = hcb::TaskBulkAction::SetPriority,
       .taskIds = taskIds,
       .priority = hcb::TaskPriority::High});
  QCOMPARE(summary.requested, taskCount);
  QCOMPARE(summary.eligible, taskCount);
  QCOMPARE(summary.queued, taskCount);
  QCOMPARE(summary.failed, 0);
  QCOMPARE(summary.skipped, 0);

  std::future<hcb::TaskMutationSnapshotResult> inspected = taskMutations.inspect(taskIds);
  const hcb::TaskMutationSnapshotResult snapshots = await(inspected);
  QVERIFY(std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(snapshots));
  if (!std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(snapshots)) {
    return;
  }
  const QList<hcb::TaskMutationSnapshot>& rows =
      std::get<QList<hcb::TaskMutationSnapshot>>(snapshots);
  QCOMPARE(rows.size(), taskCount);
  for (const hcb::TaskMutationSnapshot& task : rows) {
    QCOMPARE(task.priority, hcb::TaskPriority::High);
  }
}

void TaskBulkMutationServiceTest::reparentsMaximumSelectionWithCompatibleParent() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(directory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(*databasePath, clock);
  verifyReady(taskMutations);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  const QString parent = create(taskMutations, QStringLiteral("list-active"), QStringLiteral("Parent"));
  QList<QString> taskIds;
  constexpr int taskCount = 500;
  taskIds.reserve(taskCount);
  for (int index = 0; index < taskCount; ++index) {
    taskIds.append(create(taskMutations,
                          QStringLiteral("list-active"),
                          QStringLiteral("Task %1").arg(index)));
  }

  hcb::TaskBulkMutationService bulk(taskMutations);
  const hcb::TaskBulkMutationSummary summary = execute(
      bulk,
      {.action = hcb::TaskBulkAction::Reparent,
       .taskIds = taskIds,
       .parentTaskId = parent});
  QCOMPARE(summary.requested, taskCount);
  QCOMPARE(summary.eligible, taskCount);
  QCOMPARE(summary.queued, taskCount);
  QCOMPARE(summary.failed, 0);
  QCOMPARE(summary.skipped, 0);
}

void TaskBulkMutationServiceTest::replacesLiteralTaskTextWithPreview() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(directory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  FixedClock clock;
  hcb::TaskMutationService taskMutations(*databasePath, clock);
  verifyReady(taskMutations);
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
  const QString matching = create(taskMutations, QStringLiteral("list-active"), QStringLiteral("Alpha task"));
  const QString unmatched = create(taskMutations, QStringLiteral("list-active"), QStringLiteral("Other task"));
  std::future<hcb::TaskMutationResult> setNotes = taskMutations.update(
      {.taskId = matching, .notes = QStringLiteral("Alpha notes")});
  QVERIFY(std::holds_alternative<hcb::TaskMutationReceipt>(await(setNotes)));

  hcb::TaskBulkMutationService bulk(taskMutations);
  const hcb::TaskBulkMutationSummary preview = execute(
      bulk,
      {.action = hcb::TaskBulkAction::ReplaceText,
       .taskIds = {matching, unmatched},
       .findText = QStringLiteral("Alpha"),
       .replaceText = QStringLiteral("Beta"),
       .textFields = static_cast<std::uint8_t>(hcb::TaskBulkTextField::Title) |
                     static_cast<std::uint8_t>(hcb::TaskBulkTextField::Notes),
       .recurrenceScope = 0,
       .previewOnly = true});
  QCOMPARE(preview.eligible, 1);
  QCOMPARE(preview.queued, 0);
  QCOMPARE(preview.skipped, 1);

  const hcb::TaskBulkMutationSummary replaced = execute(
      bulk,
      {.action = hcb::TaskBulkAction::ReplaceText,
       .taskIds = {matching, unmatched},
       .findText = QStringLiteral("Alpha"),
       .replaceText = QStringLiteral("Beta"),
       .textFields = static_cast<std::uint8_t>(hcb::TaskBulkTextField::Title) |
                     static_cast<std::uint8_t>(hcb::TaskBulkTextField::Notes),
       .recurrenceScope = 0});
  QCOMPARE(replaced.queued, 1);
  QCOMPARE(replaced.skipped, 1);
  std::future<hcb::TaskMutationSnapshotResult> inspected = taskMutations.inspect({matching});
  const hcb::TaskMutationSnapshotResult snapshots = await(inspected);
  QVERIFY(std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(snapshots));
  if (!std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(snapshots)) {
    return;
  }
  const hcb::TaskMutationSnapshot& task =
      std::get<QList<hcb::TaskMutationSnapshot>>(snapshots).constFirst();
  QCOMPARE(task.title, QStringLiteral("Beta task"));
  QCOMPARE(task.notes, std::optional<QString>(QStringLiteral("Beta notes")));

  const auto createManaged = [&taskMutations, handle](const QString& title,
                                                       const QString& seriesId,
                                                       std::int32_t ordinal) {
    std::future<hcb::TaskMutationResult> future = taskMutations.create(
        {.taskListId = QStringLiteral("list-active"),
         .title = title,
         .notes = QStringLiteral("Alpha recurrence"),
         .due = hcb::TaskDue{.at = QStringLiteral("2026-08-01T00:00:00.000Z"),
                              .timeZone = QStringLiteral("UTC")}});
    const hcb::TaskMutationResult result = await(future);
    if (!std::holds_alternative<hcb::TaskMutationReceipt>(result)) {
      qFatal("managed task create failed");
    }
    const QString taskId = std::get<hcb::TaskMutationReceipt>(result).taskId;
    hcb::TaskRecurrenceMarker marker{.seriesId = seriesId,
                                     .occurrenceId = seriesId + QStringLiteral(":%1").arg(ordinal),
                                     .ordinal = ordinal,
                                     .anchorDate = QStringLiteral("2026-08-01"),
                                     .timeZone = QStringLiteral("UTC"),
                                     .templateTitle = title,
                                     .templateDueDate = QStringLiteral("2026-08-01"),
                                     .templatePriority = QStringLiteral("none")};
    const hcb::TaskRecurrenceSerializationResult serialized =
        hcb::serializeTaskRecurrenceNotes(QStringLiteral("Alpha recurrence"), marker);
    if (serialized.error.has_value()) {
      qFatal("managed recurrence serialization failed");
    }
    QByteArray escapedNotes = serialized.notes.toUtf8();
    escapedNotes.replace("'", "''");
    const QByteArray sql = "UPDATE local_tasks SET notes = '" + escapedNotes + "' WHERE id = '" +
                           taskId.toUtf8() + "'";
    execute(handle, sql.constData());
    return taskId;
  };
  const QString seriesId = QStringLiteral("bf3c1fea-ae9d-4ed6-85e6-3de20b790002");
  const QString recurrenceCurrent = createManaged(QStringLiteral("Alpha current"), seriesId, 0);
  const QString recurrenceFuture = createManaged(QStringLiteral("Alpha future"), seriesId, 1);
  std::future<hcb::TaskMutationSnapshotResult> managedInspection =
      taskMutations.inspectManagedSeries({recurrenceCurrent});
  const hcb::TaskMutationSnapshotResult managedSnapshots = await(managedInspection);
  QVERIFY(std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(managedSnapshots));
  if (!std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(managedSnapshots)) {
    return;
  }
  QCOMPARE(std::get<QList<hcb::TaskMutationSnapshot>>(managedSnapshots).size(), 2);
  const hcb::TaskBulkMutationSummary fullSeries = execute(
      bulk,
      {.action = hcb::TaskBulkAction::ReplaceText,
       .taskIds = {recurrenceCurrent},
       .findText = QStringLiteral("Alpha"),
       .replaceText = QStringLiteral("Beta"),
       .textFields = static_cast<std::uint8_t>(hcb::TaskBulkTextField::Title) |
                     static_cast<std::uint8_t>(hcb::TaskBulkTextField::Notes),
       .recurrenceScope = 3});
  QCOMPARE(fullSeries.items.size(), 2);
  QCOMPARE(fullSeries.eligible, 2);
  QCOMPARE(fullSeries.queued, 2);
  std::future<hcb::TaskMutationSnapshotResult> recurringInspection =
      taskMutations.inspect({recurrenceCurrent, recurrenceFuture});
  const hcb::TaskMutationSnapshotResult recurringSnapshots = await(recurringInspection);
  QVERIFY(std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(recurringSnapshots));
  if (!std::holds_alternative<QList<hcb::TaskMutationSnapshot>>(recurringSnapshots)) {
    return;
  }
  const QList<hcb::TaskMutationSnapshot>& recurringRows =
      std::get<QList<hcb::TaskMutationSnapshot>>(recurringSnapshots);
  QCOMPARE(recurringRows.at(0).title, QStringLiteral("Beta current"));
  QCOMPARE(recurringRows.at(1).title, QStringLiteral("Beta future"));
}

QTEST_GUILESS_MAIN(TaskBulkMutationServiceTest)

#include "TaskBulkMutationServiceTest.moc"
