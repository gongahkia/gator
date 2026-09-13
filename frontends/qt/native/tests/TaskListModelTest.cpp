#include <QtTest/QTest>

#include "core/TaskListModel.h"

class TaskListModelTest final : public QObject {
  Q_OBJECT

private slots:
  void exposesTaskListRolesAndResets();
  void returnsSelectedTaskLists();
  void rejectsInvalidIndexes();
};

void TaskListModelTest::exposesTaskListRolesAndResets() {
  hcb::TaskListModel model;
  model.setTaskLists({{.id = QStringLiteral("list-a"),
                       .accountId = QStringLiteral("account-a"),
                       .remoteId = QStringLiteral("remote-a"),
                       .title = QStringLiteral("Inbox"),
                       .sortOrder = 3,
                       .selected = true,
                       .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z"),
                       .taskCount = 4,
                       .activeTaskCount = 2}});
  QCOMPARE(model.rowCount(), 1);
  const QModelIndex index = model.index(0, 0);
  QCOMPARE(model.data(index, Qt::DisplayRole).toString(), QStringLiteral("Inbox"));
  QCOMPARE(model.data(index, hcb::TaskListModel::IdRole).toString(), QStringLiteral("list-a"));
  QCOMPARE(model.data(index, hcb::TaskListModel::AccountIdRole).toString(),
           QStringLiteral("account-a"));
  QCOMPARE(model.data(index, hcb::TaskListModel::SortOrderRole).toLongLong(), qint64{3});
  QCOMPARE(model.data(index, hcb::TaskListModel::SelectedRole).toBool(), true);
  QCOMPARE(model.data(index, hcb::TaskListModel::TaskCountRole).toLongLong(), qint64{4});
  QCOMPARE(model.data(index, hcb::TaskListModel::ActiveTaskCountRole).toLongLong(), qint64{2});
  QCOMPARE(model.roleNames().value(hcb::TaskListModel::ActiveTaskCountRole),
           QByteArrayLiteral("activeTaskCount"));

  model.setTaskLists({});
  QCOMPARE(model.rowCount(), 0);
}

void TaskListModelTest::returnsSelectedTaskLists() {
  hcb::TaskListModel model;
  model.setTaskLists({{.id = QStringLiteral("list-a"),
                       .accountId = QStringLiteral("account-a"),
                       .remoteId = QStringLiteral("remote-a"),
                       .title = QStringLiteral("Inbox"),
                       .selected = true,
                       .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")},
                      {.id = QStringLiteral("list-b"),
                       .accountId = QStringLiteral("account-a"),
                       .remoteId = QStringLiteral("remote-b"),
                       .title = QStringLiteral("Hidden"),
                       .selected = false,
                       .updatedAt = QStringLiteral("2026-07-25T00:00:00.000Z")}});
  const QVariantList selected = model.selectedTaskLists();
  QCOMPARE(selected.size(), 1);
  const QVariantMap row = selected.constFirst().toMap();
  QCOMPARE(row.value(QStringLiteral("id")).toString(), QStringLiteral("list-a"));
  QCOMPARE(row.value(QStringLiteral("title")).toString(), QStringLiteral("Inbox"));
  QCOMPARE(model.revision(), 1);
}

void TaskListModelTest::rejectsInvalidIndexes() {
  hcb::TaskListModel model;
  QVERIFY(!model.data(QModelIndex(), Qt::DisplayRole).isValid());
  QVERIFY(!model.data(model.index(0, 0), Qt::DisplayRole).isValid());
  QCOMPARE(model.rowCount(model.index(0, 0)), 0);
}

QTEST_GUILESS_MAIN(TaskListModelTest)

#include "TaskListModelTest.moc"
