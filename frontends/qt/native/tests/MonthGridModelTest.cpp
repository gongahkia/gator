#include <QtTest/QTest>

#include "core/MonthGridModel.h"

class MonthGridModelTest final : public QObject {
  Q_OBJECT

private slots:
  void buildsSundayFirstGridAndAssignsEndExclusiveEventSpans();
  void keepsAllDayDatesStableAcrossDisplayTimeZones();
  void createsWeeklySegmentsForMultiWeekAllDayEvents();
  void indexesVisibleMonthPresentationByCalendarAndLane();
  void mapsGridCoordinatesAndMovesMultiDayEventsInDisplayTimeZone();
  void clearsForInvalidMonthOrTimeZone();
};

void MonthGridModelTest::buildsSundayFirstGridAndAssignsEndExclusiveEventSpans() {
  hcb::MonthGridModel model;
  model.setMonth(QDate(2026, 8, 1),
                 {{.id = QStringLiteral("multi"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Multi day"),
                   .startAt = QStringLiteral("2026-08-30T00:00:00.000Z"),
                   .endAt = QStringLiteral("2026-09-02T00:00:00.000Z"),
                   .allDay = true,
                   .colorId = QStringLiteral("7"),
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")},
                  {.id = QStringLiteral("overnight"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Overnight"),
                   .startAt = QStringLiteral("2026-08-31T23:00:00.000Z"),
                   .endAt = QStringLiteral("2026-09-01T00:00:00.000Z"),
                   .allDay = false,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")}},
                 QTimeZone::utc());

  QCOMPARE(model.rowCount(), 6);
  QCOMPARE(model.columnCount(), 7);
  const QModelIndex first = model.index(0, 0);
  QCOMPARE(model.data(first, hcb::MonthGridModel::DateRole).toString(),
           QStringLiteral("2026-07-26"));
  QCOMPARE(model.data(first, hcb::MonthGridModel::OutsideMonthRole).toBool(), true);

  const QModelIndex august31 = model.index(5, 1);
  QCOMPARE(model.data(august31, hcb::MonthGridModel::DateRole).toString(),
           QStringLiteral("2026-08-31"));
  QCOMPARE(model.data(august31, hcb::MonthGridModel::EventCountRole).toInt(), 2);
  const QVariantList august31Events =
      model.data(august31, hcb::MonthGridModel::EventsRole).toList();
  QCOMPARE(august31Events.size(), 2);
  QCOMPARE(august31Events.at(0).toMap().value(QStringLiteral("id")).toString(),
           QStringLiteral("multi"));
  QCOMPARE(august31Events.at(1).toMap().value(QStringLiteral("id")).toString(),
           QStringLiteral("overnight"));

  const QModelIndex september1 = model.index(5, 2);
  QCOMPARE(model.data(september1, hcb::MonthGridModel::OutsideMonthRole).toBool(), true);
  QCOMPARE(model.data(september1, hcb::MonthGridModel::EventCountRole).toInt(), 1);
  QCOMPARE(model.data(september1, hcb::MonthGridModel::EventsRole)
               .toList()
               .at(0)
               .toMap()
               .value(QStringLiteral("id"))
               .toString(),
           QStringLiteral("multi"));
  QCOMPARE(model.roleNames().value(hcb::MonthGridModel::OutsideMonthRole),
           QByteArrayLiteral("outsideMonth"));
}

void MonthGridModelTest::keepsAllDayDatesStableAcrossDisplayTimeZones() {
  const QTimeZone timeZone(QByteArrayLiteral("America/Los_Angeles"));
  QVERIFY(timeZone.isValid());
  hcb::MonthGridModel model;
  model.setMonth(QDate(2026, 8, 1),
                 {{.id = QStringLiteral("all-day"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Offsite"),
                   .startAt = QStringLiteral("2026-08-01T00:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-02T00:00:00.000Z"),
                   .allDay = true,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")}},
                 timeZone);

  const QModelIndex august1 = model.index(0, 6);
  QCOMPARE(model.data(august1, hcb::MonthGridModel::DateRole).toString(),
           QStringLiteral("2026-08-01"));
  QCOMPARE(model.data(august1, hcb::MonthGridModel::EventCountRole).toInt(), 1);
}

void MonthGridModelTest::createsWeeklySegmentsForMultiWeekAllDayEvents() {
  hcb::MonthGridModel model;
  model.setMonth(QDate(2026, 8, 1),
                 {{.id = QStringLiteral("span"),
                   .calendarId = QStringLiteral("calendar-a"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Trip"),
                   .startAt = QStringLiteral("2026-08-01T00:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-12T00:00:00.000Z"),
                   .allDay = true,
                   .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")}},
                 QTimeZone::utc());

  const QVariantList spans = model.allDaySpans();
  QCOMPARE(spans.size(), 3);
  QCOMPARE(spans.at(0).toMap().value(QStringLiteral("daySpan")).toInt(), 1);
  QCOMPARE(spans.at(1).toMap().value(QStringLiteral("daySpan")).toInt(), 7);
  QCOMPARE(spans.at(2).toMap().value(QStringLiteral("daySpan")).toInt(), 3);
  QCOMPARE(spans.at(1).toMap().value(QStringLiteral("startsBeforeRange")).toBool(), true);
  QCOMPARE(spans.at(1).toMap().value(QStringLiteral("endsAfterRange")).toBool(), true);
}

void MonthGridModelTest::indexesVisibleMonthPresentationByCalendarAndLane() {
  hcb::MonthGridModel model;
  model.setMonth(QDate(2026, 8, 1),
                 {{.id = QStringLiteral("visible-all-day"),
                   .calendarId = QStringLiteral("calendar-visible"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Visible all-day"),
                   .startAt = QStringLiteral("2026-08-05T00:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-06T00:00:00.000Z"),
                   .allDay = true},
                  {.id = QStringLiteral("hidden-all-day"),
                   .calendarId = QStringLiteral("calendar-hidden"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Hidden all-day"),
                   .startAt = QStringLiteral("2026-08-05T00:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-06T00:00:00.000Z"),
                   .allDay = true},
                  {.id = QStringLiteral("visible-timed"),
                   .calendarId = QStringLiteral("calendar-visible"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Visible timed"),
                   .startAt = QStringLiteral("2026-08-05T10:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-05T11:00:00.000Z"),
                   .allDay = false},
                  {.id = QStringLiteral("hidden-timed"),
                   .calendarId = QStringLiteral("calendar-hidden"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Hidden timed"),
                   .startAt = QStringLiteral("2026-08-05T12:00:00.000Z"),
                   .endAt = QStringLiteral("2026-08-05T13:00:00.000Z"),
                   .allDay = false}},
                 QTimeZone::utc());
  model.setVisibleCalendarIds({QStringLiteral("calendar-visible")});
  model.setVisibleAllDayLanes(0);

  const QModelIndex august5 = model.index(1, 3);
  const QVariantList timed =
      model.data(august5, hcb::MonthGridModel::VisibleTimedEventsRole).toList();
  QCOMPARE(timed.size(), 1);
  QCOMPARE(timed.front().toMap().value(QStringLiteral("id")).toString(),
           QStringLiteral("visible-timed"));
  QCOMPARE(model.data(august5, hcb::MonthGridModel::HiddenAllDayCountRole).toInt(), 1);
  QVERIFY(model.visibleAllDaySpans().isEmpty());

  model.setVisibleAllDayLanes(1);
  QCOMPARE(model.visibleAllDaySpans().size(), 1);
  const QVariantList allDay =
      model.data(august5, hcb::MonthGridModel::VisibleAllDayEventsRole).toList();
  QCOMPARE(allDay.size(), 1);
  QCOMPARE(allDay.front().toMap().value(QStringLiteral("id")).toString(),
           QStringLiteral("visible-all-day"));
}

void MonthGridModelTest::mapsGridCoordinatesAndMovesMultiDayEventsInDisplayTimeZone() {
  const QTimeZone zone(QByteArrayLiteral("Australia/Adelaide"));
  QVERIFY(zone.isValid());
  hcb::MonthGridModel model;
  model.setMonth(QDate(2026, 10, 1), {}, zone);
  QCOMPARE(model.dateForPoint(0.0, 0.0, 700.0, 600.0), QStringLiteral("2026-09-27"));
  QCOMPARE(model.dateForPoint(699.0, 599.0, 700.0, 600.0), QStringLiteral("2026-11-07"));
  QCOMPARE(model.dateIndex(QStringLiteral("2026-10-04")), 7);
  QCOMPARE(model.dateForIndex(7), QStringLiteral("2026-10-04"));
  QCOMPARE(model.dateForIndex(-1), QString());
  const QVariantMap created = model.allDayRangeInput(7, 9);
  QCOMPARE(created.value(QStringLiteral("startAt")).toString(),
           QStringLiteral("2026-10-04T00:00:00.000Z"));
  QCOMPARE(created.value(QStringLiteral("endAt")).toString(),
           QStringLiteral("2026-10-07T00:00:00.000Z"));
  const QVariantMap moved = model.moveInput(
      {{QStringLiteral("id"), QStringLiteral("event-a")},
       {QStringLiteral("allDay"), false},
       {QStringLiteral("startAt"), QStringLiteral("2026-10-03T13:30:00.000Z")},
       {QStringLiteral("endAt"), QStringLiteral("2026-10-03T14:30:00.000Z")}},
      7);
  QVERIFY(!moved.isEmpty());
  const QDateTime expected(QDate(2026, 10, 4), QTime(23, 0), zone,
                           QDateTime::TransitionResolution::PreferAfter);
  QCOMPARE(moved.value(QStringLiteral("startAt")).toString(),
           expected.toUTC().toString(Qt::ISODateWithMs));
  const QVariantMap resized = model.resizeAllDayRangeInput(
      {{QStringLiteral("id"), QStringLiteral("event-a")}, {QStringLiteral("allDay"), true}}, 8, 10);
  QCOMPARE(resized.value(QStringLiteral("startAt")).toString(),
           QStringLiteral("2026-10-05T00:00:00.000Z"));
  QCOMPARE(resized.value(QStringLiteral("endAt")).toString(),
           QStringLiteral("2026-10-08T00:00:00.000Z"));
}

void MonthGridModelTest::clearsForInvalidMonthOrTimeZone() {
  hcb::MonthGridModel model;
  model.setMonth(QDate(), {}, QTimeZone::utc());
  QCOMPARE(model.rowCount(), 0);
  QCOMPARE(model.columnCount(), 7);
  QVERIFY(!model.data(model.index(0, 0), Qt::DisplayRole).isValid());

  model.setMonth(QDate(2026, 8, 1), {}, QTimeZone());
  QCOMPARE(model.rowCount(), 0);
}

QTEST_GUILESS_MAIN(MonthGridModelTest)

#include "MonthGridModelTest.moc"
