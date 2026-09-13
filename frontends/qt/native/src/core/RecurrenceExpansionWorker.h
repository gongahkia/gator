#pragma once

#include "core/AppError.h"
#include "core/Cancellation.h"

#include <QList>
#include <QString>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct RecurrenceExpansionRequest final {
  QString eventId;
  QString startAt;
  QString endAt;
  bool allDay{false};
  std::optional<QString> timeZone;
  std::optional<QString> recurrenceRule;
  std::optional<QString> rangeStartAt;
  std::optional<QString> rangeEndAt;
};

struct RecurrenceOccurrence final {
  QString id;
  QString startAt;
  QString endAt;
  std::optional<QString> originalStartAt;
};

using RecurrenceExpansionResult = std::variant<QList<RecurrenceOccurrence>, AppError>;

class RecurrenceExpansionWorker final {
public:
  [[nodiscard]] std::future<RecurrenceExpansionResult>
  expand(RecurrenceExpansionRequest request, const CancellationToken& cancellation = {}) const;
};

} // namespace hcb
