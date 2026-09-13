#include <QtTest/QTest>
#include <QtTest/QSignalSpy>

#include "core/CalendarSourceModel.h"

class CalendarSourceModelTest final : public QObject {
  Q_OBJECT

private slots:
  void exposesCalendarSourceRolesAndResets();
  void rejectsInvalidIndexes();
};

void CalendarSourceModelTest::exposesCalendarSourceRolesAndResets() {
  hcb::CalendarSourceModel model;
  QSignalSpy revisions(&model, &hcb::CalendarSourceModel::revisionChanged);
  model.setCalendars({{.id = QStringLiteral("calendar-a"),
                       .accountId = QStringLiteral("account-a"),
                       .remoteId = QStringLiteral("remote-a"),
                       .title = QStringLiteral("Work"),
                       .description = QStringLiteral("Projects"),
                       .timeZone = QStringLiteral("Asia/Singapore"),
                       .backgroundColor = QStringLiteral("#0b57d0"),
                       .foregroundColor = QStringLiteral("#ffffff"),
                       .accessRole = QStringLiteral("owner"),
                       .selected = true,
                       .primary = true,
                       .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z"),
                       .eventCount = 4}});

  QCOMPARE(model.rowCount(), 1);
  const QModelIndex index = model.index(0, 0);
  QCOMPARE(model.data(index, Qt::DisplayRole).toString(), QStringLiteral("Work"));
  QCOMPARE(model.data(index, hcb::CalendarSourceModel::IdRole).toString(),
           QStringLiteral("calendar-a"));
  QCOMPARE(model.data(index, hcb::CalendarSourceModel::DescriptionRole).toString(),
           QStringLiteral("Projects"));
  QCOMPARE(model.data(index, hcb::CalendarSourceModel::TimeZoneRole).toString(),
           QStringLiteral("Asia/Singapore"));
  QCOMPARE(model.data(index, hcb::CalendarSourceModel::BackgroundColorRole).toString(),
           QStringLiteral("#0b57d0"));
  QCOMPARE(model.data(index, hcb::CalendarSourceModel::AccessRoleRole).toString(),
           QStringLiteral("owner"));
  QCOMPARE(model.data(index, hcb::CalendarSourceModel::SelectedRole).toBool(), true);
  QCOMPARE(model.data(index, hcb::CalendarSourceModel::PrimaryRole).toBool(), true);
  QCOMPARE(model.data(index, hcb::CalendarSourceModel::EventCountRole).toLongLong(), qint64{4});
  QCOMPARE(model.roleNames().value(hcb::CalendarSourceModel::ForegroundColorRole),
           QByteArrayLiteral("foregroundColor"));
  QCOMPARE(model.revision(), 1);
  QCOMPARE(revisions.count(), 1);
  QCOMPARE(model.calendarIds(), QStringList({QStringLiteral("calendar-a")}));
  QCOMPARE(model.selectedCalendarIds(), QStringList({QStringLiteral("calendar-a")}));
  QCOMPARE(model.calendarBackgroundColor(QStringLiteral("calendar-a")), QStringLiteral("#0b57d0"));
  QCOMPARE(model.calendarBackgroundColor(QStringLiteral("missing")), QString());

  model.setCalendars({});
  QCOMPARE(model.rowCount(), 0);
  QCOMPARE(model.revision(), 2);
  QCOMPARE(revisions.count(), 2);
  QCOMPARE(model.calendarIds(), QStringList());
  QCOMPARE(model.selectedCalendarIds(), QStringList());
}

void CalendarSourceModelTest::rejectsInvalidIndexes() {
  hcb::CalendarSourceModel model;
  QVERIFY(!model.data(QModelIndex(), Qt::DisplayRole).isValid());
  QVERIFY(!model.data(model.index(0, 0), Qt::DisplayRole).isValid());
  QCOMPARE(model.rowCount(model.index(0, 0)), 0);
}

QTEST_GUILESS_MAIN(CalendarSourceModelTest)

#include "CalendarSourceModelTest.moc"
