#pragma once

#include "core/Clock.h"

#include <QHash>
#include <QString>
#include <QStringView>

#include <cstddef>
#include <cstdint>
#include <deque>
#include <mutex>
#include <vector>

namespace hcb {

enum class LogLevel : std::uint8_t {
  Debug,
  Info,
  Warning,
  Error
};

using LogMetadata = QHash<QString, QString>;

struct LogEvent final {
  QStringView category;
  QStringView message;
};

struct LogEntry final {
  std::uint64_t sequence;
  WallTimePoint timestamp;
  LogLevel level;
  QString category;
  QString message;
  LogMetadata metadata;
};

class StructuredLogger final {
public:
  explicit StructuredLogger(const Clock& clock, std::size_t capacity = 500);

  void log(LogLevel level, LogEvent event, const LogMetadata& metadata = {});

  [[nodiscard]] std::vector<LogEntry> entries(LogLevel minimumLevel = LogLevel::Debug) const;
  [[nodiscard]] std::size_t size() const;
  void clear();

private:
  [[nodiscard]] static LogMetadata redactMetadata(const LogMetadata& metadata);
  [[nodiscard]] static bool includes(LogLevel level, LogLevel minimumLevel) noexcept;

  const Clock& clock_;
  const std::size_t capacity_;
  mutable std::mutex mutex_;
  std::deque<LogEntry> entries_;
  std::uint64_t nextSequence_{1};
};

} // namespace hcb
