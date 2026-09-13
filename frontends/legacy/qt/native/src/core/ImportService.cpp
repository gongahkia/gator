#include "core/ImportService.h"

#include <QHash>
#include <QSet>
#include <QStringConverter>

#include <limits>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumImportBytes = 5 * 1024 * 1024;
constexpr qsizetype kMaximumImportRows = 1'000;
constexpr qsizetype kMaximumLineLength = 32 * 1024;
constexpr qsizetype kMaximumFieldLength = 524'416;
constexpr int kSchemaVersion = 1;

struct CsvRow final {
  int sourceLine{1};
  QStringList fields;
};

[[nodiscard]] std::optional<ImportItemKind> kindForText(QString value) {
  value = value.trimmed().toCaseFolded();
  if (value == QStringLiteral("task")) return ImportItemKind::Task;
  if (value == QStringLiteral("event")) return ImportItemKind::Event;
  return std::nullopt;
}

[[nodiscard]] std::optional<bool> booleanForText(QString value) {
  value = value.trimmed().toCaseFolded();
  if (value == QStringLiteral("true")) return true;
  if (value == QStringLiteral("false")) return false;
  return std::nullopt;
}

[[nodiscard]] std::optional<int> countForText(const QString& value) {
  if (value.isEmpty()) return std::optional<int>{};
  bool valid = false;
  const int count = value.toInt(&valid);
  return valid && count > 0 && count <= 10'000 ? std::optional<int>(count) : std::nullopt;
}

[[nodiscard]] std::optional<QString> optionalValue(const QHash<QString, QString>& fields,
                                                    const QString& key) {
  const auto value = fields.constFind(key);
  return value == fields.cend() || value->isEmpty() ? std::optional<QString>{}
                                                     : std::optional<QString>(*value);
}

[[nodiscard]] ImportPreviewRow rejected(int line, QString message) {
  return {.sourceLine = line, .accepted = false, .message = std::move(message)};
}

[[nodiscard]] bool hasOnly(const QHash<QString, QString>& fields, const QSet<QString>& allowed) {
  for (auto it = fields.cbegin(); it != fields.cend(); ++it) {
    if (!allowed.contains(it.key())) return false;
  }
  return true;
}

