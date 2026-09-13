#pragma once

#include <QString>

namespace hcb {

enum class NotificationIcon : unsigned char {
  Information,
  Warning,
  Critical,
};

enum class NotificationState : unsigned char {
  Ready,
  Disabled,
  Unsupported,
  Error,
};

struct NotificationRequest final {
  QString title;
  QString body;
  NotificationIcon icon{NotificationIcon::Information};
  int timeoutMilliseconds{10'000};
};

struct NotificationStatus final {
  NotificationState state;
  bool supportsMessages;
  QString message;
};

class NotificationTransport {
public:
  virtual ~NotificationTransport() = default;

  [[nodiscard]] virtual NotificationStatus notificationStatus() const = 0;
  [[nodiscard]] virtual bool showNotification(const NotificationRequest& request) = 0;
};

class NotificationAdapter final {
public:
  explicit NotificationAdapter(NotificationTransport& transport);

  [[nodiscard]] NotificationStatus status() const;
  [[nodiscard]] NotificationStatus send(const NotificationRequest& request) const;

private:
  NotificationTransport& transport_;
};

} // namespace hcb
