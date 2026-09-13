#pragma once

#include <QObject>
#include <QDateTime>
#include <QString>

namespace hcb {

enum class ReminderAction : unsigned char {
  Dismiss,
  SnoozeTenMinutes
};

struct NativeReminderNotification final {
  QString identifier;
  QString title;
  QString body;
  QDateTime deliverAt;
};

class NativeReminderNotifier final : public QObject {
  Q_OBJECT

public:
  explicit NativeReminderNotifier(QObject* parent = nullptr);
  ~NativeReminderNotifier() override;
  NativeReminderNotifier(const NativeReminderNotifier&) = delete;
  NativeReminderNotifier& operator=(const NativeReminderNotifier&) = delete;

  void requestAuthorization();
  void schedule(NativeReminderNotification notification);
  void cancel(QString identifier);

  void handleAction(QString identifier, ReminderAction action);

signals:
  void statusChanged(QString message);
  void actionRequested(QString identifier, hcb::ReminderAction action);

private:
#if defined(Q_OS_LINUX)
private slots:
  void notificationActionInvoked(uint notificationId, QString action);
  void notificationClosed(uint notificationId, uint reason);
#endif

private:
  class NativeReminderNotifierPrivate;
  NativeReminderNotifierPrivate* state_{nullptr};
};

} // namespace hcb
