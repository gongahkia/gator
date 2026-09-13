#include "core/CalendarLayoutEngine.h"

#include <algorithm>
#include <utility>

namespace hcb {
namespace {

constexpr int kMinutesPerDay = 24 * 60;
constexpr int kMinimumTimedDurationMinutes = 5;

struct TimedCandidate final {
  CalendarTimedLayoutEvent event;
};

struct AllDayCandidate final {
  CalendarAllDayLayoutEvent event;
  int startDayIndex{0};
  int endDayIndex{0};
  bool startsBeforeRange{false};
  bool endsAfterRange{false};
};

[[nodiscard]] bool isValidTimedEvent(const CalendarTimedLayoutEvent& event) {
  return !event.id.isEmpty() && event.id == event.id.trimmed() && event.id.size() <= 256 &&
         !event.id.contains(QChar::Null) && event.startMinute >= 0 &&
         event.endMinute <= kMinutesPerDay && event.endMinute > event.startMinute;
}

[[nodiscard]] bool isValidAllDayEvent(const CalendarAllDayLayoutEvent& event) {
  return !event.id.isEmpty() && event.id == event.id.trimmed() && event.id.size() <= 256 &&
         !event.id.contains(QChar::Null) && event.endDayIndex >= event.startDayIndex;
}

} // namespace

QList<CalendarTimedLayout>
CalendarLayoutEngine::layoutTimed(QList<CalendarTimedLayoutEvent> events) {
  QList<TimedCandidate> candidates;
  candidates.reserve(events.size());
  for (CalendarTimedLayoutEvent& event : events) {
    if (isValidTimedEvent(event)) {
      candidates.append({.event = std::move(event)});
    }
  }
  std::sort(candidates.begin(),
            candidates.end(),
            [](const TimedCandidate& left, const TimedCandidate& right) {
              return left.event.startMinute != right.event.startMinute
                         ? left.event.startMinute < right.event.startMinute
                     : left.event.endMinute != right.event.endMinute
                         ? left.event.endMinute < right.event.endMinute
                         : left.event.id < right.event.id;
            });

  QList<CalendarTimedLayout> layouts;
  layouts.reserve(candidates.size());
  QList<TimedCandidate> cluster;
  int clusterEnd = -1;
  const auto flushCluster = [&layouts, &cluster]() {
    QList<int> laneEnds;
    QList<CalendarTimedLayout> pending;
    pending.reserve(cluster.size());
    for (const TimedCandidate& candidate : cluster) {
      int laneIndex = 0;
      while (laneIndex < laneEnds.size() && laneEnds.at(laneIndex) > candidate.event.startMinute) {
        ++laneIndex;
      }
      if (laneIndex == laneEnds.size()) {
        laneEnds.append(candidate.event.endMinute);
      } else {
        laneEnds[laneIndex] = candidate.event.endMinute;
      }
      pending.append(
          {.id = candidate.event.id,
           .startMinute = candidate.event.startMinute,
           .durationMinutes = std::max(kMinimumTimedDurationMinutes,
                                       candidate.event.endMinute - candidate.event.startMinute),
           .laneIndex = laneIndex});
    }
    const int laneCount = std::max(1, static_cast<int>(laneEnds.size()));
    for (CalendarTimedLayout& layout : pending) {
      layout.laneCount = laneCount;
      layouts.append(std::move(layout));
    }
    cluster.clear();
  };

  for (TimedCandidate& candidate : candidates) {
    if (!cluster.isEmpty() && candidate.event.startMinute >= clusterEnd) {
      flushCluster();
      clusterEnd = -1;
    }
    clusterEnd = std::max(clusterEnd, candidate.event.endMinute);
    cluster.append(std::move(candidate));
  }
  if (!cluster.isEmpty()) {
    flushCluster();
  }
  return layouts;
}

CalendarAllDayLayout CalendarLayoutEngine::layoutAllDay(QList<CalendarAllDayLayoutEvent> events,
                                                        int dayCount,
                                                        int visibleLaneCount) {
  if (dayCount < 1 || visibleLaneCount < 0) {
    return {};
  }
  QList<AllDayCandidate> candidates;
  candidates.reserve(events.size());
  for (CalendarAllDayLayoutEvent& event : events) {
    if (!isValidAllDayEvent(event) || event.endDayIndex < 0 || event.startDayIndex >= dayCount) {
      continue;
    }
    const int startDayIndex = std::max(0, event.startDayIndex);
    const int endDayIndex = std::min(dayCount - 1, event.endDayIndex);
    const bool startsBeforeRange = event.startDayIndex < 0;
    const bool endsAfterRange = event.endDayIndex >= dayCount;
    candidates.append({.event = std::move(event),
                       .startDayIndex = startDayIndex,
                       .endDayIndex = endDayIndex,
                       .startsBeforeRange = startsBeforeRange,
                       .endsAfterRange = endsAfterRange});
  }
  std::sort(candidates.begin(),
            candidates.end(),
            [](const AllDayCandidate& left, const AllDayCandidate& right) {
              const int leftSpan = left.endDayIndex - left.startDayIndex;
              const int rightSpan = right.endDayIndex - right.startDayIndex;
              return left.startDayIndex != right.startDayIndex
                         ? left.startDayIndex < right.startDayIndex
                     : leftSpan != rightSpan ? leftSpan > rightSpan
                                             : left.event.id < right.event.id;
            });

  CalendarAllDayLayout layout;
  layout.overflowCounts.fill(0, dayCount);
  QList<int> laneEnds;
  for (const AllDayCandidate& candidate : candidates) {
    int laneIndex = 0;
    while (laneIndex < laneEnds.size() && laneEnds.at(laneIndex) >= candidate.startDayIndex) {
      ++laneIndex;
    }
    if (laneIndex == laneEnds.size()) {
      laneEnds.append(candidate.endDayIndex);
    } else {
      laneEnds[laneIndex] = candidate.endDayIndex;
    }
    if (laneIndex >= visibleLaneCount) {
      for (int dayIndex = candidate.startDayIndex; dayIndex <= candidate.endDayIndex; ++dayIndex) {
        ++layout.overflowCounts[dayIndex];
      }
      continue;
    }
    layout.segments.append({.id = candidate.event.id,
                            .startDayIndex = candidate.startDayIndex,
                            .daySpan = candidate.endDayIndex - candidate.startDayIndex + 1,
                            .laneIndex = laneIndex,
                            .startsBeforeRange = candidate.startsBeforeRange,
                            .endsAfterRange = candidate.endsAfterRange});
  }
  return layout;
}

} // namespace hcb
