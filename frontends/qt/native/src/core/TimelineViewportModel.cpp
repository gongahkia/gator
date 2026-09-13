#include "core/TimelineViewportModel.h"

#include <QtGlobal>

#include <algorithm>

namespace hcb {
namespace {

constexpr int kMinutesPerDay = 24 * 60;

} // namespace

TimelineViewportModel::TimelineViewportModel(QObject* parent) : QSortFilterProxyModel(parent) {
  setDynamicSortFilter(true);
}

void TimelineViewportModel::setSourceModel(QAbstractItemModel* sourceModel) {
  if (sourceModel == QSortFilterProxyModel::sourceModel()) {
    return;
  }
  updateSourceRoles(sourceModel);
  QSortFilterProxyModel::setSourceModel(sourceModel);
  emit sourceModelChanged();
}

int TimelineViewportModel::firstDayIndex() const { return firstDayIndex_; }

void TimelineViewportModel::setFirstDayIndex(int firstDayIndex) {
  const int bounded = std::max(0, firstDayIndex);
  if (firstDayIndex_ == bounded) {
    return;
  }
  beginFilterChange();
  firstDayIndex_ = bounded;
  endFilterChange(QSortFilterProxyModel::Direction::Rows);
  emit firstDayIndexChanged();
}

int TimelineViewportModel::dayCount() const { return dayCount_; }

void TimelineViewportModel::setDayCount(int dayCount) {
  const int bounded = std::max(1, dayCount);
  if (dayCount_ == bounded) {
    return;
  }
  beginFilterChange();
  dayCount_ = bounded;
  endFilterChange(QSortFilterProxyModel::Direction::Rows);
  emit dayCountChanged();
}

int TimelineViewportModel::visibleStartMinute() const { return visibleStartMinute_; }

void TimelineViewportModel::setVisibleStartMinute(int visibleStartMinute) {
  const int bounded = std::clamp(visibleStartMinute, 0, kMinutesPerDay - 1);
  if (visibleStartMinute_ == bounded || bounded >= visibleEndMinute_) {
    return;
  }
  beginFilterChange();
  visibleStartMinute_ = bounded;
  endFilterChange(QSortFilterProxyModel::Direction::Rows);
  emit visibleStartMinuteChanged();
}

int TimelineViewportModel::visibleEndMinute() const { return visibleEndMinute_; }

void TimelineViewportModel::setVisibleEndMinute(int visibleEndMinute) {
  const int bounded = std::clamp(visibleEndMinute, 1, kMinutesPerDay);
  if (visibleEndMinute_ == bounded || bounded <= visibleStartMinute_) {
    return;
  }
  beginFilterChange();
  visibleEndMinute_ = bounded;
  endFilterChange(QSortFilterProxyModel::Direction::Rows);
  emit visibleEndMinuteChanged();
}

bool TimelineViewportModel::allDay() const { return allDay_; }

void TimelineViewportModel::setAllDay(bool allDay) {
  if (allDay_ == allDay) {
    return;
  }
  beginFilterChange();
  allDay_ = allDay;
  endFilterChange(QSortFilterProxyModel::Direction::Rows);
  emit allDayChanged();
}

bool TimelineViewportModel::active() const { return active_; }

void TimelineViewportModel::setActive(bool active) {
  if (active_ == active) {
    return;
  }
  beginFilterChange();
  active_ = active;
  endFilterChange(QSortFilterProxyModel::Direction::Rows);
  emit activeChanged();
}

bool TimelineViewportModel::filterCalendarVisibility() const {
  return filterCalendarVisibility_;
}

void TimelineViewportModel::setFilterCalendarVisibility(bool filterCalendarVisibility) {
  if (filterCalendarVisibility_ == filterCalendarVisibility) {
    return;
  }
  beginFilterChange();
  filterCalendarVisibility_ = filterCalendarVisibility;
  endFilterChange(QSortFilterProxyModel::Direction::Rows);
  emit filterCalendarVisibilityChanged();
}

QStringList TimelineViewportModel::visibleCalendarIds() const { return visibleCalendarIds_; }

void TimelineViewportModel::setVisibleCalendarIds(const QStringList& visibleCalendarIds) {
  if (visibleCalendarIds_ == visibleCalendarIds) {
    return;
  }
  beginFilterChange();
  visibleCalendarIds_ = visibleCalendarIds;
  endFilterChange(QSortFilterProxyModel::Direction::Rows);
  emit visibleCalendarIdsChanged();
}

bool TimelineViewportModel::filterAcceptsRow(int sourceRow,
                                              const QModelIndex& sourceParent) const {
  if (!active_ || sourceModel() == nullptr) {
    return false;
  }
  const int allDayRole = roleForName(QByteArrayLiteral("allDay"));
  const int dayIndexRole = roleForName(QByteArrayLiteral("dayIndex"));
  if (allDayRole < 0 || dayIndexRole < 0) {
    return false;
  }
  const QModelIndex index = sourceModel()->index(sourceRow, 0, sourceParent);
  if (!index.isValid() || sourceModel()->data(index, allDayRole).toBool() != allDay_) {
    return false;
  }
  const int dayIndex = sourceModel()->data(index, dayIndexRole).toInt();
  if (allDay_) {
    const int daySpanRole = roleForName(QByteArrayLiteral("daySpan"));
    const int daySpan = daySpanRole < 0 ? 1
                                        : std::max(1, sourceModel()->data(index, daySpanRole).toInt());
    if (dayIndex >= firstDayIndex_ + dayCount_ || dayIndex + daySpan <= firstDayIndex_) {
      return false;
    }
  } else {
    if (dayIndex < firstDayIndex_ || dayIndex >= firstDayIndex_ + dayCount_) {
      return false;
    }
    const int startMinuteRole = roleForName(QByteArrayLiteral("startMinute"));
    const int durationMinutesRole = roleForName(QByteArrayLiteral("durationMinutes"));
    if (startMinuteRole < 0 || durationMinutesRole < 0) {
      return false;
    }
    const int startMinute = sourceModel()->data(index, startMinuteRole).toInt();
    const int endMinute = startMinute + std::max(1, sourceModel()->data(index, durationMinutesRole).toInt());
    if (startMinute >= visibleEndMinute_ || endMinute <= visibleStartMinute_) {
      return false;
    }
  }
  if (!filterCalendarVisibility_) {
    return true;
  }
  const int calendarIdRole = roleForName(QByteArrayLiteral("calendarId"));
  return calendarIdRole >= 0 &&
         visibleCalendarIds_.contains(sourceModel()->data(index, calendarIdRole).toString());
}

int TimelineViewportModel::roleForName(const QByteArray& name) const {
  return sourceRoles_.value(name, -1);
}

void TimelineViewportModel::updateSourceRoles(QAbstractItemModel* sourceModel) {
  sourceRoles_.clear();
  if (sourceModel == nullptr) {
    return;
  }
  const QHash<int, QByteArray> roles = sourceModel->roleNames();
  for (auto iterator = roles.cbegin(); iterator != roles.cend(); ++iterator) {
    sourceRoles_.insert(iterator.value(), iterator.key());
  }
}

} // namespace hcb
