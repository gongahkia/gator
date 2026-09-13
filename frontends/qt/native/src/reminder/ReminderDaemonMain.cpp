#include <QCoreApplication>
#include <QDBusConnection>
#include <QTextStream>

#include <utility>

#include "app/AppPaths.h"
#include "app/NativeReminderNotifier.h"
#include "core/Clock.h"
#include "core/ReminderService.h"

namespace {

constexpr char kReminderServiceName[] = "dev.hotcrossbuns.Reminders";
constexpr char kReminderObjectPath[] = "/dev/hotcrossbuns/Reminders";

class ReminderDaemon final : public QObject {
  Q_OBJECT
  Q_CLASSINFO("D-Bus Interface", "dev.hotcrossbuns.Reminders")

public:
  explicit ReminderDaemon(hcb::ReminderService& reminders, QObject* parent = nullptr)
      : QObject(parent), reminders_(reminders) {
    QObject::connect(&reminders_,
                     &hcb::ReminderService::statusMessageChanged,
                     this,
                     &ReminderDaemon::statusChanged);
  }

public slots:
  [[nodiscard]] QString Status() const { return reminders_.statusMessage(); }
  void Refresh() { reminders_.refresh(); }
  void Dismiss(QString identifier) { reminders_.dismiss(std::move(identifier)); }
  void Snooze(QString identifier, int minutes) {
    reminders_.snooze(std::move(identifier), minutes);
  }

signals:
  void statusChanged();

private:
  hcb::ReminderService& reminders_;
};

} // namespace

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  QCoreApplication::setOrganizationName(QStringLiteral("Hot Cross Buns"));
  QCoreApplication::setOrganizationDomain(QStringLiteral("gongahkia.github.io"));
  QCoreApplication::setApplicationName(QStringLiteral("Hot Cross Buns"));

  const std::optional<hcb::AppPaths> paths = hcb::AppPaths::discover();
  if (!paths.has_value()) {
    QTextStream(stderr) << "Hot Cross Buns reminder service could not find its data directory"
                        << Qt::endl;
    return 1;
  }
  const std::optional<hcb::FilePath> databasePath =
      paths->dataDirectory().child(QStringLiteral("hot-cross-buns.sqlite"));
  if (!databasePath.has_value()) {
    QTextStream(stderr) << "Hot Cross Buns reminder service could not resolve its database"
                        << Qt::endl;
    return 1;
  }

  hcb::SystemClock clock;
  hcb::NativeReminderNotifier notifier(&application);
  hcb::ReminderService reminders(*databasePath, clock, notifier, &application);
  ReminderDaemon daemon(reminders, &application);
  QDBusConnection connection = QDBusConnection::sessionBus();
  if (!connection.isConnected() ||
      !connection.registerService(QString::fromLatin1(kReminderServiceName)) ||
      !connection.registerObject(QString::fromLatin1(kReminderObjectPath),
                                 &daemon,
                                 QDBusConnection::ExportAllSlots |
                                     QDBusConnection::ExportAllSignals)) {
    QTextStream(stderr) << "Hot Cross Buns reminder service could not register on the session bus"
                        << Qt::endl;
    return 1;
  }
  reminders.start();
  return application.exec();
}

#include "ReminderDaemonMain.moc"
