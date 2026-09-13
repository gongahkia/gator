#include <QtTest>

#include "app/SystemTrayAdapter.h"

#include <QSystemTrayIcon>

class SystemTrayAdapterTest final : public QObject {
  Q_OBJECT

private slots:
  void dispatchesConfiguredActions();
  void rejectsUnavailableActions();
  void reportsInvalidIcon();
  void reportsDisabledAndPlatformStatus();
};

void SystemTrayAdapterTest::dispatchesConfiguredActions() {
  int calls = 0;
  hcb::TrayActionDispatcher dispatcher({.openMainWindow = [&calls] { ++calls; },
                                        .toggleMainWindow = {},
                                        .openQuickCapture = {},
                                        .refresh = {},
                                        .openSettings = {},
                                        .quit = {}});

  QVERIFY(dispatcher.isAvailable(hcb::TrayAction::OpenMainWindow));
  QVERIFY(dispatcher.dispatch(hcb::TrayAction::OpenMainWindow));
  QCOMPARE(calls, 1);
}

void SystemTrayAdapterTest::rejectsUnavailableActions() {
  hcb::TrayActionDispatcher dispatcher({});

  QVERIFY(!dispatcher.isAvailable(hcb::TrayAction::Refresh));
  QVERIFY(!dispatcher.dispatch(hcb::TrayAction::Refresh));
}

void SystemTrayAdapterTest::reportsInvalidIcon() {
  hcb::SystemTrayAdapter adapter({}, {}, this);

  adapter.setEnabled(true);
  const hcb::TrayStatus status = adapter.status();
  QCOMPARE(status.state, hcb::TrayStatusState::Error);
  QVERIFY(!status.visible);
  QVERIFY(!status.supportsMessages);
}

void SystemTrayAdapterTest::reportsDisabledAndPlatformStatus() {
  hcb::SystemTrayAdapter adapter(hcb::SystemTrayAdapter::defaultIcon(), {}, this);

  adapter.setEnabled(false);
  const hcb::TrayStatus disabled = adapter.status();
  QCOMPARE(disabled.state, hcb::TrayStatusState::Disabled);
  QVERIFY(!disabled.visible);
  QVERIFY(!disabled.supportsMessages);
  const hcb::NotificationStatus disabledNotifications = adapter.notificationStatus();
  QCOMPARE(disabledNotifications.state, hcb::NotificationState::Disabled);

  adapter.setEnabled(true);
  const hcb::TrayStatus enabled = adapter.status();
  if (QSystemTrayIcon::isSystemTrayAvailable()) {
    QCOMPARE(enabled.state, hcb::TrayStatusState::Ready);
    QCOMPARE(enabled.supportsMessages, QSystemTrayIcon::supportsMessages());
  } else {
    QCOMPARE(enabled.state, hcb::TrayStatusState::Unsupported);
    QVERIFY(!enabled.visible);
    QVERIFY(!enabled.supportsMessages);
  }

  const hcb::NotificationStatus notifications = adapter.notificationStatus();
  if (QSystemTrayIcon::isSystemTrayAvailable() && QSystemTrayIcon::supportsMessages()) {
    QCOMPARE(notifications.state, hcb::NotificationState::Ready);
  } else {
    QCOMPARE(notifications.state, hcb::NotificationState::Unsupported);
  }
}

QTEST_MAIN(SystemTrayAdapterTest)

#include "SystemTrayAdapterTest.moc"
