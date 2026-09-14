#include <QtTest/QTest>

#include "core/PythonBridgeProjection.h"

#include <QJsonDocument>
#include <QJsonObject>

#include <variant>

namespace {

QJsonObject object(const char* source) {
  const QJsonDocument document = QJsonDocument::fromJson(QByteArray(source));
  Q_ASSERT(document.isObject());
  return document.object();
}

} // namespace

class PythonBridgeProjectionTest final : public QObject {
  Q_OBJECT

private slots:
  void projectsSummaryAndTaskPage();
  void projectsDateBoundedEvents();
  void projectsSearchResults();
  void rejectsCrossAccountAndMalformedPayloads();
};

void PythonBridgeProjectionTest::projectsSummaryAndTaskPage() {
  const QJsonObject summaryData = object(R"json(
    {"workspace":{"account":{"id":"work","email":"work@example.test"},"pending":2,
    "task_lists":[{"id":"inbox","account_id":"work","title":"Inbox","remote_id":null,
      "position":3,"selected":false,"metadata":{"etag":null,"local_updated_at":"2026-09-14T00:00:00+00:00"}}],
    "calendars":[{"id":"primary","account_id":"work","summary":"Primary","remote_id":null,
      "description":null,"time_zone":"UTC","color":"#123456","selected":true,"hidden":false,
      "metadata":{"etag":null,"local_updated_at":"2026-09-14T00:00:00+00:00"}}]}}
  )json");
  const hcb::PythonBridgeWorkspaceSummaryOrError decoded =
      hcb::PythonBridgeProjection::workspaceSummary(summaryData);
  QVERIFY(std::holds_alternative<hcb::PythonBridgeWorkspaceSummary>(decoded));
  const hcb::PythonBridgeWorkspaceSummary& summary =
      std::get<hcb::PythonBridgeWorkspaceSummary>(decoded);
  QCOMPARE(summary.accountId, QStringLiteral("work"));
  QCOMPARE(summary.accountEmail, QStringLiteral("work@example.test"));
  QCOMPARE(summary.pending, 2);
  QCOMPARE(summary.taskLists.size(), 1);
  QCOMPARE(summary.taskLists.first().sortOrder, std::int64_t(3));
  QVERIFY(!summary.taskLists.first().selected);
  QCOMPARE(summary.calendars.first().backgroundColor,
           std::optional<QString>(QStringLiteral("#123456")));

  const QJsonObject taskPageData = object(R"json(
    {"page":{"tasks":[{"id":"task-1","account_id":"work","list_id":"inbox",
      "title":"Bridge task","notes":"private note","parent_id":null,"due":"2026-09-15",
      "due_time_zone":null,"priority":"high","status":"needsAction"}],"next_cursor":"djE6MjAw"}}
  )json");
  const hcb::PythonBridgeTaskPageOrError page = hcb::PythonBridgeProjection::taskPage(
      taskPageData, hcb::PythonBridgeProjection::taskListTitles(summary));
  QVERIFY(std::holds_alternative<hcb::PythonBridgeTaskPage>(page));
  const hcb::TaskModelTask& task = std::get<hcb::PythonBridgeTaskPage>(page).tasks.first();
  QCOMPARE(task.taskListTitle, QStringLiteral("Inbox"));
  QCOMPARE(task.priority, hcb::TaskPriority::High);
  QCOMPARE(task.due->at, std::optional<QString>(QStringLiteral("2026-09-15")));
  QVERIFY(!task.completed);
}

void PythonBridgeProjectionTest::projectsDateBoundedEvents() {
  const QJsonObject data = object(R"json(
    {"workspace":{"events":[{"id":"event-1","calendar_id":"primary","remote_id":null,
      "summary":"Planning","status":"confirmed","description":null,"location":"Desk",
      "start":{"kind":"dateTime","value":"2026-09-15T09:00:00+00:00","time_zone":"UTC"},
      "end":{"kind":"dateTime","value":"2026-09-15T10:00:00+00:00","time_zone":"UTC"},
      "color_id":null,"transparency":null,"visibility":null,"event_type":null,
      "attendees":[],"reminder_overrides":[],"attachments":[],
      "metadata":{"etag":null,"local_updated_at":"2026-09-14T00:00:00+00:00"}}]}}
  )json");
  const hcb::PythonBridgeEventRangeOrError decoded = hcb::PythonBridgeProjection::eventRange(data);
  QVERIFY(std::holds_alternative<QList<hcb::CalendarEventSummary>>(decoded));
  const hcb::CalendarEventSummary& event =
      std::get<QList<hcb::CalendarEventSummary>>(decoded).first();
  QCOMPARE(event.title, QStringLiteral("Planning"));
  QCOMPARE(event.startAt, QStringLiteral("2026-09-15T09:00:00+00:00"));
  QCOMPARE(event.startTimeZone, std::optional<QString>(QStringLiteral("UTC")));
  QVERIFY(!event.allDay);
  QCOMPARE(event.location, std::optional<QString>(QStringLiteral("Desk")));
}

