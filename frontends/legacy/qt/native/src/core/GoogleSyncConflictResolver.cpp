#include "core/GoogleSyncConflictResolver.h"

#include "core/GoogleHttpClient.h"
#include "core/SyncConflictStore.h"
#include "core/SyncThreeWayMerge.h"

#include <QDate>
#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSet>
#include <QString>
#include <QTime>
#include <QTimeZone>

#include <future>
#include <optional>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumResponseBytes = 262'144;
constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumEtagLength = 4'096;
constexpr qsizetype kMaximumAttendeeCount = 200;
constexpr qsizetype kMaximumReminderCount = 5;

struct RemoteSnapshot final {
  QJsonObject fields;
  std::optional<QString> etag;
};

using RemoteSnapshotResult = std::variant<RemoteSnapshot, GoogleApiError, AppError>;

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] AppError databaseError(QString message) {
  return AppError(AppErrorCode::Database, std::move(message));
}

[[nodiscard]] bool isValidText(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximumLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isPresent(const QJsonValue& value) {
  return !value.isUndefined() && !value.isNull();
}

[[nodiscard]] std::optional<QString> requiredString(const QJsonObject& object, QStringView key,
                                                     qsizetype maximumLength) {
  const QJsonValue value = object.value(key);
  return value.isString() && isValidText(value.toString(), maximumLength)
             ? std::optional<QString>(value.toString())
             : std::nullopt;
}

[[nodiscard]] std::optional<QString> optionalString(const QJsonObject& object, QStringView key,
                                                     qsizetype maximumLength) {
  const QJsonValue value = object.value(key);
  if (!isPresent(value)) {
    return std::optional<QString>{};
  }
  return value.isString() && value.toString().size() <= maximumLength &&
                 !value.toString().contains(QChar::Null)
             ? std::optional<QString>(value.toString())
             : std::nullopt;
}

[[nodiscard]] bool hasInvalidOptional(const std::optional<QString>& value,
                                      const QJsonObject& object,
                                      QStringView key) {
  return !value.has_value() && isPresent(object.value(key));
}

[[nodiscard]] bool hasExplicitOffset(const QString& value) {
  const qsizetype timeSeparator = value.indexOf(u'T');
  if (timeSeparator < 0) {
    return false;
  }
  const QStringView time = QStringView(value).sliced(timeSeparator + 1);
  return time.endsWith(u'Z') || time.contains(u'+') || time.contains(u'-');
}

[[nodiscard]] std::optional<QString> taskDue(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("due"));
  if (value.isUndefined() || value.isNull()) {
    return std::optional<QString>{};
  }
  if (!value.isString() || value.toString().size() < 10 || value.toString().size() > 64 ||
      value.toString().contains(QChar::Null)) {
    return std::nullopt;
  }
  const QDate date = QDate::fromString(value.toString().left(10), Qt::ISODate);
  return date.isValid() ? std::optional<QString>(date.toString(Qt::ISODate)) : std::nullopt;
}

[[nodiscard]] std::optional<QJsonObject> eventTime(const QJsonValue& value) {
  if (!value.isObject()) {
    return std::nullopt;
  }
  const QJsonObject source = value.toObject();
  const QJsonValue date = source.value(QStringLiteral("date"));
  const QJsonValue dateTime = source.value(QStringLiteral("dateTime"));
  const std::optional<QString> timeZone = optionalString(source, u"timeZone", 120);
  if (hasInvalidOptional(timeZone, source, u"timeZone") ||
      (timeZone.has_value() && !QTimeZone(timeZone->toUtf8()).isValid()) ||
      isPresent(date) == isPresent(dateTime)) {
    return std::nullopt;
  }
  QJsonObject result;
  if (isPresent(date)) {
    if (!date.isString()) {
      return std::nullopt;
    }
    const QDate parsed = QDate::fromString(date.toString(), Qt::ISODate);
    if (!parsed.isValid() || date.toString().size() != 10 || date.toString().contains(QChar::Null)) {
      return std::nullopt;
    }
    result.insert(QStringLiteral("date"), parsed.toString(Qt::ISODate));
  } else {
    if (!dateTime.isString() || dateTime.toString().size() > 64 ||
        dateTime.toString().contains(QChar::Null) || !dateTime.toString().contains(u'T')) {
      return std::nullopt;
    }
    QDateTime parsed = QDateTime::fromString(dateTime.toString(), Qt::ISODate);
    if (!parsed.isValid() || (!hasExplicitOffset(dateTime.toString()) && !timeZone.has_value())) {
      return std::nullopt;
    }
    if (!hasExplicitOffset(dateTime.toString())) {
      parsed = QDateTime(parsed.date(), parsed.time(), QTimeZone(timeZone->toUtf8()));
    }
    result.insert(QStringLiteral("dateTime"), parsed.toUTC().toString(Qt::ISODateWithMs));
  }
  if (timeZone.has_value()) {
    result.insert(QStringLiteral("timeZone"), *timeZone);
  }
  return result;
}

