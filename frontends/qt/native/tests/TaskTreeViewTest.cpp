#include <QGuiApplication>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickView>
#include <QtTest/QTest>

#include "core/TaskModel.h"

class TaskTreeViewTest final : public QObject {
  Q_OBJECT

private slots:
  void expandsSubtasksFromTaskModel();
};

void TaskTreeViewTest::expandsSubtasksFromTaskModel() {
  hcb::TaskModel model;
  model.setTasks({{.id = QStringLiteral("child"),
                   .taskListId = QStringLiteral("list-a"),
                   .parentTaskId = QStringLiteral("parent"),
                   .title = QStringLiteral("Child"),
                   .sortOrder = 0},
                  {.id = QStringLiteral("parent"),
                   .taskListId = QStringLiteral("list-a"),
                   .title = QStringLiteral("Parent"),
                   .sortOrder = 0}});

  QQuickView view;
  view.resize(960, 640);
  view.setSource(QUrl::fromLocalFile(QStringLiteral(HCB_NATIVE_QML_DIR "/TaskListView.qml")));
  QVERIFY(view.status() == QQuickView::Ready);

  QObject* const root = view.rootObject();
  QVERIFY(root != nullptr);
  root->setProperty("taskModel", QVariant::fromValue(static_cast<QObject*>(&model)));
  view.show();
  QObject* const taskRows = root->property("taskRows").value<QObject*>();
  QVERIFY(taskRows != nullptr);
  QTRY_COMPARE(taskRows->property("rows").toInt(), 1);
  QVERIFY(QMetaObject::invokeMethod(taskRows, "expand", Q_ARG(int, 0)));
  QTRY_COMPARE(taskRows->property("rows").toInt(), 2);
}

int main(int argc, char* argv[]) {
  QGuiApplication application(argc, argv);
  TaskTreeViewTest test;
  return QTest::qExec(&test, argc, argv);
}

#include "TaskTreeViewTest.moc"
