#include <QDateTime>
#include <QDir>
#include <QFileInfo>
#include <QJsonObject>
#include <QTemporaryDir>
#include <QTimeZone>
#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/OptimisticMutationCoordinator.h"
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
    qFatal("optimistic mutation coordinator request timed out");
  }
  return future.get();
}

void verifyReady(hcb::OptimisticMutationCoordinator& coordinator) {
  const std::shared_future<hcb::SqliteWriteResult> ready = coordinator.ready();
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

void seedAccount(hcb::SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
}

[[nodiscard]] hcb::PendingMutation awaitMutation(std::future<hcb::PendingMutationResult>& future) {
  const hcb::PendingMutationResult result = awaitResult(future);
  if (!std::holds_alternative<hcb::PendingMutation>(result)) {
    qFatal("optimistic mutation coordinator result was an error");
  }
  return std::get<hcb::PendingMutation>(result);
}

void verifyValidationError(const hcb::PendingMutationResult& result) {
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  if (std::holds_alternative<hcb::AppError>(result)) {
    QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
  }
}

} // namespace

class OptimisticMutationCoordinatorTest final : public QObject {
  Q_OBJECT

private slots:
  void enqueuesClaimsRetriesAndAppliesMutations();
  void persistsBaseSnapshotAndRemoteEtag();
  void rebasesClaimedAndFailedMutations();
  void recoversExpiredLeasesAtStartup();
  void rejectsInvalidAndUnavailableTransitions();
};

