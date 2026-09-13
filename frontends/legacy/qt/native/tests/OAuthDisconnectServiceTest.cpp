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
#include "core/OAuthDisconnectService.h"

using namespace std::chrono_literals;

class OAuthDisconnectServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void revokesRefreshTokenErasesCredentialAndSignsOut();
  void signsOutLocallyWhenRemoteRevocationFails();
  void preservesAccountStatusWhenCredentialErasureFails();
  void rejectsInvalidAccountIdentifier();
};

namespace {

class TestClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

private:
  hcb::WallTimePoint wallTime_{std::chrono::seconds{1'725'000'000}};
};

class FakeCredentialStore final : public hcb::OAuthCredentialStore {
public:
  std::optional<hcb::OAuthStoredCredential> credential;
  std::optional<hcb::AppError> eraseError;
  QStringList operations;

  [[nodiscard]] std::future<hcb::OAuthCredentialReadResult> read(QString accountId) override {
    operations.append(QStringLiteral("read:%1").arg(accountId));
    return ready(hcb::OAuthCredentialReadResult(credential));
  }

  [[nodiscard]] std::future<hcb::OAuthCredentialSaveResult>
  save(QString accountId, hcb::OAuthStoredCredential value) override {
    operations.append(QStringLiteral("save:%1").arg(accountId));
    credential = std::move(value);
    return ready(hcb::OAuthCredentialSaveResult(std::monostate{}));
  }

  [[nodiscard]] std::future<hcb::OAuthCredentialDeleteResult> erase(QString accountId) override {
    operations.append(QStringLiteral("erase:%1").arg(accountId));
    if (eraseError.has_value()) {
      return ready(hcb::OAuthCredentialDeleteResult(*eraseError));
    }
    credential.reset();
    return ready(hcb::OAuthCredentialDeleteResult(std::monostate{}));
  }

private:
  template <typename Result> [[nodiscard]] static std::future<Result> ready(Result result) {
    std::promise<Result> promise;
    std::future<Result> future = promise.get_future();
    promise.set_value(std::move(result));
    return future;
  }
};

class FakeTokenRevoker final : public hcb::OAuthTokenRevoker {
public:
  std::optional<hcb::AppError> error;
  QString revokedToken;

  [[nodiscard]] std::future<hcb::OAuthTokenRevocationResult> revoke(QString token) override {
    revokedToken = std::move(token);
    std::promise<hcb::OAuthTokenRevocationResult> promise;
    std::future<hcb::OAuthTokenRevocationResult> future = promise.get_future();
    promise.set_value(error.has_value() ? hcb::OAuthTokenRevocationResult(*error)
                                        : hcb::OAuthTokenRevocationResult(std::monostate{}));
    return future;
  }
};

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("OAuth disconnect request timed out");
  }
  return future.get();
}

void verifyReady(hcb::AccountStatusService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

void connectAccount(hcb::AccountStatusService& service) {
  std::future<hcb::AccountStatusSaveResultOrError> saved =
      service.upsert({.accountId = QStringLiteral("google:account-1"),
                      .providerAccountId = QStringLiteral("google-account-1"),
                      .email = QStringLiteral("person@example.com"),
                      .connectionState = hcb::AccountConnectionState::Connected,
                      .grantedScopes = {QStringLiteral("https://www.googleapis.com/auth/tasks"),
                                        QStringLiteral("https://www.googleapis.com/auth/calendar")},
                      .lastAuthenticatedAt = QStringLiteral("2026-07-24T00:00:00.000Z")});
  const hcb::AccountStatusSaveResultOrError result = awaitResult(saved);
  QVERIFY(std::holds_alternative<hcb::AccountStatusSaveResult>(result));
}

} // namespace

