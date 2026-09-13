#include <QtTest/QTest>

#include "core/TaskRecurrenceMarker.h"

class TaskRecurrenceMarkerTest final : public QObject {
  Q_OBJECT

private slots:
  void roundTripsWithoutChangingUserNotes();
  void rejectsMalformedAndUnknownMarkersWithoutDiscardingNotes();
  void enforcesGoogleNotesLimit();
  void advancesDatesFromTheAnchorAcrossDstAndMonthEnds();
  void expandsDateOnlyWeekdayOrdinalAndExceptionRules();
};

namespace {

[[nodiscard]] hcb::TaskRecurrenceMarker marker() {
  return {
      .seriesId = QStringLiteral("b5c71e7f-2cf6-4f49-9bcd-d46c56574492"),
      .occurrenceId = QStringLiteral("b5c71e7f-2cf6-4f49-9bcd-d46c56574492:3"),
      .ordinal = 3,
      .frequency = hcb::TaskRecurrenceFrequency::Weekly,
      .interval = 2,
      .anchorDate = QStringLiteral("2026-07-26"),
      .timeZone = QStringLiteral("Asia/Singapore"),
      .end = {.kind = hcb::TaskRecurrenceEndKind::Until, .untilDate = QStringLiteral("2026-12-31")},
      .templateTitle = QStringLiteral("Pay rent"),
      .templateDueDate = QStringLiteral("2026-07-26"),
      .templatePriority = QStringLiteral("high")};
}

} // namespace

void TaskRecurrenceMarkerTest::roundTripsWithoutChangingUserNotes() {
  const QString body = QStringLiteral("First line\n\nKeep every byte.\n");
  const hcb::TaskRecurrenceSerializationResult serialized =
      hcb::serializeTaskRecurrenceNotes(body, marker());
  QVERIFY(!serialized.error.has_value());
  QVERIFY(serialized.notes.contains(QStringLiteral("[HCB-RECURRENCE v2]")));

  const hcb::TaskRecurrenceNotes parsed = hcb::parseTaskRecurrenceNotes(serialized.notes);
  QCOMPARE(parsed.state, hcb::TaskRecurrenceNotesState::Managed);
  QCOMPARE(parsed.userNotes, body);
  QVERIFY(parsed.marker.has_value());
  if (!parsed.marker.has_value()) {
    return;
  }
  QCOMPARE(parsed.marker->seriesId, marker().seriesId);
  QCOMPARE(parsed.marker->occurrenceId, marker().occurrenceId);
  QCOMPARE(parsed.marker->ordinal, 3);
  QCOMPARE(hcb::taskRecurrenceSummary(*parsed.marker),
           QStringLiteral("Every 2 weeks until 2026-12-31"));

  const hcb::TaskRecurrenceSerializationResult repeated =
      hcb::serializeTaskRecurrenceNotes(parsed.userNotes, *parsed.marker);
  QVERIFY(!repeated.error.has_value());
  QCOMPARE(repeated.notes, serialized.notes);
}

void TaskRecurrenceMarkerTest::rejectsMalformedAndUnknownMarkersWithoutDiscardingNotes() {
  const QString body = QStringLiteral("User authored notes");
  const hcb::TaskRecurrenceSerializationResult serialized =
      hcb::serializeTaskRecurrenceNotes(body, marker());
  QVERIFY(!serialized.error.has_value());

  const QString malformed = serialized.notes + QStringLiteral("\nextra");
  const hcb::TaskRecurrenceNotes malformedParsed = hcb::parseTaskRecurrenceNotes(malformed);
  QCOMPARE(malformedParsed.state, hcb::TaskRecurrenceNotesState::Malformed);
  QCOMPARE(malformedParsed.userNotes, malformed);
  QVERIFY(!malformedParsed.diagnostic.isEmpty());

  QString unsupported = serialized.notes;
  unsupported.replace(QStringLiteral("[HCB-RECURRENCE v2]"), QStringLiteral("[HCB-RECURRENCE v3]"));
  const hcb::TaskRecurrenceNotes unsupportedParsed = hcb::parseTaskRecurrenceNotes(unsupported);
  QCOMPARE(unsupportedParsed.state, hcb::TaskRecurrenceNotesState::UnsupportedVersion);
  QCOMPARE(unsupportedParsed.userNotes, unsupported);
}

