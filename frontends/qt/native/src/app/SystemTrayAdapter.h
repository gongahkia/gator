#pragma once

#include "app/NotificationAdapter.h"

#include <QIcon>
#include <QObject>
#include <QString>

#include <functional>
#include <memory>

class QMenu;
class QSystemTrayIcon;

namespace hcb {

enum class TrayAction : unsigned char {
  OpenMainWindow,
  ToggleMainWindow,
  OpenQuickCapture,
  Refresh,
  OpenSettings,
  Quit,
};

struct TrayActionHandlers final {
  std::function<void()> openMainWindow;
  std::function<void()> toggleMainWindow;
  std::function<void()> openQuickCapture;
  std::function<void()> refresh;
  std::function<void()> openSettings;
  std::function<void()> quit;
};

class TrayActionDispatcher final {
public:
  explicit TrayActionDispatcher(TrayActionHandlers handlers);

  [[nodiscard]] bool isAvailable(TrayAction action) const;
  [[nodiscard]] bool dispatch(TrayAction action) const;

private:
  TrayActionHandlers handlers_;
};

enum class TrayStatusState : unsigned char {
  Ready,
  Disabled,
  Unsupported,
  Error,
};

struct TrayStatus final {
  TrayStatusState state;
  bool visible;
  bool supportsMessages;
  QString message;
};

class SystemTrayAdapter final : public QObject, public NotificationTransport {
  Q_OBJECT

public:
  explicit SystemTrayAdapter(QIcon icon, TrayActionHandlers handlers, QObject* parent = nullptr);
  ~SystemTrayAdapter() override;

  SystemTrayAdapter(const SystemTrayAdapter&) = delete;
  SystemTrayAdapter& operator=(const SystemTrayAdapter&) = delete;

  [[nodiscard]] static QIcon defaultIcon();
  [[nodiscard]] TrayStatus status() const;
  [[nodiscard]] NotificationStatus notificationStatus() const override;
  [[nodiscard]] bool showNotification(const NotificationRequest& request) override;
  void setEnabled(bool enabled);

private:
  void rebuildMenu();
  void addAction(TrayAction action, const QString& label);

  QIcon icon_;
  TrayActionDispatcher dispatcher_;
  std::unique_ptr<QSystemTrayIcon> tray_;
  std::unique_ptr<QMenu> menu_;
  bool enabled_{false};
  bool failed_{false};
};

} // namespace hcb
