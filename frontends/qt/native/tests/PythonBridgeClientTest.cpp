#include <QtTest/QTest>

#include "core/PythonBridgeClient.h"
#include "support/MockNetworkAccessManager.h"

#include <QFile>
#include <QJsonDocument>
#include <QTemporaryDir>
#include <QUrlQuery>

#include <chrono>
#include <future>
#include <variant>

namespace {

constexpr auto kToken = "test_desktop_bridge_token_123456";

hcb::PythonBridgeConnection connection() {
  return {.endpoint = QUrl(QStringLiteral("http://127.0.0.1:4242")),
          .bearerToken = QByteArray(kToken)};
}

void waitFor(std::future<hcb::PythonBridgeResult>& future) {
  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
}

} // namespace

class PythonBridgeClientTest final : public QObject {
  Q_OBJECT

private slots:
  void readsPrivateLoopbackDescriptor();
  void rejectsUnsafeDescriptor();
  void requestsWorkspaceAndBoundedPages();
  void requestsBoundedSearch();
  void requestsAuthenticationState();
  void sendsIdempotentMutations();
  void sendsTaskMoveMutation();
  void sendsTaskRecurrenceMutations();
  void sendsTaskListAndCalendarManagementMutations();
  void rejectsInvalidRequestsBeforeNetwork();
  void cancelsBeforeNetwork();
  void cancelsAfterDispatch();
  void propagatesBridgeErrors();
  void rejectsMalformedBridgeResponses();
};

void PythonBridgeClientTest::readsPrivateLoopbackDescriptor() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const QString path = directory.filePath(QStringLiteral("bridge.json"));
  QFile file(path);
  QVERIFY(file.open(QIODevice::WriteOnly));
  const QJsonObject descriptor{{QStringLiteral("api_version"), 1},
                               {QStringLiteral("url"), QStringLiteral("http://127.0.0.1:4242")},
                               {QStringLiteral("token"), QString::fromLatin1(kToken)}};
  const QByteArray payload = QJsonDocument(descriptor).toJson(QJsonDocument::Compact);
  QCOMPARE(file.write(payload), qint64(payload.size()));
  file.close();
  QVERIFY(file.setPermissions(QFile::ReadOwner | QFile::WriteOwner));

  const hcb::PythonBridgeConnectionOrError result =
      hcb::PythonBridgeClient::readConnectionDescriptor(path);
  QVERIFY(std::holds_alternative<hcb::PythonBridgeConnection>(result));
  const hcb::PythonBridgeConnection& loaded = std::get<hcb::PythonBridgeConnection>(result);
  QCOMPARE(loaded.endpoint, QUrl(QStringLiteral("http://127.0.0.1:4242")));
  QCOMPARE(loaded.bearerToken, QByteArray(kToken));
}

void PythonBridgeClientTest::rejectsUnsafeDescriptor() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const QString path = directory.filePath(QStringLiteral("bridge.json"));
  QFile file(path);
  QVERIFY(file.open(QIODevice::WriteOnly));
  QVERIFY(file.write("{}") > 0);
  file.close();
  QVERIFY(file.setPermissions(QFile::ReadOwner | QFile::ReadGroup | QFile::WriteOwner));

  const hcb::PythonBridgeConnectionOrError result =
      hcb::PythonBridgeClient::readConnectionDescriptor(path);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Configuration);
}

