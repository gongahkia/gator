#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include "core/Clock.h"
#include "core/SavedSearchStore.h"

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

using namespace std::chrono_literals;

class SavedSearchStoreTest final : public QObject {
  Q_OBJECT

private slots:
  void persistsLoadsAndValidatesSavedSearches();
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
    qFatal("saved-search request timed out");
  }
  return future.get();
}

void verifyReady(hcb::SettingsService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

} // namespace

void SavedSearchStoreTest::persistsLoadsAndValidatesSavedSearches() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  TestClock clock;
  {
    hcb::SettingsService settings(*databasePath, clock);
    hcb::SavedSearchStore store(settings);
    verifyReady(settings);
    std::future<hcb::SavedSearchListResult> empty = store.load();
    const hcb::SavedSearchListResult emptyResult = awaitResult(empty);
    QVERIFY(std::holds_alternative<QList<hcb::SavedSearch>>(emptyResult));
    QVERIFY(std::get<QList<hcb::SavedSearch>>(emptyResult).isEmpty());

    std::future<hcb::SavedSearchMutationResult> save = store.save(
        {{.id = QStringLiteral("release"),
          .name = QStringLiteral("Release"),
          .query = QStringLiteral("source:tasks status:open")}});
    const hcb::SavedSearchMutationResult saveResult = awaitResult(save);
    QVERIFY(std::holds_alternative<hcb::SettingsMutationResult>(saveResult));

    std::future<hcb::SavedSearchMutationResult> duplicate = store.save(
        {{.id = QStringLiteral("first"), .name = QStringLiteral("Release"), .query = QStringLiteral("a")},
         {.id = QStringLiteral("second"), .name = QStringLiteral("release"), .query = QStringLiteral("b")}});
    const hcb::SavedSearchMutationResult duplicateResult = awaitResult(duplicate);
    QVERIFY(std::holds_alternative<hcb::AppError>(duplicateResult));
    QCOMPARE(std::get<hcb::AppError>(duplicateResult).code(), hcb::AppErrorCode::Validation);
  }

  hcb::SettingsService reopenedSettings(*databasePath, clock);
  hcb::SavedSearchStore reopened(reopenedSettings);
  verifyReady(reopenedSettings);
  std::future<hcb::SavedSearchListResult> loaded = reopened.load();
  const hcb::SavedSearchListResult loadedResult = awaitResult(loaded);
  QVERIFY(std::holds_alternative<QList<hcb::SavedSearch>>(loadedResult));
  const QList<hcb::SavedSearch> searches = std::get<QList<hcb::SavedSearch>>(loadedResult);
  QCOMPARE(searches.size(), 1);
  QCOMPARE(searches.front().name, QStringLiteral("Release"));
  QCOMPARE(searches.front().query, QStringLiteral("source:tasks status:open"));
}

QTEST_GUILESS_MAIN(SavedSearchStoreTest)

#include "SavedSearchStoreTest.moc"
