#include "core/RecurrenceResultCache.h"

#include <QHash>

#include <algorithm>
#include <cstdint>
#include <mutex>
#include <unordered_map>
#include <utility>

namespace {

struct RecurrenceCacheKey final {
  QString eventId;
  QString startAt;
  QString endAt;
  bool allDay{false};
  std::optional<QString> recurrenceRule;

  [[nodiscard]] bool operator==(const RecurrenceCacheKey&) const = default;
};

struct RecurrenceCacheKeyHash final {
  [[nodiscard]] std::size_t operator()(const RecurrenceCacheKey& key) const noexcept {
    const auto combine = [](std::size_t left, std::size_t right) {
      return left ^ (right + 0x9e3779b97f4a7c15ULL + (left << 6U) + (left >> 2U));
    };
    std::size_t hash = qHash(key.eventId);
    hash = combine(hash, qHash(key.startAt));
    hash = combine(hash, qHash(key.endAt));
    hash = combine(hash, key.allDay ? 1U : 0U);
    hash = combine(hash, key.recurrenceRule.has_value() ? 1U : 0U);
    return combine(hash, qHash(key.recurrenceRule.value_or(QString())));
  }
};

[[nodiscard]] RecurrenceCacheKey cacheKey(const hcb::RecurrenceExpansionRequest& request) {
  return {.eventId = request.eventId,
          .startAt = request.startAt,
          .endAt = request.endAt,
          .allDay = request.allDay,
          .recurrenceRule = request.recurrenceRule};
}

} // namespace

struct hcb::RecurrenceResultCacheState final {
  struct Entry final {
    QList<RecurrenceOccurrence> occurrences;
    std::uint64_t lastUse{0};
  };

  explicit RecurrenceResultCacheState(std::size_t capacityValue)
      : capacity(std::max<std::size_t>(capacityValue, 1)) {}

  [[nodiscard]] std::optional<QList<RecurrenceOccurrence>> find(const RecurrenceCacheKey& key) {
    std::lock_guard lock(mutex);
    const auto entry = entries.find(key);
    if (entry == entries.end()) {
      return std::nullopt;
    }
    entry->second.lastUse = nextUse();
    return entry->second.occurrences;
  }

  void store(RecurrenceCacheKey key, QList<RecurrenceOccurrence> occurrences) {
    std::lock_guard lock(mutex);
    const auto existing = entries.find(key);
    if (existing != entries.end()) {
      existing->second = {.occurrences = std::move(occurrences), .lastUse = nextUse()};
      return;
    }
    if (entries.size() == capacity) {
      entries.erase(leastRecentlyUsed());
    }
    entries.emplace(std::move(key),
                    Entry{.occurrences = std::move(occurrences), .lastUse = nextUse()});
  }

  void invalidateEvent(const QString& eventId) {
    std::lock_guard lock(mutex);
    for (auto entry = entries.begin(); entry != entries.end();) {
      if (entry->first.eventId == eventId) {
        entry = entries.erase(entry);
      } else {
        ++entry;
      }
    }
  }

  void clear() {
    std::lock_guard lock(mutex);
    entries.clear();
  }

  [[nodiscard]] std::size_t size() const {
    std::lock_guard lock(mutex);
    return entries.size();
  }

private:
  [[nodiscard]] std::unordered_map<RecurrenceCacheKey, Entry, RecurrenceCacheKeyHash>::iterator
  leastRecentlyUsed() {
    return std::min_element(
        entries.begin(), entries.end(), [](const auto& left, const auto& right) {
          return left.second.lastUse < right.second.lastUse;
        });
  }

  [[nodiscard]] std::uint64_t nextUse() noexcept { return ++useCounter; }

  mutable std::mutex mutex;
  const std::size_t capacity;
  std::uint64_t useCounter{0};
  std::unordered_map<RecurrenceCacheKey, Entry, RecurrenceCacheKeyHash> entries;
};

namespace hcb {

RecurrenceResultCache::RecurrenceResultCache(std::size_t capacity)
    : state_(std::make_shared<RecurrenceResultCacheState>(capacity)) {}

RecurrenceResultCache::~RecurrenceResultCache() = default;

std::optional<QList<RecurrenceOccurrence>>
RecurrenceResultCache::find(const RecurrenceExpansionRequest& request) const {
  return state_->find(cacheKey(request));
}

void RecurrenceResultCache::store(const RecurrenceExpansionRequest& request,
                                  QList<RecurrenceOccurrence> occurrences) {
  state_->store(cacheKey(request), std::move(occurrences));
}

void RecurrenceResultCache::invalidateEvent(const QString& eventId) {
  state_->invalidateEvent(eventId);
}

void RecurrenceResultCache::clear() { state_->clear(); }

std::size_t RecurrenceResultCache::size() const { return state_->size(); }

} // namespace hcb