void OptimisticMutationCoordinatorTest::enqueuesClaimsRetriesAndAppliesMutations() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const hcb::WallTimePoint fixedTime{std::chrono::milliseconds{1'753'408'000'123}};
  const FixedClock clock(fixedTime);
  hcb::OptimisticMutationCoordinator coordinator(*databasePath, clock);
  verifyReady(coordinator);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seedAccount(connection);

  std::future<hcb::PendingMutationResult> enqueue =
      coordinator.enqueue(hcb::OptimisticMutationInput{
          .accountId = QStringLiteral("account-a"),
          .resource = hcb::PendingMutationResource::Task,
          .resourceId = QStringLiteral("task-1"),
          .operation = QStringLiteral("task.update"),
          .payload = QJsonObject{{QStringLiteral("title"), QStringLiteral("Write tests")}}});
  const hcb::PendingMutation enqueued = awaitMutation(enqueue);
  QVERIFY(enqueued.id.startsWith(QStringLiteral("mutation:task:")));
  QCOMPARE(enqueued.accountId, std::optional<QString>(QStringLiteral("account-a")));
  QVERIFY(enqueued.resource == hcb::PendingMutationResource::Task);
  QCOMPARE(enqueued.resourceId, QStringLiteral("task-1"));
  QCOMPARE(enqueued.operation, QStringLiteral("task.update"));
  QVERIFY(
      (enqueued.payload == QJsonObject{{QStringLiteral("title"), QStringLiteral("Write tests")}}));
  QVERIFY(enqueued.status == hcb::PendingMutationStatus::Pending);
  QCOMPARE(enqueued.attemptCount, 0);

  std::future<hcb::PendingMutationListResult> due = coordinator.listDue();
  const hcb::PendingMutationListResult dueResult = awaitResult(due);
  QVERIFY(std::holds_alternative<QList<hcb::PendingMutation>>(dueResult));
  if (!std::holds_alternative<QList<hcb::PendingMutation>>(dueResult)) {
    return;
  }
  const QList<hcb::PendingMutation> dueMutations = std::get<QList<hcb::PendingMutation>>(dueResult);
  QCOMPARE(dueMutations.size(), 1);
  QCOMPARE(dueMutations.constFirst().id, enqueued.id);

  std::future<hcb::PendingMutationResult> claim = coordinator.claim(enqueued.id, 30s);
  const hcb::PendingMutation leased = awaitMutation(claim);
  QVERIFY(leased.status == hcb::PendingMutationStatus::Applying);
  QVERIFY(leased.leaseId.has_value());
  QCOMPARE(leased.leaseExpiresAt,
           std::optional<QString>(QDateTime::fromMSecsSinceEpoch(1'753'408'030'123, QTimeZone::UTC)
                                      .toString(Qt::ISODateWithMs)));

  std::future<hcb::PendingMutationListResult> noDue = coordinator.listDue();
  const hcb::PendingMutationListResult noDueResult = awaitResult(noDue);
  QVERIFY(std::holds_alternative<QList<hcb::PendingMutation>>(noDueResult));
  if (!std::holds_alternative<QList<hcb::PendingMutation>>(noDueResult)) {
    return;
  }
  QVERIFY(std::get<QList<hcb::PendingMutation>>(noDueResult).isEmpty());

  const QString retryAt =
      QDateTime::fromMSecsSinceEpoch(1'753'408'060'123, QTimeZone::UTC).toString(Qt::ISODateWithMs);
  std::future<hcb::PendingMutationResult> fail = coordinator.markFailed(
      hcb::MutationFailureInput{.mutationId = leased.id,
                                .leaseId = leased.leaseId.value_or(QStringLiteral("lease:missing")),
                                .errorCode = QStringLiteral("network"),
                                .errorMessage = QStringLiteral("offline"),
                                .nextRetryAt = retryAt});
  const hcb::PendingMutation failed = awaitMutation(fail);
  QVERIFY(failed.status == hcb::PendingMutationStatus::Failed);
  QCOMPARE(failed.attemptCount, 1);
  QCOMPARE(failed.nextRetryAt, std::optional<QString>(retryAt));
  QCOMPARE(failed.lastErrorCode, std::optional<QString>(QStringLiteral("network")));
  QVERIFY(!failed.leaseId.has_value());

  std::future<hcb::PendingMutationResult> retry = coordinator.retry(failed.id);
  const hcb::PendingMutation retried = awaitMutation(retry);
  QVERIFY(retried.status == hcb::PendingMutationStatus::Pending);
  QVERIFY(!retried.nextRetryAt.has_value());
  QVERIFY(!retried.lastErrorCode.has_value());

  std::future<hcb::PendingMutationResult> secondClaim = coordinator.claim(retried.id, 30s);
  const hcb::PendingMutation leasedAgain = awaitMutation(secondClaim);
  QVERIFY(leasedAgain.leaseId.has_value());
  QVERIFY(leasedAgain.leaseId != leased.leaseId);

  std::future<hcb::PendingMutationResult> apply = coordinator.markApplied(
      leasedAgain.id, leasedAgain.leaseId.value_or(QStringLiteral("lease:missing")));
  const hcb::PendingMutation applied = awaitMutation(apply);
  QVERIFY(applied.status == hcb::PendingMutationStatus::Applied);
  QVERIFY(applied.appliedAt.has_value());
  QVERIFY(!applied.leaseId.has_value());
  QVERIFY(!applied.lastErrorCode.has_value());

  std::future<hcb::PendingMutationLookupResult> find = coordinator.find(applied.id);
  const hcb::PendingMutationLookupResult findResult = awaitResult(find);
  QVERIFY(std::holds_alternative<std::optional<hcb::PendingMutation>>(findResult));
  if (!std::holds_alternative<std::optional<hcb::PendingMutation>>(findResult)) {
    return;
  }
  const std::optional<hcb::PendingMutation> found =
      std::get<std::optional<hcb::PendingMutation>>(findResult);
  QVERIFY(found.has_value());
  if (found.has_value()) {
    QVERIFY(found->status == hcb::PendingMutationStatus::Applied);
  }
}

