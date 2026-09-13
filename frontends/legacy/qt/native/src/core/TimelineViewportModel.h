#pragma once

#include <QSortFilterProxyModel>
#include <QStringList>

namespace hcb {

class TimelineViewportModel final : public QSortFilterProxyModel {
  Q_OBJECT
  Q_PROPERTY(QAbstractItemModel* sourceModel READ sourceModel WRITE setSourceModel NOTIFY sourceModelChanged)
  Q_PROPERTY(int firstDayIndex READ firstDayIndex WRITE setFirstDayIndex NOTIFY firstDayIndexChanged)
  Q_PROPERTY(int dayCount READ dayCount WRITE setDayCount NOTIFY dayCountChanged)
  Q_PROPERTY(int visibleStartMinute READ visibleStartMinute WRITE setVisibleStartMinute NOTIFY visibleStartMinuteChanged)
  Q_PROPERTY(int visibleEndMinute READ visibleEndMinute WRITE setVisibleEndMinute NOTIFY visibleEndMinuteChanged)
  Q_PROPERTY(bool allDay READ allDay WRITE setAllDay NOTIFY allDayChanged)
  Q_PROPERTY(bool active READ active WRITE setActive NOTIFY activeChanged)
  Q_PROPERTY(bool filterCalendarVisibility READ filterCalendarVisibility WRITE setFilterCalendarVisibility NOTIFY filterCalendarVisibilityChanged)
  Q_PROPERTY(QStringList visibleCalendarIds READ visibleCalendarIds WRITE setVisibleCalendarIds NOTIFY visibleCalendarIdsChanged)

public:
  explicit TimelineViewportModel(QObject* parent = nullptr);

  void setSourceModel(QAbstractItemModel* sourceModel) override;
  [[nodiscard]] int firstDayIndex() const;
  void setFirstDayIndex(int firstDayIndex);
  [[nodiscard]] int dayCount() const;
  void setDayCount(int dayCount);
  [[nodiscard]] int visibleStartMinute() const;
  void setVisibleStartMinute(int visibleStartMinute);
  [[nodiscard]] int visibleEndMinute() const;
  void setVisibleEndMinute(int visibleEndMinute);
  [[nodiscard]] bool allDay() const;
  void setAllDay(bool allDay);
  [[nodiscard]] bool active() const;
  void setActive(bool active);
  [[nodiscard]] bool filterCalendarVisibility() const;
  void setFilterCalendarVisibility(bool filterCalendarVisibility);
  [[nodiscard]] QStringList visibleCalendarIds() const;
  void setVisibleCalendarIds(const QStringList& visibleCalendarIds);

signals:
  void sourceModelChanged();
  void firstDayIndexChanged();
  void dayCountChanged();
  void visibleStartMinuteChanged();
  void visibleEndMinuteChanged();
  void allDayChanged();
  void activeChanged();
  void filterCalendarVisibilityChanged();
  void visibleCalendarIdsChanged();

protected:
  [[nodiscard]] bool filterAcceptsRow(int sourceRow,
                                      const QModelIndex& sourceParent) const override;

private:
  int firstDayIndex_{0};
  int dayCount_{7};
  int visibleStartMinute_{0};
  int visibleEndMinute_{24 * 60};
  bool allDay_{false};
  bool active_{true};
  bool filterCalendarVisibility_{false};
  QStringList visibleCalendarIds_;
  QHash<QByteArray, int> sourceRoles_;

  [[nodiscard]] int roleForName(const QByteArray& name) const;
  void updateSourceRoles(QAbstractItemModel* sourceModel);
};

} // namespace hcb
