#include "core/MonthGridModel.h"

#include "core/CalendarLayoutEngine.h"

#include <QDateTime>
#include <QSet>
#include <QVariantList>
#include <QVariantMap>

#include <algorithm>
#include <limits>
#include <optional>
#include <utility>

namespace hcb {

namespace {

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

[[nodiscard]] std::optional<QPair<QDate, QDate>> eventDateRange(const CalendarEventSummary& event,
                                                                const QTimeZone& displayTimeZone) {
  if (event.allDay) {
    const QDate startDate = QDate::fromString(event.startAt.left(10), Qt::ISODate);
    const QDate endDate = QDate::fromString(event.endAt.left(10), Qt::ISODate);
    return startDate.isValid() && endDate.isValid() && endDate > startDate
               ? std::optional<QPair<QDate, QDate>>(qMakePair(startDate, endDate.addDays(-1)))
               : std::nullopt;
  }
  const std::optional<QDateTime> start = parseDateTime(event.startAt, displayTimeZone);
  const std::optional<QDateTime> end = parseDateTime(event.endAt, displayTimeZone);
  if (!start.has_value() || !end.has_value() || *end <= *start) {
    return std::nullopt;
  }
  QDate last = end->date();
  if (end->time() == QTime(0, 0)) {
    last = last.addDays(-1);
  }
  return last < start->date() ? std::nullopt
                              : std::optional<QPair<QDate, QDate>>(qMakePair(start->date(), last));
}

[[nodiscard]] QVariantMap eventMap(const CalendarEventSummary& event) {
  return {{QStringLiteral("id"), event.id},
          {QStringLiteral("calendarId"), event.calendarId},
          {QStringLiteral("recurringRemoteId"), event.recurringRemoteId.value_or(QString())},
          {QStringLiteral("originalStartAt"), event.originalStartAt.value_or(QString())},
          {QStringLiteral("recurrenceRule"), event.recurrenceRule.value_or(QString())},
          {QStringLiteral("title"), event.title},
          {QStringLiteral("status"), event.status},
          {QStringLiteral("startAt"), event.startAt},
          {QStringLiteral("endAt"), event.endAt},
          {QStringLiteral("allDay"), event.allDay},
          {QStringLiteral("colorId"), event.colorId.value_or(QString())},
          {QStringLiteral("description"), event.description.value_or(QString())},
          {QStringLiteral("location"), event.location.value_or(QString())},
          {QStringLiteral("startTimeZone"), event.startTimeZone.value_or(QString())},
          {QStringLiteral("endTimeZone"), event.endTimeZone.value_or(QString())},
          {QStringLiteral("transparency"), event.transparency.value_or(QString())},
          {QStringLiteral("visibility"), event.visibility.value_or(QString())},
          {QStringLiteral("attendeeEmailsJson"), event.attendeeEmailsJson},
          {QStringLiteral("attendeeDetailsJson"), event.attendeeDetailsJson},
          {QStringLiteral("remindersJson"), event.remindersJson},
          {QStringLiteral("remindersUseDefault"), event.remindersUseDefault},
          {QStringLiteral("conferenceJson"), event.conferenceJson.value_or(QString())},
          {QStringLiteral("attachmentsJson"), event.attachmentsJson},
          {QStringLiteral("guestPermissionsJson"), event.guestPermissionsJson},
          {QStringLiteral("statusPropertiesJson"), event.statusPropertiesJson},
          {QStringLiteral("eventType"), event.eventType.value_or(QStringLiteral("default"))}};
}

[[nodiscard]] QVariantMap allDaySpanMap(const MonthGridModel::AllDaySpan& span) {
  QVariantMap result = eventMap(span.event);
  result.insert(QStringLiteral("weekIndex"), span.weekIndex);
  result.insert(QStringLiteral("startColumn"), span.startColumn);
  result.insert(QStringLiteral("daySpan"), span.daySpan);
  result.insert(QStringLiteral("laneIndex"), span.laneIndex);
  result.insert(QStringLiteral("startsBeforeRange"), span.startsBeforeRange);
  result.insert(QStringLiteral("endsAfterRange"), span.endsAfterRange);
  return result;
}

[[nodiscard]] std::optional<QDateTime> mapDateTime(const QVariantMap& event,
                                                    const QString& key,
                                                    const QTimeZone& timeZone) {
  return parseDateTime(event.value(key).toString(), timeZone);
}

} // namespace

MonthGridModel::MonthGridModel(QObject* parent) : QAbstractTableModel(parent) {}

int MonthGridModel::rowCount(const QModelIndex& parent) const {
  return parent.isValid() || cells_.isEmpty() ? 0 : 6;
}

int MonthGridModel::columnCount(const QModelIndex& parent) const {
  return parent.isValid() ? 0 : 7;
}

QVariant MonthGridModel::data(const QModelIndex& index, int role) const {
  if (!index.isValid() || index.row() < 0 || index.row() >= rowCount() || index.column() < 0 ||
      index.column() >= columnCount()) {
    return {};
  }
  const Cell& cell = cells_.at(index.row() * columnCount() + index.column());
  switch (role) {
  case Qt::DisplayRole:
  case DateRole:
    return cell.date.toString(Qt::ISODate);
  case DayRole:
    return cell.date.day();
  case OutsideMonthRole:
    return cell.date.month() != month_.month() || cell.date.year() != month_.year();
  case EventCountRole:
    return cell.events.size();
  case EventsRole: {
    QVariantList events;
    events.reserve(cell.events.size());
    for (const CalendarEventSummary& event : cell.events) {
      events.append(eventMap(event));
    }
    return events;
  }
  case AllDayOverflowCountRole:
    return allDayOverflowCounts_.value(index.row() * columnCount() + index.column());
  case VisibleTimedEventsRole:
    return visibleTimedEvents_.value(index.row() * columnCount() + index.column());
  case VisibleAllDayEventsRole:
    return visibleAllDayEvents_.value(index.row() * columnCount() + index.column());
  case HiddenAllDayCountRole:
    return hiddenAllDayCounts_.value(index.row() * columnCount() + index.column());
  default:
    return {};
  }
}

QHash<int, QByteArray> MonthGridModel::roleNames() const {
  return {{DateRole, "date"},
          {DayRole, "day"},
          {OutsideMonthRole, "outsideMonth"},
          {EventCountRole, "eventCount"},
          {EventsRole, "events"},
          {AllDayOverflowCountRole, "allDayOverflowCount"},
          {VisibleTimedEventsRole, "visibleTimedEvents"},
          {VisibleAllDayEventsRole, "visibleAllDayEvents"},
          {HiddenAllDayCountRole, "hiddenAllDayCount"}};
}

QVariantList MonthGridModel::allDaySpans() const {
  QVariantList spans;
  spans.reserve(allDaySpans_.size());
  for (const AllDaySpan& span : allDaySpans_) {
    spans.append(allDaySpanMap(span));
  }
  return spans;
}

QVariantList MonthGridModel::visibleAllDaySpans() const { return visibleAllDaySpanMaps_; }

QStringList MonthGridModel::visibleCalendarIds() const { return visibleCalendarIds_; }

void MonthGridModel::setVisibleCalendarIds(QStringList visibleCalendarIds) {
  visibleCalendarIds.removeDuplicates();
  std::sort(visibleCalendarIds.begin(), visibleCalendarIds.end());
  if (filterCalendarVisibility_ && visibleCalendarIds_ == visibleCalendarIds) {
    return;
  }
  filterCalendarVisibility_ = true;
  visibleCalendarIds_ = std::move(visibleCalendarIds);
  rebuildPresentation(true);
}

int MonthGridModel::visibleAllDayLanes() const { return visibleAllDayLanes_; }

void MonthGridModel::setVisibleAllDayLanes(int visibleAllDayLanes) {
  const int normalized = std::max(0, visibleAllDayLanes);
  if (visibleAllDayLanes_ == normalized) {
    return;
  }
  visibleAllDayLanes_ = normalized;
  rebuildPresentation(true);
}

QString MonthGridModel::dateForPoint(double x, double y, double width, double height) const {
  if (cells_.size() != 42 || width <= 0.0 || height <= 0.0) {
    return {};
  }
  const int column = std::clamp(static_cast<int>(x / (width / 7.0)), 0, 6);
  const int row = std::clamp(static_cast<int>(y / (height / 6.0)), 0, 5);
  return cells_.at(row * 7 + column).date.toString(Qt::ISODate);
}

int MonthGridModel::dateIndex(const QString& date) const {
  const QDate parsed = QDate::fromString(date.left(10), Qt::ISODate);
  if (!parsed.isValid() || cells_.isEmpty()) {
    return -1;
  }
  const qint64 index = cells_.first().date.daysTo(parsed);
  return index >= 0 && index < cells_.size() ? static_cast<int>(index) : -1;
}

QString MonthGridModel::dateForIndex(int dayIndex) const {
  if (dayIndex < 0 || dayIndex >= cells_.size()) {
    return {};
  }
  return cells_.at(dayIndex).date.toString(Qt::ISODate);
}

QVariantMap MonthGridModel::allDayRangeInput(int firstDayIndex, int lastDayIndex) const {
  if (cells_.size() != 42 || firstDayIndex < 0 || firstDayIndex >= cells_.size() ||
      lastDayIndex < 0 || lastDayIndex >= cells_.size()) {
    return {};
  }
  const int first = std::min(firstDayIndex, lastDayIndex);
  const int last = std::max(firstDayIndex, lastDayIndex);
  return {{QStringLiteral("startAt"),
           QDateTime(cells_.at(first).date, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs)},
          {QStringLiteral("endAt"),
           QDateTime(cells_.at(last).date.addDays(1), QTime(0, 0), QTimeZone::UTC)
               .toString(Qt::ISODateWithMs)},
          {QStringLiteral("allDay"), true}};
}

QVariantMap MonthGridModel::moveInput(const QVariantMap& event, int targetDayIndex) const {
  if (cells_.size() != 42 || !displayTimeZone_.isValid() || targetDayIndex < 0 ||
      targetDayIndex >= cells_.size()) {
    return {};
  }
  const bool allDay = event.value(QStringLiteral("allDay")).toBool();
  const QString id = event.value(QStringLiteral("id")).toString();
  if (id.isEmpty()) {
    return {};
  }
  if (allDay) {
    const QDate source = QDate::fromString(event.value(QStringLiteral("startAt")).toString().left(10), Qt::ISODate);
    const QDate end = QDate::fromString(event.value(QStringLiteral("endAt")).toString().left(10), Qt::ISODate);
    if (!source.isValid() || !end.isValid() || end <= source) {
      return {};
    }
    const QDate target = cells_.at(targetDayIndex).date;
    const qint64 duration = source.daysTo(end);
    return {{QStringLiteral("id"), id},
            {QStringLiteral("startAt"), QDateTime(target, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs)},
            {QStringLiteral("endAt"), QDateTime(target.addDays(duration), QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs)},
            {QStringLiteral("allDay"), true}};
  }
  const std::optional<QDateTime> start = mapDateTime(event, QStringLiteral("startAt"), displayTimeZone_);
  const std::optional<QDateTime> end = mapDateTime(event, QStringLiteral("endAt"), displayTimeZone_);
  if (!start.has_value() || !end.has_value() || *end <= *start) {
    return {};
  }
  const QDateTime targetStart(cells_.at(targetDayIndex).date, start->time(), displayTimeZone_,
                              QDateTime::TransitionResolution::PreferAfter);
  const QDateTime targetEnd = targetStart.addMSecs(start->msecsTo(*end));
  if (!targetStart.isValid() || !targetEnd.isValid()) {
    return {};
  }
  return {{QStringLiteral("id"), id},
          {QStringLiteral("startAt"), targetStart.toUTC().toString(Qt::ISODateWithMs)},
          {QStringLiteral("endAt"), targetEnd.toUTC().toString(Qt::ISODateWithMs)},
          {QStringLiteral("allDay"), false}};
}

QVariantMap MonthGridModel::resizeAllDayRangeInput(const QVariantMap& event,
                                                    int firstDayIndex,
                                                    int lastDayIndex) const {
  const QString id = event.value(QStringLiteral("id")).toString();
  if (id.isEmpty() || !event.value(QStringLiteral("allDay")).toBool()) {
    return {};
  }
  QVariantMap range = allDayRangeInput(firstDayIndex, lastDayIndex);
  if (!range.isEmpty()) {
    range.insert(QStringLiteral("id"), id);
  }
  return range;
}

MonthGridModel::Layout MonthGridModel::buildLayout(QDate month,
                                                   const QList<CalendarEventSummary>& events,
                                                   const QTimeZone& displayTimeZone,
                                                   int weekStartDay) {
  QList<Cell> cells;
  QList<AllDaySpan> allDaySpans;
  QList<int> allDayOverflowCounts;
  if (month.isValid() && displayTimeZone.isValid() && (weekStartDay == 0 || weekStartDay == 1)) {
    const QDate firstDay(month.year(), month.month(), 1);
    const int firstQtDay = weekStartDay == 0 ? 7 : 1;
    const QDate gridStart = firstDay.addDays(-(firstDay.dayOfWeek() - firstQtDay + 7) % 7);
    cells.reserve(42);
    for (int dayOffset = 0; dayOffset < 42; ++dayOffset) {
      cells.append(Cell{.date = gridStart.addDays(dayOffset)});
    }
    for (const CalendarEventSummary& event : events) {
      const std::optional<QPair<QDate, QDate>> dates = eventDateRange(event, displayTimeZone);
      if (!dates.has_value()) {
        continue;
      }
      for (Cell& cell : cells) {
        if (cell.date >= dates->first && cell.date <= dates->second) {
          cell.events.append(event);
        }
      }
    }
    QList<CalendarAllDayLayoutEvent> allDayEvents;
    QList<CalendarEventSummary> allDaySources;
    const auto gridDayIndex = [&gridStart](const QDate& date) {
      const qint64 index = gridStart.daysTo(date);
      return static_cast<int>(std::clamp(index,
                                         static_cast<qint64>(std::numeric_limits<int>::min()),
                                         static_cast<qint64>(std::numeric_limits<int>::max())));
    };
    for (const CalendarEventSummary& event : events) {
      if (!event.allDay) {
        continue;
      }
      const std::optional<QPair<QDate, QDate>> dates = eventDateRange(event, displayTimeZone);
      if (!dates.has_value()) {
        continue;
      }
      allDayEvents.append({.id = QString::number(allDaySources.size()),
                           .startDayIndex = gridDayIndex(dates->first),
                           .endDayIndex = gridDayIndex(dates->second)});
      allDaySources.append(event);
    }
    const CalendarAllDayLayout layout = CalendarLayoutEngine::layoutAllDay(
        allDayEvents, 42, std::max(1, static_cast<int>(allDayEvents.size())));
    allDayOverflowCounts.fill(0, 42);
    for (const CalendarAllDaySegment& segment : layout.segments) {
      bool validIndex = false;
      const int sourceIndex = segment.id.toInt(&validIndex);
      if (!validIndex || sourceIndex < 0 || sourceIndex >= allDaySources.size()) {
        continue;
      }
      const int segmentEnd = segment.startDayIndex + segment.daySpan - 1;
      for (int start = segment.startDayIndex; start <= segmentEnd;) {
        const int weekEnd = (start / 7) * 7 + 6;
        const int end = std::min(segmentEnd, weekEnd);
        const bool startsBeforeRange = segment.startsBeforeRange || start > segment.startDayIndex;
        const bool endsAfterRange = segment.endsAfterRange || end < segmentEnd;
        allDaySpans.append({.event = allDaySources.at(sourceIndex),
                            .weekIndex = start / 7,
                            .startColumn = start % 7,
                            .daySpan = end - start + 1,
                            .laneIndex = segment.laneIndex,
                            .startsBeforeRange = startsBeforeRange,
                            .endsAfterRange = endsAfterRange});
        if (segment.laneIndex >= 3) {
          for (int day = start; day <= end; ++day) {
            ++allDayOverflowCounts[day];
          }
        }
        start = end + 1;
      }
    }
  }
  return {.month = month,
          .cells = std::move(cells),
          .allDaySpans = std::move(allDaySpans),
          .allDayOverflowCounts = std::move(allDayOverflowCounts),
          .displayTimeZone = displayTimeZone};
}

void MonthGridModel::applyLayout(Layout layout) {
  beginResetModel();
  month_ = layout.month;
  cells_ = std::move(layout.cells);
  allDaySpans_ = std::move(layout.allDaySpans);
  allDayOverflowCounts_ = std::move(layout.allDayOverflowCounts);
  displayTimeZone_ = std::move(layout.displayTimeZone);
  endResetModel();
  rebuildPresentation(false);
  emit allDaySpansChanged();
}

void MonthGridModel::setMonth(QDate month,
                              const QList<CalendarEventSummary>& events,
                              const QTimeZone& displayTimeZone,
                              int weekStartDay) {
  applyLayout(buildLayout(month, events, displayTimeZone, weekStartDay));
}

bool MonthGridModel::isCalendarVisible(const QString& calendarId) const {
  return !filterCalendarVisibility_ || visibleCalendarIds_.contains(calendarId);
}

void MonthGridModel::rebuildPresentation(bool notifyViews) {
  visibleTimedEvents_.clear();
  visibleAllDayEvents_.clear();
  hiddenAllDayCounts_.clear();
  visibleAllDaySpanMaps_.clear();
  visibleTimedEvents_.reserve(cells_.size());
  visibleAllDayEvents_.reserve(cells_.size());
  hiddenAllDayCounts_.fill(0, cells_.size());

  for (const Cell& cell : cells_) {
    QVariantList timed;
    QVariantList allDay;
    for (const CalendarEventSummary& event : cell.events) {
      if (!isCalendarVisible(event.calendarId)) {
        continue;
      }
      (event.allDay ? allDay : timed).append(eventMap(event));
    }
    visibleTimedEvents_.append(std::move(timed));
    visibleAllDayEvents_.append(std::move(allDay));
  }

  QList<QSet<QString>> visibleSpanIds;
  visibleSpanIds.resize(cells_.size());
  for (const AllDaySpan& span : allDaySpans_) {
    if (!isCalendarVisible(span.event.calendarId) || span.laneIndex >= visibleAllDayLanes_) {
      continue;
    }
    visibleAllDaySpanMaps_.append(allDaySpanMap(span));
    const int first = span.weekIndex * 7 + span.startColumn;
    const int last = std::min(static_cast<int>(cells_.size()) - 1, first + span.daySpan - 1);
    for (int day = std::max(0, first); day <= last; ++day) {
      visibleSpanIds[day].insert(span.event.id);
    }
  }
  for (int row = 0; row < visibleAllDayEvents_.size(); ++row) {
    for (const QVariant& event : visibleAllDayEvents_.at(row)) {
      if (!visibleSpanIds.at(row).contains(event.toMap().value(QStringLiteral("id")).toString())) {
        ++hiddenAllDayCounts_[row];
      }
    }
  }

  if (notifyViews && !cells_.isEmpty()) {
    emit dataChanged(index(0, 0), index(rowCount() - 1, columnCount() - 1),
                     {VisibleTimedEventsRole, VisibleAllDayEventsRole, HiddenAllDayCountRole});
  }
  emit presentationChanged();
}

} // namespace hcb
