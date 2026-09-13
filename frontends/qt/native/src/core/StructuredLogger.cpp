#include "core/StructuredLogger.h"

#include "core/SecretRedactor.h"

#include <algorithm>
#include <utility>

namespace hcb {

StructuredLogger::StructuredLogger(const Clock& clock, std::size_t capacity)
    : clock_(clock), capacity_(std::max<std::size_t>(capacity, 1)) {}

void StructuredLogger::log(LogLevel level, LogEvent event, const LogMetadata& metadata) {
  const WallTimePoint timestamp = clock_.wallNow();
  const QString safeCategory = SecretRedactor::redactText(event.category, 80);
  const QString safeMessage = SecretRedactor::redactText(event.message, 1'000);
  LogMetadata safeMetadata = redactMetadata(metadata);

  std::lock_guard<std::mutex> lock(mutex_);
  entries_.push_back(LogEntry{nextSequence_++,
                              timestamp,
                              level,
                              safeCategory.isEmpty() ? QStringLiteral("misc") : safeCategory,
                              safeMessage.isEmpty() ? QStringLiteral("event") : safeMessage,
                              std::move(safeMetadata)});
  while (entries_.size() > capacity_) {
    entries_.pop_front();
  }
}

std::vector<LogEntry> StructuredLogger::entries(LogLevel minimumLevel) const {
  std::lock_guard<std::mutex> lock(mutex_);
  std::vector<LogEntry> matchingEntries;
  matchingEntries.reserve(entries_.size());
  for (const LogEntry& entry : entries_) {
    if (includes(entry.level, minimumLevel)) {
      matchingEntries.push_back(entry);
    }
  }
  return matchingEntries;
}

std::size_t StructuredLogger::size() const {
  std::lock_guard<std::mutex> lock(mutex_);
  return entries_.size();
}

void StructuredLogger::clear() {
  std::lock_guard<std::mutex> lock(mutex_);
  entries_.clear();
}

LogMetadata StructuredLogger::redactMetadata(const LogMetadata& metadata) {
  LogMetadata redacted;
  redacted.reserve(metadata.size());
  for (auto iterator = metadata.cbegin(); iterator != metadata.cend(); ++iterator) {
    const bool sensitiveKey = SecretRedactor::isSensitiveKey(iterator.key());
    redacted.insert(SecretRedactor::redactKey(iterator.key()),
                    sensitiveKey ? QStringLiteral("[redacted]")
                                 : SecretRedactor::redactText(iterator.value(), 500));
  }
  return redacted;
}

bool StructuredLogger::includes(LogLevel level, LogLevel minimumLevel) noexcept {
  return static_cast<std::uint8_t>(level) >= static_cast<std::uint8_t>(minimumLevel);
}

} // namespace hcb
