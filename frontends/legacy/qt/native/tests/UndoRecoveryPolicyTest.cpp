#include <QDir>
#include <QFileInfo>
#include <QJsonObject>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/UndoRecoveryPolicy.h"

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
    qFatal("undo recovery policy request timed out");
  }
  return future.get();
}

void verifyReady(hcb::UndoRecoveryPolicy& policy) {
  const std::shared_future<hcb::SqliteWriteResult> ready = policy.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

void verifyNoError(const std::optional<hcb::AppError>& result) {
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

void verifyValidationError(const hcb::UndoReplayResult& result) {
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  if (std::holds_alternative<hcb::AppError>(result)) {
    QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
  }
}

} // namespace

class UndoRecoveryPolicyTest final : public QObject {
  Q_OBJECT

private slots:
  void recordsUndoesAndRedoesSnapshots();
  void guardsConflictsAndRecoversCurrentSession();
};

void UndoRecoveryPolicyTest::recordsUndoesAndRedoesSnapshots() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::UndoRecoveryPolicy policy(*databasePath, clock, QStringLiteral("session-test"));
  verifyReady(policy);
  QCOMPARE(policy.sessionId(), QStringLiteral("session-test"));

  const QJsonObject before{{QStringLiteral("title"), QStringLiteral("Before")}};
  const QJsonObject after{{QStringLiteral("title"), QStringLiteral("After")}};
  std::future<std::optional<hcb::AppError>> record =
      policy.record(hcb::UndoChangeInput{.actionKind = QStringLiteral("task.update"),
                                         .label = QStringLiteral("Edit task"),
                                         .resource = hcb::UndoResourceKind::Task,
                                         .resourceId = QStringLiteral("task-1"),
                                         .before = before,
                                         .after = after});
  verifyNoError(awaitResult(record));

  std::future<hcb::UndoStatusResult> status = policy.status();
  const hcb::UndoStatusResult statusResult = awaitResult(status);
  QVERIFY(std::holds_alternative<hcb::UndoStatus>(statusResult));
  if (!std::holds_alternative<hcb::UndoStatus>(statusResult)) {
    return;
  }
  const hcb::UndoStatus initialStatus = std::get<hcb::UndoStatus>(statusResult);
  QCOMPARE(initialStatus.undoLabel, std::optional<QString>(QStringLiteral("Edit task")));
  QVERIFY(!initialStatus.redoLabel.has_value());

  std::future<hcb::UndoEntryResult> nextUndo = policy.nextUndo();
  const hcb::UndoEntryResult nextUndoResult = awaitResult(nextUndo);
  QVERIFY(std::holds_alternative<hcb::UndoEntry>(nextUndoResult));
  if (!std::holds_alternative<hcb::UndoEntry>(nextUndoResult)) {
    return;
  }
  const hcb::UndoEntry undoEntry = std::get<hcb::UndoEntry>(nextUndoResult);
  QVERIFY(undoEntry.action == hcb::UndoAction::Undo);
  QCOMPARE(undoEntry.actionKind, QStringLiteral("task.update"));
  QVERIFY(undoEntry.expected == after);
  QVERIFY(undoEntry.target == before);

  std::future<hcb::UndoReplayResult> undo = policy.undo(after);
  const hcb::UndoReplayResult undoResult = awaitResult(undo);
  QVERIFY(std::holds_alternative<hcb::UndoReplay>(undoResult));
  if (!std::holds_alternative<hcb::UndoReplay>(undoResult)) {
    return;
  }
  const hcb::UndoReplay undoReplay = std::get<hcb::UndoReplay>(undoResult);
  QVERIFY(undoReplay.action == hcb::UndoAction::Undo);
  QCOMPARE(undoReplay.label, QStringLiteral("Edit task"));
  QVERIFY(undoReplay.resource == hcb::UndoResourceKind::Task);
  QCOMPARE(undoReplay.resourceId, QStringLiteral("task-1"));
  QVERIFY(undoReplay.target == before);

  std::future<hcb::UndoStatusResult> afterUndoStatusFuture = policy.status();
  const hcb::UndoStatusResult afterUndoStatusResult = awaitResult(afterUndoStatusFuture);
  QVERIFY(std::holds_alternative<hcb::UndoStatus>(afterUndoStatusResult));
  if (!std::holds_alternative<hcb::UndoStatus>(afterUndoStatusResult)) {
    return;
  }
  const hcb::UndoStatus afterUndoStatus = std::get<hcb::UndoStatus>(afterUndoStatusResult);
  QVERIFY(!afterUndoStatus.undoLabel.has_value());
  QCOMPARE(afterUndoStatus.redoLabel, std::optional<QString>(QStringLiteral("Edit task")));

  std::future<hcb::UndoEntryResult> nextRedo = policy.nextRedo();
  const hcb::UndoEntryResult nextRedoResult = awaitResult(nextRedo);
  QVERIFY(std::holds_alternative<hcb::UndoEntry>(nextRedoResult));
  if (!std::holds_alternative<hcb::UndoEntry>(nextRedoResult)) {
    return;
  }
  const hcb::UndoEntry redoEntry = std::get<hcb::UndoEntry>(nextRedoResult);
  QVERIFY(redoEntry.action == hcb::UndoAction::Redo);
  QCOMPARE(redoEntry.actionKind, QStringLiteral("task.update"));
  QVERIFY(redoEntry.expected == before);
  QVERIFY(redoEntry.target == after);

  std::future<hcb::UndoReplayResult> redo = policy.redo(before);
  const hcb::UndoReplayResult redoResult = awaitResult(redo);
  QVERIFY(std::holds_alternative<hcb::UndoReplay>(redoResult));
  if (!std::holds_alternative<hcb::UndoReplay>(redoResult)) {
    return;
  }
  const hcb::UndoReplay redoReplay = std::get<hcb::UndoReplay>(redoResult);
  QVERIFY(redoReplay.action == hcb::UndoAction::Redo);
  QVERIFY(redoReplay.target == after);
}

