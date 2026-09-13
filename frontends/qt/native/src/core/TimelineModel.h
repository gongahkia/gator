#pragma once

#include "core/CalendarReadService.h"

#include <QAbstractListModel>
#include <QDate>
#include <QObject>
#include <QTimeZone>
#include <QVariantMap>

#include <cstdint>

namespace hcb {

class TimelineModel final : public QAbstractListModel {
  Q_OBJECT
  Q_PROPERTY(int totalItemCount READ totalItemCount NOTIFY totalItemCountChanged)

public:
  enum Role : std::int32_t {
    IdRole = Qt::UserRole + 1,
    CalendarIdRole,
    RecurringRemoteIdRole,
    OriginalStartAtRole,
    RecurrenceRuleRole,
    TitleRole,
    StatusRole,
    ColorIdRole,
    DescriptionRole,
    LocationRole,
    StartAtRole,
    StartTimeZoneRole,
    EndAtRole,
    EndTimeZoneRole,
    TransparencyRole,
    VisibilityRole,
    AttendeeEmailsJsonRole,
    AttendeeDetailsJsonRole,
    RemindersJsonRole,
    RemindersUseDefaultRole,
    ConferenceJsonRole,
    AttachmentsJsonRole,
    GuestPermissionsJsonRole,
    StatusPropertiesJsonRole,
    EventTypeRole,
    AllDayRole,
    DayIndexRole,
    StartMinuteRole,
    DurationMinutesRole,
    LaneIndexRole,
    LaneCountRole,
    DaySpanRole,
    StartsBeforeRangeRole,
    EndsAfterRangeRole
  };
  Q_ENUM(Role)

  explicit TimelineModel(QObject* parent = nullptr);

  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;
  [[nodiscard]] int totalItemCount() const;

  Q_INVOKABLE QObject* createViewport();

  Q_INVOKABLE QVariantMap moveInput(const QString& eventId,
                                    int targetDayIndex,
                                    int targetMinute) const;
  Q_INVOKABLE QVariantMap moveAllDayInput(const QString& eventId, int targetDayIndex) const;
  Q_INVOKABLE QVariantMap resizeInput(const QString& eventId,
                                      int targetEndDayIndex,
                                      int targetEndMinute) const;
  Q_INVOKABLE QVariantMap resizeAllDayInput(const QString& eventId, int targetEndDayIndex) const;
  Q_INVOKABLE QVariantMap resizeAllDayRangeInput(const QString& eventId,
                                                 int targetStartDayIndex,
                                                 int targetEndDayIndex) const;
  Q_INVOKABLE QVariantMap timedRangeInput(int firstDayIndex,
                                          int firstMinute,
                                          int lastDayIndex,
                                          int lastMinute) const;
  Q_INVOKABLE QVariantMap allDayRangeInput(int firstDayIndex, int lastDayIndex) const;
  Q_INVOKABLE QVariantMap timelinePointInput(double x,
                                             double y,
                                             double availableWidth,
                                             double timeColumnWidth,
                                             double hourHeight,
                                             bool endPoint = false) const;
  Q_INVOKABLE int dayIndexForDate(const QString& date) const;
  Q_INVOKABLE QString dateForDayIndex(int dayIndex) const;

  struct Item final {
    CalendarEventSummary event;
    bool allDay{false};
    int dayIndex{0};
    int startMinute{0};
    int durationMinutes{0};
    int laneIndex{0};
    int laneCount{1};
    int daySpan{1};
    bool startsBeforeRange{false};
    bool endsAfterRange{false};
  };

  struct Layout final {
    QList<Item> items;
    QDate rangeStartDate;
    int dayCount{0};
    QTimeZone displayTimeZone;
  };

  [[nodiscard]] static Layout buildLayout(QDate startDate,
                                          int dayCount,
                                          const QList<CalendarEventSummary>& events,
                                          const QTimeZone& displayTimeZone,
                                          int visibleAllDayLaneCount);
  void applyLayout(Layout layout);

  void setRange(QDate startDate,
                int dayCount,
                const QList<CalendarEventSummary>& events,
                const QTimeZone& displayTimeZone,
                int visibleAllDayLaneCount);

signals:
  void totalItemCountChanged();

private:
  QList<Item> items_;
  QDate rangeStartDate_;
  int dayCount_{0};
  QTimeZone displayTimeZone_;
};

} // namespace hcb
