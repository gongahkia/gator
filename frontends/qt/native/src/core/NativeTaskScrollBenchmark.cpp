#include "core/NativeTaskScrollBenchmark.h"

#include "core/NativePerformanceFixture.h"
#include "core/TaskModel.h"

#include <QElapsedTimer>
#include <QEventLoop>
#include <QFontDatabase>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QQmlComponent>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickView>
#include <QQuickWindow>
#include <QTimer>

#include <algorithm>
#include <chrono>
#include <optional>
#include <utility>

namespace hcb {
namespace {

constexpr std::size_t kMaximumFrameCount = 60;
constexpr int kViewportWidth = 360;
constexpr int kViewportHeight = 640;
constexpr int kFrameTimeoutMilliseconds = 5'000;

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

[[nodiscard]] QList<TaskModelTask> createTasks() {
  const NativePerformanceFixture fixture = NativePerformanceFixtureGenerator::wrapperScale();
  QList<TaskModelTask> tasks;
  tasks.reserve(static_cast<qsizetype>(fixture.tasks.size()));
  for (const NativePerformanceTaskFixture& task : fixture.tasks) {
    tasks.append({.id = task.id,
                  .taskListId = task.taskListId,
                  .title = task.title,
                  .notes = QStringLiteral("Virtualized task-scroll benchmark"),
                  .completed = task.status == QStringLiteral("completed"),
                  .sortOrder = static_cast<std::int64_t>(task.sortOrder)});
  }
  return tasks;
}

} // namespace

std::optional<NativeTaskScrollBenchmarkResult>
NativeTaskScrollBenchmark::run(std::size_t frameCount) {
  if (frameCount == 0 || frameCount > kMaximumFrameCount) {
    return std::nullopt;
  }
  const QList<TaskModelTask> tasks = createTasks();
  if (tasks.empty()) {
    return std::nullopt;
  }
  TaskModel model;
  const QString fontFamily = QFontDatabase::systemFont(QFontDatabase::GeneralFont).family();
  QQuickView view;
  view.resize(kViewportWidth, kViewportHeight);
  view.rootContext()->setContextProperty(QStringLiteral("benchmarkFontFamily"), fontFamily);
  QQmlComponent component(view.engine());
  component.setData(R"(
import QtQuick

Item {
  id: root
  width: 360
  height: 640
  property var benchmarkTaskModel: null

  Text {
    text: "Warm"
    font.family: benchmarkFontFamily
  }

  ListView {
    objectName: "taskScrollView"
    anchors.fill: parent
    clip: true
    cacheBuffer: 0
    model: root.benchmarkTaskModel

    delegate: Rectangle {
      required property string title
      required property string notes
      required property bool completed
      width: ListView.view.width
      height: 48
      color: completed ? "#eeeeee" : "#ffffff"

      Text {
        anchors.left: parent.left
        anchors.leftMargin: 12
        anchors.verticalCenter: parent.verticalCenter
        text: title + " — " + notes
        font.family: benchmarkFontFamily
      }
    }
  }
}
)",
                    QUrl(QStringLiteral("qrc:/NativeTaskScrollBenchmark.qml")));
  if (!component.isReady()) {
    return std::nullopt;
  }
  QObject* const root = component.create(view.rootContext());
  if (root == nullptr) {
    return std::nullopt;
  }
  view.setContent(component.url(), &component, root);
  QQuickItem* const listView = root->findChild<QQuickItem*>(QStringLiteral("taskScrollView"));
  if (listView == nullptr) {
    return std::nullopt;
  }
  FrameWaiter waiter(view);
  view.show();
  view.update();
  if (!waiter.wait()) {
    return std::nullopt;
  }
  QElapsedTimer firstCachedRenderTimer;
  firstCachedRenderTimer.start();
  model.setTasks(tasks);
  root->setProperty("benchmarkTaskModel", QVariant::fromValue(&model));
  view.update();
  if (!waiter.wait()) {
    return std::nullopt;
  }
  const qint64 firstCachedRenderNanoseconds = firstCachedRenderTimer.nsecsElapsed();
  const qreal maximumContentY =
      listView->property("contentHeight").toReal() - listView->property("height").toReal();
  if (maximumContentY <= 0.0) {
    return std::nullopt;
  }
  std::vector<qint64> samples;
  samples.reserve(frameCount);
  for (std::size_t frame = 0; frame < frameCount; ++frame) {
    const qreal progress = static_cast<qreal>(frame + 1) / static_cast<qreal>(frameCount);
    QElapsedTimer timer;
    timer.start();
    listView->setProperty("contentY", maximumContentY * progress);
    view.update();
    if (!waiter.wait()) {
      return std::nullopt;
    }
    samples.push_back(timer.nsecsElapsed());
  }
  std::vector<qint64> bulkSelectionSamples;
  bulkSelectionSamples.reserve(frameCount);
  for (std::size_t iteration = 0; iteration < frameCount; ++iteration) {
    QElapsedTimer timer;
    timer.start();
    const QVariantList selectedTaskIds = model.taskIds();
    if (selectedTaskIds.size() != tasks.size()) {
      return std::nullopt;
    }
    bulkSelectionSamples.push_back(timer.nsecsElapsed());
  }
  return summarize(static_cast<std::size_t>(tasks.size()),
                   std::move(bulkSelectionSamples),
                   firstCachedRenderNanoseconds,
                   std::move(samples));
}