void OptimisticMutationCoordinatorTest::persistsBaseSnapshotAndRemoteEtag() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::OptimisticMutationCoordinator coordinator(*databasePath, clock);
  verifyReady(coordinator);
  std::future<hcb::PendingMutationResult> enqueue = coordinator.enqueue(
      {.resource = hcb::PendingMutationResource::Task,
       .resourceId = QStringLiteral("task-1"),
       .operation = QStringLiteral("task.update"),
       .payload = {{QStringLiteral("task"),
                    QJsonObject{{QStringLiteral("title"), QStringLiteral("Local")}}}},
       .baseSnapshot = {{QStringLiteral("title"), QStringLiteral("Base")},
                        {QStringLiteral("notes"), QStringLiteral("Base notes")}},
       .remoteEtag = QStringLiteral("etag-base")});
  const hcb::PendingMutation mutation = awaitMutation(enqueue);
  QCOMPARE(mutation.payload,
           (QJsonObject{{QStringLiteral("task"),
                         QJsonObject{{QStringLiteral("title"), QStringLiteral("Local")}}}}));
  QCOMPARE(mutation.baseSnapshot,
           QJsonObject({{QStringLiteral("title"), QStringLiteral("Base")},
                        {QStringLiteral("notes"), QStringLiteral("Base notes")}}));
  QCOMPARE(mutation.remoteEtag, std::optional<QString>(QStringLiteral("etag-base")));
  std::future<hcb::PendingMutationLookupResult> find = coordinator.find(mutation.id);
  const hcb::PendingMutationLookupResult result = awaitResult(find);
  QVERIFY(std::holds_alternative<std::optional<hcb::PendingMutation>>(result));
  const std::optional<hcb::PendingMutation>& restored =
      std::get<std::optional<hcb::PendingMutation>>(result);
  QVERIFY(restored.has_value());
  if (!restored.has_value()) {
    return;
  }
  QCOMPARE(restored->baseSnapshot, mutation.baseSnapshot);
  QCOMPARE(restored->remoteEtag, mutation.remoteEtag);
}

void OptimisticMutationCoordinatorTest::rebasesClaimedAndFailedMutations() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::OptimisticMutationCoordinator coordinator(*databasePath, clock);
  verifyReady(coordinator);
  std::future<hcb::PendingMutationResult> enqueued = coordinator.enqueue(
      {.resource = hcb::PendingMutationResource::Task,
       .resourceId = QStringLiteral("task-1"),
       .operation = QStringLiteral("task.update"),
       .payload = {{QStringLiteral("task"),
                    QJsonObject{{QStringLiteral("title"), QStringLiteral("Local")}}}},
       .baseSnapshot = {{QStringLiteral("title"), QStringLiteral("Base")}},
       .remoteEtag = QStringLiteral("etag-base")});
  const hcb::PendingMutation pending = awaitMutation(enqueued);
  std::future<hcb::PendingMutationResult> claimed = coordinator.claim(pending.id, 30s);
  const hcb::PendingMutation lease = awaitMutation(claimed);
  QVERIFY(lease.leaseId.has_value());
  std::future<hcb::PendingMutationResult> rebased = coordinator.rebase(
      {.mutationId = lease.id,
       .leaseId = lease.leaseId,
       .payload = {{QStringLiteral("task"),
                    QJsonObject{{QStringLiteral("title"), QStringLiteral("Local")}}}},
       .baseSnapshot = {{QStringLiteral("title"), QStringLiteral("Remote")}},
       .remoteEtag = QStringLiteral("etag-remote")});
  const hcb::PendingMutation rebasedMutation = awaitMutation(rebased);
  QCOMPARE(rebasedMutation.status, hcb::PendingMutationStatus::Pending);
  QVERIFY(!rebasedMutation.leaseId.has_value());
  QCOMPARE(rebasedMutation.baseSnapshot,
           QJsonObject({{QStringLiteral("title"), QStringLiteral("Remote")}}));
  QCOMPARE(rebasedMutation.remoteEtag, std::optional<QString>(QStringLiteral("etag-remote")));

  std::future<hcb::PendingMutationResult> retryLease = coordinator.claim(pending.id, 30s);
  const hcb::PendingMutation secondLease = awaitMutation(retryLease);
  QVERIFY(secondLease.leaseId.has_value());
  std::future<hcb::PendingMutationResult> failed = coordinator.markFailed(
      {.mutationId = secondLease.id,
       .leaseId = *secondLease.leaseId,
       .errorCode = QStringLiteral("conflict"),
       .errorMessage = QStringLiteral("changed"),
       .nextRetryAt = std::nullopt});
  QVERIFY(awaitMutation(failed).status == hcb::PendingMutationStatus::Failed);
  std::future<hcb::PendingMutationResult> rebaseFailed = coordinator.rebase(
      {.mutationId = pending.id,
       .payload = {{QStringLiteral("task"),
                    QJsonObject{{QStringLiteral("title"), QStringLiteral("Reapplied")}}}},
       .baseSnapshot = {{QStringLiteral("title"), QStringLiteral("Remote again")}},
       .remoteEtag = QStringLiteral("etag-remote-2")});
  const hcb::PendingMutation reapplied = awaitMutation(rebaseFailed);
  QCOMPARE(reapplied.status, hcb::PendingMutationStatus::Pending);
  QCOMPARE(reapplied.payload.value(QStringLiteral("task")).toObject().value(QStringLiteral("title")),
           QJsonValue(QStringLiteral("Reapplied")));
}

