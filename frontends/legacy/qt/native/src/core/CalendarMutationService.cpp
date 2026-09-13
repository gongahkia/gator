#include "core/CalendarMutationService.h"

#include "core/RecurrenceExpansionWorker.h"

#include "data/LocalSchema.h"
#include "data/SqliteTransaction.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDate>
#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QRegularExpression>
#include <QSet>
#include <QString>
#include <QTimeZone>
#include <QUuid>

#include <chrono>
#include <future>
#include <initializer_list>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumTitleLength = 500;
constexpr qsizetype kMaximumDescriptionLength = 20'000;
constexpr qsizetype kMaximumLocationLength = 1'000;
constexpr qsizetype kMaximumTimeZoneLength = 120;
constexpr qsizetype kMaximumTimestampLength = 64;
constexpr qsizetype kMaximumColorIdLength = 32;
constexpr qsizetype kMaximumAttendeeCount = 200;
constexpr qsizetype kMaximumReminderCount = 5;
constexpr qsizetype kMaximumAttachmentCount = 25;
constexpr qsizetype kMaximumConferenceJsonBytes = 32'768;
constexpr qsizetype kMaximumAttachmentsJsonBytes = 65'536;
constexpr qsizetype kMaximumEventPropertiesJsonBytes = 16'384;
constexpr char kConflictMetadataKey[] = "_hcbSync";

struct StoredEventContext final {
  QString eventId;
  QString accountId;
  QString calendarId;
  QString calendarRemoteId;
  std::optional<QString> calendarAccessRole;
  QString remoteId;
  std::optional<QString> remoteEtag;
  QString title;
  std::optional<QString> description;
  std::optional<QString> location;
  QString startAt;
  std::optional<QString> startTimeZone;
  QString endAt;
  std::optional<QString> endTimeZone;
  bool allDay{false};
  std::optional<QString> recurrenceRule;
  std::optional<QString> recurringRemoteId;
  std::optional<QString> originalStartAt;
  QString status;
  std::optional<QString> colorId;
  std::optional<QString> transparency;
  std::optional<QString> visibility;
  std::optional<QString> eventType;
  QString attendeeEmailsJson;
  QString attendeeDetailsJson;
  QString remindersJson;
  bool remindersUseDefault{true};
  std::optional<QString> conferenceJson;
  QString attachmentsJson;
  QString guestPermissionsJson;
  QString statusPropertiesJson;
  bool calendarPrimary{false};
};

