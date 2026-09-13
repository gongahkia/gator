#include <QtTest/QTest>

#include "core/TaskModel.h"

class TaskModelTest final : public QObject {
  Q_OBJECT

private slots:
  void exposesHierarchicalTaskRoles();
  void filtersInvalidTasksAndBreaksCycles();
};

void TaskModelTest::exposesHierarchicalTaskRoles() {
  hcb::TaskModel model;
  model.setTasks({{.id = QStringLiteral("child"),
                   .taskListId = QStringLiteral("list-a"),
                   .parentTaskId = QStringLiteral("root"),
                   .title = QStringLiteral("Child"),
                   .notes = QStringLiteral("Details"),
                   .due = hcb::TaskDue{.at = QStringLiteral("2026-07-25T10:00:00.000Z"),
                                       .timeZone = QStringLiteral("UTC")},
                   .priority = hcb::TaskPriority::High,
                   .sortOrder = 2},
                  {.id = QStringLiteral("root"),
                   .taskListId = QStringLiteral("list-a"),
                   .title = QStringLiteral("Root"),
                   .completed = true,
                   .managedRecurrence = true,
                   .recurrenceSummary = QStringLiteral("Every week"),
                   .recurrenceSeriesId = QStringLiteral("series"),
                   .recurrenceOccurrenceId = QStringLiteral("series:0"),
                   .recurrenceFrequency = 1,
                   .recurrenceInterval = 2,
                   .recurrenceEndKind = 1,
                   .recurrenceEndUntil = QStringLiteral("2026-08-01"),
                   .sortOrder = 1}});

  QCOMPARE(model.rowCount(), 1);
  const QModelIndex root = model.index(0, 0);
  QCOMPARE(model.data(root, Qt::DisplayRole).toString(), QStringLiteral("Root"));
  QCOMPARE(model.data(root, hcb::TaskModel::CompletedRole).toBool(), true);
  QVERIFY(model.data(root, hcb::TaskModel::ManagedRecurrenceRole).toBool());
  QCOMPARE(model.data(root, hcb::TaskModel::RecurrenceSummaryRole).toString(),
           QStringLiteral("Every week"));
  QCOMPARE(model.data(root, hcb::TaskModel::RecurrenceFrequencyRole).toInt(), 1);
  QCOMPARE(model.data(root, hcb::TaskModel::RecurrenceIntervalRole).toInt(), 2);
  QCOMPARE(model.data(root, hcb::TaskModel::RecurrenceEndKindRole).toInt(), 1);
  QCOMPARE(model.data(root, hcb::TaskModel::RecurrenceEndUntilRole).toString(),
           QStringLiteral("2026-08-01"));
  QCOMPARE(model.rowCount(root), 1);
  const QModelIndex child = model.index(0, 0, root);
  QCOMPARE(model.parent(child), root);
  QCOMPARE(model.data(child, hcb::TaskModel::IdRole).toString(), QStringLiteral("child"));
  QCOMPARE(model.data(child, hcb::TaskModel::ParentTaskIdRole).toString(), QStringLiteral("root"));
  QCOMPARE(model.data(child, hcb::TaskModel::NotesRole).toString(), QStringLiteral("Details"));
  QCOMPARE(model.data(child, hcb::TaskModel::DueAtRole).toString(),
           QStringLiteral("2026-07-25T10:00:00.000Z"));
  QCOMPARE(model.data(child, hcb::TaskModel::DueTimeZoneRole).toString(), QStringLiteral("UTC"));
  QCOMPARE(model.data(child, hcb::TaskModel::PriorityRole).toInt(),
           static_cast<int>(hcb::TaskPriority::High));
  QCOMPARE(model.roleNames().value(hcb::TaskModel::SortOrderRole), QByteArrayLiteral("sortOrder"));
  QCOMPARE(model.roleNames().value(hcb::TaskModel::RecurrenceEndCountRole),
           QByteArrayLiteral("recurrenceEndCount"));
  QCOMPARE(model.flatRowForTaskId(QStringLiteral("root")), 0);
  QCOMPARE(model.flatRowForTaskId(QStringLiteral("child")), 1);
  QCOMPARE(model.taskForId(QStringLiteral("child")).value(QStringLiteral("title")).toString(),
           QStringLiteral("Child"));
}

void TaskModelTest::filtersInvalidTasksAndBreaksCycles() {
  hcb::TaskModel model;
  model.setTasks({{.id = QStringLiteral("a"),
                   .taskListId = QStringLiteral("list-a"),
                   .parentTaskId = QStringLiteral("b"),
                   .title = QStringLiteral("A"),
                   .sortOrder = 1},
                  {.id = QStringLiteral("b"),
                   .taskListId = QStringLiteral("list-a"),
                   .parentTaskId = QStringLiteral("a"),
                   .title = QStringLiteral("B"),
                   .sortOrder = 2},
                  {.id = QString(),
                   .taskListId = QStringLiteral("list-a"),
                   .title = QStringLiteral("Invalid")}});

  QCOMPARE(model.rowCount(), 1);
  const QModelIndex root = model.index(0, 0);
  QCOMPARE(model.data(root, hcb::TaskModel::IdRole).toString(), QStringLiteral("a"));
  QCOMPARE(model.rowCount(root), 1);
  QCOMPARE(model.data(model.index(0, 0, root), hcb::TaskModel::IdRole).toString(),
           QStringLiteral("b"));
  QVERIFY(!model.data(QModelIndex(), Qt::DisplayRole).isValid());
}

QTEST_GUILESS_MAIN(TaskModelTest)

#include "TaskModelTest.moc"
