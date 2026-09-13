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
#include "core/CredentialHealthDiagnostics.h"

using namespace std::chrono_literals;

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
  std::optional<hcb::AppError> readError;
  int readCount{0};

  [[nodiscard]] std::future<hcb::OAuthCredentialReadResult> read(QString) override {
    ++readCount;
    return ready(readError.has_value() ? hcb::OAuthCredentialReadResult(*readError)
                                       : hcb::OAuthCredentialReadResult(credential));
  }

  [[nodiscard]] std::future<hcb::OAuthCredentialSaveResult>
  save(QString, hcb::OAuthStoredCredential value) override {
    credential = std::move(value);
    return ready(hcb::OAuthCredentialSaveResult(std::monostate{}));
  }

  [[nodiscard]] std::future<hcb::OAuthCredentialDeleteResult> erase(QString) override {
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

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("credential health request timed out");
  }
  return future.get();
}

void verifyReady(hcb::AccountStatusService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

void saveAccount(hcb::AccountStatusService& service, QStringList scopes) {
  std::future<hcb::AccountStatusSaveResultOrError> future =
      service.upsert({.accountId = QStringLiteral("google:account-1"),
                      .connectionState = hcb::AccountConnectionState::Connected,
                      .grantedScopes = std::move(scopes)});
  const hcb::AccountStatusSaveResultOrError result = awaitResult(future);
  QVERIFY(std::holds_alternative<hcb::AccountStatusSaveResult>(result));
}

[[nodiscard]] hcb::CredentialHealthDiagnostic
diagnosticFor(hcb::CredentialHealthDiagnostics& diagnostics, QString accountId) {
  std::future<hcb::CredentialHealthDiagnosticResult> future =
      diagnostics.inspect(std::move(accountId));
  const hcb::CredentialHealthDiagnosticResult result = awaitResult(future);
  if (!std::holds_alternative<hcb::CredentialHealthDiagnostic>(result)) {
    qFatal("credential health inspection failed");
  }
  return std::get<hcb::CredentialHealthDiagnostic>(result);
}

} // namespace

class CredentialHealthDiagnosticsTest final : public QObject {
  Q_OBJECT

private slots:
  void reportsRefreshableAndDegradedCredentials();
  void reportsMissingAccountWithoutCredentialRead();
  void propagatesCredentialReadFailure();
  void rejectsInvalidAccountIdentifier();
};

void CredentialHealthDiagnosticsTest::reportsRefreshableAndDegradedCredentials() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService accounts(*databasePath, clock);
  verifyReady(accounts);
  saveAccount(accounts,
              {QStringLiteral("https://www.googleapis.com/auth/tasks"),
               QStringLiteral("https://www.googleapis.com/auth/calendar")});
  FakeCredentialStore credentials;
  credentials.credential = {.accessToken = QStringLiteral("access-token"),
                            .refreshToken = QStringLiteral("refresh-token")};
  hcb::CredentialHealthDiagnostics diagnostics(accounts, credentials);

  const hcb::CredentialHealthDiagnostic healthy =
      diagnosticFor(diagnostics, QStringLiteral("google:account-1"));
  QCOMPARE(healthy.connectionState,
           std::optional<hcb::AccountConnectionState>(hcb::AccountConnectionState::Connected));
  QVERIFY(healthy.missingScopes.isEmpty());
  QCOMPARE(healthy.credentialStorage, hcb::CredentialStorageHealth::Refreshable);
  QVERIFY(healthy.isReady());

  credentials.credential = {.accessToken = QStringLiteral("access-token")};
  saveAccount(accounts, {QStringLiteral("https://www.googleapis.com/auth/tasks")});
  const hcb::CredentialHealthDiagnostic degraded =
      diagnosticFor(diagnostics, QStringLiteral("google:account-1"));
  QCOMPARE(degraded.connectionState,
           std::optional<hcb::AccountConnectionState>(hcb::AccountConnectionState::ReauthRequired));
  QCOMPARE(degraded.missingScopes,
           QStringList({QStringLiteral("https://www.googleapis.com/auth/calendar")}));
  QCOMPARE(degraded.credentialStorage, hcb::CredentialStorageHealth::AccessOnly);
  QVERIFY(!degraded.isReady());

  credentials.credential = {.accessToken = QString()};
  const hcb::CredentialHealthDiagnostic invalid =
      diagnosticFor(diagnostics, QStringLiteral("google:account-1"));
  QCOMPARE(invalid.credentialStorage, hcb::CredentialStorageHealth::Invalid);
  QCOMPARE(credentials.readCount, 3);
}

void CredentialHealthDiagnosticsTest::reportsMissingAccountWithoutCredentialRead() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService accounts(*databasePath, clock);
  verifyReady(accounts);
  FakeCredentialStore credentials;
  hcb::CredentialHealthDiagnostics diagnostics(accounts, credentials);

  const hcb::CredentialHealthDiagnostic missing =
      diagnosticFor(diagnostics, QStringLiteral("google:missing"));
  QCOMPARE(missing.accountId, QStringLiteral("google:missing"));
  QVERIFY(!missing.connectionState.has_value());
  QVERIFY(missing.missingScopes.isEmpty());
  QCOMPARE(missing.credentialStorage, hcb::CredentialStorageHealth::NotChecked);
  QVERIFY(!missing.isReady());
  QCOMPARE(credentials.readCount, 0);
}

void CredentialHealthDiagnosticsTest::propagatesCredentialReadFailure() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService accounts(*databasePath, clock);
  verifyReady(accounts);
  saveAccount(accounts,
              {QStringLiteral("https://www.googleapis.com/auth/tasks"),
               QStringLiteral("https://www.googleapis.com/auth/calendar")});
  FakeCredentialStore credentials;
  credentials.readError = hcb::AppError(hcb::AppErrorCode::Database, QStringLiteral("unavailable"));
  hcb::CredentialHealthDiagnostics diagnostics(accounts, credentials);
  std::future<hcb::CredentialHealthDiagnosticResult> future =
      diagnostics.inspect(QStringLiteral("google:account-1"));
  const hcb::CredentialHealthDiagnosticResult result = awaitResult(future);

  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Database);
  QCOMPARE(credentials.readCount, 1);
}

void CredentialHealthDiagnosticsTest::rejectsInvalidAccountIdentifier() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::AccountStatusService accounts(*databasePath, clock);
  verifyReady(accounts);
  FakeCredentialStore credentials;
  hcb::CredentialHealthDiagnostics diagnostics(accounts, credentials);
  std::future<hcb::CredentialHealthDiagnosticResult> future =
      diagnostics.inspect(QStringLiteral(" "));
  const hcb::CredentialHealthDiagnosticResult result = awaitResult(future);

  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
  QCOMPARE(credentials.readCount, 0);
}

QTEST_GUILESS_MAIN(CredentialHealthDiagnosticsTest)

#include "CredentialHealthDiagnosticsTest.moc"
