#include "core/QuickCaptureParser.h"

#include <QRegularExpression>

#include <algorithm>
#include <array>
#include <limits>
#include <utility>

namespace hcb {
namespace {

struct TextSpan final {
  qsizetype start{0};
  qsizetype length{0};
  QString id;
  QString label;
  bool removable{true};
};

struct DateMatch final {
  QDate date;
  qsizetype start{0};
  qsizetype length{0};
};

struct TimeMatch final {
  QTime time;
  qsizetype start{0};
  qsizetype length{0};
};

[[nodiscard]] bool overlaps(const TextSpan& left, const TextSpan& right) {
  return left.start < right.start + right.length && right.start < left.start + left.length;
}

[[nodiscard]] bool isDisabled(const QStringList& disabled, const QString& id) {
  return disabled.contains(id);
}

void addSpan(QList<TextSpan>& spans,
             const QStringList& disabled,
             TextSpan span,
             QList<QuickCaptureRecognition>& recognitions) {
  if (span.length <= 0 || isDisabled(disabled, span.id)) {
    return;
  }
  if (std::any_of(spans.cbegin(), spans.cend(), [&span](const TextSpan& current) {
        return overlaps(current, span);
      })) {
    return;
  }
  recognitions.append({.id = span.id, .label = span.label, .removable = span.removable});
  spans.append(std::move(span));
}

[[nodiscard]] std::optional<QRegularExpressionMatch>
firstAliasMatch(const QString& text, const QStringList& aliases) {
  std::optional<QRegularExpressionMatch> first;
  for (const QString& alias : aliases) {
    const QString trimmed = alias.trimmed();
    if (trimmed.isEmpty()) {
      continue;
    }
    const QRegularExpression expression(
        QStringLiteral("(?<![\\p{L}\\p{N}])%1(?![\\p{L}\\p{N}])")
            .arg(QRegularExpression::escape(trimmed)),
        QRegularExpression::CaseInsensitiveOption);
    const QRegularExpressionMatch match = expression.match(text);
    if (!match.hasMatch() ||
        (first.has_value() && match.capturedStart() >= first->capturedStart())) {
      continue;
    }
    first = match;
  }
  return first;
}

[[nodiscard]] std::optional<DateMatch> firstDateMatch(const QString& text,
                                                       const QDate& today) {
  QList<DateMatch> candidates;
  const auto addCandidate = [&candidates](const QDate& date, const QRegularExpressionMatch& match) {
    if (date.isValid() && match.hasMatch()) {
      candidates.append({.date = date, .start = match.capturedStart(), .length = match.capturedLength()});
    }
  };

  const QRegularExpression isoExpression(QStringLiteral("\\b(\\d{4})-(\\d{2})-(\\d{2})\\b"));
  const QRegularExpressionMatch iso = isoExpression.match(text);
  if (iso.hasMatch()) {
    addCandidate(QDate(iso.captured(1).toInt(), iso.captured(2).toInt(), iso.captured(3).toInt()), iso);
  }

  const QRegularExpression relativeExpression(QStringLiteral("\\b(today|tomorrow)\\b"),
                                              QRegularExpression::CaseInsensitiveOption);
  const QRegularExpressionMatch relative = relativeExpression.match(text);
  if (relative.hasMatch()) {
    const bool tomorrow = relative.captured(1).compare(QStringLiteral("tomorrow"), Qt::CaseInsensitive) == 0;
    addCandidate(today.addDays(tomorrow ? 1 : 0), relative);
  }

  const QRegularExpression weekdayExpression(
      QStringLiteral("\\b(next\\s+)?(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\\b"),
      QRegularExpression::CaseInsensitiveOption);
  const QRegularExpressionMatch weekday = weekdayExpression.match(text);
  if (weekday.hasMatch()) {
    static const std::array<QString, 7> names{QStringLiteral("monday"),
                                               QStringLiteral("tuesday"),
                                               QStringLiteral("wednesday"),
                                               QStringLiteral("thursday"),
                                               QStringLiteral("friday"),
                                               QStringLiteral("saturday"),
                                               QStringLiteral("sunday")};
    const QString name = weekday.captured(2).toCaseFolded();
    const auto position = std::find(names.cbegin(), names.cend(), name);
    if (position != names.cend()) {
      const int target = static_cast<int>(std::distance(names.cbegin(), position)) + 1;
      int delta = (target - today.dayOfWeek() + 7) % 7;
      if (!weekday.captured(1).isEmpty() && delta == 0) {
        delta = 7;
      }
      addCandidate(today.addDays(delta), weekday);
    }
  }

  const QRegularExpression inDaysExpression(
      QStringLiteral("\\bin\\s+(\\d{1,3})\\s+(days?|weeks?)\\b"),
      QRegularExpression::CaseInsensitiveOption);
  const QRegularExpressionMatch inDays = inDaysExpression.match(text);
  if (inDays.hasMatch()) {
    const int amount = inDays.captured(1).toInt();
    const bool weeks = inDays.captured(2).startsWith(QStringLiteral("week"), Qt::CaseInsensitive);
    addCandidate(today.addDays(amount * (weeks ? 7 : 1)), inDays);
  }

  const QRegularExpression monthExpression(
      QStringLiteral("\\b(january|february|march|april|may|june|july|august|september|october|november|december)\\s+"
                     "(\\d{1,2})(?:st|nd|rd|th)?(?:,?\\s+(\\d{4}))?\\b"),
      QRegularExpression::CaseInsensitiveOption);
  const QRegularExpressionMatch month = monthExpression.match(text);
  if (month.hasMatch()) {
    static const std::array<QString, 12> names{QStringLiteral("january"),
                                                QStringLiteral("february"),
                                                QStringLiteral("march"),
                                                QStringLiteral("april"),
                                                QStringLiteral("may"),
                                                QStringLiteral("june"),
                                                QStringLiteral("july"),
                                                QStringLiteral("august"),
                                                QStringLiteral("september"),
                                                QStringLiteral("october"),
                                                QStringLiteral("november"),
                                                QStringLiteral("december")};
    const auto position = std::find(names.cbegin(), names.cend(), month.captured(1).toCaseFolded());
    if (position != names.cend()) {
      const int monthNumber = static_cast<int>(std::distance(names.cbegin(), position)) + 1;
      int year = month.captured(3).isEmpty() ? today.year() : month.captured(3).toInt();
      QDate date(year, monthNumber, month.captured(2).toInt());
      if (month.captured(3).isEmpty() && date.isValid() && date < today) {
        date = QDate(year + 1, monthNumber, month.captured(2).toInt());
      }
      addCandidate(date, month);
    }
  }

  if (candidates.isEmpty()) {
    return std::nullopt;
  }
  std::sort(candidates.begin(), candidates.end(), [](const DateMatch& left, const DateMatch& right) {
    return left.start < right.start;
  });
  return candidates.constFirst();
}

[[nodiscard]] std::optional<TimeMatch> firstTimeMatch(const QString& text) {
  const QRegularExpression twelveHourExpression(
      QStringLiteral("\\b(?:at\\s+)?(\\d{1,2})(?::(\\d{2}))?\\s*(am|pm)\\b"),
      QRegularExpression::CaseInsensitiveOption);
  const QRegularExpressionMatch twelveHour = twelveHourExpression.match(text);
  if (twelveHour.hasMatch()) {
    int hour = twelveHour.captured(1).toInt();
    const int minute = twelveHour.captured(2).isEmpty() ? 0 : twelveHour.captured(2).toInt();
    const bool afternoon = twelveHour.captured(3).compare(QStringLiteral("pm"), Qt::CaseInsensitive) == 0;
    if (hour >= 1 && hour <= 12 && minute >= 0 && minute <= 59) {
      hour %= 12;
      if (afternoon) {
        hour += 12;
      }
      return TimeMatch{.time = QTime(hour, minute),
                       .start = twelveHour.capturedStart(),
                       .length = twelveHour.capturedLength()};
    }
  }

  const QRegularExpression twentyFourHourExpression(
      QStringLiteral("\\b(?:at\\s+)?([01]?\\d|2[0-3]):([0-5]\\d)\\b"),
      QRegularExpression::CaseInsensitiveOption);
  const QRegularExpressionMatch twentyFourHour = twentyFourHourExpression.match(text);
  if (twentyFourHour.hasMatch()) {
    return TimeMatch{.time = QTime(twentyFourHour.captured(1).toInt(), twentyFourHour.captured(2).toInt()),
                     .start = twentyFourHour.capturedStart(),
                     .length = twentyFourHour.capturedLength()};
  }
  return std::nullopt;
}

[[nodiscard]] QString removeSpans(const QString& text, QList<TextSpan> spans) {
  std::sort(spans.begin(), spans.end(), [](const TextSpan& left, const TextSpan& right) {
    return left.start > right.start;
  });
  QString result = text;
  for (const TextSpan& span : spans) {
    if (span.removable) {
      result.remove(span.start, span.length);
    }
  }
  return result.simplified();
}

[[nodiscard]] QDateTime normalizedNow(const QuickCaptureParseRequest& request) {
  const QTimeZone timeZone = request.timeZone.isValid() ? request.timeZone : QTimeZone::systemTimeZone();
  return (request.now.isValid() ? request.now : QDateTime::currentDateTimeUtc()).toTimeZone(timeZone);
}

[[nodiscard]] int boundedDuration(int value) {
  return value >= 1 && value <= 1'440 ? value : 30;
}

} // namespace

QuickCaptureAliases QuickCaptureParser::defaultAliases() {
  return {.task = {QStringLiteral("task")},
          .event = {QStringLiteral("event")},
          .highPriority = {QStringLiteral("p1")},
          .mediumPriority = {QStringLiteral("p2")},
          .lowPriority = {QStringLiteral("p3")}};
}

QuickCaptureParseResult QuickCaptureParser::parse(const QuickCaptureParseRequest& request) {
  const QDateTime now = normalizedNow(request);
  QuickCaptureParseResult result{.kind = request.kind,
                                 .rawTitle = request.text.trimmed(),
                                 .parsedTitle = request.text.trimmed(),
                                 .eventDurationMinutes = boundedDuration(request.defaultEventDurationMinutes)};
  QList<TextSpan> spans;

  const auto chooseType = [&result, &spans, &request](const QStringList& aliases,
                                                        QuickCaptureKind kind,
                                                        const QString& label) {
    const std::optional<QRegularExpressionMatch> match = firstAliasMatch(request.text, aliases);
    if (!match.has_value()) {
      return;
    }
    TextSpan span{.start = match->capturedStart(),
                  .length = match->capturedLength(),
                  .id = QStringLiteral("type:%1:%2").arg(match->capturedStart()).arg(match->capturedLength()),
                  .label = label};
    if (isDisabled(request.disabledRecognitionIds, span.id) ||
        std::any_of(spans.cbegin(), spans.cend(), [&span](const TextSpan& current) {
          return overlaps(current, span);
        })) {
      return;
    }
    result.kind = kind;
    addSpan(spans, request.disabledRecognitionIds, std::move(span), result.recognitions);
  };
  const std::optional<QRegularExpressionMatch> taskMatch = firstAliasMatch(request.text, request.aliases.task);
  const std::optional<QRegularExpressionMatch> eventMatch = firstAliasMatch(request.text, request.aliases.event);
  if (taskMatch.has_value() && (!eventMatch.has_value() || taskMatch->capturedStart() < eventMatch->capturedStart())) {
    chooseType(request.aliases.task, QuickCaptureKind::Task, QStringLiteral("Task"));
  } else if (eventMatch.has_value()) {
    chooseType(request.aliases.event, QuickCaptureKind::Event, QStringLiteral("Event"));
  }

  const auto choosePriority = [&result, &spans, &request](const QStringList& aliases,
                                                            int priority,
                                                            const QString& label) {
    const std::optional<QRegularExpressionMatch> match = firstAliasMatch(request.text, aliases);
    if (!match.has_value()) {
      return;
    }
    TextSpan span{.start = match->capturedStart(),
                  .length = match->capturedLength(),
                  .id = QStringLiteral("priority:%1:%2").arg(match->capturedStart()).arg(match->capturedLength()),
                  .label = label};
    if (isDisabled(request.disabledRecognitionIds, span.id) ||
        std::any_of(spans.cbegin(), spans.cend(), [&span](const TextSpan& current) {
          return overlaps(current, span);
        })) {
      return;
    }
    result.taskPriority = priority;
    addSpan(spans, request.disabledRecognitionIds, std::move(span), result.recognitions);
  };
  if (result.kind == QuickCaptureKind::Task) {
    choosePriority(request.aliases.highPriority, 3, QStringLiteral("High priority"));
    choosePriority(request.aliases.mediumPriority, 2, QStringLiteral("Medium priority"));
    choosePriority(request.aliases.lowPriority, 1, QStringLiteral("Low priority"));
  }

  const QRegularExpression recurrenceExpression(
      QStringLiteral("\\bevery(?:\\s+(\\d{1,3}))?\\s+(day|week|month|year)s?\\b"),
      QRegularExpression::CaseInsensitiveOption);
  const QRegularExpressionMatch recurrence = recurrenceExpression.match(request.text);
  if (recurrence.hasMatch()) {
    const int interval = recurrence.captured(1).isEmpty() ? 1 : recurrence.captured(1).toInt();
    const QString unit = recurrence.captured(2).toCaseFolded();
    const int frequency = unit == QStringLiteral("day") ? 0
                        : unit == QStringLiteral("week") ? 1
                        : unit == QStringLiteral("month") ? 2
                        : 3;
    TextSpan span{.start = recurrence.capturedStart(),
                  .length = recurrence.capturedLength(),
                  .id = QStringLiteral("recurrence:%1:%2").arg(recurrence.capturedStart()).arg(recurrence.capturedLength()),
                  .label = interval == 1 ? QStringLiteral("Repeats every %1").arg(unit)
                                         : QStringLiteral("Repeats every %1 %2s").arg(interval).arg(unit)};
    if (!isDisabled(request.disabledRecognitionIds, span.id)) {
      result.recurrence = {.enabled = true,
                           .frequency = frequency,
                           .interval = interval};
      if (unit == QStringLiteral("day")) {
        result.recurrence.rrule = QStringLiteral("RRULE:FREQ=DAILY;INTERVAL=%1").arg(interval);
      } else if (unit == QStringLiteral("week")) {
        result.recurrence.rrule = QStringLiteral("RRULE:FREQ=WEEKLY;INTERVAL=%1").arg(interval);
      } else if (unit == QStringLiteral("month")) {
        result.recurrence.rrule = QStringLiteral("RRULE:FREQ=MONTHLY;INTERVAL=%1").arg(interval);
      } else {
        result.recurrence.rrule = QStringLiteral("RRULE:FREQ=YEARLY;INTERVAL=%1").arg(interval);
      }
      addSpan(spans, request.disabledRecognitionIds, std::move(span), result.recognitions);
    }
  }

  if (result.kind == QuickCaptureKind::Event) {
    const QRegularExpression durationExpression(
        QStringLiteral("\\bfor\\s+(\\d{1,4})\\s*(m|min|mins|minute|minutes|h|hr|hrs|hour|hours)\\b"),
        QRegularExpression::CaseInsensitiveOption);
    const QRegularExpressionMatch duration = durationExpression.match(request.text);
    if (duration.hasMatch()) {
      const int multiplier = duration.captured(2).startsWith(QStringLiteral("h"), Qt::CaseInsensitive) ? 60 : 1;
      const int minutes = duration.captured(1).toInt() * multiplier;
      TextSpan span{.start = duration.capturedStart(),
                    .length = duration.capturedLength(),
                    .id = QStringLiteral("duration:%1:%2").arg(duration.capturedStart()).arg(duration.capturedLength()),
                    .label = QStringLiteral("%1 minutes").arg(minutes)};
      if (minutes >= 1 && minutes <= 1'440 && !isDisabled(request.disabledRecognitionIds, span.id)) {
        result.eventDurationMinutes = minutes;
        addSpan(spans, request.disabledRecognitionIds, std::move(span), result.recognitions);
      }
    }
  }

  const std::optional<DateMatch> date = firstDateMatch(request.text, now.date());
  if (date.has_value()) {
    TextSpan span{.start = date->start,
                  .length = date->length,
                  .id = QStringLiteral("date:%1:%2").arg(date->start).arg(date->length),
                  .label = date->date.toString(Qt::ISODate)};
    if (!isDisabled(request.disabledRecognitionIds, span.id)) {
      result.date = date->date;
      addSpan(spans, request.disabledRecognitionIds, std::move(span), result.recognitions);
    }
  }

  const std::optional<TimeMatch> time = firstTimeMatch(request.text);
  if (time.has_value()) {
    TextSpan span{.start = time->start,
                  .length = time->length,
                  .id = QStringLiteral("time:%1:%2").arg(time->start).arg(time->length),
                  .label = time->time.toString(QStringLiteral("HH:mm")),
                  .removable = result.kind == QuickCaptureKind::Event};
    if (!isDisabled(request.disabledRecognitionIds, span.id)) {
      result.time = time->time;
      if (result.kind == QuickCaptureKind::Task) {
        result.recognitions.append({.id = span.id,
                                    .label = QStringLiteral("%1 remains in task title").arg(span.label),
                                    .removable = false});
      } else {
        addSpan(spans, request.disabledRecognitionIds, std::move(span), result.recognitions);
      }
    }
  }

  if (result.kind == QuickCaptureKind::Task && result.recurrence.enabled && !result.date.has_value()) {
    result.date = now.date();
  }
  if (result.kind == QuickCaptureKind::Event && result.time.has_value() && !result.date.has_value()) {
    QDate dateForTime = now.date();
    if (*result.time <= now.time()) {
      dateForTime = dateForTime.addDays(1);
    }
    result.date = dateForTime;
  }
  result.allDay = result.kind == QuickCaptureKind::Event && result.date.has_value() && !result.time.has_value();
  result.eventReady = result.kind != QuickCaptureKind::Event || result.date.has_value();
  result.parsedTitle = removeSpans(request.text, spans);
  return result;
}

} // namespace hcb
