#include <QtTest/QTest>

#include "core/ImportService.h"

class ImportServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void parsesDelimitedTasksAndEvents();
  void rejectsInvalidDelimitedLinesIndependently();
  void parsesVersionedCsvWithQuotedFields();
  void rejectsCsvSchemaAndUtf8Errors();
  void enforcesRecordAndCsvQuotingLimits();
};

void ImportServiceTest::parsesDelimitedTasksAndEvents() {
  const QByteArray input(
      "# HCB import\n"
      "task title=Report list=Work due=2026-08-01 priority=high rrule=FREQ=WEEKLY;BYDAY=MO,FR "
      "exclude=2026-08-03 include=2026-08-04\n"
      "event title=Planning calendar=Team start=2026-08-01T09:00:00.000Z "
      "end=2026-08-01T10:00:00.000Z all_day=false recurrence=\"RRULE:FREQ=WEEKLY\\nEXDATE:20260808T090000Z\"\n");
  const hcb::ImportParseResult result =
      hcb::ImportService::parse(hcb::ImportFormat::Delimited, input);
  QCOMPARE(result.items.size(), 2);
  QCOMPARE(result.rows.size(), 2);
  QVERIFY(result.rows.at(0).accepted);
  QCOMPARE(result.items.at(0).taskList, std::optional<QString>(QStringLiteral("Work")));
  QCOMPARE(result.items.at(0).taskRecurrenceRule,
           std::optional<QString>(QStringLiteral("FREQ=WEEKLY;BYDAY=MO,FR")));
  QCOMPARE(result.items.at(1).eventRecurrence,
           std::optional<QString>(QStringLiteral("RRULE:FREQ=WEEKLY\nEXDATE:20260808T090000Z")));
}

void ImportServiceTest::rejectsInvalidDelimitedLinesIndependently() {
  const hcb::ImportParseResult result = hcb::ImportService::parse(
      hcb::ImportFormat::Delimited,
      QByteArray("task title=valid\nevent title=no-end start=2026-08-01T09:00:00Z\n"
                 "task title=bad rrule=FREQ=DAILY until=2026-08-02 count=2\n"));
  QCOMPARE(result.items.size(), 1);
  QCOMPARE(result.rows.size(), 3);
  QVERIFY(result.rows.at(0).accepted);
  QVERIFY(!result.rows.at(1).accepted);
  QVERIFY(!result.rows.at(2).accepted);
}

void ImportServiceTest::parsesVersionedCsvWithQuotedFields() {
  const QByteArray input(
      "schema_version,kind,title,list,calendar,due,notes,priority,rrule,until,count,exclude,include,start,end,all_day,time_zone,description,location,recurrence\r\n"
      "1,task,Report,Work,,2026-08-01,\"first, second\",high,,,,,,,,,,,,\r\n"
      "1,event,Planning,,Team,,,,,,,,,2026-08-01T09:00:00.000Z,2026-08-01T10:00:00.000Z,false,Asia/Singapore,Desc,Room,\"RRULE:FREQ=WEEKLY\"\r\n");
  const hcb::ImportParseResult result = hcb::ImportService::parse(hcb::ImportFormat::Csv, input);
  QCOMPARE(result.items.size(), 2);
  QCOMPARE(result.items.at(0).taskNotes, std::optional<QString>(QStringLiteral("first, second")));
  QCOMPARE(result.items.at(1).eventTimeZone,
           std::optional<QString>(QStringLiteral("Asia/Singapore")));
}

void ImportServiceTest::rejectsCsvSchemaAndUtf8Errors() {
  const hcb::ImportParseResult header = hcb::ImportService::parse(
      hcb::ImportFormat::Csv, QByteArray("kind,title\ntask,Title\n"));
  QCOMPARE(header.items.size(), 0);
  QCOMPARE(header.rows.size(), 1);
  QVERIFY(!header.rows.front().accepted);

  const hcb::ImportParseResult utf8 =
      hcb::ImportService::parse(hcb::ImportFormat::Delimited, QByteArray("task title=\xff"));
  QCOMPARE(utf8.items.size(), 0);
  QCOMPARE(utf8.rows.size(), 1);
  QVERIFY(!utf8.rows.front().accepted);
}

void ImportServiceTest::enforcesRecordAndCsvQuotingLimits() {
  QByteArray records;
  for (int index = 0; index < 1'001; ++index) {
    records.append("task title=item\n");
  }
  const hcb::ImportParseResult oversized =
      hcb::ImportService::parse(hcb::ImportFormat::Delimited, records);
  QCOMPARE(oversized.items.size(), 0);
  QCOMPARE(oversized.rows.size(), 1);
  QVERIFY(oversized.rows.front().message.contains(QStringLiteral("1,000")));

  const hcb::ImportParseResult malformed = hcb::ImportService::parse(
      hcb::ImportFormat::Csv,
      QByteArray(
          "schema_version,kind,title,list,calendar,due,notes,priority,rrule,until,count,exclude,include,start,end,all_day,time_zone,description,location,recurrence\n"
          "1,task,\"title\"suffix,,,,,,,,,,,,,,,,,\n"));
  QCOMPARE(malformed.items.size(), 0);
  QCOMPARE(malformed.rows.size(), 1);
  QVERIFY(malformed.rows.front().message.contains(QStringLiteral("closing quote")));
}

QTEST_GUILESS_MAIN(ImportServiceTest)

#include "ImportServiceTest.moc"
