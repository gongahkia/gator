#include <QtTest/QTest>

#include <optional>
#include <variant>

#include "core/OAuthBrowserAuthorizationLauncher.h"

class OAuthBrowserAuthorizationLauncherTest final : public QObject {
  Q_OBJECT

private slots:
  void opensApprovedGoogleAuthorizationUrl();
  void rejectsUnapprovedAuthorizationUrls();
  void reportsBrowserLaunchFailure();
};

void OAuthBrowserAuthorizationLauncherTest::opensApprovedGoogleAuthorizationUrl() {
  std::optional<QUrl> openedUrl;
  hcb::OAuthBrowserAuthorizationLauncher launcher([&openedUrl](const QUrl& url) {
    openedUrl = url;
    return true;
  });
  const QUrl authorizationUrl(
      QStringLiteral("https://accounts.google.com/o/oauth2/v2/auth?client_id=desktop-client"));

  const hcb::OAuthBrowserAuthorizationLaunchResult result = launcher.launch(authorizationUrl);

  QVERIFY(std::holds_alternative<std::monostate>(result));
  QCOMPARE(openedUrl, std::optional<QUrl>(authorizationUrl));
  QVERIFY(hcb::OAuthBrowserAuthorizationLauncher::isAllowedAuthorizationUrl(authorizationUrl));
}

void OAuthBrowserAuthorizationLauncherTest::rejectsUnapprovedAuthorizationUrls() {
  int openCount = 0;
  hcb::OAuthBrowserAuthorizationLauncher launcher([&openCount](const QUrl&) {
    ++openCount;
    return true;
  });
  const QList<QUrl> rejected = {
      QUrl(QStringLiteral("http://accounts.google.com/o/oauth2/v2/auth")),
      QUrl(QStringLiteral("https://example.invalid/o/oauth2/v2/auth")),
      QUrl(QStringLiteral("https://user@accounts.google.com/o/oauth2/v2/auth")),
      QUrl(QStringLiteral("https://accounts.google.com:444/o/oauth2/v2/auth")),
      QUrl(QStringLiteral("https://accounts.google.com/o/oauth2/v2/auth#fragment")),
  };

  for (const QUrl& url : rejected) {
    const hcb::OAuthBrowserAuthorizationLaunchResult result = launcher.launch(url);
    QVERIFY(std::holds_alternative<hcb::AppError>(result));
    QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
  }
  QCOMPARE(openCount, 0);
}

void OAuthBrowserAuthorizationLauncherTest::reportsBrowserLaunchFailure() {
  hcb::OAuthBrowserAuthorizationLauncher launcher([](const QUrl&) { return false; });
  const hcb::OAuthBrowserAuthorizationLaunchResult result = launcher.launch(
      QUrl(QStringLiteral("https://accounts.google.com/o/oauth2/v2/auth?state=state-value")));

  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Network);
}

QTEST_GUILESS_MAIN(OAuthBrowserAuthorizationLauncherTest)

#include "OAuthBrowserAuthorizationLauncherTest.moc"
