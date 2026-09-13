#include <QtTest/QTest>

#include "core/SyncThreeWayMerge.h"

class SyncThreeWayMergeTest final : public QObject {
  Q_OBJECT

private slots:
  void preservesDisjointTaskEdits();
  void appliesConfiguredSameFieldPolicy();
  void requiresExplicitResolutionForSameFieldAskPolicy();
  void classifiesDeleteMoveAndRecurrenceAsStructural();
};

namespace {

[[nodiscard]] hcb::SyncThreeWayMergeInput
taskInput(hcb::SyncConflictPolicy policy, QJsonObject local, QJsonObject remote) {
  return {.resource = hcb::SyncConflictResource::Task,
          .operation = QStringLiteral("task.update"),
          .baseSnapshot = {{QStringLiteral("title"), QStringLiteral("Base")},
                           {QStringLiteral("notes"), QStringLiteral("Base notes")},
                           {QStringLiteral("status"), QStringLiteral("needsAction")}},
          .localIntent = {{QStringLiteral("task"), std::move(local)}},
          .remoteSnapshot = std::move(remote),
          .policy = policy};
}

} // namespace

void SyncThreeWayMergeTest::preservesDisjointTaskEdits() {
  const hcb::SyncThreeWayMergeResult result = hcb::SyncThreeWayMerge::merge(
      taskInput(hcb::SyncConflictPolicy::AskEachTime,
                {{QStringLiteral("title"), QStringLiteral("Local")}},
                {{QStringLiteral("title"), QStringLiteral("Base")},
                 {QStringLiteral("notes"), QStringLiteral("Remote notes")},
                 {QStringLiteral("status"), QStringLiteral("needsAction")}}));
  QCOMPARE(result.decision, hcb::SyncMergeDecision::ReapplyLocal);
  QVERIFY(result.conflicts.isEmpty());
  QCOMPARE(result.reapplyIntent.value(QStringLiteral("task"))
               .toObject()
               .value(QStringLiteral("title"))
               .toString(),
           QStringLiteral("Local"));
}

void SyncThreeWayMergeTest::appliesConfiguredSameFieldPolicy() {
  const QJsonObject local{{QStringLiteral("title"), QStringLiteral("Local")}};
  const QJsonObject remote{{QStringLiteral("title"), QStringLiteral("Remote")},
                           {QStringLiteral("notes"), QStringLiteral("Base notes")},
                           {QStringLiteral("status"), QStringLiteral("needsAction")}};
  const hcb::SyncThreeWayMergeResult google = hcb::SyncThreeWayMerge::merge(
      taskInput(hcb::SyncConflictPolicy::PreferGoogle, local, remote));
  QCOMPARE(google.decision, hcb::SyncMergeDecision::KeepRemote);
  QCOMPARE(google.conflicts.size(), 1);
  QCOMPARE(google.conflicts.constFirst().field, QStringLiteral("title"));
  const hcb::SyncThreeWayMergeResult hcb =
      hcb::SyncThreeWayMerge::merge(taskInput(hcb::SyncConflictPolicy::PreferHcb, local, remote));
  QCOMPARE(hcb.decision, hcb::SyncMergeDecision::ReapplyLocal);
  QCOMPARE(hcb.conflicts.size(), 1);
  QCOMPARE(hcb.reapplyIntent.value(QStringLiteral("task"))
               .toObject()
               .value(QStringLiteral("title"))
               .toString(),
           QStringLiteral("Local"));
}

void SyncThreeWayMergeTest::requiresExplicitResolutionForSameFieldAskPolicy() {
  const hcb::SyncThreeWayMergeResult result = hcb::SyncThreeWayMerge::merge(
      taskInput(hcb::SyncConflictPolicy::AskEachTime,
                {{QStringLiteral("title"), QStringLiteral("Local")}},
                {{QStringLiteral("title"), QStringLiteral("Remote")},
                 {QStringLiteral("notes"), QStringLiteral("Base notes")},
                 {QStringLiteral("status"), QStringLiteral("needsAction")}}));
  QCOMPARE(result.decision, hcb::SyncMergeDecision::RequireUser);
  QCOMPARE(result.conflicts.size(), 1);
  QVERIFY(result.reapplyIntent.isEmpty());
}

void SyncThreeWayMergeTest::classifiesDeleteMoveAndRecurrenceAsStructural() {
  const QJsonObject remote{{QStringLiteral("title"), QStringLiteral("Remote")}};
  for (const hcb::SyncThreeWayMergeInput& input :
       {hcb::SyncThreeWayMergeInput{.resource = hcb::SyncConflictResource::Task,
                                    .operation = QStringLiteral("task.delete"),
                                    .localIntent = QJsonObject{},
                                    .remoteSnapshot = remote,
                                    .policy = hcb::SyncConflictPolicy::AskEachTime},
        hcb::SyncThreeWayMergeInput{
            .resource = hcb::SyncConflictResource::Task,
            .operation = QStringLiteral("task.update"),
            .localIntent = {{QStringLiteral("parentTaskId"), QStringLiteral("parent-2")}},
            .remoteSnapshot = remote,
            .policy = hcb::SyncConflictPolicy::PreferGoogle},
        hcb::SyncThreeWayMergeInput{
            .resource = hcb::SyncConflictResource::Task,
            .operation = QStringLiteral("task.update"),
            .localIntent = {{QStringLiteral("task"),
                             QJsonObject{{QStringLiteral("title"), QStringLiteral("Local")}}}},
            .remoteSnapshot = {{QStringLiteral("_deleted"), true}},
            .policy = hcb::SyncConflictPolicy::AskEachTime},
        hcb::SyncThreeWayMergeInput{
            .resource = hcb::SyncConflictResource::Event,
            .operation = QStringLiteral("event.update"),
            .localIntent = {{QStringLiteral("event"),
                             QJsonObject{{QStringLiteral("recurrence"),
                                          QStringLiteral("RRULE:FREQ=WEEKLY")}}}},
            .remoteSnapshot = remote,
            .policy = hcb::SyncConflictPolicy::PreferHcb}}) {
    const hcb::SyncThreeWayMergeResult result = hcb::SyncThreeWayMerge::merge(input);
    QVERIFY(result.structural);
  }
}

QTEST_GUILESS_MAIN(SyncThreeWayMergeTest)

#include "SyncThreeWayMergeTest.moc"
