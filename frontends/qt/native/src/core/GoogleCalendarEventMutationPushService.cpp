#include "core/GoogleCalendarEventMutationPushService.h"

#include "core/CalendarMutationService.h"
#include "core/Clock.h"
#include "core/GoogleApiError.h"
#include "core/GoogleHttpClient.h"
#include "core/GoogleSyncConflictResolver.h"
#include "core/MutationTelemetryStore.h"
#include "core/OptimisticMutationCoordinator.h"
#include "core/SyncBackoffPolicy.h"

#include <QDate>
#include <QDateTime>
#include <QHash>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QRegularExpression>
#include <QSet>
#include <QTime>
#include <QTimeZone>
#include <QUuid>
#include <QUrl>

#include <algorithm>
#include <chrono>
#include <future>
#include <functional>
#include <limits>
#include <memory>
#include <optional>
#include <thread>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr int kMaximumPushBatch = 100;
constexpr int kMaximumGoogleBatchParts = 100;
constexpr qsizetype kMaximumBatchResponseBytes = 10 * 1024 * 1024;
constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumTitleLength = 500;
constexpr qsizetype kMaximumDescriptionLength = 20'000;
constexpr qsizetype kMaximumLocationLength = 1'000;
constexpr qsizetype kMaximumTimeZoneLength = 120;
constexpr qsizetype kMaximumEtagLength = 4'096;
constexpr qsizetype kMaximumAttendeeCount = 200;
constexpr qsizetype kMaximumReminderCount = 5;
constexpr qsizetype kMaximumAttachmentCount = 25;
constexpr qsizetype kMaximumAttachmentUrlLength = 2'048;
constexpr auto kMutationLeaseDuration = std::chrono::minutes(5);

struct CanonicalEventTime final {
  QJsonObject json;
  QDateTime at;
  bool allDay;
};

struct EventPushRequest final {
  GoogleHttpRequest request;
};

using EventPushRequestOrError = std::variant<EventPushRequest, QString>;

struct ResolvedInstanceRequest final {
  PendingMutation mutation;
  GoogleHttpRequest request;
};

using ResolvedInstanceRequestOrError =
    std::variant<ResolvedInstanceRequest, GoogleApiError, QString>;

struct GoogleEventWriteResponse final {
  QString remoteEventId;
  std::optional<QString> remoteEtag;
};

using GoogleEventWriteResponseOrError = std::variant<GoogleEventWriteResponse, QString>;

struct BatchItem final {
  PendingMutation mutation;
  GoogleHttpRequest request;
};

struct EventPushOutcome final {
  int applied{0};
  int failed{0};
  int skipped{0};
};

using EventPushOutcomeOrError = std::variant<EventPushOutcome, AppError>;

[[nodiscard]] AppError databaseError(const QString& message) {
  return AppError(AppErrorCode::Database, message);
}

[[nodiscard]] AppError networkError(const QString& message) {
  return AppError(AppErrorCode::Network, message);
}

[[nodiscard]] bool isValidIdentifier(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximumLength &&
         !value.contains(QChar::Null) && !value.contains(u'/') && !value.contains(u'\\') &&
         !value.contains(u'?') && !value.contains(u'#');
}

[[nodiscard]] bool isValidRequiredText(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximumLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isValidOptionalText(const QString& value, qsizetype maximumLength) {
  return value.size() <= maximumLength && !value.contains(QChar::Null);
}

[[nodiscard]] bool isValidTimeZone(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumTimeZoneLength &&
         !value.contains(QChar::Null) && QTimeZone(value.toUtf8()).isValid();
}

[[nodiscard]] bool isValidEtag(const QString& value) {
  return !value.isEmpty() && value.size() <= kMaximumEtagLength && !value.contains(QChar::Null) &&
         !value.contains(u'\r') && !value.contains(u'\n');
}

[[nodiscard]] std::optional<QString> requiredIdentifier(const QJsonObject& object,
                                                        QStringView key) {
  const QJsonValue value = object.value(key);
  if (!value.isString() || !isValidIdentifier(value.toString(), kMaximumIdentifierLength)) {
    return std::nullopt;
  }
  return value.toString();
}

[[nodiscard]] std::optional<QString> optionalEtag(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("etag"));
  if (value.isUndefined() || value.isNull()) {
    return std::optional<QString>{};
  }
  if (!value.isString() || !isValidEtag(value.toString())) {
    return std::nullopt;
  }
  return value.toString();
}

[[nodiscard]] bool hasExplicitOffset(const QString& value) {
  const qsizetype separator = value.indexOf(u'T');
  if (separator < 0) {
    return false;
  }
  const QStringView time = QStringView(value).sliced(separator + 1);
  return time.endsWith(u'Z') || time.contains(u'+') || time.contains(u'-');
}

[[nodiscard]] std::optional<CanonicalEventTime> canonicalTime(const QJsonValue& value) {
  if (!value.isObject()) {
    return std::nullopt;
  }
  const QJsonObject raw = value.toObject();
  const QJsonValue date = raw.value(QStringLiteral("date"));
  const QJsonValue dateTime = raw.value(QStringLiteral("dateTime"));
  const QJsonValue timeZone = raw.value(QStringLiteral("timeZone"));
  if (date.isUndefined() == dateTime.isUndefined() ||
      (!timeZone.isUndefined() &&
       (!timeZone.isString() || !isValidTimeZone(timeZone.toString())))) {
    return std::nullopt;
  }
  QJsonObject json;
  if (timeZone.isString()) {
    json.insert(QStringLiteral("timeZone"), timeZone.toString());
  }
  if (!date.isUndefined()) {
    if (!date.isString() || date.toString().size() != 10 || date.toString().contains(QChar::Null)) {
      return std::nullopt;
    }
    const QDate parsed = QDate::fromString(date.toString(), Qt::ISODate);
    if (!parsed.isValid()) {
      return std::nullopt;
    }
    json.insert(QStringLiteral("date"), parsed.toString(Qt::ISODate));
    return CanonicalEventTime{.json = std::move(json),
                              .at = QDateTime(parsed, QTime(0, 0), QTimeZone::UTC),
                              .allDay = true};
  }
  if (!dateTime.isString() || dateTime.toString().size() > 64 ||
      dateTime.toString().contains(QChar::Null) || !dateTime.toString().contains(u'T')) {
    return std::nullopt;
  }
  QDateTime parsed = QDateTime::fromString(dateTime.toString(), Qt::ISODate);
  if (!parsed.isValid() || (!hasExplicitOffset(dateTime.toString()) && !timeZone.isString())) {
    return std::nullopt;
  }
  if (!hasExplicitOffset(dateTime.toString())) {
    parsed = QDateTime(parsed.date(), parsed.time(), QTimeZone(timeZone.toString().toUtf8()));
  }
  parsed = parsed.toUTC();
  json.insert(QStringLiteral("dateTime"), parsed.toString(Qt::ISODateWithMs));
  return CanonicalEventTime{.json = std::move(json), .at = parsed, .allDay = false};
}