[[nodiscard]] std::optional<ImportItem>
itemFromFields(int line, ImportItemKind kind, const QHash<QString, QString>& fields, QString& error) {
  const QSet<QString> taskFields{QStringLiteral("title"), QStringLiteral("list"),
                                 QStringLiteral("due"), QStringLiteral("notes"),
                                 QStringLiteral("priority"), QStringLiteral("rrule"),
                                 QStringLiteral("until"), QStringLiteral("count"),
                                 QStringLiteral("exclude"), QStringLiteral("include")};
  const QSet<QString> eventFields{QStringLiteral("title"), QStringLiteral("calendar"),
                                  QStringLiteral("start"), QStringLiteral("end"),
                                  QStringLiteral("all_day"), QStringLiteral("time_zone"),
                                  QStringLiteral("description"), QStringLiteral("location"),
                                  QStringLiteral("recurrence")};
  if (!hasOnly(fields, kind == ImportItemKind::Task ? taskFields : eventFields)) {
    error = QStringLiteral("contains an unsupported field");
    return std::nullopt;
  }
  const auto title = fields.constFind(QStringLiteral("title"));
  if (title == fields.cend() || title->trimmed().isEmpty()) {
    error = QStringLiteral("requires a title");
    return std::nullopt;
  }
  ImportItem item{.kind = kind, .sourceLine = line, .title = title->trimmed()};
  if (kind == ImportItemKind::Task) {
    item.taskList = optionalValue(fields, QStringLiteral("list"));
    item.taskDue = optionalValue(fields, QStringLiteral("due"));
    item.taskNotes = optionalValue(fields, QStringLiteral("notes"));
    item.taskPriority = optionalValue(fields, QStringLiteral("priority"));
    item.taskRecurrenceRule = optionalValue(fields, QStringLiteral("rrule"));
    item.taskRecurrenceUntil = optionalValue(fields, QStringLiteral("until"));
    item.taskExclusionDates = optionalValue(fields, QStringLiteral("exclude"));
    item.taskAdditionDates = optionalValue(fields, QStringLiteral("include"));
    const auto count = fields.constFind(QStringLiteral("count"));
    if (count != fields.cend()) {
      const std::optional<int> parsed = countForText(*count);
      if (!parsed.has_value()) {
        error = QStringLiteral("has an invalid recurrence count");
        return std::nullopt;
      }
      item.taskRecurrenceCount = *parsed;
    }
    if ((item.taskRecurrenceUntil.has_value() || item.taskRecurrenceCount.has_value() ||
         item.taskExclusionDates.has_value() || item.taskAdditionDates.has_value()) &&
        !item.taskRecurrenceRule.has_value()) {
      error = QStringLiteral("recurrence options require rrule");
      return std::nullopt;
    }
    if (item.taskRecurrenceUntil.has_value() && item.taskRecurrenceCount.has_value()) {
      error = QStringLiteral("cannot use recurrence until and count together");
      return std::nullopt;
    }
    return item;
  }
  const auto start = fields.constFind(QStringLiteral("start"));
  const auto end = fields.constFind(QStringLiteral("end"));
  if (start == fields.cend() || start->isEmpty() || end == fields.cend() || end->isEmpty()) {
    error = QStringLiteral("requires start and end");
    return std::nullopt;
  }
  item.calendar = optionalValue(fields, QStringLiteral("calendar"));
  item.eventStart = *start;
  item.eventEnd = *end;
  item.eventTimeZone = optionalValue(fields, QStringLiteral("time_zone"));
  item.eventDescription = optionalValue(fields, QStringLiteral("description"));
  item.eventLocation = optionalValue(fields, QStringLiteral("location"));
  item.eventRecurrence = optionalValue(fields, QStringLiteral("recurrence"));
  const auto allDay = fields.constFind(QStringLiteral("all_day"));
  if (allDay != fields.cend()) {
    const std::optional<bool> parsed = booleanForText(*allDay);
    if (!parsed.has_value()) {
      error = QStringLiteral("has an invalid all_day value");
      return std::nullopt;
    }
    item.eventAllDay = *parsed;
  }
  return item;
}

[[nodiscard]] std::optional<QHash<QString, QString>>
parseDelimitedFields(QStringView text, QString& error) {
  QHash<QString, QString> fields;
  qsizetype index = 0;
  while (index < text.size()) {
    while (index < text.size() && text.at(index).isSpace()) ++index;
    if (index == text.size()) break;
    const qsizetype keyStart = index;
    while (index < text.size() && (text.at(index).isLetterOrNumber() || text.at(index) == u'_')) ++index;
    if (keyStart == index || index == text.size() || text.at(index) != u'=') {
      error = QStringLiteral("has invalid key=value syntax");
      return std::nullopt;
    }
    const QString key = text.sliced(keyStart, index - keyStart).toString().toCaseFolded();
    ++index;
    QString value;
    if (index < text.size() && text.at(index) == u'\"') {
      ++index;
      bool closed = false;
      while (index < text.size()) {
        const QChar character = text.at(index++);
        if (character == u'\"') {
          closed = true;
          break;
        }
        if (character != u'\\') {
          value.append(character);
          continue;
        }
        if (index == text.size()) {
          error = QStringLiteral("has an unterminated escape");
          return std::nullopt;
        }
        const QChar escaped = text.at(index++);
        if (escaped == u'n') value.append(u'\n');
        else if (escaped == u'r') value.append(u'\r');
        else if (escaped == u't') value.append(u'\t');
        else if (escaped == u'\"' || escaped == u'\\') value.append(escaped);
        else {
          error = QStringLiteral("has an unsupported escape");
          return std::nullopt;
        }
      }
      if (!closed || (index < text.size() && !text.at(index).isSpace())) {
        error = QStringLiteral("has an unterminated quoted value");
        return std::nullopt;
      }
    } else {
      const qsizetype valueStart = index;
      while (index < text.size() && !text.at(index).isSpace()) ++index;
      value = text.sliced(valueStart, index - valueStart).toString();
    }
    if (value.size() > kMaximumFieldLength || fields.contains(key)) {
      error = value.size() > kMaximumFieldLength ? QStringLiteral("has an oversized field")
                                                   : QStringLiteral("repeats a field");
      return std::nullopt;
    }
    fields.insert(key, std::move(value));
  }
  return fields;
}

