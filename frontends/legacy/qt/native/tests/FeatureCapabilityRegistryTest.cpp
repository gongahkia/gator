#include <QtTest>

#include "core/FeatureCapabilityRegistry.h"

class FeatureCapabilityRegistryTest final : public QObject {
  Q_OBJECT

private slots:
  void constructsWithDeclaredCapabilities();
  void enablesAndDisablesCapabilitiesIdempotently();
  void rejectsInvalidCapabilities();
};

void FeatureCapabilityRegistryTest::constructsWithDeclaredCapabilities() {
  hcb::FeatureCapabilityRegistry capabilities{
      hcb::FeatureCapability::Tasks,
      hcb::FeatureCapability::Calendar,
  };

  QVERIFY(capabilities.isEnabled(hcb::FeatureCapability::Tasks));
  QVERIFY(capabilities.isEnabled(hcb::FeatureCapability::Calendar));
  QVERIFY(!capabilities.isEnabled(hcb::FeatureCapability::Notes));
}

void FeatureCapabilityRegistryTest::enablesAndDisablesCapabilitiesIdempotently() {
  hcb::FeatureCapabilityRegistry capabilities;

  QVERIFY(capabilities.enable(hcb::FeatureCapability::Notes));
  QVERIFY(!capabilities.enable(hcb::FeatureCapability::Notes));
  QVERIFY(capabilities.isEnabled(hcb::FeatureCapability::Notes));
  QVERIFY(capabilities.disable(hcb::FeatureCapability::Notes));
  QVERIFY(!capabilities.disable(hcb::FeatureCapability::Notes));
  QVERIFY(!capabilities.isEnabled(hcb::FeatureCapability::Notes));
}

void FeatureCapabilityRegistryTest::rejectsInvalidCapabilities() {
  hcb::FeatureCapabilityRegistry capabilities;
  constexpr hcb::FeatureCapability invalidCapability = hcb::FeatureCapability::Invalid;

  QVERIFY(!capabilities.enable(invalidCapability));
  QVERIFY(!capabilities.disable(invalidCapability));
  QVERIFY(!capabilities.isEnabled(invalidCapability));
}

QTEST_MAIN(FeatureCapabilityRegistryTest)
#include "FeatureCapabilityRegistryTest.moc"
