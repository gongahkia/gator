#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/Clock.h"
#include "core/SettingsService.h"

using namespace std::chrono_literals;

class SettingsServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void queuesSchemaAndPersistsJsonValues();
  void rejectsInvalidSettingsInput();
};

namespace {

class TestClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

private:
  hcb::WallTimePoint wallTime_{std::chrono::seconds{1'725'000'000}};
};

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("settings service request timed out");
  }
  return future.get();
}

void verifyReady(hcb::SettingsService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

} // namespace

void SettingsServiceTest::queuesSchemaAndPersistsJsonValues() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  {
    hcb::SettingsService service(*databasePath, clock);
    std::future<hcb::SettingsMutationResultOrError> write = service.writeJson(
        QStringLiteral("appearance"), QStringLiteral("theme"), QStringLiteral("\"dark\""));
    verifyReady(service);
    const hcb::SettingsMutationResultOrError firstWrite = awaitResult(write);
    QVERIFY(std::holds_alternative<hcb::SettingsMutationResult>(firstWrite));
    QCOMPARE(std::get<hcb::SettingsMutationResult>(firstWrite),
             hcb::SettingsMutationResult::Changed);

    std::future<hcb::SettingsMutationResultOrError> unchanged = service.writeJson(
        QStringLiteral("appearance"), QStringLiteral("theme"), QStringLiteral("\"dark\""));
    const hcb::SettingsMutationResultOrError unchangedResult = awaitResult(unchanged);
    QVERIFY(std::holds_alternative<hcb::SettingsMutationResult>(unchangedResult));
    QCOMPARE(std::get<hcb::SettingsMutationResult>(unchangedResult),
             hcb::SettingsMutationResult::Unchanged);

    std::future<hcb::SettingsJsonReadResult> read =
        service.readJson(QStringLiteral("appearance"), QStringLiteral("theme"));
    const hcb::SettingsJsonReadResult readResult = awaitResult(read);
    QVERIFY(std::holds_alternative<std::optional<QString>>(readResult));
    QCOMPARE(std::get<std::optional<QString>>(readResult),
             std::optional<QString>(QStringLiteral("\"dark\"")));
  }

  hcb::SettingsService reopened(*databasePath, clock);
  verifyReady(reopened);
  std::future<hcb::SettingsJsonReadResult> persisted =
      reopened.readJson(QStringLiteral("appearance"), QStringLiteral("theme"));
  const hcb::SettingsJsonReadResult persistedResult = awaitResult(persisted);
  QVERIFY(std::holds_alternative<std::optional<QString>>(persistedResult));
  QCOMPARE(std::get<std::optional<QString>>(persistedResult),
           std::optional<QString>(QStringLiteral("\"dark\"")));

  std::future<hcb::SettingsMutationResultOrError> erase =
      reopened.erase(QStringLiteral("appearance"), QStringLiteral("theme"));
  const hcb::SettingsMutationResultOrError eraseResult = awaitResult(erase);
  QVERIFY(std::holds_alternative<hcb::SettingsMutationResult>(eraseResult));
  QCOMPARE(std::get<hcb::SettingsMutationResult>(eraseResult),
           hcb::SettingsMutationResult::Changed);
  std::future<hcb::SettingsJsonReadResult> erased =
      reopened.readJson(QStringLiteral("appearance"), QStringLiteral("theme"));
  const hcb::SettingsJsonReadResult erasedResult = awaitResult(erased);
  QVERIFY(std::holds_alternative<std::optional<QString>>(erasedResult));
  QVERIFY(!std::get<std::optional<QString>>(erasedResult).has_value());
}

void SettingsServiceTest::rejectsInvalidSettingsInput() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::SettingsService service(*databasePath, clock);
  verifyReady(service);

  std::future<hcb::SettingsMutationResultOrError> invalidScope = service.writeJson(
      QStringLiteral(" appearance"), QStringLiteral("theme"), QStringLiteral("\"dark\""));
  const hcb::SettingsMutationResultOrError invalidScopeResult = awaitResult(invalidScope);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidScopeResult));
  QCOMPARE(std::get<hcb::AppError>(invalidScopeResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::SettingsMutationResultOrError> invalidJson = service.writeJson(
      QStringLiteral("appearance"), QStringLiteral("theme"), QStringLiteral("not-json"));
  const hcb::SettingsMutationResultOrError invalidJsonResult = awaitResult(invalidJson);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidJsonResult));
  QCOMPARE(std::get<hcb::AppError>(invalidJsonResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::SettingsJsonReadResult> unknown =
      service.readJson(QStringLiteral("appearance"), QStringLiteral("theme"));
  const hcb::SettingsJsonReadResult unknownResult = awaitResult(unknown);
  QVERIFY(std::holds_alternative<std::optional<QString>>(unknownResult));
  QVERIFY(!std::get<std::optional<QString>>(unknownResult).has_value());
}

QTEST_GUILESS_MAIN(SettingsServiceTest)

#include "SettingsServiceTest.moc"
