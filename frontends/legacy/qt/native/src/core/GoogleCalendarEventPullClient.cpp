#include "core/GoogleCalendarEventPullClient.h"

#include "core/GoogleHttpClient.h"

#include <QDate>
#include <QDateTime>
#include <QDebug>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSet>
#include <QTime>
#include <QTimeZone>

#include <cmath>
#include <future>
#include <limits>
#include <memory>
#include <thread>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumResponseBytes = static_cast<qsizetype>(8) * 1024 * 1024;
constexpr qsizetype kMaximumEventCount = 100'000;
constexpr qsizetype kMaximumEventIdLength = 256;
constexpr qsizetype kMaximumTitleLength = 500;
constexpr qsizetype kMaximumDescriptionLength = 20'000;
constexpr qsizetype kMaximumLocationLength = 1'000;
constexpr qsizetype kMaximumTimeZoneLength = 120;
constexpr qsizetype kMaximumRecurrenceCount = 128;
constexpr qsizetype kMaximumRecurrenceLength = 4'096;
constexpr qsizetype kMaximumColorIdLength = 32;
constexpr qsizetype kMaximumAttendeeCount = 200;
constexpr qsizetype kMaximumAttendeeJsonBytes = 262'144;
constexpr qsizetype kMaximumReminderCount = 5;
constexpr qsizetype kMaximumAttachmentCount = 25;
constexpr qsizetype kMaximumConferenceJsonBytes = 32'768;
constexpr qsizetype kMaximumAttachmentsJsonBytes = 65'536;
constexpr qsizetype kMaximumEventPropertiesJsonBytes = 16'384;
constexpr qsizetype kMaximumEtagLength = 4'096;
constexpr qsizetype kMaximumPageTokenLength = 8'192;
constexpr qsizetype kMaximumSyncTokenLength = 8'192;
constexpr qsizetype kMaximumTimestampLength = 64;
constexpr int kMaximumPages = 400;
constexpr int kMaximumInstancesPerPage = 2'500;

struct DecodedEventTime final {
  QString at;
  std::optional<QString> timeZone;
  bool allDay;
};

struct DecodedCalendarEventPage final {
  QList<GoogleCalendarEventMirror> events;
  std::optional<QString> nextPageToken;
  std::optional<QString> nextSyncToken;
};

using DecodedCalendarEventPageOrError = std::variant<DecodedCalendarEventPage, GoogleApiError>;

[[nodiscard]] GoogleApiError invalidPayloadError() {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::InvalidPayload,
       .message = QStringLiteral("Google calendar-event response payload is invalid")});
}

[[nodiscard]] GoogleApiError invalidRequestError() {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::InvalidPayload,
       .message = QStringLiteral("Google calendar-event pull request is invalid")});
}

[[nodiscard]] GoogleApiError transportError() {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::Transport,
       .message = QStringLiteral("Google calendar-event pull failed before completion")});
}

[[nodiscard]] bool isPresent(const QJsonValue& value) {
  return !value.isUndefined() && !value.isNull();
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumEventIdLength &&
         !value.contains(QChar::Null) && !value.contains(u'/') && !value.contains(u'?') &&
         !value.contains(u'#') && !value.contains(u'\\');
}

[[nodiscard]] bool isValidSyncToken(const std::optional<QString>& token) {
  return !token.has_value() || (!token->isEmpty() && token->size() <= kMaximumSyncTokenLength &&
                                !token->contains(QChar::Null));
}

[[nodiscard]] bool isValidRange(const QString& timeMin, const QString& timeMax) {
  if (timeMin.size() > kMaximumTimestampLength || timeMax.size() > kMaximumTimestampLength ||
      !timeMin.contains(u'T') || !timeMax.contains(u'T')) {
    return false;
  }
  const QDateTime minimum = QDateTime::fromString(timeMin, Qt::ISODateWithMs);
  const QDateTime maximum = QDateTime::fromString(timeMax, Qt::ISODateWithMs);
  return minimum.isValid() && maximum.isValid() && maximum > minimum;
}