[[nodiscard]] std::optional<QJsonArray> canonicalAttendees(const QJsonValue& value) {
  if (!value.isArray() || value.toArray().size() > kMaximumAttendeeCount) {
    return std::nullopt;
  }
  QJsonArray result;
  QSet<QString> emails;
  for (const QJsonValue& attendeeValue : value.toArray()) {
    if (!attendeeValue.isObject()) {
      return std::nullopt;
    }
    const QJsonObject attendee = attendeeValue.toObject();
    const QJsonValue email = attendee.value(QStringLiteral("email"));
    const QJsonValue displayName = attendee.value(QStringLiteral("displayName"));
    const QJsonValue comment = attendee.value(QStringLiteral("comment"));
    const QJsonValue optional = attendee.value(QStringLiteral("optional"));
    const QJsonValue status = attendee.value(QStringLiteral("responseStatus"));
    const QJsonValue additionalGuests = attendee.value(QStringLiteral("additionalGuests"));
    const QJsonValue resource = attendee.value(QStringLiteral("resource"));
    const bool validStatus = status.isUndefined() ||
                             (status.isString() &&
                              (status.toString() == QStringLiteral("needsAction") ||
                               status.toString() == QStringLiteral("declined") ||
                               status.toString() == QStringLiteral("tentative") ||
                               status.toString() == QStringLiteral("accepted")));
    if (!email.isString() || !isValidRequiredText(email.toString(), 254) ||
        !email.toString().contains(u'@') ||
        (!displayName.isUndefined() &&
         (!displayName.isString() || !isValidOptionalText(displayName.toString(), 1'024))) ||
        (!comment.isUndefined() &&
         (!comment.isString() || !isValidOptionalText(comment.toString(), 4'096))) ||
        (!optional.isUndefined() && !optional.isBool()) || !validStatus ||
        (!additionalGuests.isUndefined() &&
         (!additionalGuests.isDouble() || additionalGuests.toInteger(-1) < 0 ||
          additionalGuests.toInteger(-1) > 10'000)) ||
        (!resource.isUndefined() && !resource.isBool())) {
      return std::nullopt;
    }
    const QString key = email.toString().toCaseFolded();
    if (emails.contains(key)) {
      return std::nullopt;
    }
    emails.insert(key);
    QJsonObject canonical{{QStringLiteral("email"), email.toString()}};
    if (!displayName.isUndefined()) {
      canonical.insert(QStringLiteral("displayName"), displayName);
    }
    if (!comment.isUndefined()) {
      canonical.insert(QStringLiteral("comment"), comment);
    }
    if (!optional.isUndefined()) {
      canonical.insert(QStringLiteral("optional"), optional);
    }
    if (!status.isUndefined()) {
      canonical.insert(QStringLiteral("responseStatus"), status);
    }
    if (!additionalGuests.isUndefined()) {
      canonical.insert(QStringLiteral("additionalGuests"), additionalGuests.toInteger());
    }
    if (!resource.isUndefined()) {
      canonical.insert(QStringLiteral("resource"), resource);
    }
    result.append(canonical);
  }
  return result;
}

[[nodiscard]] std::optional<QJsonObject> canonicalReminders(const QJsonValue& value) {
  if (!value.isObject()) {
    return std::nullopt;
  }
  const QJsonObject source = value.toObject();
  const QJsonValue useDefault = source.value(QStringLiteral("useDefault"));
  const QJsonValue overrides = source.value(QStringLiteral("overrides"));
  if (!useDefault.isBool() || !overrides.isArray() ||
      overrides.toArray().size() > kMaximumReminderCount) {
    return std::nullopt;
  }
  QJsonArray canonicalOverrides;
  for (const QJsonValue& reminderValue : overrides.toArray()) {
    if (!reminderValue.isObject()) {
      return std::nullopt;
    }
    const QJsonObject reminder = reminderValue.toObject();
    const QJsonValue method = reminder.value(QStringLiteral("method"));
    const QJsonValue minutes = reminder.value(QStringLiteral("minutes"));
    if (!method.isString() || (method.toString() != QStringLiteral("email") &&
                               method.toString() != QStringLiteral("popup")) ||
        !minutes.isDouble() || minutes.toInteger(-1) < 0 || minutes.toInteger(-1) > 40'320) {
      return std::nullopt;
    }
    canonicalOverrides.append(QJsonObject{{QStringLiteral("method"), method.toString()},
                                          {QStringLiteral("minutes"), minutes.toInteger()}});
  }
  return QJsonObject{{QStringLiteral("useDefault"), useDefault.toBool()},
                     {QStringLiteral("overrides"), canonicalOverrides}};
}

[[nodiscard]] std::optional<QJsonObject> canonicalConferenceData(const QJsonValue& value) {
  if (!value.isObject()) {
    return std::nullopt;
  }
  const QJsonObject source = value.toObject();
  const QJsonValue requestValue = source.value(QStringLiteral("createRequest"));
  if (!requestValue.isObject()) {
    return std::nullopt;
  }
  const QJsonObject request = requestValue.toObject();
  const QJsonValue requestId = request.value(QStringLiteral("requestId"));
  const QJsonValue solutionKey = request.value(QStringLiteral("conferenceSolutionKey"));
  if (!requestId.isString() || !isValidRequiredText(requestId.toString(), 128) ||
      !solutionKey.isObject() ||
      solutionKey.toObject().value(QStringLiteral("type")).toString() !=
          QStringLiteral("hangoutsMeet")) {
    return std::nullopt;
  }
  QJsonObject canonicalRequest{
      {QStringLiteral("requestId"), requestId.toString()},
      {QStringLiteral("conferenceSolutionKey"),
       QJsonObject{{QStringLiteral("type"), QStringLiteral("hangoutsMeet")}}}};
  return QJsonObject{{QStringLiteral("createRequest"), canonicalRequest}};
}

[[nodiscard]] std::optional<QJsonArray> canonicalAttachments(const QJsonValue& value) {
  if (!value.isArray() || value.toArray().size() > kMaximumAttachmentCount) {
    return std::nullopt;
  }
  QJsonArray canonical;
  for (const QJsonValue& attachmentValue : value.toArray()) {
    if (!attachmentValue.isObject()) {
      return std::nullopt;
    }
    const QJsonObject attachment = attachmentValue.toObject();
    const QJsonValue fileUrl = attachment.value(QStringLiteral("fileUrl"));
    const QJsonValue title = attachment.value(QStringLiteral("title"));
    const QJsonValue mimeType = attachment.value(QStringLiteral("mimeType"));
    if (!fileUrl.isString() || fileUrl.toString().size() > kMaximumAttachmentUrlLength ||
        !fileUrl.toString().startsWith(QStringLiteral("https://")) ||
        (!title.isUndefined() && (!title.isString() || title.toString().size() > 1'024)) ||
        (!mimeType.isUndefined() && (!mimeType.isString() || mimeType.toString().size() > 256))) {
      return std::nullopt;
    }
    QJsonObject item{{QStringLiteral("fileUrl"), fileUrl.toString()}};
    if (title.isString()) {
      item.insert(QStringLiteral("title"), title.toString());
    }
    if (mimeType.isString()) {
      item.insert(QStringLiteral("mimeType"), mimeType.toString());
    }
    canonical.append(item);
  }
  return canonical;
}

[[nodiscard]] bool isStatusEventType(const QString& eventType) {
  return eventType == QStringLiteral("focusTime") || eventType == QStringLiteral("outOfOffice") ||
         eventType == QStringLiteral("workingLocation");
}

[[nodiscard]] bool isSupportedEventType(const QString& eventType) {
  return eventType == QStringLiteral("default") || isStatusEventType(eventType);
}

[[nodiscard]] std::optional<QString> canonicalSendUpdates(const QJsonObject& payload) {
  const QJsonValue sendUpdates = payload.value(QStringLiteral("sendUpdates"));
  if (sendUpdates.isUndefined()) {
    return QStringLiteral("all");
  }
  if (!sendUpdates.isString() || (sendUpdates.toString() != QStringLiteral("all") &&
                                  sendUpdates.toString() != QStringLiteral("externalOnly") &&
                                  sendUpdates.toString() != QStringLiteral("none"))) {
    return std::nullopt;
  }
  return sendUpdates.toString();
}

[[nodiscard]] std::optional<QJsonArray> canonicalRecurrence(const QJsonValue& value, bool creating) {
  constexpr qsizetype kMaximumRecurrenceLineCount = 128;
  constexpr qsizetype kMaximumRecurrenceLineLength = 4'096;
  constexpr qsizetype kMaximumRecurrenceLength = 524'416;
  if (!value.isArray() || value.toArray().size() > kMaximumRecurrenceLineCount) {
    return std::nullopt;
  }
  QJsonArray result;
  qsizetype totalLength = 0;
  for (const QJsonValue& item : value.toArray()) {
    if (!item.isString()) {
      return std::nullopt;
    }
    const QString line = item.toString();
    const qsizetype separator = line.indexOf(u':');
    if (line.isEmpty() || line != line.trimmed() || line.size() > kMaximumRecurrenceLineLength ||
        separator <= 0 || separator == line.size() - 1 || line.contains(QChar::Null) ||
        totalLength > kMaximumRecurrenceLength - line.size()) {
      return std::nullopt;
    }
    totalLength += line.size();
    const QString name = line.first(separator);
    const QString property = name.section(u';', 0, 0);
    if (property != QStringLiteral("RRULE") && property != QStringLiteral("EXDATE") &&
        property != QStringLiteral("RDATE") && property != QStringLiteral("EXRULE")) {
      return std::nullopt;
    }
    if (property == QStringLiteral("RRULE")) {
      if (!QRegularExpression(QStringLiteral("(?:^|;)FREQ=[A-Z]+(?:;|$)"))
               .match(line.sliced(separator + 1))
               .hasMatch()) {
        return std::nullopt;
      }
    }
    result.append(line);
  }
  return !creating || !result.isEmpty() ? std::optional<QJsonArray>(result) : std::nullopt;
}

[[nodiscard]] std::optional<QJsonObject> canonicalEvent(const QJsonObject& payload, bool creating) {
  const QJsonValue eventValue = payload.value(QStringLiteral("event"));
  if (!eventValue.isObject()) {
    return std::nullopt;
  }
  const QJsonObject event = eventValue.toObject();
  QJsonObject result;
  const QJsonValue summary = event.value(QStringLiteral("summary"));
  if (!summary.isUndefined()) {
    if (!summary.isString()) {
      return std::nullopt;
    }
    const QString text = summary.toString().trimmed();
    if (!isValidRequiredText(text, kMaximumTitleLength)) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("summary"), text);
  }
  if (creating && !result.contains(QStringLiteral("summary"))) {
    return std::nullopt;
  }
  for (const auto& [key, maximumLength] :
       {std::pair<QStringView, qsizetype>{u"description", kMaximumDescriptionLength},
        {u"location", kMaximumLocationLength}}) {
    const QJsonValue value = event.value(key);
    if (value.isUndefined()) {
      continue;
    }
    if (value.isNull()) {
      result.insert(key.toString(), QJsonValue::Null);
    } else if (value.isString() && isValidOptionalText(value.toString(), maximumLength)) {
      result.insert(key.toString(), value.toString());
    } else {
      return std::nullopt;
    }
  }
  const QJsonValue colorId = event.value(QStringLiteral("colorId"));
  if (!colorId.isUndefined()) {
    if (colorId.isNull()) {
      if (creating) {
        return std::nullopt;
      }
      result.insert(QStringLiteral("colorId"), QJsonValue::Null);
    } else if (!colorId.isString() || !isValidRequiredText(colorId.toString(), 32)) {
      return std::nullopt;
    } else {
      result.insert(QStringLiteral("colorId"), colorId.toString());
    }
  }
  const QJsonValue transparency = event.value(QStringLiteral("transparency"));
  if (!transparency.isUndefined()) {
    if (!transparency.isString() ||
        (transparency.toString() != QStringLiteral("opaque") &&
         transparency.toString() != QStringLiteral("transparent"))) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("transparency"), transparency.toString());
  }
  const QJsonValue visibility = event.value(QStringLiteral("visibility"));
  if (!visibility.isUndefined()) {
    if (!visibility.isString() ||
        (visibility.toString() != QStringLiteral("default") &&
         visibility.toString() != QStringLiteral("public") &&
         visibility.toString() != QStringLiteral("private") &&
         visibility.toString() != QStringLiteral("confidential"))) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("visibility"), visibility.toString());
  }
  const QJsonValue startValue = event.value(QStringLiteral("start"));
  const QJsonValue endValue = event.value(QStringLiteral("end"));
  const std::optional<CanonicalEventTime> start =
      startValue.isUndefined() ? std::optional<CanonicalEventTime>{} : canonicalTime(startValue);
  const std::optional<CanonicalEventTime> end =
      endValue.isUndefined() ? std::optional<CanonicalEventTime>{} : canonicalTime(endValue);
  if ((startValue.isUndefined() != endValue.isUndefined()) ||
      (creating && (!start.has_value() || !end.has_value())) ||
      (!startValue.isUndefined() && (!start.has_value() || !end.has_value())) ||
      (start.has_value() && end.has_value() &&
       (start->allDay != end->allDay || end->at <= start->at))) {
    return std::nullopt;
  }
  if (start.has_value() && end.has_value()) {
    result.insert(QStringLiteral("start"), start->json);
    result.insert(QStringLiteral("end"), end->json);
  }
  const QJsonValue recurrence = event.value(QStringLiteral("recurrence"));
  if (!recurrence.isUndefined()) {
    const std::optional<QJsonArray> canonical = canonicalRecurrence(recurrence, creating);
    if (!canonical.has_value()) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("recurrence"), *canonical);
  }
  const QJsonValue attendees = event.value(QStringLiteral("attendees"));
  if (!attendees.isUndefined()) {
    const std::optional<QJsonArray> canonical = canonicalAttendees(attendees);
    if (!canonical.has_value()) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("attendees"), *canonical);
  }
  const QJsonValue attendeesOmitted = event.value(QStringLiteral("attendeesOmitted"));
  if (!attendeesOmitted.isUndefined()) {
    if (creating || !attendeesOmitted.isBool() || !attendees.isArray()) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("attendeesOmitted"), attendeesOmitted.toBool());
  }
  const QJsonValue reminders = event.value(QStringLiteral("reminders"));
  if (!reminders.isUndefined()) {
    const std::optional<QJsonObject> canonical = canonicalReminders(reminders);
    if (!canonical.has_value()) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("reminders"), *canonical);
  }
  const QJsonValue conferenceData = event.value(QStringLiteral("conferenceData"));
  if (!conferenceData.isUndefined()) {
    const std::optional<QJsonObject> canonical = canonicalConferenceData(conferenceData);
    if (!canonical.has_value()) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("conferenceData"), *canonical);
  }
  const QJsonValue attachments = event.value(QStringLiteral("attachments"));
  if (!attachments.isUndefined()) {
    const std::optional<QJsonArray> canonical = canonicalAttachments(attachments);
    if (!canonical.has_value()) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("attachments"), *canonical);
  }
  for (const QStringView key : {u"guestsCanInviteOthers", u"guestsCanModify",
                                u"guestsCanSeeOtherGuests"}) {
    const QJsonValue value = event.value(key);
    if (value.isUndefined()) {
      continue;
    }
    if (!value.isBool()) {
      return std::nullopt;
    }
    result.insert(key.toString(), value.toBool());
  }
  const QJsonValue eventType = event.value(QStringLiteral("eventType"));
  if (!eventType.isUndefined()) {
    if (!creating || !eventType.isString() || !isSupportedEventType(eventType.toString())) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("eventType"), eventType.toString());
  }
  const QString effectiveEventType = eventType.isString() ? eventType.toString()
                                                            : QStringLiteral("default");
  const QString statusKey = effectiveEventType == QStringLiteral("focusTime")
                                ? QStringLiteral("focusTimeProperties")
                            : effectiveEventType == QStringLiteral("outOfOffice")
                                ? QStringLiteral("outOfOfficeProperties")
                            : effectiveEventType == QStringLiteral("workingLocation")
                                ? QStringLiteral("workingLocationProperties")
                                : QString();
  int statusPropertyCount = 0;
  for (const QStringView key : {u"focusTimeProperties", u"outOfOfficeProperties",
                                u"workingLocationProperties"}) {
    const QJsonValue value = event.value(key);
    if (value.isUndefined()) {
      continue;
    }
    if ((!statusKey.isEmpty() && key.toString() != statusKey) || !value.isObject() ||
        QJsonDocument(value.toObject()).toJson(QJsonDocument::Compact).size() > 16'384) {
      return std::nullopt;
    }
    ++statusPropertyCount;
    result.insert(key.toString(), value.toObject());
  }
  if (statusPropertyCount > 1) {
    return std::nullopt;
  }
  return !result.isEmpty() ? std::optional<QJsonObject>(result) : std::nullopt;
}

