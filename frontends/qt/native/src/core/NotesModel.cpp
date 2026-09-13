#include "core/NotesModel.h"
#include "core/ModelDiffPolicy.h"

#include <algorithm>
#include <utility>

namespace hcb {

namespace {

[[nodiscard]] bool equivalentNote(const NoteSummary& left, const NoteSummary& right) {
  return left.id == right.id && left.taskListId == right.taskListId &&
         left.taskListTitle == right.taskListTitle && left.title == right.title &&
         left.body == right.body && left.completed == right.completed;
}

[[nodiscard]] bool isUndated(const TaskModelTask& task) {
  return !task.due.has_value() || !task.due->at.has_value();
}

[[nodiscard]] QList<NoteSummary> noteProjection(const QList<TaskModelTask>& tasks) {
  QList<NoteSummary> notes;
  notes.reserve(tasks.size());
  for (const TaskModelTask& task : tasks) {
    if (!isUndated(task)) {
      continue;
    }
    notes.append({.id = task.id,
                  .taskListId = task.taskListId,
                  .taskListTitle = task.taskListTitle,
                  .title = task.title,
                  .body = task.notes.value_or(QString()),
                  .completed = task.completed});
  }
  std::sort(notes.begin(), notes.end(), [](const NoteSummary& left, const NoteSummary& right) {
    const int list = QString::compare(left.taskListTitle, right.taskListTitle, Qt::CaseInsensitive);
    if (list != 0) {
      return list < 0;
    }
    const int title = QString::compare(left.title, right.title, Qt::CaseInsensitive);
    return title != 0 ? title < 0 : left.id < right.id;
  });
  return notes;
}

} // namespace

NotesModel::NotesModel(QObject* parent) : QAbstractListModel(parent) {}

int NotesModel::rowCount(const QModelIndex& parent) const {
  return parent.isValid() ? 0 : static_cast<int>(notes_.size());
}

QVariant NotesModel::data(const QModelIndex& index, int role) const {
  if (!index.isValid() || index.row() < 0 || index.row() >= static_cast<int>(notes_.size())) {
    return {};
  }
  const NoteSummary& note = notes_.at(index.row());
  switch (role) {
  case Qt::DisplayRole:
  case TitleRole:
    return note.title;
  case IdRole:
    return note.id;
  case TaskListIdRole:
    return note.taskListId;
  case TaskListTitleRole:
    return note.taskListTitle;
  case BodyRole:
    return note.body;
  case CompletedRole:
    return note.completed;
  default:
    return {};
  }
}

QHash<int, QByteArray> NotesModel::roleNames() const {
  return {{IdRole, "id"},
          {TaskListIdRole, "taskListId"},
          {TaskListTitleRole, "taskListTitle"},
          {TitleRole, "title"},
          {BodyRole, "body"},
          {CompletedRole, "completed"}};
}

void NotesModel::setTasks(const QList<TaskModelTask>& tasks) {
  QList<NoteSummary> notes = noteProjection(tasks);
  const ModelDiffPlan plan = ModelDiffPolicy::plan(
      notes_,
      notes,
      [](const NoteSummary& note) -> const QString& { return note.id; },
      equivalentNote);
  if (plan.requiresReset) {
    beginResetModel();
    notes_ = std::move(notes);
    endResetModel();
    return;
  }
  notes_ = std::move(notes);
  for (const ModelDataChangeRange& range : plan.changedRanges) {
    emit dataChanged(index(range.firstRow, 0), index(range.lastRow, 0));
  }
}

} // namespace hcb