void PythonBridgeClientTest::requestsWorkspaceAndBoundedPages() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"workspace\":{}}}")});
  manager.enqueue(
      {.body = QByteArray(
           "{\"api_version\":1,\"data\":{\"page\":{\"tasks\":[],\"next_cursor\":null}}}")});
  manager.enqueue(
      {.body = QByteArray("{\"api_version\":1,\"data\":{\"workspace\":{\"events\":[]}}}")});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  std::future<hcb::PythonBridgeResult> workspace = client.workspace(QStringLiteral("work"));
  waitFor(workspace);
  QVERIFY(std::holds_alternative<QJsonObject>(workspace.get()));

  std::future<hcb::PythonBridgeResult> tasks = client.taskPage(
      QStringLiteral("work"), 200, QStringLiteral("djE6MjAw"), QStringLiteral("inbox"));
  waitFor(tasks);
  QVERIFY(std::holds_alternative<QJsonObject>(tasks.get()));

  std::future<hcb::PythonBridgeResult> events =
      client.eventRange(QStringLiteral("work"), QDate(2026, 9, 14), QDate(2026, 9, 15));
  waitFor(events);
  QVERIFY(std::holds_alternative<QJsonObject>(events.get()));

  QCOMPARE(manager.requests().size(), 3);
  QCOMPARE(manager.requests().at(0).request.rawHeader("Authorization"),
           QByteArray("Bearer ") + QByteArray(kToken));
  QCOMPARE(manager.requests().at(0).request.url().path(),
           QStringLiteral("/v1/accounts/work/workspace"));
  const QUrlQuery taskQuery(manager.requests().at(1).request.url());
  QCOMPARE(taskQuery.queryItemValue(QStringLiteral("limit")), QStringLiteral("200"));
  QCOMPARE(taskQuery.queryItemValue(QStringLiteral("cursor")), QStringLiteral("djE6MjAw"));
  QCOMPARE(taskQuery.queryItemValue(QStringLiteral("list_id")), QStringLiteral("inbox"));
  const QUrlQuery eventQuery(manager.requests().at(2).request.url());
  QCOMPARE(eventQuery.queryItemValue(QStringLiteral("include")), QStringLiteral("events"));
  QCOMPARE(eventQuery.queryItemValue(QStringLiteral("start")), QStringLiteral("2026-09-14"));
  QCOMPARE(eventQuery.queryItemValue(QStringLiteral("end")), QStringLiteral("2026-09-15"));
}

void PythonBridgeClientTest::requestsBoundedSearch() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"results\":[]}}")});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  std::future<hcb::PythonBridgeResult> future =
      client.search(QStringLiteral("work"), QStringLiteral("type:task planning"), 50);
  waitFor(future);
  QVERIFY(std::holds_alternative<QJsonObject>(future.get()));
  QCOMPARE(manager.requests().size(), 1);
  QCOMPARE(manager.requests().first().request.url().path(),
           QStringLiteral("/v1/accounts/work/search"));
  const QUrlQuery query(manager.requests().first().request.url());
  QCOMPARE(query.queryItemValue(QStringLiteral("q")), QStringLiteral("type:task planning"));
  QCOMPARE(query.queryItemValue(QStringLiteral("limit")), QStringLiteral("50"));
}

void PythonBridgeClientTest::requestsAuthenticationState() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(
                       "{\"api_version\":1,\"data\":{\"authentication\":{\"connected\":false}}}")});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  std::future<hcb::PythonBridgeResult> future = client.authenticationState(QStringLiteral("work"));
  waitFor(future);
  QVERIFY(std::holds_alternative<QJsonObject>(future.get()));
  QCOMPARE(manager.requests().size(), 1);
  QCOMPARE(manager.requests().first().request.url().path(),
           QStringLiteral("/v1/accounts/work/auth"));
}

void PythonBridgeClientTest::sendsIdempotentMutations() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"task\":{}}}")});
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"event\":{}}}")});
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"operation\":{}}}")});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  const QByteArray taskKey("task-request-123");
  std::future<hcb::PythonBridgeResult> task =
      client.createTask(QStringLiteral("work"),
                        QJsonObject{{QStringLiteral("list_id"), QStringLiteral("inbox")},
                                    {QStringLiteral("title"), QStringLiteral("Bridge task")}},
                        taskKey);
  waitFor(task);
  QVERIFY(std::holds_alternative<QJsonObject>(task.get()));

  const QByteArray eventKey("event-request-456");
  std::future<hcb::PythonBridgeResult> event =
      client.deleteEvent(QStringLiteral("work"), QStringLiteral("event-1"), eventKey);
  waitFor(event);
  QVERIFY(std::holds_alternative<QJsonObject>(event.get()));

  std::future<hcb::PythonBridgeResult> sync = client.startSync(QStringLiteral("work"));
  waitFor(sync);
  QVERIFY(std::holds_alternative<QJsonObject>(sync.get()));

  QCOMPARE(manager.requests().size(), 3);
  QCOMPARE(manager.requests().at(0).request.url().path(),
           QStringLiteral("/v1/accounts/work/tasks"));
  QCOMPARE(manager.requests().at(0).request.rawHeader("Idempotency-Key"), taskKey);
  QCOMPARE(manager.requests().at(0).request.header(QNetworkRequest::ContentTypeHeader).toString(),
           QStringLiteral("application/json"));
  QCOMPARE(manager.requests().at(0).body,
           QByteArray("{\"list_id\":\"inbox\",\"title\":\"Bridge task\"}"));
  QCOMPARE(manager.requests().at(1).request.url().path(),
           QStringLiteral("/v1/accounts/work/events/event-1"));
  QCOMPARE(manager.requests().at(1).request.rawHeader("Idempotency-Key"), eventKey);
  QVERIFY(manager.requests().at(1).body.isEmpty());
  QCOMPARE(manager.requests().at(2).request.url().path(), QStringLiteral("/v1/accounts/work/sync"));
  QVERIFY(manager.requests().at(2).request.rawHeader("Idempotency-Key").isEmpty());
  QCOMPARE(manager.requests().at(2).body, QByteArray("{}"));
}

