#include <QSignalSpy>
#include <QtTest/QTest>

#include "core/ScheduledTaskDateIndex.h"

class ScheduledTaskDateIndexTest final : public QObject {
  Q_OBJECT

private slots:
  void indexesScheduledTasksByLocalDueDate();
};

void ScheduledTaskDateIndexTest::indexesScheduledTasksByLocalDueDate() {
  hcb::ScheduledTaskDateIndex index;
  QSignalSpy changed(&index, &hcb::ScheduledTaskDateIndex::changed);
  const QVariantList tasks{
      QVariantMap{{QStringLiteral("id"), QStringLiteral("task-a")},
                  {QStringLiteral("dueAt"), QStringLiteral("2026-08-05")}},
      QVariantMap{{QStringLiteral("id"), QStringLiteral("task-b")},
                  {QStringLiteral("dueAt"), QStringLiteral("2026-08-05T10:30:00.000Z")}},
      QVariantMap{{QStringLiteral("id"), QStringLiteral("task-c")},
                  {QStringLiteral("dueAt"), QStringLiteral("2026-08-06")}},
      QVariantMap{{QStringLiteral("id"), QStringLiteral("task-invalid")},
                  {QStringLiteral("dueAt"), QStringLiteral("not-a-date")}}};

  index.setTasks(tasks);
  QCOMPARE(index.revision(), 1);
  QCOMPARE(changed.count(), 1);
  QCOMPARE(index.tasksForDate(QStringLiteral("2026-08-05")).size(), 2);
  QCOMPARE(index.tasksForDate(QStringLiteral("2026-08-06")).size(), 1);
  QVERIFY(index.tasksForDate(QStringLiteral("2026-08-07")).isEmpty());
  QCOMPARE(index.tasksForRange(QStringLiteral("2026-08-06"), QStringLiteral("2026-08-05")).size(),
           3);

  index.setTasks(tasks);
  QCOMPARE(index.revision(), 1);
  QCOMPARE(changed.count(), 1);
}

QTEST_GUILESS_MAIN(ScheduledTaskDateIndexTest)

#include "ScheduledTaskDateIndexTest.moc"
