#pragma once

#include "core/TaskModel.h"

#include <QAbstractListModel>

#include <cstdint>

namespace hcb {

struct NoteSummary final {
  QString id;
  QString taskListId;
  QString taskListTitle;
  QString title;
  QString body;
  bool completed{false};
};

class NotesModel final : public QAbstractListModel {
  Q_OBJECT

public:
  enum Role : std::int32_t {
    IdRole = Qt::UserRole + 1,
    TaskListIdRole,
    TaskListTitleRole,
    TitleRole,
    BodyRole,
    CompletedRole
  };
  Q_ENUM(Role)

  explicit NotesModel(QObject* parent = nullptr);

  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;

  void setTasks(const QList<TaskModelTask>& tasks);

private:
  QList<NoteSummary> notes_;
};

} // namespace hcb
