#pragma once

#include "core/AppError.h"
#include "core/CalendarMutationService.h"

#include <QList>
#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

enum class CalendarEventBulkAction : std::uint8_t {
  Delete,
  MoveToCalendar,
  SetColor,
  SetAvailability,
  SetVisibility,
  ShiftTime,
  ReplaceText
};

enum class CalendarEventBulkTextField : std::uint8_t {
  Title = 1,
  Description = 2,
  Location = 4
};

enum class CalendarEventBulkItemOutcome : std::uint8_t {
  Queued,
  Skipped,
  Failed
};

struct CalendarEventBulkMutationInput final {
  CalendarEventBulkAction action{CalendarEventBulkAction::Delete};
  QList<QString> eventIds;
  std::optional<QString> calendarId;
  std::optional<QString> colorId;
  std::optional<bool> available;
  std::optional<QString> visibility;
  std::optional<int> shiftMinutes;
  QString findText;
  QString replaceText;
  std::uint8_t textFields{0};
  int recurrenceScope{0}; // 0 skip, 1 current, 2 current+future, 3 full series
  bool previewOnly{false};
};

struct CalendarEventBulkMutationItem final {
  QString eventId;
  CalendarEventBulkItemOutcome outcome{CalendarEventBulkItemOutcome::Skipped};
  QString message;
};

struct CalendarEventBulkMutationSummary final {
  int requested{0};
  int eligible{0};
  int applied{0};
  int queued{0};
  int conflicted{0};
  int failed{0};
  int skipped{0};
  QList<CalendarEventBulkMutationItem> items;
};

using CalendarEventBulkMutationResult =
    std::variant<CalendarEventBulkMutationSummary, AppError>;

class CalendarEventBulkMutationService final {
public:
  explicit CalendarEventBulkMutationService(CalendarMutationService& calendarMutationService);
  CalendarEventBulkMutationService(const CalendarEventBulkMutationService&) = delete;
  CalendarEventBulkMutationService& operator=(const CalendarEventBulkMutationService&) = delete;

  [[nodiscard]] std::future<CalendarEventBulkMutationResult>
  execute(CalendarEventBulkMutationInput input);

private:
  CalendarMutationService& calendarMutationService_;
};

} // namespace hcb
