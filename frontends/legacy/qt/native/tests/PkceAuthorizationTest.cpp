#include <QtTest>

#include "core/PkceAuthorization.h"

namespace {

class ControlledClock final : public hcb::Clock {
public:
  explicit ControlledClock(hcb::MonotonicTimePoint monotonicTime) : monotonicTime_(monotonicTime) {}

  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return {}; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return monotonicTime_;
  }
  void advance(std::chrono::steady_clock::duration duration) { monotonicTime_ += duration; }

private:
  hcb::MonotonicTimePoint monotonicTime_;
};

} // namespace

class PkceAuthorizationTest final : public QObject {
  Q_OBJECT

private slots:
  void generatesRfc7636Challenge();
  void rejectsInvalidCodeVerifiers();
  void createsValidRandomAuthorizationValues();
  void consumesStateExactlyOnce();
  void rejectsMalformedStateWithoutConsumingRequest();
  void rejectsExpiredState();
};

void PkceAuthorizationTest::generatesRfc7636Challenge() {
  const std::optional<QString> challenge = hcb::PkceAuthorization::codeChallengeForVerifier(
      u"dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk");

  if (!challenge.has_value()) {
    QFAIL("expected RFC 7636 challenge");
    return;
  }
  QCOMPARE(challenge.value(), QStringLiteral("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"));
}

void PkceAuthorizationTest::rejectsInvalidCodeVerifiers() {
  QVERIFY(!hcb::PkceAuthorization::isValidCodeVerifier(u"too-short"));
  QVERIFY(!hcb::PkceAuthorization::isValidCodeVerifier(QString(129, QChar(u'a'))));
  QVERIFY(!hcb::PkceAuthorization::isValidCodeVerifier(
      QStringLiteral("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjX!")));
  QVERIFY(!hcb::PkceAuthorization::codeChallengeForVerifier(u"too-short").has_value());
}

void PkceAuthorizationTest::createsValidRandomAuthorizationValues() {
  const QString codeVerifier = hcb::PkceAuthorization::generateCodeVerifier();
  const QString state = hcb::PkceAuthorization::generateState();
  const std::optional<QString> challenge =
      hcb::PkceAuthorization::codeChallengeForVerifier(codeVerifier);

  QVERIFY(hcb::PkceAuthorization::isValidCodeVerifier(codeVerifier));
  QCOMPARE(codeVerifier.size(), 86);
  QCOMPARE(state.size(), 32);
  if (!challenge.has_value()) {
    QFAIL("expected generated code challenge");
    return;
  }
  QCOMPARE(challenge.value().size(), 43);
}

void PkceAuthorizationTest::consumesStateExactlyOnce() {
  ControlledClock clock(hcb::MonotonicTimePoint{});
  hcb::PkceStateRegistry registry(clock);
  const hcb::PkceAuthorizationRequest request = registry.begin();

  const hcb::PkceStateValidationResult unrecognized = registry.consume(u"not-the-state");
  QCOMPARE(unrecognized.status, hcb::PkceStateValidationStatus::Unrecognized);
  QVERIFY(unrecognized.codeVerifier.isEmpty());

  const hcb::PkceStateValidationResult accepted = registry.consume(request.state);
  QCOMPARE(accepted.status, hcb::PkceStateValidationStatus::Accepted);
  QVERIFY(hcb::PkceAuthorization::isValidCodeVerifier(accepted.codeVerifier));
  const std::optional<QString> codeChallenge =
      hcb::PkceAuthorization::codeChallengeForVerifier(accepted.codeVerifier);
  if (!codeChallenge.has_value()) {
    QFAIL("expected accepted verifier challenge");
    return;
  }
  QCOMPARE(codeChallenge.value(), request.codeChallenge);

  const hcb::PkceStateValidationResult repeated = registry.consume(request.state);
  QCOMPARE(repeated.status, hcb::PkceStateValidationStatus::Unrecognized);
}

void PkceAuthorizationTest::rejectsMalformedStateWithoutConsumingRequest() {
  ControlledClock clock(hcb::MonotonicTimePoint{});
  hcb::PkceStateRegistry registry(clock);
  const hcb::PkceAuthorizationRequest request = registry.begin();

  const hcb::PkceStateValidationResult oversized = registry.consume(QString(4096, QChar(u'a')));
  QCOMPARE(oversized.status, hcb::PkceStateValidationStatus::Unrecognized);
  QVERIFY(oversized.codeVerifier.isEmpty());

  QString malformed = request.state;
  malformed[0] = u'=';
  const hcb::PkceStateValidationResult invalidCharacters = registry.consume(malformed);
  QCOMPARE(invalidCharacters.status, hcb::PkceStateValidationStatus::Unrecognized);
  QVERIFY(invalidCharacters.codeVerifier.isEmpty());

  const hcb::PkceStateValidationResult accepted = registry.consume(request.state);
  QCOMPARE(accepted.status, hcb::PkceStateValidationStatus::Accepted);
  QVERIFY(hcb::PkceAuthorization::isValidCodeVerifier(accepted.codeVerifier));
}

void PkceAuthorizationTest::rejectsExpiredState() {
  ControlledClock clock(hcb::MonotonicTimePoint{});
  hcb::PkceStateRegistry registry(clock);
  const hcb::PkceAuthorizationRequest request = registry.begin();
  clock.advance(hcb::PkceStateRegistry::kStateTtl);

  const hcb::PkceStateValidationResult expired = registry.consume(request.state);

  QCOMPARE(expired.status, hcb::PkceStateValidationStatus::Expired);
  QVERIFY(expired.codeVerifier.isEmpty());
}

QTEST_MAIN(PkceAuthorizationTest)
#include "PkceAuthorizationTest.moc"
