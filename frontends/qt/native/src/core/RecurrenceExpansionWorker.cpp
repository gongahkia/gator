#include "core/RecurrenceExpansionWorker.h"

#include <QDate>
#include <QDateTime>
#include <QHash>
#include <QRegularExpression>
#include <QSet>
#include <QTime>
#include <QTimeZone>

#include <algorithm>
#include <cstdint>
#include <future>
#include <limits>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr int kMaximumOccurrences = 366;
constexpr int kMaximumExpansionIterations = 200'000;
constexpr int kMaximumRangeOccurrences = 5'000;
constexpr int kMaximumInterval = 366;
constexpr int kMaximumMonthDay = 31;
constexpr int kMaximumWeekdayPosition = 5;

enum class RecurrenceFrequency : std::uint8_t {
  Daily,
  Weekly,
  Monthly,
  Yearly
};

struct ParsedRecurrenceRule final {
  RecurrenceFrequency frequency;
  int interval{1};
  QList<int> byDay;
  std::optional<int> byMonthDay;
  std::optional<int> bySetPosition;
  std::optional<int> count;
  std::optional<QDateTime> until;
};

[[nodiscard]] int positiveBoundedInteger(const QString& text, int maximum) {
  bool converted = false;
  const int value = text.toInt(&converted);
  return converted ? std::clamp(value, 1, maximum) : 1;
}

[[nodiscard]] int boundedWeekdayPosition(const QString& text) {
  bool converted = false;
  const int value = text.toInt(&converted);
  if (!converted || value == 0) {
    return 1;
  }
  return std::clamp(value, -kMaximumWeekdayPosition, kMaximumWeekdayPosition);
}

[[nodiscard]] int weekdayCode(const QDateTime& value) { return value.date().dayOfWeek() % 7; }

