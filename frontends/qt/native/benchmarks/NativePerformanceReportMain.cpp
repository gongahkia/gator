#include <QCommandLineOption>
#include <QCommandLineParser>
#include <QCoreApplication>
#include <QTextStream>

#include "core/NativeIdleRssBenchmark.h"
#include "core/NativeLaunchBenchmark.h"
#include "core/NativePerformanceReport.h"

#include <chrono>
#include <cstddef>
#include <limits>

namespace {

constexpr int kDefaultLaunchIterations = 3;
constexpr int kDefaultIdleDurationMilliseconds = 1'000;
constexpr int kDefaultTimeoutMilliseconds = 15'000;
constexpr int kMaximumLaunchIterations = 20;
constexpr int kMaximumDurationMilliseconds = 60'000;

} // namespace

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  QCoreApplication::setApplicationName(QStringLiteral("hcb_performance_report"));

  QCommandLineParser parser;
  parser.addHelpOption();
  const QCommandLineOption executableOption(
      {QStringLiteral("e"), QStringLiteral("executable")},
      QStringLiteral("Native application executable to benchmark."),
      QStringLiteral("path"));
  const QCommandLineOption launchIterationsOption(
      {QStringLiteral("launch-iterations")},
      QStringLiteral("Number of launch samples (1-20)."),
      QStringLiteral("count"),
      QString::number(kDefaultLaunchIterations));
  const QCommandLineOption idleDurationOption(
      {QStringLiteral("idle-duration-ms")},
      QStringLiteral("Delay after QML load before sampling RSS (1-60000)."),
      QStringLiteral("milliseconds"),
      QString::number(kDefaultIdleDurationMilliseconds));
  const QCommandLineOption timeoutOption(
      {QStringLiteral("timeout-ms")},
      QStringLiteral("Per-child-process timeout in milliseconds (1-60000)."),
      QStringLiteral("milliseconds"),
      QString::number(kDefaultTimeoutMilliseconds));
  parser.addOption(executableOption);
  parser.addOption(launchIterationsOption);
  parser.addOption(idleDurationOption);
  parser.addOption(timeoutOption);
  parser.addPositionalArgument(QStringLiteral("arguments"),
                               QStringLiteral("Arguments for the native application."));
  parser.process(application);

  bool launchIterationsValid = false;
  const int launchIterations = parser.value(launchIterationsOption).toInt(&launchIterationsValid);
  bool idleDurationValid = false;
  const int idleDuration = parser.value(idleDurationOption).toInt(&idleDurationValid);
  bool timeoutValid = false;
  const int timeoutMilliseconds = parser.value(timeoutOption).toInt(&timeoutValid);
  if (!parser.isSet(executableOption) || !launchIterationsValid || launchIterations <= 0 ||
      launchIterations > kMaximumLaunchIterations || !idleDurationValid || idleDuration <= 0 ||
      idleDuration > kMaximumDurationMilliseconds || !timeoutValid || timeoutMilliseconds <= 0 ||
      timeoutMilliseconds > kMaximumDurationMilliseconds) {
    QTextStream(stderr) << "Invalid performance report arguments.\n";
    return 2;
  }

  const QString executable = parser.value(executableOption);
  const QStringList arguments = parser.positionalArguments();
  const std::chrono::milliseconds timeout{timeoutMilliseconds};
  QString error;
  const auto launch = hcb::NativeLaunchBenchmark::measure(
      executable, arguments, static_cast<std::size_t>(launchIterations), timeout, &error);
  if (!launch.has_value()) {
    QTextStream(stderr) << error << '\n';
    return 1;
  }
  const auto idleRssBytes = hcb::NativeIdleRssBenchmark::measure(
      executable, arguments, std::chrono::milliseconds{idleDuration}, timeout, &error);
  if (!idleRssBytes.has_value() ||
      *idleRssBytes > static_cast<quint64>(std::numeric_limits<qint64>::max())) {
    QTextStream(stderr) << (error.isEmpty() ? QStringLiteral("Invalid idle-RSS result.") : error)
                        << '\n';
    return 1;
  }

  const hcb::NativePerformanceReport report{
      1,
      hcb::NativePerformanceReportSerializer::currentPlatform(),
      *launch,
      idleDuration,
      static_cast<qint64>(*idleRssBytes)};
  QTextStream(stdout) << hcb::NativePerformanceReportSerializer::toJson(report) << '\n';
  return 0;
}