void PythonBridgeClientTest::sendsTaskMoveMutation() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"task\":{}}}")});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  const QByteArray key("task-move-request-123");
  std::future<hcb::PythonBridgeResult> move =
      client.moveTask(QStringLiteral("work"),
                      QStringLiteral("task-1"),
                      QJsonObject{{QStringLiteral("list_id"), QStringLiteral("archive")}},
                      key);
  waitFor(move);
  QVERIFY(std::holds_alternative<QJsonObject>(move.get()));

  QCOMPARE(manager.requests().size(), 1);
  QCOMPARE(manager.requests().first().request.url().path(),
           QStringLiteral("/v1/accounts/work/tasks/task-1/move"));
  QCOMPARE(manager.requests().first().request.rawHeader("Idempotency-Key"), key);
  QCOMPARE(manager.requests().first().body, QByteArray("{\"list_id\":\"archive\"}"));
}

void PythonBridgeClientTest::sendsTaskRecurrenceMutations() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"tasks\":[]}}")});
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"tasks\":[]}}")});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  std::future<hcb::PythonBridgeResult> stop =
      client.stopTaskRecurrence(QStringLiteral("work"),
                                QStringLiteral("task-1"),
                                QStringLiteral("following"),
                                QByteArray("recurrence-stop-123"));
  waitFor(stop);
  QVERIFY(std::holds_alternative<QJsonObject>(stop.get()));

  std::future<hcb::PythonBridgeResult> split = client.splitTaskRecurrence(
      QStringLiteral("work"), QStringLiteral("task-1"), QByteArray("recurrence-split-456"));
  waitFor(split);
  QVERIFY(std::holds_alternative<QJsonObject>(split.get()));

  QCOMPARE(manager.requests().size(), 2);
  QCOMPARE(manager.requests().at(0).request.url().path(),
           QStringLiteral("/v1/accounts/work/tasks/task-1/recurrence/stop"));
  QCOMPARE(manager.requests().at(0).request.rawHeader("Idempotency-Key"),
           QByteArray("recurrence-stop-123"));
  QCOMPARE(manager.requests().at(0).body, QByteArray("{\"scope\":\"following\"}"));
  QCOMPARE(manager.requests().at(1).request.url().path(),
           QStringLiteral("/v1/accounts/work/tasks/task-1/recurrence/split"));
  QCOMPARE(manager.requests().at(1).request.rawHeader("Idempotency-Key"),
           QByteArray("recurrence-split-456"));
  QCOMPARE(manager.requests().at(1).body, QByteArray("{}"));

  std::future<hcb::PythonBridgeResult> invalid =
      client.stopTaskRecurrence(QStringLiteral("work"),
                                QStringLiteral("task-1"),
                                QStringLiteral("invalid"),
                                QByteArray("recurrence-invalid-789"));
  const hcb::PythonBridgeResult invalidResult = invalid.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidResult));
  QCOMPARE(manager.requests().size(), 2);
}

