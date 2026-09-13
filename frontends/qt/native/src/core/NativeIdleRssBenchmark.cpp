#include "core/NativeIdleRssBenchmark.h"

#include <QProcess>
#include <QProcessEnvironment>
#include <QRegularExpression>

#include <limits>
#include <utility>

namespace hcb {
namespace {

void setError(QString* error, QString message) {
  if (error != nullptr) {
    *error = std::move(message);
  }
}

} // namespace

std::optional<quint64> NativeIdleRssBenchmark::measure(const QString& executable,
                                                       const QStringList& arguments,
                                                       std::chrono::milliseconds idleDuration,
                                                       std::chrono::milliseconds timeout,
                                                       QString* error) {
  if (executable.isEmpty()) {
    setError(error, QStringLiteral("idle-RSS executable is required"));
    return std::nullopt;
  }
  if (idleDuration.count() <= 0 || idleDuration.count() > std::numeric_limits<int>::max() ||
      timeout.count() <= 0 || timeout.count() > std::numeric_limits<int>::max()) {
    setError(error, QStringLiteral("idle-RSS durations are outside the supported range"));
    return std::nullopt;
  }

  QProcess process;
  QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
  environment.insert(QStringLiteral("HCB_BENCHMARK_IDLE_RSS_AFTER_MS"),
                     QString::number(idleDuration.count()));
  process.setProcessEnvironment(environment);
  process.setProgram(executable);
  process.setArguments(arguments);
  const int timeoutMilliseconds = static_cast<int>(timeout.count());
  process.start();
  if (!process.waitForStarted(timeoutMilliseconds)) {
    setError(error,
             QStringLiteral("idle-RSS process did not start: %1").arg(process.errorString()));
    return std::nullopt;
  }
  if (!process.waitForFinished(timeoutMilliseconds)) {
    process.kill();
    process.waitForFinished(1'000);
    setError(error, QStringLiteral("idle-RSS process exceeded the timeout"));
    return std::nullopt;
  }
  if (process.exitStatus() != QProcess::NormalExit || process.exitCode() != 0) {
    setError(error,
             QStringLiteral("idle-RSS process failed with exit code %1: %2")
                 .arg(process.exitCode())
                 .arg(QString::fromUtf8(process.readAllStandardError())));
    return std::nullopt;
  }
  const auto residentBytes = parseResidentBytes(process.readAllStandardOutput());
  if (!residentBytes.has_value()) {
    setError(error, QStringLiteral("idle-RSS process did not emit a valid resident byte count"));
  }
  return residentBytes;
}

std::optional<quint64> NativeIdleRssBenchmark::parseResidentBytes(const QByteArray& output) {
  const QRegularExpression expression(QStringLiteral(R"(^HCB_IDLE_RSS_BYTES=(\d+)$)"),
                                      QRegularExpression::MultilineOption);
  const QRegularExpressionMatch match = expression.match(QString::fromUtf8(output));
  bool valid = false;
  const quint64 bytes = match.captured(1).toULongLong(&valid);
  if (!valid || bytes == 0 || bytes > static_cast<quint64>(std::numeric_limits<qint64>::max())) {
    return std::nullopt;
  }
  return bytes;
}

} // namespace hcb