[[nodiscard]] std::optional<QString>
optionalString(const QJsonObject& object, QStringView key, qsizetype maximumLength) {
  const QJsonValue value = object.value(key);
  if (!isPresent(value)) {
    return std::optional<QString>{};
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString text = value.toString();
  return text.size() <= maximumLength && !text.contains(QChar::Null) ? std::optional<QString>(text)
                                                                     : std::nullopt;
}

[[nodiscard]] bool hasInvalidOptional(const std::optional<QString>& value,
                                      const QJsonObject& object,
                                      QStringView key) {
  return !value.has_value() && isPresent(object.value(key));
}

[[nodiscard]] std::optional<QString> normalizedTimestamp(const QJsonObject& object,
                                                         QStringView key) {
  const QJsonValue value = object.value(key);
  if (!isPresent(value)) {
    return std::optional<QString>{};
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString timestamp = value.toString();
  if (timestamp.isEmpty() || timestamp.size() > kMaximumTimestampLength ||
      timestamp.contains(QChar::Null)) {
    return std::nullopt;
  }
  const QDateTime parsed = QDateTime::fromString(timestamp, Qt::ISODate);
  return parsed.isValid() ? std::optional<QString>(parsed.toUTC().toString(Qt::ISODateWithMs))
                          : std::nullopt;
}

[[nodiscard]] std::optional<QString> normalizedTitle(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("summary"));
  if (!isPresent(value)) {
    return QStringLiteral("Untitled event");
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString title = value.toString();
  if (title.trimmed().isEmpty()) {
    return QStringLiteral("Untitled event");
  }
  if (title.contains(QChar::Null)) {
    return std::nullopt;
  }
  if (title.size() > kMaximumTitleLength) {
    qWarning().noquote() << "google.calendar_event_title_truncated"
                          << "length" << title.size()
                          << "limit" << kMaximumTitleLength;
    return title.first(kMaximumTitleLength);
  }
  return title;
}

[[nodiscard]] std::optional<GoogleCalendarEventStatus> eventStatus(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("status"));
  if (!value.isString()) {
    return std::nullopt;
  }
  if (value.toString() == QStringLiteral("confirmed")) {
    return GoogleCalendarEventStatus::Confirmed;
  }
  if (value.toString() == QStringLiteral("tentative")) {
    return GoogleCalendarEventStatus::Tentative;
  }
  if (value.toString() == QStringLiteral("cancelled")) {
    return GoogleCalendarEventStatus::Cancelled;
  }
  return std::nullopt;
}

[[nodiscard]] bool hasExplicitOffset(const QString& value) {
  const qsizetype timeSeparator = value.indexOf(u'T');
  if (timeSeparator < 0) {
    return false;
  }
  const QStringView time = QStringView(value).sliced(timeSeparator + 1);
  return time.endsWith(u'Z') || time.contains(u'+') || time.contains(u'-');
}

[[nodiscard]] std::optional<DecodedEventTime> eventTime(const QJsonValue& value) {
  if (!value.isObject()) {
    return std::nullopt;
  }
  const QJsonObject object = value.toObject();
  const std::optional<QString> timeZone =
      optionalString(object, u"timeZone", kMaximumTimeZoneLength);
  if (hasInvalidOptional(timeZone, object, u"timeZone") ||
      (timeZone.has_value() && !QTimeZone(timeZone->toUtf8()).isValid())) {
    return std::nullopt;
  }
  const QJsonValue dateValue = object.value(QStringLiteral("date"));
  const QJsonValue dateTimeValue = object.value(QStringLiteral("dateTime"));
  if (isPresent(dateValue) == isPresent(dateTimeValue) ||
      (!isPresent(dateValue) && !isPresent(dateTimeValue))) {
    return std::nullopt;
  }
  if (isPresent(dateValue)) {
    if (!dateValue.isString()) {
      return std::nullopt;
    }
    const QString dateText = dateValue.toString();
    const QDate date = QDate::fromString(dateText, Qt::ISODate);
    if (!date.isValid() || dateText.size() != 10 || dateText.contains(QChar::Null)) {
      return std::nullopt;
    }
    return DecodedEventTime{
        .at = QDateTime(date, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs),
        .timeZone = timeZone,
        .allDay = true};
  }
  if (!dateTimeValue.isString()) {
    return std::nullopt;
  }
  const QString dateTimeText = dateTimeValue.toString();
  if (dateTimeText.size() > kMaximumTimestampLength || dateTimeText.contains(QChar::Null) ||
      !dateTimeText.contains(u'T')) {
    return std::nullopt;
  }
  QDateTime parsed = QDateTime::fromString(dateTimeText, Qt::ISODate);
  if (!parsed.isValid() || (!hasExplicitOffset(dateTimeText) && !timeZone.has_value())) {
    return std::nullopt;
  }
  if (!hasExplicitOffset(dateTimeText)) {
    parsed = QDateTime(parsed.date(), parsed.time(), QTimeZone(timeZone->toUtf8()));
  }
  return DecodedEventTime{
      .at = parsed.toUTC().toString(Qt::ISODateWithMs), .timeZone = timeZone, .allDay = false};
}

[[nodiscard]] std::optional<QList<QString>> recurrenceRules(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("recurrence"));
  if (!isPresent(value)) {
    return QList<QString>{};
  }
  if (!value.isArray()) {
    return std::nullopt;
  }
  const QJsonArray rules = value.toArray();
  if (rules.size() > kMaximumRecurrenceCount) {
    return std::nullopt;
  }
  QList<QString> result;
  result.reserve(rules.size());
  for (const auto& rule : rules) {
    if (!rule.isString() || rule.toString().isEmpty() ||
        rule.toString().size() > kMaximumRecurrenceLength ||
        rule.toString().contains(QChar::Null)) {
      return std::nullopt;
    }
    result.append(rule.toString());
  }
  return result;
}

