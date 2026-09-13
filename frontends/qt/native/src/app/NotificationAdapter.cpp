#include "app/NotificationAdapter.h"

namespace hcb {

NotificationAdapter::NotificationAdapter(NotificationTransport& transport)
    : transport_(transport) {}

NotificationStatus NotificationAdapter::status() const { return transport_.notificationStatus(); }

NotificationStatus NotificationAdapter::send(const NotificationRequest& request) const {
  const NotificationStatus current = status();
  if (current.state != NotificationState::Ready) {
    return current;
  }
  if (request.title.trimmed().isEmpty() || request.body.trimmed().isEmpty()) {
    return {.state = NotificationState::Error,
            .supportsMessages = current.supportsMessages,
            .message = QStringLiteral("Notification title and body are required.")};
  }
  if (request.timeoutMilliseconds <= 0) {
    return {.state = NotificationState::Error,
            .supportsMessages = current.supportsMessages,
            .message = QStringLiteral("Notification timeout must be positive.")};
  }
  if (!transport_.showNotification(request)) {
    return {.state = NotificationState::Error,
            .supportsMessages = current.supportsMessages,
            .message = QStringLiteral("Notification request could not be submitted.")};
  }
  return {.state = NotificationState::Ready,
          .supportsMessages = current.supportsMessages,
          .message = QStringLiteral("Notification request submitted.")};
}

} // namespace hcb
