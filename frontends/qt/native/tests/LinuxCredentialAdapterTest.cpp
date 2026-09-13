#include <QtTest>

#include "app/LinuxCredentialAdapter.h"

#if defined(Q_OS_LINUX)
#include <QDBusConnection>
#include <QDBusInterface>
#endif
#include <QScopeGuard>
#include <QUuid>

#include <chrono>
#include <future>
#include <optional>
#include <variant>

using namespace std::chrono_literals;

class LinuxCredentialAdapterTest final : public QObject {
  Q_OBJECT

private slots:
  void savesReadsAndErasesCredential();
  void rejectsInvalidInput();
};

namespace {

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("Linux credential operation timed out");
  }
  return future.get();
}

} // namespace

void LinuxCredentialAdapterTest::savesReadsAndErasesCredential() {
#if defined(Q_OS_LINUX)
  const QDBusConnection connection = QDBusConnection::sessionBus();
  if (!connection.isConnected()) {
    QSKIP("no D-Bus session is available");
  }
  QDBusInterface service(QStringLiteral("org.freedesktop.secrets"),
                         QStringLiteral("/org/freedesktop/secrets"),
                         QStringLiteral("org.freedesktop.Secret.Service"),
                         connection);
  if (!service.isValid()) {
    QSKIP("Linux Secret Service is unavailable");
  }

  hcb::LinuxCredentialAdapter adapter;
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
  QSKIP("Linux-only adapter");
#endif
}

void LinuxCredentialAdapterTest::rejectsInvalidInput() {
  hcb::LinuxCredentialAdapter adapter;

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

QTEST_GUILESS_MAIN(LinuxCredentialAdapterTest)

#include "LinuxCredentialAdapterTest.moc"
