#include <QtTest/QTest>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/Cancellation.h"
#include "core/RecurrenceExpansionWorker.h"

using namespace std::chrono_literals;

class RecurrenceExpansionWorkerTest final : public QObject {
  Q_OBJECT

private slots:
  void expandsDailyRuleWithStableOccurrenceIds();
  void expandsWeeklyAndMonthlySelectors();
  void projectsOnlyOccurrencesWithinTheRequestedRange();
  void appliesExceptionAndAdditionalDates();
  void preservesLocalStartTimeAcrossDst();
  void fallsBackForUnsupportedRulesAndCancels();
};

namespace {

hcb::RecurrenceExpansionResult awaitResult(std::future<hcb::RecurrenceExpansionResult>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("recurrence expansion timed out");
  }
  return future.get();
}

const QList<hcb::RecurrenceOccurrence>& occurrences(const hcb::RecurrenceExpansionResult& result) {
  if (!std::holds_alternative<QList<hcb::RecurrenceOccurrence>>(result)) {
    qFatal("recurrence expansion failed");
  }
  return std::get<QList<hcb::RecurrenceOccurrence>>(result);
}

} // namespace

void RecurrenceExpansionWorkerTest::expandsDailyRuleWithStableOccurrenceIds() {
  hcb::RecurrenceExpansionWorker worker;
  std::future<hcb::RecurrenceExpansionResult> future =
      worker.expand({.eventId = QStringLiteral("event-daily"),
                     .startAt = QStringLiteral("2026-07-25T09:00:00.000Z"),
                     .endAt = QStringLiteral("2026-07-25T10:30:00.000Z"),
                     .recurrenceRule = QStringLiteral("RRULE:FREQ=DAILY;COUNT=3")});
  const hcb::RecurrenceExpansionResult result = awaitResult(future);
  const QList<hcb::RecurrenceOccurrence>& expanded = occurrences(result);
  QCOMPARE(expanded.size(), 3);
  QCOMPARE(expanded.at(0).id, QStringLiteral("event-daily"));
  QCOMPARE(expanded.at(1).id, QStringLiteral("event-daily:instance:20260726T090000Z"));
  QCOMPARE(expanded.at(2).startAt, QStringLiteral("2026-07-27T09:00:00.000Z"));
  QCOMPARE(expanded.at(2).endAt, QStringLiteral("2026-07-27T10:30:00.000Z"));
  QCOMPARE(expanded.at(0).originalStartAt,
           std::optional<QString>(QStringLiteral("2026-07-25T09:00:00.000Z")));
}

void RecurrenceExpansionWorkerTest::expandsWeeklyAndMonthlySelectors() {
  hcb::RecurrenceExpansionWorker worker;
  std::future<hcb::RecurrenceExpansionResult> weeklyFuture =
      worker.expand({.eventId = QStringLiteral("event-weekly"),
                     .startAt = QStringLiteral("2026-07-01T09:00:00.000Z"),
                     .endAt = QStringLiteral("2026-07-01T10:00:00.000Z"),
                     .recurrenceRule = QStringLiteral("RRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=4")});
  const hcb::RecurrenceExpansionResult weeklyResult = awaitResult(weeklyFuture);
  const QList<hcb::RecurrenceOccurrence>& weekly = occurrences(weeklyResult);
  QCOMPARE(weekly.size(), 4);
  QCOMPARE(weekly.at(0).startAt, QStringLiteral("2026-07-01T09:00:00.000Z"));
  QCOMPARE(weekly.at(1).startAt, QStringLiteral("2026-07-06T09:00:00.000Z"));
  QCOMPARE(weekly.at(3).startAt, QStringLiteral("2026-07-13T09:00:00.000Z"));

  std::future<hcb::RecurrenceExpansionResult> monthlyFuture = worker.expand(
      {.eventId = QStringLiteral("event-monthly"),
       .startAt = QStringLiteral("2026-07-01T00:00:00.000Z"),
       .endAt = QStringLiteral("2026-07-02T00:00:00.000Z"),
       .allDay = true,
       .recurrenceRule = QStringLiteral("RRULE:FREQ=MONTHLY;BYDAY=MO;BYSETPOS=-1;COUNT=3")});
  const hcb::RecurrenceExpansionResult monthlyResult = awaitResult(monthlyFuture);
  const QList<hcb::RecurrenceOccurrence>& monthly = occurrences(monthlyResult);
  QCOMPARE(monthly.size(), 3);
  QCOMPARE(monthly.at(0).startAt, QStringLiteral("2026-07-27T00:00:00.000Z"));
  QCOMPARE(monthly.at(1).startAt, QStringLiteral("2026-08-31T00:00:00.000Z"));
  QCOMPARE(monthly.at(2).startAt, QStringLiteral("2026-09-28T00:00:00.000Z"));
  QCOMPARE(monthly.at(1).id, QStringLiteral("event-monthly:instance:20260831"));
}

void RecurrenceExpansionWorkerTest::projectsOnlyOccurrencesWithinTheRequestedRange() {
  hcb::RecurrenceExpansionWorker worker;
  std::future<hcb::RecurrenceExpansionResult> future = worker.expand(
      {.eventId = QStringLiteral("event-range"),
       .startAt = QStringLiteral("2024-01-01T09:00:00.000Z"),
       .endAt = QStringLiteral("2024-01-01T10:00:00.000Z"),
       .recurrenceRule = QStringLiteral("RRULE:FREQ=DAILY"),
       .rangeStartAt = QStringLiteral("2026-07-25T00:00:00.000Z"),
       .rangeEndAt = QStringLiteral("2026-07-28T00:00:00.000Z")});
  const hcb::RecurrenceExpansionResult result = awaitResult(future);
  const QList<hcb::RecurrenceOccurrence>& expanded = occurrences(result);
  QCOMPARE(expanded.size(), 3);
  QCOMPARE(expanded.at(0).startAt, QStringLiteral("2026-07-25T09:00:00.000Z"));
  QCOMPARE(expanded.at(2).startAt, QStringLiteral("2026-07-27T09:00:00.000Z"));
}