[[nodiscard]] std::optional<QJsonArray> eventAttendees(const QJsonValue& value) {
  if (value.isUndefined()) {
    return QJsonArray();
  }
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
    const std::optional<QString> email = optionalString(attendee, u"email", 254);
    const std::optional<QString> displayName = optionalString(attendee, u"displayName", 1'024);
    const std::optional<QString> comment = optionalString(attendee, u"comment", 4'096);
    const QJsonValue optional = attendee.value(QStringLiteral("optional"));
    const QJsonValue responseStatus = attendee.value(QStringLiteral("responseStatus"));
    const QJsonValue additionalGuests = attendee.value(QStringLiteral("additionalGuests"));
    const QJsonValue resource = attendee.value(QStringLiteral("resource"));
    const bool validStatus = responseStatus.isUndefined() ||
                             (responseStatus.isString() &&
                              (responseStatus.toString() == QStringLiteral("needsAction") ||
                               responseStatus.toString() == QStringLiteral("declined") ||
                               responseStatus.toString() == QStringLiteral("tentative") ||
                               responseStatus.toString() == QStringLiteral("accepted")));
    if (hasInvalidOptional(email, attendee, u"email") ||
        hasInvalidOptional(displayName, attendee, u"displayName") ||
        hasInvalidOptional(comment, attendee, u"comment") ||
        (!optional.isUndefined() && !optional.isBool()) || !validStatus ||
        (!additionalGuests.isUndefined() &&
         (!additionalGuests.isDouble() || additionalGuests.toInteger(-1) < 0 ||
          additionalGuests.toInteger(-1) > 10'000)) ||
        (!resource.isUndefined() && !resource.isBool())) {
      return std::nullopt;
    }
    QJsonObject canonical;
    if (email.has_value()) {
      const QString key = email->toCaseFolded();
      if (email->trimmed() != *email || emails.contains(key)) {
        return std::nullopt;
      }
      emails.insert(key);
      canonical.insert(QStringLiteral("email"), *email);
    }
    if (displayName.has_value()) canonical.insert(QStringLiteral("displayName"), *displayName);
    if (comment.has_value()) canonical.insert(QStringLiteral("comment"), *comment);
    if (!optional.isUndefined()) canonical.insert(QStringLiteral("optional"), optional);
    if (!responseStatus.isUndefined()) canonical.insert(QStringLiteral("responseStatus"), responseStatus);
    if (!additionalGuests.isUndefined()) {
      canonical.insert(QStringLiteral("additionalGuests"), additionalGuests.toInteger());
    }
    if (!resource.isUndefined()) canonical.insert(QStringLiteral("resource"), resource);
    result.append(canonical);
  }
  return result;
}

[[nodiscard]] std::optional<QJsonObject> eventReminders(const QJsonValue& value) {
  if (value.isUndefined()) {
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
  QJsonArray canonical;
  if (overrides.isArray()) {
    for (const QJsonValue& reminderValue : overrides.toArray()) {
      const QJsonObject reminder = reminderValue.toObject();
      const QJsonValue method = reminder.value(QStringLiteral("method"));
      const QJsonValue minutes = reminder.value(QStringLiteral("minutes"));
      if (!reminderValue.isObject() || !method.isString() ||
          (method.toString() != QStringLiteral("email") &&
           method.toString() != QStringLiteral("popup")) ||
          !minutes.isDouble() || minutes.toInteger(-1) < 0 || minutes.toInteger(-1) > 40'320) {
        return std::nullopt;
      }
      canonical.append(QJsonObject{{QStringLiteral("method"), method.toString()},
                                   {QStringLiteral("minutes"), minutes.toInteger()}});
    }
  }
  return QJsonObject{{QStringLiteral("useDefault"), useDefault.toBool(true)},
                     {QStringLiteral("overrides"), canonical}};
}

[[nodiscard]] std::optional<RemoteSnapshot> decodeTask(const GoogleHttpResponse& response,
                                                        const QString& expectedId) {
  if (response.body.size() > kMaximumResponseBytes) {
    return std::nullopt;
  }
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(response.body, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return std::nullopt;
  }
  const QJsonObject object = document.object();
  const std::optional<QString> id = requiredString(object, u"id", kMaximumIdentifierLength);
  const std::optional<QString> etag = requiredString(object, u"etag", kMaximumEtagLength);
  const std::optional<QString> title = optionalString(object, u"title", 500);
  const std::optional<QString> notes = optionalString(object, u"notes", 20'000);
  const QJsonValue dueValue = object.value(QStringLiteral("due"));
  const std::optional<QString> due = taskDue(object);
  const QJsonValue status = object.value(QStringLiteral("status"));
  const QJsonValue deleted = object.value(QStringLiteral("deleted"));
  if (!id.has_value() || *id != expectedId || !etag.has_value() || !title.has_value() ||
      hasInvalidOptional(notes, object, u"notes") ||
      (!(dueValue.isUndefined() || dueValue.isNull()) && !due.has_value()) ||
      (!status.isUndefined() &&
       (!status.isString() || (status.toString() != QStringLiteral("needsAction") &&
                               status.toString() != QStringLiteral("completed")))) ||
      (!deleted.isUndefined() && !deleted.isBool())) {
    return std::nullopt;
  }
  const bool isDeleted = deleted.isBool() && deleted.toBool();
  QJsonObject fields{{QStringLiteral("_deleted"), isDeleted},
                     {QStringLiteral("title"), *title},
                     {QStringLiteral("notes"), notes.has_value() ? QJsonValue(*notes)
                                                                  : QJsonValue::Null},
                     {QStringLiteral("status"), status.isString() ? status
                                                                     : QJsonValue("needsAction")},
                     {QStringLiteral("due"), due.has_value() ? QJsonValue(*due)
                                                               : QJsonValue::Null}};
  return RemoteSnapshot{.fields = std::move(fields), .etag = etag};
}

[[nodiscard]] std::optional<RemoteSnapshot> decodeTaskList(const GoogleHttpResponse& response,
                                                            const QString& expectedId) {
  if (response.body.size() > kMaximumResponseBytes) {
    return std::nullopt;
  }
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(response.body, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return std::nullopt;
  }
  const QJsonObject object = document.object();
  const std::optional<QString> id = requiredString(object, u"id", kMaximumIdentifierLength);
  const std::optional<QString> etag = requiredString(object, u"etag", kMaximumEtagLength);
  const std::optional<QString> title = requiredString(object, u"title", 500);
  if (!id.has_value() || *id != expectedId || !etag.has_value() || !title.has_value()) {
    return std::nullopt;
  }
  return RemoteSnapshot{.fields = {{QStringLiteral("title"), *title}}, .etag = etag};
}

[[nodiscard]] std::optional<RemoteSnapshot> decodeEvent(const GoogleHttpResponse& response,
                                                         const QString& expectedId) {
  if (response.body.size() > kMaximumResponseBytes) {
    return std::nullopt;
  }
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(response.body, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return std::nullopt;
  }
  const QJsonObject object = document.object();
  const std::optional<QString> id = requiredString(object, u"id", kMaximumIdentifierLength);
  const std::optional<QString> etag = requiredString(object, u"etag", kMaximumEtagLength);
  const std::optional<QString> summary = optionalString(object, u"summary", 500);
  const std::optional<QString> description = optionalString(object, u"description", 20'000);
  const std::optional<QString> location = optionalString(object, u"location", 1'000);
  const std::optional<QString> colorId = optionalString(object, u"colorId", 32);
  const std::optional<QString> transparency = optionalString(object, u"transparency", 32);
  const std::optional<QString> visibility = optionalString(object, u"visibility", 32);
  const std::optional<QJsonArray> attendees =
      eventAttendees(object.value(QStringLiteral("attendees")));
  const std::optional<QJsonObject> reminders =
      eventReminders(object.value(QStringLiteral("reminders")));
  const QJsonValue status = object.value(QStringLiteral("status"));
  if (!id.has_value() || *id != expectedId || !etag.has_value() || !summary.has_value() ||
      hasInvalidOptional(description, object, u"description") ||
      hasInvalidOptional(location, object, u"location") ||
      hasInvalidOptional(colorId, object, u"colorId") ||
      hasInvalidOptional(transparency, object, u"transparency") ||
      hasInvalidOptional(visibility, object, u"visibility") ||
      (transparency.has_value() && *transparency != QStringLiteral("opaque") &&
       *transparency != QStringLiteral("transparent")) ||
      (visibility.has_value() && *visibility != QStringLiteral("default") &&
       *visibility != QStringLiteral("public") && *visibility != QStringLiteral("private") &&
       *visibility != QStringLiteral("confidential")) ||
      !attendees.has_value() || !reminders.has_value() ||
      (!status.isUndefined() &&
       (!status.isString() || (status.toString() != QStringLiteral("confirmed") &&
                               status.toString() != QStringLiteral("tentative") &&
                               status.toString() != QStringLiteral("cancelled"))))) {
    return std::nullopt;
  }
  const bool deleted = status.isString() && status.toString() == QStringLiteral("cancelled");
  const std::optional<QJsonObject> start = eventTime(object.value(QStringLiteral("start")));
  const std::optional<QJsonObject> end = eventTime(object.value(QStringLiteral("end")));
  if (!deleted && (!start.has_value() || !end.has_value())) {
    return std::nullopt;
  }
  QJsonObject fields{{QStringLiteral("_deleted"), deleted},
                     {QStringLiteral("summary"), *summary},
                     {QStringLiteral("description"), description.has_value()
                                                           ? QJsonValue(*description)
                                                           : QJsonValue::Null},
                     {QStringLiteral("location"), location.has_value() ? QJsonValue(*location)
                                                                         : QJsonValue::Null},
                     {QStringLiteral("colorId"), colorId.has_value() ? QJsonValue(*colorId)
                                                                        : QJsonValue::Null},
                     {QStringLiteral("transparency"),
                      transparency.has_value() ? QJsonValue(*transparency) : QJsonValue::Null},
                     {QStringLiteral("visibility"),
                      visibility.has_value() ? QJsonValue(*visibility) : QJsonValue::Null},
                     {QStringLiteral("attendees"), *attendees},
                     {QStringLiteral("reminders"), *reminders}};
  if (start.has_value() && end.has_value()) {
    fields.insert(QStringLiteral("start"), *start);
    fields.insert(QStringLiteral("end"), *end);
  }
  const QJsonValue recurrence = object.value(QStringLiteral("recurrence"));
  if (isPresent(recurrence)) {
    if (!recurrence.isArray() || recurrence.toArray().size() > 128) {
      return std::nullopt;
    }
    for (const QJsonValue& rule : recurrence.toArray()) {
      if (!rule.isString() || rule.toString().isEmpty() || rule.toString().size() > 2'048 ||
          rule.toString().contains(QChar::Null)) {
        return std::nullopt;
      }
    }
    if (!recurrence.toArray().isEmpty()) {
      fields.insert(QStringLiteral("_recurrence"), true);
    }
  }
  return RemoteSnapshot{.fields = std::move(fields), .etag = etag};
}

[[nodiscard]] std::optional<QString> remoteIdFor(const PendingMutation& mutation) {
  QJsonValue remoteId = mutation.payload.value(QStringLiteral("remoteTaskId"));
  if (mutation.resource == PendingMutationResource::Event) {
    remoteId = mutation.payload.value(QStringLiteral("remoteEventId"));
  }
  if (mutation.resource == PendingMutationResource::TaskList) {
    remoteId = mutation.payload.value(QStringLiteral("remoteTaskListId"));
  }
  return remoteId.isString() && isValidText(remoteId.toString(), kMaximumIdentifierLength)
             ? std::optional<QString>(remoteId.toString())
             : std::nullopt;
}

[[nodiscard]] std::optional<QString> calendarIdFor(const PendingMutation& mutation) {
  const QString key = mutation.operation == QStringLiteral("event.move")
                          ? QStringLiteral("sourceCalendarId")
                          : QStringLiteral("calendarId");
  const QJsonValue calendarId = mutation.payload.value(key);
  return calendarId.isString() && isValidText(calendarId.toString(), kMaximumIdentifierLength)
             ? std::optional<QString>(calendarId.toString())
             : std::nullopt;
}

[[nodiscard]] RemoteSnapshotResult fetchRemote(const PendingMutation& mutation,
                                                GoogleHttpClient& httpClient,
                                                const QString& accessToken) {
  const std::optional<QString> remoteId = remoteIdFor(mutation);
  if (!remoteId.has_value()) {
    return validationError(QStringLiteral("Conflict mutation remote identity is invalid"));
  }
  GoogleHttpRequest request;
  if (mutation.resource == PendingMutationResource::Task) {
    const QJsonValue taskListId = mutation.payload.value(QStringLiteral("taskListId"));
    if (!taskListId.isString() || !isValidText(taskListId.toString(), kMaximumIdentifierLength)) {
      return validationError(QStringLiteral("Conflict task-list identity is invalid"));
    }
    request.path = QStringLiteral("/tasks/v1/lists/") + taskListId.toString() +
                   QStringLiteral("/tasks/") + *remoteId;
  } else if (mutation.resource == PendingMutationResource::TaskList) {
    request.path = QStringLiteral("/tasks/v1/users/@me/lists/") + *remoteId;
  } else {
    const std::optional<QString> calendarId = calendarIdFor(mutation);
    if (!calendarId.has_value()) {
      return validationError(QStringLiteral("Conflict calendar identity is invalid"));
    }
    request.path = QStringLiteral("/calendar/v3/calendars/") + *calendarId +
                   QStringLiteral("/events/") + *remoteId;
  }
  GoogleHttpResult response = httpClient.send(std::move(request), accessToken).get();
  if (std::holds_alternative<GoogleApiError>(response)) {
    const GoogleApiError& error = std::get<GoogleApiError>(response);
    if (error.kind() == GoogleApiErrorKind::NotFound) {
      return RemoteSnapshot{.fields = {{QStringLiteral("_deleted"), true}}, .etag = std::nullopt};
    }
    return error;
  }
  const GoogleHttpResponse& remote = std::get<GoogleHttpResponse>(response);
  std::optional<RemoteSnapshot> decoded;
  if (mutation.resource == PendingMutationResource::Task) {
    decoded = decodeTask(remote, *remoteId);
  } else if (mutation.resource == PendingMutationResource::TaskList) {
    decoded = decodeTaskList(remote, *remoteId);
  } else {
    decoded = decodeEvent(remote, *remoteId);
  }
  return decoded.has_value()
             ? RemoteSnapshotResult(std::move(*decoded))
             : RemoteSnapshotResult(validationError(QStringLiteral("Google conflict response is invalid")));
}

[[nodiscard]] SyncConflictResource conflictResource(PendingMutationResource resource) {
  switch (resource) {
  case PendingMutationResource::Task:
    return SyncConflictResource::Task;
  case PendingMutationResource::TaskList:
    return SyncConflictResource::TaskList;
  case PendingMutationResource::Event:
    return SyncConflictResource::Event;
  }
  return SyncConflictResource::Task;
}

[[nodiscard]] std::optional<AppError> markAwaitingUser(OptimisticMutationCoordinator& mutations,
                                                        const PendingMutation& mutation) {
  if (!mutation.leaseId.has_value()) {
    return databaseError(QStringLiteral("Conflict mutation lease is missing"));
  }
  PendingMutationResult failed =
      mutations
          .markFailed({.mutationId = mutation.id,
                       .leaseId = *mutation.leaseId,
                       .errorCode = QStringLiteral("conflict"),
                       .errorMessage = QStringLiteral("Google resource changed remotely"),
                       .nextRetryAt = std::nullopt})
          .get();
  return std::holds_alternative<AppError>(failed)
             ? std::optional<AppError>(std::get<AppError>(std::move(failed)))
             : std::nullopt;
}

[[nodiscard]] std::optional<SyncConflict> findConflict(SyncConflictStore& store,
                                                        const QString& conflictId) {
  SyncConflictListResult listed = store.listUnresolved(100).get();
  if (std::holds_alternative<AppError>(listed)) {
    return std::nullopt;
  }
  const QList<SyncConflict>& conflicts = std::get<QList<SyncConflict>>(listed);
  for (const SyncConflict& conflict : conflicts) {
    if (conflict.id == conflictId) {
      return conflict;
    }
  }
  return std::nullopt;
}

} // namespace

GoogleSyncConflictResolver::GoogleSyncConflictResolver(OptimisticMutationCoordinator& mutations,
                                                       SyncConflictStore& conflicts,
                                                       GoogleHttpClient& httpClient)
    : mutations_(mutations), conflicts_(conflicts), httpClient_(httpClient) {}

void GoogleSyncConflictResolver::setPolicy(SyncConflictPolicy policy) noexcept {
  policy_.store(policy, std::memory_order_relaxed);
}

SyncConflictPolicy GoogleSyncConflictResolver::policy() const noexcept {
  return policy_.load(std::memory_order_relaxed);
}

GoogleSyncConflictResult GoogleSyncConflictResolver::handle(PendingMutation mutation,
                                                            QString errorCode,
                                                            QString errorMessage,
                                                            QString accessToken) {
  if (!mutation.leaseId.has_value()) {
    return databaseError(QStringLiteral("Conflict mutation lease is missing"));
  }
  const RemoteSnapshotResult fetched = fetchRemote(mutation, httpClient_, accessToken);
  if (std::holds_alternative<GoogleApiError>(fetched)) {
    return std::get<GoogleApiError>(fetched);
  }
  if (std::holds_alternative<AppError>(fetched)) {
    return std::get<AppError>(fetched);
  }
  const RemoteSnapshot remote = std::get<RemoteSnapshot>(fetched);
  const SyncConflictPolicy selectedPolicy = policy();
  const SyncThreeWayMergeResult merged = SyncThreeWayMerge::merge(
      {.resource = conflictResource(mutation.resource),
       .operation = mutation.operation,
       .baseSnapshot = mutation.baseSnapshot,
       .localIntent = mutation.payload,
       .remoteSnapshot = remote.fields,
       .policy = selectedPolicy});
  SyncConflictResult recorded =
      conflicts_
          .record({.accountId = mutation.accountId,
                   .resource = conflictResource(mutation.resource),
                   .resourceId = mutation.resourceId,
                   .mutationId = mutation.id,
                   .errorCode = std::move(errorCode),
                   .errorMessage = std::move(errorMessage),
                   .baseSnapshot = mutation.baseSnapshot,
                   .localPayload = mutation.payload,
                   .remoteSnapshot = remote.fields,
                   .remoteEtag = remote.etag,
                   .policy = selectedPolicy})
          .get();
  if (std::holds_alternative<AppError>(recorded)) {
    return std::get<AppError>(std::move(recorded));
  }
  const SyncConflict conflict = std::get<SyncConflict>(std::move(recorded));
  if (merged.decision == SyncMergeDecision::KeepRemote) {
    PendingMutationResult cancelled = mutations_.cancel(mutation.id).get();
    if (std::holds_alternative<AppError>(cancelled)) {
      return std::get<AppError>(std::move(cancelled));
    }
    SyncConflictResult resolved =
        conflicts_.resolve(conflict.id, SyncConflictResolution::KeepRemote).get();
    return std::holds_alternative<AppError>(resolved)
               ? GoogleSyncConflictResult(std::get<AppError>(std::move(resolved)))
               : GoogleSyncConflictResult(GoogleSyncConflictOutcome::KeptRemote);
  }
  if (merged.decision == SyncMergeDecision::ReapplyLocal && remote.etag.has_value()) {
    PendingMutationResult rebased =
        mutations_
            .rebase({.mutationId = mutation.id,
                     .leaseId = mutation.leaseId,
                     .payload = merged.reapplyIntent,
                     .baseSnapshot = remote.fields,
                     .remoteEtag = remote.etag})
            .get();
    if (std::holds_alternative<AppError>(rebased)) {
      return std::get<AppError>(std::move(rebased));
    }
    SyncConflictResult resolved =
        conflicts_.resolve(conflict.id, SyncConflictResolution::KeepLocal).get();
    return std::holds_alternative<AppError>(resolved)
               ? GoogleSyncConflictResult(std::get<AppError>(std::move(resolved)))
               : GoogleSyncConflictResult(GoogleSyncConflictOutcome::ReappliedLocal);
  }
  if (const std::optional<AppError> error = markAwaitingUser(mutations_, mutation); error.has_value()) {
    return *error;
  }
  return GoogleSyncConflictOutcome::AwaitingUser;
}

std::future<std::optional<AppError>>
GoogleSyncConflictResolver::resolve(QString conflictId, SyncConflictResolution resolution) {
  return std::async(std::launch::async, [this, conflictId = std::move(conflictId), resolution] {
    const std::optional<SyncConflict> conflict = findConflict(conflicts_, conflictId);
    if (!conflict.has_value()) {
      return std::optional<AppError>(
          validationError(QStringLiteral("Sync conflict is unavailable for resolution")));
    }
    PendingMutationLookupResult found = mutations_.find(conflict->mutationId).get();
    if (std::holds_alternative<AppError>(found)) {
      return std::optional<AppError>(std::get<AppError>(std::move(found)));
    }
    const std::optional<PendingMutation>& mutation =
        std::get<std::optional<PendingMutation>>(found);
    if (!mutation.has_value() || mutation->status != PendingMutationStatus::Failed) {
      return std::optional<AppError>(
          validationError(QStringLiteral("Sync conflict mutation is unavailable")));
    }
    if (resolution == SyncConflictResolution::KeepRemote) {
      PendingMutationResult cancelled = mutations_.cancel(mutation->id).get();
      if (std::holds_alternative<AppError>(cancelled)) {
        return std::optional<AppError>(std::get<AppError>(std::move(cancelled)));
      }
    } else {
      if (conflict->remoteSnapshot.value(QStringLiteral("_deleted")).toBool() ||
          !conflict->remoteEtag.has_value()) {
        return std::optional<AppError>(
            validationError(QStringLiteral("Deleted Google resources cannot be reapplied")));
      }
      const SyncThreeWayMergeResult merged = SyncThreeWayMerge::merge(
          {.resource = conflict->resource,
           .operation = mutation->operation,
           .baseSnapshot = conflict->baseSnapshot,
           .localIntent = conflict->localPayload,
           .remoteSnapshot = conflict->remoteSnapshot,
           .policy = SyncConflictPolicy::PreferHcb});
      if (merged.decision != SyncMergeDecision::ReapplyLocal) {
        return std::optional<AppError>(
            validationError(QStringLiteral("Sync conflict cannot be reapplied")));
      }
      PendingMutationResult rebased =
          mutations_
              .rebase({.mutationId = mutation->id,
                       .payload = merged.reapplyIntent,
                       .baseSnapshot = conflict->remoteSnapshot,
                       .remoteEtag = conflict->remoteEtag})
              .get();
      if (std::holds_alternative<AppError>(rebased)) {
        return std::optional<AppError>(std::get<AppError>(std::move(rebased)));
      }
    }
    SyncConflictResult resolved = conflicts_.resolve(conflict->id, resolution).get();
    return std::holds_alternative<AppError>(resolved)
               ? std::optional<AppError>(std::get<AppError>(std::move(resolved)))
               : std::optional<AppError>{};
  });
}

} // namespace hcb