struct ActiveEventMutation final {
  QString id;
  QString operation;
  QJsonObject payload;
};

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] bool isValidRequiredText(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximumLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isValidOptionalText(const std::optional<QString>& value,
                                       qsizetype maximumLength) {
  return !value.has_value() || (value->size() <= maximumLength && !value->contains(QChar::Null));
}

[[nodiscard]] std::optional<QString> canonicalTimestamp(const QString& value) {
  if (!isValidRequiredText(value, kMaximumTimestampLength) || !value.contains(u'T')) {
    return std::nullopt;
  }
  const QDateTime parsed = QDateTime::fromString(value, Qt::ISODateWithMs);
  if (!parsed.isValid()) {
    return std::nullopt;
  }
  return parsed.toUTC().toString(Qt::ISODateWithMs);
}

[[nodiscard]] bool isValidTimeZone(const std::optional<QString>& value) {
  return !value.has_value() || (isValidRequiredText(*value, kMaximumTimeZoneLength) &&
                                QTimeZone(value->toUtf8()).isValid());
}

[[nodiscard]] bool isValidColorId(const QString& value) {
  return isValidRequiredText(value, kMaximumColorIdLength);
}

[[nodiscard]] bool isValidTransparency(const QString& value) {
  return value == QStringLiteral("opaque") || value == QStringLiteral("transparent");
}

[[nodiscard]] bool isValidVisibility(const QString& value) {
  return value == QStringLiteral("default") || value == QStringLiteral("public") ||
         value == QStringLiteral("private") || value == QStringLiteral("confidential");
}

[[nodiscard]] bool isValidEmail(const QString& value) {
  static const QRegularExpression pattern(
      QStringLiteral("^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$"));
  return value.size() <= 254 && !value.contains(QChar::Null) && pattern.match(value).hasMatch();
}

[[nodiscard]] std::optional<QList<QString>> canonicalAttendees(QList<QString> emails) {
  if (emails.size() > kMaximumAttendeeCount) {
    return std::nullopt;
  }
  QSet<QString> seen;
  QList<QString> canonical;
  canonical.reserve(emails.size());
  for (QString& email : emails) {
    email = email.trimmed();
    const QString key = email.toCaseFolded();
    if (!isValidEmail(email) || seen.contains(key)) {
      return std::nullopt;
    }
    seen.insert(key);
    canonical.append(std::move(email));
  }
  return canonical;
}

[[nodiscard]] bool isValidReminders(const CalendarEventReminderSettings& reminders) {
  if (reminders.overrides.size() > kMaximumReminderCount) {
    return false;
  }
  for (const CalendarEventReminder& reminder : reminders.overrides) {
    if ((reminder.method != QStringLiteral("email") && reminder.method != QStringLiteral("popup")) ||
        reminder.minutes < 0 || reminder.minutes > 40'320) {
      return false;
    }
  }
  return true;
}

[[nodiscard]] std::optional<QString> canonicalRecurrenceRule(
    const std::optional<QString>& recurrenceRule) {
  if (!recurrenceRule.has_value()) {
    return std::optional<QString>{};
  }
  constexpr qsizetype kMaximumRecurrenceLength = 524'416;
  constexpr qsizetype kMaximumRecurrenceLineLength = 4'096;
  constexpr qsizetype kMaximumRecurrenceLineCount = 128;
  if (recurrenceRule->isEmpty() || recurrenceRule->size() > kMaximumRecurrenceLength ||
      recurrenceRule->contains(QChar::Null)) {
    return std::nullopt;
  }
  const QStringList lines = recurrenceRule->split(u'\n', Qt::KeepEmptyParts);
  if (lines.size() > kMaximumRecurrenceLineCount) {
    return std::nullopt;
  }
  for (const QString& line : lines) {
    const qsizetype separator = line.indexOf(u':');
    if (line.isEmpty() || separator <= 0 || separator == line.size() - 1 ||
        line != line.trimmed() || line.size() > kMaximumRecurrenceLineLength) {
      return std::nullopt;
    }
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
  }
  return recurrenceRule;
}

[[nodiscard]] QJsonArray recurrenceLines(const std::optional<QString>& recurrenceRule) {
  QJsonArray result;
  if (!recurrenceRule.has_value()) {
    return result;
  }
  for (const QString& line : recurrenceRule->split(u'\n', Qt::SkipEmptyParts)) {
    result.append(line);
  }
  return result;
}

[[nodiscard]] std::optional<QString> truncateRecurrenceRule(const QString& recurrenceRule,
                                                             const QString& targetStart,
                                                             bool allDay) {
  const std::optional<QString> canonical = canonicalRecurrenceRule(recurrenceRule);
  if (!canonical.has_value() || canonical->contains(u'\n')) {
    return std::nullopt;
  }
  const QDateTime target = QDateTime::fromString(targetStart, Qt::ISODateWithMs);
  if (!target.isValid()) {
    return std::nullopt;
  }
  QStringList fields;
  for (const QString& part : canonical->sliced(6).split(u';', Qt::SkipEmptyParts)) {
    if (!part.startsWith(QStringLiteral("COUNT=")) && !part.startsWith(QStringLiteral("UNTIL="))) {
      fields.append(part);
    }
  }
  const QString until = allDay
                            ? target.toUTC().date().addDays(-1).toString(QStringLiteral("yyyyMMdd"))
                            : target.toUTC()
                                  .addMSecs(-1'000)
                                  .toString(QStringLiteral("yyyyMMdd'T'hhmmss'Z'"));
  fields.append(QStringLiteral("UNTIL=") + until);
  const std::optional<QString> result =
      canonicalRecurrenceRule(QStringLiteral("RRULE:") + fields.join(u';'));
  return result.has_value() ? result : std::nullopt;
}

[[nodiscard]] QString compactJson(const QJsonArray& value) {
  return QString::fromUtf8(QJsonDocument(value).toJson(QJsonDocument::Compact));
}

[[nodiscard]] QString compactJson(const QJsonObject& value) {
  return QString::fromUtf8(QJsonDocument(value).toJson(QJsonDocument::Compact));
}

[[nodiscard]] std::optional<QJsonObject> boundedJsonObject(const QString& value,
                                                            qsizetype maximumBytes) {
  if (value.toUtf8().size() > maximumBytes) {
    return std::nullopt;
  }
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(value.toUtf8(), &error);
  return error.error == QJsonParseError::NoError && document.isObject()
             ? std::optional<QJsonObject>(document.object())
             : std::nullopt;
}

[[nodiscard]] std::optional<QJsonArray> boundedJsonArray(const QString& value,
                                                          qsizetype maximumBytes,
                                                          qsizetype maximumItems) {
  if (value.toUtf8().size() > maximumBytes) {
    return std::nullopt;
  }
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(value.toUtf8(), &error);
  return error.error == QJsonParseError::NoError && document.isArray() &&
                 document.array().size() <= maximumItems
             ? std::optional<QJsonArray>(document.array())
             : std::nullopt;
}

[[nodiscard]] bool isValidSendUpdates(const QString& value) {
  return value == QStringLiteral("all") || value == QStringLiteral("externalOnly") ||
         value == QStringLiteral("none");
}

[[nodiscard]] bool isValidResponseStatus(const QString& value) {
  return value == QStringLiteral("needsAction") || value == QStringLiteral("declined") ||
         value == QStringLiteral("tentative") || value == QStringLiteral("accepted");
}

[[nodiscard]] bool isSupportedEventType(const QString& value) {
  return value == QStringLiteral("default") || value == QStringLiteral("focusTime") ||
         value == QStringLiteral("outOfOffice") || value == QStringLiteral("workingLocation");
}

[[nodiscard]] std::optional<QString> canonicalAttachmentsJson(const QString& value) {
  const std::optional<QJsonArray> attachments =
      boundedJsonArray(value, kMaximumAttachmentsJsonBytes, kMaximumAttachmentCount);
  if (!attachments.has_value()) {
    return std::nullopt;
  }
  QJsonArray canonical;
  for (const QJsonValue& attachmentValue : *attachments) {
    if (!attachmentValue.isObject()) {
      return std::nullopt;
    }
    const QJsonObject attachment = attachmentValue.toObject();
    const QJsonValue fileUrl = attachment.value(QStringLiteral("fileUrl"));
    const QJsonValue title = attachment.value(QStringLiteral("title"));
    const QJsonValue mimeType = attachment.value(QStringLiteral("mimeType"));
    if (!fileUrl.isString() || fileUrl.toString().size() > 2'048 ||
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
  return compactJson(canonical);
}

[[nodiscard]] std::optional<QString> canonicalGuestPermissionsJson(const QString& value) {
  const std::optional<QJsonObject> permissions =
      boundedJsonObject(value, kMaximumEventPropertiesJsonBytes);
  if (!permissions.has_value()) {
    return std::nullopt;
  }
  QJsonObject canonical;
  for (const QStringView key : {u"guestsCanInviteOthers", u"guestsCanModify",
                                u"guestsCanSeeOtherGuests"}) {
    const QJsonValue permission = permissions->value(key);
    if (permission.isUndefined()) {
      continue;
    }
    if (!permission.isBool()) {
      return std::nullopt;
    }
    canonical.insert(key.toString(), permission.toBool());
  }
  return compactJson(canonical);
}

[[nodiscard]] std::optional<QString> canonicalStatusPropertiesJson(const QString& value,
                                                                     const QString& eventType) {
  const std::optional<QJsonObject> properties =
      boundedJsonObject(value, kMaximumEventPropertiesJsonBytes);
  if (!properties.has_value()) {
    return std::nullopt;
  }
  if (eventType == QStringLiteral("default") && !properties->isEmpty()) {
    return std::nullopt;
  }
  const QString expected = eventType == QStringLiteral("focusTime")
                               ? QStringLiteral("focusTimeProperties")
                           : eventType == QStringLiteral("outOfOffice")
                               ? QStringLiteral("outOfOfficeProperties")
                           : eventType == QStringLiteral("workingLocation")
                               ? QStringLiteral("workingLocationProperties")
                               : QString();
  if (!expected.isEmpty() && (properties->size() != 1 || !properties->value(expected).isObject())) {
    return std::nullopt;
  }
  return compactJson(*properties);
}

[[nodiscard]] QString googleMeetCreateRequestJson() {
  QJsonObject createRequest{
      {QStringLiteral("requestId"), QUuid::createUuid().toString(QUuid::WithoutBraces)},
      {QStringLiteral("conferenceSolutionKey"),
       QJsonObject{{QStringLiteral("type"), QStringLiteral("hangoutsMeet")}}}};
  return compactJson(QJsonObject{{QStringLiteral("createRequest"), createRequest}});
}

[[nodiscard]] QJsonArray attendeeDetails(const QList<QString>& emails,
                                          const QString& previousDetails = QStringLiteral("[]")) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(previousDetails.toUtf8(), &error);
  QHash<QString, QJsonObject> byEmail;
  if (error.error == QJsonParseError::NoError && document.isArray()) {
    for (const QJsonValue& attendee : document.array()) {
      const QJsonObject object = attendee.toObject();
      const QJsonValue email = object.value(QStringLiteral("email"));
      if (email.isString()) {
        byEmail.insert(email.toString().toCaseFolded(), object);
      }
    }
  }
  QJsonArray details;
  for (const QString& email : emails) {
    QJsonObject attendee = byEmail.value(email.toCaseFolded());
    const bool newAttendee = attendee.isEmpty();
    attendee.insert(QStringLiteral("email"), email);
    if (newAttendee) {
      attendee.insert(QStringLiteral("responseStatus"), QStringLiteral("needsAction"));
    }
    details.append(attendee);
  }
  return details;
}

[[nodiscard]] QJsonObject remindersJson(const CalendarEventReminderSettings& reminders) {
  QJsonArray overrides;
  for (const CalendarEventReminder& reminder : reminders.overrides) {
    overrides.append(QJsonObject{{QStringLiteral("method"), reminder.method},
                                 {QStringLiteral("minutes"), reminder.minutes}});
  }
  return {{QStringLiteral("useDefault"), reminders.useDefault},
          {QStringLiteral("overrides"), overrides}};
}

[[nodiscard]] QJsonArray reminderMinutes(const CalendarEventReminderSettings& reminders) {
  QJsonArray minutes;
  for (const CalendarEventReminder& reminder : reminders.overrides) {
    minutes.append(reminder.minutes);
  }
  return minutes;
}

[[nodiscard]] bool isWritableCalendar(const std::optional<QString>& accessRole) {
  return !accessRole.has_value() || *accessRole == QStringLiteral("writer") ||
         *accessRole == QStringLiteral("owner");
}

[[nodiscard]] bool canMoveFromCalendar(const std::optional<QString>& accessRole) {
  return !accessRole.has_value() || *accessRole == QStringLiteral("owner");
}

[[nodiscard]] bool isSupportedEventType(const QString& value);

[[nodiscard]] bool isMutableEventType(const std::optional<QString>& eventType) {
  return !eventType.has_value() || *eventType == QStringLiteral("default");
}

[[nodiscard]] bool isEditableEventType(const std::optional<QString>& eventType) {
  return !eventType.has_value() || isSupportedEventType(*eventType);
}

[[nodiscard]] QString timestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar-event binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
bindOptionalText(sqlite3_stmt* statement, int index, const std::optional<QString>& value) {
  if (!value.has_value()) {
    const int result = sqlite3_bind_null(statement, index);
    if (result != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite calendar-event binding failed (%1)"), result);
    }
    return std::nullopt;
  }
  return bindText(statement, index, *value);
}

[[nodiscard]] std::optional<AppError> bindInteger(sqlite3_stmt* statement, int index, int value) {
  const int result = sqlite3_bind_int(statement, index, value);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar-event binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
bindAll(sqlite3_stmt* statement, const std::initializer_list<std::optional<AppError>>& results) {
  for (const std::optional<AppError>& result : results) {
    if (result.has_value()) {
      sqlite3_finalize(statement);
      return result;
    }
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
writeStoredRecurrence(sqlite3* handle,
                      const QString& eventId,
                      const std::optional<QString>& recurrenceRule) {
  constexpr char insertSql[] = R"(
INSERT INTO local_calendar_event_recurrences(event_id, recurrence_rule) VALUES (?1, ?2)
ON CONFLICT(event_id) DO UPDATE SET recurrence_rule = excluded.recurrence_rule
)";
  constexpr char deleteSql[] = R"(
DELETE FROM local_calendar_event_recurrences WHERE event_id = ?1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepared = sqlite3_prepare_v3(handle,
                                          recurrenceRule.has_value() ? insertSql : deleteSql,
                                          -1,
                                          SQLITE_PREPARE_PERSISTENT,
                                          &statement,
                                          nullptr);
  if (prepared != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar recurrence preparation failed (%1)"), prepared);
  }
  const std::optional<AppError> eventIdError = bindText(statement, 1, eventId);
  const std::optional<AppError> recurrenceError =
      recurrenceRule.has_value() ? bindText(statement, 2, *recurrenceRule) : std::nullopt;
  if (eventIdError.has_value() || recurrenceError.has_value()) {
    sqlite3_finalize(statement);
    return eventIdError.has_value() ? eventIdError : recurrenceError;
  }
  const int stepped = sqlite3_step(statement);
  const int finalized = sqlite3_finalize(statement);
  if (stepped != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar recurrence write failed (%1)"), stepped);
  }
  return finalized == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite calendar recurrence finalization failed (%1)"), finalized));
}

[[nodiscard]] std::optional<QString> optionalText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int size = sqlite3_column_bytes(statement, index);
  return value == nullptr || size < 0 ? std::nullopt
                                      : std::optional<QString>(QString::fromUtf8(value, size));
}

[[nodiscard]] bool isPendingRemoteId(const QString& remoteId) {
  return remoteId.startsWith(QStringLiteral("pending:"));
}

[[nodiscard]] QList<QString> storedAttendeeEmails(const QString& json);

[[nodiscard]] std::variant<std::optional<StoredEventContext>, AppError>
readEventContext(SqliteConnection& connection, const QString& eventId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT events.id, calendars.account_id, events.calendar_id, calendars.remote_id,
       calendars.access_role, events.remote_id, events.etag, events.title, events.description,
       events.location, events.start_at, events.start_time_zone, events.end_at,
       events.end_time_zone, events.is_all_day,
       COALESCE(recurrences.recurrence_rule, events.recurrence_rule), events.recurring_remote_id,
       events.original_start_at, events.status, events.color_id, events.transparency, events.visibility,
       events.event_type, events.attendee_emails_json, events.attendee_details_json,
       events.reminders_json, events.reminders_use_default, events.conference_json,
       events.attachments_json, events.guest_permissions_json, events.status_properties_json,
       calendars.is_primary
FROM local_calendar_events AS events
INNER JOIN local_calendars AS calendars ON calendars.id = events.calendar_id
LEFT JOIN local_calendar_event_recurrences AS recurrences ON recurrences.event_id = events.id
WHERE events.id = ?1 AND events.deleted_at IS NULL AND calendars.deleted_at IS NULL
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-event context preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, eventId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? std::optional<StoredEventContext>{}
               : std::variant<std::optional<StoredEventContext>, AppError>(databaseError(
                     QStringLiteral("SQLite calendar-event context finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-event context lookup failed (%1)"),
                         stepResult);
  }
  const std::optional<QString> storedEventId = optionalText(statement, 0);
  const std::optional<QString> accountId = optionalText(statement, 1);
  const std::optional<QString> calendarId = optionalText(statement, 2);
  const std::optional<QString> calendarRemoteId = optionalText(statement, 3);
  const std::optional<QString> remoteId = optionalText(statement, 5);
  const std::optional<QString> title = optionalText(statement, 7);
  const std::optional<QString> startAt = optionalText(statement, 10);
  const std::optional<QString> endAt = optionalText(statement, 12);
  if (!storedEventId.has_value() || !accountId.has_value() || !calendarId.has_value() ||
      !calendarRemoteId.has_value() || !remoteId.has_value() || !title.has_value() ||
      !startAt.has_value() || !endAt.has_value()) {
    sqlite3_finalize(statement);
    return AppError(AppErrorCode::Database, QStringLiteral("Stored calendar event is invalid"));
  }
  StoredEventContext context{.eventId = *storedEventId,
                             .accountId = *accountId,
                             .calendarId = *calendarId,
                             .calendarRemoteId = *calendarRemoteId,
                             .calendarAccessRole = optionalText(statement, 4),
                             .remoteId = *remoteId,
                             .remoteEtag = optionalText(statement, 6),
                             .title = *title,
                             .description = optionalText(statement, 8),
                             .location = optionalText(statement, 9),
                             .startAt = *startAt,
                             .startTimeZone = optionalText(statement, 11),
                             .endAt = *endAt,
                             .endTimeZone = optionalText(statement, 13),
                             .allDay = sqlite3_column_int(statement, 14) != 0,
                             .recurrenceRule = optionalText(statement, 15),
                             .recurringRemoteId = optionalText(statement, 16),
                             .originalStartAt = optionalText(statement, 17),
                             .status = optionalText(statement, 18).value_or(QString()),
                             .colorId = optionalText(statement, 19),
                             .transparency = optionalText(statement, 20),
                             .visibility = optionalText(statement, 21),
                             .eventType = optionalText(statement, 22),
                             .attendeeEmailsJson = optionalText(statement, 23).value_or(QString()),
                             .attendeeDetailsJson = optionalText(statement, 24).value_or(QString()),
                             .remindersJson = optionalText(statement, 25).value_or(QString()),
                             .remindersUseDefault = sqlite3_column_int(statement, 26) == 1,
                             .conferenceJson = optionalText(statement, 27),
                             .attachmentsJson = optionalText(statement, 28).value_or(QStringLiteral("[]")),
                             .guestPermissionsJson = optionalText(statement, 29).value_or(QStringLiteral("{}")),
                             .statusPropertiesJson = optionalText(statement, 30).value_or(QStringLiteral("{}")),
                             .calendarPrimary = sqlite3_column_int(statement, 31) == 1};
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? std::variant<std::optional<StoredEventContext>, AppError>(std::move(context))
             : std::variant<std::optional<StoredEventContext>, AppError>(databaseError(
                   QStringLiteral("SQLite calendar-event context finalization failed (%1)"),
                   finalizeResult));
}

struct ScopedEventTarget final {
  StoredEventContext event;
  bool isVirtualInstance{false};
};

[[nodiscard]] std::optional<QString> virtualOccurrenceStart(const QString& eventId, bool allDay) {
  const qsizetype marker = eventId.lastIndexOf(QStringLiteral(":instance:"));
  if (marker <= 0) {
    return std::nullopt;
  }
  const QString suffix = eventId.sliced(marker + QStringLiteral(":instance:").size());
  if (allDay) {
    const QDate date = QDate::fromString(suffix, QStringLiteral("yyyyMMdd"));
    return date.isValid()
               ? std::optional<QString>(
                     QDateTime(date, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs))
               : std::optional<QString>{};
  }
  static const QRegularExpression pattern(QStringLiteral("^\\d{8}T\\d{6}Z$"));
  if (!pattern.match(suffix).hasMatch()) {
    return std::nullopt;
  }
  const QDate date = QDate::fromString(suffix.left(8), QStringLiteral("yyyyMMdd"));
  const QTime time(suffix.sliced(9, 2).toInt(), suffix.sliced(11, 2).toInt(),
                   suffix.sliced(13, 2).toInt());
  return date.isValid() && time.isValid()
             ? std::optional<QString>(
                   QDateTime(date, time, QTimeZone::UTC).toString(Qt::ISODateWithMs))
             : std::optional<QString>{};
}

[[nodiscard]] std::variant<std::optional<ScopedEventTarget>, AppError>
readScopedEventTarget(SqliteConnection& connection, const QString& eventId) {
  const std::variant<std::optional<StoredEventContext>, AppError> stored =
      readEventContext(connection, eventId);
  if (std::holds_alternative<AppError>(stored)) {
    return std::get<AppError>(stored);
  }
  if (std::get<std::optional<StoredEventContext>>(stored).has_value()) {
    return ScopedEventTarget{.event = *std::get<std::optional<StoredEventContext>>(stored)};
  }
  const qsizetype marker = eventId.lastIndexOf(QStringLiteral(":instance:"));
  if (marker <= 0) {
    return std::optional<ScopedEventTarget>{};
  }
  const std::variant<std::optional<StoredEventContext>, AppError> masterResult =
      readEventContext(connection, eventId.first(marker));
  if (std::holds_alternative<AppError>(masterResult)) {
    return std::get<AppError>(masterResult);
  }
  const std::optional<StoredEventContext>& master =
      std::get<std::optional<StoredEventContext>>(masterResult);
  if (!master.has_value() || !master->recurrenceRule.has_value()) {
    return std::optional<ScopedEventTarget>{};
  }
  const std::optional<QString> start = virtualOccurrenceStart(eventId, master->allDay);
  const QDateTime masterStart = QDateTime::fromString(master->startAt, Qt::ISODateWithMs);
  const QDateTime masterEnd = QDateTime::fromString(master->endAt, Qt::ISODateWithMs);
  const QDateTime occurrenceStart =
      start.has_value() ? QDateTime::fromString(*start, Qt::ISODateWithMs) : QDateTime{};
  if (!occurrenceStart.isValid() || !masterStart.isValid() || !masterEnd.isValid() ||
      masterEnd <= masterStart) {
    return std::optional<ScopedEventTarget>{};
  }
  RecurrenceExpansionWorker worker;
  const RecurrenceExpansionResult expanded =
      worker.expand({.eventId = master->eventId,
                     .startAt = master->startAt,
                     .endAt = master->endAt,
                     .allDay = master->allDay,
                     .timeZone = master->startTimeZone,
                     .recurrenceRule = master->recurrenceRule,
                     .rangeStartAt = *start,
                     .rangeEndAt = occurrenceStart.addMSecs(1).toUTC().toString(Qt::ISODateWithMs)})
          .get();
  if (!std::holds_alternative<QList<RecurrenceOccurrence>>(expanded)) {
    return std::optional<ScopedEventTarget>{};
  }
  const QList<RecurrenceOccurrence>& occurrences = std::get<QList<RecurrenceOccurrence>>(expanded);
  const bool represented = std::any_of(
      occurrences.cbegin(), occurrences.cend(), [&start](const RecurrenceOccurrence& occurrence) {
        return occurrence.originalStartAt == start;
      });
  if (!represented) {
    return std::optional<ScopedEventTarget>{};
  }
  StoredEventContext occurrence = *master;
  occurrence.eventId = eventId;
  occurrence.startAt = *start;
  occurrence.endAt = occurrenceStart.addMSecs(masterStart.msecsTo(masterEnd)).toUTC().toString(
      Qt::ISODateWithMs);
  occurrence.recurrenceRule.reset();
  occurrence.recurringRemoteId = master->remoteId;
  occurrence.originalStartAt = start;
  return ScopedEventTarget{.event = std::move(occurrence), .isVirtualInstance = true};
}

[[nodiscard]] QJsonObject eventTime(const QString& at,
                                    const std::optional<QString>& timeZone,
                                    bool allDay) {
  const QDateTime parsed = QDateTime::fromString(at, Qt::ISODate);
  QJsonObject result;
  if (allDay) {
    result.insert(QStringLiteral("date"), parsed.date().toString(Qt::ISODate));
  } else {
    result.insert(QStringLiteral("dateTime"), parsed.toUTC().toString(Qt::ISODateWithMs));
  }
  if (timeZone.has_value()) {
    result.insert(QStringLiteral("timeZone"), *timeZone);
  }
  return result;
}

[[nodiscard]] QJsonArray storedArray(const QString& value) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(value.toUtf8(), &error);
  return error.error == QJsonParseError::NoError && document.isArray() ? document.array()
                                                                         : QJsonArray();
}

[[nodiscard]] QJsonObject storedObject(const QString& value) {
  const std::optional<QJsonObject> object =
      boundedJsonObject(value, kMaximumEventPropertiesJsonBytes);
  return object.value_or(QJsonObject());
}

[[nodiscard]] QJsonObject conferenceCreateRequest(const std::optional<QString>& conferenceJson) {
  if (!conferenceJson.has_value()) {
    return {};
  }
  const std::optional<QJsonObject> conference =
      boundedJsonObject(*conferenceJson, kMaximumConferenceJsonBytes);
  if (!conference.has_value() || !conference->value(QStringLiteral("createRequest")).isObject()) {
    return {};
  }
  return *conference;
}

[[nodiscard]] QJsonObject storedReminders(const StoredEventContext& event) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(event.remindersJson.toUtf8(), &error);
  const QJsonArray overrides = error.error == QJsonParseError::NoError && document.isArray()
                                  ? document.array()
                                  : QJsonArray();
  return {{QStringLiteral("useDefault"), event.remindersUseDefault},
          {QStringLiteral("overrides"), overrides}};
}

[[nodiscard]] QJsonObject eventSnapshot(const StoredEventContext& event) {
  return {{QStringLiteral("summary"), event.title},
          {QStringLiteral("description"),
           event.description.has_value() ? QJsonValue(*event.description) : QJsonValue::Null},
          {QStringLiteral("location"),
           event.location.has_value() ? QJsonValue(*event.location) : QJsonValue::Null},
          {QStringLiteral("start"), eventTime(event.startAt, event.startTimeZone, event.allDay)},
          {QStringLiteral("end"), eventTime(event.endAt, event.endTimeZone, event.allDay)},
          {QStringLiteral("colorId"),
           event.colorId.has_value() ? QJsonValue(*event.colorId) : QJsonValue::Null},
          {QStringLiteral("transparency"),
           event.transparency.has_value() ? QJsonValue(*event.transparency) : QJsonValue::Null},
          {QStringLiteral("visibility"),
           event.visibility.has_value() ? QJsonValue(*event.visibility) : QJsonValue::Null},
          {QStringLiteral("recurrence"), recurrenceLines(event.recurrenceRule)},
          {QStringLiteral("attendees"), storedArray(event.attendeeDetailsJson)},
          {QStringLiteral("reminders"), storedReminders(event)},
          {QStringLiteral("conferenceData"), conferenceCreateRequest(event.conferenceJson)},
          {QStringLiteral("attachments"), storedArray(event.attachmentsJson)},
          {QStringLiteral("guestPermissions"), storedObject(event.guestPermissionsJson)},
          {QStringLiteral("statusProperties"), storedObject(event.statusPropertiesJson)}};
}

[[nodiscard]] QJsonObject eventBody(const StoredEventContext& event, bool creating) {
  QJsonObject body{{QStringLiteral("summary"), event.title},
                   {QStringLiteral("start"),
                    eventTime(event.startAt, event.startTimeZone, event.allDay)},
                   {QStringLiteral("end"), eventTime(event.endAt, event.endTimeZone, event.allDay)}};
  if (event.description.has_value()) {
    body.insert(QStringLiteral("description"), *event.description);
  } else if (!creating) {
    body.insert(QStringLiteral("description"), QJsonValue::Null);
  }
  if (event.location.has_value()) {
    body.insert(QStringLiteral("location"), *event.location);
  } else if (!creating) {
    body.insert(QStringLiteral("location"), QJsonValue::Null);
  }
  if (event.colorId.has_value()) {
    body.insert(QStringLiteral("colorId"), *event.colorId);
  }
  if (event.transparency.has_value()) {
    body.insert(QStringLiteral("transparency"), *event.transparency);
  }
  if (event.visibility.has_value()) {
    body.insert(QStringLiteral("visibility"), *event.visibility);
  }
  if (event.recurrenceRule.has_value()) {
    body.insert(QStringLiteral("recurrence"), recurrenceLines(event.recurrenceRule));
  }
  body.insert(QStringLiteral("attendees"), storedArray(event.attendeeDetailsJson));
  body.insert(QStringLiteral("reminders"), storedReminders(event));
  const QJsonObject conference = conferenceCreateRequest(event.conferenceJson);
  if (!conference.isEmpty()) {
    body.insert(QStringLiteral("conferenceData"), conference);
  }
  const QJsonArray attachments = storedArray(event.attachmentsJson);
  if (!attachments.isEmpty()) {
    body.insert(QStringLiteral("attachments"), attachments);
  }
  const QJsonObject permissions = storedObject(event.guestPermissionsJson);
  for (auto it = permissions.constBegin(); it != permissions.constEnd(); ++it) {
    body.insert(it.key(), it.value());
  }
  if (creating && event.eventType.value_or(QStringLiteral("default")) != QStringLiteral("default")) {
    body.insert(QStringLiteral("eventType"), *event.eventType);
    const QJsonObject properties = storedObject(event.statusPropertiesJson);
    for (auto it = properties.constBegin(); it != properties.constEnd(); ++it) {
      body.insert(it.key(), it.value());
    }
  }
  return body;
}

[[nodiscard]] QJsonObject eventPayload(const StoredEventContext& event,
                                      bool includeRemoteIdentity) {
  QJsonObject payload{{QStringLiteral("calendarId"), event.calendarRemoteId},
                      {QStringLiteral("localCalendarId"), event.calendarId},
                      {QStringLiteral("localEventId"), event.eventId},
                      {QStringLiteral("event"), eventBody(event, !includeRemoteIdentity)}};
  if (includeRemoteIdentity) {
    payload.insert(QStringLiteral("remoteEventId"), event.remoteId);
  }
  return payload;
}

[[nodiscard]] QJsonObject eventPatchBody(const StoredEventContext& before,
                                         const StoredEventContext& after) {
  const QJsonObject beforeSnapshot = eventSnapshot(before);
  const QJsonObject afterSnapshot = eventSnapshot(after);
  QJsonObject patch;
  for (const QStringView key : {u"summary", u"description", u"location", u"start", u"end",
                                u"colorId", u"transparency", u"visibility", u"attendees",
                                u"reminders", u"recurrence", u"conferenceData", u"attachments"}) {
    if (beforeSnapshot.value(key) != afterSnapshot.value(key)) {
      patch.insert(key.toString(), afterSnapshot.value(key));
    }
  }
  if (beforeSnapshot.value(QStringLiteral("guestPermissions")) !=
      afterSnapshot.value(QStringLiteral("guestPermissions"))) {
    const QJsonObject permissions = afterSnapshot.value(QStringLiteral("guestPermissions")).toObject();
    for (auto it = permissions.constBegin(); it != permissions.constEnd(); ++it) {
      patch.insert(it.key(), it.value());
    }
  }
  if (beforeSnapshot.value(QStringLiteral("statusProperties")) !=
      afterSnapshot.value(QStringLiteral("statusProperties"))) {
    const QJsonObject properties = afterSnapshot.value(QStringLiteral("statusProperties")).toObject();
    for (auto it = properties.constBegin(); it != properties.constEnd(); ++it) {
      patch.insert(it.key(), it.value());
    }
  }
  return patch;
}

[[nodiscard]] QJsonObject eventUpdatePayload(const StoredEventContext& before,
                                             const StoredEventContext& after) {
  return {{QStringLiteral("calendarId"), after.calendarRemoteId},
          {QStringLiteral("localCalendarId"), after.calendarId},
          {QStringLiteral("localEventId"), after.eventId},
          {QStringLiteral("remoteEventId"), after.remoteId},
          {QStringLiteral("event"), eventPatchBody(before, after)}};
}

[[nodiscard]] QJsonObject mergeEventPatch(QJsonObject existing, QJsonObject patch) {
  const QJsonValue existingEvent = existing.value(QStringLiteral("event"));
  if (!existingEvent.isObject()) {
    return patch;
  }
  QJsonObject merged = existingEvent.toObject();
  const QJsonValue patchEvent = patch.value(QStringLiteral("event"));
  if (!patchEvent.isObject()) {
    return existing;
  }
  const QJsonObject patchBody = patchEvent.toObject();
  for (auto it = patchBody.constBegin(); it != patchBody.constEnd(); ++it) {
    merged.insert(it.key(), it.value());
  }
  if (patchBody.contains(QStringLiteral("attendees")) &&
      !patchBody.contains(QStringLiteral("attendeesOmitted"))) {
    merged.remove(QStringLiteral("attendeesOmitted"));
  }
  existing.insert(QStringLiteral("event"), std::move(merged));
  existing.insert(QStringLiteral("calendarId"), patch.value(QStringLiteral("calendarId")));
  existing.insert(QStringLiteral("localCalendarId"), patch.value(QStringLiteral("localCalendarId")));
  existing.insert(QStringLiteral("remoteEventId"), patch.value(QStringLiteral("remoteEventId")));
  if (patch.contains(QStringLiteral("sendUpdates"))) {
    existing.insert(QStringLiteral("sendUpdates"), patch.value(QStringLiteral("sendUpdates")));
  }
  return existing;
}

[[nodiscard]] QJsonObject movePayload(const StoredEventContext& before,
                                     const StoredEventContext& after) {
  QJsonObject payload = eventPayload(after, true);
  payload.insert(QStringLiteral("sourceCalendarId"), before.calendarRemoteId);
  payload.insert(QStringLiteral("destinationCalendarId"), after.calendarRemoteId);
  payload.insert(QStringLiteral("remoteEventId"), before.remoteId);
  return payload;
}

[[nodiscard]] QJsonObject deletePayload(const StoredEventContext& event) {
  return {{QStringLiteral("calendarId"), event.calendarRemoteId},
          {QStringLiteral("localCalendarId"), event.calendarId},
          {QStringLiteral("localEventId"), event.eventId},
          {QStringLiteral("remoteEventId"), event.remoteId}};
}

[[nodiscard]] QJsonObject withConflictMetadata(QJsonObject payload,
                                               QJsonObject baseSnapshot,
                                               const std::optional<QString>& remoteEtag) {
  QJsonObject metadata{{QStringLiteral("base"), std::move(baseSnapshot)}};
  if (remoteEtag.has_value()) {
    metadata.insert(QStringLiteral("etag"), *remoteEtag);
  }
  payload.insert(QString::fromLatin1(kConflictMetadataKey), std::move(metadata));
  return payload;
}

[[nodiscard]] std::optional<QString> mutationDependency(const QJsonObject& payload) {
  const QJsonValue value = payload.value(QStringLiteral("dependsOnMutationId"));
  if (value.isUndefined() || value.isNull()) {
    return std::nullopt;
  }
  if (!value.isString() || !isValidRequiredText(value.toString(), kMaximumIdentifierLength)) {
    return std::nullopt;
  }
  return value.toString();
}

[[nodiscard]] std::variant<std::optional<ActiveEventMutation>, AppError>
findActiveEventMutation(SqliteConnection& connection, const QString& eventId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT id, operation, payload_json
FROM local_pending_mutations
WHERE resource_type = 'event' AND resource_id = ?1 AND status IN ('pending', 'failed')
ORDER BY created_at DESC, id DESC
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-event mutation lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, eventId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? std::optional<ActiveEventMutation>{}
               : std::variant<std::optional<ActiveEventMutation>, AppError>(databaseError(
                     QStringLiteral("SQLite calendar-event mutation lookup finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-event mutation lookup failed (%1)"),
                         stepResult);
  }
  const std::optional<QString> mutationId = optionalText(statement, 0);
  const std::optional<QString> operation = optionalText(statement, 1);
  const std::optional<QString> payloadJson = optionalText(statement, 2);
  QJsonParseError parseError;
  const QJsonDocument payloadDocument =
      payloadJson.has_value() ? QJsonDocument::fromJson(payloadJson->toUtf8(), &parseError)
                              : QJsonDocument();
  if (!mutationId.has_value() || !operation.has_value() || !payloadJson.has_value() ||
      parseError.error != QJsonParseError::NoError || !payloadDocument.isObject()) {
    sqlite3_finalize(statement);
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored calendar-event mutation is invalid"));
  }
  ActiveEventMutation mutation{
      .id = *mutationId, .operation = *operation, .payload = payloadDocument.object()};
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? std::variant<std::optional<ActiveEventMutation>, AppError>(std::move(mutation))
             : std::variant<std::optional<ActiveEventMutation>, AppError>(databaseError(
                   QStringLiteral("SQLite calendar-event mutation lookup finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError> replaceActiveEventMutation(SqliteConnection& connection,
                                                                 const ActiveEventMutation& mutation,
                                                                 QString operation,
                                                                 QJsonObject payload,
                                                                 const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_pending_mutations
SET operation = ?2, payload_json = ?3, status = 'pending', next_retry_at = NULL,
    last_error_code = NULL, last_error_message = NULL, updated_at = ?4
WHERE id = ?1 AND status IN ('pending', 'failed')
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(
        QStringLiteral("SQLite calendar-event mutation replacement preparation failed (%1)"),
        prepareResult);
  }
  const QString payloadJson =
      QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, mutation.id),
                                                     bindText(statement, 2, operation),
                                                     bindText(statement, 3, payloadJson),
                                                     bindText(statement, 4, updatedAt)});
      error.has_value()) {
    return error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-event mutation replacement failed (%1)"),
                         stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite calendar-event mutation replacement finalization failed (%1)"),
        finalizeResult);
  }
  return changedRows == 1
             ? std::nullopt
             : std::optional<AppError>(AppError(
                   AppErrorCode::Database,
                   QStringLiteral("Active calendar-event mutation was not replaced")));
}

