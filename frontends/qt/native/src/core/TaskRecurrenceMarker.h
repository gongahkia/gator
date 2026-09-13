#pragma once

#include <QList>
#include <QString>

#include <cstdint>
#include <optional>

namespace hcb {

enum class TaskRecurrenceFrequency : std::uint8_t {
  Daily,
  Weekly,
  Monthly,
  Yearly
};

enum class TaskRecurrenceEndKind : std::uint8_t {
  Never,
  Until,
  Count
};

struct TaskRecurrenceEndCondition final {
  TaskRecurrenceEndKind kind{TaskRecurrenceEndKind::Never};
  std::optional<QString> untilDate;
  std::optional<std::int32_t> count;
};

struct TaskRecurrenceMarker final {
  QString seriesId;
  QString occurrenceId;
  std::int32_t ordinal{0};
  TaskRecurrenceFrequency frequency{TaskRecurrenceFrequency::Daily};
  std::int32_t interval{1};
  QString anchorDate;
  QString timeZone;
  TaskRecurrenceEndCondition end;
  QString recurrenceRule;
  QList<QString> exclusionDates;
  QList<QString> additionDates;
  QString templateTitle;
  QString templateDueDate;
  QString templatePriority;
};

struct TaskRecurrenceRuleInfo final {
  TaskRecurrenceFrequency frequency{TaskRecurrenceFrequency::Daily};
  std::int32_t interval{1};
};

enum class TaskRecurrenceNotesState : std::uint8_t {
  Unmanaged,
  Managed,
  Malformed,
  UnsupportedVersion
};

struct TaskRecurrenceNotes final {
  TaskRecurrenceNotesState state{TaskRecurrenceNotesState::Unmanaged};
  QString userNotes;
  std::optional<TaskRecurrenceMarker> marker;
  QString diagnostic;
};

struct TaskRecurrenceSerializationResult final {
  QString notes;
  std::optional<QString> error;
};

[[nodiscard]] TaskRecurrenceNotes parseTaskRecurrenceNotes(const QString& notes);
[[nodiscard]] std::optional<TaskRecurrenceRuleInfo> parseTaskRecurrenceRule(const QString& rule);
[[nodiscard]] TaskRecurrenceSerializationResult
serializeTaskRecurrenceNotes(const QString& userNotes, const TaskRecurrenceMarker& marker);
[[nodiscard]] std::optional<QString> taskRecurrenceDate(const TaskRecurrenceMarker& marker,
                                                         std::int32_t ordinal);
[[nodiscard]] std::optional<TaskRecurrenceMarker>
taskRecurrenceSuccessor(const TaskRecurrenceMarker& marker);
[[nodiscard]] QString taskRecurrenceSummary(const TaskRecurrenceMarker& marker);

} // namespace hcb
