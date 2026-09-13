#include "core/LocalSearchQuery.h"

#include <QRegularExpression>

#include <algorithm>
#include <utility>

namespace hcb {
namespace {

[[nodiscard]] AppError invalid(const QString& message) {
  return AppError(AppErrorCode::Validation, message);
}

[[nodiscard]] std::variant<QStringList, AppError> tokenize(const QString& query) {
  QStringList tokens;
  QString token;
  bool quoted = false;
  bool escaped = false;
  for (const QChar character : query) {
    if (escaped) {
      token.append(character);
      escaped = false;
      continue;
    }
    if (character == u'\\' && quoted) {
      escaped = true;
      continue;
    }
    if (character == u'\"') {
      quoted = !quoted;
      continue;
    }
    if (character.isSpace() && !quoted) {
      if (!token.isEmpty()) {
        tokens.append(std::move(token));
        token.clear();
      }
      continue;
    }
    token.append(character);
  }
  if (escaped || quoted) {
    return invalid(QStringLiteral("Search query has an unterminated quoted value"));
  }
  if (!token.isEmpty()) {
    tokens.append(std::move(token));
  }
  return tokens;
}

[[nodiscard]] std::variant<LocalSearchDateFilter, AppError>
parseDateFilter(const QString& value, const QDate& today, const QString& name) {
  const QString normalized = value.toCaseFolded();
  if (normalized == QStringLiteral("any")) {
    return LocalSearchDateFilter{.match = LocalSearchDateMatch::Any};
  }
  if (normalized == QStringLiteral("none")) {
    return LocalSearchDateFilter{.match = LocalSearchDateMatch::None};
  }
  if (normalized == QStringLiteral("today")) {
    return LocalSearchDateFilter{.match = LocalSearchDateMatch::Exact, .first = today};
  }
  if (normalized == QStringLiteral("tomorrow")) {
    return LocalSearchDateFilter{.match = LocalSearchDateMatch::Exact,
                                 .first = today.addDays(1)};
  }
  if (normalized == QStringLiteral("yesterday")) {
    return LocalSearchDateFilter{.match = LocalSearchDateMatch::Exact,
                                 .first = today.addDays(-1)};
  }
  const auto parseDate = [](const QString& text) -> std::optional<QDate> {
    const QDate date = QDate::fromString(text, Qt::ISODate);
    return date.isValid() ? std::optional<QDate>(date) : std::nullopt;
  };
  if (normalized.startsWith(QStringLiteral("before:"))) {
    const std::optional<QDate> date = parseDate(value.sliced(7));
    if (!date.has_value()) {
      return invalid(QStringLiteral("Search filter %1 requires a valid date").arg(name));
    }
    return LocalSearchDateFilter{.match = LocalSearchDateMatch::Before, .first = *date};
  }
  if (normalized.startsWith(QStringLiteral("after:"))) {
    const std::optional<QDate> date = parseDate(value.sliced(6));
    if (!date.has_value()) {
      return invalid(QStringLiteral("Search filter %1 requires a valid date").arg(name));
    }
    return LocalSearchDateFilter{.match = LocalSearchDateMatch::After, .first = *date};
  }
  const qsizetype rangeSeparator = value.indexOf(QStringLiteral(".."));
  if (rangeSeparator >= 0) {
    const std::optional<QDate> first = parseDate(value.left(rangeSeparator));
    const std::optional<QDate> last = parseDate(value.mid(rangeSeparator + 2));
    if (!first.has_value() || !last.has_value() || *last < *first) {
      return invalid(QStringLiteral("Search filter %1 requires a valid date range").arg(name));
    }
    return LocalSearchDateFilter{.match = LocalSearchDateMatch::Range,
                                 .first = *first,
                                 .last = *last};
  }
  const std::optional<QDate> date = parseDate(value);
  if (!date.has_value()) {
    return invalid(QStringLiteral("Search filter %1 requires a valid date").arg(name));
  }
  return LocalSearchDateFilter{.match = LocalSearchDateMatch::Exact, .first = *date};
}

void appendResource(QList<LocalSearchResource>& resources, LocalSearchResource resource) {
  if (!resources.contains(resource)) {
    resources.append(resource);
  }
}

[[nodiscard]] std::optional<AppError> parseSources(const QString& value,
                                                    LocalSearchParsedQuery& parsed) {
  const QStringList values = value.split(u',', Qt::SkipEmptyParts);
  if (values.isEmpty()) {
    return invalid(QStringLiteral("Search filter source requires a value"));
  }
  for (const QString& source : values) {
    const QString normalized = source.trimmed().toCaseFolded();
    if (normalized == QStringLiteral("tasks")) {
      appendResource(parsed.resources, LocalSearchResource::TaskList);
      appendResource(parsed.resources, LocalSearchResource::Task);
    } else if (normalized == QStringLiteral("task")) {
      appendResource(parsed.resources, LocalSearchResource::Task);
    } else if (normalized == QStringLiteral("notes") || normalized == QStringLiteral("note")) {
      appendResource(parsed.resources, LocalSearchResource::Note);
    } else if (normalized == QStringLiteral("calendar")) {
      appendResource(parsed.resources, LocalSearchResource::Calendar);
      appendResource(parsed.resources, LocalSearchResource::Event);
    } else if (normalized == QStringLiteral("event")) {
      appendResource(parsed.resources, LocalSearchResource::Event);
    } else {
      return invalid(QStringLiteral("Search filter source has an unsupported value"));
    }
  }
  return std::nullopt;
}

[[nodiscard]] bool isFilterName(const QString& key) {
  static const QRegularExpression identifier(QStringLiteral("^[A-Za-z][A-Za-z0-9_-]*$"));
  return identifier.match(key).hasMatch();
}

} // namespace

LocalSearchQueryResult LocalSearchQuery::parse(QString query, QDate today) {
  const std::variant<QStringList, AppError> tokenResult = tokenize(query.trimmed());
  if (std::holds_alternative<AppError>(tokenResult)) {
    return std::get<AppError>(tokenResult);
  }
  if (!today.isValid()) {
    return invalid(QStringLiteral("Search date context is invalid"));
  }
  LocalSearchParsedQuery parsed;
  QStringList plainTokens;
  for (const QString& token : std::get<QStringList>(tokenResult)) {
    const qsizetype separator = token.indexOf(u':');
    if (separator < 0) {
      plainTokens.append(token);
      continue;
    }
    const QString key = token.left(separator).toCaseFolded();
    const QString value = token.mid(separator + 1).trimmed();
    if (!isFilterName(key)) {
      plainTokens.append(token);
      continue;
    }
    if (value.isEmpty()) {
      return invalid(QStringLiteral("Search filter %1 requires a value").arg(key));
    }
    if (key == QStringLiteral("source") || key == QStringLiteral("domain")) {
      if (const std::optional<AppError> error = parseSources(value, parsed); error.has_value()) {
        return *error;
      }
      parsed.chips.append(key + u':' + value);
      continue;
    }
    const auto duplicate = [&parsed, &key] {
      return (key == QStringLiteral("status") && parsed.taskStatus.has_value()) ||
             (key == QStringLiteral("due") && parsed.due.has_value()) ||
             (key == QStringLiteral("start") && parsed.start.has_value()) ||
             (key == QStringLiteral("priority") && parsed.priority.has_value()) ||
             (key == QStringLiteral("list") && parsed.taskList.has_value()) ||
             ((key == QStringLiteral("calendar") || key == QStringLiteral("cal")) &&
              parsed.calendar.has_value()) ||
             ((key == QStringLiteral("notes") || key == QStringLiteral("body")) &&
              parsed.hasBody.has_value());
    };
    if (duplicate()) {
      return invalid(QStringLiteral("Search filter %1 may only appear once").arg(key));
    }
    const QString normalized = value.toCaseFolded();
    if (key == QStringLiteral("status")) {
      if (normalized == QStringLiteral("open")) {
        parsed.taskStatus = QStringLiteral("active");
      } else if (normalized == QStringLiteral("done")) {
        parsed.taskStatus = QStringLiteral("completed");
      } else if (normalized == QStringLiteral("active") || normalized == QStringLiteral("completed") ||
                 normalized == QStringLiteral("hidden") || normalized == QStringLiteral("deleted")) {
        parsed.taskStatus = normalized;
      } else {
        return invalid(QStringLiteral("Search filter status has an unsupported value"));
      }
    } else if (key == QStringLiteral("due") || key == QStringLiteral("start")) {
      const std::variant<LocalSearchDateFilter, AppError> date =
          parseDateFilter(value, today, key);
      if (std::holds_alternative<AppError>(date)) {
        return std::get<AppError>(date);
      }
      if (key == QStringLiteral("due")) {
        parsed.due = std::get<LocalSearchDateFilter>(date);
      } else {
        parsed.start = std::get<LocalSearchDateFilter>(date);
      }
    } else if (key == QStringLiteral("priority")) {
      if (normalized != QStringLiteral("none") && normalized != QStringLiteral("low") &&
          normalized != QStringLiteral("medium") && normalized != QStringLiteral("high")) {
        return invalid(QStringLiteral("Search filter priority has an unsupported value"));
      }
      parsed.priority = normalized;
    } else if (key == QStringLiteral("list")) {
      parsed.taskList = value;
    } else if (key == QStringLiteral("calendar") || key == QStringLiteral("cal")) {
      parsed.calendar = value;
    } else if (key == QStringLiteral("notes") || key == QStringLiteral("body")) {
      if (normalized == QStringLiteral("yes")) {
        parsed.hasBody = true;
      } else if (normalized == QStringLiteral("no")) {
        parsed.hasBody = false;
      } else {
        return invalid(QStringLiteral("Search filter %1 accepts yes or no").arg(key));
      }
    } else {
      return invalid(QStringLiteral("Search filter %1 is not supported").arg(key));
    }
    parsed.chips.append(key + u':' + value);
  }
  parsed.plainText = plainTokens.join(u' ').trimmed();
  return parsed;
}

} // namespace hcb