[[nodiscard]] std::optional<AppError> removeActiveEventMutation(SqliteConnection& connection,
                                                                const ActiveEventMutation& mutation) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(handle,
                                               "DELETE FROM local_pending_mutations "
                                               "WHERE id = ?1 AND status IN ('pending', 'failed')",
                                               -1,
                                               SQLITE_PREPARE_PERSISTENT,
                                               &statement,
                                               nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(
        QStringLiteral("SQLite calendar-event mutation removal preparation failed (%1)"),
        prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, mutation.id);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-event mutation removal failed (%1)"),
                         stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite calendar-event mutation removal finalization failed (%1)"),
        finalizeResult);
  }
  return changedRows == 1
             ? std::nullopt
             : std::optional<AppError>(AppError(
                   AppErrorCode::Database,
                   QStringLiteral("Active calendar-event mutation was not removed")));
}

using EventMutationInsertResult = std::variant<QString, AppError>;

[[nodiscard]] EventMutationInsertResult insertEventMutation(SqliteConnection& connection,
                                                             const StoredEventContext& event,
                                                             QString operation,
                                                             QJsonObject payload,
                                                             const QString& createdAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_pending_mutations (
  id, account_id, resource_type, resource_id, operation, payload_json, status, attempt_count,
  created_at, updated_at
) VALUES (?1, ?2, 'event', ?3, ?4, ?5, 'pending', 0, ?6, ?6)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(
        QStringLiteral("SQLite calendar-event mutation enqueue preparation failed (%1)"),
        prepareResult);
  }
  const QString mutationId =
      QStringLiteral("mutation:event:") + QUuid::createUuid().toString(QUuid::WithoutBraces);
  const QString payloadJson =
      QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, mutationId),
                                                     bindText(statement, 2, event.accountId),
                                                     bindText(statement, 3, event.eventId),
                                                     bindText(statement, 4, operation),
                                                     bindText(statement, 5, payloadJson),
                                                     bindText(statement, 6, createdAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-event mutation enqueue failed (%1)"),
                         stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? EventMutationInsertResult(mutationId)
             : EventMutationInsertResult(databaseError(
                   QStringLiteral("SQLite calendar-event mutation enqueue finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError>
queueEventMutation(SqliteConnection& connection,
                   const StoredEventContext& before,
                   const std::optional<StoredEventContext>& after,
                   const QString& operation,
                   const QString& updatedAt,
                   const QString& sendUpdates = QStringLiteral("all"),
                   bool selfResponseOnly = false) {
  const auto withDeliveryOptions = [&](QJsonObject payload, const StoredEventContext& event,
                                       bool canLimitToSelf) {
    payload.insert(QStringLiteral("sendUpdates"), sendUpdates);
    if (!selfResponseOnly || !canLimitToSelf) {
      return payload;
    }
    QJsonObject body = payload.value(QStringLiteral("event")).toObject();
    QJsonArray selfAttendee;
    for (const QJsonValue& value : storedArray(event.attendeeDetailsJson)) {
      if (value.isObject() && value.toObject().value(QStringLiteral("self")).toBool()) {
        selfAttendee.append(value);
        break;
      }
    }
    if (!selfAttendee.isEmpty()) {
      body.insert(QStringLiteral("attendees"), std::move(selfAttendee));
      body.insert(QStringLiteral("attendeesOmitted"), true);
      payload.insert(QStringLiteral("event"), std::move(body));
    }
    return payload;
  };
  const std::variant<std::optional<ActiveEventMutation>, AppError> activeResult =
      findActiveEventMutation(connection, before.eventId);
  if (std::holds_alternative<AppError>(activeResult)) {
    return std::get<AppError>(activeResult);
  }
  const std::optional<ActiveEventMutation>& active =
      std::get<std::optional<ActiveEventMutation>>(activeResult);
  const bool deleting = operation == QStringLiteral("event.delete");
  if (!deleting && operation == QStringLiteral("event.update") && after.has_value() &&
      eventPatchBody(before, *after).isEmpty()) {
    return std::nullopt;
  }
  if (active.has_value()) {
    const QJsonValue metadata = active->payload.value(QString::fromLatin1(kConflictMetadataKey));
    if (!metadata.isObject()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored calendar-event mutation is invalid"));
    }
    if (active->operation == QStringLiteral("event.create")) {
      if (deleting) {
        return removeActiveEventMutation(connection, *active);
      }
      if (!after.has_value()) {
        return AppError(AppErrorCode::Database,
                        QStringLiteral("Updated calendar event is unavailable"));
      }
      QJsonObject payload = withDeliveryOptions(eventPayload(*after, false), *after, false);
      payload.insert(QString::fromLatin1(kConflictMetadataKey), metadata);
      return replaceActiveEventMutation(
          connection, *active, QStringLiteral("event.create"), std::move(payload), updatedAt);
    }
    std::optional<QString> dependency = mutationDependency(active->payload);
    if (operation == QStringLiteral("event.move")) {
      if (!after.has_value()) {
        return AppError(AppErrorCode::Database,
                        QStringLiteral("Moved calendar event is unavailable"));
      }
      QJsonObject payload = withDeliveryOptions(movePayload(before, *after), *after, false);
      if (dependency.has_value()) {
        payload.insert(QStringLiteral("dependsOnMutationId"), *dependency);
      }
      payload.insert(QString::fromLatin1(kConflictMetadataKey), metadata);
      if (const std::optional<AppError> error = replaceActiveEventMutation(
              connection, *active, QStringLiteral("event.move"), std::move(payload), updatedAt);
          error.has_value()) {
        return error;
      }
      QJsonObject followUp = withDeliveryOptions(eventPayload(*after, true), *after, true);
      followUp.insert(QStringLiteral("dependsOnMutationId"), active->id);
      followUp = withConflictMetadata(std::move(followUp), eventSnapshot(before), before.remoteEtag);
      const EventMutationInsertResult inserted = insertEventMutation(
          connection, *after, QStringLiteral("event.update"), std::move(followUp), updatedAt);
      return std::holds_alternative<AppError>(inserted)
                 ? std::optional<AppError>(std::get<AppError>(inserted))
                 : std::nullopt;
    }
    QJsonObject payload = deleting ? deletePayload(before)
                                   : withDeliveryOptions(eventUpdatePayload(before, *after),
                                                         *after,
                                                         !active->payload.value(QStringLiteral("event"))
                                                              .toObject()
                                                              .contains(QStringLiteral("attendees")));
    if (!deleting) {
      payload = mergeEventPatch(active->payload, std::move(payload));
    }
    if (dependency.has_value()) {
      payload.insert(QStringLiteral("dependsOnMutationId"), *dependency);
    }
    payload.insert(QString::fromLatin1(kConflictMetadataKey), metadata);
    return replaceActiveEventMutation(connection, *active, operation, std::move(payload), updatedAt);
  }
  QJsonObject payload;
  if (deleting) {
    payload = deletePayload(before);
  } else if (operation == QStringLiteral("event.move")) {
    if (!after.has_value()) {
      return AppError(AppErrorCode::Database, QStringLiteral("Moved calendar event is unavailable"));
    }
    payload = movePayload(before, *after);
  } else {
    if (!after.has_value()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Updated calendar event is unavailable"));
    }
    payload = operation == QStringLiteral("event.create")
                  ? withDeliveryOptions(eventPayload(*after, false), *after, false)
                  : withDeliveryOptions(eventUpdatePayload(before, *after), *after, true);
  }
  payload = withConflictMetadata(
      std::move(payload),
      operation == QStringLiteral("event.create") ? QJsonObject() : eventSnapshot(before),
      operation == QStringLiteral("event.create") ? std::optional<QString>{} : before.remoteEtag);
  const EventMutationInsertResult inserted =
      insertEventMutation(connection, before, operation, std::move(payload), updatedAt);
  if (std::holds_alternative<AppError>(inserted)) {
    return std::get<AppError>(inserted);
  }
  if (operation != QStringLiteral("event.move")) {
    return std::nullopt;
  }
  if (!after.has_value()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Moved calendar event is unavailable"));
  }
  QJsonObject followUp = withDeliveryOptions(eventPayload(*after, true), *after, true);
  followUp.insert(QStringLiteral("dependsOnMutationId"), std::get<QString>(inserted));
  followUp = withConflictMetadata(std::move(followUp), eventSnapshot(before), before.remoteEtag);
  const EventMutationInsertResult followUpInserted = insertEventMutation(
      connection, *after, QStringLiteral("event.update"), std::move(followUp), updatedAt);
  return std::holds_alternative<AppError>(followUpInserted)
             ? std::optional<AppError>(std::get<AppError>(followUpInserted))
             : std::nullopt;
}

[[nodiscard]] std::variant<CalendarEventCreateInput, AppError>
canonicalize(CalendarEventCreateInput input) {
  input.title = input.title.trimmed();
  const std::optional<QString> startAt = canonicalTimestamp(input.startAt);
  const std::optional<QString> endAt = canonicalTimestamp(input.endAt);
  const std::optional<QString> recurrenceRule = canonicalRecurrenceRule(input.recurrenceRule);
  if (!isValidRequiredText(input.calendarId, kMaximumIdentifierLength) ||
      !isValidRequiredText(input.title, kMaximumTitleLength) ||
      !isValidOptionalText(input.description, kMaximumDescriptionLength) ||
      !isValidOptionalText(input.location, kMaximumLocationLength) ||
      !isValidTimeZone(input.startTimeZone) || !isValidTimeZone(input.endTimeZone) ||
      (input.colorId.has_value() && !isValidColorId(*input.colorId)) ||
      (input.transparency.has_value() && !isValidTransparency(*input.transparency)) ||
      (input.visibility.has_value() && !isValidVisibility(*input.visibility)) ||
      !isValidReminders(input.reminders) ||
      (input.recurrenceRule.has_value() && !recurrenceRule.has_value()) ||
      !startAt.has_value() || !endAt.has_value() ||
      QDateTime::fromString(*endAt, Qt::ISODateWithMs) <=
          QDateTime::fromString(*startAt, Qt::ISODateWithMs)) {
    return validationError(QStringLiteral("Calendar event create input is invalid"));
  }
  input.startAt = *startAt;
  input.endAt = *endAt;
  input.recurrenceRule = recurrenceRule;
  if (!isSupportedEventType(input.richMetadata.eventType) ||
      !isValidSendUpdates(input.richMetadata.sendUpdates)) {
    return validationError(QStringLiteral("Calendar event rich metadata is invalid"));
  }
  const std::optional<QString> attachments =
      canonicalAttachmentsJson(input.richMetadata.attachmentsJson);
  const std::optional<QString> guestPermissions =
      canonicalGuestPermissionsJson(input.richMetadata.guestPermissionsJson);
  const std::optional<QString> statusProperties = canonicalStatusPropertiesJson(
      input.richMetadata.statusPropertiesJson, input.richMetadata.eventType);
  if (!attachments.has_value() || !guestPermissions.has_value() || !statusProperties.has_value()) {
    return validationError(QStringLiteral("Calendar event rich metadata is invalid"));
  }
  input.richMetadata.attachmentsJson = *attachments;
  input.richMetadata.guestPermissionsJson = *guestPermissions;
  input.richMetadata.statusPropertiesJson = *statusProperties;
  const std::optional<QList<QString>> attendees = canonicalAttendees(std::move(input.attendeeEmails));
  if (!attendees.has_value()) {
    return validationError(QStringLiteral("Calendar event create input is invalid"));
  }
  input.attendeeEmails = *attendees;
  return input;
}

[[nodiscard]] std::variant<CalendarEventUpdateInput, AppError>
canonicalize(CalendarEventUpdateInput input) {
  if (input.title.has_value()) {
    *input.title = input.title->trimmed();
  }
  if (input.startAt.has_value()) {
    const std::optional<QString> startAt = canonicalTimestamp(*input.startAt);
    if (!startAt.has_value()) {
      return validationError(QStringLiteral("Calendar event update input is invalid"));
    }
    input.startAt = *startAt;
  }
  if (input.endAt.has_value()) {
    const std::optional<QString> endAt = canonicalTimestamp(*input.endAt);
    if (!endAt.has_value()) {
      return validationError(QStringLiteral("Calendar event update input is invalid"));
    }
    input.endAt = *endAt;
  }
  if (input.recurrenceRule.has_value()) {
    const std::optional<QString> recurrenceRule = canonicalRecurrenceRule(*input.recurrenceRule);
    if (input.recurrenceRule->has_value() && !recurrenceRule.has_value()) {
      return validationError(QStringLiteral("Calendar event update input is invalid"));
    }
    input.recurrenceRule = recurrenceRule;
  }
  const bool hasPatch =
      input.calendarId.has_value() || input.title.has_value() || input.description.has_value() ||
      input.location.has_value() || input.startAt.has_value() || input.endAt.has_value() ||
      input.allDay.has_value() || input.startTimeZone.has_value() || input.endTimeZone.has_value() ||
      input.colorId.has_value() || input.transparency.has_value() || input.visibility.has_value() ||
      input.attendeeEmails.has_value() || input.reminders.has_value() || input.recurrenceRule.has_value() ||
      input.createGoogleMeet.has_value() || input.attachmentsJson.has_value() ||
      input.guestPermissionsJson.has_value() || input.statusPropertiesJson.has_value() ||
      input.sendUpdates.has_value() || input.selfResponseStatus.has_value() ||
      input.selfResponseComment.has_value();
  if (!isValidRequiredText(input.eventId, kMaximumIdentifierLength) ||
      (input.calendarId.has_value() &&
       !isValidRequiredText(*input.calendarId, kMaximumIdentifierLength)) ||
      (input.title.has_value() && !isValidRequiredText(*input.title, kMaximumTitleLength)) ||
      (input.description.has_value() &&
       !isValidOptionalText(*input.description, kMaximumDescriptionLength)) ||
      (input.location.has_value() &&
       !isValidOptionalText(*input.location, kMaximumLocationLength)) ||
      (input.startTimeZone.has_value() && !isValidTimeZone(*input.startTimeZone)) ||
      (input.endTimeZone.has_value() && !isValidTimeZone(*input.endTimeZone)) || !hasPatch) {
    return validationError(QStringLiteral("Calendar event update input is invalid"));
  }
  if ((input.colorId.has_value() && input.colorId->has_value() &&
       !isValidColorId(**input.colorId)) ||
      (input.transparency.has_value() && !isValidTransparency(*input.transparency)) ||
      (input.visibility.has_value() && !isValidVisibility(*input.visibility))) {
    return validationError(QStringLiteral("Calendar event update input is invalid"));
  }
  if (input.attendeeEmails.has_value()) {
    const std::optional<QList<QString>> attendees = canonicalAttendees(std::move(*input.attendeeEmails));
    if (!attendees.has_value()) {
      return validationError(QStringLiteral("Calendar event update input is invalid"));
    }
    input.attendeeEmails = *attendees;
  }
  if (input.reminders.has_value() && !isValidReminders(*input.reminders)) {
    return validationError(QStringLiteral("Calendar event update input is invalid"));
  }
  if ((input.attachmentsJson.has_value() && !canonicalAttachmentsJson(*input.attachmentsJson).has_value()) ||
      (input.guestPermissionsJson.has_value() &&
       !canonicalGuestPermissionsJson(*input.guestPermissionsJson).has_value()) ||
      (input.statusPropertiesJson.has_value() &&
       !boundedJsonObject(*input.statusPropertiesJson, kMaximumEventPropertiesJsonBytes).has_value()) ||
      (input.sendUpdates.has_value() && !isValidSendUpdates(*input.sendUpdates)) ||
      (input.selfResponseStatus.has_value() && !isValidResponseStatus(*input.selfResponseStatus)) ||
      (input.selfResponseComment.has_value() &&
       !isValidOptionalText(*input.selfResponseComment, 4'096))) {
    return validationError(QStringLiteral("Calendar event rich metadata is invalid"));
  }
  if (input.attachmentsJson.has_value()) {
    input.attachmentsJson = *canonicalAttachmentsJson(*input.attachmentsJson);
  }
  if (input.guestPermissionsJson.has_value()) {
    input.guestPermissionsJson = *canonicalGuestPermissionsJson(*input.guestPermissionsJson);
  }
  if (input.statusPropertiesJson.has_value()) {
    input.statusPropertiesJson = compactJson(
        *boundedJsonObject(*input.statusPropertiesJson, kMaximumEventPropertiesJsonBytes));
  }
  if (input.startAt.has_value() && input.endAt.has_value() &&
      QDateTime::fromString(*input.endAt, Qt::ISODateWithMs) <=
          QDateTime::fromString(*input.startAt, Qt::ISODateWithMs)) {
    return validationError(QStringLiteral("Calendar event update range is invalid"));
  }
  return input;
}

[[nodiscard]] CalendarEventMutationResult createStoredEvent(SqliteConnection& connection,
                                                            const CalendarEventCreateInput& input,
                                                            const QString& eventId,
                                                            const QString& remoteId,
                                                            const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_calendar_events (
  id, calendar_id, remote_id, status, title, description, location, start_at, start_time_zone,
  end_at, end_time_zone, is_all_day, recurrence_rule, color_id, transparency, visibility, attendee_emails_json,
  attendee_details_json, reminder_minutes_json, reminders_json, reminders_use_default, event_type,
  conference_json, attachments_json, guest_permissions_json, status_properties_json, created_at, updated_at
)
SELECT ?1, calendars.id, ?3, 'confirmed', ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15,
       ?16, ?17, ?18, ?19, ?20, ?21, ?22, ?23, ?24, ?25, ?26, ?26
FROM local_calendars AS calendars
WHERE calendars.id = ?2 AND calendars.deleted_at IS NULL
  AND (calendars.access_role IS NULL OR calendars.access_role IN ('writer', 'owner'))
  AND (?21 = 'default' OR calendars.is_primary = 1)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-event create preparation failed (%1)"),
                         prepareResult);
  }
  const QJsonArray details = attendeeDetails(input.attendeeEmails);
  const QJsonObject reminderSettings = remindersJson(input.reminders);
  const QString reminderOverrides = compactJson(reminderSettings.value(QStringLiteral("overrides")).toArray());
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, eventId),
                   bindText(statement, 2, input.calendarId),
                   bindText(statement, 3, remoteId),
                   bindText(statement, 4, input.title),
                   bindOptionalText(statement, 5, input.description),
                   bindOptionalText(statement, 6, input.location),
                   bindText(statement, 7, input.startAt),
                   bindOptionalText(statement, 8, input.startTimeZone),
                   bindText(statement, 9, input.endAt),
                   bindOptionalText(statement, 10, input.endTimeZone),
                   bindInteger(statement, 11, input.allDay ? 1 : 0),
                   bindOptionalText(statement, 12, std::optional<QString>{}),
                   bindOptionalText(statement, 13, input.colorId),
                   bindOptionalText(statement, 14, input.transparency),
                   bindOptionalText(statement, 15, input.visibility),
                   bindText(statement, 16, compactJson(QJsonArray::fromStringList(input.attendeeEmails))),
                   bindText(statement, 17, compactJson(details)),
                   bindText(statement, 18, compactJson(reminderMinutes(input.reminders))),
                   bindText(statement, 19, reminderOverrides),
                   bindInteger(statement, 20, input.reminders.useDefault ? 1 : 0),
                   bindText(statement, 21, input.richMetadata.eventType),
                   bindOptionalText(statement, 22,
                                    input.richMetadata.createGoogleMeet
                                        ? std::optional<QString>(googleMeetCreateRequestJson())
                                        : std::optional<QString>{}),
                   bindText(statement, 23, input.richMetadata.attachmentsJson),
                   bindText(statement, 24, input.richMetadata.guestPermissionsJson),
                   bindText(statement, 25, input.richMetadata.statusPropertiesJson),
                   bindText(statement, 26, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-event create failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar-event create finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Calendar is unavailable for event creation"));
  }
  if (input.recurrenceRule.has_value()) {
    if (const std::optional<AppError> error =
            writeStoredRecurrence(handle, eventId, input.recurrenceRule);
        error.has_value()) {
      return *error;
    }
  }
  return CalendarEventMutationReceipt{.eventId = eventId, .updatedAt = updatedAt};
}

[[nodiscard]] CalendarEventMutationResult updateStoredEvent(SqliteConnection& connection,
                                                            const CalendarEventUpdateInput& input,
                                                            const StoredEventContext& before,
                                                            const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  if (input.statusPropertiesJson.has_value() &&
      !canonicalStatusPropertiesJson(*input.statusPropertiesJson,
                                     before.eventType.value_or(QStringLiteral("default"))).has_value()) {
    return validationError(QStringLiteral("Calendar event status properties are invalid"));
  }
  if (input.createGoogleMeet.value_or(false) && before.conferenceJson.has_value()) {
    return validationError(QStringLiteral("Calendar event already has conference data"));
  }
  constexpr char sql[] = R"(
UPDATE local_calendar_events
SET calendar_id = CASE WHEN ?2 = 1 THEN ?3 ELSE calendar_id END,
    title = CASE WHEN ?4 = 1 THEN ?5 ELSE title END,
    description = CASE WHEN ?6 = 1 THEN ?7 ELSE description END,
    location = CASE WHEN ?8 = 1 THEN ?9 ELSE location END,
    start_at = CASE WHEN ?10 = 1 THEN ?11 ELSE start_at END,
    end_at = CASE WHEN ?12 = 1 THEN ?13 ELSE end_at END,
    is_all_day = CASE WHEN ?14 = 1 THEN ?15 ELSE is_all_day END,
    recurrence_rule = CASE WHEN ?16 = 1 THEN NULL ELSE recurrence_rule END,
    start_time_zone = CASE WHEN ?18 = 1 THEN ?19 ELSE start_time_zone END,
    end_time_zone = CASE WHEN ?20 = 1 THEN ?21 ELSE end_time_zone END,
    color_id = CASE WHEN ?22 = 1 THEN ?23 ELSE color_id END,
    transparency = CASE WHEN ?24 = 1 THEN ?25 ELSE transparency END,
    visibility = CASE WHEN ?26 = 1 THEN ?27 ELSE visibility END,
    attendee_emails_json = CASE WHEN ?28 = 1 THEN ?29 ELSE attendee_emails_json END,
    attendee_details_json = CASE WHEN ?28 = 1 THEN ?30 ELSE attendee_details_json END,
    reminder_minutes_json = CASE WHEN ?31 = 1 THEN ?32 ELSE reminder_minutes_json END,
    reminders_json = CASE WHEN ?31 = 1 THEN ?33 ELSE reminders_json END,
    reminders_use_default = CASE WHEN ?31 = 1 THEN ?34 ELSE reminders_use_default END,
    conference_json = CASE WHEN ?35 = 1 THEN ?36 ELSE conference_json END,
    attachments_json = CASE WHEN ?37 = 1 THEN ?38 ELSE attachments_json END,
    guest_permissions_json = CASE WHEN ?39 = 1 THEN ?40 ELSE guest_permissions_json END,
    status_properties_json = CASE WHEN ?41 = 1 THEN ?42 ELSE status_properties_json END,
    updated_at = ?43
WHERE id = ?1
  AND deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM local_calendars AS source
              WHERE source.id = local_calendar_events.calendar_id
                AND source.deleted_at IS NULL
                AND (source.access_role IS NULL OR source.access_role IN ('writer', 'owner')))
  AND (?2 = 0 OR EXISTS (SELECT 1 FROM local_calendars AS target
                          INNER JOIN local_calendars AS source ON source.id = local_calendar_events.calendar_id
                          WHERE target.id = ?3
                            AND target.deleted_at IS NULL
                            AND source.deleted_at IS NULL
                            AND (target.access_role IS NULL OR target.access_role IN ('writer', 'owner'))
                            AND target.account_id = source.account_id))
  AND julianday(CASE WHEN ?10 = 1 THEN ?11 ELSE start_at END) IS NOT NULL
  AND julianday(CASE WHEN ?12 = 1 THEN ?13 ELSE end_at END) >
      julianday(CASE WHEN ?10 = 1 THEN ?11 ELSE start_at END)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-event update preparation failed (%1)"),
                         prepareResult);
  }
  const std::optional<QString> description =
      input.description.has_value() ? *input.description : std::nullopt;
  const std::optional<QString> location =
      input.location.has_value() ? *input.location : std::nullopt;
  const std::optional<QString> startTimeZone =
      input.startTimeZone.has_value() ? *input.startTimeZone : std::nullopt;
  const std::optional<QString> endTimeZone =
      input.endTimeZone.has_value() ? *input.endTimeZone : std::nullopt;
  const std::optional<QString> colorId =
      input.colorId.has_value() ? *input.colorId : std::nullopt;
  const QList<QString> attendees = input.attendeeEmails.has_value()
                                       ? *input.attendeeEmails
                                       : storedAttendeeEmails(before.attendeeEmailsJson);
  QJsonArray details = (input.attendeeEmails.has_value() || input.selfResponseStatus.has_value() ||
                        input.selfResponseComment.has_value())
                           ? attendeeDetails(attendees, before.attendeeDetailsJson)
                           : QJsonArray();
  if (input.selfResponseStatus.has_value() || input.selfResponseComment.has_value()) {
    bool foundSelf = false;
    for (QJsonValueRef attendeeValue : details) {
      QJsonObject attendee = attendeeValue.toObject();
      if (attendee.value(QStringLiteral("self")).toBool()) {
        if (input.selfResponseStatus.has_value()) {
          attendee.insert(QStringLiteral("responseStatus"), *input.selfResponseStatus);
        }
        if (input.selfResponseComment.has_value()) {
          if (input.selfResponseComment->isEmpty()) {
            attendee.remove(QStringLiteral("comment"));
          } else {
            attendee.insert(QStringLiteral("comment"), *input.selfResponseComment);
          }
        }
        attendeeValue = attendee;
        foundSelf = true;
        break;
      }
    }
    if (!foundSelf) {
      return validationError(QStringLiteral("Google attendee identity is unavailable for RSVP"));
    }
  }
  const bool attendeeUpdate = input.attendeeEmails.has_value() || input.selfResponseStatus.has_value() ||
                              input.selfResponseComment.has_value();
  const CalendarEventReminderSettings reminders =
      input.reminders.value_or(CalendarEventReminderSettings{});
  const QJsonObject reminderSettings = remindersJson(reminders);
  const QString reminderOverrideJson =
      compactJson(reminderSettings.value(QStringLiteral("overrides")).toArray());
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, input.eventId),
                   bindInteger(statement, 2, input.calendarId.has_value()),
                   bindOptionalText(statement, 3, input.calendarId),
                   bindInteger(statement, 4, input.title.has_value()),
                   bindOptionalText(statement, 5, input.title),
                   bindInteger(statement, 6, input.description.has_value()),
                   bindOptionalText(statement, 7, description),
                   bindInteger(statement, 8, input.location.has_value()),
                   bindOptionalText(statement, 9, location),
                   bindInteger(statement, 10, input.startAt.has_value()),
                   bindOptionalText(statement, 11, input.startAt),
                   bindInteger(statement, 12, input.endAt.has_value()),
                   bindOptionalText(statement, 13, input.endAt),
                   bindInteger(statement, 14, input.allDay.has_value()),
                   bindInteger(statement, 15, input.allDay.value_or(false) ? 1 : 0),
                   bindInteger(statement, 16, input.recurrenceRule.has_value()),
                   bindOptionalText(statement, 17, std::optional<QString>{}),
                   bindInteger(statement, 18, input.startTimeZone.has_value()),
                   bindOptionalText(statement, 19, startTimeZone),
                   bindInteger(statement, 20, input.endTimeZone.has_value()),
                   bindOptionalText(statement, 21, endTimeZone),
                   bindInteger(statement, 22, input.colorId.has_value()),
                   bindOptionalText(statement, 23, colorId),
                   bindInteger(statement, 24, input.transparency.has_value()),
                   bindOptionalText(statement, 25, input.transparency),
                   bindInteger(statement, 26, input.visibility.has_value()),
                   bindOptionalText(statement, 27, input.visibility),
                   bindInteger(statement, 28, attendeeUpdate),
                   bindText(statement, 29,
                            compactJson(QJsonArray::fromStringList(attendees))),
                   bindText(statement, 30, compactJson(details)),
                   bindInteger(statement, 31, input.reminders.has_value()),
                   bindText(statement, 32, compactJson(reminderMinutes(reminders))),
                   bindText(statement, 33, reminderOverrideJson),
                   bindInteger(statement, 34, reminders.useDefault ? 1 : 0),
                   bindInteger(statement, 35, input.createGoogleMeet.value_or(false)),
                   bindOptionalText(statement, 36,
                                    input.createGoogleMeet.value_or(false)
                                        ? std::optional<QString>(googleMeetCreateRequestJson())
                                        : std::optional<QString>{}),
                   bindInteger(statement, 37, input.attachmentsJson.has_value()),
                   bindOptionalText(statement, 38, input.attachmentsJson),
                   bindInteger(statement, 39, input.guestPermissionsJson.has_value()),
                   bindOptionalText(statement, 40, input.guestPermissionsJson),
                   bindInteger(statement, 41, input.statusPropertiesJson.has_value()),
                   bindOptionalText(statement, 42, input.statusPropertiesJson),
                   bindText(statement, 43, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-event update failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar-event update finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Calendar event is unavailable for update"));
  }
  if (input.recurrenceRule.has_value()) {
    const std::optional<QString>& recurrence = *input.recurrenceRule;
    if (const std::optional<AppError> error =
            writeStoredRecurrence(handle, input.eventId, recurrence);
        error.has_value()) {
      return *error;
    }
  }
  return CalendarEventMutationReceipt{.eventId = input.eventId, .updatedAt = updatedAt};
}