[[nodiscard]] QString eventCollectionPath(const QString& calendarId) {
  return QStringLiteral("/calendar/v3/calendars/") + calendarId + QStringLiteral("/events");
}

[[nodiscard]] EventPushRequestOrError buildRequest(const PendingMutation& mutation) {
  if (mutation.resource != PendingMutationResource::Event) {
    return QStringLiteral("Pending mutation resource is not an event");
  }
  const std::optional<QString> calendarId = requiredIdentifier(mutation.payload, u"calendarId");
  if (!calendarId.has_value()) {
    return QStringLiteral("Pending event mutation payload is invalid");
  }
  if (mutation.operation == QStringLiteral("event.create")) {
    const std::optional<QJsonObject> event = canonicalEvent(mutation.payload, true);
    const std::optional<QString> sendUpdates = canonicalSendUpdates(mutation.payload);
    if (!event.has_value() || !sendUpdates.has_value()) {
      return QStringLiteral("Pending event mutation payload is invalid");
    }
    GoogleHttpRequest request{.method = GoogleHttpMethod::Post,
                              .path = eventCollectionPath(*calendarId),
                              .body = QJsonDocument(*event).toJson(QJsonDocument::Compact)};
    request.query.append({.name = QStringLiteral("sendUpdates"), .value = *sendUpdates});
    if (event->contains(QStringLiteral("conferenceData"))) {
      request.query.append({.name = QStringLiteral("conferenceDataVersion"), .value = QStringLiteral("1")});
    }
    if (event->contains(QStringLiteral("attachments"))) {
      request.query.append({.name = QStringLiteral("supportsAttachments"), .value = QStringLiteral("true")});
    }
    return EventPushRequest{.request = std::move(request)};
  }
  const std::optional<QString> remoteEventId =
      requiredIdentifier(mutation.payload, u"remoteEventId");
  const std::optional<QString> etag =
      mutation.remoteEtag.has_value() ? mutation.remoteEtag : optionalEtag(mutation.payload);
  const QJsonValue etagValue = mutation.payload.value(QStringLiteral("etag"));
  if (!remoteEventId.has_value() || (!mutation.remoteEtag.has_value() && !etag.has_value() &&
                                     !(etagValue.isUndefined() || etagValue.isNull()))) {
    return QStringLiteral("Pending event mutation payload is invalid");
  }
  GoogleHttpRequest request;
  request.path = eventCollectionPath(*calendarId) + QStringLiteral("/") + *remoteEventId;
  request.ifMatch = etag;
  if (mutation.operation == QStringLiteral("event.update")) {
    const std::optional<QJsonObject> event = canonicalEvent(mutation.payload, false);
    const std::optional<QString> sendUpdates = canonicalSendUpdates(mutation.payload);
    if (!event.has_value() || !sendUpdates.has_value()) {
      return QStringLiteral("Pending event mutation payload is invalid");
    }
    request.method = GoogleHttpMethod::Patch;
    request.body = QJsonDocument(*event).toJson(QJsonDocument::Compact);
    request.query.append({.name = QStringLiteral("sendUpdates"), .value = *sendUpdates});
    if (event->contains(QStringLiteral("conferenceData"))) {
      request.query.append({.name = QStringLiteral("conferenceDataVersion"), .value = QStringLiteral("1")});
    }
    if (event->contains(QStringLiteral("attachments"))) {
      request.query.append({.name = QStringLiteral("supportsAttachments"), .value = QStringLiteral("true")});
    }
    return EventPushRequest{.request = std::move(request)};
  }
  if (mutation.operation == QStringLiteral("event.move")) {
    const std::optional<QString> sourceCalendarId =
        requiredIdentifier(mutation.payload, u"sourceCalendarId");
    const std::optional<QString> destinationCalendarId =
        requiredIdentifier(mutation.payload, u"destinationCalendarId");
    if (!sourceCalendarId.has_value() || !destinationCalendarId.has_value()) {
      return QStringLiteral("Pending event move mutation payload is invalid");
    }
    request.method = GoogleHttpMethod::Post;
    request.path = eventCollectionPath(*sourceCalendarId) + QStringLiteral("/") + *remoteEventId +
                   QStringLiteral("/move");
    request.query.append({.name = QStringLiteral("destination"), .value = *destinationCalendarId});
    return EventPushRequest{.request = std::move(request)};
  }
  if (mutation.operation == QStringLiteral("event.delete")) {
    request.method = GoogleHttpMethod::Delete;
    return EventPushRequest{.request = std::move(request)};
  }
  return QStringLiteral("Pending event mutation operation is invalid");
}