[[nodiscard]] std::optional<qint64> sequence(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("sequence"));
  if (!isPresent(value)) {
    return std::optional<qint64>{};
  }
  if (!value.isDouble()) {
    return std::nullopt;
  }
  const double raw = value.toDouble();
  if (!std::isfinite(raw) || raw < 0 ||
      raw > static_cast<double>(std::numeric_limits<qint64>::max())) {
    return std::nullopt;
  }
  const qint64 integral = static_cast<qint64>(raw);
  return static_cast<double>(integral) == raw ? std::optional<qint64>(integral) : std::nullopt;
}

[[nodiscard]] bool isKnownEventType(const std::optional<QString>& eventType) {
  return !eventType.has_value() || *eventType == QStringLiteral("default") ||
         *eventType == QStringLiteral("birthday") || *eventType == QStringLiteral("focusTime") ||
         *eventType == QStringLiteral("fromGmail") || *eventType == QStringLiteral("outOfOffice") ||
         *eventType == QStringLiteral("workingLocation");
}

[[nodiscard]] bool isKnownTransparency(const std::optional<QString>& transparency) {
  return !transparency.has_value() || *transparency == QStringLiteral("opaque") ||
         *transparency == QStringLiteral("transparent");
}

[[nodiscard]] bool isKnownVisibility(const std::optional<QString>& visibility) {
  return !visibility.has_value() || *visibility == QStringLiteral("default") ||
         *visibility == QStringLiteral("public") || *visibility == QStringLiteral("private") ||
         *visibility == QStringLiteral("confidential");
}

