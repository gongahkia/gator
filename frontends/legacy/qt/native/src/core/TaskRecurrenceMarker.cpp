#include "core/TaskRecurrenceMarker.h"

#include <QDate>
#include <QHash>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonParseError>
#include <QJsonValue>
#include <QSet>
#include <QTimeZone>
#include <QUuid>

#include <cmath>
#include <limits>

namespace hcb {
namespace {

constexpr qsizetype kMaximumNotesBytes = 8'192;
constexpr qsizetype kMaximumTitleLength = 500;
constexpr qsizetype kMaximumTimeZoneLength = 128;
constexpr std::int32_t kMaximumInterval = 1'000;
constexpr std::int32_t kMaximumOccurrenceCount = 10'000;
constexpr qsizetype kMaximumRuleLength = 512;
constexpr int kMaximumExpansionYears = 1'000;
constexpr char kMarkerPrefix[] = "[HCB-RECURRENCE v";
constexpr char kMarkerSuffix[] = "\n[/HCB-RECURRENCE]";

[[nodiscard]] QString malformed(QString diagnostic) {
  return QStringLiteral("HCB recurrence marker is malformed: ") + std::move(diagnostic);
}

[[nodiscard]] bool isValidDate(const QString& value) {
  return value.size() == 10 && QDate::fromString(value, Qt::ISODate).isValid();
}

[[nodiscard]] bool isValidSeriesId(const QString& value) {
  const QUuid parsed(value);
  return !parsed.isNull() && parsed.toString(QUuid::WithoutBraces) == value;
}

[[nodiscard]] bool isValidPriority(const QString& value) {
  return value == QStringLiteral("none") || value == QStringLiteral("low") ||
         value == QStringLiteral("medium") || value == QStringLiteral("high");
}

[[nodiscard]] QString frequencyText(TaskRecurrenceFrequency frequency) {
  switch (frequency) {
  case TaskRecurrenceFrequency::Daily:
    return QStringLiteral("daily");
  case TaskRecurrenceFrequency::Weekly:
    return QStringLiteral("weekly");
  case TaskRecurrenceFrequency::Monthly:
    return QStringLiteral("monthly");
  case TaskRecurrenceFrequency::Yearly:
    return QStringLiteral("yearly");
  }
  return {};
}

[[nodiscard]] std::optional<TaskRecurrenceFrequency> frequencyForText(const QString& value) {
  if (value == QStringLiteral("daily")) {
    return TaskRecurrenceFrequency::Daily;
  }
  if (value == QStringLiteral("weekly")) {
    return TaskRecurrenceFrequency::Weekly;
  }
  if (value == QStringLiteral("monthly")) {
    return TaskRecurrenceFrequency::Monthly;
  }
  if (value == QStringLiteral("yearly")) {
    return TaskRecurrenceFrequency::Yearly;
  }
  return std::nullopt;
}

struct DateOnlyRule final {
  TaskRecurrenceFrequency frequency;
  std::int32_t interval{1};
  QSet<int> weekdays;
  QHash<int, QSet<int>> ordinalWeekdays;
  QSet<int> monthDays;
  QSet<int> months;
};

[[nodiscard]] std::optional<int> weekdayForToken(QStringView value) {
  if (value == u"MO") return 1;
  if (value == u"TU") return 2;
  if (value == u"WE") return 3;
  if (value == u"TH") return 4;
  if (value == u"FR") return 5;
  if (value == u"SA") return 6;
  if (value == u"SU") return 7;
  return std::nullopt;
}

[[nodiscard]] std::optional<DateOnlyRule> parseDateOnlyRule(const QString& text) {
  if (text.isEmpty()) {
    return std::nullopt;
  }
  if (text.size() > kMaximumRuleLength || text != text.trimmed() || text.contains(QChar::Null)) {
    return std::nullopt;
  }
  QHash<QString, QString> parts;
  for (const QString& component : text.split(u';', Qt::SkipEmptyParts)) {
    const qsizetype separator = component.indexOf(u'=');
    if (separator <= 0 || separator == component.size() - 1) {
      return std::nullopt;
    }
    const QString key = component.left(separator);
    const QString value = component.sliced(separator + 1);
    if (key != key.toUpper() || value != value.toUpper() || parts.contains(key) ||
        (key != QStringLiteral("FREQ") && key != QStringLiteral("INTERVAL") &&
         key != QStringLiteral("BYDAY") && key != QStringLiteral("BYMONTHDAY") &&
         key != QStringLiteral("BYMONTH"))) {
      return std::nullopt;
    }
    parts.insert(key, value);
  }
  const auto frequency = parts.constFind(QStringLiteral("FREQ"));
  if (frequency == parts.cend()) {
    return std::nullopt;
  }
  const std::optional<TaskRecurrenceFrequency> parsedFrequency =
      frequencyForText(frequency->toLower());
  if (!parsedFrequency.has_value()) {
    return std::nullopt;
  }
  DateOnlyRule rule{.frequency = *parsedFrequency};
  if (const auto interval = parts.constFind(QStringLiteral("INTERVAL")); interval != parts.cend()) {
    bool valid = false;
    const int parsed = interval->toInt(&valid);
    if (!valid || parsed < 1 || parsed > kMaximumInterval) {
      return std::nullopt;
    }
    rule.interval = parsed;
  }
  if (const auto days = parts.constFind(QStringLiteral("BYDAY")); days != parts.cend()) {
    for (const QString& token : days->split(u',', Qt::SkipEmptyParts)) {
      const QString suffix = token.right(2);
      const std::optional<int> weekday = weekdayForToken(suffix);
      if (!weekday.has_value()) {
        return std::nullopt;
      }
      const QString ordinalText = token.left(token.size() - 2);
      if (ordinalText.isEmpty()) {
        rule.weekdays.insert(*weekday);
      } else {
        bool valid = false;
        const int ordinal = ordinalText.toInt(&valid);
        if (!valid || ordinal == 0 || ordinal < -5 || ordinal > 5) {
          return std::nullopt;
        }
        rule.ordinalWeekdays[ordinal].insert(*weekday);
      }
    }
  }
  if (const auto days = parts.constFind(QStringLiteral("BYMONTHDAY")); days != parts.cend()) {
    for (const QString& token : days->split(u',', Qt::SkipEmptyParts)) {
      bool valid = false;
      const int day = token.toInt(&valid);
      if (!valid || day == 0 || day < -31 || day > 31) {
        return std::nullopt;
      }
      rule.monthDays.insert(day);
    }
  }
  if (const auto months = parts.constFind(QStringLiteral("BYMONTH")); months != parts.cend()) {
    for (const QString& token : months->split(u',', Qt::SkipEmptyParts)) {
      bool valid = false;
      const int month = token.toInt(&valid);
      if (!valid || month < 1 || month > 12) {
        return std::nullopt;
      }
      rule.months.insert(month);
    }
  }
  return rule;
}

[[nodiscard]] bool isValidDateList(const QList<QString>& values) {
  if (values.size() > kMaximumOccurrenceCount) {
    return false;
  }
  QSet<QString> unique;
  for (const QString& value : values) {
    if (!isValidDate(value) || unique.contains(value)) {
      return false;
    }
    unique.insert(value);
  }
  return true;
}

[[nodiscard]] QString endKindText(TaskRecurrenceEndKind kind) {
  switch (kind) {
  case TaskRecurrenceEndKind::Never:
    return QStringLiteral("never");
  case TaskRecurrenceEndKind::Until:
    return QStringLiteral("until");
  case TaskRecurrenceEndKind::Count:
    return QStringLiteral("count");
  }
  return {};
}

[[nodiscard]] std::optional<TaskRecurrenceEndKind> endKindForText(const QString& value) {
  if (value == QStringLiteral("never")) {
    return TaskRecurrenceEndKind::Never;
  }
  if (value == QStringLiteral("until")) {
    return TaskRecurrenceEndKind::Until;
  }
  if (value == QStringLiteral("count")) {
    return TaskRecurrenceEndKind::Count;
  }
  return std::nullopt;
}

[[nodiscard]] bool hasKeys(const QJsonObject& object, const QStringList& keys) {
  if (object.size() != keys.size()) {
    return false;
  }
  for (const QString& key : keys) {
    if (!object.contains(key)) {
      return false;
    }
  }
  return true;
}

[[nodiscard]] std::optional<QString> requiredString(const QJsonObject& object, const QString& key) {
  const QJsonValue value = object.value(key);
  return value.isString() && !value.toString().contains(QChar::Null)
             ? std::optional<QString>(value.toString())
             : std::nullopt;
}

[[nodiscard]] std::optional<std::int32_t> requiredInteger(const QJsonObject& object,
                                                          const QString& key) {
  const QJsonValue value = object.value(key);
  if (!value.isDouble()) {
    return std::nullopt;
  }
  const double number = value.toDouble();
  if (!std::isfinite(number) || std::floor(number) != number ||
      number < static_cast<double>(std::numeric_limits<std::int32_t>::min()) ||
      number > static_cast<double>(std::numeric_limits<std::int32_t>::max())) {
    return std::nullopt;
  }
  return static_cast<std::int32_t>(number);
}

[[nodiscard]] std::optional<QString> validate(const TaskRecurrenceMarker& marker) {
  if (!isValidSeriesId(marker.seriesId)) {
    return QStringLiteral("series identifier is invalid");
  }
  if (marker.ordinal < 0 || marker.occurrenceId != marker.seriesId + QStringLiteral(":") +
                                                       QString::number(marker.ordinal)) {
    return QStringLiteral("occurrence identity is invalid");
  }
  if (marker.interval < 1 || marker.interval > kMaximumInterval ||
      frequencyText(marker.frequency).isEmpty()) {
    return QStringLiteral("rule is invalid");
  }
  if (!isValidDate(marker.anchorDate) || !isValidDate(marker.templateDueDate) ||
      marker.templateTitle.isEmpty() || marker.templateTitle != marker.templateTitle.trimmed() ||
      marker.templateTitle.size() > kMaximumTitleLength ||
      marker.templateTitle.contains(QChar::Null) || !isValidPriority(marker.templatePriority)) {
    return QStringLiteral("template is invalid");
  }
  if (marker.timeZone.isEmpty() || marker.timeZone.size() > kMaximumTimeZoneLength ||
      marker.timeZone != marker.timeZone.trimmed() || marker.timeZone.contains(QChar::Null) ||
      !QTimeZone(marker.timeZone.toUtf8()).isValid()) {
    return QStringLiteral("timezone is invalid");
  }
  if (!marker.recurrenceRule.isEmpty()) {
    const std::optional<DateOnlyRule> rule = parseDateOnlyRule(marker.recurrenceRule);
    if (!rule.has_value() || rule->frequency != marker.frequency || rule->interval != marker.interval) {
      return QStringLiteral("date-only recurrence rule is invalid");
    }
  }
  if (!isValidDateList(marker.exclusionDates) || !isValidDateList(marker.additionDates)) {
    return QStringLiteral("recurrence date exceptions are invalid");
  }
  if (marker.exclusionDates.contains(marker.anchorDate) ||
      std::any_of(marker.additionDates.cbegin(), marker.additionDates.cend(),
                  [&marker](const QString& date) { return date < marker.anchorDate; })) {
    return QStringLiteral("recurrence date exceptions are outside the series");
  }
  switch (marker.end.kind) {
  case TaskRecurrenceEndKind::Never:
    if (marker.end.untilDate.has_value() || marker.end.count.has_value()) {
      return QStringLiteral("never end condition has data");
    }
    return std::nullopt;
  case TaskRecurrenceEndKind::Until:
    if (!marker.end.untilDate.has_value() || marker.end.count.has_value() ||
        !isValidDate(*marker.end.untilDate) || *marker.end.untilDate < marker.anchorDate) {
      return QStringLiteral("until end condition is invalid");
    }
    return std::nullopt;
  case TaskRecurrenceEndKind::Count:
    if (marker.end.untilDate.has_value() || !marker.end.count.has_value() ||
        *marker.end.count < 1 || *marker.end.count > kMaximumOccurrenceCount) {
      return QStringLiteral("count end condition is invalid");
    }
    return std::nullopt;
  }
  return QStringLiteral("end condition is invalid");
}

[[nodiscard]] TaskRecurrenceNotes invalidNotes(const QString& notes, QString diagnostic) {
  return {.state = TaskRecurrenceNotesState::Malformed,
          .userNotes = notes,
          .diagnostic = malformed(std::move(diagnostic))};
}

[[nodiscard]] std::optional<QList<QString>> dateListFromJson(const QJsonValue& value) {
  if (!value.isArray() || value.toArray().size() > kMaximumOccurrenceCount) {
    return std::nullopt;
  }
  QList<QString> dates;
  dates.reserve(value.toArray().size());
  for (const QJsonValue& item : value.toArray()) {
    if (!item.isString() || !isValidDate(item.toString())) {
      return std::nullopt;
    }
    dates.append(item.toString());
  }
  return isValidDateList(dates) ? std::optional<QList<QString>>(std::move(dates)) : std::nullopt;
}

[[nodiscard]] std::optional<TaskRecurrenceMarker> markerFromJson(const QJsonObject& object,
                                                                 QString& diagnostic,
                                                                 bool versionTwo) {
  QStringList keys{QStringLiteral("a"), QStringLiteral("e"), QStringLiteral("i"),
                   QStringLiteral("n"), QStringLiteral("o"), QStringLiteral("r"),
                   QStringLiteral("s"), QStringLiteral("t"), QStringLiteral("z")};
  if (versionTwo) {
    keys.append({QStringLiteral("d"), QStringLiteral("q"), QStringLiteral("x")});
  }
  if (!hasKeys(object, keys)) {
    diagnostic = QStringLiteral("payload fields are invalid");
    return std::nullopt;
  }
  const std::optional<QString> anchorDate = requiredString(object, QStringLiteral("a"));
  const std::optional<std::int32_t> interval = requiredInteger(object, QStringLiteral("i"));
  const std::optional<std::int32_t> ordinal = requiredInteger(object, QStringLiteral("n"));
  const std::optional<QString> occurrenceId = requiredString(object, QStringLiteral("o"));
  const std::optional<QString> frequency = requiredString(object, QStringLiteral("r"));
  const std::optional<QString> seriesId = requiredString(object, QStringLiteral("s"));
  const std::optional<QString> timeZone = requiredString(object, QStringLiteral("z"));
  const QJsonValue endValue = object.value(QStringLiteral("e"));
  const QJsonValue templateValue = object.value(QStringLiteral("t"));
  if (!anchorDate.has_value() || !interval.has_value() || !ordinal.has_value() ||
      !occurrenceId.has_value() || !frequency.has_value() || !seriesId.has_value() ||
      !timeZone.has_value() || !endValue.isObject() || !templateValue.isObject()) {
    diagnostic = QStringLiteral("payload values are invalid");
    return std::nullopt;
  }
  QString recurrenceRule;
  QList<QString> exclusionDates;
  QList<QString> additionDates;
  if (versionTwo) {
    const std::optional<QString> rule = requiredString(object, QStringLiteral("q"));
    const std::optional<QList<QString>> excluded =
        dateListFromJson(object.value(QStringLiteral("x")));
    const std::optional<QList<QString>> added = dateListFromJson(object.value(QStringLiteral("d")));
    if (!rule.has_value() || !excluded.has_value() || !added.has_value()) {
      diagnostic = QStringLiteral("date-only rule fields are invalid");
      return std::nullopt;
    }
    recurrenceRule = *rule;
    exclusionDates = *excluded;
    additionDates = *added;
  }
  const std::optional<TaskRecurrenceFrequency> parsedFrequency = frequencyForText(*frequency);
  const QJsonObject endObject = endValue.toObject();
  const std::optional<QString> endKind = requiredString(endObject, QStringLiteral("k"));
  if (!endKind.has_value()) {
    diagnostic = QStringLiteral("end condition is invalid");
    return std::nullopt;
  }
  const std::optional<TaskRecurrenceEndKind> parsedEndKind = endKindForText(*endKind);
  if (!parsedFrequency.has_value() || !parsedEndKind.has_value()) {
    diagnostic = QStringLiteral("rule or end condition is unsupported");
    return std::nullopt;
  }
  TaskRecurrenceEndCondition end{.kind = *parsedEndKind};
  if (*parsedEndKind == TaskRecurrenceEndKind::Never) {
    if (!hasKeys(endObject, {QStringLiteral("k")})) {
      diagnostic = QStringLiteral("never end condition has extra data");
      return std::nullopt;
    }
  } else if (*parsedEndKind == TaskRecurrenceEndKind::Until) {
    if (!hasKeys(endObject, {QStringLiteral("k"), QStringLiteral("u")})) {
      diagnostic = QStringLiteral("until end condition is invalid");
      return std::nullopt;
    }
    end.untilDate = requiredString(endObject, QStringLiteral("u"));
  } else {
    if (!hasKeys(endObject, {QStringLiteral("c"), QStringLiteral("k")})) {
      diagnostic = QStringLiteral("count end condition is invalid");
      return std::nullopt;
    }
    end.count = requiredInteger(endObject, QStringLiteral("c"));
  }
  const QJsonObject templateObject = templateValue.toObject();
  if (!hasKeys(templateObject, {QStringLiteral("d"), QStringLiteral("p"), QStringLiteral("t")})) {
    diagnostic = QStringLiteral("template fields are invalid");
    return std::nullopt;
  }
  const std::optional<QString> templateDueDate =
      requiredString(templateObject, QStringLiteral("d"));
  const std::optional<QString> templatePriority =
      requiredString(templateObject, QStringLiteral("p"));
  const std::optional<QString> templateTitle = requiredString(templateObject, QStringLiteral("t"));
  if (!templateDueDate.has_value() || !templatePriority.has_value() || !templateTitle.has_value()) {
    diagnostic = QStringLiteral("template values are invalid");
    return std::nullopt;
  }
  TaskRecurrenceMarker marker{.seriesId = *seriesId,
                              .occurrenceId = *occurrenceId,
                              .ordinal = *ordinal,
                              .frequency = *parsedFrequency,
                              .interval = *interval,
                              .anchorDate = *anchorDate,
                              .timeZone = *timeZone,
                              .end = std::move(end),
                              .recurrenceRule = std::move(recurrenceRule),
                              .exclusionDates = std::move(exclusionDates),
                              .additionDates = std::move(additionDates),
                              .templateTitle = *templateTitle,
                              .templateDueDate = *templateDueDate,
                              .templatePriority = *templatePriority};
  if (const std::optional<QString> error = validate(marker); error.has_value()) {
    diagnostic = *error;
    return std::nullopt;
  }
  return marker;
}

[[nodiscard]] bool matchesMonthDay(const QDate& date, const QSet<int>& monthDays) {
  if (monthDays.isEmpty()) {
    return true;
  }
  return monthDays.contains(date.day()) || monthDays.contains(date.day() - date.daysInMonth() - 1);
}

[[nodiscard]] bool matchesOrdinalWeekday(const QDate& date,
                                         const QHash<int, QSet<int>>& ordinalWeekdays) {
  if (ordinalWeekdays.isEmpty()) {
    return true;
  }
  const int weekday = date.dayOfWeek();
  const int fromStart = (date.day() - 1) / 7 + 1;
  const int fromEnd = -((date.daysInMonth() - date.day()) / 7 + 1);
  const auto positive = ordinalWeekdays.constFind(fromStart);
  if (positive != ordinalWeekdays.cend() && positive->contains(weekday)) {
    return true;
  }
  const auto negative = ordinalWeekdays.constFind(fromEnd);
  return negative != ordinalWeekdays.cend() && negative->contains(weekday);
}

[[nodiscard]] bool matchesDateOnlyRule(const QDate& date,
                                       const QDate& anchor,
                                       const DateOnlyRule& rule) {
  if (date < anchor || (!rule.months.isEmpty() && !rule.months.contains(date.month()))) {
    return false;
  }
  const qint64 days = anchor.daysTo(date);
  const qint64 months = static_cast<qint64>(date.year() - anchor.year()) * 12 +
                         date.month() - anchor.month();
  const qint64 years = date.year() - anchor.year();
  switch (rule.frequency) {
  case TaskRecurrenceFrequency::Daily:
    if (days % rule.interval != 0) return false;
    break;
  case TaskRecurrenceFrequency::Weekly: {
    const QDate anchorWeek = anchor.addDays(1 - anchor.dayOfWeek());
    const qint64 weeks = anchorWeek.daysTo(date.addDays(1 - date.dayOfWeek())) / 7;
    if (weeks < 0 || weeks % rule.interval != 0) return false;
    break;
  }
  case TaskRecurrenceFrequency::Monthly:
    if (months < 0 || months % rule.interval != 0) return false;
    break;
  case TaskRecurrenceFrequency::Yearly:
    if (years < 0 || years % rule.interval != 0) return false;
    break;
  }
  if (!rule.weekdays.isEmpty() && !rule.weekdays.contains(date.dayOfWeek())) {
    return false;
  }
  if (!matchesOrdinalWeekday(date, rule.ordinalWeekdays) || !matchesMonthDay(date, rule.monthDays)) {
    return false;
  }
  if (rule.weekdays.isEmpty() && rule.ordinalWeekdays.isEmpty() && rule.monthDays.isEmpty()) {
    if (rule.frequency == TaskRecurrenceFrequency::Weekly && date.dayOfWeek() != anchor.dayOfWeek()) {
      return false;
    }
    if (rule.frequency == TaskRecurrenceFrequency::Monthly &&
        date != anchor.addMonths(static_cast<int>(months))) {
      return false;
    }
    if (rule.frequency == TaskRecurrenceFrequency::Yearly &&
        date != anchor.addYears(static_cast<int>(years))) {
      return false;
    }
  }
  return true;
}

[[nodiscard]] std::optional<QString> expandedRuleDate(const TaskRecurrenceMarker& marker,
                                                       std::int32_t ordinal) {
  const std::optional<DateOnlyRule> rule = parseDateOnlyRule(marker.recurrenceRule);
  if (!rule.has_value()) {
    return std::nullopt;
  }
  const QDate anchor = QDate::fromString(marker.anchorDate, Qt::ISODate);
  QSet<QString> excluded(marker.exclusionDates.cbegin(), marker.exclusionDates.cend());
  QSet<QString> added(marker.additionDates.cbegin(), marker.additionDates.cend());
  std::int32_t seen = 0;
  const QDate horizon = anchor.addYears(kMaximumExpansionYears);
  for (QDate date = anchor; date.isValid() && date <= horizon; date = date.addDays(1)) {
    const QString text = date.toString(Qt::ISODate);
    if (excluded.contains(text) || (!added.contains(text) && !matchesDateOnlyRule(date, anchor, *rule))) {
      continue;
    }
    if (seen == ordinal) {
      return text;
    }
    if (seen == std::numeric_limits<std::int32_t>::max()) {
      return std::nullopt;
    }
    ++seen;
  }
  return std::nullopt;
}

} // namespace

TaskRecurrenceNotes parseTaskRecurrenceNotes(const QString& notes) {
  const QString prefix = QString::fromLatin1(kMarkerPrefix);
  const QString suffix = QString::fromLatin1(kMarkerSuffix);
  const qsizetype markerStart = notes.indexOf(prefix);
  if (markerStart < 0) {
    return {.state = TaskRecurrenceNotesState::Unmanaged, .userNotes = notes};
  }
  if (notes.indexOf(prefix, markerStart + prefix.size()) >= 0) {
    return invalidNotes(notes, QStringLiteral("multiple marker headers exist"));
  }
  const qsizetype headerEnd = notes.indexOf(QStringLiteral("]\n"), markerStart + prefix.size());
  if (markerStart < 2 || notes.mid(markerStart - 2, 2) != QStringLiteral("\n\n") || headerEnd < 0) {
    return invalidNotes(notes, QStringLiteral("marker boundary is invalid"));
  }
  const QString version =
      notes.mid(markerStart + prefix.size(), headerEnd - markerStart - prefix.size());
  bool versionIsNumeric = false;
  const int parsedVersion = version.toInt(&versionIsNumeric);
  const qsizetype markerEnd = notes.indexOf(suffix, headerEnd + 2);
  if (!versionIsNumeric || parsedVersion < 1 || markerEnd < 0 ||
      markerEnd + suffix.size() != notes.size()) {
    return invalidNotes(notes, QStringLiteral("marker envelope is invalid"));
  }
  if (parsedVersion != 1 && parsedVersion != 2) {
    return {.state = TaskRecurrenceNotesState::UnsupportedVersion,
            .userNotes = notes,
            .diagnostic = QStringLiteral("HCB recurrence marker version is unsupported")};
  }
  const QString payload = notes.mid(headerEnd + 2, markerEnd - headerEnd - 2);
  if (payload.isEmpty() || payload.contains(u'\n') ||
      payload.toUtf8().size() > kMaximumNotesBytes) {
    return invalidNotes(notes, QStringLiteral("marker payload is invalid"));
  }
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(payload.toUtf8(), &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return invalidNotes(notes, QStringLiteral("marker payload is not a JSON object"));
  }
  QString diagnostic;
  const std::optional<TaskRecurrenceMarker> marker =
      markerFromJson(document.object(), diagnostic, parsedVersion == 2);
  if (!marker.has_value()) {
    return invalidNotes(notes, std::move(diagnostic));
  }
  return {.state = TaskRecurrenceNotesState::Managed,
          .userNotes = notes.left(markerStart - 2),
          .marker = marker};
}

std::optional<TaskRecurrenceRuleInfo> parseTaskRecurrenceRule(const QString& rule) {
  const std::optional<DateOnlyRule> parsed = parseDateOnlyRule(rule);
  return parsed.has_value()
             ? std::optional<TaskRecurrenceRuleInfo>(
                   {.frequency = parsed->frequency, .interval = parsed->interval})
             : std::nullopt;
}

TaskRecurrenceSerializationResult serializeTaskRecurrenceNotes(const QString& userNotes,
                                                               const TaskRecurrenceMarker& marker) {
  if (userNotes.contains(QChar::Null)) {
    return {.error = QStringLiteral("Task notes contain a null character")};
  }
  if (const std::optional<QString> error = validate(marker); error.has_value()) {
    return {.error = QStringLiteral("HCB recurrence marker is invalid: ") + *error};
  }
  QJsonObject end{{QStringLiteral("k"), endKindText(marker.end.kind)}};
  if (marker.end.untilDate.has_value()) {
    end.insert(QStringLiteral("u"), *marker.end.untilDate);
  }
  if (marker.end.count.has_value()) {
    end.insert(QStringLiteral("c"), *marker.end.count);
  }
  const QJsonObject templateObject{{QStringLiteral("d"), marker.templateDueDate},
                                   {QStringLiteral("p"), marker.templatePriority},
                                   {QStringLiteral("t"), marker.templateTitle}};
  const QJsonObject payload{{QStringLiteral("a"), marker.anchorDate},
                            {QStringLiteral("d"), QJsonArray::fromStringList(marker.additionDates)},
                            {QStringLiteral("e"), end},
                            {QStringLiteral("i"), marker.interval},
                            {QStringLiteral("n"), marker.ordinal},
                            {QStringLiteral("o"), marker.occurrenceId},
                            {QStringLiteral("q"), marker.recurrenceRule},
                            {QStringLiteral("r"), frequencyText(marker.frequency)},
                            {QStringLiteral("s"), marker.seriesId},
                            {QStringLiteral("t"), templateObject},
                            {QStringLiteral("x"), QJsonArray::fromStringList(marker.exclusionDates)},
                            {QStringLiteral("z"), marker.timeZone}};
  const QString compact = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
  const QString notes = userNotes + QStringLiteral("\n\n") + QString::fromLatin1(kMarkerPrefix) +
                        QStringLiteral("2]\n") + compact + QString::fromLatin1(kMarkerSuffix);
  if (notes.toUtf8().size() > kMaximumNotesBytes) {
    return {.error = QStringLiteral("Task notes and recurrence marker exceed Google Tasks limit")};
  }
  return {.notes = notes};
}

std::optional<QString> taskRecurrenceDate(const TaskRecurrenceMarker& marker,
                                          std::int32_t ordinal) {
  if (ordinal < 0 || validate(marker).has_value()) {
    return std::nullopt;
  }
  if (!marker.recurrenceRule.isEmpty()) {
    return expandedRuleDate(marker, ordinal);
  }
  const QDate anchor = QDate::fromString(marker.anchorDate, Qt::ISODate);
  const qint64 intervals = static_cast<qint64>(marker.interval) * ordinal;
  QDate date;
  switch (marker.frequency) {
  case TaskRecurrenceFrequency::Daily:
    date = anchor.addDays(intervals);
    break;
  case TaskRecurrenceFrequency::Weekly:
    date = anchor.addDays(intervals * 7);
    break;
  case TaskRecurrenceFrequency::Monthly:
    if (intervals > std::numeric_limits<int>::max()) {
      return std::nullopt;
    }
    date = anchor.addMonths(static_cast<int>(intervals));
    break;
  case TaskRecurrenceFrequency::Yearly:
    if (intervals > std::numeric_limits<int>::max()) {
      return std::nullopt;
    }
    date = anchor.addYears(static_cast<int>(intervals));
    break;
  }
  return date.isValid() ? std::optional<QString>(date.toString(Qt::ISODate)) : std::nullopt;
}

std::optional<TaskRecurrenceMarker> taskRecurrenceSuccessor(const TaskRecurrenceMarker& marker) {
  if (marker.ordinal == std::numeric_limits<std::int32_t>::max()) {
    return std::nullopt;
  }
  TaskRecurrenceMarker successor = marker;
  successor.ordinal += 1;
  successor.occurrenceId = successor.seriesId + QStringLiteral(":") +
                           QString::number(successor.ordinal);
  const std::optional<QString> dueDate = taskRecurrenceDate(successor, successor.ordinal);
  if (!dueDate.has_value() ||
      (successor.end.kind == TaskRecurrenceEndKind::Until &&
       *dueDate > *successor.end.untilDate) ||
      (successor.end.kind == TaskRecurrenceEndKind::Count &&
       successor.ordinal >= *successor.end.count)) {
    return std::nullopt;
  }
  successor.templateDueDate = *dueDate;
  return successor;
}

QString taskRecurrenceSummary(const TaskRecurrenceMarker& marker) {
  if (!marker.recurrenceRule.isEmpty()) {
    QString summary = QStringLiteral("Custom: ") + marker.recurrenceRule;
    if (!marker.exclusionDates.isEmpty()) {
      summary += QStringLiteral(" · skips %1 date(s)").arg(marker.exclusionDates.size());
    }
    if (!marker.additionDates.isEmpty()) {
      summary += QStringLiteral(" · adds %1 date(s)").arg(marker.additionDates.size());
    }
    return summary;
  }
  const QString singular = frequencyText(marker.frequency);
  const QString plural =
      marker.frequency == TaskRecurrenceFrequency::Daily     ? QStringLiteral("days")
      : marker.frequency == TaskRecurrenceFrequency::Weekly  ? QStringLiteral("weeks")
      : marker.frequency == TaskRecurrenceFrequency::Monthly ? QStringLiteral("months")
                                                             : QStringLiteral("years");
  QString summary = marker.interval == 1
                        ? QStringLiteral("Every ") + singular
                        : QStringLiteral("Every %1 %2").arg(marker.interval).arg(plural);
  if (marker.end.kind == TaskRecurrenceEndKind::Until) {
    summary += QStringLiteral(" until ") + *marker.end.untilDate;
  } else if (marker.end.kind == TaskRecurrenceEndKind::Count) {
    summary += QStringLiteral(" for %1 occurrences").arg(*marker.end.count);
  }
  return summary;
}

} // namespace hcb
