#include "app/SystemTrayAdapter.h"

#include <QAction>
#include <QMenu>
#include <QSystemTrayIcon>

#include <utility>

namespace hcb {
namespace {

[[nodiscard]] const std::function<void()>& handlerFor(const TrayActionHandlers& handlers,
                                                      TrayAction action) {
  switch (action) {
  case TrayAction::OpenMainWindow:
    return handlers.openMainWindow;
  case TrayAction::ToggleMainWindow:
    return handlers.toggleMainWindow;
  case TrayAction::OpenQuickCapture:
    return handlers.openQuickCapture;
  case TrayAction::Refresh:
    return handlers.refresh;
  case TrayAction::OpenSettings:
    return handlers.openSettings;
  case TrayAction::Quit:
    return handlers.quit;
  }
  return handlers.quit;
}

[[nodiscard]] QSystemTrayIcon::MessageIcon messageIconFor(NotificationIcon icon) {
  switch (icon) {
  case NotificationIcon::Information:
    return QSystemTrayIcon::Information;
  case NotificationIcon::Warning:
    return QSystemTrayIcon::Warning;
  case NotificationIcon::Critical:
    return QSystemTrayIcon::Critical;
  }
  return QSystemTrayIcon::Information;
}

} // namespace

TrayActionDispatcher::TrayActionDispatcher(TrayActionHandlers handlers)
    : handlers_(std::move(handlers)) {}

bool TrayActionDispatcher::isAvailable(TrayAction action) const {
  return static_cast<bool>(handlerFor(handlers_, action));
}

bool TrayActionDispatcher::dispatch(TrayAction action) const {
  const std::function<void()>& handler = handlerFor(handlers_, action);
  if (!handler) {
    return false;
  }
  handler();
  return true;
}

SystemTrayAdapter::SystemTrayAdapter(QIcon icon, TrayActionHandlers handlers, QObject* parent)
    : QObject(parent), icon_(std::move(icon)), dispatcher_(std::move(handlers)) {}

SystemTrayAdapter::~SystemTrayAdapter() {
  if (tray_ != nullptr) {
    tray_->hide();
    tray_->setContextMenu(nullptr);
  }
}

QIcon SystemTrayAdapter::defaultIcon() { return QIcon(QStringLiteral(":/hcb/tray-icon.png")); }

TrayStatus SystemTrayAdapter::status() const {
  if (!enabled_) {
    return {.state = TrayStatusState::Disabled,
            .visible = false,
            .supportsMessages = false,
            .message = QStringLiteral("Tray icon is disabled.")};
  }
  if (failed_ || icon_.isNull()) {
    return {.state = TrayStatusState::Error,
            .visible = false,
            .supportsMessages = false,
            .message = QStringLiteral("Tray icon could not be created.")};
  }
  const bool available = QSystemTrayIcon::isSystemTrayAvailable();
  return {.state = available ? TrayStatusState::Ready : TrayStatusState::Unsupported,
          .visible = available && tray_ != nullptr && tray_->isVisible(),
          .supportsMessages = available && QSystemTrayIcon::supportsMessages(),
          .message = available
                         ? QStringLiteral("Tray icon is available.")
                         : QStringLiteral("System tray is unavailable in this desktop session.")};
}

NotificationStatus SystemTrayAdapter::notificationStatus() const {
  if (!enabled_) {
    return {.state = NotificationState::Disabled,
            .supportsMessages = false,
            .message = QStringLiteral("Tray icon is disabled.")};
  }
  if (failed_ || icon_.isNull()) {
    return {.state = NotificationState::Error,
            .supportsMessages = false,
            .message = QStringLiteral("Tray icon could not be created.")};
  }
  if (!QSystemTrayIcon::isSystemTrayAvailable()) {
    return {.state = NotificationState::Unsupported,
            .supportsMessages = false,
            .message = QStringLiteral("System tray is unavailable in this desktop session.")};
  }
  if (!QSystemTrayIcon::supportsMessages()) {
    return {.state = NotificationState::Unsupported,
            .supportsMessages = false,
            .message = QStringLiteral("System tray does not support notification messages.")};
  }
  return {.state = NotificationState::Ready,
          .supportsMessages = true,
          .message = QStringLiteral("System tray notifications are available.")};
}

bool SystemTrayAdapter::showNotification(const NotificationRequest& request) {
  if (notificationStatus().state != NotificationState::Ready || tray_ == nullptr) {
    return false;
  }
  tray_->showMessage(
      request.title, request.body, messageIconFor(request.icon), request.timeoutMilliseconds);
  return true;
}

void SystemTrayAdapter::setEnabled(bool enabled) {
  enabled_ = enabled;
  failed_ = false;
  if (!enabled_) {
    if (tray_ != nullptr) {
      tray_->hide();
    }
    return;
  }
  if (icon_.isNull()) {
    failed_ = true;
    return;
  }
  try {
    if (tray_ == nullptr) {
      tray_ = std::make_unique<QSystemTrayIcon>(icon_, this);
      menu_ = std::make_unique<QMenu>();
      tray_->setToolTip(QStringLiteral("Hot Cross Buns"));
      QObject::connect(tray_.get(),
                       &QSystemTrayIcon::activated,
                       this,
                       [this](QSystemTrayIcon::ActivationReason reason) {
                         if (reason == QSystemTrayIcon::Trigger) {
#if defined(Q_OS_WIN)
                           static_cast<void>(dispatcher_.dispatch(TrayAction::ToggleMainWindow));
#else
                           static_cast<void>(dispatcher_.dispatch(TrayAction::OpenMainWindow));
#endif
                         }
                       });
    }
    rebuildMenu();
    tray_->show();
  } catch (...) {
    failed_ = true;
    if (tray_ != nullptr) {
      tray_->hide();
    }
  }
}

void SystemTrayAdapter::rebuildMenu() {
  if (menu_ == nullptr || tray_ == nullptr) {
    return;
  }
  menu_->clear();
  addAction(TrayAction::OpenMainWindow, QStringLiteral("Open Hot Cross Buns"));
  addAction(TrayAction::ToggleMainWindow, QStringLiteral("Show or Hide Window"));
  addAction(TrayAction::OpenQuickCapture, QStringLiteral("Quick Capture"));
  menu_->addSeparator();
  addAction(TrayAction::Refresh, QStringLiteral("Refresh Tasks and Calendar"));
  addAction(TrayAction::OpenSettings, QStringLiteral("Settings"));
  menu_->addSeparator();
  addAction(TrayAction::Quit, QStringLiteral("Quit"));
  tray_->setContextMenu(menu_.get());
}

void SystemTrayAdapter::addAction(TrayAction action, const QString& label) {
  if (menu_ == nullptr) {
    return;
  }
  QAction* menuAction = menu_->addAction(label);
  menuAction->setEnabled(dispatcher_.isAvailable(action));
  QObject::connect(menuAction, &QAction::triggered, this, [this, action] {
    static_cast<void>(dispatcher_.dispatch(action));
  });
}

} // namespace hcb
