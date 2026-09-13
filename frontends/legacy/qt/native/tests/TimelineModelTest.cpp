#include <QtTest/QTest>

#include "core/TimelineModel.h"
#include "core/TimelineViewportModel.h"

class TimelineModelTest final : public QObject {
  Q_OBJECT

private slots:
  void laysOutDayAndWeekItemsInDisplayTimeZone();
  void buildsMoveInputInDisplayTimeZone();
  void buildsResizeInputInDisplayTimeZone();
  void movesAndResizesAllDayEventsWithoutTimezoneShift();
  void mapsCoordinatesAcrossDstAndHalfHourZones();
  void normalizesMultiDayTimedRanges();
  void virtualizesDenseTimedRowsByViewportAndCalendar();
  void clearsInvalidRanges();
};

void TimelineModelTest::laysOutDayAndWeekItemsInDisplayTimeZone() {
  const QTimeZone timeZone(QByteArrayLiteral("America/Los_Angeles"));
  QVERIFY(timeZone.isValid());
  hcb::TimelineModel model;
  model.setRange(QDate(2026, 8, 1),
                 2,
                 {{.id = QStringLiteral("all-day"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("All day"),
                   .startAt = QStringLiteral("2026-08-01T07:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-04T07:00:00.000Z"),
                   .allDay = true,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")},
                  {.id = QStringLiteral("event-a"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("A"),
                   .startAt = QStringLiteral("2026-08-01T16:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-01T17:00:00.000Z"),
                   .allDay = false,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")},
                  {.id = QStringLiteral("event-b"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("B"),
                   .startAt = QStringLiteral("2026-08-01T16:30:00.000Z"),
                   .endAt = QStringLiteral("2026-08-01T17:30:00.000Z"),
                   .allDay = false,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")},
                  {.id = QStringLiteral("overnight"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Overnight"),
                   .startAt = QStringLiteral("2026-08-02T06:30:00.000Z"),
                   .endAt = QStringLiteral("2026-08-02T07:30:00.000Z"),
                   .allDay = false,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")}},
                 timeZone,
                 1);

  QCOMPARE(model.rowCount(), 5);
  QCOMPARE(model.data(model.index(0, 0), hcb::TimelineModel::IdRole).toString(),
           QStringLiteral("all-day"));
  QCOMPARE(model.data(model.index(0, 0), hcb::TimelineModel::AllDayRole).toBool(), true);
  QCOMPARE(model.data(model.index(0, 0), hcb::TimelineModel::DaySpanRole).toInt(), 2);
  QCOMPARE(model.data(model.index(0, 0), hcb::TimelineModel::EndsAfterRangeRole).toBool(), true);

  QCOMPARE(model.data(model.index(1, 0), hcb::TimelineModel::IdRole).toString(),
           QStringLiteral("event-a"));
  QCOMPARE(model.data(model.index(1, 0), hcb::TimelineModel::StartMinuteRole).toInt(), 9 * 60);
  QCOMPARE(model.data(model.index(1, 0), hcb::TimelineModel::LaneIndexRole).toInt(), 0);
  QCOMPARE(model.data(model.index(1, 0), hcb::TimelineModel::LaneCountRole).toInt(), 2);
  QCOMPARE(model.data(model.index(2, 0), hcb::TimelineModel::IdRole).toString(),
           QStringLiteral("event-b"));
  QCOMPARE(model.data(model.index(2, 0), hcb::TimelineModel::LaneIndexRole).toInt(), 1);

  QCOMPARE(model.data(model.index(3, 0), hcb::TimelineModel::IdRole).toString(),
           QStringLiteral("overnight"));
  QCOMPARE(model.data(model.index(3, 0), hcb::TimelineModel::DayIndexRole).toInt(), 0);
  QCOMPARE(model.data(model.index(3, 0), hcb::TimelineModel::DurationMinutesRole).toInt(), 30);
  QCOMPARE(model.data(model.index(4, 0), hcb::TimelineModel::DayIndexRole).toInt(), 1);
  QCOMPARE(model.data(model.index(4, 0), hcb::TimelineModel::StartMinuteRole).toInt(), 0);
  QCOMPARE(model.roleNames().value(hcb::TimelineModel::StartsBeforeRangeRole),
           QByteArrayLiteral("startsBeforeRange"));
}

void TimelineModelTest::buildsMoveInputInDisplayTimeZone() {
  const QTimeZone timeZone(QByteArrayLiteral("America/Los_Angeles"));
  QVERIFY(timeZone.isValid());
  hcb::TimelineModel model;
  model.setRange(QDate(2026, 3, 7),
                 2,
                 {{.id = QStringLiteral("event-a"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("A"),
                   .startAt = QStringLiteral("2026-03-07T18:00:00.000Z"),
                   .endAt = QStringLiteral("2026-03-07T19:00:00.000Z"),
                   .allDay = false,
                   .updatedAt = QStringLiteral("2026-03-01T00:00:00.000Z")}},
                 timeZone,
                 1);

  const QVariantMap move = model.moveInput(QStringLiteral("event-a"), 1, 3 * 60);
  QCOMPARE(move.value(QStringLiteral("id")).toString(), QStringLiteral("event-a"));
  QCOMPARE(move.value(QStringLiteral("startAt")).toString(),
           QStringLiteral("2026-03-08T10:00:00.000Z"));
  QCOMPARE(move.value(QStringLiteral("endAt")).toString(),
           QStringLiteral("2026-03-08T11:00:00.000Z"));
  QCOMPARE(move.value(QStringLiteral("allDay")).toBool(), false);
  QVERIFY(model.moveInput(QStringLiteral("event-a"), -1, 0).isEmpty());
  QVERIFY(model.moveInput(QStringLiteral("event-a"), 2, 0).isEmpty());
  QVERIFY(model.moveInput(QStringLiteral("event-a"), 0, 24 * 60).isEmpty());
  QVERIFY(model.moveInput(QStringLiteral("missing"), 0, 0).isEmpty());
}

void TimelineModelTest::buildsResizeInputInDisplayTimeZone() {
  const QTimeZone timeZone(QByteArrayLiteral("America/Los_Angeles"));
  QVERIFY(timeZone.isValid());
  hcb::TimelineModel model;
  model.setRange(QDate(2026, 8, 1),
                 1,
                 {{.id = QStringLiteral("event-a"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("A"),
                   .startAt = QStringLiteral("2026-08-01T16:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-01T17:00:00.000Z"),
                   .allDay = false,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")}},
                 timeZone,
                 1);

  const QVariantMap resize = model.resizeInput(QStringLiteral("event-a"), 0, 12 * 60);
  QCOMPARE(resize.value(QStringLiteral("id")).toString(), QStringLiteral("event-a"));
  QCOMPARE(resize.value(QStringLiteral("endAt")).toString(),
           QStringLiteral("2026-08-01T19:00:00.000Z"));
  QCOMPARE(model.resizeInput(QStringLiteral("event-a"), 0, 24 * 60)
               .value(QStringLiteral("endAt"))
               .toString(),
           QStringLiteral("2026-08-02T07:00:00.000Z"));
  QVERIFY(model.resizeInput(QStringLiteral("event-a"), 0, 9 * 60).isEmpty());
  QVERIFY(model.resizeInput(QStringLiteral("event-a"), 0, 24 * 60 + 1).isEmpty());
  QVERIFY(model.resizeInput(QStringLiteral("missing"), 0, 12 * 60).isEmpty());
}

void TimelineModelTest::movesAndResizesAllDayEventsWithoutTimezoneShift() {
  const QTimeZone timeZone(QByteArrayLiteral("America/Los_Angeles"));
  QVERIFY(timeZone.isValid());
  hcb::TimelineModel model;
  model.setRange(QDate(2026, 8, 1),
                 7,
                 {{.id = QStringLiteral("all-day"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Offsite"),
                   .startAt = QStringLiteral("2026-08-01T00:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-03T00:00:00.000Z"),
                   .allDay = true,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")}},
                 timeZone,
                 2);

  QCOMPARE(model.rowCount(), 1);
  QCOMPARE(model.data(model.index(0, 0), hcb::TimelineModel::DayIndexRole).toInt(), 0);
  QCOMPARE(model.data(model.index(0, 0), hcb::TimelineModel::DaySpanRole).toInt(), 2);
  const QVariantMap move = model.moveAllDayInput(QStringLiteral("all-day"), 3);
  QCOMPARE(move.value(QStringLiteral("startAt")).toString(),
           QStringLiteral("2026-08-04T00:00:00.000Z"));
  QCOMPARE(move.value(QStringLiteral("endAt")).toString(),
           QStringLiteral("2026-08-06T00:00:00.000Z"));
  QCOMPARE(move.value(QStringLiteral("allDay")).toBool(), true);
  QCOMPARE(model.resizeAllDayInput(QStringLiteral("all-day"), 0)
               .value(QStringLiteral("endAt"))
               .toString(),
           QStringLiteral("2026-08-02T00:00:00.000Z"));
  const QVariantMap resized =
      model.resizeAllDayRangeInput(QStringLiteral("all-day"), 1, 4);
  QCOMPARE(resized.value(QStringLiteral("startAt")).toString(),
           QStringLiteral("2026-08-02T00:00:00.000Z"));
  QCOMPARE(resized.value(QStringLiteral("endAt")).toString(),
           QStringLiteral("2026-08-06T00:00:00.000Z"));
  QVERIFY(model.resizeAllDayRangeInput(QStringLiteral("all-day"), 4, 1).isEmpty());
}

void TimelineModelTest::mapsCoordinatesAcrossDstAndHalfHourZones() {
  const QList<QByteArray> zones{QByteArrayLiteral("UTC"), QByteArrayLiteral("Asia/Singapore"),
                                QByteArrayLiteral("Asia/Kolkata"), QByteArrayLiteral("Australia/Adelaide"),
                                QByteArrayLiteral("America/Los_Angeles")};
  for (const QByteArray& zoneId : zones) {
    const QTimeZone zone(zoneId);
    QVERIFY2(zone.isValid(), zoneId.constData());
    hcb::TimelineModel model;
    model.setRange(QDate(2026, 3, 8), 2, {}, zone, 1);
    const QVariantMap point = model.timelinePointInput(400.0, 150.0, 724.0, 24.0, 60.0);
    QCOMPARE(point.value(QStringLiteral("dayIndex")).toInt(), 1);
    QCOMPARE(point.value(QStringLiteral("minute")).toInt(), 150);
    const QVariantMap timed = model.timedRangeInput(0, 150, 0, 210);
    QVERIFY(!timed.isEmpty());
    const QDateTime expectedStart(QDate(2026, 3, 8), QTime(2, 30), zone,
                                  QDateTime::TransitionResolution::PreferAfter);
    QCOMPARE(timed.value(QStringLiteral("startAt")).toString(),
             expectedStart.toUTC().toString(Qt::ISODateWithMs));
    QVERIFY(QDateTime::fromString(timed.value(QStringLiteral("endAt")).toString(), Qt::ISODateWithMs) >
            QDateTime::fromString(timed.value(QStringLiteral("startAt")).toString(), Qt::ISODateWithMs));
    QCOMPARE(model.dateForDayIndex(1), QStringLiteral("2026-03-09"));
    QCOMPARE(model.dayIndexForDate(QStringLiteral("2026-03-09")), 1);
    const QVariantMap allDay = model.allDayRangeInput(0, 1);
    QCOMPARE(allDay.value(QStringLiteral("startAt")).toString(),
             QStringLiteral("2026-03-08T00:00:00.000Z"));
    QCOMPARE(allDay.value(QStringLiteral("endAt")).toString(),
             QStringLiteral("2026-03-10T00:00:00.000Z"));
  }
}

void TimelineModelTest::normalizesMultiDayTimedRanges() {
  const QTimeZone timeZone(QByteArrayLiteral("Asia/Singapore"));
  QVERIFY(timeZone.isValid());
  hcb::TimelineModel model;
  model.setRange(QDate(2026, 8, 2), 7, {}, timeZone, 1);

  const QVariantMap forward = model.timedRangeInput(1, 10 * 60, 3, 12 * 60);
  QCOMPARE(forward.value(QStringLiteral("startAt")).toString(),
           QStringLiteral("2026-08-03T02:00:00.000Z"));
  QCOMPARE(forward.value(QStringLiteral("endAt")).toString(),
           QStringLiteral("2026-08-05T04:00:00.000Z"));

  const QVariantMap reverse = model.timedRangeInput(3, 12 * 60, 1, 10 * 60);
  QCOMPARE(reverse, forward);

  const QVariantMap reverseSameDay = model.timedRangeInput(2, 12 * 60, 2, 10 * 60);
  QCOMPARE(reverseSameDay.value(QStringLiteral("startAt")).toString(),
           QStringLiteral("2026-08-04T02:00:00.000Z"));
  QCOMPARE(reverseSameDay.value(QStringLiteral("endAt")).toString(),
           QStringLiteral("2026-08-04T04:00:00.000Z"));
}

void TimelineModelTest::virtualizesDenseTimedRowsByViewportAndCalendar() {
  hcb::TimelineModel model;
  const QDate startDate(2026, 8, 2);
  QList<hcb::CalendarEventSummary> events;
  events.reserve(25'000);
  for (int index = 0; index < 25'000; ++index) {
    const int dayIndex = index % 7;
    const int startMinute = index / 7 % (24 * 60);
    const QDateTime startsAt(startDate.addDays(dayIndex),
                             QTime(startMinute / 60, startMinute % 60),
                             QTimeZone::utc());
    events.append({.id = QStringLiteral("event-%1").arg(index),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Event %1").arg(index),
                   .startAt = startsAt.toString(Qt::ISODateWithMs),
                   .endAt = startsAt.addSecs(60).toString(Qt::ISODateWithMs),
                   .allDay = false});
  }
  model.setRange(startDate, 7, events, QTimeZone::utc(), 2);
  QCOMPARE(model.totalItemCount(), 25'000);
  QCOMPARE(model.rowCount(), 25'000);

  hcb::TimelineViewportModel dayViewport;
  dayViewport.setSourceModel(&model);
  dayViewport.setFirstDayIndex(0);
  dayViewport.setDayCount(1);
  dayViewport.setVisibleStartMinute(9 * 60);
  dayViewport.setVisibleEndMinute(10 * 60);
  QCOMPARE(dayViewport.rowCount(), 192);

  hcb::TimelineViewportModel weekViewport;
  weekViewport.setSourceModel(&model);
  weekViewport.setFirstDayIndex(0);
  weekViewport.setDayCount(7);
  weekViewport.setVisibleStartMinute(9 * 60);
  weekViewport.setVisibleEndMinute(10 * 60);
  QCOMPARE(weekViewport.rowCount(), 1'344);

  dayViewport.setFilterCalendarVisibility(true);
  QCOMPARE(dayViewport.rowCount(), 0);
  dayViewport.setVisibleCalendarIds({QStringLiteral("calendar-a")});
  QCOMPARE(dayViewport.rowCount(), 192);
}

void TimelineModelTest::clearsInvalidRanges() {
  hcb::TimelineModel model;
  model.setRange(QDate(2026, 8, 1), 8, {}, QTimeZone::utc(), 1);
  QCOMPARE(model.rowCount(), 0);
  QVERIFY(!model.data(model.index(0, 0), Qt::DisplayRole).isValid());
}

QTEST_GUILESS_MAIN(TimelineModelTest)

#include "TimelineModelTest.moc"
