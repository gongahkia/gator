#include <QtTest>

#include <QDir>

#include <chrono>
#include <optional>

#include "core/FilePath.h"
#include "support/TestApplicationBootstrap.h"

class TestApplicationBootstrapTest final : public QObject {
  Q_OBJECT

private slots:
  void createsServicesWithDeterministicDependencies();
};

void TestApplicationBootstrapTest::createsServicesWithDeterministicDependencies() {
  const std::optional<hcb::FilePath> dataDirectory = hcb::FilePath::fromAbsolute(QDir::tempPath());
  QVERIFY(dataDirectory.has_value());
  if (!dataDirectory.has_value()) {
    return;
  }
  const std::optional<hcb::FilePath> cacheDirectory = dataDirectory->child(u"hcb-test-cache");
  QVERIFY(cacheDirectory.has_value());
  if (!cacheDirectory.has_value()) {
    return;
  }

  const hcb::WallTimePoint wallTime{std::chrono::seconds{1'725'000'000}};
  const hcb::MonotonicTimePoint monotonicTime{std::chrono::milliseconds{123'456}};
  hcb::test::TestApplicationBootstrap bootstrap(
      hcb::AppPaths::fromDirectories(*dataDirectory, *cacheDirectory), wallTime, monotonicTime);
  hcb::AppServices services = bootstrap.makeServices();

  QCOMPARE(services.paths().dataDirectory().nativePath(), dataDirectory->nativePath());
  QCOMPARE(services.paths().cacheDirectory().nativePath(), cacheDirectory->nativePath());
  QCOMPARE(services.clock().wallNow(), wallTime);
  QCOMPARE(services.clock().monotonicNow(), monotonicTime);

  const hcb::WallTimePoint updatedWallTime{std::chrono::seconds{1'725'000'001}};
  const hcb::MonotonicTimePoint updatedMonotonicTime{std::chrono::milliseconds{123'457}};
  bootstrap.clock().setTimes(updatedWallTime, updatedMonotonicTime);
  QCOMPARE(services.clock().wallNow(), updatedWallTime);
  QCOMPARE(services.clock().monotonicNow(), updatedMonotonicTime);

  const hcb::SettingsKey<bool> animationsEnabled{u"appearance/animations-enabled", true};
  QVERIFY(bootstrap.settings().registerKey(animationsEnabled) ==
          hcb::SettingsRegistrationResult::Registered);
  QVERIFY(services.settings().set(animationsEnabled, false) == hcb::SettingsWriteResult::Changed);
  QVERIFY(bootstrap.settings().value(animationsEnabled) == std::optional<bool>{false});
}

QTEST_MAIN(TestApplicationBootstrapTest)
#include "TestApplicationBootstrapTest.moc"