void PythonBridgeClientTest::sendsTaskListAndCalendarManagementMutations() {
  hcb::test::MockNetworkAccessManager manager;
  for (int index = 0; index < 8; ++index) {
    manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{}}")});
  }
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  const QByteArray taskListKey("task-list-request-123");
  std::future<hcb::PythonBridgeResult> createTaskList =
      client.createTaskList(QStringLiteral("work"),
                            QJsonObject{{QStringLiteral("title"), QStringLiteral("Bridge list")}},
                            taskListKey);
  waitFor(createTaskList);
  QVERIFY(std::holds_alternative<QJsonObject>(createTaskList.get()));

  std::future<hcb::PythonBridgeResult> updateTaskList =
      client.updateTaskList(QStringLiteral("work"),
                            QStringLiteral("list-1"),
                            QJsonObject{{QStringLiteral("selected"), false}},
                            QByteArray("task-list-update-456"));
  waitFor(updateTaskList);
  QVERIFY(std::holds_alternative<QJsonObject>(updateTaskList.get()));

  std::future<hcb::PythonBridgeResult> deleteTaskList = client.deleteTaskList(
      QStringLiteral("work"), QStringLiteral("list-1"), QByteArray("task-list-delete-789"));
  waitFor(deleteTaskList);
  QVERIFY(std::holds_alternative<QJsonObject>(deleteTaskList.get()));

  std::future<hcb::PythonBridgeResult> createCalendar = client.createCalendar(
      QStringLiteral("work"),
      QJsonObject{{QStringLiteral("summary"), QStringLiteral("Bridge calendar")}},
      QByteArray("calendar-create-123"));
  waitFor(createCalendar);
  QVERIFY(std::holds_alternative<QJsonObject>(createCalendar.get()));

  std::future<hcb::PythonBridgeResult> updateCalendar =
      client.updateCalendar(QStringLiteral("work"),
                            QStringLiteral("calendar-1"),
                            QJsonObject{{QStringLiteral("hidden"), true}},
                            QByteArray("calendar-update-456"));
  waitFor(updateCalendar);
  QVERIFY(std::holds_alternative<QJsonObject>(updateCalendar.get()));

  std::future<hcb::PythonBridgeResult> deleteCalendar = client.deleteCalendar(
      QStringLiteral("work"), QStringLiteral("calendar-1"), QByteArray("calendar-delete-789"));
  waitFor(deleteCalendar);
  QVERIFY(std::holds_alternative<QJsonObject>(deleteCalendar.get()));

  std::future<hcb::PythonBridgeResult> subscribeCalendar = client.subscribeCalendar(
      QStringLiteral("work"),
      QJsonObject{{QStringLiteral("remote_calendar_id"), QStringLiteral("synthetic")}},
      QByteArray("calendar-subscribe-123"));
  waitFor(subscribeCalendar);
  QVERIFY(std::holds_alternative<QJsonObject>(subscribeCalendar.get()));

  std::future<hcb::PythonBridgeResult> unsubscribeCalendar = client.unsubscribeCalendar(
      QStringLiteral("work"), QStringLiteral("calendar-1"), QByteArray("calendar-unsubscribe-456"));
  waitFor(unsubscribeCalendar);
  QVERIFY(std::holds_alternative<QJsonObject>(unsubscribeCalendar.get()));

  QCOMPARE(manager.requests().size(), 8);
  QCOMPARE(manager.requests().at(0).request.url().path(),
           QStringLiteral("/v1/accounts/work/task-lists"));
  QCOMPARE(manager.requests().at(0).request.rawHeader("Idempotency-Key"), taskListKey);
  QCOMPARE(manager.requests().at(1).request.url().path(),
           QStringLiteral("/v1/accounts/work/task-lists/list-1"));
  QCOMPARE(manager.requests().at(2).request.url().path(),
           QStringLiteral("/v1/accounts/work/task-lists/list-1"));
  QCOMPARE(manager.requests().at(3).request.url().path(),
           QStringLiteral("/v1/accounts/work/calendars"));
  QCOMPARE(manager.requests().at(4).request.url().path(),
           QStringLiteral("/v1/accounts/work/calendars/calendar-1"));
  QCOMPARE(manager.requests().at(5).request.url().path(),
           QStringLiteral("/v1/accounts/work/calendars/calendar-1"));
  QCOMPARE(manager.requests().at(6).request.url().path(),
           QStringLiteral("/v1/accounts/work/calendar-subscriptions"));
  QCOMPARE(manager.requests().at(7).request.url().path(),
           QStringLiteral("/v1/accounts/work/calendar-subscriptions/calendar-1"));
  QCOMPARE(manager.requests().at(1).body, QByteArray("{\"selected\":false}"));
  QCOMPARE(manager.requests().at(4).body, QByteArray("{\"hidden\":true}"));
  QVERIFY(manager.requests().at(2).body.isEmpty());
  QVERIFY(manager.requests().at(5).body.isEmpty());
  QVERIFY(manager.requests().at(7).body.isEmpty());
}