void OAuthDisconnectServiceTest::revokesRefreshTokenErasesCredentialAndSignsOut() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService statuses(*databasePath, clock);
  verifyReady(statuses);
  connectAccount(statuses);
  FakeCredentialStore credentials;
  credentials.credential =
      hcb::OAuthStoredCredential{.accessToken = QStringLiteral("access-token"),
                                 .refreshToken = QStringLiteral("refresh-token")};
  FakeTokenRevoker revoker;
  hcb::OAuthDisconnectService service(statuses, credentials, revoker);

  std::future<hcb::OAuthDisconnectResultOrError> future =
      service.disconnect(QStringLiteral("google:account-1"));
  const hcb::OAuthDisconnectResultOrError result = awaitResult(future);

  QVERIFY(std::holds_alternative<hcb::OAuthDisconnectResult>(result));
  const hcb::OAuthDisconnectResult& disconnected = std::get<hcb::OAuthDisconnectResult>(result);
  QCOMPARE(revoker.revokedToken, QStringLiteral("refresh-token"));
  QCOMPARE(credentials.operations,
           QStringList({QStringLiteral("read:google:account-1"),
                        QStringLiteral("erase:google:account-1")}));
  QVERIFY(!credentials.credential.has_value());
  QVERIFY(disconnected.account.status.connectionState == hcb::AccountConnectionState::SignedOut);
  QVERIFY(disconnected.remoteRevocationState == hcb::OAuthRemoteRevocationState::Revoked);
  QVERIFY(!disconnected.remoteRevocationError.has_value());
}

void OAuthDisconnectServiceTest::signsOutLocallyWhenRemoteRevocationFails() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService statuses(*databasePath, clock);
  verifyReady(statuses);
  connectAccount(statuses);
  FakeCredentialStore credentials;
  credentials.credential =
      hcb::OAuthStoredCredential{.accessToken = QStringLiteral("access-token"),
                                 .refreshToken = QStringLiteral("refresh-token")};
  FakeTokenRevoker revoker;
  revoker.error = hcb::AppError(hcb::AppErrorCode::Network, QStringLiteral("network unavailable"));
  hcb::OAuthDisconnectService service(statuses, credentials, revoker);

  std::future<hcb::OAuthDisconnectResultOrError> future =
      service.disconnect(QStringLiteral("google:account-1"));
  const hcb::OAuthDisconnectResultOrError result = awaitResult(future);

  QVERIFY(std::holds_alternative<hcb::OAuthDisconnectResult>(result));
  const hcb::OAuthDisconnectResult& disconnected = std::get<hcb::OAuthDisconnectResult>(result);
  QVERIFY(!credentials.credential.has_value());
  QVERIFY(disconnected.account.status.connectionState == hcb::AccountConnectionState::SignedOut);
  QVERIFY(disconnected.remoteRevocationState == hcb::OAuthRemoteRevocationState::Failed);
  QVERIFY(disconnected.remoteRevocationError.has_value());
  QCOMPARE(disconnected.remoteRevocationError->code(), hcb::AppErrorCode::Network);
}

void OAuthDisconnectServiceTest::preservesAccountStatusWhenCredentialErasureFails() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService statuses(*databasePath, clock);
  verifyReady(statuses);
  connectAccount(statuses);
  FakeCredentialStore credentials;
  credentials.credential =
      hcb::OAuthStoredCredential{.accessToken = QStringLiteral("access-token")};
  credentials.eraseError =
      hcb::AppError(hcb::AppErrorCode::Database, QStringLiteral("erase failed"));
  FakeTokenRevoker revoker;
  hcb::OAuthDisconnectService service(statuses, credentials, revoker);

  std::future<hcb::OAuthDisconnectResultOrError> future =
      service.disconnect(QStringLiteral("google:account-1"));
  const hcb::OAuthDisconnectResultOrError result = awaitResult(future);

  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Database);
  QVERIFY(credentials.credential.has_value());
  std::future<hcb::AccountStatusLookupResult> account =
      statuses.find(QStringLiteral("google:account-1"));
  const hcb::AccountStatusLookupResult accountResult = awaitResult(account);
  QVERIFY(std::holds_alternative<std::optional<hcb::AccountStatus>>(accountResult));
  const std::optional<hcb::AccountStatus>& stored =
      std::get<std::optional<hcb::AccountStatus>>(accountResult);
  QVERIFY(stored.has_value());
  if (stored.has_value()) {
    QVERIFY(stored->connectionState == hcb::AccountConnectionState::Connected);
  }
}

void OAuthDisconnectServiceTest::rejectsInvalidAccountIdentifier() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService statuses(*databasePath, clock);
  verifyReady(statuses);
  FakeCredentialStore credentials;
  FakeTokenRevoker revoker;
  hcb::OAuthDisconnectService service(statuses, credentials, revoker);

  std::future<hcb::OAuthDisconnectResultOrError> future = service.disconnect(QStringLiteral(" "));
  const hcb::OAuthDisconnectResultOrError result = awaitResult(future);

  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
  QVERIFY(credentials.operations.isEmpty());
}

QTEST_GUILESS_MAIN(OAuthDisconnectServiceTest)

#include "OAuthDisconnectServiceTest.moc"
