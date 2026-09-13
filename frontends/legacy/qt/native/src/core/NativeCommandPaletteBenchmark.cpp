#include "core/NativeCommandPaletteBenchmark.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QProcess>
#include <QProcessEnvironment>
#include <QRegularExpression>

#include <algorithm>
#include <limits>
#include <utility>

namespace hcb {
namespace {

constexpr std::size_t kMaximumIterations = 20;

void setError(QString* error, QString message) {
  if (error != nullptr) {
    *error = std::move(message);
  }
}

} // namespace

std::optional<NativeCommandPaletteBenchmarkResult>
NativeCommandPaletteBenchmark::measure(const QString& executable,
                                       const QStringList& arguments,
                                       std::size_t iterations,
                                       std::chrono::milliseconds timeout,
                                       QString* error) {
  if (executable.isEmpty()) {
    setError(error, QStringLiteral("command-palette executable is required"));
    return std::nullopt;
  }
  if (iterations == 0 || iterations > kMaximumIterations) {
    setError(error,
             QStringLiteral("command-palette iteration count is outside the supported range"));
    return std::nullopt;
  }
  if (timeout.count() <= 0 || timeout.count() > std::numeric_limits<int>::max()) {
    setError(error, QStringLiteral("command-palette timeout is outside the supported range"));
    return std::nullopt;
  }

  std::vector<qint64> samples;
  samples.reserve(iterations);
  const int timeoutMilliseconds = static_cast<int>(timeout.count());
  for (std::size_t iteration = 0; iteration < iterations; ++iteration) {
    QProcess process;
    QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
    environment.insert(QStringLiteral("HCB_BENCHMARK_COMMAND_PALETTE_AFTER_LOAD"),
                       QStringLiteral("1"));
    process.setProcessEnvironment(environment);
    process.setProgram(executable);
    process.setArguments(arguments);
    process.start();
    if (!process.waitForStarted(timeoutMilliseconds)) {
      setError(
          error,
          QStringLiteral("command-palette process did not start: %1").arg(process.errorString()));
      return std::nullopt;
    }
    if (!process.waitForFinished(timeoutMilliseconds)) {
      process.kill();
      process.waitForFinished(1'000);
      setError(error, QStringLiteral("command-palette process exceeded the timeout"));
      return std::nullopt;
    }
    if (process.exitStatus() != QProcess::NormalExit || process.exitCode() != 0) {
      setError(error,
               QStringLiteral("command-palette process failed with exit code %1: %2")
                   .arg(process.exitCode())
                   .arg(QString::fromUtf8(process.readAllStandardError())));
      return std::nullopt;
    }
    const auto elapsedMilliseconds = parseElapsedMilliseconds(process.readAllStandardOutput());
    if (!elapsedMilliseconds.has_value()) {
      setError(error,
               QStringLiteral("command-palette process did not emit a valid elapsed duration"));
      return std::nullopt;
    }
    samples.push_back(*elapsedMilliseconds);
  }
  return summarize(std::move(samples));
}

std::optional<NativeCommandPaletteBenchmarkResult>
NativeCommandPaletteBenchmark::summarize(std::vector<qint64> samplesMilliseconds) {
  if (samplesMilliseconds.empty()) {
    return std::nullopt;
  }
  std::sort(samplesMilliseconds.begin(), samplesMilliseconds.end());
  const qint64 minimum = samplesMilliseconds.front();
  const std::size_t medianIndex = samplesMilliseconds.size() / 2;
  const qint64 median = samplesMilliseconds[medianIndex];
  const qint64 maximum = samplesMilliseconds.back();
  return NativeCommandPaletteBenchmarkResult{
      std::move(samplesMilliseconds), minimum, median, maximum};
}

std::optional<qint64>
NativeCommandPaletteBenchmark::parseElapsedMilliseconds(const QByteArray& output) {
  const QRegularExpression expression(QStringLiteral(R"(^HCB_COMMAND_PALETTE_OPEN_MS=(\d+)$)"),
                                      QRegularExpression::MultilineOption);
  const QRegularExpressionMatch match = expression.match(QString::fromUtf8(output));
  bool valid = false;
  const qint64 milliseconds = match.captured(1).toLongLong(&valid);
  if (!valid || milliseconds < 0) {
    return std::nullopt;
  }
  return milliseconds;
}

QByteArray
NativeCommandPaletteBenchmark::toJson(const NativeCommandPaletteBenchmarkResult& result) {
  QJsonArray samples;
  for (const qint64 milliseconds : result.samplesMilliseconds) {
    samples.append(milliseconds);
  }
  return QJsonDocument(QJsonObject{{QStringLiteral("schema_version"), 1},
                                   {QStringLiteral("iterations"),
                                    static_cast<qint64>(result.samplesMilliseconds.size())},
                                   {QStringLiteral("minimum_ms"), result.minimumMilliseconds},
                                   {QStringLiteral("median_ms"), result.medianMilliseconds},
                                   {QStringLiteral("maximum_ms"), result.maximumMilliseconds},
                                   {QStringLiteral("samples_ms"), samples}})
      .toJson(QJsonDocument::Compact);
}

} // namespace hcb
