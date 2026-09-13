#pragma once

#include "core/CalendarReadService.h"

#include <QAbstractListModel>
#include <QVariantMap>

#include <cstdint>

namespace hcb {

class AgendaModel final : public QAbstractListModel {
  Q_OBJECT

public:
  enum Role : std::int32_t {
    IdRole = Qt::UserRole + 1,
    CalendarIdRole,
    RecurringRemoteIdRole,
    OriginalStartAtRole,
    RecurrenceRuleRole,
    StatusRole,
    TitleRole,
    DescriptionRole,
    LocationRole,
    StartAtRole,
    StartTimeZoneRole,
    EndAtRole,
    EndTimeZoneRole,
    AllDayRole,
    ColorIdRole,
    TransparencyRole,
    VisibilityRole,
    HcbKindRole,
    AttendeeEmailsJsonRole,
    AttendeeDetailsJsonRole,
    RemindersJsonRole,
    RemindersUseDefaultRole,
    ConferenceJsonRole,
    AttachmentsJsonRole,
    GuestPermissionsJsonRole,
    StatusPropertiesJsonRole,
    EventTypeRole,
    AgendaDayRole,
    AgendaWeekRole
  };
  Q_ENUM(Role)

  explicit AgendaModel(QObject* parent = nullptr);

  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;

  Q_INVOKABLE int rowForEvent(QString eventId) const;
  Q_INVOKABLE QVariantMap eventForId(QString eventId) const;
  void setEvents(QList<CalendarEventSummary> events);

private:
  QList<CalendarEventSummary> events_;
};

} // namespace hcb