[[nodiscard]] std::optional<QJsonArray> decodedAttendees(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("attendees"));
  if (!isPresent(value)) {
    return QJsonArray();
  }
  if (!value.isArray() || value.toArray().size() > kMaximumAttendeeCount ||
      QJsonDocument(value.toArray()).toJson(QJsonDocument::Compact).size() >
          kMaximumAttendeeJsonBytes) {
    return std::nullopt;
  }
  QJsonArray result;
  QSet<QString> emails;
  for (const QJsonValue& attendeeValue : value.toArray()) {
    if (!attendeeValue.isObject()) {
      return std::nullopt;
    }
    const QJsonObject source = attendeeValue.toObject();
    const std::optional<QString> email = optionalString(source, u"email", 254);
    const std::optional<QString> displayName = optionalString(source, u"displayName", 1'024);
    const std::optional<QString> comment = optionalString(source, u"comment", 4'096);
    const QJsonValue optional = source.value(QStringLiteral("optional"));
    const QJsonValue responseStatus = source.value(QStringLiteral("responseStatus"));
    const QJsonValue additionalGuests = source.value(QStringLiteral("additionalGuests"));
    const QJsonValue resource = source.value(QStringLiteral("resource"));
    const QJsonValue self = source.value(QStringLiteral("self"));
    const bool validResponse = responseStatus.isUndefined() ||
                               (responseStatus.isString() &&
                                (responseStatus.toString() == QStringLiteral("needsAction") ||
                                 responseStatus.toString() == QStringLiteral("declined") ||
                                 responseStatus.toString() == QStringLiteral("tentative") ||
                                 responseStatus.toString() == QStringLiteral("accepted")));
    const bool validAdditionalGuests =
        additionalGuests.isUndefined() ||
        (additionalGuests.isDouble() && additionalGuests.toInteger(-1) >= 0 &&
         additionalGuests.toInteger(-1) <= 10'000);
    if (hasInvalidOptional(email, source, u"email") ||
        hasInvalidOptional(displayName, source, u"displayName") ||
        hasInvalidOptional(comment, source, u"comment") ||
        (!optional.isUndefined() && !optional.isBool()) ||
        (!resource.isUndefined() && !resource.isBool()) || !validResponse ||
        !validAdditionalGuests || (!self.isUndefined() && !self.isBool())) {
      return std::nullopt;
    }
    QJsonObject canonical;
    if (email.has_value()) {
      const QString key = email->toCaseFolded();
      if (email->trimmed() != *email || email->contains(QChar::Null) || emails.contains(key)) {
        return std::nullopt;
      }
      emails.insert(key);
      canonical.insert(QStringLiteral("email"), *email);
    }
    if (displayName.has_value()) {
      canonical.insert(QStringLiteral("displayName"), *displayName);
    }
    if (comment.has_value()) {
      canonical.insert(QStringLiteral("comment"), *comment);
    }
    if (!optional.isUndefined()) {
      canonical.insert(QStringLiteral("optional"), optional);
    }
    if (!responseStatus.isUndefined()) {
      canonical.insert(QStringLiteral("responseStatus"), responseStatus);
    }
    if (!additionalGuests.isUndefined()) {
      canonical.insert(QStringLiteral("additionalGuests"), additionalGuests.toInteger());
    }
    if (!resource.isUndefined()) {
      canonical.insert(QStringLiteral("resource"), resource);
    }
    if (!self.isUndefined()) {
      canonical.insert(QStringLiteral("self"), self);
    }
    result.append(canonical);
  }
  return result;
}

[[nodiscard]] std::optional<QJsonObject> decodedReminders(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("reminders"));
  if (!isPresent(value)) {
    return QJsonObject{{QStringLiteral("useDefault"), true},
                       {QStringLiteral("overrides"), QJsonArray()}};
  }
  if (!value.isObject()) {
    return std::nullopt;
  }
  const QJsonObject source = value.toObject();
  const QJsonValue useDefault = source.value(QStringLiteral("useDefault"));
  const QJsonValue overrides = source.value(QStringLiteral("overrides"));
  if ((!useDefault.isUndefined() && !useDefault.isBool()) ||
      (!overrides.isUndefined() && !overrides.isArray()) ||
      (overrides.isArray() && overrides.toArray().size() > kMaximumReminderCount)) {
    return std::nullopt;
  }
  QJsonArray canonicalOverrides;
  if (overrides.isArray()) {
    for (const QJsonValue& overrideValue : overrides.toArray()) {
      if (!overrideValue.isObject()) {
        return std::nullopt;
      }
      const QJsonObject reminder = overrideValue.toObject();
      const QJsonValue method = reminder.value(QStringLiteral("method"));
      const QJsonValue minutes = reminder.value(QStringLiteral("minutes"));
      if (!method.isString() || (method.toString() != QStringLiteral("email") &&
                                 method.toString() != QStringLiteral("popup")) ||
          !minutes.isDouble() || minutes.toInteger(-1) < 0 ||
          minutes.toInteger(-1) > 40'320) {
        return std::nullopt;
      }
      canonicalOverrides.append(
          QJsonObject{{QStringLiteral("method"), method.toString()},
                      {QStringLiteral("minutes"), minutes.toInteger()}});
    }
  }
  return QJsonObject{{QStringLiteral("useDefault"), useDefault.toBool(true)},
                     {QStringLiteral("overrides"), canonicalOverrides}};
}

[[nodiscard]] std::optional<QJsonObject>
decodedObject(const QJsonObject& source, QStringView key, qsizetype maximumBytes) {
  const QJsonValue value = source.value(key);
  if (!isPresent(value)) {
    return QJsonObject();
  }
  if (!value.isObject() ||
      QJsonDocument(value.toObject()).toJson(QJsonDocument::Compact).size() > maximumBytes) {
    return std::nullopt;
  }
  return value.toObject();
}

[[nodiscard]] std::optional<QJsonArray> decodedAttachments(const QJsonObject& source) {
  const QJsonValue value = source.value(QStringLiteral("attachments"));
  if (!isPresent(value)) {
    return QJsonArray();
  }
  if (!value.isArray() || value.toArray().size() > kMaximumAttachmentCount ||
      QJsonDocument(value.toArray()).toJson(QJsonDocument::Compact).size() >
          kMaximumAttachmentsJsonBytes) {
    return std::nullopt;
  }
  for (const QJsonValue& attachment : value.toArray()) {
    if (!attachment.isObject()) {
      return std::nullopt;
    }
  }
  return value.toArray();
}

[[nodiscard]] std::optional<QJsonObject> decodedGuestPermissions(const QJsonObject& source) {
  QJsonObject permissions;
  for (const QStringView key : {u"guestsCanInviteOthers", u"guestsCanModify",
                                u"guestsCanSeeOtherGuests"}) {
    const QJsonValue value = source.value(key);
    if (!isPresent(value)) {
      continue;
    }
    if (!value.isBool()) {
      return std::nullopt;
    }
    permissions.insert(key.toString(), value);
  }
  return permissions;
}

[[nodiscard]] std::optional<QJsonObject> decodedStatusProperties(const QJsonObject& source) {
  QJsonObject properties;
  for (const QStringView key : {u"focusTimeProperties", u"outOfOfficeProperties",
                                u"workingLocationProperties"}) {
    const QJsonValue value = source.value(key);
    if (!isPresent(value)) {
      continue;
    }
    if (!value.isObject() ||
        QJsonDocument(value.toObject()).toJson(QJsonDocument::Compact).size() >
            kMaximumEventPropertiesJsonBytes) {
      return std::nullopt;
    }
    properties.insert(key.toString(), value);
  }
  return QJsonDocument(properties).toJson(QJsonDocument::Compact).size() <=
                 kMaximumEventPropertiesJsonBytes
             ? std::optional<QJsonObject>(properties)
             : std::nullopt;
}

[[nodiscard]] DecodedCalendarEventPageOrError decodePage(const QByteArray& responseBody,
                                                         const QString& calendarId,
                                                         qsizetype maximumItems) {
  if (responseBody.size() > kMaximumResponseBytes) {
    return invalidPayloadError();
  }
  const QJsonDocument document = QJsonDocument::fromJson(responseBody);
  if (!document.isObject()) {
    return invalidPayloadError();
  }
  const QJsonObject response = document.object();
  const QJsonValue itemsValue = response.value(QStringLiteral("items"));
  if (isPresent(itemsValue) && !itemsValue.isArray()) {
    return invalidPayloadError();
  }
  std::optional<QString> nextPageToken =
      optionalString(response, u"nextPageToken", kMaximumPageTokenLength);
  std::optional<QString> nextSyncToken =
      optionalString(response, u"nextSyncToken", kMaximumSyncTokenLength);
  if (hasInvalidOptional(nextPageToken, response, u"nextPageToken") ||
      hasInvalidOptional(nextSyncToken, response, u"nextSyncToken") ||
      (nextPageToken.has_value() && nextPageToken->isEmpty()) ||
      (nextSyncToken.has_value() && nextSyncToken->isEmpty()) ||
      (nextPageToken.has_value() && nextSyncToken.has_value())) {
    return invalidPayloadError();
  }
  const QJsonArray items = itemsValue.isArray() ? itemsValue.toArray() : QJsonArray();
  if (items.size() > maximumItems) {
    return invalidPayloadError();
  }
  QSet<QString> seenIds;
  QList<GoogleCalendarEventMirror> events;
  events.reserve(items.size());
  for (qsizetype itemIndex = 0; itemIndex < items.size(); ++itemIndex) {
    const QJsonValue itemValue = items.at(itemIndex);
    if (!itemValue.isObject()) {
      return invalidPayloadError();
    }
    const QJsonObject item = itemValue.toObject();
    const QJsonValue idValue = item.value(QStringLiteral("id"));
    if (!idValue.isString() || !isValidIdentifier(idValue.toString()) ||
        seenIds.contains(idValue.toString())) {
      return invalidPayloadError();
    }
    const std::optional<GoogleCalendarEventStatus> status = eventStatus(item);
    const std::optional<QString> title = normalizedTitle(item);
    const std::optional<QString> description =
        optionalString(item, u"description", kMaximumDescriptionLength);
    const std::optional<QString> location =
        optionalString(item, u"location", kMaximumLocationLength);
    const std::optional<QString> recurringEventId =
        optionalString(item, u"recurringEventId", kMaximumEventIdLength);
    const std::optional<QString> colorId = optionalString(item, u"colorId", kMaximumColorIdLength);
    const std::optional<QString> transparency = optionalString(item, u"transparency", 32);
    const std::optional<QString> visibility = optionalString(item, u"visibility", 32);
    const std::optional<QString> eventType = optionalString(item, u"eventType", 32);
    const std::optional<QString> etag = optionalString(item, u"etag", kMaximumEtagLength);
    const std::optional<QString> updatedAt = normalizedTimestamp(item, u"updated");
    const std::optional<QList<QString>> recurrence = recurrenceRules(item);
    const std::optional<qint64> eventSequence = sequence(item);
    const std::optional<QJsonArray> eventAttendees = decodedAttendees(item);
    const std::optional<QJsonObject> eventReminders = decodedReminders(item);
    const std::optional<QJsonObject> conferenceData =
        decodedObject(item, u"conferenceData", kMaximumConferenceJsonBytes);
    const std::optional<QJsonArray> attachments = decodedAttachments(item);
    const std::optional<QJsonObject> guestPermissions = decodedGuestPermissions(item);
    const std::optional<QJsonObject> statusProperties = decodedStatusProperties(item);
    const std::optional<DecodedEventTime> originalStart =
        isPresent(item.value(QStringLiteral("originalStartTime")))
            ? eventTime(item.value(QStringLiteral("originalStartTime")))
            : std::optional<DecodedEventTime>{};
    const bool hasStart = isPresent(item.value(QStringLiteral("start")));
    const bool hasEnd = isPresent(item.value(QStringLiteral("end")));
    const std::optional<DecodedEventTime> start =
        hasStart ? eventTime(item.value(QStringLiteral("start")))
                 : std::optional<DecodedEventTime>{};
    const std::optional<DecodedEventTime> end =
        hasEnd ? eventTime(item.value(QStringLiteral("end"))) : std::optional<DecodedEventTime>{};
    const bool malformed =
        !status.has_value() || !title.has_value() ||
        hasInvalidOptional(description, item, u"description") ||
        hasInvalidOptional(location, item, u"location") ||
        hasInvalidOptional(recurringEventId, item, u"recurringEventId") ||
        hasInvalidOptional(colorId, item, u"colorId") ||
        hasInvalidOptional(transparency, item, u"transparency") ||
        hasInvalidOptional(visibility, item, u"visibility") ||
        hasInvalidOptional(eventType, item, u"eventType") ||
        hasInvalidOptional(etag, item, u"etag") ||
        hasInvalidOptional(updatedAt, item, u"updated") || !recurrence.has_value() ||
        (!eventSequence.has_value() && isPresent(item.value(QStringLiteral("sequence")))) ||
        (isPresent(item.value(QStringLiteral("originalStartTime"))) &&
         !originalStart.has_value()) ||
        (hasStart != hasEnd) || (hasStart && (!start.has_value() || !end.has_value())) ||
        (start.has_value() && end.has_value() &&
         (start->allDay != end->allDay ||
          QDateTime::fromString(end->at, Qt::ISODateWithMs) <=
              QDateTime::fromString(start->at, Qt::ISODateWithMs))) ||
        (!start.has_value() && *status != GoogleCalendarEventStatus::Cancelled) ||
        !isKnownEventType(eventType) || !isKnownTransparency(transparency) ||
        !isKnownVisibility(visibility) || !eventAttendees.has_value() ||
        !eventReminders.has_value() || !conferenceData.has_value() || !attachments.has_value() ||
        !guestPermissions.has_value() || !statusProperties.has_value();
    if (malformed) {
      qWarning().noquote()
          << "google.calendar_event_payload_invalid"
          << "item_index" << itemIndex
          << "status" << status.has_value()
          << "title" << title.has_value()
          << "updated" << updatedAt.has_value()
          << "recurrence" << recurrence.has_value()
          << "sequence" << eventSequence.has_value()
          << "start" << start.has_value()
          << "end" << end.has_value()
          << "event_type" << isKnownEventType(eventType)
          << "attendees" << eventAttendees.has_value()
          << "reminders" << eventReminders.has_value()
          << "conference" << conferenceData.has_value()
          << "attachments" << attachments.has_value()
          << "permissions" << guestPermissions.has_value()
          << "properties" << statusProperties.has_value();
      return invalidPayloadError();
    }
    seenIds.insert(idValue.toString());
    events.append({.id = idValue.toString(),
                   .calendarId = calendarId,
                   .status = *status,
                   .title = *title,
                   .description = description,
                   .location = location,
                   .startAt = start.has_value() ? std::optional<QString>(start->at) : std::nullopt,
                   .startTimeZone = start.has_value() ? start->timeZone : std::nullopt,
                   .endAt = end.has_value() ? std::optional<QString>(end->at) : std::nullopt,
                   .endTimeZone = end.has_value() ? end->timeZone : std::nullopt,
                   .allDay = start.has_value() && start->allDay,
                   .recurringEventId = recurringEventId,
                   .originalStartAt = originalStart.has_value()
                                          ? std::optional<QString>(originalStart->at)
                                          : std::nullopt,
                   .recurrence = *recurrence,
                   .colorId = colorId,
                   .transparency = transparency,
                   .visibility = visibility,
                   .eventType = eventType,
                   .attendees = *eventAttendees,
                   .reminders = *eventReminders,
                   .conferenceData = *conferenceData,
                   .attachments = *attachments,
                   .guestPermissions = *guestPermissions,
                   .statusProperties = *statusProperties,
                   .etag = etag,
                   .sequence = eventSequence,
                   .updatedAt = updatedAt});
  }
  return DecodedCalendarEventPage{
      .events = std::move(events), .nextPageToken = nextPageToken, .nextSyncToken = nextSyncToken};
}

[[nodiscard]] GoogleHttpRequest requestForPage(const GoogleCalendarEventPullRequest& request,
                                               const std::optional<QString>& pageToken) {
  GoogleHttpRequest httpRequest;
  httpRequest.path =
      QStringLiteral("/calendar/v3/calendars/") + request.calendarId + QStringLiteral("/events");
  httpRequest.query = {
      {.name = QStringLiteral("maxResults"), .value = QStringLiteral("250")},
      {.name = QStringLiteral("showDeleted"), .value = QStringLiteral("true")},
      {.name = QStringLiteral("showHiddenInvitations"), .value = QStringLiteral("true")},
      {.name = QStringLiteral("singleEvents"), .value = QStringLiteral("false")},
      {.name = QStringLiteral("conferenceDataVersion"), .value = QStringLiteral("1")},
      {.name = QStringLiteral("fields"),
       .value = QStringLiteral(
           "nextPageToken,nextSyncToken,items(id,status,summary,description,location,"
           "start,end,recurringEventId,originalStartTime,recurrence,colorId,transparency,"
           "visibility,eventType,attendees(email,displayName,comment,optional,responseStatus,additionalGuests,resource,self),reminders,conferenceData,attachments,"
           "guestsCanInviteOthers,guestsCanModify,guestsCanSeeOtherGuests,focusTimeProperties,"
           "outOfOfficeProperties,workingLocationProperties,etag,sequence,updated)")}};
  if (request.syncToken.has_value()) {
    httpRequest.query.append({.name = QStringLiteral("syncToken"), .value = *request.syncToken});
  }
  if (pageToken.has_value()) {
    httpRequest.query.append({.name = QStringLiteral("pageToken"), .value = *pageToken});
  }
  return httpRequest;
}

[[nodiscard]] GoogleHttpRequest
requestForInstancesPage(const GoogleCalendarEventInstancesPullRequest& request,
                        const std::optional<QString>& pageToken) {
  GoogleHttpRequest httpRequest;
  httpRequest.path = QStringLiteral("/calendar/v3/calendars/") + request.calendarId +
                     QStringLiteral("/events/") + request.recurringEventId +
                     QStringLiteral("/instances");
  httpRequest.query = {
      {.name = QStringLiteral("maxResults"), .value = QString::number(kMaximumInstancesPerPage)},
      {.name = QStringLiteral("showDeleted"), .value = QStringLiteral("true")},
      {.name = QStringLiteral("timeMin"), .value = request.timeMin},
      {.name = QStringLiteral("timeMax"), .value = request.timeMax},
      {.name = QStringLiteral("conferenceDataVersion"), .value = QStringLiteral("1")},
      {.name = QStringLiteral("fields"),
       .value = QStringLiteral(
           "nextPageToken,items(id,status,summary,description,location,start,end,recurringEventId,"
           "originalStartTime,recurrence,colorId,transparency,visibility,eventType,attendees(email,displayName,comment,optional,responseStatus,additionalGuests,resource,self),"
           "reminders,conferenceData,attachments,guestsCanInviteOthers,guestsCanModify,"
           "guestsCanSeeOtherGuests,focusTimeProperties,outOfOfficeProperties,"
           "workingLocationProperties,etag,sequence,updated)")}};
  if (pageToken.has_value()) {
    httpRequest.query.append({.name = QStringLiteral("pageToken"), .value = *pageToken});
  }
  return httpRequest;
}

} // namespace

