#include <QtTest>

#include "core/NativePerformanceFixture.h"

#include <QJsonDocument>
#include <QJsonObject>

class NativePerformanceFixtureTest final : public QObject {
  Q_OBJECT

private slots:
  void generatesExpectedSmallFixture();
  void generatesExpectedMediumFixture();
  void generatesExpected15kEventFixture();
  void generatesExpectedWrapperScaleFixture();
  void serializesDeterministicallyWithoutCredentials();
};

void NativePerformanceFixtureTest::generatesExpectedSmallFixture() {
  const hcb::NativePerformanceFixture fixture = hcb::NativePerformanceFixtureGenerator::small();

  QCOMPARE(fixture.schemaVersion, std::uint32_t{1});
  QCOMPARE(fixture.size, QStringLiteral("small"));
  QCOMPARE(fixture.seed, QStringLiteral("hot-cross-buns-perf-small-v1"));
  QVERIFY(fixture.generatedDataOnly);
  QCOMPARE(fixture.baseTime, QStringLiteral("2026-01-05T09:00:00.000Z"));
  QCOMPARE(fixture.counts.tasks, std::size_t{50});
  QCOMPARE(fixture.counts.eventInstances, std::size_t{20});
  QCOMPARE(fixture.counts.notes, std::size_t{10});
  QCOMPARE(fixture.tasks.size(), fixture.counts.tasks);
  QCOMPARE(fixture.eventInstances.size(), fixture.counts.eventInstances);
  QCOMPARE(fixture.notes.size(), fixture.counts.notes);
  QCOMPARE(fixture.tasks.at(0).id, QStringLiteral("generated-small-task-00001"));
  QCOMPARE(fixture.tasks.at(0).status, QStringLiteral("completed"));
  QVERIFY(!fixture.tasks.at(0).dueAt.has_value());
  const std::optional<QString>& parentTaskId = fixture.tasks.at(17).parentTaskId;
  if (!parentTaskId.has_value()) {
    QFAIL("expected generated parent task ID");
    return;
  }
  QCOMPARE(parentTaskId.value(), QStringLiteral("generated-small-task-00017"));
  QCOMPARE(fixture.eventInstances.at(0).startsAt, QStringLiteral("2026-01-05T09:00:00.000Z"));
  QVERIFY(fixture.eventInstances.at(0).isAllDay);
  const std::optional<QString>& linkedResourceType = fixture.notes.at(0).linkedResourceType;
  if (!linkedResourceType.has_value()) {
    QFAIL("expected generated linked resource type");
    return;
  }
  QCOMPARE(linkedResourceType.value(), QStringLiteral("task"));
  const std::optional<QString>& linkedResourceId = fixture.notes.at(0).linkedResourceId;
  if (!linkedResourceId.has_value()) {
    QFAIL("expected generated linked resource ID");
    return;
  }
  QCOMPARE(linkedResourceId.value(), QStringLiteral("generated-small-task-00001"));
}

void NativePerformanceFixtureTest::generatesExpectedMediumFixture() {
  const hcb::NativePerformanceFixture fixture = hcb::NativePerformanceFixtureGenerator::medium();

  QCOMPARE(fixture.size, QStringLiteral("medium"));
  QCOMPARE(fixture.seed, QStringLiteral("hot-cross-buns-perf-medium-v1"));
  QCOMPARE(fixture.counts.tasks, std::size_t{1'000});
  QCOMPARE(fixture.counts.eventInstances, std::size_t{1'000});
  QCOMPARE(fixture.counts.notes, std::size_t{200});
  QCOMPARE(fixture.tasks.at(999).id, QStringLiteral("generated-medium-task-01000"));
  QCOMPARE(fixture.eventInstances.at(999).id,
           QStringLiteral("generated-medium-event-instance-01000"));
  QCOMPARE(fixture.notes.at(199).id, QStringLiteral("generated-medium-note-00200"));
}

void NativePerformanceFixtureTest::generatesExpected15kEventFixture() {
  const hcb::NativePerformanceFixture fixture = hcb::NativePerformanceFixtureGenerator::event15k();

  QCOMPARE(fixture.size, QStringLiteral("event15k"));
  QCOMPARE(fixture.seed, QStringLiteral("hot-cross-buns-perf-event15k-v1"));
  QCOMPARE(fixture.counts.tasks, std::size_t{1'000});
  QCOMPARE(fixture.counts.eventInstances, std::size_t{15'000});
  QCOMPARE(fixture.counts.notes, std::size_t{500});
  QCOMPARE(fixture.eventInstances.size(), std::size_t{15'000});
  QCOMPARE(fixture.eventInstances.at(14'999).id,
           QStringLiteral("generated-event15k-event-instance-15000"));
}

void NativePerformanceFixtureTest::generatesExpectedWrapperScaleFixture() {
  const hcb::NativePerformanceFixture fixture =
      hcb::NativePerformanceFixtureGenerator::wrapperScale();

  QCOMPARE(fixture.size, QStringLiteral("wrapper-scale"));
  QCOMPARE(fixture.seed, QStringLiteral("hot-cross-buns-perf-wrapper-scale-v1"));
  QCOMPARE(fixture.counts.tasks, std::size_t{10'000});
  QCOMPARE(fixture.counts.eventInstances, std::size_t{25'000});
  QCOMPARE(fixture.counts.notes, std::size_t{2'000});
  QCOMPARE(fixture.counts.recurrenceExceptions, std::size_t{500});
  QCOMPARE(fixture.counts.queuedMutations, std::size_t{500});
  QCOMPARE(fixture.eventInstances.at(499).recurringEventId.has_value(), true);
  QCOMPARE(fixture.eventInstances.at(500).recurringEventId.has_value(), false);
  QCOMPARE(fixture.queuedMutations.size(), std::size_t{500});
}

void NativePerformanceFixtureTest::serializesDeterministicallyWithoutCredentials() {
  const QByteArray first = hcb::NativePerformanceFixtureGenerator::toJson(
      hcb::NativePerformanceFixtureGenerator::small());
  const QByteArray second = hcb::NativePerformanceFixtureGenerator::toJson(
      hcb::NativePerformanceFixtureGenerator::small());

  QCOMPARE(first, second);
  QVERIFY(!first.contains("oauth"));
  QVERIFY(!first.contains("access_token"));
  QVERIFY(!first.contains("refresh_token"));

  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(first, &error);
  QCOMPARE(error.error, QJsonParseError::NoError);
  const QJsonObject root = document.object();
  QCOMPARE(root.value(QStringLiteral("generatedDataOnly")).toBool(), true);
  QCOMPARE(root.value(QStringLiteral("tasks")).toArray().size(), 50);
  QCOMPARE(root.value(QStringLiteral("eventInstances")).toArray().size(), 20);
  QCOMPARE(root.value(QStringLiteral("notes")).toArray().size(), 10);
}

QTEST_MAIN(NativePerformanceFixtureTest)
#include "NativePerformanceFixtureTest.moc"
