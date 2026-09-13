#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/AccountStatusService.h"
#include "core/Clock.h"

using namespace std::chrono_literals;

class AccountStatusServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void canonicalizesAndPersistsAccountStatuses();
  void rejectsInvalidAccountStatusInput();
};

namespace {

[[nodiscard]] QString googleTasksScope() {
  return QStringLiteral("https://www.googleapis.com/auth/tasks");
}

[[nodiscard]] QString googleCalendarScope() {
  return QStringLiteral("https://www.googleapis.com/auth/calendar");
}

class TestClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

  void advance(std::chrono::seconds duration) { wallTime_ += duration; }

private:
  hcb::WallTimePoint wallTime_{std::chrono::seconds{1'725'000'000}};
};

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("account status service request timed out");
  }
  return future.get();
}

void verifyReady(hcb::AccountStatusService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

[[nodiscard]] hcb::AccountStatusInput connectedInput(QString accountId) {
  return hcb::AccountStatusInput{.accountId = std::move(accountId),
                                 .providerAccountId = QStringLiteral("google-account-1"),
                                 .email = QStringLiteral("person@example.com"),
                                 .displayName = QStringLiteral("Person Example"),
                                 .avatarUrl = QStringLiteral("https://example.com/avatar.png"),
                                 .locale = QStringLiteral("en-SG"),
                                 .timeZone = QStringLiteral("Asia/Singapore"),
                                 .connectionState = hcb::AccountConnectionState::Connected,
                                 .grantedScopes = {googleTasksScope(),
                                                   QStringLiteral("  "),
                                                   googleCalendarScope(),
                                                   googleTasksScope()},
                                 .lastAuthenticatedAt = QStringLiteral("2026-07-24T00:00:00.000Z")};
}

} // namespace

void AccountStatusServiceTest::canonicalizesAndPersistsAccountStatuses() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  QString firstUpdatedAt;
  {
    hcb::AccountStatusService service(*databasePath, clock);
    std::future<hcb::AccountStatusSaveResultOrError> first =
        service.upsert(connectedInput(QStringLiteral("google:account-1")));
    verifyReady(service);
    const hcb::AccountStatusSaveResultOrError firstResult = awaitResult(first);
    QVERIFY(std::holds_alternative<hcb::AccountStatusSaveResult>(firstResult));
    const hcb::AccountStatusSaveResult& saved = std::get<hcb::AccountStatusSaveResult>(firstResult);
    QVERIFY(saved.mutation == hcb::AccountStatusMutationResult::Changed);
    QVERIFY(saved.status.connectionState == hcb::AccountConnectionState::Connected);
    QCOMPARE(saved.status.grantedScopes, QStringList({googleCalendarScope(), googleTasksScope()}));
    QVERIFY(saved.status.missingScopes.isEmpty());
    firstUpdatedAt = saved.status.updatedAt;

    clock.advance(1h);
    std::future<hcb::AccountStatusSaveResultOrError> unchanged =
        service.upsert(connectedInput(QStringLiteral("google:account-1")));
    const hcb::AccountStatusSaveResultOrError unchangedResult = awaitResult(unchanged);
    QVERIFY(std::holds_alternative<hcb::AccountStatusSaveResult>(unchangedResult));
    const hcb::AccountStatusSaveResult& unchangedSave =
        std::get<hcb::AccountStatusSaveResult>(unchangedResult);
    QVERIFY(unchangedSave.mutation == hcb::AccountStatusMutationResult::Unchanged);
    QCOMPARE(unchangedSave.status.updatedAt, firstUpdatedAt);

    clock.advance(1h);
    hcb::AccountStatusInput missingScope = connectedInput(QStringLiteral("google:account-2"));
    missingScope.providerAccountId = QStringLiteral("google-account-2");
    missingScope.grantedScopes = {googleTasksScope()};
    std::future<hcb::AccountStatusSaveResultOrError> incomplete =
        service.upsert(std::move(missingScope));
    const hcb::AccountStatusSaveResultOrError incompleteResult = awaitResult(incomplete);
    QVERIFY(std::holds_alternative<hcb::AccountStatusSaveResult>(incompleteResult));
    const hcb::AccountStatusSaveResult& incompleteSave =
        std::get<hcb::AccountStatusSaveResult>(incompleteResult);
    QVERIFY(incompleteSave.status.connectionState == hcb::AccountConnectionState::ReauthRequired);
    QCOMPARE(incompleteSave.status.missingScopes, QStringList({googleCalendarScope()}));

    std::future<hcb::AccountStatusListResult> list = service.list();
    const hcb::AccountStatusListResult listResult = awaitResult(list);
    QVERIFY(std::holds_alternative<QList<hcb::AccountStatus>>(listResult));
    const QList<hcb::AccountStatus>& accounts = std::get<QList<hcb::AccountStatus>>(listResult);
    QCOMPARE(accounts.size(), 2);
    QCOMPARE(accounts.at(0).accountId, QStringLiteral("google:account-1"));
    QCOMPARE(accounts.at(1).accountId, QStringLiteral("google:account-2"));
  }

  hcb::AccountStatusService reopened(*databasePath, clock);
  verifyReady(reopened);
  std::future<hcb::AccountStatusLookupResult> persisted =
      reopened.find(QStringLiteral("google:account-1"));
  const hcb::AccountStatusLookupResult persistedResult = awaitResult(persisted);
  QVERIFY(std::holds_alternative<std::optional<hcb::AccountStatus>>(persistedResult));
  const std::optional<hcb::AccountStatus>& persistedStatus =
      std::get<std::optional<hcb::AccountStatus>>(persistedResult);
  QVERIFY(persistedStatus.has_value());
  if (!persistedStatus.has_value()) {
    return;
  }
  QCOMPARE(persistedStatus->updatedAt, firstUpdatedAt);
  QCOMPARE(persistedStatus->email, std::optional<QString>(QStringLiteral("person@example.com")));

  clock.advance(1h);
  std::future<hcb::AccountStatusSaveResultOrError> disconnected =
      reopened.disconnect(QStringLiteral("google:account-1"));
  const hcb::AccountStatusSaveResultOrError disconnectedResult = awaitResult(disconnected);
  QVERIFY(std::holds_alternative<hcb::AccountStatusSaveResult>(disconnectedResult));
  const hcb::AccountStatusSaveResult& disconnectedSave =
      std::get<hcb::AccountStatusSaveResult>(disconnectedResult);
  QVERIFY(disconnectedSave.mutation == hcb::AccountStatusMutationResult::Changed);
  QVERIFY(disconnectedSave.status.connectionState == hcb::AccountConnectionState::SignedOut);
  QVERIFY(disconnectedSave.status.grantedScopes.isEmpty());
  QCOMPARE(disconnectedSave.status.missingScopes,
           QStringList({googleTasksScope(), googleCalendarScope()}));
  QVERIFY(!disconnectedSave.status.lastAuthenticatedAt.has_value());
  QCOMPARE(disconnectedSave.status.email,
           std::optional<QString>(QStringLiteral("person@example.com")));
}

