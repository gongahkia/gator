#include <QtTest/QTest>

#include "core/AgendaModel.h"

class AgendaModelTest final : public QObject {
  Q_OBJECT

private slots:
  void exposesAgendaRolesAndResets();
  void rejectsInvalidIndexes();
};

void AgendaModelTest::exposesAgendaRolesAndResets() {
  hcb::AgendaModel model;
  model.setEvents({{.id = QStringLiteral("event-a"),
                    .calendarId = QStringLiteral("calendar-a"),
                    .recurringRemoteId = QStringLiteral("series-a"),
                    .originalStartAt = QStringLiteral("2026-07-25T09:00:00.000Z"),
                    .status = QStringLiteral("confirmed"),
                    .title = QStringLiteral("Review"),
                    .description = QStringLiteral("Plan release"),
                    .location = QStringLiteral("Room A"),
                    .startAt = QStringLiteral("2026-07-25T10:00:00.000Z"),
                    .startTimeZone = QStringLiteral("UTC"),
                    .endAt = QStringLiteral("2026-07-25T11:00:00.000Z"),
                    .endTimeZone = QStringLiteral("UTC"),
                    .allDay = false,
                    .colorId = QStringLiteral("7"),
                    .transparency = QStringLiteral("opaque"),
                    .visibility = QStringLiteral("private"),
                    .hcbKind = QStringLiteral("birthday"),
                    .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")}});

  QCOMPARE(model.rowCount(), 1);
  const QModelIndex index = model.index(0, 0);
  QCOMPARE(model.data(index, Qt::DisplayRole).toString(), QStringLiteral("Review"));
  QCOMPARE(model.data(index, hcb::AgendaModel::CalendarIdRole).toString(),
           QStringLiteral("calendar-a"));
  QCOMPARE(model.data(index, hcb::AgendaModel::RecurringRemoteIdRole).toString(),
           QStringLiteral("series-a"));
  QCOMPARE(model.data(index, hcb::AgendaModel::DescriptionRole).toString(),
           QStringLiteral("Plan release"));
  QCOMPARE(model.data(index, hcb::AgendaModel::StartAtRole).toString(),
           QStringLiteral("2026-07-25T10:00:00.000Z"));
  QCOMPARE(model.data(index, hcb::AgendaModel::AllDayRole).toBool(), false);
  QCOMPARE(model.data(index, hcb::AgendaModel::AgendaDayRole).toString(),
           QStringLiteral("2026-07-25"));
  QCOMPARE(model.data(index, hcb::AgendaModel::AgendaWeekRole).toString(),
           QStringLiteral("2026-07-20"));
  QCOMPARE(model.data(index, hcb::AgendaModel::HcbKindRole).toString(), QStringLiteral("birthday"));
  QCOMPARE(model.roleNames().value(hcb::AgendaModel::EndTimeZoneRole),
           QByteArrayLiteral("endTimeZone"));
  QCOMPARE(model.rowForEvent(QStringLiteral("event-a")), 0);
  QCOMPARE(model.eventForId(QStringLiteral("event-a")).value(QStringLiteral("title")).toString(),
           QStringLiteral("Review"));

  model.setEvents({});
  QCOMPARE(model.rowCount(), 0);
}

void AgendaModelTest::rejectsInvalidIndexes() {
  hcb::AgendaModel model;
  QVERIFY(!model.data(QModelIndex(), Qt::DisplayRole).isValid());
  QVERIFY(!model.data(model.index(0, 0), Qt::DisplayRole).isValid());
  QCOMPARE(model.rowCount(model.index(0, 0)), 0);
}

QTEST_GUILESS_MAIN(AgendaModelTest)

#include "AgendaModelTest.moc"
