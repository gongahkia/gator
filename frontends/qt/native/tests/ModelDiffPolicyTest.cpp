#include <QtTest/QTest>

#include "core/ModelDiffPolicy.h"

namespace {

struct Row final {
  QString id;
  int value{0};
};

} // namespace

class ModelDiffPolicyTest final : public QObject {
  Q_OBJECT

private slots:
  void preservesStableRowsAndCoalescesChangedRanges();
  void resetsForCountOrIdentityChanges();
};

void ModelDiffPolicyTest::preservesStableRowsAndCoalescesChangedRanges() {
  const QList<Row> current = {{.id = QStringLiteral("a"), .value = 1},
                              {.id = QStringLiteral("b"), .value = 2},
                              {.id = QStringLiteral("c"), .value = 3},
                              {.id = QStringLiteral("d"), .value = 4},
                              {.id = QStringLiteral("e"), .value = 5}};
  const QList<Row> next = {{.id = QStringLiteral("a"), .value = 1},
                           {.id = QStringLiteral("b"), .value = 20},
                           {.id = QStringLiteral("c"), .value = 30},
                           {.id = QStringLiteral("d"), .value = 4},
                           {.id = QStringLiteral("e"), .value = 50}};
  const hcb::ModelDiffPlan plan = hcb::ModelDiffPolicy::plan(
      current,
      next,
      [](const Row& row) -> const QString& { return row.id; },
      [](const Row& left, const Row& right) { return left.value == right.value; });

  QVERIFY(!plan.requiresReset);
  QCOMPARE(plan.changedRanges.size(), 2);
  QCOMPARE(plan.changedRanges.at(0).firstRow, 1);
  QCOMPARE(plan.changedRanges.at(0).lastRow, 2);
  QCOMPARE(plan.changedRanges.at(1).firstRow, 4);
  QCOMPARE(plan.changedRanges.at(1).lastRow, 4);
}

void ModelDiffPolicyTest::resetsForCountOrIdentityChanges() {
  const QList<Row> current = {{.id = QStringLiteral("a"), .value = 1},
                              {.id = QStringLiteral("b"), .value = 2}};
  const auto key = [](const Row& row) -> const QString& { return row.id; };
  const auto equivalent = [](const Row& left, const Row& right) {
    return left.value == right.value;
  };

  QVERIFY(hcb::ModelDiffPolicy::plan(current, {current.front()}, key, equivalent).requiresReset);
  QVERIFY(hcb::ModelDiffPolicy::plan(
              current,
              {{.id = QStringLiteral("b"), .value = 2}, {.id = QStringLiteral("a"), .value = 1}},
              key,
              equivalent)
              .requiresReset);
}

QTEST_GUILESS_MAIN(ModelDiffPolicyTest)

#include "ModelDiffPolicyTest.moc"
