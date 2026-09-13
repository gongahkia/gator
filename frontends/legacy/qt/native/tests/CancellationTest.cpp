#include <QtTest>

#include "core/Cancellation.h"

class CancellationTest final : public QObject {
  Q_OBJECT

private slots:
  void stopRequestPropagatesToToken();
  void stopRequestIsIdempotent();
};

void CancellationTest::stopRequestPropagatesToToken() {
  hcb::CancellationSource source;
  const hcb::CancellationToken token = source.token();

  QVERIFY(token.stop_possible());
  QVERIFY(!token.stop_requested());
  QVERIFY(!source.stopRequested());
  QVERIFY(source.requestStop());
  QVERIFY(source.stopRequested());
  QVERIFY(token.stop_requested());
}

void CancellationTest::stopRequestIsIdempotent() {
  hcb::CancellationSource source;

  QVERIFY(source.requestStop());
  QVERIFY(!source.requestStop());
}

QTEST_MAIN(CancellationTest)
#include "CancellationTest.moc"
