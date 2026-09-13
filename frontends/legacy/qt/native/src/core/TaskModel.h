#pragma once

#include "core/TaskMutationService.h"

#include <QAbstractItemModel>
#include <QVariantList>
#include <QVariantMap>

#include <QList>
#include <QString>

#include <cstdint>
#include <memory>
#include <optional>
#include <vector>

namespace hcb {

struct TaskModelTask final {
  QString id;
  QString taskListId;
  QString taskListTitle;
  std::optional<QString> parentTaskId;
  QString title;
  std::optional<QString> notes;
  std::optional<TaskDue> due;
  TaskPriority priority{TaskPriority::None};
  bool completed{false};
  bool managedRecurrence{false};
  QString recurrenceSummary;
  QString recurrenceSeriesId;
  QString recurrenceOccurrenceId;
  int recurrenceFrequency{-1};
  int recurrenceInterval{1};
  int recurrenceEndKind{0};
  QString recurrenceEndUntil;
  int recurrenceEndCount{0};
  QString recurrenceRule;
  QString recurrenceExclusionDates;
  QString recurrenceAdditionDates;
  QString recurrenceDiagnostic;
  std::int64_t sortOrder{0};
};

class TaskModel final : public QAbstractItemModel {
  Q_OBJECT

public:
  enum Role : std::int32_t {
    IdRole = Qt::UserRole + 1,
    TaskListIdRole,
    TaskListTitleRole,
    ParentTaskIdRole,
    TitleRole,
    NotesRole,
    DueAtRole,
    DueTimeZoneRole,
    PriorityRole,
    CompletedRole,
    ManagedRecurrenceRole,
    RecurrenceSummaryRole,
    RecurrenceSeriesIdRole,
    RecurrenceOccurrenceIdRole,
    RecurrenceFrequencyRole,
    RecurrenceIntervalRole,
    RecurrenceEndKindRole,
    RecurrenceEndUntilRole,
    RecurrenceEndCountRole,
    RecurrenceRuleRole,
    RecurrenceExclusionDatesRole,
    RecurrenceAdditionDatesRole,
    RecurrenceDiagnosticRole,
    SortOrderRole
  };
  Q_ENUM(Role)

  explicit TaskModel(QObject* parent = nullptr);
  ~TaskModel() override;

  [[nodiscard]] QModelIndex
  index(int row, int column, const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QModelIndex parent(const QModelIndex& index) const override;
  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] int columnCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;

  Q_INVOKABLE QVariantList taskIds() const;
  Q_INVOKABLE QVariantList topLevelTasks() const;
  Q_INVOKABLE QVariantMap taskForId(QString taskId) const;
  Q_INVOKABLE int flatRowForTaskId(QString taskId) const;
  void setTasks(QList<TaskModelTask> tasks);

private:
  struct Node;

  QList<Node*> roots_;
  std::vector<std::unique_ptr<Node>> nodes_;
};

} // namespace hcb
