#pragma once

#include "core/CalendarReadService.h"

#include <QAbstractListModel>
#include <QStringList>

#include <cstdint>

namespace hcb {

class CalendarSourceModel final : public QAbstractListModel {
  Q_OBJECT

public:
  enum Role : std::int32_t {
    IdRole = Qt::UserRole + 1,
    AccountIdRole,
    TitleRole,
    DescriptionRole,
    TimeZoneRole,
    ColorIdRole,
    BackgroundColorRole,
    ForegroundColorRole,
    AccessRoleRole,
    SelectedRole,
    HiddenRole,
    PrimaryRole,
    EventCountRole
  };
  Q_ENUM(Role)

  explicit CalendarSourceModel(QObject* parent = nullptr);

  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;
  [[nodiscard]] int revision() const;

  Q_PROPERTY(int revision READ revision NOTIFY revisionChanged)

  Q_INVOKABLE QStringList calendarIds() const;
  Q_INVOKABLE QStringList selectedCalendarIds() const;
  Q_INVOKABLE QString calendarTitle(const QString& calendarId) const;
  Q_INVOKABLE QString calendarBackgroundColor(const QString& calendarId) const;

  void setCalendars(QList<CalendarSummary> calendars);

signals:
  void revisionChanged();

private:
  QList<CalendarSummary> calendars_;
  int revision_{0};
};

} // namespace hcb