void TaskRecurrenceMarkerTest::enforcesGoogleNotesLimit() {
  const QString body(8'192, u'x');
  const hcb::TaskRecurrenceSerializationResult serialized =
      hcb::serializeTaskRecurrenceNotes(body, marker());
  QVERIFY(serialized.error.has_value());
  QVERIFY(serialized.notes.isEmpty());

  hcb::TaskRecurrenceMarker invalid = marker();
  invalid.occurrenceId = QStringLiteral("not-derived");
  const hcb::TaskRecurrenceSerializationResult invalidSerialized =
      hcb::serializeTaskRecurrenceNotes(QString(), invalid);
  QVERIFY(invalidSerialized.error.has_value());
}

void TaskRecurrenceMarkerTest::advancesDatesFromTheAnchorAcrossDstAndMonthEnds() {
  hcb::TaskRecurrenceMarker weekly = marker();
  weekly.interval = 1;
  weekly.anchorDate = QStringLiteral("2026-03-08");
  weekly.templateDueDate = weekly.anchorDate;
  weekly.timeZone = QStringLiteral("America/New_York");
  weekly.ordinal = 0;
  weekly.occurrenceId = weekly.seriesId + QStringLiteral(":0");
  const std::optional<hcb::TaskRecurrenceMarker> weeklySuccessor =
      hcb::taskRecurrenceSuccessor(weekly);
  QVERIFY(weeklySuccessor.has_value());
  if (!weeklySuccessor.has_value()) {
    return;
  }
  QCOMPARE(weeklySuccessor->templateDueDate, QStringLiteral("2026-03-15"));

  hcb::TaskRecurrenceMarker monthly = marker();
  monthly.frequency = hcb::TaskRecurrenceFrequency::Monthly;
  monthly.interval = 1;
  monthly.anchorDate = QStringLiteral("2024-01-31");
  monthly.templateDueDate = monthly.anchorDate;
  monthly.end = {.kind = hcb::TaskRecurrenceEndKind::Count, .count = 3};
  monthly.ordinal = 0;
  monthly.occurrenceId = monthly.seriesId + QStringLiteral(":0");
  const std::optional<hcb::TaskRecurrenceMarker> february = hcb::taskRecurrenceSuccessor(monthly);
  QVERIFY(february.has_value());
  if (!february.has_value()) {
    return;
  }
  QCOMPARE(february->templateDueDate, QStringLiteral("2024-02-29"));
  const std::optional<hcb::TaskRecurrenceMarker> march = hcb::taskRecurrenceSuccessor(*february);
  QVERIFY(march.has_value());
  if (!march.has_value()) {
    return;
  }
  QCOMPARE(march->templateDueDate, QStringLiteral("2024-03-31"));
  QVERIFY(!hcb::taskRecurrenceSuccessor(*march).has_value());

  hcb::TaskRecurrenceMarker daily = marker();
  daily.frequency = hcb::TaskRecurrenceFrequency::Daily;
  daily.interval = 3;
  daily.anchorDate = QStringLiteral("2026-07-26");
  daily.templateDueDate = daily.anchorDate;
  daily.end = {.kind = hcb::TaskRecurrenceEndKind::Until,
               .untilDate = QStringLiteral("2026-07-31")};
  daily.ordinal = 0;
  daily.occurrenceId = daily.seriesId + QStringLiteral(":0");
  const std::optional<hcb::TaskRecurrenceMarker> dailySuccessor =
      hcb::taskRecurrenceSuccessor(daily);
  QVERIFY(dailySuccessor.has_value());
  if (!dailySuccessor.has_value()) {
    return;
  }
  QCOMPARE(dailySuccessor->templateDueDate, QStringLiteral("2026-07-29"));
  QVERIFY(!hcb::taskRecurrenceSuccessor(*dailySuccessor).has_value());

  hcb::TaskRecurrenceMarker yearly = marker();
  yearly.frequency = hcb::TaskRecurrenceFrequency::Yearly;
  yearly.interval = 1;
  yearly.anchorDate = QStringLiteral("2024-02-29");
  yearly.templateDueDate = yearly.anchorDate;
  yearly.end = {.kind = hcb::TaskRecurrenceEndKind::Never};
  yearly.ordinal = 0;
  yearly.occurrenceId = yearly.seriesId + QStringLiteral(":0");
  const std::optional<hcb::TaskRecurrenceMarker> yearlySuccessor =
      hcb::taskRecurrenceSuccessor(yearly);
  QVERIFY(yearlySuccessor.has_value());
  if (!yearlySuccessor.has_value()) {
    return;
  }
  QCOMPARE(yearlySuccessor->templateDueDate, QStringLiteral("2025-02-28"));
}

void TaskRecurrenceMarkerTest::expandsDateOnlyWeekdayOrdinalAndExceptionRules() {
  hcb::TaskRecurrenceMarker weekdays = marker();
  weekdays.frequency = hcb::TaskRecurrenceFrequency::Weekly;
  weekdays.interval = 1;
  weekdays.anchorDate = QStringLiteral("2026-07-27");
  weekdays.templateDueDate = weekdays.anchorDate;
  weekdays.ordinal = 0;
  weekdays.occurrenceId = weekdays.seriesId + QStringLiteral(":0");
  weekdays.recurrenceRule = QStringLiteral("FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,WE,FR");
  weekdays.exclusionDates = {QStringLiteral("2026-07-29")};
  weekdays.additionDates = {QStringLiteral("2026-07-30")};
  const std::optional<hcb::TaskRecurrenceMarker> addedSuccessor =
      hcb::taskRecurrenceSuccessor(weekdays);
  QVERIFY(addedSuccessor.has_value());
  if (!addedSuccessor.has_value()) {
    return;
  }
  QCOMPARE(addedSuccessor->templateDueDate, QStringLiteral("2026-07-30"));

  hcb::TaskRecurrenceMarker lastFriday = marker();
  lastFriday.frequency = hcb::TaskRecurrenceFrequency::Monthly;
  lastFriday.interval = 1;
  lastFriday.anchorDate = QStringLiteral("2026-08-28");
  lastFriday.templateDueDate = lastFriday.anchorDate;
  lastFriday.ordinal = 0;
  lastFriday.occurrenceId = lastFriday.seriesId + QStringLiteral(":0");
  lastFriday.recurrenceRule = QStringLiteral("FREQ=MONTHLY;INTERVAL=1;BYDAY=-1FR");
  const std::optional<hcb::TaskRecurrenceMarker> monthlySuccessor =
      hcb::taskRecurrenceSuccessor(lastFriday);
  QVERIFY(monthlySuccessor.has_value());
  if (!monthlySuccessor.has_value()) {
    return;
  }
  QCOMPARE(monthlySuccessor->templateDueDate, QStringLiteral("2026-09-25"));

  hcb::TaskRecurrenceMarker businessDays = marker();
  businessDays.frequency = hcb::TaskRecurrenceFrequency::Daily;
  businessDays.interval = 1;
  businessDays.anchorDate = QStringLiteral("2026-07-31");
  businessDays.templateDueDate = businessDays.anchorDate;
  businessDays.ordinal = 0;
  businessDays.occurrenceId = businessDays.seriesId + QStringLiteral(":0");
  businessDays.recurrenceRule = QStringLiteral("FREQ=DAILY;INTERVAL=1;BYDAY=MO,TU,WE,TH,FR");
  const std::optional<hcb::TaskRecurrenceMarker> businessSuccessor =
      hcb::taskRecurrenceSuccessor(businessDays);
  QVERIFY(businessSuccessor.has_value());
  if (!businessSuccessor.has_value()) {
    return;
  }
  QCOMPARE(businessSuccessor->templateDueDate, QStringLiteral("2026-08-03"));
}

QTEST_GUILESS_MAIN(TaskRecurrenceMarkerTest)

#include "TaskRecurrenceMarkerTest.moc"
