#include <QtTest>

#include "core/QuickCaptureParser.h"

namespace {

[[nodiscard]] hcb::QuickCaptureParseRequest requestFor(QString text,
                                                        hcb::QuickCaptureKind kind = hcb::QuickCaptureKind::Event) {
  return {.text = std::move(text),
          .kind = kind,
          .now = QDateTime(QDate(2026, 7, 28), QTime(16, 0), QTimeZone("Asia/Singapore")),
          .timeZone = QTimeZone("Asia/Singapore"),
          .defaultEventDurationMinutes = 30,
          .aliases = hcb::QuickCaptureParser::defaultAliases()};
}

} // namespace

class QuickCaptureParserTest final : public QObject {
  Q_OBJECT

private slots:
  void parsesTimedRecurringEvent();
  void preservesTaskTimeWhileExtractingTaskMetadata();
  void createsAllDayEventsFromDates();
  void rollsTimedEventsWithoutDatesToTheNextFutureSlot();
  void honorsExplicitAliasesAndDisabledRecognitions();
  void leavesUnsupportedTextUntouched();
};

void QuickCaptureParserTest::parsesTimedRecurringEvent() {
  const hcb::QuickCaptureParseResult result = hcb::QuickCaptureParser::parse(
      requestFor(QStringLiteral("Team sync tomorrow at 9am for 45m every 2 weeks")));

  QCOMPARE(result.kind, hcb::QuickCaptureKind::Event);
  QCOMPARE(result.date, std::optional<QDate>(QDate(2026, 7, 29)));
  QCOMPARE(result.time, std::optional<QTime>(QTime(9, 0)));
  QCOMPARE(result.eventDurationMinutes, 45);
  QVERIFY(result.recurrence.enabled);
  QCOMPARE(result.recurrence.frequency, 1);
  QCOMPARE(result.recurrence.interval, 2);
  QCOMPARE(result.recurrence.rrule, QStringLiteral("RRULE:FREQ=WEEKLY;INTERVAL=2"));
  QCOMPARE(result.parsedTitle, QStringLiteral("Team sync"));
  QVERIFY(result.eventReady);
}

void QuickCaptureParserTest::preservesTaskTimeWhileExtractingTaskMetadata() {
  const hcb::QuickCaptureParseResult result = hcb::QuickCaptureParser::parse(
      requestFor(QStringLiteral("Call Sam tomorrow at 5pm P1 every day"), hcb::QuickCaptureKind::Task));

  QCOMPARE(result.kind, hcb::QuickCaptureKind::Task);
  QCOMPARE(result.date, std::optional<QDate>(QDate(2026, 7, 29)));
  QCOMPARE(result.time, std::optional<QTime>(QTime(17, 0)));
  QCOMPARE(result.taskPriority, 3);
  QVERIFY(result.recurrence.enabled);
  QCOMPARE(result.recurrence.rrule, QStringLiteral("RRULE:FREQ=DAILY;INTERVAL=1"));
  QCOMPARE(result.parsedTitle, QStringLiteral("Call Sam at 5pm"));
}

void QuickCaptureParserTest::createsAllDayEventsFromDates() {
  const hcb::QuickCaptureParseResult result =
      hcb::QuickCaptureParser::parse(requestFor(QStringLiteral("Offsite August 3")));

  QCOMPARE(result.date, std::optional<QDate>(QDate(2026, 8, 3)));
  QVERIFY(result.allDay);
  QVERIFY(result.eventReady);
  QCOMPARE(result.parsedTitle, QStringLiteral("Offsite"));
}

void QuickCaptureParserTest::rollsTimedEventsWithoutDatesToTheNextFutureSlot() {
  const hcb::QuickCaptureParseResult result =
      hcb::QuickCaptureParser::parse(requestFor(QStringLiteral("Dentist at 10am")));

  QCOMPARE(result.date, std::optional<QDate>(QDate(2026, 7, 29)));
  QCOMPARE(result.time, std::optional<QTime>(QTime(10, 0)));
  QCOMPARE(result.eventDurationMinutes, 30);
  QVERIFY(!result.allDay);
  QVERIFY(result.eventReady);
}

void QuickCaptureParserTest::honorsExplicitAliasesAndDisabledRecognitions() {
  hcb::QuickCaptureParseRequest request = requestFor(QStringLiteral("Todo review P1 tomorrow"));
  request.aliases.task = {QStringLiteral("todo")};
  request.aliases.highPriority = {QStringLiteral("p1")};
  const hcb::QuickCaptureParseResult task = hcb::QuickCaptureParser::parse(request);

  QCOMPARE(task.kind, hcb::QuickCaptureKind::Task);
  QCOMPARE(task.taskPriority, 3);
  QVERIFY(!task.recognitions.isEmpty());
  const auto priority = std::find_if(task.recognitions.cbegin(), task.recognitions.cend(),
                                     [](const hcb::QuickCaptureRecognition& recognition) {
                                       return recognition.label == QStringLiteral("High priority");
                                     });
  QVERIFY(priority != task.recognitions.cend());

  request.disabledRecognitionIds = {priority->id};
  const hcb::QuickCaptureParseResult disabled = hcb::QuickCaptureParser::parse(request);
  QCOMPARE(disabled.taskPriority, 0);
  QVERIFY(disabled.parsedTitle.contains(QStringLiteral("P1")));
}

void QuickCaptureParserTest::leavesUnsupportedTextUntouched() {
  const hcb::QuickCaptureParseResult result =
      hcb::QuickCaptureParser::parse(requestFor(QStringLiteral("Draft the project brief"), hcb::QuickCaptureKind::Task));

  QCOMPARE(result.parsedTitle, QStringLiteral("Draft the project brief"));
  QVERIFY(!result.date.has_value());
  QVERIFY(!result.recurrence.enabled);
  QCOMPARE(result.taskPriority, 0);
}

QTEST_MAIN(QuickCaptureParserTest)
#include "QuickCaptureParserTest.moc"
