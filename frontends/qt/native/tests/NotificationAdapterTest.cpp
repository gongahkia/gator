#include <QtTest>

#include "app/NotificationAdapter.h"

#include <optional>

class NotificationAdapterTest final : public QObject {
  Q_OBJECT

private slots:
  void reportsTransportStatus();
  void rejectsInvalidRequests();
  void submitsReadyRequest();
  void reportsSubmissionFailure();
};

namespace {

class FakeNotificationTransport final : public hcb::NotificationTransport {
public:
  [[nodiscard]] hcb::NotificationStatus notificationStatus() const override { return status; }

  [[nodiscard]] bool showNotification(const hcb::NotificationRequest& request) override {
    submitted = request;
    return submitResult;
  }

  hcb::NotificationStatus status{.state = hcb::NotificationState::Ready,
                                 .supportsMessages = true,
                                 .message = QStringLiteral("ready")};
  bool submitResult{true};
  std::optional<hcb::NotificationRequest> submitted;
};

} // namespace

void NotificationAdapterTest::reportsTransportStatus() {
  FakeNotificationTransport transport;
  transport.status = {.state = hcb::NotificationState::Unsupported,
                      .supportsMessages = false,
                      .message = QStringLiteral("unavailable")};
  hcb::NotificationAdapter adapter(transport);

  const hcb::NotificationStatus status = adapter.status();
  QCOMPARE(status.state, hcb::NotificationState::Unsupported);
  QVERIFY(!status.supportsMessages);
  QCOMPARE(status.message, QStringLiteral("unavailable"));

  const hcb::NotificationStatus send =
      adapter.send({.title = QStringLiteral("Reminder"), .body = QStringLiteral("Review tasks")});
  QCOMPARE(send.state, hcb::NotificationState::Unsupported);
  QVERIFY(!transport.submitted.has_value());
}

void NotificationAdapterTest::rejectsInvalidRequests() {
  FakeNotificationTransport transport;
  hcb::NotificationAdapter adapter(transport);

  const hcb::NotificationStatus empty = adapter.send({});
  QCOMPARE(empty.state, hcb::NotificationState::Error);
  QVERIFY(!transport.submitted.has_value());

  const hcb::NotificationStatus invalidTimeout =
      adapter.send({.title = QStringLiteral("Reminder"),
                    .body = QStringLiteral("Review tasks"),
                    .timeoutMilliseconds = 0});
  QCOMPARE(invalidTimeout.state, hcb::NotificationState::Error);
  QVERIFY(!transport.submitted.has_value());
}

void NotificationAdapterTest::submitsReadyRequest() {
  FakeNotificationTransport transport;
  hcb::NotificationAdapter adapter(transport);
  const hcb::NotificationRequest request{.title = QStringLiteral("Reminder"),
                                         .body = QStringLiteral("Review tasks"),
                                         .icon = hcb::NotificationIcon::Warning,
                                         .timeoutMilliseconds = 3'000};

  const hcb::NotificationStatus result = adapter.send(request);
  QCOMPARE(result.state, hcb::NotificationState::Ready);
  QVERIFY(transport.submitted.has_value());
  QCOMPARE(transport.submitted->title, request.title);
  QCOMPARE(transport.submitted->body, request.body);
  QCOMPARE(transport.submitted->icon, request.icon);
  QCOMPARE(transport.submitted->timeoutMilliseconds, request.timeoutMilliseconds);
}

void NotificationAdapterTest::reportsSubmissionFailure() {
  FakeNotificationTransport transport;
  transport.submitResult = false;
  hcb::NotificationAdapter adapter(transport);

  const hcb::NotificationStatus result =
      adapter.send({.title = QStringLiteral("Reminder"), .body = QStringLiteral("Review tasks")});
  QCOMPARE(result.state, hcb::NotificationState::Error);
  QVERIFY(transport.submitted.has_value());
}

QTEST_APPLESS_MAIN(NotificationAdapterTest)

#include "NotificationAdapterTest.moc"