void UndoRecoveryPolicyTest::guardsConflictsAndRecoversCurrentSession() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::UndoRecoveryPolicy policy(*databasePath, clock, QStringLiteral("session-recovery"));
  verifyReady(policy);
  const QJsonObject before{{QStringLiteral("state"), QStringLiteral("active")}};
  const QJsonObject after{{QStringLiteral("state"), QStringLiteral("completed")}};

  std::future<std::optional<hcb::AppError>> record =
      policy.record(hcb::UndoChangeInput{.actionKind = QStringLiteral("task.complete"),
                                         .label = QStringLiteral("Complete task"),
                                         .resource = hcb::UndoResourceKind::Task,
                                         .resourceId = QStringLiteral("task-2"),
                                         .before = before,
                                         .after = after});
  verifyNoError(awaitResult(record));

  std::future<hcb::UndoReplayResult> conflict =
      policy.undo(QJsonObject{{QStringLiteral("state"), QStringLiteral("edited")}});
  verifyValidationError(awaitResult(conflict));

  hcb::UndoRecoveryPolicy recovered(*databasePath, clock, QStringLiteral("session-recovery"));
  verifyReady(recovered);
  std::future<hcb::UndoRecoveryResult> recover = recovered.recover();
  const hcb::UndoRecoveryResult recoverResult = awaitResult(recover);
  QVERIFY(std::holds_alternative<hcb::UndoRecoveryReport>(recoverResult));
  if (!std::holds_alternative<hcb::UndoRecoveryReport>(recoverResult)) {
    return;
  }
  const hcb::UndoRecoveryReport report = std::get<hcb::UndoRecoveryReport>(recoverResult);
  QCOMPARE(report.discardedEntries, 0);
  QCOMPARE(report.status.undoLabel, std::optional<QString>(QStringLiteral("Complete task")));
  QVERIFY(!report.status.redoLabel.has_value());

  std::future<hcb::UndoReplayResult> undo = recovered.undo(after);
  const hcb::UndoReplayResult undoResult = awaitResult(undo);
  QVERIFY(std::holds_alternative<hcb::UndoReplay>(undoResult));
  if (std::holds_alternative<hcb::UndoReplay>(undoResult)) {
    QVERIFY(std::get<hcb::UndoReplay>(undoResult).target == before);
  }
}

QTEST_GUILESS_MAIN(UndoRecoveryPolicyTest)

#include "UndoRecoveryPolicyTest.moc"
