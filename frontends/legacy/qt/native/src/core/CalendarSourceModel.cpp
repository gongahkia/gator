#include "core/CalendarSourceModel.h"
#include "core/ModelDiffPolicy.h"

#include <utility>

namespace hcb {

namespace {

[[nodiscard]] bool equivalentCalendar(const CalendarSummary& left, const CalendarSummary& right) {
  return left.id == right.id && left.accountId == right.accountId &&
         left.remoteId == right.remoteId && left.title == right.title &&
         left.description == right.description && left.timeZone == right.timeZone &&
         left.colorId == right.colorId &&
         left.backgroundColor == right.backgroundColor &&
         left.foregroundColor == right.foregroundColor && left.accessRole == right.accessRole &&
         left.selected == right.selected && left.hidden == right.hidden && left.primary == right.primary &&
         left.etag == right.etag && left.remoteUpdatedAt == right.remoteUpdatedAt &&
         left.updatedAt == right.updatedAt && left.eventCount == right.eventCount;
}

} // namespace

CalendarSourceModel::CalendarSourceModel(QObject* parent) : QAbstractListModel(parent) {}

int CalendarSourceModel::rowCount(const QModelIndex& parent) const {
  return parent.isValid() ? 0 : static_cast<int>(calendars_.size());
}

QVariant CalendarSourceModel::data(const QModelIndex& index, int role) const {
  if (!index.isValid() || index.row() < 0 || index.row() >= static_cast<int>(calendars_.size())) {
    return {};
  }
  const CalendarSummary& calendar = calendars_.at(index.row());
  switch (role) {
  case Qt::DisplayRole:
  case TitleRole:
    return calendar.title;
  case IdRole:
    return calendar.id;
  case AccountIdRole:
    return calendar.accountId;
  case DescriptionRole:
    return calendar.description.value_or(QString());
  case TimeZoneRole:
    return calendar.timeZone.value_or(QString());
  case ColorIdRole:
    return calendar.colorId.value_or(QString());
  case BackgroundColorRole:
    return calendar.backgroundColor.value_or(QString());
  case ForegroundColorRole:
    return calendar.foregroundColor.value_or(QString());
  case AccessRoleRole:
    return calendar.accessRole.value_or(QString());
  case SelectedRole:
    return calendar.selected;
  case HiddenRole:
    return calendar.hidden;
  case PrimaryRole:
    return calendar.primary;
  case EventCountRole:
    return static_cast<qlonglong>(calendar.eventCount);
  default:
    return {};
  }
}

QHash<int, QByteArray> CalendarSourceModel::roleNames() const {
  return {{IdRole, "id"},
          {AccountIdRole, "accountId"},
          {TitleRole, "title"},
          {DescriptionRole, "description"},
          {TimeZoneRole, "timeZone"},
          {ColorIdRole, "colorId"},
          {BackgroundColorRole, "backgroundColor"},
          {ForegroundColorRole, "foregroundColor"},
          {AccessRoleRole, "accessRole"},
          {SelectedRole, "selected"},
          {HiddenRole, "hidden"},
          {PrimaryRole, "primary"},
          {EventCountRole, "eventCount"}};
}

int CalendarSourceModel::revision() const { return revision_; }

QStringList CalendarSourceModel::calendarIds() const {
  QStringList ids;
  ids.reserve(calendars_.size());
  for (const CalendarSummary& calendar : calendars_) {
    ids.append(calendar.id);
  }
  return ids;
}

QStringList CalendarSourceModel::selectedCalendarIds() const {
  QStringList ids;
  for (const CalendarSummary& calendar : calendars_) {
    if (calendar.selected) {
      ids.append(calendar.id);
    }
  }
  return ids;
}

QString CalendarSourceModel::calendarTitle(const QString& calendarId) const {
  for (const CalendarSummary& calendar : calendars_) {
    if (calendar.id == calendarId) {
      return calendar.title;
    }
  }
  return {};
}

QString CalendarSourceModel::calendarBackgroundColor(const QString& calendarId) const {
  for (const CalendarSummary& calendar : calendars_) {
    if (calendar.id == calendarId) {
      return calendar.backgroundColor.value_or(QString());
    }
  }
  return {};
}

void CalendarSourceModel::setCalendars(QList<CalendarSummary> calendars) {
  const ModelDiffPlan plan = ModelDiffPolicy::plan(
      calendars_,
      calendars,
      [](const CalendarSummary& calendar) -> const QString& { return calendar.id; },
      equivalentCalendar);
  if (plan.requiresReset) {
    beginResetModel();
    calendars_ = std::move(calendars);
    endResetModel();
    ++revision_;
    emit revisionChanged();
    return;
  }
  calendars_ = std::move(calendars);
  for (const ModelDataChangeRange& range : plan.changedRanges) {
    emit dataChanged(index(range.firstRow, 0), index(range.lastRow, 0));
  }
  ++revision_;
  emit revisionChanged();
}

} // namespace hcb
