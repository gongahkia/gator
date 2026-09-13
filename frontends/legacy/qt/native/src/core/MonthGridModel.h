#pragma once

#include "core/CalendarReadService.h"

#include <QAbstractTableModel>
#include <QDate>
#include <QStringList>
#include <QTimeZone>
#include <QVariantList>

#include <cstdint>

namespace hcb {

class MonthGridModel final : public QAbstractTableModel {
  Q_OBJECT
  Q_PROPERTY(QVariantList allDaySpans READ allDaySpans NOTIFY allDaySpansChanged)
  Q_PROPERTY(QVariantList visibleAllDaySpans READ visibleAllDaySpans NOTIFY presentationChanged)
  Q_PROPERTY(QStringList visibleCalendarIds READ visibleCalendarIds WRITE setVisibleCalendarIds NOTIFY
                 presentationChanged)
  Q_PROPERTY(int visibleAllDayLanes READ visibleAllDayLanes WRITE setVisibleAllDayLanes NOTIFY
                 presentationChanged)

public:
  enum Role : std::int32_t {
    DateRole = Qt::UserRole + 1,
    DayRole,
    OutsideMonthRole,
    EventCountRole,
    EventsRole,
    AllDayOverflowCountRole,
    VisibleTimedEventsRole,
    VisibleAllDayEventsRole,
    HiddenAllDayCountRole
  };
  Q_ENUM(Role)

  explicit MonthGridModel(QObject* parent = nullptr);

  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] int columnCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;
  [[nodiscard]] QVariantList allDaySpans() const;
  [[nodiscard]] QVariantList visibleAllDaySpans() const;
  [[nodiscard]] QStringList visibleCalendarIds() const;
  void setVisibleCalendarIds(QStringList visibleCalendarIds);
  [[nodiscard]] int visibleAllDayLanes() const;
  void setVisibleAllDayLanes(int visibleAllDayLanes);
  Q_INVOKABLE QString dateForPoint(double x, double y, double width, double height) const;
  Q_INVOKABLE int dateIndex(const QString& date) const;
  Q_INVOKABLE QString dateForIndex(int dayIndex) const;
  Q_INVOKABLE QVariantMap allDayRangeInput(int firstDayIndex, int lastDayIndex) const;
  Q_INVOKABLE QVariantMap moveInput(const QVariantMap& event, int targetDayIndex) const;
  Q_INVOKABLE QVariantMap resizeAllDayRangeInput(const QVariantMap& event,
                                                 int firstDayIndex,
                                                 int lastDayIndex) const;

  struct Cell final {
    QDate date;
    QList<CalendarEventSummary> events;
  };

  struct AllDaySpan final {
    CalendarEventSummary event;
    int weekIndex{0};
    int startColumn{0};
    int daySpan{1};
    int laneIndex{0};
    bool startsBeforeRange{false};
    bool endsAfterRange{false};
  };

  struct Layout final {
    QDate month;
    QList<Cell> cells;
    QList<AllDaySpan> allDaySpans;
    QList<int> allDayOverflowCounts;
    QTimeZone displayTimeZone;
  };

  [[nodiscard]] static Layout buildLayout(QDate month,
                                          const QList<CalendarEventSummary>& events,
                                          const QTimeZone& displayTimeZone,
                                          int weekStartDay = 0);
  void applyLayout(Layout layout);

  void setMonth(QDate month,
                const QList<CalendarEventSummary>& events,
                const QTimeZone& displayTimeZone,
                int weekStartDay = 0);

signals:
  void allDaySpansChanged();
  void presentationChanged();

private:
  [[nodiscard]] bool isCalendarVisible(const QString& calendarId) const;
  void rebuildPresentation(bool notifyViews);

  QDate month_;
  QList<Cell> cells_;
  QList<AllDaySpan> allDaySpans_;
  QList<int> allDayOverflowCounts_;
  QList<QVariantList> visibleTimedEvents_;
  QList<QVariantList> visibleAllDayEvents_;
  QList<int> hiddenAllDayCounts_;
  QVariantList visibleAllDaySpanMaps_;
  QStringList visibleCalendarIds_;
  int visibleAllDayLanes_{3};
  bool filterCalendarVisibility_{false};
  QTimeZone displayTimeZone_;
};

} // namespace hcb