GoogleCalendarEventPullClient::GoogleCalendarEventPullClient(GoogleHttpClient& httpClient)
    : httpClient_(httpClient) {}

std::future<GoogleCalendarEventPullResultOrError>
GoogleCalendarEventPullClient::list(GoogleCalendarEventPullRequest request,
                                    const QString& accessToken) {
  if (!isValidIdentifier(request.calendarId) || !isValidSyncToken(request.syncToken)) {
    std::promise<GoogleCalendarEventPullResultOrError> completion;
    std::future<GoogleCalendarEventPullResultOrError> future = completion.get_future();
    completion.set_value(invalidRequestError());
    return future;
  }
  auto completion = std::make_shared<std::promise<GoogleCalendarEventPullResultOrError>>();
  std::future<GoogleCalendarEventPullResultOrError> future = completion->get_future();
  try {
    std::thread([this, request = std::move(request), accessToken, completion] {
      try {
        QList<GoogleCalendarEventMirror> events;
        events.reserve(kMaximumEventCount);
        QSet<QString> seenIds;
        std::optional<QString> pageToken;
        std::optional<QString> nextSyncToken;
        std::optional<QString> serverDate;
        for (int page = 0; page < kMaximumPages; ++page) {
          GoogleHttpResult response =
              httpClient_.send(requestForPage(request, pageToken), accessToken).get();
          if (std::holds_alternative<GoogleApiError>(response)) {
            completion->set_value(std::get<GoogleApiError>(std::move(response)));
            return;
          }
          GoogleHttpResponse httpResponse = std::get<GoogleHttpResponse>(std::move(response));
          if (!serverDate.has_value()) {
            serverDate = std::move(httpResponse.serverDate);
          }
          DecodedCalendarEventPageOrError decoded =
              decodePage(httpResponse.body, request.calendarId, 250);
          if (std::holds_alternative<GoogleApiError>(decoded)) {
            completion->set_value(std::get<GoogleApiError>(std::move(decoded)));
            return;
          }
          DecodedCalendarEventPage pageData =
              std::get<DecodedCalendarEventPage>(std::move(decoded));
          if (events.size() + pageData.events.size() > kMaximumEventCount) {
            completion->set_value(invalidPayloadError());
            return;
          }
          for (GoogleCalendarEventMirror& event : pageData.events) {
            if (seenIds.contains(event.id)) {
              completion->set_value(invalidPayloadError());
              return;
            }
            seenIds.insert(event.id);
            events.append(std::move(event));
          }
          if (!pageData.nextPageToken.has_value()) {
            completion->set_value(
                GoogleCalendarEventPullResult{.events = std::move(events),
                                              .nextSyncToken = pageData.nextSyncToken,
                                              .serverDate = serverDate});
            return;
          }
          pageToken = std::move(pageData.nextPageToken);
        }
        completion->set_value(invalidPayloadError());
      } catch (...) {
        completion->set_value(transportError());
      }
    }).detach();
  } catch (...) {
    completion->set_value(transportError());
  }
  return future;
}

