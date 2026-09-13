#include "core/NativeCalendarNavigationBenchmark.h"

#include "core/MonthGridModel.h"
#include "core/NativePerformanceFixture.h"

#include <QElapsedTimer>
#include <QEventLoop>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QQmlComponent>
#include <QQmlContext>
#include <QQuickView>
#include <QQuickWindow>
#include <QTimeZone>
#include <QTimer>

#include <algorithm>
#include <optional>
#include <utility>

namespace hcb {
namespace {

constexpr std::size_t kMaximumFrameCount = 60;
constexpr int kViewportWidth = 700;
constexpr int kViewportHeight = 480;
constexpr int kFrameTimeoutMilliseconds = 5'000;

[[nodiscard]] QDate initialMonth() { return QDate(2026, 1, 1); }

class FrameWaiter final {
public:
  explicit FrameWaiter(QQuickWindow& window)
      : connection_(QObject::connect(&window, &QQuickWindow::afterFrameEnd, &eventLoop_, [this] {
          received_ = true;
          eventLoop_.quit();
        })) {}

  FrameWaiter(const FrameWaiter&) = delete;
  FrameWaiter& operator=(const FrameWaiter&) = delete;

  [[nodiscard]] bool wait() {
    received_ = false;
    bool timedOut = false;
    QTimer timeout;
    timeout.setSingleShot(true);
    QObject::connect(&timeout, &QTimer::timeout, &eventLoop_, [&] {
      timedOut = true;
      eventLoop_.quit();
    });
    timeout.start(kFrameTimeoutMilliseconds);
    if (!received_) {
      eventLoop_.exec();
    }
    timeout.stop();
    return received_ && !timedOut;
  }

private:
  QEventLoop eventLoop_;
  QMetaObject::Connection connection_;
  bool received_{false};
};

[[nodiscard]] QList<CalendarEventSummary> createEvents(const NativePerformanceFixture& fixture) {
  QList<CalendarEventSummary> events;
  events.reserve(static_cast<qsizetype>(fixture.eventInstances.size()));
  for (const NativePerformanceEventFixture& event : fixture.eventInstances) {
    events.append({.id = event.id,
                   .calendarId = event.calendarId,
                   .status = QStringLiteral("confirmed"),
                   .title = event.title,
                   .startAt = event.startsAt,
                   .endAt = event.endsAt,
                   .allDay = event.isAllDay,
                   .updatedAt = event.updatedAt});
  }
  return events;
}

} // namespace

std::optional<NativeCalendarNavigationBenchmarkResult>
NativeCalendarNavigationBenchmark::run(std::size_t frameCount) {
  if (frameCount == 0 || frameCount > kMaximumFrameCount) {
    return std::nullopt;
  }
  const QTimeZone timeZone = QTimeZone::utc();
  const QList<CalendarEventSummary> events =
      createEvents(NativePerformanceFixtureGenerator::wrapperScale());
  if (events.empty()) {
    return std::nullopt;
  }
  MonthGridModel model;
  model.setMonth(initialMonth(), events, timeZone);
  QQuickView view;
  view.resize(kViewportWidth, kViewportHeight);
  view.rootContext()->setContextProperty(QStringLiteral("monthGrid"), &model);
  QQmlComponent component(view.engine());
  component.setData(R"(
import QtQuick

Item {
  width: 700
  height: 480

  TableView {
    objectName: "monthGridView"
    anchors.fill: parent
    clip: true
    columnSpacing: 1
    rowSpacing: 1
    model: monthGrid
    columnWidthProvider: function() { return 99 }
    rowHeightProvider: function() { return 79 }

    delegate: Rectangle {
      required property int day
      required property bool outsideMonth
      required property int eventCount
      implicitWidth: 99
      implicitHeight: 79
      color: outsideMonth ? "#eeeeee" : "#ffffff"

      Text {
        anchors.left: parent.left
        anchors.leftMargin: 8
        anchors.top: parent.top
        anchors.topMargin: 8
        text: day + "  " + eventCount
      }
    }
  }
}
)",
                    QUrl(QStringLiteral("qrc:/NativeCalendarNavigationBenchmark.qml")));
  if (!component.isReady()) {
    return std::nullopt;
  }
  QObject* const root = component.create(view.rootContext());
  if (root == nullptr) {
    return std::nullopt;
  }
  view.setContent(component.url(), &component, root);
  FrameWaiter waiter(view);
  view.show();
  view.update();
  if (!waiter.wait()) {
    return std::nullopt;
  }
  std::vector<qint64> samples;
  samples.reserve(frameCount);
  for (std::size_t frame = 0; frame < frameCount; ++frame) {
    QElapsedTimer timer;
    timer.start();
    model.setMonth(initialMonth().addMonths(static_cast<int>(frame) + 1), events, timeZone);
    view.update();
    if (!waiter.wait()) {
      return std::nullopt;
    }
    samples.push_back(timer.nsecsElapsed());
  }
  return summarize(static_cast<std::size_t>(events.size()), std::move(samples));
}

std::optional<NativeCalendarNavigationBenchmarkResult>
NativeCalendarNavigationBenchmark::summarize(std::size_t eventCount,
                                             std::vector<qint64> samplesNanoseconds) {
  if (eventCount == 0 || samplesNanoseconds.empty() ||
      std::any_of(samplesNanoseconds.cbegin(), samplesNanoseconds.cend(), [](qint64 sample) {
        return sample < 0;
      })) {
    return std::nullopt;
  }
  std::sort(samplesNanoseconds.begin(), samplesNanoseconds.end());
  const qint64 minimum = samplesNanoseconds.front();
  const qint64 median = samplesNanoseconds[samplesNanoseconds.size() / 2];
  const qint64 maximum = samplesNanoseconds.back();
  return NativeCalendarNavigationBenchmarkResult{.eventCount = eventCount,
                                                 .samplesNanoseconds =
                                                     std::move(samplesNanoseconds),
                                                 .minimumNanoseconds = minimum,
                                                 .medianNanoseconds = median,
                                                 .maximumNanoseconds = maximum};
}

QByteArray
NativeCalendarNavigationBenchmark::toJson(const NativeCalendarNavigationBenchmarkResult& result) {
  QJsonArray samples;
  for (const qint64 sample : result.samplesNanoseconds) {
    samples.append(sample);
  }
  return QJsonDocument(
             QJsonObject{
                 {QStringLiteral("schema_version"), 1},
                 {QStringLiteral("event_count"), static_cast<qint64>(result.eventCount)},
                 {QStringLiteral("frames"), static_cast<qint64>(result.samplesNanoseconds.size())},
                 {QStringLiteral("minimum_ns"), result.minimumNanoseconds},
                 {QStringLiteral("median_ns"), result.medianNanoseconds},
                 {QStringLiteral("maximum_ns"), result.maximumNanoseconds},
                 {QStringLiteral("samples_ns"), samples}})
      .toJson(QJsonDocument::Compact);
}

} // namespace hcb