void AccountStatusServiceTest::rejectsInvalidAccountStatusInput() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService service(*databasePath, clock);
  verifyReady(service);

  hcb::AccountStatusInput invalidAccount = connectedInput(QStringLiteral(" google:account-1"));
  std::future<hcb::AccountStatusSaveResultOrError> invalidAccountWrite =
      service.upsert(std::move(invalidAccount));
  const hcb::AccountStatusSaveResultOrError invalidAccountResult = awaitResult(invalidAccountWrite);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidAccountResult));
  QCOMPARE(std::get<hcb::AppError>(invalidAccountResult).code(), hcb::AppErrorCode::Validation);

  hcb::AccountStatusInput invalidScopes = connectedInput(QStringLiteral("google:account-1"));
  for (int index = 0; index <= 20; ++index) {
    invalidScopes.grantedScopes.append(QStringLiteral("scope-%1").arg(index));
  }
  std::future<hcb::AccountStatusSaveResultOrError> invalidScopeWrite =
      service.upsert(std::move(invalidScopes));
  const hcb::AccountStatusSaveResultOrError invalidScopeResult = awaitResult(invalidScopeWrite);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidScopeResult));
  QCOMPARE(std::get<hcb::AppError>(invalidScopeResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::AccountStatusLookupResult> unknown =
      service.find(QStringLiteral("google:missing"));
  const hcb::AccountStatusLookupResult unknownResult = awaitResult(unknown);
  QVERIFY(std::holds_alternative<std::optional<hcb::AccountStatus>>(unknownResult));
  QVERIFY(!std::get<std::optional<hcb::AccountStatus>>(unknownResult).has_value());
}

QTEST_GUILESS_MAIN(AccountStatusServiceTest)

#include "AccountStatusServiceTest.moc"
