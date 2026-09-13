#include <QtTest/QTest>

#include "core/UnifiedLocalSearchRanker.h"

class UnifiedLocalSearchRankerTest final : public QObject {
  Q_OBJECT

private slots:
  void prioritizesTitlePhraseAndPrefixMatches();
  void requiresEveryQueryTokenAndCapsResults();
  void ordersTiesDeterministically();
};

void UnifiedLocalSearchRankerTest::prioritizesTitlePhraseAndPrefixMatches() {
  hcb::UnifiedLocalSearchRanker ranker;
  const QList<hcb::LocalSearchRankedResult> results =
      ranker.rank(QStringLiteral("review"),
                  {{.resource = hcb::LocalSearchResource::Event,
                    .id = QStringLiteral("event-body"),
                    .title = QStringLiteral("Planning"),
                    .detail = QStringLiteral("Review agenda")},
                   {.resource = hcb::LocalSearchResource::Task,
                    .id = QStringLiteral("task-prefix"),
                    .title = QStringLiteral("Review notes"),
                    .detail = QString()},
                   {.resource = hcb::LocalSearchResource::Note,
                    .id = QStringLiteral("note-exact"),
                    .title = QStringLiteral("Review"),
                    .detail = QString()}});
  QCOMPARE(results.size(), 3);
  QCOMPARE(results.at(0).id, QStringLiteral("note-exact"));
  QCOMPARE(results.at(1).id, QStringLiteral("task-prefix"));
  QCOMPARE(results.at(2).id, QStringLiteral("event-body"));
}

void UnifiedLocalSearchRankerTest::requiresEveryQueryTokenAndCapsResults() {
  hcb::UnifiedLocalSearchRanker ranker;
  const QList<hcb::LocalSearchRankedResult> results =
      ranker.rank(QStringLiteral("release review"),
                  {{.id = QStringLiteral("both"),
                    .title = QStringLiteral("Release review"),
                    .detail = QString()},
                   {.id = QStringLiteral("release-only"),
                    .title = QStringLiteral("Release"),
                    .detail = QString()},
                   {.id = QStringLiteral("review-only"),
                    .title = QStringLiteral("Review"),
                    .detail = QString()}},
                  1);
  QCOMPARE(results.size(), 1);
  QCOMPARE(results.front().id, QStringLiteral("both"));
}

void UnifiedLocalSearchRankerTest::ordersTiesDeterministically() {
  hcb::UnifiedLocalSearchRanker ranker;
  const QList<hcb::LocalSearchRankedResult> results =
      ranker.rank(QString(),
                  {{.resource = hcb::LocalSearchResource::Note,
                    .id = QStringLiteral("b"),
                    .title = QStringLiteral("Alpha"),
                    .detail = QString()},
                   {.resource = hcb::LocalSearchResource::Task,
                    .id = QStringLiteral("c"),
                    .title = QStringLiteral("Zeta"),
                    .detail = QString()},
                   {.resource = hcb::LocalSearchResource::Task,
                    .id = QStringLiteral("a"),
                    .title = QStringLiteral("Zeta"),
                    .detail = QString()}});
  QCOMPARE(results.size(), 3);
  QCOMPARE(results.at(0).id, QStringLiteral("a"));
  QCOMPARE(results.at(1).id, QStringLiteral("c"));
  QCOMPARE(results.at(2).id, QStringLiteral("b"));
}

QTEST_GUILESS_MAIN(UnifiedLocalSearchRankerTest)

#include "UnifiedLocalSearchRankerTest.moc"
