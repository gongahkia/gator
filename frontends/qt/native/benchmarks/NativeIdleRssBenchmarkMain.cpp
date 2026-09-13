#include <QCommandLineOption>
#include <QCommandLineParser>
#include <QCoreApplication>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTextStream>

#include "core/NativeIdleRssBenchmark.h"

#include <chrono>

namespace {

constexpr int kDefaultIdleDurationMilliseconds = 1'000;
constexpr int kDefaultTimeoutMilliseconds = 15'000;
constexpr int kMaximumDurationMilliseconds = 60'000;

} // namespace

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  QCoreApplication::setApplicationName(QStringLiteral("hcb_idle_rss_benchmark"));

  QCommandLineParser parser;
  parser.addHelpOption();
  const QCommandLineOption executableOption(
      {QStringLiteral("e"), QStringLiteral("executable")},
      QStringLiteral("Native application executable to launch."),
      QStringLiteral("path"));
  const QCommandLineOption idleDurationOption(
      {QStringLiteral("idle-duration-ms")},
      QStringLiteral("Delay after QML load before sampling RSS (1-60000)."),
      QStringLiteral("milliseconds"),
      QString::number(kDefaultIdleDurationMilliseconds));
  const QCommandLineOption timeoutOption(
      {QStringLiteral("timeout-ms")},
      QStringLiteral("Process timeout in milliseconds (1-60000)."),
      QStringLiteral("milliseconds"),
      QString::number(kDefaultTimeoutMilliseconds));
  parser.addOption(executableOption);
  parser.addOption(idleDurationOption);
  parser.addOption(timeoutOption);
  parser.addPositionalArgument(QStringLiteral("arguments"),
                               QStringLiteral("Arguments for the native application."));
  parser.process(application);

  bool idleDurationValid = false;
  const int idleDuration = parser.value(idleDurationOption).toInt(&idleDurationValid);
  bool timeoutValid = false;
  const int timeoutMilliseconds = parser.value(timeoutOption).toInt(&timeoutValid);
  if (!parser.isSet(executableOption) || !idleDurationValid || idleDuration <= 0 ||
      idleDuration > kMaximumDurationMilliseconds || !timeoutValid || timeoutMilliseconds <= 0 ||
      timeoutMilliseconds > kMaximumDurationMilliseconds) {
    QTextStream(stderr) << "Invalid idle-RSS benchmark arguments.\n";
    return 2;
  }

  QString error;
  const auto residentBytes =
      hcb::NativeIdleRssBenchmark::measure(parser.value(executableOption),
                                           parser.positionalArguments(),
                                           std::chrono::milliseconds{idleDuration},
                                           std::chrono::milliseconds{timeoutMilliseconds},
                                           &error);
  if (!residentBytes.has_value()) {
    QTextStream(stderr) << error << '\n';
    return 1;
  }

  const QJsonObject output{{QStringLiteral("schema_version"), 1},
                           {QStringLiteral("idle_duration_ms"), idleDuration},
                           {QStringLiteral("idle_rss_bytes"), static_cast<qint64>(*residentBytes)}};
  QTextStream(stdout) << QJsonDocument(output).toJson(QJsonDocument::Compact) << '\n';
  return 0;
}