[[nodiscard]] bool isInstanceMutation(const PendingMutation& mutation) {
  return mutation.operation == QStringLiteral("event.instance.update") ||
         mutation.operation == QStringLiteral("event.instance.delete");
}

[[nodiscard]] std::optional<QDateTime> canonicalTimestamp(const QString& value) {
  if (value.size() > 64 || value.contains(QChar::Null) || !value.contains(u'T')) {
    return std::nullopt;
  }
  const QDateTime parsed = QDateTime::fromString(value, Qt::ISODateWithMs);
  return parsed.isValid() ? std::optional<QDateTime>(parsed.toUTC()) : std::nullopt;
}

[[nodiscard]] ResolvedInstanceRequestOrError
resolveInstanceRequest(const PendingMutation& mutation,
                       GoogleHttpClient& httpClient,
                       const QString& accessToken) {
  const std::optional<QString> calendarId = requiredIdentifier(mutation.payload, u"calendarId");
  const std::optional<QString> seriesId = requiredIdentifier(mutation.payload, u"recurringRemoteId");
  const QJsonValue originalStartValue = mutation.payload.value(QStringLiteral("originalStartAt"));
  if (!calendarId.has_value() || !seriesId.has_value() || !originalStartValue.isString()) {
    return QStringLiteral("Pending calendar-instance mutation payload is invalid");
  }
  const std::optional<QDateTime> originalStart = canonicalTimestamp(originalStartValue.toString());
  if (!originalStart.has_value()) {
    return QStringLiteral("Pending calendar-instance original start is invalid");
  }
  GoogleHttpRequest lookup{.method = GoogleHttpMethod::Get,
                           .path = eventCollectionPath(*calendarId) + QStringLiteral("/") +
                                   *seriesId + QStringLiteral("/instances"),
                           .query = {{.name = QStringLiteral("originalStart"),
                                      .value = originalStart->toString(Qt::ISODateWithMs)},
                                     {.name = QStringLiteral("maxResults"),
                                      .value = QStringLiteral("1")},
                                     {.name = QStringLiteral("showDeleted"),
                                      .value = QStringLiteral("true")},
                                     {.name = QStringLiteral("fields"),
                                      .value = QStringLiteral(
                                          "items(id,etag,status,recurringEventId,originalStartTime)")}}};
  GoogleHttpResult lookupResult = httpClient.send(std::move(lookup), accessToken).get();
  if (std::holds_alternative<GoogleApiError>(lookupResult)) {
    return std::get<GoogleApiError>(std::move(lookupResult));
  }
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(
      std::get<GoogleHttpResponse>(lookupResult).body, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return QStringLiteral("Google calendar-instance lookup response is invalid");
  }
  const QJsonValue itemsValue = document.object().value(QStringLiteral("items"));
  if (!itemsValue.isArray() || itemsValue.toArray().size() != 1 || !itemsValue.toArray().at(0).isObject()) {
    return QStringLiteral("Google calendar instance is unavailable");
  }
  const QJsonObject instance = itemsValue.toArray().at(0).toObject();
  const std::optional<QString> instanceId = requiredIdentifier(instance, u"id");
  const std::optional<QString> instanceEtag = optionalEtag(instance);
  const std::optional<QString> instanceSeriesId = requiredIdentifier(instance, u"recurringEventId");
  const std::optional<CanonicalEventTime> instanceOriginalStart =
      canonicalTime(instance.value(QStringLiteral("originalStartTime")));
  if (!instanceId.has_value() || !instanceEtag.has_value() || !instanceSeriesId.has_value() ||
      *instanceSeriesId != *seriesId || !instanceOriginalStart.has_value() ||
      instanceOriginalStart->at != *originalStart) {
    return QStringLiteral("Google calendar-instance lookup response is invalid");
  }
  GoogleHttpRequest request;
  request.path = eventCollectionPath(*calendarId) + QStringLiteral("/") + *instanceId;
  request.ifMatch = *instanceEtag;
  if (mutation.operation == QStringLiteral("event.instance.update")) {
    const std::optional<QJsonObject> event = canonicalEvent(mutation.payload, false);
    const std::optional<QString> sendUpdates = canonicalSendUpdates(mutation.payload);
    if (!event.has_value() || !sendUpdates.has_value()) {
      return QStringLiteral("Pending calendar-instance update payload is invalid");
    }
    request.method = GoogleHttpMethod::Patch;
    request.body = QJsonDocument(*event).toJson(QJsonDocument::Compact);
    request.query.append({.name = QStringLiteral("sendUpdates"), .value = *sendUpdates});
    if (event->contains(QStringLiteral("conferenceData"))) {
      request.query.append({.name = QStringLiteral("conferenceDataVersion"), .value = QStringLiteral("1")});
    }
    if (event->contains(QStringLiteral("attachments"))) {
      request.query.append({.name = QStringLiteral("supportsAttachments"), .value = QStringLiteral("true")});
    }
  } else if (mutation.operation == QStringLiteral("event.instance.delete")) {
    request.method = GoogleHttpMethod::Delete;
  } else {
    return QStringLiteral("Pending calendar-instance mutation operation is invalid");
  }
  PendingMutation resolved = mutation;
  resolved.remoteEtag = *instanceEtag;
  resolved.payload.insert(QStringLiteral("remoteEventId"), *instanceId);
  return ResolvedInstanceRequest{.mutation = std::move(resolved), .request = std::move(request)};
}

