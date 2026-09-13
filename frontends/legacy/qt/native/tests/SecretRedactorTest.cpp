#include <QtTest>

#include "core/SecretRedactor.h"

class SecretRedactorTest final : public QObject {
  Q_OBJECT

private slots:
  void redactsSecretAssignmentsAndBearerTokens();
  void redactsBasicAndBearerAuthorizationHeaders();
  void redactsOAuthQueryParameters();
  void redactsSensitiveKeysAndBoundsOutput();
};

void SecretRedactorTest::redactsSecretAssignmentsAndBearerTokens() {
  const QString redacted = hcb::SecretRedactor::redactText(
      QStringLiteral("access_token=fake-access-token client_secret: fake-client-secret "
                     "Authorization: Bearer fake-bearer-token "
                     "{\"mcpToken\":\"fake-mcp-token\"}"));

  QVERIFY(redacted.contains(QStringLiteral("[redacted]")));
  QVERIFY(!redacted.contains(QStringLiteral("fake-access-token")));
  QVERIFY(!redacted.contains(QStringLiteral("fake-client-secret")));
  QVERIFY(!redacted.contains(QStringLiteral("fake-bearer-token")));
  QVERIFY(!redacted.contains(QStringLiteral("fake-mcp-token")));
}

void SecretRedactorTest::redactsBasicAndBearerAuthorizationHeaders() {
  const QString redacted = hcb::SecretRedactor::redactText(
      QStringLiteral("Authorization: Basic ZmFrZS11c2VyOmZha2UtcGFzcw== "
                     "Proxy-Authorization: Bearer fake-proxy-token"));

  QVERIFY(redacted.contains(QStringLiteral("Authorization: [redacted]")));
  QVERIFY(redacted.contains(QStringLiteral("Proxy-Authorization: [redacted]")));
  QVERIFY(!redacted.contains(QStringLiteral("ZmFrZS11c2VyOmZha2UtcGFzcw==")));
  QVERIFY(!redacted.contains(QStringLiteral("fake-proxy-token")));
}

void SecretRedactorTest::redactsOAuthQueryParameters() {
  const QString redacted = hcb::SecretRedactor::redactText(
      QStringLiteral("https://example.invalid/callback?code=fake-code&state=fake-state"));

  QVERIFY(redacted.contains(QStringLiteral("code=[redacted]")));
  QVERIFY(redacted.contains(QStringLiteral("state=[redacted]")));
  QVERIFY(!redacted.contains(QStringLiteral("fake-code")));
  QVERIFY(!redacted.contains(QStringLiteral("fake-state")));
}

void SecretRedactorTest::redactsSensitiveKeysAndBoundsOutput() {
  QCOMPARE(hcb::SecretRedactor::redactKey(u"refreshToken"), QStringLiteral("[redacted]"));
  QVERIFY(hcb::SecretRedactor::isSensitiveKey(u"Authorization"));
  QVERIFY(!hcb::SecretRedactor::isSensitiveKey(u"calendarId"));

  const QString redacted = hcb::SecretRedactor::redactText(QStringLiteral("line one\nline two"), 8);
  QCOMPARE(redacted, QStringLiteral("line one"));
}

QTEST_MAIN(SecretRedactorTest)
#include "SecretRedactorTest.moc"