[[nodiscard]] std::optional<QList<CsvRow>> parseCsvRows(const QString& text, QString& error) {
  QList<CsvRow> rows;
  QStringList fields;
  QString field;
  int line = 1;
  int recordLine = 1;
  bool quoted = false;
  bool afterQuote = false;
  const auto appendRecord = [&] {
    fields.append(std::move(field));
    field.clear();
    if (fields.size() != 1 || !fields.front().isEmpty()) {
      rows.append({.sourceLine = recordLine, .fields = std::move(fields)});
    }
    fields.clear();
    recordLine = line;
  };
  for (qsizetype index = 0; index < text.size(); ++index) {
    const QChar character = text.at(index);
    if (quoted) {
      if (character == u'\"') {
        if (index + 1 < text.size() && text.at(index + 1) == u'\"') {
          field.append(u'\"');
          ++index;
        } else {
          quoted = false;
          afterQuote = true;
        }
      } else {
        field.append(character);
        if (character == u'\n') ++line;
      }
      continue;
    }
    if (afterQuote) {
      if (character == u',') {
        fields.append(std::move(field));
        field.clear();
        afterQuote = false;
      } else if (character == u'\n' || character == u'\r') {
        if (character == u'\r' && index + 1 < text.size() && text.at(index + 1) == u'\n') ++index;
        ++line;
        appendRecord();
        afterQuote = false;
      } else {
        error = QStringLiteral("CSV has text after a closing quote");
        return std::nullopt;
      }
      continue;
    }
    if (character == u'\"') {
      if (!field.isEmpty()) {
        error = QStringLiteral("CSV quote does not start a field");
        return std::nullopt;
      }
      quoted = true;
    } else if (character == u',') {
      fields.append(std::move(field));
      field.clear();
    } else if (character == u'\n' || character == u'\r') {
      if (character == u'\r' && index + 1 < text.size() && text.at(index + 1) == u'\n') ++index;
      ++line;
      appendRecord();
    } else {
      field.append(character);
    }
    if (field.size() > kMaximumFieldLength) {
      error = QStringLiteral("CSV has an oversized field");
      return std::nullopt;
    }
  }
  if (quoted) {
    error = QStringLiteral("CSV has an unterminated quoted field");
    return std::nullopt;
  }
  if (afterQuote || !field.isEmpty() || !fields.isEmpty()) appendRecord();
  return rows;
}

[[nodiscard]] ImportParseResult parseDelimited(const QString& text) {
  ImportParseResult result;
  const QStringList lines = text.split(u'\n');
  for (qsizetype index = 0; index < lines.size(); ++index) {
    QString line = lines.at(index);
    if (line.endsWith(u'\r')) line.chop(1);
    const int sourceLine = static_cast<int>(index + 1);
    if (line.trimmed().isEmpty() || line.trimmed().startsWith(u'#')) continue;
    if (line.size() > kMaximumLineLength) {
      result.rows.append(rejected(sourceLine, QStringLiteral("line exceeds the import limit")));
      continue;
    }
    const qsizetype separator = line.indexOf(QChar::Space);
    const QString type = (separator < 0 ? line : line.first(separator)).toCaseFolded();
    const std::optional<ImportItemKind> kind = kindForText(type);
    if (!kind.has_value()) {
      result.rows.append(rejected(sourceLine, QStringLiteral("must start with task or event")));
      continue;
    }
    QString error;
    const std::optional<QHash<QString, QString>> fields =
        parseDelimitedFields(separator < 0 ? QStringView{} : QStringView(line).sliced(separator + 1), error);
    if (!fields.has_value()) {
      result.rows.append(rejected(sourceLine, std::move(error)));
      continue;
    }
    const std::optional<ImportItem> item = itemFromFields(sourceLine, *kind, *fields, error);
    if (!item.has_value()) {
      result.rows.append(rejected(sourceLine, std::move(error)));
      continue;
    }
    result.rows.append({.sourceLine = sourceLine,
                        .kind = *kind,
                        .title = item->title,
                        .accepted = true,
                        .message = QStringLiteral("ready")});
    result.items.append(*item);
  }
  return result;
}