void RecurrenceExpansionWorkerTest::appliesExceptionAndAdditionalDates() {
  hcb::RecurrenceExpansionWorker worker;
  std::future<hcb::RecurrenceExpansionResult> future = worker.expand(
      {.eventId = QStringLiteral("event-exceptions"),
       .startAt = QStringLiteral("2026-07-25T09:00:00.000Z"),
       .endAt = QStringLiteral("2026-07-25T10:00:00.000Z"),
       .recurrenceRule = QStringLiteral("RRULE:FREQ=DAILY;COUNT=3\n"
                                        "EXDATE:20260726T090000Z\n"
                                        "RDATE:20260728T090000Z")});
  const hcb::RecurrenceExpansionResult result = awaitResult(future);
  const QList<hcb::RecurrenceOccurrence>& expanded = occurrences(result);
  QCOMPARE(expanded.size(), 3);
  QCOMPARE(expanded.at(0).startAt, QStringLiteral("2026-07-25T09:00:00.000Z"));
  QCOMPARE(expanded.at(1).startAt, QStringLiteral("2026-07-27T09:00:00.000Z"));
  QCOMPARE(expanded.at(2).startAt, QStringLiteral("2026-07-28T09:00:00.000Z"));

  std::future<hcb::RecurrenceExpansionResult> allDayFuture = worker.expand(
      {.eventId = QStringLiteral("event-all-day-exception"),
       .startAt = QStringLiteral("2026-07-25T00:00:00.000Z"),
       .endAt = QStringLiteral("2026-07-26T00:00:00.000Z"),
       .allDay = true,
       .recurrenceRule = QStringLiteral("RRULE:FREQ=DAILY;COUNT=3\n"
                                        "EXRULE:FREQ=DAILY;COUNT=1\n"
                                        "RDATE:20260728")});
  const hcb::RecurrenceExpansionResult allDayResult = awaitResult(allDayFuture);
  const QList<hcb::RecurrenceOccurrence>& allDay = occurrences(allDayResult);
  QCOMPARE(allDay.size(), 3);
  QCOMPARE(allDay.at(0).startAt, QStringLiteral("2026-07-26T00:00:00.000Z"));
  QCOMPARE(allDay.at(2).startAt, QStringLiteral("2026-07-28T00:00:00.000Z"));
}

void RecurrenceExpansionWorkerTest::preservesLocalStartTimeAcrossDst() {
  hcb::RecurrenceExpansionWorker worker;
  std::future<hcb::RecurrenceExpansionResult> future = worker.expand(
      {.eventId = QStringLiteral("event-dst"),
       .startAt = QStringLiteral("2026-03-07T14:00:00.000Z"),
       .endAt = QStringLiteral("2026-03-07T15:00:00.000Z"),
       .timeZone = QStringLiteral("America/New_York"),
       .recurrenceRule = QStringLiteral("RRULE:FREQ=DAILY;COUNT=3")});
  const hcb::RecurrenceExpansionResult result = awaitResult(future);
  const QList<hcb::RecurrenceOccurrence>& expanded = occurrences(result);
  QCOMPARE(expanded.size(), 3);
  QCOMPARE(expanded.at(0).startAt, QStringLiteral("2026-03-07T14:00:00.000Z"));
  QCOMPARE(expanded.at(1).startAt, QStringLiteral("2026-03-08T13:00:00.000Z"));
  QCOMPARE(expanded.at(2).startAt, QStringLiteral("2026-03-09T13:00:00.000Z"));
}

void RecurrenceExpansionWorkerTest::fallsBackForUnsupportedRulesAndCancels() {
  hcb::RecurrenceExpansionWorker worker;
  std::future<hcb::RecurrenceExpansionResult> fallbackFuture =
      worker.expand({.eventId = QStringLiteral("event-single"),
                     .startAt = QStringLiteral("2026-07-25T09:00:00.000Z"),
                     .endAt = QStringLiteral("2026-07-25T10:00:00.000Z"),
                     .recurrenceRule = QStringLiteral("RRULE:FREQ=HOURLY;COUNT=2")});
  const hcb::RecurrenceExpansionResult fallbackResult = awaitResult(fallbackFuture);
  const QList<hcb::RecurrenceOccurrence>& fallback = occurrences(fallbackResult);
  QCOMPARE(fallback.size(), 1);
  QCOMPARE(fallback.at(0).id, QStringLiteral("event-single"));
  QVERIFY(!fallback.at(0).originalStartAt.has_value());

  hcb::CancellationSource cancellation;
  QVERIFY(cancellation.requestStop());
  std::future<hcb::RecurrenceExpansionResult> cancelledFuture =
      worker.expand({.eventId = QStringLiteral("event-cancelled"),
                     .startAt = QStringLiteral("2026-07-25T09:00:00.000Z"),
                     .endAt = QStringLiteral("2026-07-25T10:00:00.000Z"),
                     .recurrenceRule = QStringLiteral("RRULE:FREQ=DAILY")},
                    cancellation.token());
  const hcb::RecurrenceExpansionResult cancelled = awaitResult(cancelledFuture);
  QVERIFY(std::holds_alternative<hcb::AppError>(cancelled));
  QCOMPARE(std::get<hcb::AppError>(cancelled).code(), hcb::AppErrorCode::Cancelled);
}

QTEST_GUILESS_MAIN(RecurrenceExpansionWorkerTest)

#include "RecurrenceExpansionWorkerTest.moc"
