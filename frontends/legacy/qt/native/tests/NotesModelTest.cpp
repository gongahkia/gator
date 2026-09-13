#include <QtTest/QTest>

#include "core/NotesModel.h"

class NotesModelTest final : public QObject {
  Q_OBJECT

private slots:
  void projectsUndatedTasksAndResets();
  void rejectsInvalidIndexes();
};

void NotesModelTest::projectsUndatedTasksAndResets() {
  hcb::NotesModel model;
  model.setTasks({{.id = QStringLiteral("note-a"),
                   .taskListId = QStringLiteral("list-a"),
                   .taskListTitle = QStringLiteral("Notes"),
                   .title = QStringLiteral("Release plan"),
                   .notes = QStringLiteral("Ship the native models.")},
                  {.id = QStringLiteral("scheduled"),
                   .taskListId = QStringLiteral("list-a"),
                   .taskListTitle = QStringLiteral("Notes"),
                   .title = QStringLiteral("Scheduled task"),
                   .due = hcb::TaskDue{.at = QStringLiteral("2026-07-26T00:00:00.000Z")}}});

  QCOMPARE(model.rowCount(), 1);
  const QModelIndex index = model.index(0, 0);
  QCOMPARE(model.data(index, Qt::DisplayRole).toString(), QStringLiteral("Release plan"));
  QCOMPARE(model.data(index, hcb::NotesModel::IdRole).toString(), QStringLiteral("note-a"));
  QCOMPARE(model.data(index, hcb::NotesModel::TaskListIdRole).toString(), QStringLiteral("list-a"));
  QCOMPARE(model.data(index, hcb::NotesModel::TaskListTitleRole).toString(),
           QStringLiteral("Notes"));
  QCOMPARE(model.data(index, hcb::NotesModel::BodyRole).toString(),
           QStringLiteral("Ship the native models."));
  QVERIFY(!model.data(index, hcb::NotesModel::CompletedRole).toBool());
  QCOMPARE(model.roleNames().value(hcb::NotesModel::TaskListTitleRole),
           QByteArrayLiteral("taskListTitle"));

  model.setTasks({});
  QCOMPARE(model.rowCount(), 0);
}

void NotesModelTest::rejectsInvalidIndexes() {
  hcb::NotesModel model;
  QVERIFY(!model.data(QModelIndex(), Qt::DisplayRole).isValid());
  QVERIFY(!model.data(model.index(0, 0), Qt::DisplayRole).isValid());
  QCOMPARE(model.rowCount(model.index(0, 0)), 0);
}

QTEST_GUILESS_MAIN(NotesModelTest)

#include "NotesModelTest.moc"