[[nodiscard]] CalendarEventMutationResult
removeStoredEvent(SqliteConnection& connection, const QString& eventId, const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_calendar_events
SET deleted_at = ?2,
    updated_at = ?2
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-event deletion preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement, {bindText(statement, 1, eventId), bindText(statement, 2, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-event deletion failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar-event deletion finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Calendar event is unavailable for deletion"));
  }
  return CalendarEventMutationReceipt{.eventId = eventId, .updatedAt = updatedAt};
}

[[nodiscard]] CalendarEventMutationResult
restoreStoredEvent(SqliteConnection& connection,
                   const QString& eventId,
                   bool createRemote,
                   const QString& remoteId,
                   const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_calendar_events
SET deleted_at = NULL,
    remote_id = CASE WHEN ?2 = 1 THEN ?3 ELSE remote_id END,
    etag = CASE WHEN ?2 = 1 THEN NULL ELSE etag END,
    updated_at = ?4
WHERE id = ?1 AND deleted_at IS NOT NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-event restoration preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, eventId),
                   bindInteger(statement, 2, createRemote ? 1 : 0),
                   bindText(statement, 3, remoteId),
                   bindText(statement, 4, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-event restoration failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar-event restoration finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Calendar event is unavailable for restoration"));
  }
  return CalendarEventMutationReceipt{.eventId = eventId, .updatedAt = updatedAt};
}

