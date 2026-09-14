#include "app/PythonBridgeProcessLauncher.h"

#include <QElapsedTimer>
#include <QFileInfo>
#include <QStandardPaths>
#include <QThread>

#include <utility>

namespace hcb {
namespace {

constexpr int kLaunchTimeoutMilliseconds = 5'000;
constexpr int kShutdownTimeoutMilliseconds = 2'000;

[[nodiscard]] AppError configurationError(QString message) {
  return AppError(AppErrorCode::Configuration, std::move(message));
}

} // namespace

PythonBridgeProcessLauncher::PythonBridgeProcessLauncher(QString executable)
    : executable_(std::move(executable)) {}

PythonBridgeProcessLauncher::~PythonBridgeProcessLauncher() { stop(); }

PythonBridgeLaunchResult PythonBridgeProcessLauncher::start() {
  if (process_.state() != QProcess::NotRunning) {
    return configurationError(QStringLiteral("HCB bridge process is already running"));
  }
  if (!directory_.isValid()) {
    return configurationError(QStringLiteral("Cannot create a private HCB bridge directory"));
  }
  if (executable_.isEmpty()) {
    executable_ = QStandardPaths::findExecutable(QStringLiteral("hcb"));
  }
  if (executable_.isEmpty() || !QFileInfo::exists(executable_)) {
    return configurationError(
        QStringLiteral("Cannot find the HCB CLI needed to launch the local bridge"));
  }
  descriptorPath_ = directory_.filePath(QStringLiteral("bridge.json"));
  process_.setProcessChannelMode(QProcess::SeparateChannels);
  process_.start(executable_,
                 {QStringLiteral("bridge"),
                  QStringLiteral("serve"),
                  QStringLiteral("--ready-file"),
                  descriptorPath_});
  if (!process_.waitForStarted(kLaunchTimeoutMilliseconds)) {
    stop();
    return configurationError(QStringLiteral("HCB bridge process did not start"));
  }

  QElapsedTimer elapsed;
  elapsed.start();
  while (elapsed.elapsed() < kLaunchTimeoutMilliseconds) {
    const PythonBridgeConnectionOrError connection =
        PythonBridgeClient::readConnectionDescriptor(descriptorPath_);
    if (std::holds_alternative<PythonBridgeConnection>(connection)) {
      return std::get<PythonBridgeConnection>(connection);
    }
    if (process_.state() == QProcess::NotRunning) {
      stop();
      return configurationError(QStringLiteral("HCB bridge process stopped before becoming ready"));
    }
    QThread::msleep(25);
  }
  stop();
  return configurationError(QStringLiteral("HCB bridge process did not become ready"));
}

QString PythonBridgeProcessLauncher::descriptorPath() const { return descriptorPath_; }

void PythonBridgeProcessLauncher::stop() {
  if (process_.state() == QProcess::NotRunning) {
    return;
  }
  process_.terminate();
  if (!process_.waitForFinished(kShutdownTimeoutMilliseconds)) {
    process_.kill();
    process_.waitForFinished(kShutdownTimeoutMilliseconds);
  }
}

} // namespace hcb
