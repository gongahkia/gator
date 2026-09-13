#include <QtTest>

#include "core/GoogleApiError.h"
#include "core/SyncBackoffPolicy.h"

class SyncBackoffPolicyTest final : public QObject {
  Q_OBJECT

private slots:
  void appliesConstraintMultipliers();
  void multipliesExponentialDelayUnderConstraints();
  void multipliesRetryAfterUnderConstraints();
  void rejectsNonRetryableAndQuotaErrors();
  void clampsInvalidConfigurationAndAttempts();
  void saturatesLargeExponentialDelays();
};

void SyncBackoffPolicyTest::appliesConstraintMultipliers() {
  QCOMPARE(hcb::syncBackoffConstraintMultiplier({}), 1.0);
  QCOMPARE(hcb::syncBackoffConstraintMultiplier({.lowPowerMode = true}), 1.5);
  QCOMPARE(hcb::syncBackoffConstraintMultiplier({.constrainedNetwork = true}), 2.0);
  QCOMPARE(hcb::syncBackoffConstraintMultiplier({.lowPowerMode = true, .constrainedNetwork = true}),
           3.0);
}

void SyncBackoffPolicyTest::multipliesExponentialDelayUnderConstraints() {
  hcb::SyncBackoffPolicyOptions options;
  options.baseDelayMilliseconds = 1'000;
  options.jitterMilliseconds = 200;
  options.random = [] { return 0.5; };
  options.constraintState = [] { return hcb::SyncBackoffConstraintState{true, true}; };
  const hcb::SyncBackoffPolicy policy(std::move(options));

  QCOMPARE(policy.delayMillisecondsForAttempt(1), 6'300);
}

void SyncBackoffPolicyTest::multipliesRetryAfterUnderConstraints() {
  hcb::SyncBackoffPolicyOptions options;
  options.constraintState = [] { return hcb::SyncBackoffConstraintState{false, true}; };
  const hcb::SyncBackoffPolicy policy(std::move(options));
  hcb::GoogleApiErrorOptions errorOptions;
  errorOptions.kind = hcb::GoogleApiErrorKind::Server;
  errorOptions.message = QStringLiteral("Google service unavailable");
  errorOptions.retryAfterMilliseconds = 10'000;
  const hcb::GoogleApiError error(errorOptions);

  QCOMPARE(policy.retryDelayMilliseconds(error, 0), std::optional<qint64>(20'000));
}

void SyncBackoffPolicyTest::rejectsNonRetryableAndQuotaErrors() {
  const hcb::SyncBackoffPolicy policy;
  hcb::GoogleApiErrorOptions forbiddenOptions;
  forbiddenOptions.kind = hcb::GoogleApiErrorKind::Forbidden;
  forbiddenOptions.message = QStringLiteral("Google denied access");
  const hcb::GoogleApiError forbidden(forbiddenOptions);
  QVERIFY(!policy.shouldBackOff(forbidden));
  QVERIFY(!policy.retryDelayMilliseconds(forbidden, 0).has_value());

  hcb::GoogleApiErrorOptions quotaOptions;
  quotaOptions.kind = hcb::GoogleApiErrorKind::RateLimited;
  quotaOptions.message = QStringLiteral("Google quota exhausted");
  quotaOptions.quotaExceeded = true;
  const hcb::GoogleApiError quotaError(quotaOptions);
  QVERIFY(!policy.shouldBackOff(quotaError));
  QVERIFY(!policy.retryDelayMilliseconds(quotaError, 0).has_value());
}

void SyncBackoffPolicyTest::clampsInvalidConfigurationAndAttempts() {
  hcb::SyncBackoffPolicyOptions options;
  options.baseDelayMilliseconds = -1;
  options.maximumDelayMilliseconds = -1;
  options.jitterMilliseconds = -1;
  options.maximumAttempts = -1;
  options.random = [] { return std::numeric_limits<double>::quiet_NaN(); };
  const hcb::SyncBackoffPolicy policy(std::move(options));

  QCOMPARE(policy.delayMillisecondsForAttempt(-1), 0);
  QCOMPARE(policy.delayMillisecondsForAttempt(20), 0);
}

void SyncBackoffPolicyTest::saturatesLargeExponentialDelays() {
  hcb::SyncBackoffPolicyOptions options;
  options.baseDelayMilliseconds = std::numeric_limits<qint64>::max() / 2 + 1;
  options.maximumDelayMilliseconds = std::numeric_limits<qint64>::max();
  options.jitterMilliseconds = 0;
  options.maximumAttempts = 1;
  options.random = [] { return 0.0; };
  const hcb::SyncBackoffPolicy policy(std::move(options));

  QCOMPARE(policy.delayMillisecondsForAttempt(1), std::numeric_limits<qint64>::max());
}

QTEST_MAIN(SyncBackoffPolicyTest)
#include "SyncBackoffPolicyTest.moc"
