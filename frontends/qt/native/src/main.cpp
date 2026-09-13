#include <QApplication>
#include <QDateTime>
#include <QDir>
#if defined(Q_OS_LINUX)
#include <QDBusConnection>
#include <QDBusInterface>
#include <QDBusMessage>
#endif
#include <QElapsedTimer>
#include <QFileInfo>
#include <QQuickItem>
#include <QQuickWindow>
#include <QProcess>
#include <QQmlApplicationEngine>
#include <QTextStream>
#include <QTimer>
#include <QVariant>

#include <exception>
#include <functional>
#include <memory>
#include <optional>
#include <utility>

#include "app/AppPaths.h"
#include "app/AppController.h"
#include "app/DeepLinkAdapter.h"
#include "app/NativeReminderNotifier.h"
#include "app/SystemTrayAdapter.h"
#include "core/AgendaModel.h"
#include "core/CalendarSourceModel.h"
#include "core/Clock.h"
#include "core/CommandRegistryModel.h"
#include "core/MonthGridModel.h"
#include "core/NativeProcessMemory.h"
#include "core/NotesModel.h"
#include "core/ReminderService.h"
#include "core/SearchResultsModel.h"
#include "core/StartupTimingTracker.h"
#include "core/StructuredLogger.h"
#include "core/TaskListModel.h"
#include "core/TaskModel.h"
#include "core/TimelineModel.h"
#include "core/UiTransitionTimingTracker.h"

namespace {

constexpr int kMaximumBenchmarkIdleRssDurationMilliseconds = 60'000;
constexpr int kCommandPaletteBenchmarkTimeoutMilliseconds = 5'000;
constexpr int kMaximumTimelineProfileEventCount = 100'000;

#if defined(Q_OS_LINUX)
constexpr char kReminderServiceName[] = "dev.hotcrossbuns.Reminders";
constexpr char kReminderObjectPath[] = "/dev/hotcrossbuns/Reminders";
constexpr char kReminderServiceInterface[] = "dev.hotcrossbuns.Reminders";

class LinuxReminderDaemonClient final : public QObject {
public:
  using StatusHandler = std::function<void(QString)>;

