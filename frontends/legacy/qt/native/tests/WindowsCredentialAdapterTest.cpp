#include <QtTest>

#include "app/WindowsCredentialAdapter.h"

#include <QScopeGuard>
#include <QUuid>

#include <chrono>
#include <future>
#include <optional>
#include <variant>

using namespace std::chrono_literals;

class WindowsCredentialAdapterTest final : public QObject {
  Q_OBJECT

private slots:
  void savesReadsAndErasesCredential();
  void rejectsCredentialBeyondCredentialManagerLimit();
  void rejectsInvalidInput();
};

namespace {

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("Windows credential operation timed out");
  }
  return future.get();
}

} // namespace

void WindowsCredentialAdapterTest::savesReadsAndErasesCredential() {
#if defined(Q_OS_WIN)
  hcb::WindowsCredentialAdapter adapter;
  const QString accountId =
      QStringLiteral("test:%1").arg(QUuid::createUuid().toString(QUuid::WithoutBraces));
  const auto cleanup = qScopeGuard([&adapter, &accountId] {
    std::future<hcb::OAuthCredentialDeleteResult> erased = adapter.erase(accountId);
    static_cast<void>(erased.get());
  });

  std::future<hcb::OAuthCredentialSaveResult> saved =
      adapter.save(accountId,
                   {.accessToken = QStringLiteral("access-token"),
                    .refreshToken = QStringLiteral("refresh-token")});
  const hcb::OAuthCredentialSaveResult savedResult = awaitResult(saved);
  QVERIFY(std::holds_alternative<std::monostate>(savedResult));

  std::future<hcb::OAuthCredentialReadResult> read = adapter.read(accountId);
  const hcb::OAuthCredentialReadResult readResult = awaitResult(read);
  QVERIFY(std::holds_alternative<std::optional<hcb::OAuthStoredCredential>>(readResult));
  const std::optional<hcb::OAuthStoredCredential>& credential =
      std::get<std::optional<hcb::OAuthStoredCredential>>(readResult);
  QVERIFY(credential.has_value());
  if (!credential.has_value()) {
    return;
  }
  QCOMPARE(credential->accessToken, QStringLiteral("access-token"));
  QCOMPARE(credential->refreshToken, std::optional<QString>(QStringLiteral("refresh-token")));

  std::future<hcb::OAuthCredentialDeleteResult> erased = adapter.erase(accountId);
  const hcb::OAuthCredentialDeleteResult erasedResult = awaitResult(erased);
  QVERIFY(std::holds_alternative<std::monostate>(erasedResult));

  std::future<hcb::OAuthCredentialReadResult> missing = adapter.read(accountId);
  const hcb::OAuthCredentialReadResult missingResult = awaitResult(missing);
  QVERIFY(std::holds_alternative<std::optional<hcb::OAuthStoredCredential>>(missingResult));
  QVERIFY(!std::get<std::optional<hcb::OAuthStoredCredential>>(missingResult).has_value());
#else
  QSKIP("Windows-only adapter");
#endif
}

void WindowsCredentialAdapterTest::rejectsCredentialBeyondCredentialManagerLimit() {
#if defined(Q_OS_WIN)
  hcb::WindowsCredentialAdapter adapter;
  std::future<hcb::OAuthCredentialSaveResult> saved =
      adapter.save(QStringLiteral("google:account-1"), {.accessToken = QString(2'560, u'x')});
  const hcb::OAuthCredentialSaveResult result = awaitResult(saved);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Configuration);
#else
  QSKIP("Windows-only adapter");
#endif
}

void WindowsCredentialAdapterTest::rejectsInvalidInput() {
  hcb::WindowsCredentialAdapter adapter;

  std::future<hcb::OAuthCredentialSaveResult> invalidAccount =
      adapter.save(QStringLiteral(" "), {.accessToken = QStringLiteral("access-token")});
  const hcb::OAuthCredentialSaveResult invalidAccountResult = awaitResult(invalidAccount);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidAccountResult));
  QCOMPARE(std::get<hcb::AppError>(invalidAccountResult).code(), hcb::AppErrorCode::Configuration);

  std::future<hcb::OAuthCredentialSaveResult> invalidToken =
      adapter.save(QStringLiteral("google:account-1"), {.accessToken = QString()});
  const hcb::OAuthCredentialSaveResult invalidTokenResult = awaitResult(invalidToken);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidTokenResult));
  QCOMPARE(std::get<hcb::AppError>(invalidTokenResult).code(), hcb::AppErrorCode::Configuration);
}

QTEST_GUILESS_MAIN(WindowsCredentialAdapterTest)

#include "WindowsCredentialAdapterTest.moc"
