#pragma once

#include "core/RecurrenceExpansionWorker.h"

#include <QList>
#include <QString>

#include <cstddef>
#include <memory>
#include <optional>

namespace hcb {

struct RecurrenceResultCacheState;

class RecurrenceResultCache final {
public:
  explicit RecurrenceResultCache(std::size_t capacity = 128);
  RecurrenceResultCache(const RecurrenceResultCache&) = delete;
  RecurrenceResultCache& operator=(const RecurrenceResultCache&) = delete;
  ~RecurrenceResultCache();

  [[nodiscard]] std::optional<QList<RecurrenceOccurrence>>
  find(const RecurrenceExpansionRequest& request) const;
  void store(const RecurrenceExpansionRequest& request, QList<RecurrenceOccurrence> occurrences);
  void invalidateEvent(const QString& eventId);
  void clear();
  [[nodiscard]] std::size_t size() const;

private:
  std::shared_ptr<RecurrenceResultCacheState> state_;
};

} // namespace hcb