void PythonBridgeProjectionTest::projectsSearchResults() {
  const QJsonObject data = object(R"json(
    {"results":[
      {"kind":"task","score":100,"item":{"id":"task-1","account_id":"work",
       "title":"Plan bridge search","notes":"Validate the core path","due":"2026-09-15"}},
      {"kind":"event","score":80,"item":{"id":"event-1","account_id":"work",
       "summary":"Planning","location":"Desk","start":{"kind":"dateTime",
       "value":"2026-09-15T09:00:00+00:00"}}},
      {"kind":"drive","score":60,"item":{"id":"file-1","account_id":"work","name":"Plan"}}
    ]}
  )json");
  const hcb::PythonBridgeSearchOrError decoded =
      hcb::PythonBridgeProjection::searchResults(data, QStringLiteral("work"));
  QVERIFY(std::holds_alternative<QList<hcb::LocalSearchRankedResult>>(decoded));
  const QList<hcb::LocalSearchRankedResult>& results =
      std::get<QList<hcb::LocalSearchRankedResult>>(decoded);
  QCOMPARE(results.size(), 2);
  QCOMPARE(results.first().resource, hcb::LocalSearchResource::Task);
  QCOMPARE(results.first().title, QStringLiteral("Plan bridge search"));
  QCOMPARE(results.first().detail, QStringLiteral("Validate the core path"));
  QCOMPARE(results.first().scheduledAt, QStringLiteral("2026-09-15"));
  QCOMPARE(results.last().resource, hcb::LocalSearchResource::Event);
  QCOMPARE(results.last().scheduledAt, QStringLiteral("2026-09-15T09:00:00+00:00"));
}

void PythonBridgeProjectionTest::rejectsCrossAccountAndMalformedPayloads() {
  QVERIFY(std::holds_alternative<hcb::AppError>(hcb::PythonBridgeProjection::searchResults(
      object(R"json({"results":[{"kind":"task","score":100,"item":{"id":"task-1",
      "account_id":"other","title":"Wrong account","notes":null,"due":null}}]})json"),
      QStringLiteral("work"))));
  const QJsonObject crossAccount = object(R"json(
    {"workspace":{"account":{"id":"work","email":"work@example.test"},"pending":0,
    "task_lists":[{"id":"inbox","account_id":"other","title":"Inbox","remote_id":null,
      "position":0,"metadata":{"etag":null,"local_updated_at":"2026-09-14T00:00:00+00:00"}}],
    "calendars":[]}}
  )json");
  QVERIFY(std::holds_alternative<hcb::AppError>(
      hcb::PythonBridgeProjection::workspaceSummary(crossAccount)));
  QVERIFY(std::holds_alternative<hcb::AppError>(hcb::PythonBridgeProjection::taskPage(
      object("{\"page\":{\"tasks\":[{\"id\":\"bad\"}]}}"), {})));
  QVERIFY(std::holds_alternative<hcb::AppError>(hcb::PythonBridgeProjection::taskPage(
      object(R"json({"page":{"tasks":[{"id":"task-1","account_id":"work","list_id":"unknown",
      "title":"Bridge task","notes":null,"parent_id":null,"due":null,"due_time_zone":null,
      "priority":"none","status":"needsAction"}]}})json"),
      {{QStringLiteral("inbox"), QStringLiteral("Inbox")}})));
  QVERIFY(
      std::holds_alternative<hcb::AppError>(hcb::PythonBridgeProjection::eventRange(object(R"json(
    {"workspace":{"events":[{"id":"event-1","calendar_id":"primary","remote_id":null,
      "summary":"Planning","status":"confirmed","description":null,"location":null,
      "start":{"kind":"dateTime","value":"2026-09-15T09:00:00+00:00","time_zone":"UTC"},
      "end":{"kind":"dateTime","value":"2026-09-15T10:00:00+00:00","time_zone":"UTC"},
      "color_id":null,"transparency":null,"visibility":null,"event_type":null,
      "attendees":{},"reminder_overrides":[],"attachments":[],
      "metadata":{"etag":null,"local_updated_at":"2026-09-14T00:00:00+00:00"}}]}}
  )json"))));
}

QTEST_GUILESS_MAIN(PythonBridgeProjectionTest)

#include "PythonBridgeProjectionTest.moc"
