#pragma once

#include "core/TaskListReadService.h"

#include <QAbstractListModel>
#include <QVariantList>

#include <cstdint>

namespace hcb {

class TaskListModel final : public QAbstractListModel {
  Q_OBJECT

public:
  enum Role : std::int32_t {
    IdRole = Qt::UserRole + 1,
    AccountIdRole,
    TitleRole,
    SortOrderRole,
    SelectedRole,
    TaskCountRole,
    ActiveTaskCountRole,
    TaskTitlesRole
  };
  Q_ENUM(Role)

  explicit TaskListModel(QObject* parent = nullptr);

  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;
  [[nodiscard]] int revision() const;

  Q_PROPERTY(int revision READ revision NOTIFY revisionChanged)

  Q_INVOKABLE QVariantList selectedTaskLists() const;

  void setTaskLists(QList<TaskListSummary> taskLists);

signals:
  void revisionChanged();

private:
  QList<TaskListSummary> taskLists_;
  int revision_{0};
};

} // namespace hcb