[[nodiscard]] std::variant<std::optional<StoredEventContext>, AppError>
readSeriesMasterContext(SqliteConnection& connection, const StoredEventContext& event) {
  if (!event.recurringRemoteId.has_value()) {
    return event.recurrenceRule.has_value()
               ? std::variant<std::optional<StoredEventContext>, AppError>(event)
               : std::variant<std::optional<StoredEventContext>, AppError>(
                     std::optional<StoredEventContext>{});
  }
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT id
FROM local_calendar_events
WHERE calendar_id = ?1 AND remote_id = ?2 AND recurring_remote_id IS NULL AND deleted_at IS NULL
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepared = sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepared != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite recurring-event master preparation failed (%1)"), prepared);
  }
  if (const std::optional<AppError> error =
          bindAll(statement, {bindText(statement, 1, event.calendarId),
                              bindText(statement, 2, *event.recurringRemoteId)});
      error.has_value()) {
    return *error;
  }
  const int stepped = sqlite3_step(statement);
  const std::optional<QString> eventId = stepped == SQLITE_ROW ? optionalText(statement, 0)
                                                                 : std::optional<QString>{};
  const int finalized = sqlite3_finalize(statement);
  if (stepped != SQLITE_ROW && stepped != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite recurring-event master lookup failed (%1)"), stepped);
  }
  if (finalized != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite recurring-event master finalization failed (%1)"), finalized);
  }
  return eventId.has_value() ? readEventContext(connection, *eventId)
                             : std::variant<std::optional<StoredEventContext>, AppError>(
                                   std::optional<StoredEventContext>{});
}

[[nodiscard]] QList<QString> storedAttendeeEmails(const QString& json) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(json.toUtf8(), &error);
  if (error.error != QJsonParseError::NoError || !document.isArray()) {
    return {};
  }
  QList<QString> result;
  for (const QJsonValue& value : document.array()) {
    if (!value.isString()) {
      return {};
    }
    result.append(value.toString());
  }
  return result;
}

[[nodiscard]] CalendarEventReminderSettings storedReminderSettings(const StoredEventContext& event) {
  CalendarEventReminderSettings result{.useDefault = event.remindersUseDefault};
  const QJsonArray overrides = storedReminders(event).value(QStringLiteral("overrides")).toArray();
  for (const QJsonValue& value : overrides) {
    const QJsonObject reminder = value.toObject();
    const QJsonValue method = reminder.value(QStringLiteral("method"));
    const QJsonValue minutes = reminder.value(QStringLiteral("minutes"));
    if (!method.isString() || !minutes.isDouble()) {
      return {};
    }
    result.overrides.append({.method = method.toString(), .minutes = static_cast<int>(minutes.toInteger())});
  }
  return result;
}

[[nodiscard]] std::optional<QString>
successorRecurrenceRule(const StoredEventContext& master, const QString& targetOriginalStart) {
  if (!master.recurrenceRule.has_value() || master.recurrenceRule->contains(u'\n')) {
    return std::nullopt;
  }
  const QRegularExpression countPattern(QStringLiteral("(?:^|;)COUNT=(\\d+)(?:;|$)"));
  const QRegularExpressionMatch countMatch = countPattern.match(*master.recurrenceRule);
  if (!countMatch.hasMatch()) {
    return master.recurrenceRule;
  }
  RecurrenceExpansionWorker worker;
  const RecurrenceExpansionResult expanded =
      worker.expand({.eventId = master.eventId,
                     .startAt = master.startAt,
                     .endAt = master.endAt,
                     .allDay = master.allDay,
                     .timeZone = master.startTimeZone,
                     .recurrenceRule = master.recurrenceRule})
          .get();
  if (!std::holds_alternative<QList<RecurrenceOccurrence>>(expanded)) {
    return std::nullopt;
  }
  const QList<RecurrenceOccurrence>& occurrences = std::get<QList<RecurrenceOccurrence>>(expanded);
  qsizetype targetIndex = occurrences.size();
  for (qsizetype index = 0; index < occurrences.size(); ++index) {
    if (occurrences.at(index).originalStartAt == targetOriginalStart) {
      targetIndex = index;
      break;
    }
  }
  if (targetIndex == occurrences.size()) {
    return std::nullopt;
  }
  bool converted = false;
  const int count = countMatch.captured(1).toInt(&converted);
  const int remaining = count - static_cast<int>(targetIndex);
  if (!converted || remaining < 1) {
    return std::nullopt;
  }
  QStringList fields;
  for (const QString& field : master.recurrenceRule->sliced(6).split(u';', Qt::SkipEmptyParts)) {
    fields.append(field.startsWith(QStringLiteral("COUNT="))
                      ? QStringLiteral("COUNT=") + QString::number(remaining)
                      : field);
  }
  return canonicalRecurrenceRule(QStringLiteral("RRULE:") + fields.join(u';'));
}

[[nodiscard]] CalendarEventCreateInput successorInput(const StoredEventContext& master,
                                                       const StoredEventContext& target,
                                                       const CalendarEventUpdateInput& patch,
                                                       const QString& recurrenceRule) {
  const QDateTime masterStart = QDateTime::fromString(master.startAt, Qt::ISODateWithMs);
  const QDateTime masterEnd = QDateTime::fromString(master.endAt, Qt::ISODateWithMs);
  const QString startAt = patch.startAt.value_or(target.startAt);
  const QDateTime parsedStart = QDateTime::fromString(startAt, Qt::ISODateWithMs);
  return {.calendarId = master.calendarId,
          .title = patch.title.value_or(master.title),
          .startAt = startAt,
          .endAt = patch.endAt.value_or(parsedStart.addMSecs(masterStart.msecsTo(masterEnd))
                                            .toUTC()
                                            .toString(Qt::ISODateWithMs)),
          .allDay = patch.allDay.value_or(master.allDay),
          .description = patch.description.has_value() ? *patch.description : master.description,
          .location = patch.location.has_value() ? *patch.location : master.location,
          .startTimeZone = patch.startTimeZone.has_value() ? *patch.startTimeZone : master.startTimeZone,
          .endTimeZone = patch.endTimeZone.has_value() ? *patch.endTimeZone : master.endTimeZone,
          .colorId = patch.colorId.has_value() ? *patch.colorId : master.colorId,
          .transparency = patch.transparency.has_value() ? std::optional<QString>(*patch.transparency)
                                                         : master.transparency,
          .visibility = patch.visibility.has_value() ? std::optional<QString>(*patch.visibility)
                                                     : master.visibility,
          .attendeeEmails = patch.attendeeEmails.value_or(storedAttendeeEmails(master.attendeeEmailsJson)),
          .reminders = patch.reminders.value_or(storedReminderSettings(master)),
          .recurrenceRule = recurrenceRule,
          .richMetadata = {.attachmentsJson = patch.attachmentsJson.value_or(master.attachmentsJson),
                           .guestPermissionsJson = patch.guestPermissionsJson.value_or(
                               master.guestPermissionsJson),
                           .eventType = master.eventType.value_or(QStringLiteral("default")),
                           .statusPropertiesJson = patch.statusPropertiesJson.value_or(
                               master.statusPropertiesJson),
                           .sendUpdates = patch.sendUpdates.value_or(QStringLiteral("all"))}};
}

[[nodiscard]] CalendarEventCreateInput instanceInput(const StoredEventContext& occurrence,
                                                      const CalendarEventUpdateInput& patch) {
  const QDateTime start = QDateTime::fromString(
      patch.startAt.value_or(occurrence.startAt), Qt::ISODateWithMs);
  const QDateTime originalStart = QDateTime::fromString(occurrence.startAt, Qt::ISODateWithMs);
  const QDateTime originalEnd = QDateTime::fromString(occurrence.endAt, Qt::ISODateWithMs);
  return {.calendarId = occurrence.calendarId,
          .title = patch.title.value_or(occurrence.title),
          .startAt = patch.startAt.value_or(occurrence.startAt),
          .endAt = patch.endAt.value_or(
              start.addMSecs(originalStart.msecsTo(originalEnd)).toUTC().toString(Qt::ISODateWithMs)),
          .allDay = patch.allDay.value_or(occurrence.allDay),
          .description = patch.description.has_value() ? *patch.description : occurrence.description,
          .location = patch.location.has_value() ? *patch.location : occurrence.location,
          .startTimeZone = patch.startTimeZone.has_value() ? *patch.startTimeZone
                                                            : occurrence.startTimeZone,
          .endTimeZone = patch.endTimeZone.has_value() ? *patch.endTimeZone : occurrence.endTimeZone,
          .colorId = patch.colorId.has_value() ? *patch.colorId : occurrence.colorId,
          .transparency = patch.transparency.has_value() ? std::optional<QString>(*patch.transparency)
                                                         : occurrence.transparency,
          .visibility = patch.visibility.has_value() ? std::optional<QString>(*patch.visibility)
                                                     : occurrence.visibility,
          .attendeeEmails = patch.attendeeEmails.value_or(
              storedAttendeeEmails(occurrence.attendeeEmailsJson)),
          .reminders = patch.reminders.value_or(storedReminderSettings(occurrence)),
          .richMetadata = {.attachmentsJson = patch.attachmentsJson.value_or(occurrence.attachmentsJson),
                           .guestPermissionsJson = patch.guestPermissionsJson.value_or(
                               occurrence.guestPermissionsJson),
                           .eventType = occurrence.eventType.value_or(QStringLiteral("default")),
                           .statusPropertiesJson = patch.statusPropertiesJson.value_or(
                               occurrence.statusPropertiesJson),
                           .sendUpdates = patch.sendUpdates.value_or(QStringLiteral("all"))}};
}

[[nodiscard]] std::optional<AppError>
setMaterializedInstanceIdentity(SqliteConnection& connection,
                                const StoredEventContext& master,
                                const StoredEventContext& occurrence,
                                QString status,
                                const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr || !occurrence.originalStartAt.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Calendar instance storage is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_calendar_events
SET recurring_remote_id = ?2, original_start_at = ?3, status = ?4, updated_at = ?5
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepared = sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepared != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar-instance preparation failed (%1)"), prepared);
  }
  const std::optional<AppError> error = bindAll(
      statement, {bindText(statement, 1, occurrence.eventId), bindText(statement, 2, master.remoteId),
                  bindText(statement, 3, *occurrence.originalStartAt), bindText(statement, 4, status),
                  bindText(statement, 5, updatedAt)});
  if (error.has_value()) {
    sqlite3_finalize(statement);
    return error;
  }
  const int stepped = sqlite3_step(statement);
  const int changed = sqlite3_changes(handle);
  const int finalized = sqlite3_finalize(statement);
  if (stepped != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-instance update failed (%1)"), stepped);
  }
  if (finalized != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar-instance finalization failed (%1)"), finalized);
  }
  return changed == 1 ? std::nullopt
                      : std::optional<AppError>(AppError(
                            AppErrorCode::Database,
                            QStringLiteral("Calendar instance storage was not updated")));
}