  explicit LinuxReminderDaemonClient(StatusHandler statusHandler, QObject* parent = nullptr)
      : QObject(parent), statusHandler_(std::move(statusHandler)) {
    refreshTimer_.setInterval(30'000);
    QObject::connect(&refreshTimer_, &QTimer::timeout, this, &LinuxReminderDaemonClient::refresh);
  }

  void start() {
    static_cast<void>(QProcess::startDetached(QStringLiteral("systemctl"),
                                              {QStringLiteral("--user"),
                                               QStringLiteral("start"),
                                               QStringLiteral("hcb-reminderd.service")}));
    refreshTimer_.start();
    QTimer::singleShot(500, this, &LinuxReminderDaemonClient::refresh);
  }

private:
  void reportStatus(QString message) const {
    if (statusHandler_) {
      statusHandler_(std::move(message));
    }
  }

  void refresh() {
    QDBusInterface service(QString::fromLatin1(kReminderServiceName),
                           QString::fromLatin1(kReminderObjectPath),
                           QString::fromLatin1(kReminderServiceInterface),
                           QDBusConnection::sessionBus());
    if (!service.isValid()) {
      reportStatus(
          QStringLiteral("Calendar reminder service is unavailable; start hcb-reminderd.service"));
      return;
    }
    static_cast<void>(service.call(QStringLiteral("Refresh")));
    const QDBusMessage reply = service.call(QStringLiteral("Status"));
    if (reply.type() == QDBusMessage::ErrorMessage || reply.arguments().size() != 1) {
      reportStatus(QStringLiteral("Calendar reminder service could not report its status"));
      return;
    }
    reportStatus(reply.arguments().constFirst().toString());
  }

  StatusHandler statusHandler_;
  QTimer refreshTimer_;
};
#endif

[[nodiscard]] std::optional<int> timelineProfileEventCount(const QStringList& arguments) {
  constexpr QStringView prefix = u"--timeline-profile-events=";
  for (const QString& argument : arguments) {
    if (!argument.startsWith(prefix)) {
      continue;
    }
    bool valid = false;
    const int count = argument.sliced(prefix.size()).toInt(&valid);
    if (valid && count > 0 && count <= kMaximumTimelineProfileEventCount) {
      return count;
    }
    return std::nullopt;
  }
  return std::nullopt;
}

[[nodiscard]] QDate timelineProfileWeekStart(QDate date) {
  return date.addDays(-(date.dayOfWeek() % 7));
}

[[nodiscard]] QList<hcb::CalendarEventSummary>
timelineProfileEvents(int count, const QDate& weekStart, const QTimeZone& timeZone) {
  QList<hcb::CalendarEventSummary> events;
  events.reserve(count);
  for (int index = 0; index < count; ++index) {
    const int dayIndex = index % 7;
    const int startMinute = index / 7 % (24 * 60);
    const QDateTime startAt(weekStart.addDays(dayIndex),
                            QTime(startMinute / 60, startMinute % 60),
                            timeZone);
    events.append({.id = QStringLiteral("profile-event-%1").arg(index),
                   .calendarId = QStringLiteral("profile-calendar"),
                   .status = QStringLiteral("confirmed"),
                   .title = QStringLiteral("Profile event %1").arg(index),
                   .startAt = startAt.toUTC().toString(Qt::ISODateWithMs),
                   .endAt = startAt.addSecs(60).toUTC().toString(Qt::ISODateWithMs),
                   .allDay = false});
  }
  return events;
}

void scheduleCommandPaletteBenchmark(QApplication& application, QObject* rootObject) {
  QTimer::singleShot(0, &application, [&application, rootObject] {
    auto timer = std::make_shared<QElapsedTimer>();
    timer->start();
    if (!QMetaObject::invokeMethod(rootObject, "openCommandPalette")) {
      QCoreApplication::exit(3);
      return;
    }

    auto* pollTimer = new QTimer(&application);
    pollTimer->setInterval(1);
    QObject::connect(pollTimer, &QTimer::timeout, &application, [rootObject, timer, pollTimer] {
      const auto* query = rootObject->findChild<QQuickItem*>("commandPaletteQuery");
      if (query != nullptr && query->hasActiveFocus()) {
        QTextStream(stdout) << "HCB_COMMAND_PALETTE_OPEN_MS=" << timer->elapsed() << Qt::endl;
        pollTimer->stop();
        pollTimer->deleteLater();
        QCoreApplication::quit();
        return;
      }
      if (timer->elapsed() >= kCommandPaletteBenchmarkTimeoutMilliseconds) {
        pollTimer->stop();
        pollTimer->deleteLater();
        QCoreApplication::exit(3);
        return;
      }
    });
    pollTimer->start();
  });
}

void selectMainPage(QObject* rootObject, QString pageName) {
  static_cast<void>(QMetaObject::invokeMethod(rootObject,
                                              "selectPage",
                                              Qt::QueuedConnection,
                                              Q_ARG(QVariant, QVariant(std::move(pageName)))));
}

} // namespace

int runApplication(int argc, char* argv[]) {
  hcb::SystemClock clock;
  hcb::StructuredLogger logger(clock);
  hcb::StartupTimingTracker startupTimings(clock, logger);
  QApplication application(argc, argv);
  application.setWindowIcon(hcb::SystemTrayAdapter::defaultIcon());
  hcb::DeepLinkDispatcher deepLinks;
  hcb::DeepLinkFileOpenEventFilter deepLinkFileOpenEvents(deepLinks, &application);
  application.installEventFilter(&deepLinkFileOpenEvents);
  for (const hcb::DeepLink& link :
       hcb::DeepLinkAdapter::parseLaunchArguments(application.arguments())) {
    deepLinks.handle(link);
  }
  QCoreApplication::setOrganizationName("Hot Cross Buns");
  QCoreApplication::setOrganizationDomain("gongahkia.github.io");
  QCoreApplication::setApplicationName("Hot Cross Buns");
  startupTimings.mark(u"application.initialized");

  const std::optional<int> profileEventCount = timelineProfileEventCount(application.arguments());
  const bool timelineProfile = profileEventCount.has_value();
  std::optional<hcb::FilePath> databasePath;
  if (timelineProfile) {
    databasePath = hcb::FilePath::fromAbsolute(QStringLiteral("/dev/null"));
  } else {
    std::optional<hcb::AppPaths> paths = hcb::AppPaths::discover();
    if (!paths.has_value()) {
      startupTimings.mark(u"paths.unavailable");
      return 1;
    }
    startupTimings.mark(u"paths.discovered");
    databasePath = paths->dataDirectory().child(QStringLiteral("hot-cross-buns.sqlite"));
  }
  if (!databasePath.has_value()) {
    startupTimings.mark(u"database.path.unavailable");
    return 1;
  }
  if (!timelineProfile &&
      !QDir().mkpath(QFileInfo(databasePath->nativePath()).absolutePath())) {
    startupTimings.mark(u"database.directory.unavailable");
    return 1;
  }
  startupTimings.mark(u"core.services.initialized");
  hcb::AgendaModel agendaModel;
  hcb::CalendarSourceModel calendarSourceModel;
  hcb::CommandRegistryModel navigationCommands;
  hcb::MonthGridModel monthGridModel;
  hcb::NotesModel notesModel;
  hcb::TaskListModel taskListModel;
  hcb::TaskModel taskModel;
  hcb::TimelineModel timelineModel;
  hcb::UiTransitionTimingTracker transitionTimings(clock, logger);
  hcb::AppController appController(*databasePath,
                                   clock,
                                   agendaModel,
                                   calendarSourceModel,
                                   monthGridModel,
                                   notesModel,
                                   taskListModel,
                                   taskModel,
                                   timelineModel,
                                   &application);
#if defined(Q_OS_LINUX)
  LinuxReminderDaemonClient reminderClient(
      [&appController](QString status) {
        appController.setPlatformReminderStatus(std::move(status));
      },
      &application);
#else
  hcb::NativeReminderNotifier reminderNotifier(&application);
  hcb::ReminderService reminderService(*databasePath, clock, reminderNotifier, &application);
  appController.setReminderService(&reminderService);
#endif
  const QDate profileWeekStart = timelineProfileWeekStart(QDate::currentDate());
  if (timelineProfile) {
    timelineModel.setRange(profileWeekStart,
                           7,
                           timelineProfileEvents(*profileEventCount,
                                                 profileWeekStart,
                                                 QTimeZone::systemTimeZone()),
                           QTimeZone::systemTimeZone(),
                           2);
  }
  QQmlApplicationEngine engine;
  QVariantMap initialProperties{
      {QStringLiteral("agendaModel"), QVariant::fromValue(&agendaModel)},
      {QStringLiteral("calendarSourceModel"), QVariant::fromValue(&calendarSourceModel)},
      {QStringLiteral("navigationCommands"), QVariant::fromValue(&navigationCommands)},
      {QStringLiteral("monthGridModel"), QVariant::fromValue(&monthGridModel)},
      {QStringLiteral("notesModel"), QVariant::fromValue(&notesModel)},
      {QStringLiteral("searchResultsModel"), QVariant::fromValue(&appController.searchResultsModel())},
      {QStringLiteral("taskListModel"), QVariant::fromValue(&taskListModel)},
      {QStringLiteral("taskModel"), QVariant::fromValue(&taskModel)},
      {QStringLiteral("timelineModel"), QVariant::fromValue(&timelineModel)},
      {QStringLiteral("appController"), QVariant::fromValue(&appController)},
      {QStringLiteral("transitionTimings"), QVariant::fromValue(&transitionTimings)}};
  if (timelineProfile) {
    initialProperties.insert(QStringLiteral("timelineProfile"), true);
    initialProperties.insert(QStringLiteral("calendarDate"), profileWeekStart.toString(Qt::ISODate));
  }
  engine.setInitialProperties(initialProperties);

  QObject::connect(
      &engine,
      &QQmlApplicationEngine::objectCreationFailed,
      &application,
      [] { QCoreApplication::exit(1); },
      Qt::QueuedConnection);
  engine.loadFromModule("HCB", "Main");

  if (engine.rootObjects().isEmpty()) {
    startupTimings.mark(u"qml.load.failed");
    return 1;
  }
  startupTimings.mark(u"qml.loaded");

  const bool exitAfterLoad =
      qEnvironmentVariable("HCB_BENCHMARK_EXIT_AFTER_LOAD") == QStringLiteral("1");
  if (!timelineProfile && !exitAfterLoad) {
    appController.initialize();
#if defined(Q_OS_LINUX)
    reminderClient.start();
#else
    QTimer::singleShot(0, &reminderService, &hcb::ReminderService::start);
#endif
  }

  QObject* rootObject = engine.rootObjects().constFirst();
  auto showMainWindow = [rootObject] {
    auto* window = qobject_cast<QQuickWindow*>(rootObject);
    if (window == nullptr) {
      return;
    }
    window->showNormal();
    window->raise();
    window->requestActivate();
  };
  deepLinks.setHandler([rootObject, showMainWindow](const hcb::DeepLink& link) {
    showMainWindow();
    selectMainPage(rootObject, hcb::DeepLinkAdapter::pageName(link.destination));
  });
  hcb::TrayActionHandlers trayActions{
      .openMainWindow = showMainWindow,
      .toggleMainWindow =
          [rootObject, showMainWindow] {
            auto* window = qobject_cast<QQuickWindow*>(rootObject);
            if (window != nullptr && window->isVisible()) {
              window->hide();
              return;
            }
            showMainWindow();
          },
      .openQuickCapture =
          [rootObject, showMainWindow] {
            showMainWindow();
            static_cast<void>(
                QMetaObject::invokeMethod(rootObject, "openQuickCapture", Qt::QueuedConnection));
          },
      .refresh = {},
      .openSettings =
          [rootObject, showMainWindow] {
            showMainWindow();
            selectMainPage(rootObject, QStringLiteral("Settings"));
          },
      .quit = [&application] { application.quit(); }};
  hcb::SystemTrayAdapter tray(application.windowIcon(), std::move(trayActions), &application);
  tray.setEnabled(true);

  const bool benchmarkCommandPalette =
      qEnvironmentVariable("HCB_BENCHMARK_COMMAND_PALETTE_AFTER_LOAD") == QStringLiteral("1");
  bool idleRssDurationValid = false;
  const int idleRssDuration =
      qEnvironmentVariable("HCB_BENCHMARK_IDLE_RSS_AFTER_MS").toInt(&idleRssDurationValid);
  if (idleRssDurationValid && idleRssDuration > 0 &&
      idleRssDuration <= kMaximumBenchmarkIdleRssDurationMilliseconds) {
    startupTimings.mark(u"benchmark.idle_rss.scheduled");
    QTimer::singleShot(idleRssDuration, &application, [] {
      const auto residentBytes = hcb::NativeProcessMemory::residentBytes();
      if (residentBytes.has_value()) {
        QTextStream(stdout) << "HCB_IDLE_RSS_BYTES=" << *residentBytes << Qt::endl;
        QCoreApplication::quit();
        return;
      }
      QCoreApplication::exit(2);
    });
  } else if (benchmarkCommandPalette) {
    startupTimings.mark(u"benchmark.command_palette.scheduled");
    scheduleCommandPaletteBenchmark(application, rootObject);
  } else if (exitAfterLoad) {
    startupTimings.mark(u"benchmark.exit.scheduled");
    QTimer::singleShot(0, &application, &QCoreApplication::quit);
  }

  return application.exec();
}

int main(int argc, char* argv[]) {
  try {
    return runApplication(argc, argv);
  } catch (const std::exception& exception) {
    QTextStream(stderr) << "Fatal native startup exception: " << exception.what() << Qt::endl;
  } catch (...) {
    QTextStream(stderr) << "Fatal native startup exception" << Qt::endl;
  }
  return 1;
}