void OptimisticMutationCoordinatorTest::recoversExpiredLeasesAtStartup() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const hcb::WallTimePoint initialTime{std::chrono::milliseconds{1'753'408'000'123}};
  QString expiredMutationId;
  QString activeMutationId;
  {
    const FixedClock clock(initialTime);
    hcb::OptimisticMutationCoordinator coordinator(*databasePath, clock);
    verifyReady(coordinator);
    std::future<hcb::PendingMutationResult> expired = coordinator.enqueue(
        {.resource = hcb::PendingMutationResource::Task,
         .resourceId = QStringLiteral("task-expired"),
         .operation = QStringLiteral("task.update"),
         .payload = QJsonObject{{QStringLiteral("title"), QStringLiteral("Expired")}}});
    const hcb::PendingMutation expiredClaim = awaitMutation(expired);
    expiredMutationId = expiredClaim.id;
    std::future<hcb::PendingMutationResult> expiredLease =
        coordinator.claim(expiredMutationId, 30s);
    QVERIFY(awaitMutation(expiredLease).leaseId.has_value());
    std::future<hcb::PendingMutationResult> active = coordinator.enqueue(
        {.resource = hcb::PendingMutationResource::Task,
         .resourceId = QStringLiteral("task-active"),
         .operation = QStringLiteral("task.update"),
         .payload = QJsonObject{{QStringLiteral("title"), QStringLiteral("Active")}}});
    const hcb::PendingMutation activeClaim = awaitMutation(active);
    activeMutationId = activeClaim.id;
    std::future<hcb::PendingMutationResult> activeLease = coordinator.claim(activeMutationId, 1h);
    QVERIFY(awaitMutation(activeLease).leaseId.has_value());
  }
  {
    const FixedClock clock(initialTime + 31s);
    hcb::OptimisticMutationCoordinator coordinator(*databasePath, clock);
    verifyReady(coordinator);
    std::future<hcb::PendingMutationLookupResult> expired = coordinator.find(expiredMutationId);
    const hcb::PendingMutationLookupResult expiredResult = awaitResult(expired);
    QVERIFY(std::holds_alternative<std::optional<hcb::PendingMutation>>(expiredResult));
    if (!std::holds_alternative<std::optional<hcb::PendingMutation>>(expiredResult)) {
      return;
    }
    const std::optional<hcb::PendingMutation> recovered =
        std::get<std::optional<hcb::PendingMutation>>(expiredResult);
    QVERIFY(recovered.has_value());
    if (!recovered.has_value()) {
      return;
    }
    QCOMPARE(recovered->status, hcb::PendingMutationStatus::Pending);
    QVERIFY(!recovered->leaseId.has_value());
    QVERIFY(!recovered->leaseExpiresAt.has_value());
    std::future<hcb::PendingMutationLookupResult> active = coordinator.find(activeMutationId);
    const hcb::PendingMutationLookupResult activeResult = awaitResult(active);
    QVERIFY(std::holds_alternative<std::optional<hcb::PendingMutation>>(activeResult));
    if (!std::holds_alternative<std::optional<hcb::PendingMutation>>(activeResult)) {
      return;
    }
    const std::optional<hcb::PendingMutation> activeMutation =
        std::get<std::optional<hcb::PendingMutation>>(activeResult);
    QVERIFY(activeMutation.has_value());
    if (!activeMutation.has_value()) {
      return;
    }
    QCOMPARE(activeMutation->status, hcb::PendingMutationStatus::Applying);
    QVERIFY(activeMutation->leaseId.has_value());
    std::future<hcb::PendingMutationListResult> due = coordinator.listDue();
    const hcb::PendingMutationListResult dueResult = awaitResult(due);
    QVERIFY(std::holds_alternative<QList<hcb::PendingMutation>>(dueResult));
    if (!std::holds_alternative<QList<hcb::PendingMutation>>(dueResult)) {
      return;
    }
    const QList<hcb::PendingMutation> dueMutations =
        std::get<QList<hcb::PendingMutation>>(dueResult);
    QCOMPARE(dueMutations.size(), 1);
    QCOMPARE(dueMutations.constFirst().id, expiredMutationId);
  }
}