[[nodiscard]] std::variant<StoredEventContext, AppError>
materializeVirtualInstance(SqliteConnection& connection,
                           const StoredEventContext& master,
                           const StoredEventContext& occurrence,
                           const CalendarEventUpdateInput& patch,
                           QString status,
                           const QString& updatedAt) {
  const std::variant<CalendarEventCreateInput, AppError> canonical =
      canonicalize(instanceInput(occurrence, patch));
  if (std::holds_alternative<AppError>(canonical)) {
    return std::get<AppError>(canonical);
  }
  const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
  const QString eventId = QStringLiteral("event:") + localId;
  const QString pendingRemoteId = QStringLiteral("pending:") + localId;
  const CalendarEventMutationResult created = createStoredEvent(
      connection, std::get<CalendarEventCreateInput>(canonical), eventId, pendingRemoteId, updatedAt);
  if (std::holds_alternative<AppError>(created)) {
    return std::get<AppError>(created);
  }
  const std::variant<std::optional<StoredEventContext>, AppError> context =
      readEventContext(connection, eventId);
  if (std::holds_alternative<AppError>(context) ||
      !std::get<std::optional<StoredEventContext>>(context).has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Materialized calendar instance is unavailable"));
  }
  StoredEventContext stored = *std::get<std::optional<StoredEventContext>>(context);
  stored.originalStartAt = occurrence.originalStartAt;
  if (const std::optional<AppError> error =
          setMaterializedInstanceIdentity(connection, master, stored, std::move(status), updatedAt);
      error.has_value()) {
    return *error;
  }
  const std::variant<std::optional<StoredEventContext>, AppError> after =
      readEventContext(connection, eventId);
  if (std::holds_alternative<AppError>(after) ||
      !std::get<std::optional<StoredEventContext>>(after).has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Materialized calendar instance is unavailable"));
  }
  return *std::get<std::optional<StoredEventContext>>(after);
}

[[nodiscard]] std::optional<AppError>
queueInstanceMutation(SqliteConnection& connection,
                      const StoredEventContext& original,
                      const StoredEventContext& materialized,
                      QString operation,
                      const QString& updatedAt,
                      const QString& sendUpdates = QStringLiteral("all"),
                      bool selfResponseOnly = false) {
  if (!original.recurringRemoteId.has_value() || !original.originalStartAt.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Calendar instance identity is unavailable"));
  }
  QJsonObject payload{{QStringLiteral("calendarId"), materialized.calendarRemoteId},
                      {QStringLiteral("localCalendarId"), materialized.calendarId},
                      {QStringLiteral("localEventId"), materialized.eventId},
                      {QStringLiteral("recurringRemoteId"), *original.recurringRemoteId},
                      {QStringLiteral("originalStartAt"), *original.originalStartAt}};
  if (operation == QStringLiteral("event.instance.update")) {
    QJsonObject patch = eventPatchBody(original, materialized);
    if (patch.isEmpty()) {
      return std::nullopt;
    }
    if (selfResponseOnly) {
      QJsonArray selfAttendee;
      for (const QJsonValue& value : storedArray(materialized.attendeeDetailsJson)) {
        if (value.isObject() && value.toObject().value(QStringLiteral("self")).toBool()) {
          selfAttendee.append(value);
          break;
        }
      }
      if (!selfAttendee.isEmpty()) {
        patch.insert(QStringLiteral("attendees"), std::move(selfAttendee));
        patch.insert(QStringLiteral("attendeesOmitted"), true);
      }
    }
    payload.insert(QStringLiteral("event"), patch);
    payload.insert(QStringLiteral("sendUpdates"), sendUpdates);
  }
  const std::variant<std::optional<ActiveEventMutation>, AppError> active =
      findActiveEventMutation(connection, materialized.eventId);
  if (std::holds_alternative<AppError>(active)) {
    return std::get<AppError>(active);
  }
  const std::optional<ActiveEventMutation>& existing =
      std::get<std::optional<ActiveEventMutation>>(active);
  if (existing.has_value()) {
    const QJsonValue metadata = existing->payload.value(QString::fromLatin1(kConflictMetadataKey));
    if (!metadata.isObject()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored calendar-instance mutation is invalid"));
    }
    if (operation == QStringLiteral("event.instance.update") &&
        existing->operation == QStringLiteral("event.instance.update")) {
      payload = mergeEventPatch(existing->payload, std::move(payload));
    }
    payload.insert(QString::fromLatin1(kConflictMetadataKey), metadata);
    return replaceActiveEventMutation(connection, *existing, std::move(operation), std::move(payload), updatedAt);
  }
  payload = withConflictMetadata(std::move(payload), eventSnapshot(original), std::nullopt);
  const EventMutationInsertResult inserted =
      insertEventMutation(connection, materialized, std::move(operation), std::move(payload), updatedAt);
  return std::holds_alternative<AppError>(inserted)
             ? std::optional<AppError>(std::get<AppError>(inserted))
             : std::nullopt;
}

[[nodiscard]] std::optional<AppError>
setMutationDependency(SqliteConnection& connection,
                      const QString& eventId,
                      const QString& dependency,
                      const QString& updatedAt) {
  const std::variant<std::optional<ActiveEventMutation>, AppError> active =
      findActiveEventMutation(connection, eventId);
  if (std::holds_alternative<AppError>(active)) {
    return std::get<AppError>(active);
  }
  const std::optional<ActiveEventMutation>& mutation =
      std::get<std::optional<ActiveEventMutation>>(active);
  if (!mutation.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Recurring successor mutation is unavailable"));
  }
  QJsonObject payload = mutation->payload;
  payload.insert(QStringLiteral("dependsOnMutationId"), dependency);
  return replaceActiveEventMutation(connection, *mutation, mutation->operation, std::move(payload), updatedAt);
}

[[nodiscard]] std::optional<AppError>
hideSeriesRows(SqliteConnection& connection,
               const StoredEventContext& master,
               const std::optional<QString>& fromOriginalStart,
               const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  const char* sql = fromOriginalStart.has_value()
                        ? "UPDATE local_calendar_events SET deleted_at = ?4, updated_at = ?4 "
                          "WHERE calendar_id = ?1 AND recurring_remote_id = ?2 AND "
                          "original_start_at >= ?3 AND deleted_at IS NULL"
                        : "UPDATE local_calendar_events SET deleted_at = ?3, updated_at = ?3 "
                          "WHERE calendar_id = ?1 AND (id = ?2 OR recurring_remote_id = ?4) "
                          "AND deleted_at IS NULL";
  sqlite3_stmt* statement = nullptr;
  const int prepared = sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepared != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite recurring-event hide preparation failed (%1)"), prepared);
  }
  std::optional<AppError> error;
  if (fromOriginalStart.has_value()) {
    error = bindAll(statement, {bindText(statement, 1, master.calendarId),
                                bindText(statement, 2, master.remoteId),
                                bindText(statement, 3, *fromOriginalStart),
                                bindText(statement, 4, updatedAt)});
  } else {
    error = bindAll(statement, {bindText(statement, 1, master.calendarId),
                                bindText(statement, 2, master.eventId),
                                bindText(statement, 3, updatedAt),
                                bindText(statement, 4, master.remoteId)});
  }
  if (error.has_value()) {
    return error;
  }
  const int stepped = sqlite3_step(statement);
  const int finalized = sqlite3_finalize(statement);
  if (stepped != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite recurring-event hide failed (%1)"), stepped);
  }
  return finalized == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite recurring-event hide finalization failed (%1)"), finalized));
}

[[nodiscard]] CalendarEventMutationResult
reconcileStoredGoogleEvent(SqliteConnection& connection,
                           const CalendarEventRemoteReconciliationInput& input,
                           const QString& updatedAt) {
  SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
  if (std::holds_alternative<AppError>(transactionResult)) {
    return std::get<AppError>(std::move(transactionResult));
  }
  SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar-event connection is unavailable"));
  }
  constexpr char eventSql[] = R"(
UPDATE local_calendar_events
SET remote_id = CASE WHEN remote_id LIKE 'pending:%' THEN ?2 ELSE remote_id END,
    etag = COALESCE(?3, etag),
    updated_at = ?4
WHERE id = ?1
  AND (remote_id = ?2 OR remote_id LIKE 'pending:%')
)";
  sqlite3_stmt* eventStatement = nullptr;
  const int eventPrepareResult = sqlite3_prepare_v3(
      handle, eventSql, -1, SQLITE_PREPARE_PERSISTENT, &eventStatement, nullptr);
  if (eventPrepareResult != SQLITE_OK) {
    sqlite3_finalize(eventStatement);
    return databaseError(
        QStringLiteral("SQLite calendar-event reconciliation preparation failed (%1)"),
        eventPrepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(eventStatement,
                  {bindText(eventStatement, 1, input.localEventId),
                   bindText(eventStatement, 2, input.remoteEventId),
                   bindOptionalText(eventStatement, 3, input.remoteEtag),
                   bindText(eventStatement, 4, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int eventStepResult = sqlite3_step(eventStatement);
  const int eventChangedRows = sqlite3_changes(handle);
  const int eventFinalizeResult = sqlite3_finalize(eventStatement);
  if (eventStepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite calendar-event reconciliation failed (%1)"),
                         eventStepResult);
  }
  if (eventFinalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite calendar-event reconciliation finalization failed (%1)"),
        eventFinalizeResult);
  }
  if (eventChangedRows != 1) {
    return validationError(QStringLiteral("Calendar event is unavailable for Google reconciliation"));
  }
  constexpr char pendingSql[] = R"(
SELECT id, payload_json
FROM local_pending_mutations
WHERE resource_type = 'event' AND resource_id = ?1 AND status IN ('pending', 'failed')
)";
  sqlite3_stmt* pendingStatement = nullptr;
  const int pendingPrepareResult = sqlite3_prepare_v3(
      handle, pendingSql, -1, SQLITE_PREPARE_PERSISTENT, &pendingStatement, nullptr);
  if (pendingPrepareResult != SQLITE_OK) {
    sqlite3_finalize(pendingStatement);
    return databaseError(
        QStringLiteral("SQLite pending calendar-event reconciliation preparation failed (%1)"),
        pendingPrepareResult);
  }
  if (const std::optional<AppError> error = bindText(pendingStatement, 1, input.localEventId);
      error.has_value()) {
    sqlite3_finalize(pendingStatement);
    return *error;
  }
  struct PendingPayload final {
    QString mutationId;
    QJsonObject payload;
  };
  QList<PendingPayload> pendingPayloads;
  int pendingStepResult = SQLITE_ROW;
  while ((pendingStepResult = sqlite3_step(pendingStatement)) == SQLITE_ROW) {
    const std::optional<QString> mutationId = optionalText(pendingStatement, 0);
    const std::optional<QString> payloadJson = optionalText(pendingStatement, 1);
    QJsonParseError parseError;
    const QJsonDocument document = payloadJson.has_value()
                                       ? QJsonDocument::fromJson(payloadJson->toUtf8(), &parseError)
                                       : QJsonDocument();
    if (!mutationId.has_value() || !payloadJson.has_value() ||
        parseError.error != QJsonParseError::NoError || !document.isObject()) {
      sqlite3_finalize(pendingStatement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored pending calendar-event mutation is invalid"));
    }
    QJsonObject payload = document.object();
    const QJsonValue remoteEventId = payload.value(QStringLiteral("remoteEventId"));
    if (remoteEventId.isString() && isPendingRemoteId(remoteEventId.toString())) {
      payload.insert(QStringLiteral("remoteEventId"), input.remoteEventId);
    }
    if (input.remoteEtag.has_value()) {
      const QJsonValue metadataValue = payload.value(QString::fromLatin1(kConflictMetadataKey));
      if (!metadataValue.isObject()) {
        sqlite3_finalize(pendingStatement);
        return AppError(AppErrorCode::Database,
                        QStringLiteral("Stored pending calendar-event mutation is invalid"));
      }
      QJsonObject metadata = metadataValue.toObject();
      metadata.insert(QStringLiteral("etag"), *input.remoteEtag);
      payload.insert(QString::fromLatin1(kConflictMetadataKey), std::move(metadata));
    }
    pendingPayloads.append({.mutationId = *mutationId, .payload = std::move(payload)});
  }
  const int pendingFinalizeResult = sqlite3_finalize(pendingStatement);
  if (pendingStepResult != SQLITE_DONE) {
    return databaseError(
        QStringLiteral("SQLite pending calendar-event reconciliation lookup failed (%1)"),
        pendingStepResult);
  }
  if (pendingFinalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite pending calendar-event reconciliation lookup finalization failed (%1)"),
        pendingFinalizeResult);
  }
  constexpr char updateSql[] = R"(
UPDATE local_pending_mutations
SET payload_json = ?2, updated_at = ?3
WHERE id = ?1 AND status IN ('pending', 'failed')
)";
  for (const PendingPayload& pending : pendingPayloads) {
    sqlite3_stmt* updateStatement = nullptr;
    const int updatePrepareResult =
        sqlite3_prepare_v3(handle, updateSql, -1, SQLITE_PREPARE_PERSISTENT, &updateStatement,
                           nullptr);
    if (updatePrepareResult != SQLITE_OK) {
      sqlite3_finalize(updateStatement);
      return databaseError(
          QStringLiteral("SQLite pending calendar-event reconciliation update preparation failed (%1)"),
          updatePrepareResult);
    }
    const QString payloadJson =
        QString::fromUtf8(QJsonDocument(pending.payload).toJson(QJsonDocument::Compact));
    if (const std::optional<AppError> error =
            bindAll(updateStatement,
                    {bindText(updateStatement, 1, pending.mutationId),
                     bindText(updateStatement, 2, payloadJson),
                     bindText(updateStatement, 3, updatedAt)});
        error.has_value()) {
      return *error;
    }
    const int updateStepResult = sqlite3_step(updateStatement);
    const int updateChangedRows = sqlite3_changes(handle);
    const int updateFinalizeResult = sqlite3_finalize(updateStatement);
    if (updateStepResult != SQLITE_DONE) {
      return databaseError(
          QStringLiteral("SQLite pending calendar-event reconciliation update failed (%1)"),
          updateStepResult);
    }
    if (updateFinalizeResult != SQLITE_OK) {
      return databaseError(
          QStringLiteral("SQLite pending calendar-event reconciliation update finalization failed (%1)"),
          updateFinalizeResult);
    }
    if (updateChangedRows != 1) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Pending calendar-event mutation was unavailable for reconciliation"));
    }
  }
  if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
    return *error;
  }
  return CalendarEventMutationReceipt{.eventId = input.localEventId, .updatedAt = updatedAt};
}

} // namespace

CalendarMutationService::CalendarMutationService(FilePath databasePath, const Clock& clock)
    : clock_(clock), writerQueue_(std::move(databasePath)),
      initialization_(writerQueue_
                          .enqueue([](SqliteConnection& connection) -> SqliteWriteResult {
                            const SqliteMigrationRunResultOrError result =
                                LocalSchema::initialize(connection);
                            return std::holds_alternative<AppError>(result)
                                       ? std::optional<AppError>(std::get<AppError>(result))
                                       : std::nullopt;
                          })
                          .share()) {}

std::shared_future<SqliteWriteResult> CalendarMutationService::ready() const {
  return initialization_;
}

std::variant<CalendarEventCreateInput, AppError>
CalendarMutationService::validateCreate(CalendarEventCreateInput input) {
  return canonicalize(std::move(input));
}

std::future<CalendarEventMutationResult>
CalendarMutationService::create(CalendarEventCreateInput input) {
  const std::variant<CalendarEventCreateInput, AppError> canonical =
      validateCreate(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(CalendarEventMutationResult(std::get<AppError>(canonical)));
  }
  const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
  const QString eventId = QStringLiteral("event:") + localId;
  const QString remoteId = QStringLiteral("pending:") + localId;
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::get<CalendarEventCreateInput>(canonical), eventId, remoteId, updatedAt](
          SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return CalendarEventMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        CalendarEventMutationResult created =
            createStoredEvent(connection, input, eventId, remoteId, updatedAt);
        if (std::holds_alternative<AppError>(created)) {
          return created;
        }
        const std::variant<std::optional<StoredEventContext>, AppError> contextResult =
            readEventContext(connection, eventId);
        if (std::holds_alternative<AppError>(contextResult)) {
          return CalendarEventMutationResult(std::get<AppError>(contextResult));
        }
        const std::optional<StoredEventContext>& context =
            std::get<std::optional<StoredEventContext>>(contextResult);
        if (!context.has_value()) {
          return CalendarEventMutationResult(
              AppError(AppErrorCode::Database, QStringLiteral("Created calendar event is unavailable")));
        }
        if (const std::optional<AppError> error = queueEventMutation(
                connection,
                *context,
                context,
                QStringLiteral("event.create"),
                updatedAt,
                input.richMetadata.sendUpdates);
            error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        return created;
      });
}

