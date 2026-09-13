#include <QtTest>

#include "core/GoogleApiError.h"

class GoogleApiErrorTest final : public QObject {
  Q_OBJECT

private slots:
  void mapsHttpStatuses_data();
  void mapsHttpStatuses();
  void identifiesQuotaErrorsAndPreservesResponseMetadata();
  void redactsExplicitErrorMessages();
};

void GoogleApiErrorTest::mapsHttpStatuses_data() {
  QTest::addColumn<int>("status");
  QTest::addColumn<hcb::GoogleApiErrorKind>("expectedKind");

  QTest::newRow("unauthorized") << 401 << hcb::GoogleApiErrorKind::Unauthorized;
  QTest::newRow("forbidden") << 403 << hcb::GoogleApiErrorKind::Forbidden;
  QTest::newRow("not-found") << 404 << hcb::GoogleApiErrorKind::NotFound;
  QTest::newRow("conflict") << 409 << hcb::GoogleApiErrorKind::Conflict;
  QTest::newRow("invalid-sync-token") << 410 << hcb::GoogleApiErrorKind::InvalidSyncToken;
  QTest::newRow("precondition-failed") << 412 << hcb::GoogleApiErrorKind::PreconditionFailed;
  QTest::newRow("rate-limited") << 429 << hcb::GoogleApiErrorKind::RateLimited;
  QTest::newRow("server") << 503 << hcb::GoogleApiErrorKind::Server;
  QTest::newRow("invalid-payload") << 400 << hcb::GoogleApiErrorKind::InvalidPayload;
  QTest::newRow("other") << 418 << hcb::GoogleApiErrorKind::Transport;
}

void GoogleApiErrorTest::mapsHttpStatuses() {
  QFETCH(int, status);
  QFETCH(hcb::GoogleApiErrorKind, expectedKind);

  const hcb::GoogleApiError error = hcb::GoogleApiError::fromHttpStatus(status, u"{}");

  QCOMPARE(error.kind(), expectedKind);
  QCOMPARE(error.status(), std::optional<int>(status));
}

void GoogleApiErrorTest::identifiesQuotaErrorsAndPreservesResponseMetadata() {
  const hcb::GoogleApiError error = hcb::GoogleApiError::fromHttpStatus(
      403, u"{\"error\": {\"reason\": \"dailyLimitExceeded\"}}", 3'000);

  QCOMPARE(error.kind(), hcb::GoogleApiErrorKind::Forbidden);
  QVERIFY(error.quotaExceeded());
  QCOMPARE(error.retryAfterMilliseconds(), std::optional<qint64>(3'000));
  QCOMPARE(error.responseBodyBytes(), std::optional<qsizetype>(43));
}

void GoogleApiErrorTest::redactsExplicitErrorMessages() {
  hcb::GoogleApiErrorOptions options;
  options.kind = hcb::GoogleApiErrorKind::Transport;
  options.message = QStringLiteral("Transport failed with access_token=fake-access-token");

  const hcb::GoogleApiError error(options);

  QCOMPARE(error.message(), QStringLiteral("Transport failed with access_token=[redacted]"));
}

QTEST_MAIN(GoogleApiErrorTest)
#include "GoogleApiErrorTest.moc"