std::optional<NativeTaskScrollBenchmarkResult>
NativeTaskScrollBenchmark::summarize(std::size_t taskCount,
                                     std::vector<qint64> bulkSelectionSamplesNanoseconds,
                                     qint64 firstCachedRenderNanoseconds,
                                     std::vector<qint64> samplesNanoseconds) {
  if (taskCount == 0 || bulkSelectionSamplesNanoseconds.empty() ||
      firstCachedRenderNanoseconds < 0 || samplesNanoseconds.empty() ||
      std::any_of(
          bulkSelectionSamplesNanoseconds.cbegin(),
          bulkSelectionSamplesNanoseconds.cend(),
          [](qint64 sample) { return sample < 0; }) ||
      std::any_of(
          samplesNanoseconds.cbegin(),
          samplesNanoseconds.cend(),
          [](qint64 sample) { return sample < 0; })) {
    return std::nullopt;
  }
  std::sort(bulkSelectionSamplesNanoseconds.begin(), bulkSelectionSamplesNanoseconds.end());
  std::sort(samplesNanoseconds.begin(), samplesNanoseconds.end());
  const qint64 bulkSelectionMedian =
      bulkSelectionSamplesNanoseconds[bulkSelectionSamplesNanoseconds.size() / 2];
  const qint64 minimum = samplesNanoseconds.front();
  const qint64 median = samplesNanoseconds[samplesNanoseconds.size() / 2];
  const qint64 maximum = samplesNanoseconds.back();
  return NativeTaskScrollBenchmarkResult{
      .taskCount = taskCount,
      .bulkSelectionSamplesNanoseconds = std::move(bulkSelectionSamplesNanoseconds),
      .bulkSelectionMedianNanoseconds = bulkSelectionMedian,
      .firstCachedRenderNanoseconds = firstCachedRenderNanoseconds,
      .samplesNanoseconds = std::move(samplesNanoseconds),
      .minimumNanoseconds = minimum,
      .medianNanoseconds = median,
      .maximumNanoseconds = maximum};
}

QByteArray NativeTaskScrollBenchmark::toJson(const NativeTaskScrollBenchmarkResult& result) {
  QJsonArray bulkSelectionSamples;
  for (const qint64 sample : result.bulkSelectionSamplesNanoseconds) {
    bulkSelectionSamples.append(sample);
  }
  QJsonArray samples;
  for (const qint64 sample : result.samplesNanoseconds) {
    samples.append(sample);
  }
  return QJsonDocument(
             QJsonObject{
                 {QStringLiteral("schema_version"), 1},
                 {QStringLiteral("task_count"), static_cast<qint64>(result.taskCount)},
                 {QStringLiteral("first_cached_render_ns"), result.firstCachedRenderNanoseconds},
                 {QStringLiteral("bulk_selection"),
                  QJsonObject{{QStringLiteral("iterations"),
                               static_cast<qint64>(result.bulkSelectionSamplesNanoseconds.size())},
                              {QStringLiteral("median_ns"), result.bulkSelectionMedianNanoseconds},
                              {QStringLiteral("samples_ns"), bulkSelectionSamples}}},
                 {QStringLiteral("frames"), static_cast<qint64>(result.samplesNanoseconds.size())},
                 {QStringLiteral("minimum_ns"), result.minimumNanoseconds},
                 {QStringLiteral("median_ns"), result.medianNanoseconds},
                 {QStringLiteral("maximum_ns"), result.maximumNanoseconds},
                 {QStringLiteral("samples_ns"), samples}})
      .toJson(QJsonDocument::Compact);
}

} // namespace hcb