std::future<CalendarEventBatchMutationResult>
CalendarMutationService::createBatch(QList<CalendarEventCreateInput> inputs) {
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [inputs = std::move(inputs), updatedAt](SqliteConnection& connection) mutable {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return CalendarEventBatchMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        CalendarEventBatchMutationResult created =
            createBatchWithinTransaction(connection, std::move(inputs), updatedAt);
        if (std::holds_alternative<AppError>(created)) {
          return created;
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return CalendarEventBatchMutationResult(*error);
        }
        return created;
      });
}

CalendarEventBatchMutationResult CalendarMutationService::createBatchWithinTransaction(
    SqliteConnection& connection,
    QList<CalendarEventCreateInput> inputs,
    const QString& updatedAt) {
  constexpr qsizetype kMaximumBatchSize = 1'000;
  if (inputs.isEmpty() || inputs.size() > kMaximumBatchSize) {
    return validationError(QStringLiteral("Calendar event creation batch is invalid"));
  }
  struct PendingCreate final {
    CalendarEventCreateInput input;
    QString eventId;
    QString remoteId;
  };
  QList<PendingCreate> creates;
  creates.reserve(inputs.size());
  for (CalendarEventCreateInput& input : inputs) {
    const std::variant<CalendarEventCreateInput, AppError> canonical =
        validateCreate(std::move(input));
    if (std::holds_alternative<AppError>(canonical)) {
      return std::get<AppError>(canonical);
    }
    const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
    creates.append({.input = std::get<CalendarEventCreateInput>(canonical),
                    .eventId = QStringLiteral("event:") + localId,
                    .remoteId = QStringLiteral("pending:") + localId});
  }
  QList<CalendarEventMutationReceipt> receipts;
  receipts.reserve(creates.size());
  for (const PendingCreate& create : creates) {
    CalendarEventMutationResult created =
        createStoredEvent(connection, create.input, create.eventId, create.remoteId, updatedAt);
    if (std::holds_alternative<AppError>(created)) {
      return std::get<AppError>(std::move(created));
    }
    const std::variant<std::optional<StoredEventContext>, AppError> contextResult =
        readEventContext(connection, create.eventId);
    if (std::holds_alternative<AppError>(contextResult)) {
      return std::get<AppError>(contextResult);
    }
    const std::optional<StoredEventContext>& context =
        std::get<std::optional<StoredEventContext>>(contextResult);
    if (!context.has_value()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Created calendar event is unavailable"));
    }
    if (const std::optional<AppError> error = queueEventMutation(connection,
                                                                 *context,
                                                                 context,
                                                                 QStringLiteral("event.create"),
                                                                 updatedAt,
                                                                 create.input.richMetadata.sendUpdates);
        error.has_value()) {
      return *error;
    }
    receipts.append(std::get<CalendarEventMutationReceipt>(std::move(created)));
  }
  return receipts;
}

std::future<CalendarEventMutationResult>
CalendarMutationService::update(CalendarEventUpdateInput input) {
  const std::variant<CalendarEventUpdateInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(CalendarEventMutationResult(std::get<AppError>(canonical)));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult([input = std::get<CalendarEventUpdateInput>(canonical),
                                     updatedAt](SqliteConnection& connection) {
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return CalendarEventMutationResult(std::get<AppError>(std::move(transactionResult)));
    }
    SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
    const std::variant<std::optional<StoredEventContext>, AppError> beforeResult =
        readEventContext(connection, input.eventId);
    if (std::holds_alternative<AppError>(beforeResult)) {
      return CalendarEventMutationResult(std::get<AppError>(beforeResult));
    }
    const std::optional<StoredEventContext>& before =
        std::get<std::optional<StoredEventContext>>(beforeResult);
    if (!before.has_value()) {
      return CalendarEventMutationResult(
          validationError(QStringLiteral("Calendar event is unavailable for update")));
    }
    if (!isWritableCalendar(before->calendarAccessRole)) {
      return CalendarEventMutationResult(
          validationError(QStringLiteral("Calendar is read-only for event updates")));
    }
    if (!isMutableEventType(before->eventType) &&
        (!isEditableEventType(before->eventType) || !input.statusPropertiesJson.has_value())) {
      return CalendarEventMutationResult(
          validationError(QStringLiteral("Calendar event type is immutable")));
    }
    if (before->recurrenceRule.has_value() || before->recurringRemoteId.has_value()) {
      return CalendarEventMutationResult(validationError(
          QStringLiteral("Recurring events require an explicit recurrence scope")));
    }
    if (input.calendarId.has_value() && *input.calendarId != before->calendarId &&
        !canMoveFromCalendar(before->calendarAccessRole)) {
      return CalendarEventMutationResult(
          validationError(QStringLiteral("Only owner-calendar events can move")));
    }
    CalendarEventMutationResult updated = updateStoredEvent(connection, input, *before, updatedAt);
    if (std::holds_alternative<AppError>(updated)) {
      return updated;
    }
    const std::variant<std::optional<StoredEventContext>, AppError> afterResult =
        readEventContext(connection, input.eventId);
    if (std::holds_alternative<AppError>(afterResult)) {
      return CalendarEventMutationResult(std::get<AppError>(afterResult));
    }
    const std::optional<StoredEventContext>& after =
        std::get<std::optional<StoredEventContext>>(afterResult);
    if (!after.has_value()) {
      return CalendarEventMutationResult(
          AppError(AppErrorCode::Database, QStringLiteral("Updated calendar event is unavailable")));
    }
    const QString operation = before->calendarId == after->calendarId
                                  ? QStringLiteral("event.update")
                                  : QStringLiteral("event.move");
    if (const std::optional<AppError> error =
            queueEventMutation(connection,
                               *before,
                               after,
                               operation,
                               updatedAt,
                               input.sendUpdates.value_or(QStringLiteral("all")),
                               input.selfResponseStatus.has_value() || input.selfResponseComment.has_value());
        error.has_value()) {
      return CalendarEventMutationResult(*error);
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return CalendarEventMutationResult(*error);
    }
    return updated;
  });
}

std::future<CalendarEventMutationResult>
CalendarMutationService::respond(QString eventId, QString responseStatus, QString responseComment) {
  return update({.eventId = std::move(eventId),
                 .selfResponseStatus = std::move(responseStatus),
                 .selfResponseComment = std::move(responseComment)});
}

std::future<CalendarEventMutationResult>
CalendarMutationService::updateScoped(CalendarEventScopedUpdateInput scopedInput) {
  const std::variant<CalendarEventUpdateInput, AppError> canonical =
      canonicalize(std::move(scopedInput.update));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(CalendarEventMutationResult(std::get<AppError>(canonical)));
  }
  if (scopedInput.scope != CalendarEventRecurrenceScope::ThisInstance &&
      scopedInput.scope != CalendarEventRecurrenceScope::ThisAndFollowing &&
      scopedInput.scope != CalendarEventRecurrenceScope::FullSeries) {
    return readyFuture(CalendarEventMutationResult(
        validationError(QStringLiteral("Calendar recurrence scope is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::get<CalendarEventUpdateInput>(canonical),
       scope = scopedInput.scope,
       updatedAt](SqliteConnection& connection) -> CalendarEventMutationResult {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return CalendarEventMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<ScopedEventTarget>, AppError> targetResult =
            readScopedEventTarget(connection, input.eventId);
        if (std::holds_alternative<AppError>(targetResult)) {
          return CalendarEventMutationResult(std::get<AppError>(targetResult));
        }
        const std::optional<ScopedEventTarget>& scopedTarget =
            std::get<std::optional<ScopedEventTarget>>(targetResult);
        if (!scopedTarget.has_value()) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("Calendar event is unavailable for update")));
        }
        const StoredEventContext* const target = &scopedTarget->event;
        if (!isWritableCalendar(target->calendarAccessRole)) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("Calendar is read-only for event updates")));
        }
        if (!isMutableEventType(target->eventType) &&
            (!isEditableEventType(target->eventType) || !input.statusPropertiesJson.has_value())) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("Calendar event type is immutable")));
        }
        const bool recurring = target->recurrenceRule.has_value() || target->recurringRemoteId.has_value();
        if (!recurring && scope != CalendarEventRecurrenceScope::ThisInstance) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("A recurrence scope requires a recurring event")));
        }
        if (scope == CalendarEventRecurrenceScope::ThisInstance) {
          if (scopedTarget->isVirtualInstance) {
            const std::variant<std::optional<StoredEventContext>, AppError> masterResult =
                readSeriesMasterContext(connection, *target);
            if (std::holds_alternative<AppError>(masterResult) ||
                !std::get<std::optional<StoredEventContext>>(masterResult).has_value()) {
              return CalendarEventMutationResult(validationError(
                  QStringLiteral("Recurring event master is unavailable for this instance")));
            }
            const std::variant<StoredEventContext, AppError> materialized = materializeVirtualInstance(
                connection,
                *std::get<std::optional<StoredEventContext>>(masterResult),
                *target,
                input,
                QStringLiteral("confirmed"),
                updatedAt);
            if (std::holds_alternative<AppError>(materialized)) {
              return CalendarEventMutationResult(std::get<AppError>(materialized));
            }
          if (const std::optional<AppError> error = queueInstanceMutation(
                    connection,
                    *target,
                    std::get<StoredEventContext>(materialized),
                    QStringLiteral("event.instance.update"),
                    updatedAt,
                    input.sendUpdates.value_or(QStringLiteral("all")),
                    input.selfResponseStatus.has_value() || input.selfResponseComment.has_value());
                error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            return CalendarEventMutationReceipt{.eventId = std::get<StoredEventContext>(materialized).eventId,
                                                .updatedAt = updatedAt};
          }
          if (target->recurrenceRule.has_value()) {
            return CalendarEventMutationResult(validationError(
                QStringLiteral("Use full-series scope to edit a recurring event master")));
          }
          if (target->recurringRemoteId.has_value() && input.recurrenceRule.has_value()) {
            return CalendarEventMutationResult(validationError(
                QStringLiteral("An individual recurrence instance cannot change its rule")));
          }
          if (input.calendarId.has_value() && *input.calendarId != target->calendarId &&
              !canMoveFromCalendar(target->calendarAccessRole)) {
            return CalendarEventMutationResult(
                validationError(QStringLiteral("Only owner-calendar events can move")));
          }
          CalendarEventMutationResult updated = updateStoredEvent(connection, input, *target, updatedAt);
          if (std::holds_alternative<AppError>(updated)) {
            return updated;
          }
          const std::variant<std::optional<StoredEventContext>, AppError> afterResult =
              readEventContext(connection, input.eventId);
          if (std::holds_alternative<AppError>(afterResult) ||
              !std::get<std::optional<StoredEventContext>>(afterResult).has_value()) {
            return CalendarEventMutationResult(AppError(
                AppErrorCode::Database, QStringLiteral("Updated calendar event is unavailable")));
          }
          const StoredEventContext& after = *std::get<std::optional<StoredEventContext>>(afterResult);
          if (target->recurringRemoteId.has_value() && target->originalStartAt.has_value() &&
              isPendingRemoteId(target->remoteId)) {
            if (const std::optional<AppError> error = queueInstanceMutation(
                    connection,
                    *target,
                    after,
                    QStringLiteral("event.instance.update"),
                    updatedAt,
                    input.sendUpdates.value_or(QStringLiteral("all")),
                    input.selfResponseStatus.has_value() || input.selfResponseComment.has_value());
                error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            return updated;
          }
          const QString operation = target->calendarId == after.calendarId
                                        ? QStringLiteral("event.update")
                                        : QStringLiteral("event.move");
          if (const std::optional<AppError> error =
                  queueEventMutation(connection,
                                     *target,
                                     after,
                                     operation,
                                     updatedAt,
                                     input.sendUpdates.value_or(QStringLiteral("all")),
                                     input.selfResponseStatus.has_value() || input.selfResponseComment.has_value());
              error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          return updated;
        }
        const std::variant<std::optional<StoredEventContext>, AppError> masterResult =
            readSeriesMasterContext(connection, *target);
        if (std::holds_alternative<AppError>(masterResult)) {
          return CalendarEventMutationResult(std::get<AppError>(masterResult));
        }
        const std::optional<StoredEventContext>& master =
            std::get<std::optional<StoredEventContext>>(masterResult);
        if (!master.has_value() || !master->recurrenceRule.has_value()) {
          return CalendarEventMutationResult(validationError(
              QStringLiteral("Recurring event master is unavailable for this scope")));
        }
        if (input.calendarId.has_value() && *input.calendarId != master->calendarId) {
          return CalendarEventMutationResult(validationError(
              QStringLiteral("Recurring event series cannot move between calendars")));
        }
        if (scope == CalendarEventRecurrenceScope::FullSeries) {
          CalendarEventUpdateInput masterInput = input;
          masterInput.eventId = master->eventId;
          masterInput.calendarId = std::nullopt;
          CalendarEventMutationResult updated =
              updateStoredEvent(connection, masterInput, *master, updatedAt);
          if (std::holds_alternative<AppError>(updated)) {
            return updated;
          }
          const std::variant<std::optional<StoredEventContext>, AppError> afterResult =
              readEventContext(connection, master->eventId);
          if (std::holds_alternative<AppError>(afterResult) ||
              !std::get<std::optional<StoredEventContext>>(afterResult).has_value()) {
            return CalendarEventMutationResult(AppError(
                AppErrorCode::Database, QStringLiteral("Updated recurring-event master is unavailable")));
          }
          if (const std::optional<AppError> error = queueEventMutation(
                  connection,
                  *master,
                  *std::get<std::optional<StoredEventContext>>(afterResult),
                  QStringLiteral("event.update"),
                  updatedAt,
                  input.sendUpdates.value_or(QStringLiteral("all")),
                  input.selfResponseStatus.has_value() || input.selfResponseComment.has_value());
              error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          return updated;
        }
        if (!target->recurringRemoteId.has_value() || !target->originalStartAt.has_value()) {
          return CalendarEventMutationResult(validationError(
              QStringLiteral("This-and-following scope requires a stored recurring instance")));
        }
        const std::optional<QString> trimmed =
            truncateRecurrenceRule(*master->recurrenceRule, *target->originalStartAt, master->allDay);
        const std::optional<QString> inherited = successorRecurrenceRule(*master, *target->originalStartAt);
        if (!trimmed.has_value() || !inherited.has_value()) {
          return CalendarEventMutationResult(validationError(QStringLiteral(
              "This-and-following scope requires a supported RRULE occurrence")));
        }
        CalendarEventUpdateInput masterInput{.eventId = master->eventId,
                                             .recurrenceRule = std::optional<std::optional<QString>>(*trimmed)};
        CalendarEventMutationResult truncated =
            updateStoredEvent(connection, masterInput, *master, updatedAt);
        if (std::holds_alternative<AppError>(truncated)) {
          return truncated;
        }
        const std::variant<std::optional<StoredEventContext>, AppError> trimmedResult =
            readEventContext(connection, master->eventId);
        if (std::holds_alternative<AppError>(trimmedResult) ||
            !std::get<std::optional<StoredEventContext>>(trimmedResult).has_value()) {
          return CalendarEventMutationResult(AppError(
              AppErrorCode::Database, QStringLiteral("Trimmed recurring-event master is unavailable")));
        }
        const StoredEventContext& trimmedMaster =
            *std::get<std::optional<StoredEventContext>>(trimmedResult);
        if (const std::optional<AppError> error = queueEventMutation(
                connection,
                *master,
                trimmedMaster,
                QStringLiteral("event.update"),
                updatedAt,
                input.sendUpdates.value_or(QStringLiteral("all")),
                input.selfResponseStatus.has_value() || input.selfResponseComment.has_value());
            error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        const std::variant<std::optional<ActiveEventMutation>, AppError> masterMutation =
            findActiveEventMutation(connection, master->eventId);
        if (std::holds_alternative<AppError>(masterMutation) ||
            !std::get<std::optional<ActiveEventMutation>>(masterMutation).has_value()) {
          return CalendarEventMutationResult(AppError(
              AppErrorCode::Database, QStringLiteral("Recurring-event trim mutation is unavailable")));
        }
        const std::optional<QString> successorRule = input.recurrenceRule.has_value()
                                                          ? *input.recurrenceRule
                                                          : inherited;
        const std::variant<CalendarEventCreateInput, AppError> successorCanonical = canonicalize(
            successorInput(*master, *target, input, successorRule.value_or(QString())));
        if (std::holds_alternative<AppError>(successorCanonical)) {
          return CalendarEventMutationResult(std::get<AppError>(successorCanonical));
        }
        const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
        const QString successorId = QStringLiteral("event:") + localId;
        const QString successorRemoteId = QStringLiteral("pending:") + localId;
        CalendarEventMutationResult created = createStoredEvent(connection,
                                                                 std::get<CalendarEventCreateInput>(successorCanonical),
                                                                 successorId,
                                                                 successorRemoteId,
                                                                 updatedAt);
        if (std::holds_alternative<AppError>(created)) {
          return created;
        }
        const std::variant<std::optional<StoredEventContext>, AppError> successorResult =
            readEventContext(connection, successorId);
        if (std::holds_alternative<AppError>(successorResult) ||
            !std::get<std::optional<StoredEventContext>>(successorResult).has_value()) {
          return CalendarEventMutationResult(AppError(
              AppErrorCode::Database, QStringLiteral("Recurring successor is unavailable")));
        }
        if (const std::optional<AppError> error = queueEventMutation(
                connection,
                *std::get<std::optional<StoredEventContext>>(successorResult),
                *std::get<std::optional<StoredEventContext>>(successorResult),
                QStringLiteral("event.create"),
                updatedAt,
                std::get<CalendarEventCreateInput>(successorCanonical).richMetadata.sendUpdates);
            error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        if (const std::optional<AppError> error = setMutationDependency(
                connection,
                successorId,
                std::get<std::optional<ActiveEventMutation>>(masterMutation)->id,
                updatedAt);
            error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        return created;
      });
}

std::future<CalendarEventMutationResult> CalendarMutationService::remove(QString eventId) {
  if (!isValidRequiredText(eventId, kMaximumIdentifierLength)) {
    return readyFuture(CalendarEventMutationResult(
        validationError(QStringLiteral("Calendar event deletion input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [eventId = std::move(eventId), updatedAt](SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return CalendarEventMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<StoredEventContext>, AppError> beforeResult =
            readEventContext(connection, eventId);
        if (std::holds_alternative<AppError>(beforeResult)) {
          return CalendarEventMutationResult(std::get<AppError>(beforeResult));
        }
        const std::optional<StoredEventContext>& before =
            std::get<std::optional<StoredEventContext>>(beforeResult);
        if (!before.has_value()) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("Calendar event is unavailable for deletion")));
        }
        if (!isWritableCalendar(before->calendarAccessRole)) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("Calendar is read-only for event deletion")));
        }
        if (!isEditableEventType(before->eventType)) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("Calendar event type is immutable")));
        }
        if (before->recurrenceRule.has_value() || before->recurringRemoteId.has_value()) {
          return CalendarEventMutationResult(validationError(
              QStringLiteral("Recurring events require an explicit recurrence scope")));
        }
        CalendarEventMutationResult removed = removeStoredEvent(connection, eventId, updatedAt);
        if (std::holds_alternative<AppError>(removed)) {
          return removed;
        }
        if (const std::optional<AppError> error = queueEventMutation(
                connection, *before, std::nullopt, QStringLiteral("event.delete"), updatedAt);
            error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        return removed;
      });
}

