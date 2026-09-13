#include <QCommandLineOption>
#include <QCommandLineParser>
#include <QCoreApplication>
#include <QTextStream>

#include "core/NativeLaunchBenchmark.h"

#include <chrono>
#include <cstddef>

namespace {

constexpr int kDefaultIterations = 3;
constexpr int kDefaultTimeoutMilliseconds = 15'000;
constexpr int kMaximumIterations = 20;
constexpr int kMaximumTimeoutMilliseconds = 60'000;

} // namespace

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  QCoreApplication::setApplicationName(QStringLiteral("hcb_launch_benchmark"));

  QCommandLineParser parser;
  parser.addHelpOption();
  const QCommandLineOption executableOption(
      {QStringLiteral("e"), QStringLiteral("executable")},
      QStringLiteral("Native application executable to launch."),
      QStringLiteral("path"));
  const QCommandLineOption iterationsOption({QStringLiteral("i"), QStringLiteral("iterations")},
                                            QStringLiteral("Number of measured launches (1-20)."),
                                            QStringLiteral("count"),
                                            QString::number(kDefaultIterations));
  const QCommandLineOption timeoutOption(
      {QStringLiteral("timeout-ms")},
      QStringLiteral("Per-launch timeout in milliseconds (1-60000)."),
      QStringLiteral("milliseconds"),
      QString::number(kDefaultTimeoutMilliseconds));
  parser.addOption(executableOption);
  parser.addOption(iterationsOption);
  parser.addOption(timeoutOption);
  parser.addPositionalArgument(QStringLiteral("arguments"),
                               QStringLiteral("Arguments for the native application."));
  parser.process(application);

  bool iterationsValid = false;
  const int iterations = parser.value(iterationsOption).toInt(&iterationsValid);
  bool timeoutValid = false;
  const int timeoutMilliseconds = parser.value(timeoutOption).toInt(&timeoutValid);
  if (!parser.isSet(executableOption) || !iterationsValid || iterations <= 0 ||
      iterations > kMaximumIterations || !timeoutValid || timeoutMilliseconds <= 0 ||
      timeoutMilliseconds > kMaximumTimeoutMilliseconds) {
    QTextStream(stderr) << "Invalid launch benchmark arguments.\n";
    return 2;
  }

  QString error;
  const auto result =
      hcb::NativeLaunchBenchmark::measure(parser.value(executableOption),
                                          parser.positionalArguments(),
                                          static_cast<std::size_t>(iterations),
                                          std::chrono::milliseconds{timeoutMilliseconds},
                                          &error);
  if (!result.has_value()) {
    QTextStream(stderr) << error << '\n';
    return 1;
  }

  QTextStream(stdout) << hcb::NativeLaunchBenchmark::toJson(*result) << '\n';
  return 0;
}
