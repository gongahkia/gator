#include <QtTest/QTest>

#include "core/CalendarLayoutEngine.h"

class CalendarLayoutEngineTest final : public QObject {
  Q_OBJECT

private slots:
  void laysOutTimedOverlapClustersDeterministically();
  void laysOutAndClipsAllDayLanes();
};

void CalendarLayoutEngineTest::laysOutTimedOverlapClustersDeterministically() {
  const QList<hcb::CalendarTimedLayout> layouts = hcb::CalendarLayoutEngine::layoutTimed(
      {{.id = QStringLiteral("event-a"), .startMinute = 540, .endMinute = 660},
       {.id = QStringLiteral("event-b"), .startMinute = 570, .endMinute = 600},
       {.id = QStringLiteral("event-c"), .startMinute = 600, .endMinute = 630},
       {.id = QStringLiteral("event-d"), .startMinute = 720, .endMinute = 721}});
  QCOMPARE(layouts.size(), 4);
  QCOMPARE(layouts.at(0).id, QStringLiteral("event-a"));
  QCOMPARE(layouts.at(0).laneIndex, 0);
  QCOMPARE(layouts.at(0).laneCount, 2);
  QCOMPARE(layouts.at(1).id, QStringLiteral("event-b"));
  QCOMPARE(layouts.at(1).laneIndex, 1);
  QCOMPARE(layouts.at(2).id, QStringLiteral("event-c"));
  QCOMPARE(layouts.at(2).laneIndex, 1);
  QCOMPARE(layouts.at(2).laneCount, 2);
  QCOMPARE(layouts.at(3).laneCount, 1);
  QCOMPARE(layouts.at(3).durationMinutes, 5);
}

void CalendarLayoutEngineTest::laysOutAndClipsAllDayLanes() {
  const hcb::CalendarAllDayLayout layout = hcb::CalendarLayoutEngine::layoutAllDay(
      {{.id = QStringLiteral("event-wide"), .startDayIndex = -1, .endDayIndex = 2},
       {.id = QStringLiteral("event-overflow"), .startDayIndex = 0, .endDayIndex = 0},
       {.id = QStringLiteral("event-next"), .startDayIndex = 3, .endDayIndex = 4}},
      5,
      1);
  QCOMPARE(layout.segments.size(), 2);
  QCOMPARE(layout.segments.at(0).id, QStringLiteral("event-wide"));
  QCOMPARE(layout.segments.at(0).startDayIndex, 0);
  QCOMPARE(layout.segments.at(0).daySpan, 3);
  QVERIFY(layout.segments.at(0).startsBeforeRange);
  QCOMPARE(layout.segments.at(1).id, QStringLiteral("event-next"));
  QCOMPARE(layout.segments.at(1).laneIndex, 0);
  QCOMPARE(layout.overflowCounts, QList<int>({1, 0, 0, 0, 0}));
}

QTEST_GUILESS_MAIN(CalendarLayoutEngineTest)

#include "CalendarLayoutEngineTest.moc"