[[nodiscard]] std::optional<int> weekdayCode(const QString& value) {
  if (value == QStringLiteral("SU")) {
    return 0;
  }
  if (value == QStringLiteral("MO")) {
    return 1;
  }
  if (value == QStringLiteral("TU")) {
    return 2;
  }
  if (value == QStringLiteral("WE")) {
    return 3;
  }
  if (value == QStringLiteral("TH")) {
    return 4;
  }
  if (value == QStringLiteral("FR")) {
    return 5;
  }
  if (value == QStringLiteral("SA")) {
    return 6;
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<QDateTime> parseUntil(const QString& value) {
  static const QRegularExpression dateTimePattern(
      QStringLiteral("^(\\d{4})(\\d{2})(\\d{2})T(\\d{2})(\\d{2})(\\d{2})Z$"));
  static const QRegularExpression datePattern(QStringLiteral("^(\\d{4})(\\d{2})(\\d{2})$"));
  const QRegularExpressionMatch dateTimeMatch = dateTimePattern.match(value);
  const QRegularExpressionMatch dateMatch = datePattern.match(value);
  const QRegularExpressionMatch match = dateTimeMatch.hasMatch() ? dateTimeMatch : dateMatch;
  if (!match.hasMatch()) {
    return std::nullopt;
  }
  bool yearConverted = false;
  bool monthConverted = false;
  bool dayConverted = false;
  const int year = match.captured(1).toInt(&yearConverted);
  const int month = match.captured(2).toInt(&monthConverted);
  const int day = match.captured(3).toInt(&dayConverted);
  const QDate date(year, month, day);
  if (!yearConverted || !monthConverted || !dayConverted || !date.isValid()) {
    return std::nullopt;
  }
  if (!dateTimeMatch.hasMatch()) {
    return QDateTime(date, QTime(23, 59, 59), QTimeZone::utc());
  }
  bool hourConverted = false;
  bool minuteConverted = false;
  bool secondConverted = false;
  const QTime time(match.captured(4).toInt(&hourConverted),
                   match.captured(5).toInt(&minuteConverted),
                   match.captured(6).toInt(&secondConverted));
  return hourConverted && minuteConverted && secondConverted && time.isValid()
             ? std::optional<QDateTime>(QDateTime(date, time, QTimeZone::utc()))
             : std::nullopt;
}

[[nodiscard]] std::optional<ParsedRecurrenceRule>
parseRecurrenceRule(const std::optional<QString>& recurrenceRule) {
  if (!recurrenceRule.has_value()) {
    return std::nullopt;
  }
  QString line;
  for (const QString& candidate : recurrenceRule->split(u'\n')) {
    const QString trimmed = candidate.trimmed();
    if (trimmed.startsWith(QStringLiteral("RRULE:"))) {
      line = trimmed;
      break;
    }
  }
  if (line.isEmpty()) {
    return std::nullopt;
  }
  QHash<QString, QString> parts;
  for (const QString& field : line.sliced(6).split(u';', Qt::SkipEmptyParts)) {
    const qsizetype separator = field.indexOf(u'=');
    if (separator <= 0) {
      continue;
    }
    parts.insert(field.first(separator), field.sliced(separator + 1));
  }
  const QString frequency = parts.value(QStringLiteral("FREQ"));
  std::optional<RecurrenceFrequency> parsedFrequency;
  if (frequency == QStringLiteral("DAILY")) {
    parsedFrequency = RecurrenceFrequency::Daily;
  } else if (frequency == QStringLiteral("WEEKLY")) {
    parsedFrequency = RecurrenceFrequency::Weekly;
  } else if (frequency == QStringLiteral("MONTHLY")) {
    parsedFrequency = RecurrenceFrequency::Monthly;
  } else if (frequency == QStringLiteral("YEARLY")) {
    parsedFrequency = RecurrenceFrequency::Yearly;
  }
  if (!parsedFrequency.has_value()) {
    return std::nullopt;
  }
  ParsedRecurrenceRule parsed{.frequency = *parsedFrequency};
  if (parts.contains(QStringLiteral("INTERVAL"))) {
    parsed.interval =
        positiveBoundedInteger(parts.value(QStringLiteral("INTERVAL")), kMaximumInterval);
  }
  if (parts.contains(QStringLiteral("BYDAY"))) {
    for (QString day : parts.value(QStringLiteral("BYDAY")).split(u',')) {
      day.remove(QRegularExpression(QStringLiteral("^[+-]?\\d+")));
      const std::optional<int> code = weekdayCode(day);
      if (code.has_value() && !parsed.byDay.contains(*code)) {
        parsed.byDay.append(*code);
      }
    }
  }
  if (parts.contains(QStringLiteral("BYMONTHDAY"))) {
    parsed.byMonthDay =
        positiveBoundedInteger(parts.value(QStringLiteral("BYMONTHDAY")), kMaximumMonthDay);
  }
  if (parts.contains(QStringLiteral("BYSETPOS"))) {
    parsed.bySetPosition = boundedWeekdayPosition(parts.value(QStringLiteral("BYSETPOS")));
  }
  if (parts.contains(QStringLiteral("COUNT"))) {
    parsed.count =
        positiveBoundedInteger(parts.value(QStringLiteral("COUNT")), kMaximumExpansionIterations);
  }
  if (parts.contains(QStringLiteral("UNTIL"))) {
    parsed.until = parseUntil(parts.value(QStringLiteral("UNTIL")));
  }
  return parsed;
}

[[nodiscard]] std::optional<QDateTime> parseUtcDateTime(const QString& value) {
  if (value.size() > 64 || !value.contains(u'T')) {
    return std::nullopt;
  }
  const QDateTime parsed = QDateTime::fromString(value, Qt::ISODateWithMs);
  return parsed.isValid() ? std::optional<QDateTime>(parsed.toUTC()) : std::nullopt;
}

[[nodiscard]] bool isValidEventId(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= 256 &&
         !value.contains(QChar::Null);
}

[[nodiscard]] QDateTime addMonthsWithRollover(const QDateTime& value, int months) {
  const QDate date = value.date();
  const qint64 monthIndex =
      static_cast<qint64>(date.year()) * 12 + static_cast<qint64>(date.month() - 1) + months;
  const qint64 year = monthIndex / 12;
  const int month = static_cast<int>(monthIndex % 12) + 1;
  if (year < std::numeric_limits<int>::min() || year > std::numeric_limits<int>::max()) {
    return {};
  }
  const QDate target(static_cast<int>(year), month, 1);
  return target.isValid()
             ? QDateTime(target.addDays(date.day() - 1), value.time(), value.timeZone())
             : QDateTime{};
}

[[nodiscard]] QDateTime addYearsWithRollover(const QDateTime& value, int years) {
  const QDate date = value.date();
  const qint64 year = static_cast<qint64>(date.year()) + years;
  if (year < std::numeric_limits<int>::min() || year > std::numeric_limits<int>::max()) {
    return {};
  }
  const QDate target(static_cast<int>(year), date.month(), 1);
  return target.isValid()
             ? QDateTime(target.addDays(date.day() - 1), value.time(), value.timeZone())
             : QDateTime{};
}

[[nodiscard]] QTimeZone recurrenceTimeZone(const std::optional<QString>& value, bool allDay) {
  if (allDay || !value.has_value()) {
    return QTimeZone::utc();
  }
  const QTimeZone timeZone(value->toUtf8());
  return timeZone.isValid() ? timeZone : QTimeZone::utc();
}

[[nodiscard]] QString recurrencePropertyName(const QString& line) {
  const qsizetype separator = line.indexOf(u':');
  return separator > 0 ? line.first(separator).section(u';', 0, 0) : QString();
}

[[nodiscard]] std::optional<QTimeZone> recurrencePropertyTimeZone(const QString& line,
                                                                   const QTimeZone& fallback) {
  const qsizetype separator = line.indexOf(u':');
  if (separator <= 0) {
    return std::nullopt;
  }
  for (const QString& parameter : line.first(separator).split(u';').sliced(1)) {
    const qsizetype equals = parameter.indexOf(u'=');
    if (equals <= 0 || equals == parameter.size() - 1) {
      return std::nullopt;
    }
    if (parameter.first(equals) == QStringLiteral("TZID")) {
      const QTimeZone timeZone(parameter.sliced(equals + 1).toUtf8());
      return timeZone.isValid() ? std::optional<QTimeZone>(timeZone) : std::nullopt;
    }
  }
  return fallback;
}

[[nodiscard]] std::optional<QDateTime> recurrenceDateTime(QStringView value,
                                                           const QTimeZone& timeZone) {
  static const QRegularExpression datePattern(QStringLiteral("^\\d{8}$"));
  static const QRegularExpression localDateTimePattern(QStringLiteral("^\\d{8}T\\d{6}$"));
  if (!datePattern.matchView(value).hasMatch() &&
      !localDateTimePattern.matchView(value).hasMatch() &&
      !QRegularExpression(QStringLiteral("^\\d{8}T\\d{6}Z$")).matchView(value).hasMatch()) {
    return std::nullopt;
  }
  const QDate date = QDate::fromString(value.first(8).toString(), QStringLiteral("yyyyMMdd"));
  if (!date.isValid()) {
    return std::nullopt;
  }
  if (value.size() == 8) {
    return QDateTime(date, QTime(0, 0), timeZone).toUTC();
  }
  bool hourConverted = false;
  bool minuteConverted = false;
  bool secondConverted = false;
  const QTime time(value.sliced(9, 2).toInt(&hourConverted),
                   value.sliced(11, 2).toInt(&minuteConverted),
                   value.sliced(13, 2).toInt(&secondConverted));
  if (!hourConverted || !minuteConverted || !secondConverted || !time.isValid()) {
    return std::nullopt;
  }
  const QTimeZone zone = value.endsWith(u'Z') ? QTimeZone::utc() : timeZone;
  return QDateTime(date, time, zone).toUTC();
}

[[nodiscard]] std::optional<QList<QDateTime>> recurrenceDates(const std::optional<QString>& rule,
                                                               QStringView property,
                                                               const QTimeZone& fallback) {
  QList<QDateTime> values;
  if (!rule.has_value()) {
    return values;
  }
  for (const QString& rawLine : rule->split(u'\n', Qt::SkipEmptyParts)) {
    const QString line = rawLine.trimmed();
    if (recurrencePropertyName(line) != property) {
      continue;
    }
    const qsizetype separator = line.indexOf(u':');
    const std::optional<QTimeZone> timeZone = recurrencePropertyTimeZone(line, fallback);
    if (!timeZone.has_value()) {
      return std::nullopt;
    }
    for (const QString& rawValue : line.sliced(separator + 1).split(u',', Qt::SkipEmptyParts)) {
      const std::optional<QDateTime> parsed = recurrenceDateTime(rawValue, *timeZone);
      if (!parsed.has_value() || values.size() >= kMaximumRangeOccurrences) {
        return std::nullopt;
      }
      values.append(*parsed);
    }
  }
  return values;
}

[[nodiscard]] bool
matchesRecurrenceMonth(const QDateTime& seriesStart, const QDateTime& candidate, int interval) {
  const QDate start = seriesStart.date();
  const QDate date = candidate.date();
  const int months = (date.year() - start.year()) * 12 + date.month() - start.month();
  return months >= 0 && months % interval == 0;
}

[[nodiscard]] bool matchesWeekdayPosition(const QDateTime& candidate, int position) {
  const QDate date = candidate.date();
  if (position > 0) {
    return (date.day() - 1) / 7 + 1 == position;
  }
  QDate nextWeek = date.addDays(7);
  int fromEnd = 1;
  while (nextWeek.month() == date.month()) {
    ++fromEnd;
    nextWeek = nextWeek.addDays(7);
  }
  return -fromEnd == position;
}

[[nodiscard]] bool matchesMonthlyRule(const QDateTime& candidate,
                                      const ParsedRecurrenceRule& rule,
                                      const QDateTime& seriesStart) {
  if (!matchesRecurrenceMonth(seriesStart, candidate, rule.interval)) {
    return false;
  }
  if (rule.byMonthDay.has_value()) {
    return candidate.date().day() == *rule.byMonthDay;
  }
  if (rule.bySetPosition.has_value() && !rule.byDay.isEmpty()) {
    return rule.byDay.contains(weekdayCode(candidate)) &&
           matchesWeekdayPosition(candidate, *rule.bySetPosition);
  }
  return candidate.date().day() == seriesStart.date().day();
}

[[nodiscard]] bool
matchesRecurrenceWeek(const QDateTime& seriesStart, const QDateTime& candidate, int interval) {
  const QDate startWeek = seriesStart.date().addDays(-(seriesStart.date().dayOfWeek() % 7));
  const QDate candidateWeek = candidate.date().addDays(-(candidate.date().dayOfWeek() % 7));
  const qint64 weeks = startWeek.daysTo(candidateWeek) / 7;
  return weeks >= 0 && weeks % interval == 0;
}

[[nodiscard]] QDateTime nextWeeklyByDay(const QDateTime& current,
                                        const ParsedRecurrenceRule& rule,
                                        const QDateTime& seriesStart) {
  QDateTime next = current;
  for (int offset = 1; offset <= rule.interval * 7 + 7; ++offset) {
    next = next.addDays(1);
    if (rule.byDay.contains(weekdayCode(next)) &&
        matchesRecurrenceWeek(seriesStart, next, rule.interval)) {
      return next;
    }
  }
  return current.addDays(static_cast<qint64>(rule.interval) * 7);
}

[[nodiscard]] QDateTime nextMonthlyRuleDate(const QDateTime& current,
                                            const ParsedRecurrenceRule& rule,
                                            const QDateTime& seriesStart) {
  QDateTime next = current;
  for (int offset = 1; offset <= 370; ++offset) {
    next = next.addDays(1);
    next.setTime(seriesStart.time());
    if (matchesMonthlyRule(next, rule, seriesStart)) {
      return next;
    }
  }
  return addMonthsWithRollover(current, rule.interval);
}

[[nodiscard]] QDateTime firstRecurrenceDate(const QDateTime& start,
                                            const ParsedRecurrenceRule& rule) {
  if (rule.frequency == RecurrenceFrequency::Monthly &&
      (rule.byMonthDay.has_value() || rule.bySetPosition.has_value())) {
    return matchesMonthlyRule(start, rule, start)
               ? start
               : nextMonthlyRuleDate(start.addDays(-1), rule, start);
  }
  if (rule.frequency != RecurrenceFrequency::Weekly || rule.byDay.isEmpty() ||
      rule.byDay.contains(weekdayCode(start))) {
    return start;
  }
  return nextWeeklyByDay(start.addDays(-1), rule, start);
}

[[nodiscard]] QDateTime nextRecurrenceDate(const QDateTime& current,
                                           const ParsedRecurrenceRule& rule,
                                           const QDateTime& seriesStart) {
  if (rule.frequency == RecurrenceFrequency::Weekly && !rule.byDay.isEmpty()) {
    return nextWeeklyByDay(current, rule, seriesStart);
  }
  if (rule.frequency == RecurrenceFrequency::Monthly &&
      (rule.byMonthDay.has_value() || rule.bySetPosition.has_value())) {
    return nextMonthlyRuleDate(current, rule, seriesStart);
  }
  switch (rule.frequency) {
  case RecurrenceFrequency::Daily:
    return current.addDays(rule.interval);
  case RecurrenceFrequency::Weekly:
    return current.addDays(static_cast<qint64>(rule.interval) * 7);
  case RecurrenceFrequency::Monthly:
    return addMonthsWithRollover(current, rule.interval);
  case RecurrenceFrequency::Yearly:
    return addYearsWithRollover(current, rule.interval);
  }
  return {};
}

[[nodiscard]] RecurrenceExpansionResult expandStored(const RecurrenceExpansionRequest& request,
                                                     const CancellationToken& cancellation) {
  if (cancellation.stop_requested()) {
    return AppError(AppErrorCode::Cancelled, QStringLiteral("Recurrence expansion was cancelled"));
  }
  if (!isValidEventId(request.eventId)) {
    return AppError(AppErrorCode::Validation,
                    QStringLiteral("Recurrence event identifier is invalid"));
  }
  const std::optional<QDateTime> start = parseUtcDateTime(request.startAt);
  const std::optional<QDateTime> end = parseUtcDateTime(request.endAt);
  if (!start.has_value() || !end.has_value() || *end <= *start) {
    return AppError(AppErrorCode::Validation, QStringLiteral("Recurrence event range is invalid"));
  }
  const RecurrenceOccurrence single{.id = request.eventId,
                                    .startAt = start->toString(Qt::ISODateWithMs),
                                    .endAt = end->toString(Qt::ISODateWithMs),
                                    .originalStartAt = std::nullopt};
  const std::optional<ParsedRecurrenceRule> rule = parseRecurrenceRule(request.recurrenceRule);
  if (!rule.has_value()) {
    return QList<RecurrenceOccurrence>{single};
  }
  const QTimeZone timeZone = recurrenceTimeZone(request.timeZone, request.allDay);
  const QDateTime recurrenceStart = start->toTimeZone(timeZone);
  const bool hasRange = request.rangeStartAt.has_value() || request.rangeEndAt.has_value();
  const std::optional<QDateTime> rangeStart =
      request.rangeStartAt.has_value() ? parseUtcDateTime(*request.rangeStartAt) : std::nullopt;
  const std::optional<QDateTime> rangeEnd =
      request.rangeEndAt.has_value() ? parseUtcDateTime(*request.rangeEndAt) : std::nullopt;
  if (hasRange && (!rangeStart.has_value() || !rangeEnd.has_value() || *rangeEnd <= *rangeStart)) {
    return AppError(AppErrorCode::Validation, QStringLiteral("Recurrence display range is invalid"));
  }
  const qint64 duration = start->msecsTo(*end);
  const std::optional<QList<QDateTime>> exceptionDates =
      recurrenceDates(request.recurrenceRule, u"EXDATE", timeZone);
  const std::optional<QList<QDateTime>> additionalDates =
      recurrenceDates(request.recurrenceRule, u"RDATE", timeZone);
  if (!exceptionDates.has_value() || !additionalDates.has_value()) {
    return AppError(AppErrorCode::Validation,
                    QStringLiteral("Recurrence exception date is invalid"));
  }
  QSet<QString> excludedStarts;
  for (const QDateTime& exception : *exceptionDates) {
    excludedStarts.insert(exception.toUTC().toString(Qt::ISODateWithMs));
  }
  if (request.recurrenceRule.has_value()) {
    for (const QString& rawLine : request.recurrenceRule->split(u'\n', Qt::SkipEmptyParts)) {
      const QString line = rawLine.trimmed();
      if (recurrencePropertyName(line) != QStringLiteral("EXRULE")) {
        continue;
      }
      const qsizetype separator = line.indexOf(u':');
      const RecurrenceExpansionRequest exclusionRequest{
          .eventId = request.eventId,
          .startAt = request.startAt,
          .endAt = request.endAt,
          .allDay = request.allDay,
          .timeZone = request.timeZone,
          .recurrenceRule = QStringLiteral("RRULE:") + line.sliced(separator + 1),
          .rangeStartAt = request.rangeStartAt,
          .rangeEndAt = request.rangeEndAt};
      const RecurrenceExpansionResult exclusion = expandStored(exclusionRequest, cancellation);
      if (!std::holds_alternative<QList<RecurrenceOccurrence>>(exclusion)) {
        return std::get<AppError>(exclusion);
      }
      for (const RecurrenceOccurrence& occurrence :
           std::get<QList<RecurrenceOccurrence>>(exclusion)) {
        if (occurrence.originalStartAt.has_value()) {
          excludedStarts.insert(*occurrence.originalStartAt);
        }
      }
    }
  }
  const auto withinRange = [&rangeStart, &rangeEnd, duration](const QDateTime& occurrenceStart) {
    const QDateTime utcStart = occurrenceStart.toUTC();
    return (!rangeStart.has_value() || utcStart.addMSecs(duration) > *rangeStart) &&
           (!rangeEnd.has_value() || utcStart < *rangeEnd);
  };
  const int maximum = rule->count.value_or(hasRange ? kMaximumExpansionIterations : kMaximumOccurrences);
  QDateTime until = rule->until.value_or(
      hasRange ? *rangeEnd : start->addDays(kMaximumOccurrences));
  if (rangeEnd.has_value() && *rangeEnd < until) {
    until = *rangeEnd;
  }
  QList<QDateTime> starts;
  starts.reserve(std::min(maximum, hasRange ? kMaximumRangeOccurrences : kMaximumOccurrences));
  QSet<QString> representedStarts;
  QDateTime cursor = firstRecurrenceDate(recurrenceStart, *rule);
  for (int index = 0; index < maximum && cursor <= until; ++index) {
    if (cancellation.stop_requested()) {
      return AppError(AppErrorCode::Cancelled,
                      QStringLiteral("Recurrence expansion was cancelled"));
    }
    if (!cursor.isValid()) {
      return AppError(AppErrorCode::Validation,
                      QStringLiteral("Recurrence rule exceeds date range"));
    }
    if (!withinRange(cursor)) {
      cursor = nextRecurrenceDate(cursor, *rule, recurrenceStart);
      continue;
    }
    const QString occurrenceStart = cursor.toUTC().toString(Qt::ISODateWithMs);
    if (!excludedStarts.contains(occurrenceStart) && !representedStarts.contains(occurrenceStart)) {
      starts.append(cursor.toUTC());
      representedStarts.insert(occurrenceStart);
    }
    if (starts.size() > kMaximumRangeOccurrences) {
      return AppError(AppErrorCode::Validation,
                      QStringLiteral("Recurrence produces too many visible occurrences"));
    }
    cursor = nextRecurrenceDate(cursor, *rule, recurrenceStart);
  }
  for (const QDateTime& additional : *additionalDates) {
    const QString occurrenceStart = additional.toUTC().toString(Qt::ISODateWithMs);
    if (withinRange(additional) && !excludedStarts.contains(occurrenceStart) &&
        !representedStarts.contains(occurrenceStart)) {
      starts.append(additional.toUTC());
      representedStarts.insert(occurrenceStart);
    }
  }
  std::sort(starts.begin(), starts.end());
  QList<RecurrenceOccurrence> occurrences;
  occurrences.reserve(starts.size());
  const QString masterStart = start->toString(Qt::ISODateWithMs);
  for (const QDateTime& occurrenceStartAt : starts) {
    const QString occurrenceStart = occurrenceStartAt.toString(Qt::ISODateWithMs);
    QString suffix = occurrenceStart;
    if (request.allDay) {
      suffix = suffix.left(10).remove(u'-');
    } else {
      suffix.remove(u'-').remove(u':').remove(QStringLiteral(".000"));
    }
    occurrences.append({.id = occurrenceStart == masterStart
                                 ? request.eventId
                                 : request.eventId + QStringLiteral(":instance:") + suffix,
                        .startAt = occurrenceStart,
                        .endAt = occurrenceStartAt.addMSecs(duration).toUTC().toString(Qt::ISODateWithMs),
                        .originalStartAt = occurrenceStart});
  }
  return occurrences;
}

} // namespace

std::future<RecurrenceExpansionResult>
RecurrenceExpansionWorker::expand(RecurrenceExpansionRequest request,
                                  const CancellationToken& cancellation) const {
  return std::async(std::launch::async, [request = std::move(request), cancellation]() {
    return expandStored(request, cancellation);
  });
}

} // namespace hcb
