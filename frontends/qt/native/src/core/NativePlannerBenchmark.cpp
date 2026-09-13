#include "core/NativePlannerBenchmark.h"

#include "core/CalendarLayoutEngine.h"

#include <QDateTime>
#include <QElapsedTimer>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMap>
#include <QTimeZone>

#include <algorithm>
#include <optional>

namespace hcb {

std::optional<NativePlannerBenchmarkResult>
NativePlannerBenchmark::run(const NativePerformanceFixture& fixture) {
  if (fixture.eventInstances.empty()) {
    return std::nullopt;
  }
  QElapsedTimer timer;
  timer.start();
  QMap<QDate, QList<CalendarTimedLayoutEvent>> timedByDay;
  QList<std::pair<QDate, CalendarAllDayLayoutEvent>> allDay;
  QDate firstDay;
  QDate lastDay;
  for (const NativePerformanceEventFixture& event : fixture.eventInstances) {
    const QDateTime start = QDateTime::fromString(event.startsAt, Qt::ISODateWithMs).toUTC();
    const QDateTime end = QDateTime::fromString(event.endsAt, Qt::ISODateWithMs).toUTC();
    if (!start.isValid() || !end.isValid() || end <= start) {
      return std::nullopt;
    }
    const QDate day = start.date();
    if (!firstDay.isValid() || day < firstDay) {
      firstDay = day;
    }
    if (!lastDay.isValid() || day > lastDay) {
      lastDay = day;
    }
    const int startMinute = start.time().hour() * 60 + start.time().minute();
    const int endMinute = end.date() == day ? end.time().hour() * 60 + end.time().minute() : 1'440;
    if (event.isAllDay) {
      allDay.append({day, {.id = event.id}});
    } else {
      timedByDay[day].append({.id = event.id, .startMinute = startMinute, .endMinute = endMinute});
    }
  }
  std::size_t timedLayoutCount = 0;
  for (const QList<CalendarTimedLayoutEvent>& events : timedByDay) {
    timedLayoutCount += static_cast<std::size_t>(CalendarLayoutEngine::layoutTimed(events).size());
  }
  const int dayCount = static_cast<int>(firstDay.daysTo(lastDay) + 1);
  QList<CalendarAllDayLayoutEvent> allDayEvents;
  allDayEvents.reserve(allDay.size());
  for (const auto& [day, event] : allDay) {
    const int dayIndex = static_cast<int>(firstDay.daysTo(day));
    allDayEvents.append({.id = event.id, .startDayIndex = dayIndex, .endDayIndex = dayIndex});
  }
  const CalendarAllDayLayout allDayLayout =
      CalendarLayoutEngine::layoutAllDay(std::move(allDayEvents), dayCount, 3);
  std::size_t allDayOverflowCount = 0;
  for (const int count : allDayLayout.overflowCounts) {
    allDayOverflowCount += static_cast<std::size_t>(count);
  }
  return NativePlannerBenchmarkResult{.eventCount = fixture.eventInstances.size(),
                                      .timedDayCount = static_cast<std::size_t>(timedByDay.size()),
                                      .timedLayoutCount = timedLayoutCount,
                                      .allDaySegmentCount =
                                          static_cast<std::size_t>(allDayLayout.segments.size()),
                                      .allDayOverflowCount = allDayOverflowCount,
                                      .elapsedNanoseconds = timer.nsecsElapsed()};
}

QByteArray NativePlannerBenchmark::toJson(const NativePlannerBenchmarkResult& result) {
  return QJsonDocument(
             QJsonObject{
                 {QStringLiteral("eventCount"), static_cast<qint64>(result.eventCount)},
                 {QStringLiteral("timedDayCount"), static_cast<qint64>(result.timedDayCount)},
                 {QStringLiteral("timedLayoutCount"), static_cast<qint64>(result.timedLayoutCount)},
                 {QStringLiteral("allDaySegmentCount"),
                  static_cast<qint64>(result.allDaySegmentCount)},
                 {QStringLiteral("allDayOverflowCount"),
                  static_cast<qint64>(result.allDayOverflowCount)},
                 {QStringLiteral("elapsedNanoseconds"),
                  static_cast<qint64>(result.elapsedNanoseconds)}})
      .toJson(QJsonDocument::Compact);
}

} // namespace hcb