void PythonBridgeClientTest::rejectsInvalidRequestsBeforeNetwork() {
  hcb::test::MockNetworkAccessManager manager;
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  std::future<hcb::PythonBridgeResult> future = client.taskPage(QStringLiteral("bad/account"), 0);
  const hcb::PythonBridgeResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
  QCOMPARE(manager.requests().size(), 0);

  std::future<hcb::PythonBridgeResult> search = client.search(QStringLiteral("work"), {});
  const hcb::PythonBridgeResult searchResult = search.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(searchResult));
  QCOMPARE(std::get<hcb::AppError>(searchResult).code(), hcb::AppErrorCode::Validation);
  QCOMPARE(manager.requests().size(), 0);
}

void PythonBridgeClientTest::cancelsBeforeNetwork() {
  hcb::test::MockNetworkAccessManager manager;
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);
  hcb::CancellationSource cancellation;
  QVERIFY(cancellation.requestStop());

  std::future<hcb::PythonBridgeResult> future =
      client.workspace(QStringLiteral("work"), cancellation.token());
  const hcb::PythonBridgeResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Network);
  QCOMPARE(manager.requests().size(), 0);
}

void PythonBridgeClientTest::cancelsAfterDispatch() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":{\"workspace\":{}}}"),
                   .delayMilliseconds = 200});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);
  hcb::CancellationSource cancellation;

  std::future<hcb::PythonBridgeResult> future =
      client.workspace(QStringLiteral("work"), cancellation.token());
  QTRY_COMPARE(manager.requests().size(), 1);
  QVERIFY(cancellation.requestStop());
  waitFor(future);
  const hcb::PythonBridgeResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Network);
  QCOMPARE(std::get<hcb::AppError>(result).message(),
           QStringLiteral("HCB bridge request was cancelled"));
}

void PythonBridgeClientTest::propagatesBridgeErrors() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 401,
                   .body = QByteArray("{\"api_version\":1,\"error\":{\"code\":\"unauthorized\","
                                      "\"message\":\"missing or invalid bridge token\"}}"),
                   .error = QNetworkReply::AuthenticationRequiredError});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  std::future<hcb::PythonBridgeResult> future = client.workspace(QStringLiteral("work"));
  waitFor(future);
  const hcb::PythonBridgeResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Network);
  QCOMPARE(std::get<hcb::AppError>(result).message(),
           QStringLiteral("missing or invalid bridge token"));
}

void PythonBridgeClientTest::rejectsMalformedBridgeResponses() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("not JSON")});
  manager.enqueue({.body = QByteArray("{\"api_version\":2,\"data\":{}}")});
  manager.enqueue({.body = QByteArray("{\"api_version\":1,\"data\":[]}")});
  hcb::PythonBridgeClient client(connection(), nullptr, &manager);

  const auto checkFailure = [&client](const QString& expected) {
    std::future<hcb::PythonBridgeResult> future = client.workspace(QStringLiteral("work"));
    waitFor(future);
    const hcb::PythonBridgeResult result = future.get();
    QVERIFY(std::holds_alternative<hcb::AppError>(result));
    QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Network);
    QCOMPARE(std::get<hcb::AppError>(result).message(), expected);
  };

  checkFailure(QStringLiteral("HCB bridge returned malformed JSON"));
  checkFailure(QStringLiteral("HCB bridge protocol version is unsupported"));
  checkFailure(QStringLiteral("HCB bridge response has no data object"));
}

QTEST_GUILESS_MAIN(PythonBridgeClientTest)

#include "PythonBridgeClientTest.moc"
