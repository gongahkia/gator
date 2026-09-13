#include "app/NativeReminderNotifier.h"

#if defined(Q_OS_LINUX)
#include <QDBusConnection>
#include <QDBusInterface>
#include <QDBusMessage>
#include <QHash>
#include <QPointer>
#include <QTimer>

#include <algorithm>
#include <limits>
#include <utility>

namespace {

constexpr char kNotificationService[] = "org.freedesktop.Notifications";
constexpr char kNotificationPath[] = "/org/freedesktop/Notifications";
constexpr char kNotificationInterface[] = "org.freedesktop.Notifications";

[[nodiscard]] QDBusInterface notificationInterface() {
  return QDBusInterface(QString::fromLatin1(kNotificationService),
                        QString::fromLatin1(kNotificationPath),
                        QString::fromLatin1(kNotificationInterface),
                        QDBusConnection::sessionBus());
}

} // namespace

namespace hcb {

class NativeReminderNotifier::NativeReminderNotifierPrivate final {
public:
  QHash<QString, NativeReminderNotification> pending;
  QHash<QString, QPointer<QTimer>> timers;
  QHash<uint, QString> notificationIdentifiers;
};

NativeReminderNotifier::NativeReminderNotifier(QObject* parent) : QObject(parent) {
  state_ = new NativeReminderNotifierPrivate;
  QDBusConnection connection = QDBusConnection::sessionBus();
  static_cast<void>(connection.connect(QString::fromLatin1(kNotificationService),
                                       QString::fromLatin1(kNotificationPath),
                                       QString::fromLatin1(kNotificationInterface),
                                       QStringLiteral("ActionInvoked"),
                                       this,
                                       SLOT(notificationActionInvoked(uint, QString))));
  static_cast<void>(connection.connect(QString::fromLatin1(kNotificationService),
                                       QString::fromLatin1(kNotificationPath),
                                       QString::fromLatin1(kNotificationInterface),
                                       QStringLiteral("NotificationClosed"),
                                       this,
                                       SLOT(notificationClosed(uint, uint))));
}

NativeReminderNotifier::~NativeReminderNotifier() {
  delete state_;
  state_ = nullptr;
}

void NativeReminderNotifier::requestAuthorization() {
  QDBusInterface notifications = notificationInterface();
  if (!notifications.isValid()) {
    emit statusChanged(
        QStringLiteral("Calendar reminders require an active desktop notification service"));
    return;
  }
  const QDBusMessage reply = notifications.call(QStringLiteral("GetCapabilities"));
  if (reply.type() == QDBusMessage::ErrorMessage || reply.arguments().size() != 1) {
    emit statusChanged(QStringLiteral("Calendar reminder capabilities could not be read"));
    return;
  }
  const QStringList capabilities = reply.arguments().constFirst().toStringList();
  if (!capabilities.contains(QStringLiteral("body")) ||
      !capabilities.contains(QStringLiteral("actions"))) {
    emit statusChanged(
        QStringLiteral("Calendar reminders require notification body and action support"));
    return;
  }
  emit statusChanged(QStringLiteral("Calendar reminders are enabled"));
}

void NativeReminderNotifier::schedule(NativeReminderNotification notification) {
  if (state_ == nullptr || notification.identifier.isEmpty() ||
      notification.title.trimmed().isEmpty() || notification.body.trimmed().isEmpty() ||
      !notification.deliverAt.isValid()) {
    return;
  }
  const QString identifier = notification.identifier;
  QPointer<QTimer>& timer = state_->timers[identifier];
  if (timer == nullptr) {
    timer = new QTimer(this);
    timer->setSingleShot(true);
    QObject::connect(timer, &QTimer::timeout, this, [this, identifier] {
      if (state_ == nullptr || !state_->pending.contains(identifier)) {
        return;
      }
      const NativeReminderNotification pendingNotification = state_->pending.value(identifier);
      const qint64 remainingMilliseconds =
          QDateTime::currentDateTimeUtc().msecsTo(pendingNotification.deliverAt.toUTC());
      if (remainingMilliseconds > 0) {
        schedule(pendingNotification);
        return;
      }
      state_->pending.remove(identifier);
      QDBusInterface notifications = notificationInterface();
      if (!notifications.isValid()) {
        emit statusChanged(
            QStringLiteral("Calendar reminder could not reach the desktop notification service"));
        return;
      }
      QVariantMap hints;
      hints.insert(QStringLiteral("desktop-entry"), QStringLiteral("hot-cross-buns"));
      hints.insert(QStringLiteral("category"), QStringLiteral("calendar"));
      QVariantList arguments;
      arguments << QStringLiteral("Hot Cross Buns") << static_cast<uint>(0)
                << QStringLiteral("hot-cross-buns") << pendingNotification.title
                << pendingNotification.body
                << QStringList{QStringLiteral("dismiss"),
                               QStringLiteral("Dismiss"),
                               QStringLiteral("snooze"),
                               QStringLiteral("Snooze 10 minutes")}
                << hints << 0;
      const QDBusMessage reply =
          notifications.callWithArgumentList(QDBus::Block, QStringLiteral("Notify"), arguments);
      if (reply.type() == QDBusMessage::ErrorMessage || reply.arguments().size() != 1) {
        emit statusChanged(QStringLiteral("Calendar reminder could not be delivered"));
        return;
      }
      state_->notificationIdentifiers.insert(reply.arguments().constFirst().toUInt(), identifier);
    });
  }
  timer->stop();
  state_->pending.insert(identifier, std::move(notification));
  const qint64 remainingMilliseconds =
      QDateTime::currentDateTimeUtc().msecsTo(state_->pending.value(identifier).deliverAt.toUTC());
  const qint64 boundedDelay = std::clamp<qint64>(
      remainingMilliseconds, 0, static_cast<qint64>(std::numeric_limits<int>::max()));
  timer->start(static_cast<int>(boundedDelay));
}

void NativeReminderNotifier::cancel(QString identifier) {
  if (state_ == nullptr || identifier.isEmpty()) {
    return;
  }
  state_->pending.remove(identifier);
  if (QPointer<QTimer> timer = state_->timers.take(identifier); timer != nullptr) {
    timer->stop();
    timer->deleteLater();
  }
  QDBusInterface notifications = notificationInterface();
  for (auto iterator = state_->notificationIdentifiers.begin();
       iterator != state_->notificationIdentifiers.end();) {
    if (iterator.value() != identifier) {
      ++iterator;
      continue;
    }
    if (notifications.isValid()) {
      static_cast<void>(notifications.call(QStringLiteral("CloseNotification"), iterator.key()));
    }
    iterator = state_->notificationIdentifiers.erase(iterator);
  }
}

void NativeReminderNotifier::notificationActionInvoked(uint notificationId, QString action) {
  if (state_ == nullptr || !state_->notificationIdentifiers.contains(notificationId)) {
    return;
  }
  const QString identifier = state_->notificationIdentifiers.take(notificationId);
  if (action == QStringLiteral("snooze")) {
    emit actionRequested(identifier, ReminderAction::SnoozeTenMinutes);
  } else if (action == QStringLiteral("dismiss")) {
    emit actionRequested(identifier, ReminderAction::Dismiss);
  }
}

void NativeReminderNotifier::notificationClosed(uint notificationId, uint reason) {
  Q_UNUSED(reason);
  if (state_ != nullptr) {
    state_->notificationIdentifiers.remove(notificationId);
  }
}

} // namespace hcb

#else

namespace hcb {

NativeReminderNotifier::NativeReminderNotifier(QObject* parent) : QObject(parent) {}
NativeReminderNotifier::~NativeReminderNotifier() = default;

void NativeReminderNotifier::requestAuthorization() {
  emit statusChanged(QStringLiteral("Native calendar reminders are unavailable on this platform"));
}

void NativeReminderNotifier::schedule(NativeReminderNotification notification) {
  Q_UNUSED(notification);
}
void NativeReminderNotifier::cancel(QString identifier) { Q_UNUSED(identifier); }
void NativeReminderNotifier::handleAction(QString identifier, ReminderAction action) {
  emit actionRequested(std::move(identifier), action);
}

} // namespace hcb

#endif