void OptimisticMutationCoordinatorTest::rejectsInvalidAndUnavailableTransitions() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  const FixedClock clock(hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}});
  hcb::OptimisticMutationCoordinator coordinator(*databasePath, clock);
  verifyReady(coordinator);

  std::future<hcb::PendingMutationResult> invalid =
      coordinator.enqueue(hcb::OptimisticMutationInput{.resourceId = QStringLiteral("task-1"),
                                                       .operation = QStringLiteral(" invalid")});
  verifyValidationError(awaitResult(invalid));

  std::future<hcb::PendingMutationResult> zeroLease =
      coordinator.claim(QStringLiteral("mutation:missing"), 0s);
  verifyValidationError(awaitResult(zeroLease));

  std::future<hcb::PendingMutationResult> enqueue = coordinator.enqueue(
      hcb::OptimisticMutationInput{.resource = hcb::PendingMutationResource::Event,
                                   .resourceId = QStringLiteral("event-1"),
                                   .operation = QStringLiteral("event.delete")});
  const hcb::PendingMutation pending = awaitMutation(enqueue);

  std::future<hcb::PendingMutationResult> claim = coordinator.claim(pending.id, 30s);
  const hcb::PendingMutation leased = awaitMutation(claim);
  QVERIFY(leased.leaseId.has_value());

  std::future<hcb::PendingMutationResult> wrongLease =
      coordinator.markApplied(leased.id, QStringLiteral("lease:wrong"));
  verifyValidationError(awaitResult(wrongLease));

  std::future<hcb::PendingMutationResult> cancel = coordinator.cancel(leased.id);
  const hcb::PendingMutation cancelled = awaitMutation(cancel);
  QVERIFY(cancelled.status == hcb::PendingMutationStatus::Cancelled);
  QVERIFY(!cancelled.leaseId.has_value());

  std::future<hcb::PendingMutationResult> retry = coordinator.retry(cancelled.id);
  verifyValidationError(awaitResult(retry));
}

QTEST_GUILESS_MAIN(OptimisticMutationCoordinatorTest)

#include "OptimisticMutationCoordinatorTest.moc"
