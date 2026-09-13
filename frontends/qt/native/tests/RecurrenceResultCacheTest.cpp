#include <QtTest/QTest>

#include <optional>

#include "core/RecurrenceResultCache.h"

class RecurrenceResultCacheTest final : public QObject {
  Q_OBJECT

private slots:
  void returnsIndependentCachedOccurrences();
  void evictsLeastRecentlyUsedAndInvalidatesSeries();
};

namespace {

hcb::RecurrenceExpansionRequest request(QString eventId, QString recurrenceRule = {}) {
  return {.eventId = std::move(eventId),
          .startAt = QStringLiteral("2026-07-25T09:00:00.000Z"),
          .endAt = QStringLiteral("2026-07-25T10:00:00.000Z"),
          .recurrenceRule = recurrenceRule.isEmpty()
                                ? std::nullopt
                                : std::optional<QString>(std::move(recurrenceRule))};
}

QList<hcb::RecurrenceOccurrence> occurrences(QString eventId) {
  return {{.id = std::move(eventId),
           .startAt = QStringLiteral("2026-07-25T09:00:00.000Z"),
           .endAt = QStringLiteral("2026-07-25T10:00:00.000Z"),
           .originalStartAt = QStringLiteral("2026-07-25T09:00:00.000Z")}};
}

} // namespace

void RecurrenceResultCacheTest::returnsIndependentCachedOccurrences() {
  hcb::RecurrenceResultCache cache(2);
  const hcb::RecurrenceExpansionRequest expansion =
      request(QStringLiteral("event-a"), QStringLiteral("RRULE:FREQ=DAILY;COUNT=2"));
  cache.store(expansion, occurrences(QStringLiteral("occurrence-a")));

  std::optional<QList<hcb::RecurrenceOccurrence>> first = cache.find(expansion);
  QVERIFY(first.has_value());
  if (!first.has_value()) {
    return;
  }
  QCOMPARE(first->size(), 1);
  first->front().id = QStringLiteral("mutated");

  const std::optional<QList<hcb::RecurrenceOccurrence>> second = cache.find(expansion);
  QVERIFY(second.has_value());
  if (!second.has_value()) {
    return;
  }
  QCOMPARE(second->front().id, QStringLiteral("occurrence-a"));
  QCOMPARE(cache.size(), std::size_t{1});

  hcb::RecurrenceExpansionRequest changedRule = expansion;
  changedRule.recurrenceRule = QStringLiteral("RRULE:FREQ=DAILY;COUNT=3");
  QVERIFY(!cache.find(changedRule).has_value());
}

void RecurrenceResultCacheTest::evictsLeastRecentlyUsedAndInvalidatesSeries() {
  hcb::RecurrenceResultCache cache(2);
  const hcb::RecurrenceExpansionRequest first = request(QStringLiteral("event-a"));
  const hcb::RecurrenceExpansionRequest second = request(QStringLiteral("event-b"));
  const hcb::RecurrenceExpansionRequest third = request(QStringLiteral("event-c"));
  cache.store(first, occurrences(QStringLiteral("occurrence-a")));
  cache.store(second, occurrences(QStringLiteral("occurrence-b")));
  QVERIFY(cache.find(first).has_value());
  cache.store(third, occurrences(QStringLiteral("occurrence-c")));

  QVERIFY(cache.find(first).has_value());
  QVERIFY(!cache.find(second).has_value());
  QVERIFY(cache.find(third).has_value());

  cache.invalidateEvent(QStringLiteral("event-a"));
  QVERIFY(!cache.find(first).has_value());
  QCOMPARE(cache.size(), std::size_t{1});
  cache.clear();
  QCOMPARE(cache.size(), std::size_t{0});
}

QTEST_GUILESS_MAIN(RecurrenceResultCacheTest)

#include "RecurrenceResultCacheTest.moc"