[[nodiscard]] GoogleEventWriteResponseOrError
decodeWriteResponse(const GoogleHttpResponse& response) {
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(response.body, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return QStringLiteral("Google calendar-event write response is invalid");
  }
  const QJsonObject object = document.object();
  const std::optional<QString> remoteEventId = requiredIdentifier(object, u"id");
  const QJsonValue etagValue = object.value(QStringLiteral("etag"));
  if (!remoteEventId.has_value() ||
      (!etagValue.isUndefined() &&
       (!etagValue.isString() || !isValidEtag(etagValue.toString())))) {
    return QStringLiteral("Google calendar-event write response is invalid");
  }
  return GoogleEventWriteResponse{.remoteEventId = *remoteEventId,
                                  .remoteEtag = etagValue.isString()
                                                    ? std::optional<QString>(etagValue.toString())
                                                    : std::optional<QString>{}};
}

[[nodiscard]] QString errorCode(const GoogleApiError& error) {
  switch (error.kind()) {
  case GoogleApiErrorKind::Unauthorized:
    return QStringLiteral("unauthorized");
  case GoogleApiErrorKind::Forbidden:
    return QStringLiteral("forbidden");
  case GoogleApiErrorKind::NotFound:
    return QStringLiteral("not_found");
  case GoogleApiErrorKind::Conflict:
    return QStringLiteral("conflict");
  case GoogleApiErrorKind::PreconditionFailed:
    return QStringLiteral("precondition_failed");
  case GoogleApiErrorKind::InvalidSyncToken:
    return QStringLiteral("invalid_sync_token");
  case GoogleApiErrorKind::RateLimited:
    return QStringLiteral("rate_limited");
  case GoogleApiErrorKind::Server:
    return QStringLiteral("server");
  case GoogleApiErrorKind::InvalidPayload:
    return QStringLiteral("invalid_payload");
  case GoogleApiErrorKind::Transport:
    return QStringLiteral("transport");
  }
  return QStringLiteral("transport");
}

[[nodiscard]] QString timestampAfter(const Clock& clock, qint64 delayMilliseconds) {
  const auto now =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  const qint64 safeDelay = std::max<qint64>(0, delayMilliseconds);
  const qint64 milliseconds = safeDelay > std::numeric_limits<qint64>::max() - now.count()
                                  ? std::numeric_limits<qint64>::max()
                                  : now.count() + safeDelay;
  return QDateTime::fromMSecsSinceEpoch(milliseconds, QTimeZone::UTC).toString(Qt::ISODateWithMs);
}

[[nodiscard]] std::optional<QString> retryAt(const GoogleApiError& error,
                                             int attemptCount,
                                             const Clock& clock,
                                             const SyncBackoffPolicy& policy) {
  std::optional<qint64> delay = policy.retryDelayMilliseconds(error, attemptCount);
  if (!delay.has_value() && error.kind() == GoogleApiErrorKind::Transport) {
    delay = policy.delayMillisecondsForAttempt(attemptCount);
  }
  return delay.has_value() ? std::optional<QString>(timestampAfter(clock, *delay)) : std::nullopt;
}

