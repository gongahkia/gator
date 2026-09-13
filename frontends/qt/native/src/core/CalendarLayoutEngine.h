#pragma once

#include <QList>
#include <QString>

namespace hcb {

struct CalendarTimedLayoutEvent final {
  QString id;
  int startMinute{0};
  int endMinute{0};
};

struct CalendarTimedLayout final {
  QString id;
  int startMinute{0};
  int durationMinutes{0};
  int laneIndex{0};
  int laneCount{1};
};

struct CalendarAllDayLayoutEvent final {
  QString id;
  int startDayIndex{0};
  int endDayIndex{0};
};

struct CalendarAllDaySegment final {
  QString id;
  int startDayIndex{0};
  int daySpan{0};
  int laneIndex{0};
  bool startsBeforeRange{false};
  bool endsAfterRange{false};
};

struct CalendarAllDayLayout final {
  QList<CalendarAllDaySegment> segments;
  QList<int> overflowCounts;
};

class CalendarLayoutEngine final {
public:
  [[nodiscard]] static QList<CalendarTimedLayout>
  layoutTimed(QList<CalendarTimedLayoutEvent> events);
  [[nodiscard]] static CalendarAllDayLayout
  layoutAllDay(QList<CalendarAllDayLayoutEvent> events, int dayCount, int visibleLaneCount);
};

} // namespace hcb
