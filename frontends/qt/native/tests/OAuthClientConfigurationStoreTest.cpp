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
#include "core/OAuthClientConfigurationStore.h"

using namespace std::chrono_literals;

class OAuthClientConfigurationStoreTest final : public QObject {
  Q_OBJECT

private slots:
  void persistsAndClearsClientConfiguration();
  void rejectsInvalidClientIds();
};

namespace {

class TestClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

  void advance(std::chrono::seconds duration) { wallTime_ += duration; }

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
    qFatal("OAuth client configuration request timed out");
  }
  return future.get();
}

void verifyReady(hcb::OAuthClientConfigurationStore& store) {
  const std::shared_future<hcb::SqliteWriteResult> ready = store.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

[[nodiscard]] QString clientId() {
  return QStringLiteral("desktop-client-id.apps.googleusercontent.com");
}

[[nodiscard]] QString clientSecret() { return QStringLiteral("desktop-client-secret"); }

} // namespace

void OAuthClientConfigurationStoreTest::persistsAndClearsClientConfiguration() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  QString firstUpdatedAt;
  {
    hcb::OAuthClientConfigurationStore store(*databasePath, clock);
    verifyReady(store);

    std::future<hcb::OAuthClientConfigurationReadResult> initial = store.load();
    const hcb::OAuthClientConfigurationReadResult initialResult = awaitResult(initial);
    QVERIFY(std::holds_alternative<std::optional<hcb::OAuthClientConfiguration>>(initialResult));
    QVERIFY(!std::get<std::optional<hcb::OAuthClientConfiguration>>(initialResult).has_value());

    std::future<hcb::OAuthClientConfigurationMutationResultOrError> saved =
        store.save(QStringLiteral("  %1  ").arg(clientId()),
                   QStringLiteral("  %1  ").arg(clientSecret()));
    const hcb::OAuthClientConfigurationMutationResultOrError savedResult = awaitResult(saved);
    QVERIFY(std::holds_alternative<hcb::OAuthClientConfigurationMutationResult>(savedResult));
    QCOMPARE(std::get<hcb::OAuthClientConfigurationMutationResult>(savedResult),
             hcb::OAuthClientConfigurationMutationResult::Changed);

    std::future<hcb::OAuthClientConfigurationReadResult> loaded = store.load();
    const hcb::OAuthClientConfigurationReadResult loadedResult = awaitResult(loaded);
    QVERIFY(std::holds_alternative<std::optional<hcb::OAuthClientConfiguration>>(loadedResult));
    const std::optional<hcb::OAuthClientConfiguration>& configuration =
        std::get<std::optional<hcb::OAuthClientConfiguration>>(loadedResult);
    QVERIFY(configuration.has_value());
    if (!configuration.has_value()) {
      return;
    }
    QCOMPARE(configuration->clientId, clientId());
    QCOMPARE(configuration->clientSecret, clientSecret());
    firstUpdatedAt = configuration->updatedAt;

    clock.advance(1h);
    std::future<hcb::OAuthClientConfigurationMutationResultOrError> unchanged =
        store.save(clientId(), clientSecret());
    const hcb::OAuthClientConfigurationMutationResultOrError unchangedResult =
        awaitResult(unchanged);
    QVERIFY(std::holds_alternative<hcb::OAuthClientConfigurationMutationResult>(unchangedResult));
    QCOMPARE(std::get<hcb::OAuthClientConfigurationMutationResult>(unchangedResult),
             hcb::OAuthClientConfigurationMutationResult::Unchanged);
  }

  hcb::OAuthClientConfigurationStore reopened(*databasePath, clock);
  verifyReady(reopened);
  std::future<hcb::OAuthClientConfigurationReadResult> persisted = reopened.load();
  const hcb::OAuthClientConfigurationReadResult persistedResult = awaitResult(persisted);
  QVERIFY(std::holds_alternative<std::optional<hcb::OAuthClientConfiguration>>(persistedResult));
  const std::optional<hcb::OAuthClientConfiguration>& configuration =
      std::get<std::optional<hcb::OAuthClientConfiguration>>(persistedResult);
  QVERIFY(configuration.has_value());
  if (!configuration.has_value()) {
    return;
  }
  QCOMPARE(configuration->clientId, clientId());
  QCOMPARE(configuration->clientSecret, clientSecret());
  QCOMPARE(configuration->updatedAt, firstUpdatedAt);

  std::future<hcb::OAuthClientConfigurationMutationResultOrError> cleared = reopened.clear();
  const hcb::OAuthClientConfigurationMutationResultOrError clearedResult = awaitResult(cleared);
  QVERIFY(std::holds_alternative<hcb::OAuthClientConfigurationMutationResult>(clearedResult));
  QCOMPARE(std::get<hcb::OAuthClientConfigurationMutationResult>(clearedResult),
           hcb::OAuthClientConfigurationMutationResult::Changed);

  std::future<hcb::OAuthClientConfigurationReadResult> empty = reopened.load();
  const hcb::OAuthClientConfigurationReadResult emptyResult = awaitResult(empty);
  QVERIFY(std::holds_alternative<std::optional<hcb::OAuthClientConfiguration>>(emptyResult));
  QVERIFY(!std::get<std::optional<hcb::OAuthClientConfiguration>>(emptyResult).has_value());

  std::future<hcb::OAuthClientConfigurationMutationResultOrError> alreadyClear = reopened.clear();
  const hcb::OAuthClientConfigurationMutationResultOrError alreadyClearResult =
      awaitResult(alreadyClear);
  QVERIFY(std::holds_alternative<hcb::OAuthClientConfigurationMutationResult>(alreadyClearResult));
  QCOMPARE(std::get<hcb::OAuthClientConfigurationMutationResult>(alreadyClearResult),
           hcb::OAuthClientConfigurationMutationResult::Unchanged);
}

void OAuthClientConfigurationStoreTest::rejectsInvalidClientIds() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  hcb::OAuthClientConfigurationStore store(*databasePath, clock);
  verifyReady(store);

  std::future<hcb::OAuthClientConfigurationMutationResultOrError> empty = store.save(QString());
  const hcb::OAuthClientConfigurationMutationResultOrError emptyResult = awaitResult(empty);
  QVERIFY(std::holds_alternative<hcb::AppError>(emptyResult));
  QCOMPARE(std::get<hcb::AppError>(emptyResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::OAuthClientConfigurationMutationResultOrError> tooShort =
      store.save(QStringLiteral("short-id"));
  const hcb::OAuthClientConfigurationMutationResultOrError tooShortResult = awaitResult(tooShort);
  QVERIFY(std::holds_alternative<hcb::AppError>(tooShortResult));
  QCOMPARE(std::get<hcb::AppError>(tooShortResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::OAuthClientConfigurationMutationResultOrError> tooLong =
      store.save(QString(501, u'x'));
  const hcb::OAuthClientConfigurationMutationResultOrError tooLongResult = awaitResult(tooLong);
  QVERIFY(std::holds_alternative<hcb::AppError>(tooLongResult));
  QCOMPARE(std::get<hcb::AppError>(tooLongResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::OAuthClientConfigurationReadResult> loaded = store.load();
  const hcb::OAuthClientConfigurationReadResult loadedResult = awaitResult(loaded);
  QVERIFY(std::holds_alternative<std::optional<hcb::OAuthClientConfiguration>>(loadedResult));
  QVERIFY(!std::get<std::optional<hcb::OAuthClientConfiguration>>(loadedResult).has_value());
}

QTEST_GUILESS_MAIN(OAuthClientConfigurationStoreTest)

#include "OAuthClientConfigurationStoreTest.moc"