std::future<GoogleCalendarEventInstancesPullResultOrError>
GoogleCalendarEventPullClient::instances(GoogleCalendarEventInstancesPullRequest request,
                                         const QString& accessToken) {
  if (!isValidIdentifier(request.calendarId) || !isValidIdentifier(request.recurringEventId) ||
      !isValidRange(request.timeMin, request.timeMax)) {
    std::promise<GoogleCalendarEventInstancesPullResultOrError> completion;
    std::future<GoogleCalendarEventInstancesPullResultOrError> future = completion.get_future();
    completion.set_value(invalidRequestError());
    return future;
  }
  auto completion = std::make_shared<std::promise<GoogleCalendarEventInstancesPullResultOrError>>();
  std::future<GoogleCalendarEventInstancesPullResultOrError> future = completion->get_future();
  try {
    std::thread([this, request = std::move(request), accessToken, completion] {
      try {
        QList<GoogleCalendarEventMirror> events;
        events.reserve(kMaximumInstancesPerPage);
        QSet<QString> seenIds;
        std::optional<QString> pageToken;
        std::optional<QString> serverDate;
        for (int page = 0; page < kMaximumPages; ++page) {
          GoogleHttpResult response =
              httpClient_.send(requestForInstancesPage(request, pageToken), accessToken).get();
          if (std::holds_alternative<GoogleApiError>(response)) {
            completion->set_value(std::get<GoogleApiError>(std::move(response)));
            return;
          }
          GoogleHttpResponse httpResponse = std::get<GoogleHttpResponse>(std::move(response));
          if (!serverDate.has_value()) {
            serverDate = std::move(httpResponse.serverDate);
          }
          DecodedCalendarEventPageOrError decoded =
              decodePage(httpResponse.body, request.calendarId, kMaximumInstancesPerPage);
          if (std::holds_alternative<GoogleApiError>(decoded)) {
            completion->set_value(std::get<GoogleApiError>(std::move(decoded)));
            return;
          }
          DecodedCalendarEventPage pageData =
              std::get<DecodedCalendarEventPage>(std::move(decoded));
          if (events.size() + pageData.events.size() > kMaximumEventCount) {
            completion->set_value(invalidPayloadError());
            return;
          }
          for (GoogleCalendarEventMirror& event : pageData.events) {
            if (!event.recurringEventId.has_value() ||
                *event.recurringEventId != request.recurringEventId ||
                seenIds.contains(event.id)) {
              completion->set_value(invalidPayloadError());
              return;
            }
            seenIds.insert(event.id);
            events.append(std::move(event));
          }
          if (!pageData.nextPageToken.has_value()) {
            completion->set_value(
                GoogleCalendarEventInstancesPullResult{.events = std::move(events),
                                                        .serverDate = serverDate});
            return;
          }
          pageToken = std::move(pageData.nextPageToken);
        }
        completion->set_value(invalidPayloadError());
      } catch (...) {
        completion->set_value(transportError());
      }
    }).detach();
  } catch (...) {
    completion->set_value(transportError());
  }
  return future;
}

} // namespace hcb
