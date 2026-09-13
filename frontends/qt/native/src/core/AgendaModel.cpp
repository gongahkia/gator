#include "core/AgendaModel.h"
#include "core/ModelDiffPolicy.h"

#include <QDate>
#include <QDateTime>

#include <algorithm>
#include <utility>

namespace hcb {

namespace {

[[nodiscard]] bool equivalentEvent(const CalendarEventSummary& left,
                                   const CalendarEventSummary& right) {
  return left.id == right.id && left.calendarId == right.calendarId &&
         left.remoteId == right.remoteId && left.recurringRemoteId == right.recurringRemoteId &&
         left.originalStartAt == right.originalStartAt && left.status == right.status &&
         left.title == right.title && left.description == right.description &&
         left.location == right.location && left.startAt == right.startAt &&
         left.startTimeZone == right.startTimeZone && left.endAt == right.endAt &&
         left.endTimeZone == right.endTimeZone && left.allDay == right.allDay &&
         left.recurrenceRule == right.recurrenceRule && left.colorId == right.colorId &&
         left.transparency == right.transparency && left.visibility == right.visibility &&
         left.timeZone == right.timeZone && left.hcbKind == right.hcbKind &&
         left.attendeeEmailsJson == right.attendeeEmailsJson &&
         left.attendeeDetailsJson == right.attendeeDetailsJson &&
         left.remindersJson == right.remindersJson &&
         left.remindersUseDefault == right.remindersUseDefault &&
         left.conferenceJson == right.conferenceJson && left.attachmentsJson == right.attachmentsJson &&
         left.guestPermissionsJson == right.guestPermissionsJson &&
         left.statusPropertiesJson == right.statusPropertiesJson && left.eventType == right.eventType &&
         left.etag == right.etag && left.sequence == right.sequence &&
         left.remoteUpdatedAt == right.remoteUpdatedAt && left.updatedAt == right.updatedAt;
}

[[nodiscard]] QDate eventDate(const CalendarEventSummary& event) {
  const QDate date = QDate::fromString(event.startAt.left(10), Qt::ISODate);
  return date;
}

[[nodiscard]] QString agendaDay(const CalendarEventSummary& event) {
  const QDate date = eventDate(event);
  return date.isValid() ? date.toString(Qt::ISODate) : QString();
}

[[nodiscard]] QString agendaWeek(const CalendarEventSummary& event) {
  const QDate date = eventDate(event);
  return date.isValid() ? date.addDays(1 - date.dayOfWeek()).toString(Qt::ISODate) : QString();
}

[[nodiscard]] QVariantMap eventMap(const CalendarEventSummary& event) {
  return {{QStringLiteral("id"), event.id},
          {QStringLiteral("calendarId"), event.calendarId},
          {QStringLiteral("recurringRemoteId"), event.recurringRemoteId.value_or(QString())},
          {QStringLiteral("originalStartAt"), event.originalStartAt.value_or(QString())},
          {QStringLiteral("recurrenceRule"), event.recurrenceRule.value_or(QString())},
          {QStringLiteral("status"), event.status},
          {QStringLiteral("title"), event.title},
          {QStringLiteral("description"), event.description.value_or(QString())},
          {QStringLiteral("location"), event.location.value_or(QString())},
          {QStringLiteral("startAt"), event.startAt},
          {QStringLiteral("startTimeZone"), event.startTimeZone.value_or(QString())},
          {QStringLiteral("endAt"), event.endAt},
          {QStringLiteral("endTimeZone"), event.endTimeZone.value_or(QString())},
          {QStringLiteral("allDay"), event.allDay},
          {QStringLiteral("colorId"), event.colorId.value_or(QString())},
          {QStringLiteral("transparency"), event.transparency.value_or(QString())},
          {QStringLiteral("visibility"), event.visibility.value_or(QString())},
          {QStringLiteral("attendeeEmailsJson"), event.attendeeEmailsJson},
          {QStringLiteral("remindersJson"), event.remindersJson},
          {QStringLiteral("remindersUseDefault"), event.remindersUseDefault},
          {QStringLiteral("eventType"), event.eventType.value_or(QStringLiteral("default"))},
          {QStringLiteral("conferenceJson"), event.conferenceJson.value_or(QString())},
          {QStringLiteral("attachmentsJson"), event.attachmentsJson},
          {QStringLiteral("guestPermissionsJson"), event.guestPermissionsJson},
          {QStringLiteral("statusPropertiesJson"), event.statusPropertiesJson}};
}

} // namespace

AgendaModel::AgendaModel(QObject* parent) : QAbstractListModel(parent) {}

int AgendaModel::rowCount(const QModelIndex& parent) const {
  return parent.isValid() ? 0 : static_cast<int>(events_.size());
}

QVariant AgendaModel::data(const QModelIndex& index, int role) const {
  if (!index.isValid() || index.row() < 0 || index.row() >= static_cast<int>(events_.size())) {
    return {};
  }
  const CalendarEventSummary& event = events_.at(index.row());
  switch (role) {
  case Qt::DisplayRole:
  case TitleRole:
    return event.title;
  case IdRole:
    return event.id;
  case CalendarIdRole:
    return event.calendarId;
  case RecurringRemoteIdRole:
    return event.recurringRemoteId.value_or(QString());
  case OriginalStartAtRole:
    return event.originalStartAt.value_or(QString());
  case RecurrenceRuleRole:
    return event.recurrenceRule.value_or(QString());
  case StatusRole:
    return event.status;
  case DescriptionRole:
    return event.description.value_or(QString());
  case LocationRole:
    return event.location.value_or(QString());
  case StartAtRole:
    return event.startAt;
  case StartTimeZoneRole:
    return event.startTimeZone.value_or(QString());
  case EndAtRole:
    return event.endAt;
  case EndTimeZoneRole:
    return event.endTimeZone.value_or(QString());
  case AllDayRole:
    return event.allDay;
  case ColorIdRole:
    return event.colorId.value_or(QString());
  case TransparencyRole:
    return event.transparency.value_or(QString());
  case VisibilityRole:
    return event.visibility.value_or(QString());
  case HcbKindRole:
    return event.hcbKind.value_or(QString());
  case AttendeeEmailsJsonRole:
    return event.attendeeEmailsJson;
  case AttendeeDetailsJsonRole:
    return event.attendeeDetailsJson;
  case RemindersJsonRole:
    return event.remindersJson;
  case RemindersUseDefaultRole:
    return event.remindersUseDefault;
  case ConferenceJsonRole:
    return event.conferenceJson.value_or(QString());
  case AttachmentsJsonRole:
    return event.attachmentsJson;
  case GuestPermissionsJsonRole:
    return event.guestPermissionsJson;
  case StatusPropertiesJsonRole:
    return event.statusPropertiesJson;
  case EventTypeRole:
    return event.eventType.value_or(QStringLiteral("default"));
  case AgendaDayRole:
    return agendaDay(event);
  case AgendaWeekRole:
    return agendaWeek(event);
  default:
    return {};
  }
}

QHash<int, QByteArray> AgendaModel::roleNames() const {
  return {{IdRole, "id"},
          {CalendarIdRole, "calendarId"},
          {RecurringRemoteIdRole, "recurringRemoteId"},
          {OriginalStartAtRole, "originalStartAt"},
          {RecurrenceRuleRole, "recurrenceRule"},
          {StatusRole, "status"},
          {TitleRole, "title"},
          {DescriptionRole, "description"},
          {LocationRole, "location"},
          {StartAtRole, "startAt"},
          {StartTimeZoneRole, "startTimeZone"},
          {EndAtRole, "endAt"},
          {EndTimeZoneRole, "endTimeZone"},
          {AllDayRole, "allDay"},
          {ColorIdRole, "colorId"},
          {TransparencyRole, "transparency"},
          {VisibilityRole, "visibility"},
          {HcbKindRole, "hcbKind"},
          {AttendeeEmailsJsonRole, "attendeeEmailsJson"},
          {AttendeeDetailsJsonRole, "attendeeDetailsJson"},
          {RemindersJsonRole, "remindersJson"},
          {RemindersUseDefaultRole, "remindersUseDefault"},
          {ConferenceJsonRole, "conferenceJson"},
          {AttachmentsJsonRole, "attachmentsJson"},
          {GuestPermissionsJsonRole, "guestPermissionsJson"},
          {StatusPropertiesJsonRole, "statusPropertiesJson"},
          {EventTypeRole, "eventType"},
          {AgendaDayRole, "agendaDay"},
          {AgendaWeekRole, "agendaWeek"}};
}

int AgendaModel::rowForEvent(QString eventId) const {
  for (qsizetype row = 0; row < events_.size(); ++row) {
    if (events_.at(row).id == eventId) {
      return static_cast<int>(row);
    }
  }
  return -1;
}

QVariantMap AgendaModel::eventForId(QString eventId) const {
  const auto found = std::find_if(events_.cbegin(), events_.cend(), [&eventId](const auto& event) {
    return event.id == eventId;
  });
  return found == events_.cend() ? QVariantMap() : eventMap(*found);
}

void AgendaModel::setEvents(QList<CalendarEventSummary> events) {
  const ModelDiffPlan plan = ModelDiffPolicy::plan(
      events_,
      events,
      [](const CalendarEventSummary& event) -> const QString& { return event.id; },
      equivalentEvent);
  if (plan.requiresReset) {
    beginResetModel();
    events_ = std::move(events);
    endResetModel();
    return;
  }
  events_ = std::move(events);
  for (const ModelDataChangeRange& range : plan.changedRanges) {
    emit dataChanged(index(range.firstRow, 0), index(range.lastRow, 0));
  }
}

} // namespace hcb
