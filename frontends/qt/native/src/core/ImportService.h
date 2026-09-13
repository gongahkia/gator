#pragma once

#include <QByteArray>
#include <QList>
#include <QString>

#include <cstdint>
#include <optional>

namespace hcb {

enum class ImportFormat : std::uint8_t {
  Delimited,
  Csv
};

enum class ImportItemKind : std::uint8_t {
  Task,
  Event
};

struct ImportItem final {
  ImportItemKind kind{ImportItemKind::Task};
  int sourceLine{0};
  QString title;
  std::optional<QString> taskList;
  std::optional<QString> taskDue;
  std::optional<QString> taskNotes;
  std::optional<QString> taskPriority;
  std::optional<QString> taskRecurrenceRule;
  std::optional<QString> taskRecurrenceUntil;
  std::optional<int> taskRecurrenceCount;
  std::optional<QString> taskExclusionDates;
  std::optional<QString> taskAdditionDates;
  std::optional<QString> calendar;
  QString eventStart;
  QString eventEnd;
  bool eventAllDay{false};
  std::optional<QString> eventTimeZone;
  std::optional<QString> eventDescription;
  std::optional<QString> eventLocation;
  std::optional<QString> eventRecurrence;
};

struct ImportPreviewRow final {
  int sourceLine{0};
  ImportItemKind kind{ImportItemKind::Task};
  QString title;
  bool accepted{false};
  QString message;
};

struct ImportParseResult final {
  QList<ImportItem> items;
  QList<ImportPreviewRow> rows;
};

class ImportService final {
public:
  [[nodiscard]] static ImportFormat detectFormat(const QString& filename);
  [[nodiscard]] static ImportParseResult parse(ImportFormat format, QByteArray bytes);
};

} // namespace hcb