std::future<CalendarEventMutationResult> CalendarMutationService::restore(QString eventId) {
  if (!isValidRequiredText(eventId, kMaximumIdentifierLength)) {
    return readyFuture(CalendarEventMutationResult(
        validationError(QStringLiteral("Calendar event restoration input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [eventId = std::move(eventId), updatedAt](SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return CalendarEventMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<ActiveEventMutation>, AppError> activeResult =
            findActiveEventMutation(connection, eventId);
        if (std::holds_alternative<AppError>(activeResult)) {
          return CalendarEventMutationResult(std::get<AppError>(activeResult));
        }
        const std::optional<ActiveEventMutation>& active =
            std::get<std::optional<ActiveEventMutation>>(activeResult);
        const bool cancelPendingDelete =
            active.has_value() && active->operation == QStringLiteral("event.delete");
        const QString pendingRemoteId =
            QStringLiteral("pending:") + QUuid::createUuid().toString(QUuid::WithoutBraces);
        CalendarEventMutationResult restored = restoreStoredEvent(connection,
                                                                   eventId,
                                                                   !cancelPendingDelete,
                                                                   pendingRemoteId,
                                                                   updatedAt);
        if (std::holds_alternative<AppError>(restored)) {
          return restored;
        }
        const std::variant<std::optional<StoredEventContext>, AppError> afterResult =
            readEventContext(connection, eventId);
        if (std::holds_alternative<AppError>(afterResult)) {
          return CalendarEventMutationResult(std::get<AppError>(afterResult));
        }
        const std::optional<StoredEventContext>& after =
            std::get<std::optional<StoredEventContext>>(afterResult);
        if (!after.has_value()) {
          return CalendarEventMutationResult(AppError(
              AppErrorCode::Database, QStringLiteral("Restored calendar event is unavailable")));
        }
        std::optional<AppError> queueError;
        if (cancelPendingDelete) {
          queueError = removeActiveEventMutation(connection, *active);
        } else {
          queueError = queueEventMutation(connection,
                                          *after,
                                          after,
                                          QStringLiteral("event.create"),
                                          updatedAt);
        }
        if (queueError.has_value()) {
          return CalendarEventMutationResult(*queueError);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        return restored;
      });
}

std::future<CalendarEventMutationResult>
CalendarMutationService::removeScoped(CalendarEventScopedDeleteInput input) {
  if (!isValidRequiredText(input.eventId, kMaximumIdentifierLength) ||
      (input.scope != CalendarEventRecurrenceScope::ThisInstance &&
       input.scope != CalendarEventRecurrenceScope::ThisAndFollowing &&
       input.scope != CalendarEventRecurrenceScope::FullSeries)) {
    return readyFuture(CalendarEventMutationResult(
        validationError(QStringLiteral("Calendar recurrence deletion input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::move(input), updatedAt](SqliteConnection& connection) -> CalendarEventMutationResult {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return CalendarEventMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<ScopedEventTarget>, AppError> targetResult =
            readScopedEventTarget(connection, input.eventId);
        if (std::holds_alternative<AppError>(targetResult)) {
          return CalendarEventMutationResult(std::get<AppError>(targetResult));
        }
        const std::optional<ScopedEventTarget>& scopedTarget =
            std::get<std::optional<ScopedEventTarget>>(targetResult);
        if (!scopedTarget.has_value() || !isWritableCalendar(scopedTarget->event.calendarAccessRole) ||
            !isEditableEventType(scopedTarget->event.eventType)) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("Calendar event is unavailable for deletion")));
        }
        const StoredEventContext* const target = &scopedTarget->event;
        const bool recurring = target->recurrenceRule.has_value() || target->recurringRemoteId.has_value();
        if (!recurring && input.scope != CalendarEventRecurrenceScope::ThisInstance) {
          return CalendarEventMutationResult(
              validationError(QStringLiteral("A recurrence scope requires a recurring event")));
        }
        if (input.scope == CalendarEventRecurrenceScope::ThisInstance) {
          if (scopedTarget->isVirtualInstance) {
            const std::variant<std::optional<StoredEventContext>, AppError> masterResult =
                readSeriesMasterContext(connection, *target);
            if (std::holds_alternative<AppError>(masterResult) ||
                !std::get<std::optional<StoredEventContext>>(masterResult).has_value()) {
              return CalendarEventMutationResult(validationError(
                  QStringLiteral("Recurring event master is unavailable for this instance")));
            }
            const CalendarEventUpdateInput noPatch{.eventId = target->eventId};
            const std::variant<StoredEventContext, AppError> materialized = materializeVirtualInstance(
                connection,
                *std::get<std::optional<StoredEventContext>>(masterResult),
                *target,
                noPatch,
                QStringLiteral("cancelled"),
                updatedAt);
            if (std::holds_alternative<AppError>(materialized)) {
              return CalendarEventMutationResult(std::get<AppError>(materialized));
            }
            if (const std::optional<AppError> error = queueInstanceMutation(
                    connection,
                    *target,
                    std::get<StoredEventContext>(materialized),
                    QStringLiteral("event.instance.delete"),
                    updatedAt);
                error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            return CalendarEventMutationReceipt{.eventId = std::get<StoredEventContext>(materialized).eventId,
                                                .updatedAt = updatedAt};
          }
          if (target->recurrenceRule.has_value()) {
            return CalendarEventMutationResult(validationError(
                QStringLiteral("Use full-series scope to delete a recurring event master")));
          }
          if (target->recurringRemoteId.has_value() && target->originalStartAt.has_value() &&
              isPendingRemoteId(target->remoteId)) {
            const std::variant<std::optional<StoredEventContext>, AppError> masterResult =
                readSeriesMasterContext(connection, *target);
            if (std::holds_alternative<AppError>(masterResult) ||
                !std::get<std::optional<StoredEventContext>>(masterResult).has_value()) {
              return CalendarEventMutationResult(validationError(
                  QStringLiteral("Recurring event master is unavailable for this instance")));
            }
            if (const std::optional<AppError> error = setMaterializedInstanceIdentity(
                    connection,
                    *std::get<std::optional<StoredEventContext>>(masterResult),
                    *target,
                    QStringLiteral("cancelled"),
                    updatedAt);
                error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            const std::variant<std::optional<StoredEventContext>, AppError> cancelledResult =
                readEventContext(connection, target->eventId);
            if (std::holds_alternative<AppError>(cancelledResult) ||
                !std::get<std::optional<StoredEventContext>>(cancelledResult).has_value()) {
              return CalendarEventMutationResult(AppError(
                  AppErrorCode::Database, QStringLiteral("Cancelled calendar instance is unavailable")));
            }
            if (const std::optional<AppError> error = queueInstanceMutation(
                    connection,
                    *target,
                    *std::get<std::optional<StoredEventContext>>(cancelledResult),
                    QStringLiteral("event.instance.delete"),
                    updatedAt);
                error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
              return CalendarEventMutationResult(*error);
            }
            return CalendarEventMutationReceipt{.eventId = target->eventId, .updatedAt = updatedAt};
          }
          CalendarEventMutationResult removed = removeStoredEvent(connection, input.eventId, updatedAt);
          if (std::holds_alternative<AppError>(removed)) {
            return removed;
          }
          if (const std::optional<AppError> error = queueEventMutation(
                  connection, *target, std::nullopt, QStringLiteral("event.delete"), updatedAt);
              error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          return removed;
        }
        const std::variant<std::optional<StoredEventContext>, AppError> masterResult =
            readSeriesMasterContext(connection, *target);
        if (std::holds_alternative<AppError>(masterResult)) {
          return CalendarEventMutationResult(std::get<AppError>(masterResult));
        }
        const std::optional<StoredEventContext>& master =
            std::get<std::optional<StoredEventContext>>(masterResult);
        if (!master.has_value() || !master->recurrenceRule.has_value()) {
          return CalendarEventMutationResult(validationError(
              QStringLiteral("Recurring event master is unavailable for deletion")));
        }
        if (input.scope == CalendarEventRecurrenceScope::FullSeries) {
          if (const std::optional<AppError> error =
                  hideSeriesRows(connection, *master, std::nullopt, updatedAt);
              error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          if (const std::optional<AppError> error = queueEventMutation(
                  connection, *master, std::nullopt, QStringLiteral("event.delete"), updatedAt);
              error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
            return CalendarEventMutationResult(*error);
          }
          return CalendarEventMutationReceipt{.eventId = master->eventId, .updatedAt = updatedAt};
        }
        if (!target->recurringRemoteId.has_value() || !target->originalStartAt.has_value()) {
          return CalendarEventMutationResult(validationError(
              QStringLiteral("This-and-following scope requires a stored recurring instance")));
        }
        const std::optional<QString> trimmed =
            truncateRecurrenceRule(*master->recurrenceRule, *target->originalStartAt, master->allDay);
        if (!trimmed.has_value()) {
          return CalendarEventMutationResult(validationError(QStringLiteral(
              "This-and-following scope requires a supported RRULE occurrence")));
        }
        CalendarEventUpdateInput masterInput{.eventId = master->eventId,
                                             .recurrenceRule = std::optional<std::optional<QString>>(*trimmed)};
        CalendarEventMutationResult truncated =
            updateStoredEvent(connection, masterInput, *master, updatedAt);
        if (std::holds_alternative<AppError>(truncated)) {
          return truncated;
        }
        const std::variant<std::optional<StoredEventContext>, AppError> afterResult =
            readEventContext(connection, master->eventId);
        if (std::holds_alternative<AppError>(afterResult) ||
            !std::get<std::optional<StoredEventContext>>(afterResult).has_value()) {
          return CalendarEventMutationResult(AppError(
              AppErrorCode::Database, QStringLiteral("Trimmed recurring-event master is unavailable")));
        }
        if (const std::optional<AppError> error = queueEventMutation(
                connection,
                *master,
                *std::get<std::optional<StoredEventContext>>(afterResult),
                QStringLiteral("event.update"),
                updatedAt);
            error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        if (const std::optional<AppError> error =
                hideSeriesRows(connection, *master, target->originalStartAt, updatedAt);
            error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return CalendarEventMutationResult(*error);
        }
        return truncated;
      });
}

std::future<CalendarEventMutationSnapshotResult>
CalendarMutationService::inspect(QList<QString> eventIds) {
  constexpr qsizetype kMaximumInspectionSize = 500;
  if (eventIds.isEmpty() || eventIds.size() > kMaximumInspectionSize) {
    return readyFuture(CalendarEventMutationSnapshotResult(
        validationError(QStringLiteral("Calendar event inspection input is invalid"))));
  }
  QSet<QString> uniqueIds;
  for (const QString& eventId : eventIds) {
    if (!isValidRequiredText(eventId, kMaximumIdentifierLength) || uniqueIds.contains(eventId)) {
      return readyFuture(CalendarEventMutationSnapshotResult(
          validationError(QStringLiteral("Calendar event inspection input is invalid"))));
    }
    uniqueIds.insert(eventId);
  }
  return writerQueue_.enqueueResult([eventIds = std::move(eventIds)](SqliteConnection& connection) {
    QList<CalendarEventMutationSnapshot> snapshots;
    snapshots.reserve(eventIds.size());
    for (const QString& eventId : eventIds) {
      const std::variant<std::optional<StoredEventContext>, AppError> contextResult =
          readEventContext(connection, eventId);
      if (std::holds_alternative<AppError>(contextResult)) {
        return CalendarEventMutationSnapshotResult(std::get<AppError>(contextResult));
      }
      const std::optional<StoredEventContext>& context =
          std::get<std::optional<StoredEventContext>>(contextResult);
      if (!context.has_value()) {
        continue;
      }
      snapshots.append({.eventId = context->eventId,
                        .accountId = context->accountId,
                        .calendarId = context->calendarId,
                        .remoteId = context->remoteId,
                        .calendarAccessRole = context->calendarAccessRole,
                        .status = context->status,
                        .recurringRemoteId = context->recurringRemoteId,
                        .originalStartAt = context->originalStartAt,
                        .recurrenceRule = context->recurrenceRule,
                        .eventType = context->eventType,
                        .title = context->title,
                        .description = context->description,
                        .location = context->location,
                        .startAt = context->startAt,
                        .endAt = context->endAt,
                        .allDay = context->allDay,
                        .colorId = context->colorId,
                        .transparency = context->transparency,
                        .visibility = context->visibility,
                        .attendeeEmailsJson = context->attendeeEmailsJson,
                        .attendeeDetailsJson = context->attendeeDetailsJson,
                        .remindersJson = context->remindersJson,
                        .remindersUseDefault = context->remindersUseDefault,
                        .conferenceJson = context->conferenceJson,
                        .attachmentsJson = context->attachmentsJson,
                        .guestPermissionsJson = context->guestPermissionsJson,
                        .statusPropertiesJson = context->statusPropertiesJson});
    }
    return CalendarEventMutationSnapshotResult(std::move(snapshots));
  });
}

std::future<CalendarEventMutationResult>
CalendarMutationService::reconcileGoogleEvent(CalendarEventRemoteReconciliationInput input) {
  if (!isValidRequiredText(input.localEventId, kMaximumIdentifierLength) ||
      !isValidRequiredText(input.remoteEventId, kMaximumIdentifierLength) ||
      isPendingRemoteId(input.remoteEventId) ||
      (input.remoteEtag.has_value() &&
       (!isValidRequiredText(*input.remoteEtag, 4'096) || input.remoteEtag->contains(u'\r') ||
        input.remoteEtag->contains(u'\n')))) {
    return readyFuture(CalendarEventMutationResult(
        validationError(QStringLiteral("Google calendar-event reconciliation input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::move(input), updatedAt](SqliteConnection& connection) {
        return reconcileStoredGoogleEvent(connection, input, updatedAt);
      });
}

} // namespace hcb