[[nodiscard]] ImportParseResult parseCsv(const QString& text) {
  ImportParseResult result;
  QString error;
  const std::optional<QList<CsvRow>> rows = parseCsvRows(text, error);
  if (!rows.has_value()) {
    result.rows.append(rejected(0, std::move(error)));
    return result;
  }
  const QStringList expected{QStringLiteral("schema_version"), QStringLiteral("kind"),
                             QStringLiteral("title"), QStringLiteral("list"),
                             QStringLiteral("calendar"), QStringLiteral("due"),
                             QStringLiteral("notes"), QStringLiteral("priority"),
                             QStringLiteral("rrule"), QStringLiteral("until"),
                             QStringLiteral("count"), QStringLiteral("exclude"),
                             QStringLiteral("include"), QStringLiteral("start"),
                             QStringLiteral("end"), QStringLiteral("all_day"),
                             QStringLiteral("time_zone"), QStringLiteral("description"),
                             QStringLiteral("location"), QStringLiteral("recurrence")};
  if (rows->isEmpty() || rows->front().fields != expected) {
    result.rows.append(rejected(rows->isEmpty() ? 1 : rows->front().sourceLine,
                                QStringLiteral("CSV header does not match schema version 1")));
    return result;
  }
  for (qsizetype index = 1; index < rows->size(); ++index) {
    const CsvRow& row = rows->at(index);
    if (row.fields.size() != expected.size()) {
      result.rows.append(rejected(row.sourceLine, QStringLiteral("CSV column count is invalid")));
      continue;
    }
    if (row.fields.at(0) != QString::number(kSchemaVersion)) {
      result.rows.append(rejected(row.sourceLine, QStringLiteral("CSV schema version is unsupported")));
      continue;
    }
    const std::optional<ImportItemKind> kind = kindForText(row.fields.at(1));
    if (!kind.has_value()) {
      result.rows.append(rejected(row.sourceLine, QStringLiteral("CSV kind must be task or event")));
      continue;
    }
    QHash<QString, QString> fields;
    for (qsizetype column = 2; column < expected.size(); ++column) {
      if (!row.fields.at(column).isEmpty()) fields.insert(expected.at(column), row.fields.at(column));
    }
    QString itemError;
    const std::optional<ImportItem> item = itemFromFields(row.sourceLine, *kind, fields, itemError);
    if (!item.has_value()) {
      result.rows.append(rejected(row.sourceLine, std::move(itemError)));
      continue;
    }
    result.rows.append({.sourceLine = row.sourceLine,
                        .kind = *kind,
                        .title = item->title,
                        .accepted = true,
                        .message = QStringLiteral("ready")});
    result.items.append(*item);
  }
  return result;
}

} // namespace

ImportFormat ImportService::detectFormat(const QString& filename) {
  return filename.endsWith(QStringLiteral(".csv"), Qt::CaseInsensitive) ? ImportFormat::Csv
                                                                          : ImportFormat::Delimited;
}

ImportParseResult ImportService::parse(ImportFormat format, QByteArray bytes) {
  ImportParseResult result;
  if (bytes.isEmpty() || bytes.size() > kMaximumImportBytes) {
    result.rows.append(rejected(0, QStringLiteral("Import file must contain at most 5 MiB")));
    return result;
  }
  QStringDecoder decoder(QStringDecoder::Utf8);
  QString text = decoder.decode(bytes);
  if (decoder.hasError()) {
    result.rows.append(rejected(0, QStringLiteral("Import file must be UTF-8")));
    return result;
  }
  if (text.startsWith(QChar::ByteOrderMark)) text.remove(0, 1);
  result = format == ImportFormat::Csv ? parseCsv(text) : parseDelimited(text);
  if (result.rows.size() > kMaximumImportRows) {
    return {.items = {},
            .rows = {rejected(0, QStringLiteral("Import file exceeds 1,000 records"))}};
  }
  return result;
}

} // namespace hcb