[[nodiscard]] std::optional<AppError> markFailure(OptimisticMutationCoordinator& mutations,
                                                  const PendingMutation& mutation,
                                                  const QString& errorCodeValue,
                                                  const QString& message,
                                                  std::optional<QString> nextRetryAt) {
  if (!mutation.leaseId.has_value()) {
    return databaseError(QStringLiteral("Claimed event mutation lease is missing"));
  }
  PendingMutationResult result = mutations
                                     .markFailed({.mutationId = mutation.id,
                                                  .leaseId = *mutation.leaseId,
                                                  .errorCode = errorCodeValue,
                                                  .errorMessage = message.left(4'096),
                                                  .nextRetryAt = std::move(nextRetryAt)})
                                     .get();
  return std::holds_alternative<AppError>(result)
             ? std::optional<AppError>(std::get<AppError>(std::move(result)))
             : std::nullopt;
}

void deferClaimedBatch(OptimisticMutationCoordinator& mutations,
                       const QList<BatchItem>& batchItems,
                       const Clock& clock) {
  for (const BatchItem& item : batchItems) {
    static_cast<void>(markFailure(mutations,
                                  item.mutation,
                                  QStringLiteral("batch_claim_interrupted"),
                                  QStringLiteral("Calendar batch assembly was interrupted"),
                                  timestampAfter(clock, 0)));
  }
}

[[nodiscard]] std::optional<QString> optionalIdentifier(const QJsonObject& object,
                                                        QStringView key) {
  const QJsonValue value = object.value(key);
  if (value.isUndefined() || value.isNull()) {
    return std::optional<QString>{};
  }
  return value.isString() && isValidIdentifier(value.toString(), kMaximumIdentifierLength)
             ? std::optional<QString>(value.toString())
             : std::nullopt;
}

[[nodiscard]] QList<PendingMutation> orderByDependencies(QList<PendingMutation> mutations) {
  QHash<QString, qsizetype> indices;
  for (qsizetype index = 0; index < mutations.size(); ++index) {
    indices.insert(mutations.at(index).id, index);
  }
  QList<PendingMutation> ordered;
  QSet<QString> appended;
  QSet<QString> visiting;
  std::function<void(qsizetype)> append = [&](qsizetype index) {
    const PendingMutation& mutation = mutations.at(index);
    if (appended.contains(mutation.id) || visiting.contains(mutation.id)) {
      return;
    }
    visiting.insert(mutation.id);
    const std::optional<QString> dependency =
        optionalIdentifier(mutation.payload, u"dependsOnMutationId");
    if (dependency.has_value()) {
      const auto prerequisite = indices.constFind(*dependency);
      if (prerequisite != indices.cend()) {
        append(*prerequisite);
      }
    }
    visiting.remove(mutation.id);
    appended.insert(mutation.id);
    ordered.append(mutation);
  };
  for (qsizetype index = 0; index < mutations.size(); ++index) {
    append(index);
  }
  return ordered;
}

[[nodiscard]] QByteArray methodName(GoogleHttpMethod method) {
  switch (method) {
  case GoogleHttpMethod::Get:
    return "GET";
  case GoogleHttpMethod::Post:
    return "POST";
  case GoogleHttpMethod::Patch:
    return "PATCH";
  case GoogleHttpMethod::Put:
    return "PUT";
  case GoogleHttpMethod::Delete:
    return "DELETE";
  }
  return {};
}

[[nodiscard]] std::optional<QByteArray> batchRequestTarget(const GoogleHttpRequest& request) {
  const std::optional<QUrl> url = GoogleHttpClient::buildUrl(request);
  if (!url.has_value()) {
    return std::nullopt;
  }
  QByteArray target = url->path(QUrl::FullyEncoded).toUtf8();
  const QString query = url->query(QUrl::FullyEncoded);
  if (!query.isEmpty()) {
    target.append('?');
    target.append(query.toUtf8());
  }
  return target;
}

[[nodiscard]] std::optional<GoogleHttpRequest>
buildBatchRequest(const QList<BatchItem>& items) {
  if (items.size() < 2 || items.size() > kMaximumGoogleBatchParts) {
    return std::nullopt;
  }
  const QByteArray boundary =
      QByteArray("hcb_") + QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8();
  QByteArray body;
  for (qsizetype index = 0; index < items.size(); ++index) {
    const GoogleHttpRequest& request = items.at(index).request;
    const std::optional<QByteArray> target = batchRequestTarget(request);
    const QByteArray method = methodName(request.method);
    if (!target.has_value() || method.isEmpty()) {
      return std::nullopt;
    }
    body.append("--");
    body.append(boundary);
    body.append("\r\nContent-Type: application/http\r\nContent-ID: <item-");
    body.append(QByteArray::number(index));
    body.append(">\r\n\r\n");
    body.append(method);
    body.append(' ');
    body.append(*target);
    body.append(" HTTP/1.1\r\n");
    if (request.ifMatch.has_value()) {
      body.append("If-Match: ");
      body.append(request.ifMatch->toUtf8());
      body.append("\r\n");
    }
    if (request.body.has_value()) {
      body.append("Content-Type: application/json\r\n");
    }
    body.append("\r\n");
    if (request.body.has_value()) {
      body.append(*request.body);
    }
    body.append("\r\n");
  }
  body.append("--");
  body.append(boundary);
  body.append("--\r\n");
  return GoogleHttpRequest{.method = GoogleHttpMethod::Post,
                           .path = QStringLiteral("/batch/calendar/v3"),
                           .body = std::move(body),
                           .contentType = QByteArray("multipart/mixed; boundary=") + boundary,
                           .accept = QByteArray("multipart/mixed")};
}

[[nodiscard]] std::optional<QHash<QByteArray, QByteArray>>
parseHeaders(const QByteArray& source) {
  QHash<QByteArray, QByteArray> headers;
  const QList<QByteArray> lines = source.split('\n');
  for (QByteArray line : lines) {
    if (line.endsWith('\r')) {
      line.chop(1);
    }
    const qsizetype separator = line.indexOf(':');
    if (separator <= 0) {
      return std::nullopt;
    }
    QByteArray name = line.left(separator).trimmed().toLower();
    const QByteArray value = line.sliced(separator + 1).trimmed();
    if (name.isEmpty() || value.contains('\r') || value.contains('\n') || headers.contains(name)) {
      return std::nullopt;
    }
    headers.insert(std::move(name), value);
  }
  return headers;
}

[[nodiscard]] std::optional<int> batchContentIndex(const QHash<QByteArray, QByteArray>& headers,
                                                    int maximum) {
  QByteArray contentId = headers.value("content-id").trimmed();
  if (contentId.size() >= 2 && contentId.startsWith('<') && contentId.endsWith('>')) {
    contentId = contentId.sliced(1, contentId.size() - 2);
  }
  if (contentId.startsWith("response-")) {
    contentId = contentId.sliced(9);
  }
  if (!contentId.startsWith("item-")) {
    return std::nullopt;
  }
  const QByteArray digits = contentId.sliced(5);
  if (digits.isEmpty() ||
      std::any_of(digits.cbegin(), digits.cend(), [](char value) {
        return value < '0' || value > '9';
      })) {
    return std::nullopt;
  }
  bool valid = false;
  const int index = digits.toInt(&valid);
  return valid && index >= 0 && index < maximum ? std::optional<int>(index) : std::nullopt;
}

[[nodiscard]] std::optional<QList<GoogleHttpResult>>
decodeBatchResponse(const QByteArray& responseBody, int expectedParts) {
  if (responseBody.isEmpty() || responseBody.size() > kMaximumBatchResponseBytes ||
      expectedParts < 2 || expectedParts > kMaximumGoogleBatchParts) {
    return std::nullopt;
  }
  const qsizetype firstLineEnd = responseBody.indexOf("\r\n");
  if (firstLineEnd < 4 || !responseBody.startsWith("--")) {
    return std::nullopt;
  }
  const QByteArray delimiter = responseBody.left(firstLineEnd);
  if (delimiter.size() > 256 || delimiter.contains('\r') || delimiter.contains('\n')) {
    return std::nullopt;
  }
  QList<GoogleHttpResult> results(expectedParts);
  QSet<int> seen;
  qsizetype position = 0;
  while (true) {
    if (!responseBody.sliced(position).startsWith(delimiter)) {
      return std::nullopt;
    }
    position += delimiter.size();
    if (responseBody.sliced(position).startsWith("--")) {
      position += 2;
      if (position < responseBody.size() && !responseBody.sliced(position).startsWith("\r\n")) {
        return std::nullopt;
      }
      break;
    }
    if (!responseBody.sliced(position).startsWith("\r\n")) {
      return std::nullopt;
    }
    position += 2;
    const qsizetype outerHeaderEnd = responseBody.indexOf("\r\n\r\n", position);
    if (outerHeaderEnd < position) {
      return std::nullopt;
    }
    const std::optional<QHash<QByteArray, QByteArray>> outerHeaders =
        parseHeaders(responseBody.sliced(position, outerHeaderEnd - position));
    if (!outerHeaders.has_value() ||
        !outerHeaders->value("content-type").toLower().startsWith("application/http")) {
      return std::nullopt;
    }
    const std::optional<int> index = batchContentIndex(*outerHeaders, expectedParts);
    if (!index.has_value() || seen.contains(*index)) {
      return std::nullopt;
    }
    position = outerHeaderEnd + 4;
    const qsizetype nextDelimiter = responseBody.indexOf("\r\n" + delimiter, position);
    if (nextDelimiter < position) {
      return std::nullopt;
    }
    const QByteArray inner = responseBody.sliced(position, nextDelimiter - position);
    const qsizetype statusLineEnd = inner.indexOf("\r\n");
    const QByteArray statusLine = statusLineEnd < 0 ? inner : inner.left(statusLineEnd);
    if (statusLine.size() < 12) {
      return std::nullopt;
    }
    const QList<QByteArray> statusTokens = statusLine.split(' ');
    if (statusTokens.size() < 2 || !statusTokens.at(0).startsWith("HTTP/") ||
        statusTokens.at(1).size() != 3) {
      return std::nullopt;
    }
    bool statusValid = false;
    const int status = statusTokens.at(1).toInt(&statusValid);
    if (!statusValid || status < 100 || status > 599) {
      return std::nullopt;
    }
    std::optional<QHash<QByteArray, QByteArray>> innerHeaders = QHash<QByteArray, QByteArray>{};
    QByteArray responsePartBody;
    if (statusLineEnd >= 0 && statusLineEnd + 2 < inner.size()) {
      const qsizetype innerHeaderStart = statusLineEnd + 2;
      const qsizetype innerHeaderEnd = inner.indexOf("\r\n\r\n", innerHeaderStart);
      if (innerHeaderEnd >= innerHeaderStart) {
        innerHeaders =
            parseHeaders(inner.sliced(innerHeaderStart, innerHeaderEnd - innerHeaderStart));
        responsePartBody = inner.sliced(innerHeaderEnd + 4);
      } else if (inner.endsWith("\r\n")) {
        innerHeaders = parseHeaders(inner.sliced(
            innerHeaderStart, inner.size() - innerHeaderStart - static_cast<qsizetype>(2)));
      } else {
        return std::nullopt;
      }
    }
    if (!innerHeaders.has_value()) {
      return std::nullopt;
    }
    results[*index] = GoogleHttpClient::decodeResponse(
        status, responsePartBody, innerHeaders->value("retry-after"), innerHeaders->value("date"));
    seen.insert(*index);
    position = nextDelimiter + 2;
  }
  return seen.size() == expectedParts ? std::optional<QList<GoogleHttpResult>>(std::move(results))
                                      : std::nullopt;
}

[[nodiscard]] bool isBatchableEventMutation(const PendingMutation& mutation) {
  const QJsonValue dependency = mutation.payload.value(QStringLiteral("dependsOnMutationId"));
  return mutation.resource == PendingMutationResource::Event &&
         (mutation.operation == QStringLiteral("event.create") ||
          mutation.operation == QStringLiteral("event.update") ||
          mutation.operation == QStringLiteral("event.move") ||
          mutation.operation == QStringLiteral("event.delete")) &&
         (dependency.isUndefined() || dependency.isNull());
}

[[nodiscard]] EventPushOutcomeOrError processEventResponse(
    OptimisticMutationCoordinator& mutations,
    CalendarMutationService* calendarMutationService,
    GoogleSyncConflictResolver* conflictResolver,
    const Clock& clock,
    const SyncBackoffPolicy& backoffPolicy,
    PendingMutation claimed,
    GoogleHttpResult response,
    const QString& accessToken) {
  if (std::holds_alternative<GoogleApiError>(response)) {
    GoogleApiError error = std::get<GoogleApiError>(std::move(response));
    if (isInstanceMutation(claimed) &&
        (error.kind() == GoogleApiErrorKind::Conflict ||
         error.kind() == GoogleApiErrorKind::PreconditionFailed)) {
      if (const std::optional<AppError> failure = markFailure(
              mutations,
              claimed,
              QStringLiteral("recurrence_instance_conflict"),
              QStringLiteral("Google changed this recurring instance; refresh before retrying"),
              std::nullopt);
          failure.has_value()) {
        return *failure;
      }
      return EventPushOutcome{.failed = 1};
    }
    if ((error.kind() == GoogleApiErrorKind::Conflict ||
         error.kind() == GoogleApiErrorKind::PreconditionFailed) &&
        conflictResolver != nullptr) {
      GoogleSyncConflictResult handled =
          conflictResolver->handle(claimed, errorCode(error), error.message(), accessToken);
      if (std::holds_alternative<GoogleSyncConflictOutcome>(handled)) {
        return std::get<GoogleSyncConflictOutcome>(handled) ==
                       GoogleSyncConflictOutcome::AwaitingUser
                   ? EventPushOutcome{.failed = 1}
                   : EventPushOutcome{.skipped = 1};
      }
      if (std::holds_alternative<AppError>(handled)) {
        return std::get<AppError>(std::move(handled));
      }
      error = std::get<GoogleApiError>(std::move(handled));
    }
    const bool createOutcomeUnknown =
        claimed.operation == QStringLiteral("event.create") &&
        (error.kind() == GoogleApiErrorKind::Transport || error.kind() == GoogleApiErrorKind::Server);
    if (const std::optional<AppError> failure = markFailure(
            mutations,
            claimed,
            createOutcomeUnknown ? QStringLiteral("create_outcome_unknown") : errorCode(error),
            createOutcomeUnknown ? QStringLiteral("Google event creation outcome is unknown")
                                 : error.message(),
            createOutcomeUnknown ? std::nullopt
                                 : retryAt(error, claimed.attemptCount, clock, backoffPolicy));
        failure.has_value()) {
      return *failure;
    }
    return EventPushOutcome{.failed = 1};
  }
  if (calendarMutationService != nullptr && claimed.operation != QStringLiteral("event.delete")) {
    const std::optional<QString> localEventId =
        requiredIdentifier(claimed.payload, u"localEventId");
    const GoogleEventWriteResponseOrError written =
        decodeWriteResponse(std::get<GoogleHttpResponse>(response));
    if (!localEventId.has_value() || std::holds_alternative<QString>(written)) {
      if (const std::optional<AppError> failure = markFailure(
              mutations,
              claimed,
              QStringLiteral("invalid_response"),
              localEventId.has_value() ? std::get<QString>(written)
                                       : QStringLiteral("Pending event mutation local ID is invalid"),
              std::nullopt);
          failure.has_value()) {
        return *failure;
      }
      return EventPushOutcome{.failed = 1};
    }
    const GoogleEventWriteResponse& event = std::get<GoogleEventWriteResponse>(written);
    CalendarEventMutationResult reconciled =
        calendarMutationService
            ->reconcileGoogleEvent({.localEventId = *localEventId,
                                    .remoteEventId = event.remoteEventId,
                                    .remoteEtag = event.remoteEtag})
            .get();
    if (std::holds_alternative<AppError>(reconciled)) {
      if (const std::optional<AppError> failure = markFailure(
              mutations,
              claimed,
              QStringLiteral("reconciliation_failed"),
              std::get<AppError>(std::move(reconciled)).message(),
              std::nullopt);
          failure.has_value()) {
        return *failure;
      }
      return EventPushOutcome{.failed = 1};
    }
  }
  if (!claimed.leaseId.has_value()) {
    return databaseError(QStringLiteral("Claimed event mutation lease is missing"));
  }
  PendingMutationResult applied = mutations.markApplied(claimed.id, *claimed.leaseId).get();
  return std::holds_alternative<AppError>(applied)
             ? EventPushOutcomeOrError(std::get<AppError>(std::move(applied)))
             : EventPushOutcomeOrError(EventPushOutcome{.applied = 1});
}

void addOutcome(GoogleCalendarEventMutationPushResult& summary,
                const EventPushOutcome& outcome,
                MutationTelemetryStore* telemetry,
                OptimisticMutationCoordinator& mutations,
                const PendingMutation& mutation) {
  summary.applied += outcome.applied;
  summary.failed += outcome.failed;
  summary.skipped += outcome.skipped;
  if (telemetry == nullptr || (outcome.applied == 0 && outcome.failed == 0)) {
    return;
  }
  std::optional<QString> errorCode;
  const PendingMutationLookupResult stored = mutations.find(mutation.id).get();
  if (std::holds_alternative<std::optional<PendingMutation>>(stored)) {
    const std::optional<PendingMutation>& pending =
        std::get<std::optional<PendingMutation>>(stored);
    if (pending.has_value()) errorCode = pending->lastErrorCode;
  }
  static_cast<void>(telemetry->record(
      {.mutationId = mutation.id,
       .resource = QStringLiteral("event"),
       .operation = mutation.operation,
       .scope = QStringLiteral("none"),
       .phase = outcome.applied > 0 ? MutationTelemetryPhase::RemoteApplied
                                    : MutationTelemetryPhase::RemoteFailed,
       .remoteOutcome = outcome.applied > 0 ? std::optional<QString>(QStringLiteral("applied"))
                                             : std::optional<QString>(QStringLiteral("failed")),
       .errorCode = std::move(errorCode),
       .rollbackReason = outcome.failed > 0 ? std::optional<QString>(QStringLiteral("keep_retry"))
                                            : std::nullopt}));
}

} // namespace

