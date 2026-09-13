#pragma once

#include "core/Clock.h"
#include "core/FilePath.h"

#include <QObject>
#include <QString>
#include <QTimer>

namespace hcb {

class NativeReminderNotifier;
enum class ReminderAction : unsigned char;

class ReminderService final : public QObject {
  Q_OBJECT

public:
  ReminderService(FilePath databasePath,
                  Clock& clock,
                  NativeReminderNotifier& notifier,
                  QObject* parent = nullptr);
  ~ReminderService() override;
  ReminderService(const ReminderService&) = delete;
  ReminderService& operator=(const ReminderService&) = delete;

  void start();
  void refresh();
  void dismiss(QString identifier);
  void snooze(QString identifier, int minutes = 10);
  [[nodiscard]] QString statusMessage() const;

signals:
  void statusMessageChanged();

private:
  void handleAction(QString identifier, ReminderAction action);
  void setStatusMessage(QString message);

  FilePath databasePath_;
  Clock& clock_;
  NativeReminderNotifier& notifier_;
  QTimer* refreshTimer_{nullptr};
  QString statusMessage_{QStringLiteral("Calendar reminders are initializing")};
};

} // namespace hcb
