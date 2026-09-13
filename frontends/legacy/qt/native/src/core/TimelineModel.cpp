#include "core/TimelineModel.h"

#include "core/CalendarLayoutEngine.h"
#include "core/TimelineViewportModel.h"

#include <QDateTime>
#include <QHash>

#include <algorithm>
#include <cmath>
#include <optional>
#include <utility>

namespace hcb {

namespace {

constexpr int kMinutesPerDay = 24 * 60;
constexpr int kSnapMinutes = 15;

[[nodiscard]] int clampMinute(int minute, bool endPoint) {
  const int lower = endPoint ? kSnapMinutes : 0;
  const int upper = endPoint ? kMinutesPerDay : kMinutesPerDay - kSnapMinutes;
  return std::clamp(minute, lower, upper);
}

[[nodiscard]] QDateTime resolvedDateTime(const QDate& date,
                                         const QTime& time,
                                         const QTimeZone& timeZone) {
  return QDateTime(date, time, timeZone, QDateTime::TransitionResolution::PreferAfter);
}

struct EventRange final {
  QDateTime start;
  QDateTime end;
  QDate firstDate;
  QDate lastDate;
};

[[nodiscard]] std::optional<QDateTime> parseDateTime(const QString& value,
                                                     const QTimeZone& displayTimeZone) {
  QDateTime parsed = QDateTime::fromString(value, Qt::ISODateWithMs);
  if (!parsed.isValid()) {
    parsed = QDateTime::fromString(value, Qt::ISODate);
  }
  if (parsed.isValid()) {
    const bool hasOffset =
        value.endsWith(u'Z') || value.lastIndexOf(u'+') > 9 || value.lastIndexOf(u'-') > 9;
    return hasOffset ? parsed.toTimeZone(displayTimeZone)
                     : QDateTime(parsed.date(), parsed.time(), displayTimeZone);
  }
  const QDate date = QDate::fromString(value.left(10), Qt::ISODate);
  return date.isValid() ? std::optional<QDateTime>(QDateTime(date, QTime(0, 0), displayTimeZone))
                        : std::nullopt;
}

[[nodiscard]] std::optional<EventRange> eventRange(const CalendarEventSummary& event,
                                                   const QTimeZone& displayTimeZone) {
  if (event.allDay) {
    const QDate startDate = QDate::fromString(event.startAt.left(10), Qt::ISODate);
    const QDate endDate = QDate::fromString(event.endAt.left(10), Qt::ISODate);
    if (!startDate.isValid() || !endDate.isValid() || endDate <= startDate) {
      return std::nullopt;
    }
    return EventRange{.start = QDateTime(startDate, QTime(0, 0), displayTimeZone),
                      .end = QDateTime(endDate, QTime(0, 0), displayTimeZone),
                      .firstDate = startDate,
                      .lastDate = endDate.addDays(-1)};
  }
  const std::optional<QDateTime> start = parseDateTime(event.startAt, displayTimeZone);
  const std::optional<QDateTime> end = parseDateTime(event.endAt, displayTimeZone);
  if (!start.has_value() || !end.has_value() || *end <= *start) {
    return std::nullopt;
  }
  QDate lastDate = end->date();
  if (end->time() == QTime(0, 0)) {
    lastDate = lastDate.addDays(-1);
  }
  if (lastDate < start->date()) {
    return std::nullopt;
  }
  return EventRange{.start = *start, .end = *end, .firstDate = start->date(), .lastDate = lastDate};
}

[[nodiscard]] int minuteOfDay(const QDateTime& value, bool roundUp) {
  const QTime time = value.time();
  const int minute = time.hour() * 60 + time.minute();
  return roundUp && (time.second() > 0 || time.msec() > 0) ? minute + 1 : minute;
}

} // namespace

TimelineModel::TimelineModel(QObject* parent) : QAbstractListModel(parent) {}

int TimelineModel::rowCount(const QModelIndex& parent) const {
  return parent.isValid() ? 0 : static_cast<int>(items_.size());
}

QVariant TimelineModel::data(const QModelIndex& index, int role) const {
  if (!index.isValid() || index.row() < 0 || index.row() >= static_cast<int>(items_.size())) {
    return {};
  }
  const Item& item = items_.at(index.row());
  switch (role) {
  case Qt::DisplayRole:
  case TitleRole:
    return item.event.title;
  case IdRole:
    return item.event.id;
  case CalendarIdRole:
    return item.event.calendarId;
  case RecurringRemoteIdRole:
    return item.event.recurringRemoteId.value_or(QString());
  case OriginalStartAtRole:
    return item.event.originalStartAt.value_or(QString());
  case RecurrenceRuleRole:
    return item.event.recurrenceRule.value_or(QString());
  case StatusRole:
    return item.event.status;
  case ColorIdRole:
    return item.event.colorId.value_or(QString());
  case DescriptionRole:
    return item.event.description.value_or(QString());
  case LocationRole:
    return item.event.location.value_or(QString());
  case StartAtRole:
    return item.event.startAt;
  case StartTimeZoneRole:
    return item.event.startTimeZone.value_or(QString());
  case EndAtRole:
    return item.event.endAt;
  case EndTimeZoneRole:
    return item.event.endTimeZone.value_or(QString());
  case TransparencyRole:
    return item.event.transparency.value_or(QString());
  case VisibilityRole:
    return item.event.visibility.value_or(QString());
  case AttendeeEmailsJsonRole:
    return item.event.attendeeEmailsJson;
  case AttendeeDetailsJsonRole:
    return item.event.attendeeDetailsJson;
  case RemindersJsonRole:
    return item.event.remindersJson;
  case RemindersUseDefaultRole:
    return item.event.remindersUseDefault;
  case ConferenceJsonRole:
    return item.event.conferenceJson.value_or(QString());
  case AttachmentsJsonRole:
    return item.event.attachmentsJson;
  case GuestPermissionsJsonRole:
    return item.event.guestPermissionsJson;
  case StatusPropertiesJsonRole:
    return item.event.statusPropertiesJson;
  case EventTypeRole:
    return item.event.eventType.value_or(QStringLiteral("default"));
  case AllDayRole:
    return item.allDay;
  case DayIndexRole:
    return item.dayIndex;
  case StartMinuteRole:
    return item.startMinute;
  case DurationMinutesRole:
    return item.durationMinutes;
  case LaneIndexRole:
    return item.laneIndex;
  case LaneCountRole:
    return item.laneCount;
  case DaySpanRole:
    return item.daySpan;
  case StartsBeforeRangeRole:
    return item.startsBeforeRange;
  case EndsAfterRangeRole:
    return item.endsAfterRange;
  default:
    return {};
  }
}

int TimelineModel::totalItemCount() const { return static_cast<int>(items_.size()); }

QObject* TimelineModel::createViewport() {
  auto* viewport = new TimelineViewportModel(this);
  viewport->setSourceModel(this);
  return viewport;
}

QHash<int, QByteArray> TimelineModel::roleNames() const {
  return {{IdRole, "id"},
          {CalendarIdRole, "calendarId"},
          {RecurringRemoteIdRole, "recurringRemoteId"},
          {OriginalStartAtRole, "originalStartAt"},
          {RecurrenceRuleRole, "recurrenceRule"},
          {TitleRole, "title"},
          {StatusRole, "status"},
          {ColorIdRole, "colorId"},
          {DescriptionRole, "description"},
          {LocationRole, "location"},
          {StartAtRole, "startAt"},
          {StartTimeZoneRole, "startTimeZone"},
          {EndAtRole, "endAt"},
          {EndTimeZoneRole, "endTimeZone"},
          {TransparencyRole, "transparency"},
          {VisibilityRole, "visibility"},
          {AttendeeEmailsJsonRole, "attendeeEmailsJson"},
          {AttendeeDetailsJsonRole, "attendeeDetailsJson"},
          {RemindersJsonRole, "remindersJson"},
          {RemindersUseDefaultRole, "remindersUseDefault"},
          {ConferenceJsonRole, "conferenceJson"},
          {AttachmentsJsonRole, "attachmentsJson"},
          {GuestPermissionsJsonRole, "guestPermissionsJson"},
          {StatusPropertiesJsonRole, "statusPropertiesJson"},
          {EventTypeRole, "eventType"},
          {AllDayRole, "allDay"},
          {DayIndexRole, "dayIndex"},
          {StartMinuteRole, "startMinute"},
          {DurationMinutesRole, "durationMinutes"},
          {LaneIndexRole, "laneIndex"},
          {LaneCountRole, "laneCount"},
          {DaySpanRole, "daySpan"},
          {StartsBeforeRangeRole, "startsBeforeRange"},
          {EndsAfterRangeRole, "endsAfterRange"}};
}

QVariantMap
TimelineModel::moveInput(const QString& eventId, int targetDayIndex, int targetMinute) const {
  if (eventId.isEmpty() || !rangeStartDate_.isValid() || dayCount_ < 1 || targetDayIndex < 0 ||
      targetDayIndex >= dayCount_ || targetMinute < 0 || targetMinute >= kMinutesPerDay ||
      !displayTimeZone_.isValid()) {
    return {};
  }
  const auto item = std::find_if(items_.cbegin(), items_.cend(), [&eventId](const Item& candidate) {
    return candidate.event.id == eventId && !candidate.allDay;
  });
  if (item == items_.cend()) {
    return {};
  }
  const std::optional<EventRange> range = eventRange(item->event, displayTimeZone_);
  if (!range.has_value()) {
    return {};
  }
  const QDate targetDate = rangeStartDate_.addDays(targetDayIndex);
  const QTime targetTime(targetMinute / 60, targetMinute % 60);
  const QDateTime targetStart = resolvedDateTime(targetDate, targetTime, displayTimeZone_);
  if (!targetStart.isValid()) {
    return {};
  }
  const QDateTime targetEnd = targetStart.addMSecs(range->start.msecsTo(range->end));
  if (!targetEnd.isValid() || targetEnd <= targetStart) {
    return {};
  }
  return {{QStringLiteral("id"), item->event.id},
          {QStringLiteral("startAt"), targetStart.toUTC().toString(Qt::ISODateWithMs)},
          {QStringLiteral("endAt"), targetEnd.toUTC().toString(Qt::ISODateWithMs)},
          {QStringLiteral("allDay"), false}};
}

QVariantMap TimelineModel::moveAllDayInput(const QString& eventId, int targetDayIndex) const {
  if (eventId.isEmpty() || !rangeStartDate_.isValid() || dayCount_ < 1 || targetDayIndex < 0 ||
      targetDayIndex >= dayCount_ || !displayTimeZone_.isValid()) {
    return {};
  }
  const auto item = std::find_if(items_.cbegin(), items_.cend(), [&eventId](const Item& candidate) {
    return candidate.event.id == eventId && candidate.allDay && !candidate.startsBeforeRange &&
           !candidate.endsAfterRange;
  });
  if (item == items_.cend()) {
    return {};
  }
  const std::optional<EventRange> range = eventRange(item->event, displayTimeZone_);
  if (!range.has_value()) {
    return {};
  }
  const QDate targetStartDate = rangeStartDate_.addDays(targetDayIndex);
  const QDate targetEndDate =
      targetStartDate.addDays(range->start.date().daysTo(range->end.date()));
  return {{QStringLiteral("id"), item->event.id},
          {QStringLiteral("startAt"),
           QDateTime(targetStartDate, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs)},
          {QStringLiteral("endAt"),
           QDateTime(targetEndDate, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs)},
          {QStringLiteral("allDay"), true}};
}

QVariantMap TimelineModel::resizeInput(const QString& eventId,
                                       int targetEndDayIndex,
                                       int targetEndMinute) const {
  if (eventId.isEmpty() || !rangeStartDate_.isValid() || dayCount_ < 1 || targetEndDayIndex < 0 ||
      targetEndDayIndex >= dayCount_ || targetEndMinute < 0 || targetEndMinute > kMinutesPerDay ||
      !displayTimeZone_.isValid()) {
    return {};
  }
  const auto item = std::find_if(items_.cbegin(), items_.cend(), [&eventId](const Item& candidate) {
    return candidate.event.id == eventId && !candidate.allDay;
  });
  if (item == items_.cend()) {
    return {};
  }
  const std::optional<EventRange> range = eventRange(item->event, displayTimeZone_);
  if (!range.has_value()) {
    return {};
  }
  QDate targetDate = rangeStartDate_.addDays(targetEndDayIndex);
  if (targetEndMinute == kMinutesPerDay) {
    targetDate = targetDate.addDays(1);
  }
  const QTime targetTime(targetEndMinute % kMinutesPerDay / 60,
                         targetEndMinute % kMinutesPerDay % 60);
  const QDateTime targetEnd = resolvedDateTime(targetDate, targetTime, displayTimeZone_);
  if (!targetEnd.isValid() || targetEnd <= range->start) {
    return {};
  }
  return {{QStringLiteral("id"), item->event.id},
          {QStringLiteral("endAt"), targetEnd.toUTC().toString(Qt::ISODateWithMs)}};
}

QVariantMap TimelineModel::resizeAllDayInput(const QString& eventId, int targetEndDayIndex) const {
  const auto item = std::find_if(items_.cbegin(), items_.cend(), [&eventId](const Item& candidate) {
    return candidate.event.id == eventId && candidate.allDay && !candidate.startsBeforeRange &&
           !candidate.endsAfterRange;
  });
  if (item == items_.cend()) {
    return {};
  }
  return resizeAllDayRangeInput(eventId, item->dayIndex, targetEndDayIndex);
}

QVariantMap TimelineModel::resizeAllDayRangeInput(const QString& eventId,
                                                  int targetStartDayIndex,
                                                  int targetEndDayIndex) const {
  if (eventId.isEmpty() || !rangeStartDate_.isValid() || dayCount_ < 1 || targetEndDayIndex < 0 ||
      targetEndDayIndex >= dayCount_ || targetStartDayIndex < 0 ||
      targetStartDayIndex >= dayCount_ || targetEndDayIndex < targetStartDayIndex ||
      !displayTimeZone_.isValid()) {
    return {};
  }
  const auto item = std::find_if(items_.cbegin(), items_.cend(), [&eventId](const Item& candidate) {
    return candidate.event.id == eventId && candidate.allDay && !candidate.startsBeforeRange &&
           !candidate.endsAfterRange;
  });
  if (item == items_.cend()) {
    return {};
  }
  const QDate targetStartDate = rangeStartDate_.addDays(targetStartDayIndex);
  const QDate targetEndDate = rangeStartDate_.addDays(targetEndDayIndex + 1);
  return {{QStringLiteral("id"), item->event.id},
          {QStringLiteral("startAt"),
           QDateTime(targetStartDate, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs)},
          {QStringLiteral("endAt"),
           QDateTime(targetEndDate, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs)}};
}

QVariantMap TimelineModel::timedRangeInput(int firstDayIndex,
                                           int firstMinute,
                                           int lastDayIndex,
                                           int lastMinute) const {
  if (!rangeStartDate_.isValid() || dayCount_ < 1 || !displayTimeZone_.isValid() ||
      firstDayIndex < 0 || firstDayIndex >= dayCount_ || lastDayIndex < 0 ||
      lastDayIndex >= dayCount_) {
    return {};
  }
  firstMinute = clampMinute(firstMinute, false);
  lastMinute = clampMinute(lastMinute, true);
  if (lastDayIndex < firstDayIndex ||
      (lastDayIndex == firstDayIndex && lastMinute < firstMinute)) {
    std::swap(firstDayIndex, lastDayIndex);
    std::swap(firstMinute, lastMinute);
  }
  if (lastDayIndex == firstDayIndex && lastMinute == firstMinute) {
    lastMinute = std::min(kMinutesPerDay, firstMinute + kSnapMinutes);
  }
  QDate endDate = rangeStartDate_.addDays(lastDayIndex);
  if (lastMinute == kMinutesPerDay) {
    endDate = endDate.addDays(1);
  }
  const QDateTime requestedStart(rangeStartDate_.addDays(firstDayIndex),
                                 QTime(firstMinute / 60, firstMinute % 60),
                                 QTimeZone::UTC);
  const QDateTime requestedEnd(endDate,
                               QTime((lastMinute % kMinutesPerDay) / 60,
                                     lastMinute % kMinutesPerDay),
                               QTimeZone::UTC);
  const QDateTime start = resolvedDateTime(rangeStartDate_.addDays(firstDayIndex),
                                           QTime(firstMinute / 60, firstMinute % 60),
                                           displayTimeZone_);
  QDateTime end = resolvedDateTime(endDate,
                                   QTime((lastMinute % kMinutesPerDay) / 60,
                                         lastMinute % kMinutesPerDay),
                                   displayTimeZone_);
  if (end <= start) {
    end = start.addMSecs(std::max<qint64>(kSnapMinutes * 60'000,
                                          requestedStart.msecsTo(requestedEnd)));
  }
  if (!start.isValid() || !end.isValid() || end <= start) {
    return {};
  }
  return {{QStringLiteral("startAt"), start.toUTC().toString(Qt::ISODateWithMs)},
          {QStringLiteral("endAt"), end.toUTC().toString(Qt::ISODateWithMs)},
          {QStringLiteral("allDay"), false}};
}

QVariantMap TimelineModel::allDayRangeInput(int firstDayIndex, int lastDayIndex) const {
  if (!rangeStartDate_.isValid() || dayCount_ < 1 || firstDayIndex < 0 ||
      firstDayIndex >= dayCount_ || lastDayIndex < 0 || lastDayIndex >= dayCount_) {
    return {};
  }
  const int startIndex = std::min(firstDayIndex, lastDayIndex);
  const int endIndex = std::max(firstDayIndex, lastDayIndex);
  return {{QStringLiteral("startAt"),
           QDateTime(rangeStartDate_.addDays(startIndex), QTime(0, 0), QTimeZone::UTC)
               .toString(Qt::ISODateWithMs)},
          {QStringLiteral("endAt"),
           QDateTime(rangeStartDate_.addDays(endIndex + 1), QTime(0, 0), QTimeZone::UTC)
               .toString(Qt::ISODateWithMs)},
          {QStringLiteral("allDay"), true}};
}

QVariantMap TimelineModel::timelinePointInput(double x,
                                              double y,
                                              double availableWidth,
                                              double timeColumnWidth,
                                              double hourHeight,
                                              bool endPoint) const {
  if (dayCount_ < 1 || availableWidth <= timeColumnWidth || hourHeight <= 0.0) {
    return {};
  }
  const double dayWidth = (availableWidth - timeColumnWidth) / static_cast<double>(dayCount_);
  const int dayIndex = std::clamp(static_cast<int>((x - timeColumnWidth) / dayWidth), 0, dayCount_ - 1);
  const int rounded = static_cast<int>(std::llround(y * 60.0 / hourHeight / kSnapMinutes)) *
                      kSnapMinutes;
  return {{QStringLiteral("dayIndex"), dayIndex},
          {QStringLiteral("minute"), clampMinute(rounded, endPoint)}};
}

int TimelineModel::dayIndexForDate(const QString& date) const {
  const QDate parsed = QDate::fromString(date.left(10), Qt::ISODate);
  if (!parsed.isValid() || !rangeStartDate_.isValid()) {
    return -1;
  }
  const qint64 index = rangeStartDate_.daysTo(parsed);
  return index >= 0 && index < dayCount_ ? static_cast<int>(index) : -1;
}

QString TimelineModel::dateForDayIndex(int dayIndex) const {
  return dayIndex >= 0 && dayIndex < dayCount_ && rangeStartDate_.isValid()
             ? rangeStartDate_.addDays(dayIndex).toString(Qt::ISODate)
             : QString();
}

TimelineModel::Layout TimelineModel::buildLayout(QDate startDate,
                                                 int dayCount,
                                                 const QList<CalendarEventSummary>& events,
                                                 const QTimeZone& displayTimeZone,
                                                 int visibleAllDayLaneCount) {
  QList<Item> items;
  if (startDate.isValid() && dayCount >= 1 && dayCount <= 7 && displayTimeZone.isValid() &&
      visibleAllDayLaneCount >= 0) {
    QList<CalendarAllDayLayoutEvent> allDayEvents;
    QList<CalendarEventSummary> allDaySources;
    QHash<QString, int> allDaySourceIndexes;
    for (const CalendarEventSummary& event : events) {
      if (!event.allDay) {
        continue;
      }
      const std::optional<EventRange> range = eventRange(event, displayTimeZone);
      if (!range.has_value()) {
        continue;
      }
      const QString layoutId = QStringLiteral("all-day-%1").arg(allDaySources.size());
      allDayEvents.append({.id = layoutId,
                           .startDayIndex = static_cast<int>(startDate.daysTo(range->firstDate)),
                           .endDayIndex = static_cast<int>(startDate.daysTo(range->lastDate))});
      allDaySourceIndexes.insert(layoutId, static_cast<int>(allDaySources.size()));
      allDaySources.append(event);
    }
    const CalendarAllDayLayout allDayLayout = CalendarLayoutEngine::layoutAllDay(
        std::move(allDayEvents), dayCount, visibleAllDayLaneCount);
    for (const CalendarAllDaySegment& segment : allDayLayout.segments) {
      const int sourceIndex = allDaySourceIndexes.value(segment.id, -1);
      if (sourceIndex < 0 || sourceIndex >= allDaySources.size()) {
        continue;
      }
      items.append({.event = allDaySources.at(sourceIndex),
                    .allDay = true,
                    .dayIndex = segment.startDayIndex,
                    .laneIndex = segment.laneIndex,
                    .daySpan = segment.daySpan,
                    .startsBeforeRange = segment.startsBeforeRange,
                    .endsAfterRange = segment.endsAfterRange});
    }

    int layoutIdentifier = 0;
    for (int dayIndex = 0; dayIndex < dayCount; ++dayIndex) {
      const QDate date = startDate.addDays(dayIndex);
      const QDateTime dayStart(date, QTime(0, 0), displayTimeZone);
      const QDateTime dayEnd = dayStart.addDays(1);
      QList<CalendarTimedLayoutEvent> timedEvents;
      QList<Item> timedSources;
      QHash<QString, int> timedSourceIndexes;
      for (const CalendarEventSummary& event : events) {
        if (event.allDay) {
          continue;
        }
        const std::optional<EventRange> range = eventRange(event, displayTimeZone);
        if (!range.has_value() || range->lastDate < date || range->firstDate > date) {
          continue;
        }
        const QDateTime segmentStart = std::max(range->start, dayStart);
        const QDateTime segmentEnd = std::min(range->end, dayEnd);
        const int startMinute = minuteOfDay(segmentStart, false);
        const int endMinute = segmentEnd == dayEnd ? kMinutesPerDay : minuteOfDay(segmentEnd, true);
        if (endMinute <= startMinute) {
          continue;
        }
        const QString layoutId = QStringLiteral("timed-%1").arg(layoutIdentifier++);
        timedEvents.append({.id = layoutId, .startMinute = startMinute, .endMinute = endMinute});
        timedSourceIndexes.insert(layoutId, static_cast<int>(timedSources.size()));
        timedSources.append({.event = event, .dayIndex = dayIndex});
      }
      const QList<CalendarTimedLayout> timedLayout =
          CalendarLayoutEngine::layoutTimed(std::move(timedEvents));
      for (const CalendarTimedLayout& layout : timedLayout) {
        const int sourceIndex = timedSourceIndexes.value(layout.id, -1);
        if (sourceIndex < 0 || sourceIndex >= timedSources.size()) {
          continue;
        }
        Item item = timedSources.at(sourceIndex);
        item.startMinute = layout.startMinute;
        item.durationMinutes = layout.durationMinutes;
        item.laneIndex = layout.laneIndex;
        item.laneCount = layout.laneCount;
        items.append(std::move(item));
      }
    }
  }
  return {.items = std::move(items),
          .rangeStartDate = startDate,
          .dayCount = dayCount,
          .displayTimeZone = displayTimeZone};
}

void TimelineModel::applyLayout(Layout layout) {
  const int previousTotalItemCount = totalItemCount();
  beginResetModel();
  items_ = std::move(layout.items);
  rangeStartDate_ = layout.rangeStartDate;
  dayCount_ = layout.dayCount;
  displayTimeZone_ = std::move(layout.displayTimeZone);
  endResetModel();
  if (totalItemCount() != previousTotalItemCount) {
    emit totalItemCountChanged();
  }
}

void TimelineModel::setRange(QDate startDate,
                             int dayCount,
                             const QList<CalendarEventSummary>& events,
                             const QTimeZone& displayTimeZone,
                             int visibleAllDayLaneCount) {
  applyLayout(buildLayout(startDate, dayCount, events, displayTimeZone, visibleAllDayLaneCount));
}

} // namespace hcb