GoogleCalendarEventMutationPushService::GoogleCalendarEventMutationPushService(
    OptimisticMutationCoordinator& mutations,
    GoogleHttpClient& httpClient,
    const Clock& clock,
    SyncBackoffPolicy backoffPolicy,
    CalendarMutationService* calendarMutationService,
    GoogleSyncConflictResolver* conflictResolver,
    MutationTelemetryStore* mutationTelemetryStore)
    : mutations_(mutations), httpClient_(httpClient), clock_(clock),
      backoffPolicy_(std::move(backoffPolicy)), calendarMutationService_(calendarMutationService),
      conflictResolver_(conflictResolver), mutationTelemetryStore_(mutationTelemetryStore) {}

std::future<GoogleCalendarEventMutationPushResultOrError>
GoogleCalendarEventMutationPushService::pushDue(QString accessToken, int limit) {
  auto completion = std::make_shared<std::promise<GoogleCalendarEventMutationPushResultOrError>>();
  std::future<GoogleCalendarEventMutationPushResultOrError> future = completion->get_future();
  const int cappedLimit = std::clamp(limit, 1, kMaximumPushBatch);
  try {
    std::thread([this, accessToken = std::move(accessToken), cappedLimit, completion] {
      try {
        PendingMutationListResult dueResult = mutations_.listDue(kMaximumPushBatch).get();
        if (std::holds_alternative<AppError>(dueResult)) {
          completion->set_value(std::get<AppError>(std::move(dueResult)));
          return;
        }
        GoogleCalendarEventMutationPushResult summary;
        int processed = 0;
        const QList<PendingMutation> due =
            orderByDependencies(std::get<QList<PendingMutation>>(std::move(dueResult)));
        for (qsizetype dueIndex = 0; dueIndex < due.size();) {
          const PendingMutation& pending = due.at(dueIndex);
          if (pending.resource != PendingMutationResource::Event) {
            ++summary.skipped;
            ++dueIndex;
            continue;
          }
          if (processed >= cappedLimit) {
            break;
          }
          if (isBatchableEventMutation(pending)) {
            QList<BatchItem> batchItems;
            QSet<QString> resourceIds;
            qsizetype batchEnd = dueIndex;
            while (batchEnd < due.size() && processed < cappedLimit &&
                   batchItems.size() < kMaximumGoogleBatchParts) {
              const PendingMutation& candidate = due.at(batchEnd);
              if (!isBatchableEventMutation(candidate) || resourceIds.contains(candidate.resourceId)) {
                break;
              }
              resourceIds.insert(candidate.resourceId);
              ++processed;
              ++batchEnd;
              PendingMutationResult claim =
                  mutations_.claim(candidate.id, kMutationLeaseDuration).get();
              if (std::holds_alternative<AppError>(claim)) {
                deferClaimedBatch(mutations_, batchItems, clock_);
                completion->set_value(std::get<AppError>(std::move(claim)));
                return;
              }
              PendingMutation claimed = std::get<PendingMutation>(std::move(claim));
              const EventPushRequestOrError request = buildRequest(claimed);
              if (std::holds_alternative<QString>(request)) {
                const std::optional<AppError> failure = markFailure(mutations_,
                                                                    claimed,
                                                                    QStringLiteral("invalid_payload"),
                                                                    std::get<QString>(request),
                                                                    std::nullopt);
                if (failure.has_value()) {
                  completion->set_value(*failure);
                  return;
                }
                ++summary.failed;
                continue;
              }
              batchItems.append(
                  {.mutation = std::move(claimed),
                   .request = std::get<EventPushRequest>(request).request});
            }
            if (batchItems.size() == 1) {
              GoogleHttpResult response =
                  httpClient_.send(batchItems.constFirst().request, accessToken).get();
              EventPushOutcomeOrError outcome = processEventResponse(mutations_,
                                                                       calendarMutationService_,
                                                                       conflictResolver_,
                                                                       clock_,
                                                                       backoffPolicy_,
                                                                       batchItems.constFirst().mutation,
                                                                       std::move(response),
                                                                       accessToken);
              if (std::holds_alternative<AppError>(outcome)) {
                completion->set_value(std::get<AppError>(std::move(outcome)));
                return;
              }
              addOutcome(summary, std::get<EventPushOutcome>(outcome), mutationTelemetryStore_, mutations_,
                         batchItems.constFirst().mutation);
            } else if (batchItems.size() > 1) {
              const std::optional<GoogleHttpRequest> batchRequest = buildBatchRequest(batchItems);
              QList<GoogleHttpResult> responses;
              if (batchRequest.has_value()) {
                GoogleHttpResult batchResponse = httpClient_.send(*batchRequest, accessToken).get();
                if (std::holds_alternative<GoogleApiError>(batchResponse)) {
                  for (qsizetype itemIndex = 0; itemIndex < batchItems.size(); ++itemIndex) {
                    responses.append(std::get<GoogleApiError>(batchResponse));
                  }
                } else {
                  const std::optional<QList<GoogleHttpResult>> decoded = decodeBatchResponse(
                      std::get<GoogleHttpResponse>(batchResponse).body,
                      static_cast<int>(batchItems.size()));
                  if (decoded.has_value()) {
                    responses = std::move(*decoded);
                  } else {
                    const GoogleApiError malformed({.kind = GoogleApiErrorKind::InvalidPayload,
                                                    .message = QStringLiteral(
                                                        "Google batch response is invalid")});
                    for (qsizetype itemIndex = 0; itemIndex < batchItems.size(); ++itemIndex) {
                      responses.append(malformed);
                    }
                  }
                }
              } else {
                const GoogleApiError invalid({.kind = GoogleApiErrorKind::InvalidPayload,
                                              .message = QStringLiteral("Google batch request is invalid")});
                for (qsizetype itemIndex = 0; itemIndex < batchItems.size(); ++itemIndex) {
                  responses.append(invalid);
                }
              }
              for (qsizetype itemIndex = 0; itemIndex < batchItems.size(); ++itemIndex) {
                EventPushOutcomeOrError outcome = processEventResponse(mutations_,
                                                                         calendarMutationService_,
                                                                         conflictResolver_,
                                                                         clock_,
                                                                         backoffPolicy_,
                                                                         batchItems.at(itemIndex).mutation,
                                                                         std::move(responses[itemIndex]),
                                                                         accessToken);
                if (std::holds_alternative<AppError>(outcome)) {
                  completion->set_value(std::get<AppError>(std::move(outcome)));
                  return;
                }
                addOutcome(summary, std::get<EventPushOutcome>(outcome), mutationTelemetryStore_, mutations_,
                           batchItems.at(itemIndex).mutation);
              }
            }
            dueIndex = batchEnd;
            continue;
          }
          ++processed;
          PendingMutationResult claim = mutations_.claim(pending.id, kMutationLeaseDuration).get();
          if (std::holds_alternative<AppError>(claim)) {
            completion->set_value(std::get<AppError>(std::move(claim)));
            return;
          }
          PendingMutation claimed = std::get<PendingMutation>(std::move(claim));
          const std::optional<QString> dependency =
              optionalIdentifier(claimed.payload, u"dependsOnMutationId");
          const QJsonValue dependencyValue =
              claimed.payload.value(QStringLiteral("dependsOnMutationId"));
          if (!dependency.has_value() &&
              !(dependencyValue.isUndefined() || dependencyValue.isNull())) {
            const std::optional<AppError> failure = markFailure(
                mutations_,
                claimed,
                QStringLiteral("invalid_payload"),
                QStringLiteral("Pending event mutation dependency is invalid"),
                std::nullopt);
            if (failure.has_value()) {
              completion->set_value(*failure);
              return;
            }
            ++summary.failed;
            ++dueIndex;
            continue;
          }
          if (dependency.has_value()) {
            PendingMutationLookupResult dependencyResult = mutations_.find(*dependency).get();
            if (std::holds_alternative<AppError>(dependencyResult)) {
              completion->set_value(std::get<AppError>(std::move(dependencyResult)));
              return;
            }
            const std::optional<PendingMutation>& prerequisite =
                std::get<std::optional<PendingMutation>>(dependencyResult);
            if (!prerequisite.has_value() ||
                prerequisite->status == PendingMutationStatus::Cancelled) {
              const std::optional<AppError> failure = markFailure(
                  mutations_,
                  claimed,
                  QStringLiteral("dependency_failed"),
                  QStringLiteral("Pending event mutation prerequisite is unavailable"),
                  std::nullopt);
              if (failure.has_value()) {
                completion->set_value(*failure);
                return;
              }
              ++summary.failed;
              ++dueIndex;
              continue;
            }
            if (prerequisite->status != PendingMutationStatus::Applied) {
              const bool permanent = prerequisite->status == PendingMutationStatus::Failed &&
                                     !prerequisite->nextRetryAt.has_value();
              const std::optional<AppError> failure = markFailure(
                  mutations_,
                  claimed,
                  permanent ? QStringLiteral("dependency_failed")
                            : QStringLiteral("dependency_pending"),
                  permanent ? QStringLiteral("Pending event mutation prerequisite failed")
                            : QStringLiteral("Pending event mutation prerequisite is pending"),
                  permanent ? std::nullopt
                            : prerequisite->nextRetryAt.has_value()
                                  ? prerequisite->nextRetryAt
                                  : std::optional<QString>(timestampAfter(clock_, 0)));
              if (failure.has_value()) {
                completion->set_value(*failure);
                return;
              }
              if (permanent) {
                ++summary.failed;
              } else {
                ++summary.skipped;
              }
              ++dueIndex;
              continue;
            }
          }
          PendingMutation responseMutation = claimed;
          GoogleHttpRequest request;
          if (isInstanceMutation(claimed)) {
            ResolvedInstanceRequestOrError resolved =
                resolveInstanceRequest(claimed, httpClient_, accessToken);
            if (std::holds_alternative<GoogleApiError>(resolved)) {
              const PendingMutation telemetryMutation = claimed;
              EventPushOutcomeOrError outcome = processEventResponse(mutations_,
                                                                       calendarMutationService_,
                                                                       conflictResolver_,
                                                                       clock_,
                                                                       backoffPolicy_,
                                                                       std::move(claimed),
                                                                       std::get<GoogleApiError>(std::move(resolved)),
                                                                       accessToken);
              if (std::holds_alternative<AppError>(outcome)) {
                completion->set_value(std::get<AppError>(std::move(outcome)));
                return;
              }
              addOutcome(summary, std::get<EventPushOutcome>(outcome), mutationTelemetryStore_, mutations_,
                         telemetryMutation);
              ++dueIndex;
              continue;
            }
            if (std::holds_alternative<QString>(resolved)) {
              const std::optional<AppError> failure = markFailure(
                  mutations_,
                  claimed,
                  QStringLiteral("instance_resolution_failed"),
                  std::get<QString>(resolved),
                  std::nullopt);
              if (failure.has_value()) {
                completion->set_value(*failure);
                return;
              }
              ++summary.failed;
              ++dueIndex;
              continue;
            }
            ResolvedInstanceRequest instance = std::get<ResolvedInstanceRequest>(std::move(resolved));
            responseMutation = std::move(instance.mutation);
            request = std::move(instance.request);
          } else {
            const EventPushRequestOrError built = buildRequest(claimed);
            if (std::holds_alternative<QString>(built)) {
              const std::optional<AppError> failure = markFailure(mutations_,
                                                                  claimed,
                                                                  QStringLiteral("invalid_payload"),
                                                                  std::get<QString>(built),
                                                                  std::nullopt);
              if (failure.has_value()) {
                completion->set_value(*failure);
                return;
              }
              ++summary.failed;
              ++dueIndex;
              continue;
            }
            request = std::get<EventPushRequest>(built).request;
          }
          GoogleHttpResult response = httpClient_.send(std::move(request), accessToken).get();
          EventPushOutcomeOrError outcome = processEventResponse(mutations_,
                                                                   calendarMutationService_,
                                                                   conflictResolver_,
                                                                   clock_,
                                                                   backoffPolicy_,
                                                                   std::move(responseMutation),
                                                                   std::move(response),
                                                                   accessToken);
          if (std::holds_alternative<AppError>(outcome)) {
            completion->set_value(std::get<AppError>(std::move(outcome)));
            return;
          }
          addOutcome(summary, std::get<EventPushOutcome>(outcome), mutationTelemetryStore_, mutations_,
                     claimed);
          ++dueIndex;
        }
        completion->set_value(summary);
      } catch (...) {
        completion->set_value(networkError(QStringLiteral("Google event mutation push failed")));
      }
    }).detach();
  } catch (...) {
    completion->set_value(networkError(QStringLiteral("Google event mutation push failed")));
  }
  return future;
}

} // namespace hcb
