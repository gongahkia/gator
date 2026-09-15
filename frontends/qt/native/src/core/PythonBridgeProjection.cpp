#include "core/PythonBridgeProjection.h"
#include "core/TaskRecurrenceMarker.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonValue>

#include <cstdint>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdLength = 256;
constexpr qsizetype kMaximumTitleLength = 500;
constexpr qsizetype kMaximumTextLength = 20'000;
constexpr qsizetype kMaximumTaskLists = 20'000;
constexpr qsizetype kMaximumCalendars = 20'000;
constexpr qsizetype kMaximumTasksPerPage = 500;
constexpr qsizetype kMaximumEventsPerRange = 25'000;
constexpr qsizetype kMaximumSearchResults = 200;

[[nodiscard]] AppError invalidPayload() {
  return AppError(AppErrorCode::Validation,
                  QStringLiteral("HCB bridge response payload is invalid"));
}

[[nodiscard]] bool isIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumIdLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isText(const QString& value, qsizetype maximumLength, bool required) {
  return (!required || !value.isEmpty()) && value.size() <= maximumLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] std::optional<QString>
optionalString(const QJsonObject& object, QStringView key, qsizetype maximumLength) {
  const QJsonValue value = object.value(key);
  if (value.isUndefined() || value.isNull()) {
    return std::optional<QString>{};
  }
  if (!value.isString() || !isText(value.toString(), maximumLength, false)) {
    return std::nullopt;
  }
  return value.toString();
}

[[nodiscard]] bool
isValidOptionalString(const QJsonObject& object, QStringView key, qsizetype maximumLength) {
  const QJsonValue value = object.value(key);
  return value.isUndefined() || value.isNull() ||
         (value.isString() && isText(value.toString(), maximumLength, false));
}

[[nodiscard]] std::optional<QString>
requiredString(const QJsonObject& object, QStringView key, qsizetype maximumLength) {
  const QJsonValue value = object.value(key);
  if (!value.isString() || !isText(value.toString(), maximumLength, true)) {
    return std::nullopt;
  }
  return value.toString();
}

[[nodiscard]] std::optional<QString> metadataString(const QJsonObject& object, QStringView key) {
  const QJsonValue metadata = object.value(QStringLiteral("metadata"));
  if (!metadata.isObject()) {
    return std::nullopt;
  }
  return optionalString(metadata.toObject(), key, kMaximumTextLength);
}

[[nodiscard]] std::optional<TaskPriority> taskPriority(const QJsonValue& value) {
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString priority = value.toString();
  if (priority == QStringLiteral("none")) {
    return TaskPriority::None;
  }
  if (priority == QStringLiteral("low")) {
    return TaskPriority::Low;
  }
  if (priority == QStringLiteral("medium")) {
    return TaskPriority::Medium;
  }
  if (priority == QStringLiteral("high")) {
    return TaskPriority::High;
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<TaskModelTask> bridgeTask(const QJsonObject& object,
                                                      const QHash<QString, QString>& taskListTitles,
                                                      int sortOrder) {
  const std::optional<QString> id = requiredString(object, u"id", kMaximumIdLength);
  const std::optional<QString> accountId = requiredString(object, u"account_id", kMaximumIdLength);
  const std::optional<QString> listId = requiredString(object, u"list_id", kMaximumIdLength);
  const std::optional<QString> title = requiredString(object, u"title", kMaximumTitleLength);
  const std::optional<TaskPriority> priority =
      taskPriority(object.value(QStringLiteral("priority")));
  const QJsonValue status = object.value(QStringLiteral("status"));
  const std::optional<QString> notes = optionalString(object, u"notes", kMaximumTextLength);
  const std::optional<QString> parentId = optionalString(object, u"parent_id", kMaximumIdLength);
  const std::optional<QString> due = optionalString(object, u"due", 32);
  const std::optional<QString> dueTimeZone = optionalString(object, u"due_time_zone", 120);
  if (!id.has_value() || !accountId.has_value() || !listId.has_value() || !title.has_value() ||
      !priority.has_value() || !status.isString() ||
      !isValidOptionalString(object, u"notes", kMaximumTextLength) ||
      !isValidOptionalString(object, u"parent_id", kMaximumIdLength) ||
      !isValidOptionalString(object, u"due", 32) ||
      !isValidOptionalString(object, u"due_time_zone", 120) || !isIdentifier(*id) ||
      !isIdentifier(*accountId) || !isIdentifier(*listId) || !taskListTitles.contains(*listId) ||
      (parentId.has_value() && !isIdentifier(*parentId)) ||
      (status.toString() != QStringLiteral("needsAction") &&
       status.toString() != QStringLiteral("completed"))) {
    return std::nullopt;
  }
  const TaskRecurrenceNotes recurrence = parseTaskRecurrenceNotes(notes.value_or(QString()));
  const auto recurrenceFrequency = [&recurrence]() {
    return recurrence.marker.has_value() ? static_cast<int>(recurrence.marker->frequency) : -1;
  };
  const auto recurrenceInterval = [&recurrence]() {
    return recurrence.marker.has_value() ? recurrence.marker->interval : 1;
  };
  const auto recurrenceEndKind = [&recurrence]() {
    return recurrence.marker.has_value() ? static_cast<int>(recurrence.marker->end.kind) : 0;
  };
  const auto recurrenceEndUntil = [&recurrence]() {
    return recurrence.marker.has_value() ? recurrence.marker->end.untilDate.value_or(QString())
                                         : QString();
  };
  const auto recurrenceEndCount = [&recurrence]() {
    return recurrence.marker.has_value() ? recurrence.marker->end.count.value_or(0) : 0;
  };
  return TaskModelTask{
      .id = *id,
      .taskListId = *listId,
      .taskListTitle = taskListTitles.value(*listId),
      .parentTaskId = parentId,
      .title = *title,
      .notes = notes.has_value() ? std::optional<QString>(recurrence.userNotes)
                                 : std::optional<QString>{},
      .due = due.has_value() ? std::optional<TaskDue>(TaskDue{.at = due, .timeZone = dueTimeZone})
                             : std::optional<TaskDue>{},
      .priority = *priority,
      .completed = status.toString() == QStringLiteral("completed"),
      .managedRecurrence =
          recurrence.state == TaskRecurrenceNotesState::Managed && recurrence.diagnostic.isEmpty(),
      .recurrenceSummary =
          recurrence.marker.has_value() ? taskRecurrenceSummary(*recurrence.marker) : QString(),
      .recurrenceSeriesId = recurrence.marker.has_value() ? recurrence.marker->seriesId : QString(),
      .recurrenceOccurrenceId =
          recurrence.marker.has_value() ? recurrence.marker->occurrenceId : QString(),
      .recurrenceFrequency = recurrenceFrequency(),
      .recurrenceInterval = recurrenceInterval(),
      .recurrenceEndKind = recurrenceEndKind(),
      .recurrenceEndUntil = recurrenceEndUntil(),
      .recurrenceEndCount = recurrenceEndCount(),
      .recurrenceRule =
          recurrence.marker.has_value() ? recurrence.marker->recurrenceRule : QString(),
      .recurrenceExclusionDates = recurrence.marker.has_value()
                                      ? recurrence.marker->exclusionDates.join(QStringLiteral(","))
                                      : QString(),
      .recurrenceAdditionDates = recurrence.marker.has_value()
                                     ? recurrence.marker->additionDates.join(QStringLiteral(","))
                                     : QString(),
      .recurrenceDiagnostic = recurrence.diagnostic,
      .sortOrder = static_cast<std::int64_t>(sortOrder)};
}

[[nodiscard]] std::optional<CalendarEventSummary> bridgeEvent(const QJsonObject& object) {
  const std::optional<QString> id = requiredString(object, u"id", kMaximumIdLength);
  const std::optional<QString> calendarId =
      requiredString(object, u"calendar_id", kMaximumIdLength);
  const std::optional<QString> title = requiredString(object, u"summary", kMaximumTitleLength);
  const std::optional<QString> status = requiredString(object, u"status", 32);
  const QJsonValue startValue = object.value(QStringLiteral("start"));
  const QJsonValue endValue = object.value(QStringLiteral("end"));
  if (!id.has_value() || !calendarId.has_value() || !title.has_value() || !status.has_value() ||
      !isIdentifier(*id) || !isIdentifier(*calendarId) || !startValue.isObject() ||
      !endValue.isObject()) {
    return std::nullopt;
  }
  const QJsonObject start = startValue.toObject();
  const QJsonObject end = endValue.toObject();
  const std::optional<QString> startKind = requiredString(start, u"kind", 16);
  const std::optional<QString> startAt = requiredString(start, u"value", 64);
  const std::optional<QString> endKind = requiredString(end, u"kind", 16);
  const std::optional<QString> endAt = requiredString(end, u"value", 64);
  const std::optional<QString> startZone = optionalString(start, u"time_zone", 120);
  const std::optional<QString> endZone = optionalString(end, u"time_zone", 120);
  const std::optional<QString> description =
      optionalString(object, u"description", kMaximumTextLength);
  const std::optional<QString> location = optionalString(object, u"location", kMaximumTextLength);
  const std::optional<QString> colorId = optionalString(object, u"color_id", 32);
  const std::optional<QString> transparency = optionalString(object, u"transparency", 32);
  const std::optional<QString> visibility = optionalString(object, u"visibility", 32);
  const std::optional<QString> eventType = optionalString(object, u"event_type", 64);
  const std::optional<QString> remoteId = optionalString(object, u"remote_id", kMaximumIdLength);
  const std::optional<QString> etag = metadataString(object, u"etag");
  const std::optional<QString> updatedAt = metadataString(object, u"local_updated_at");
  const QJsonValue attendeesValue = object.value(QStringLiteral("attendees"));
  const QJsonValue remindersValue = object.value(QStringLiteral("reminder_overrides"));
  const QJsonValue attachmentsValue = object.value(QStringLiteral("attachments"));
  if (!startKind.has_value() || !startAt.has_value() || !endKind.has_value() ||
      !endAt.has_value() || !isValidOptionalString(start, u"time_zone", 120) ||
      !isValidOptionalString(end, u"time_zone", 120) ||
      !isValidOptionalString(object, u"description", kMaximumTextLength) ||
      !isValidOptionalString(object, u"location", kMaximumTextLength) ||
      !isValidOptionalString(object, u"color_id", 32) ||
      !isValidOptionalString(object, u"transparency", 32) ||
      !isValidOptionalString(object, u"visibility", 32) ||
      !isValidOptionalString(object, u"event_type", 64) ||
      !isValidOptionalString(object, u"remote_id", kMaximumIdLength) ||
      (!etag.has_value() && !object.value(QStringLiteral("metadata"))
                                 .toObject()
                                 .value(QStringLiteral("etag"))
                                 .isNull()) ||
      !updatedAt.has_value() || !attendeesValue.isArray() || !remindersValue.isArray() ||
      !attachmentsValue.isArray() ||
      (*startKind != QStringLiteral("date") && *startKind != QStringLiteral("dateTime")) ||
      *startKind != *endKind) {
    return std::nullopt;
  }
  const QJsonArray attendees = attendeesValue.toArray();
  const QJsonArray reminders = remindersValue.toArray();
  const QJsonArray attachments = attachmentsValue.toArray();
  return CalendarEventSummary{
      .id = *id,
      .calendarId = *calendarId,
      .remoteId = remoteId,
      .status = *status,
      .title = *title,
      .description = description,
      .location = location,
      .startAt = *startAt,
      .startTimeZone = startZone,
      .endAt = *endAt,
      .endTimeZone = endZone,
      .allDay = *startKind == QStringLiteral("date"),
      .colorId = colorId,
      .transparency = transparency,
      .visibility = visibility,
      .eventType = eventType,
      .attendeeDetailsJson =
          QString::fromUtf8(QJsonDocument(attendees).toJson(QJsonDocument::Compact)),
      .remindersJson = QString::fromUtf8(QJsonDocument(reminders).toJson(QJsonDocument::Compact)),
      .attachmentsJson =
          QString::fromUtf8(QJsonDocument(attachments).toJson(QJsonDocument::Compact)),
      .etag = etag,
      .updatedAt = *updatedAt};
}

[[nodiscard]] bool isKnownSearchKind(const QString& kind) {
  return kind == QStringLiteral("task") || kind == QStringLiteral("task-list") ||
         kind == QStringLiteral("calendar") || kind == QStringLiteral("event") ||
         kind == QStringLiteral("drive") || kind == QStringLiteral("saved-search") ||
         kind == QStringLiteral("conflict");
}

[[nodiscard]] std::optional<LocalSearchResource> searchResource(const QString& kind) {
  if (kind == QStringLiteral("task")) {
    return LocalSearchResource::Task;
  }
  if (kind == QStringLiteral("task-list")) {
    return LocalSearchResource::TaskList;
  }
  if (kind == QStringLiteral("calendar")) {
    return LocalSearchResource::Calendar;
  }
  if (kind == QStringLiteral("event")) {
    return LocalSearchResource::Event;
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<QString> searchTitle(const QString& kind, const QJsonObject& item) {
  if (kind == QStringLiteral("task") || kind == QStringLiteral("task-list")) {
    return requiredString(item, u"title", kMaximumTitleLength);
  }
  return requiredString(item, u"summary", kMaximumTitleLength);
}

[[nodiscard]] std::optional<QString> searchDetail(const QString& kind, const QJsonObject& item) {
  if (kind == QStringLiteral("task-list")) {
    return QString();
  }
  const QStringView primary = kind == QStringLiteral("task")       ? u"notes"
                              : kind == QStringLiteral("calendar") ? u"description"
                                                                   : u"location";
  const std::optional<QString> primaryValue = optionalString(item, primary, kMaximumTextLength);
  if (!isValidOptionalString(item, primary, kMaximumTextLength)) {
    return std::nullopt;
  }
  if (primaryValue.has_value() || kind != QStringLiteral("event")) {
    return primaryValue.value_or(QString());
  }
  const std::optional<QString> description =
      optionalString(item, u"description", kMaximumTextLength);
  if (!isValidOptionalString(item, u"description", kMaximumTextLength)) {
    return std::nullopt;
  }
  return description.value_or(QString());
}

[[nodiscard]] std::optional<QString> searchScheduledAt(const QString& kind,
                                                       const QJsonObject& item) {
  if (kind == QStringLiteral("task")) {
    const std::optional<QString> due = optionalString(item, u"due", 64);
    if (!isValidOptionalString(item, u"due", 64)) {
      return std::nullopt;
    }
    return due.value_or(QString());
  }
  if (kind != QStringLiteral("event")) {
    return QString();
  }
  const QJsonValue startValue = item.value(QStringLiteral("start"));
  if (!startValue.isObject()) {
    return std::nullopt;
  }
  return requiredString(startValue.toObject(), u"value", 64);
}

} // namespace

PythonBridgeWorkspaceSummaryOrError
PythonBridgeProjection::workspaceSummary(const QJsonObject& data) {
  const QJsonValue workspaceValue = data.value(QStringLiteral("workspace"));
  if (!workspaceValue.isObject()) {
    return invalidPayload();
  }
  const QJsonObject workspace = workspaceValue.toObject();
  const QJsonObject account = workspace.value(QStringLiteral("account")).toObject();
  const std::optional<QString> accountId = requiredString(account, u"id", kMaximumIdLength);
  const std::optional<QString> accountEmail = requiredString(account, u"email", 320);
  const QJsonValue pending = workspace.value(QStringLiteral("pending"));
  const QJsonValue taskListsValue = workspace.value(QStringLiteral("task_lists"));
  const QJsonValue calendarsValue = workspace.value(QStringLiteral("calendars"));
  if (!accountId.has_value() || !accountEmail.has_value() || !isIdentifier(*accountId) ||
      !accountEmail->contains(u'@') || !pending.isDouble() || pending.toInt(-1) < 0 ||
      !taskListsValue.isArray() || !calendarsValue.isArray() ||
      taskListsValue.toArray().size() > kMaximumTaskLists ||
      calendarsValue.toArray().size() > kMaximumCalendars) {
    return invalidPayload();
  }
  QList<TaskListSummary> taskLists;
  for (const QJsonValue& value : taskListsValue.toArray()) {
    if (!value.isObject()) {
      return invalidPayload();
    }
    const QJsonObject object = value.toObject();
    const std::optional<QString> id = requiredString(object, u"id", kMaximumIdLength);
    const std::optional<QString> owner = requiredString(object, u"account_id", kMaximumIdLength);
    const std::optional<QString> title = requiredString(object, u"title", kMaximumTitleLength);
    const std::optional<QString> remoteId = optionalString(object, u"remote_id", kMaximumIdLength);
    const std::optional<QString> etag = metadataString(object, u"etag");
    const std::optional<QString> updatedAt = metadataString(object, u"local_updated_at");
    const QJsonValue position = object.value(QStringLiteral("position"));
    const QJsonValue selected = object.value(QStringLiteral("selected"));
    if (!id.has_value() || !owner.has_value() || !title.has_value() ||
        !isValidOptionalString(object, u"remote_id", kMaximumIdLength) ||
        (!etag.has_value() && !object.value(QStringLiteral("metadata"))
                                   .toObject()
                                   .value(QStringLiteral("etag"))
                                   .isNull()) ||
        !updatedAt.has_value() || !position.isDouble() || !isIdentifier(*id) ||
        (!selected.isUndefined() && !selected.isBool()) || *owner != *accountId) {
      return invalidPayload();
    }
    taskLists.append({.id = *id,
                      .accountId = *owner,
                      .remoteId = remoteId.value_or(QString()),
                      .title = *title,
                      .etag = etag,
                      .sortOrder = position.toInteger(),
                      .selected = selected.isUndefined() ? true : selected.toBool(),
                      .updatedAt = *updatedAt});
  }
  QList<CalendarSummary> calendars;
  for (const QJsonValue& value : calendarsValue.toArray()) {
    if (!value.isObject()) {
      return invalidPayload();
    }
    const QJsonObject object = value.toObject();
    const std::optional<QString> id = requiredString(object, u"id", kMaximumIdLength);
    const std::optional<QString> owner = requiredString(object, u"account_id", kMaximumIdLength);
    const std::optional<QString> title = requiredString(object, u"summary", kMaximumTitleLength);
    const std::optional<QString> remoteId = optionalString(object, u"remote_id", kMaximumIdLength);
    const std::optional<QString> description =
        optionalString(object, u"description", kMaximumTextLength);
    const std::optional<QString> timeZone = optionalString(object, u"time_zone", 120);
    const std::optional<QString> color = optionalString(object, u"color", 32);
    const std::optional<QString> etag = metadataString(object, u"etag");
    const std::optional<QString> updatedAt = metadataString(object, u"local_updated_at");
    const QJsonValue selected = object.value(QStringLiteral("selected"));
    const QJsonValue hidden = object.value(QStringLiteral("hidden"));
    if (!id.has_value() || !owner.has_value() || !title.has_value() ||
        !isValidOptionalString(object, u"remote_id", kMaximumIdLength) ||
        !isValidOptionalString(object, u"description", kMaximumTextLength) ||
        !isValidOptionalString(object, u"time_zone", 120) ||
        !isValidOptionalString(object, u"color", 32) ||
        (!etag.has_value() && !object.value(QStringLiteral("metadata"))
                                   .toObject()
                                   .value(QStringLiteral("etag"))
                                   .isNull()) ||
        !updatedAt.has_value() || !selected.isBool() || !hidden.isBool() || !isIdentifier(*id) ||
        *owner != *accountId) {
      return invalidPayload();
    }
    calendars.append({.id = *id,
                      .accountId = *owner,
                      .remoteId = remoteId.value_or(QString()),
                      .title = *title,
                      .description = description,
                      .timeZone = timeZone,
                      .backgroundColor = color,
                      .selected = selected.toBool(),
                      .hidden = hidden.toBool(),
                      .updatedAt = *updatedAt});
  }
  return PythonBridgeWorkspaceSummary{
      *accountId, *accountEmail, std::move(taskLists), std::move(calendars), pending.toInt()};
}

PythonBridgeTaskPageOrError
PythonBridgeProjection::taskPage(const QJsonObject& data,
                                 const QHash<QString, QString>& taskListTitles) {
  const QJsonObject page = data.value(QStringLiteral("page")).toObject();
  const QJsonValue tasksValue = page.value(QStringLiteral("tasks"));
  const QJsonValue cursorValue = page.value(QStringLiteral("next_cursor"));
  if (page.isEmpty() || !tasksValue.isArray() ||
      tasksValue.toArray().size() > kMaximumTasksPerPage ||
      (!cursorValue.isNull() && !cursorValue.isString())) {
    return invalidPayload();
  }
  QList<TaskModelTask> tasks;
  int sortOrder = 0;
  for (const QJsonValue& value : tasksValue.toArray()) {
    if (!value.isObject()) {
      return invalidPayload();
    }
    const std::optional<TaskModelTask> task =
        bridgeTask(value.toObject(), taskListTitles, sortOrder++);
    if (!task.has_value()) {
      return invalidPayload();
    }
    tasks.append(*task);
  }
  return PythonBridgeTaskPage{std::move(tasks),
                              cursorValue.isString()
                                  ? std::optional<QString>(cursorValue.toString())
                                  : std::optional<QString>{}};
}

PythonBridgeEventRangeOrError PythonBridgeProjection::eventRange(const QJsonObject& data) {
  const QJsonObject workspace = data.value(QStringLiteral("workspace")).toObject();
  const QJsonValue eventsValue = workspace.value(QStringLiteral("events"));
  if (workspace.isEmpty() || !eventsValue.isArray() ||
      eventsValue.toArray().size() > kMaximumEventsPerRange) {
    return invalidPayload();
  }
  QList<CalendarEventSummary> events;
  for (const QJsonValue& value : eventsValue.toArray()) {
    if (!value.isObject()) {
      return invalidPayload();
    }
    const std::optional<CalendarEventSummary> event = bridgeEvent(value.toObject());
    if (!event.has_value()) {
      return invalidPayload();
    }
    events.append(*event);
  }
  return events;
}

PythonBridgeSearchOrError PythonBridgeProjection::searchResults(const QJsonObject& data,
                                                                const QString& accountId) {
  if (!isIdentifier(accountId)) {
    return invalidPayload();
  }
  const QJsonValue resultsValue = data.value(QStringLiteral("results"));
  if (!resultsValue.isArray() || resultsValue.toArray().size() > kMaximumSearchResults) {
    return invalidPayload();
  }
  QList<LocalSearchRankedResult> results;
  for (const QJsonValue& value : resultsValue.toArray()) {
    if (!value.isObject()) {
      return invalidPayload();
    }
    const QJsonObject result = value.toObject();
    const std::optional<QString> kind = requiredString(result, u"kind", 32);
    const QJsonValue scoreValue = result.value(QStringLiteral("score"));
    const QJsonValue itemValue = result.value(QStringLiteral("item"));
    if (!kind.has_value() || !isKnownSearchKind(*kind) || !scoreValue.isDouble() ||
        !itemValue.isObject()) {
      return invalidPayload();
    }
    const double scoreNumber = scoreValue.toDouble();
    const int score = scoreValue.toInt(-1);
    if (score < 0 || score > 100 || scoreNumber != static_cast<double>(score)) {
      return invalidPayload();
    }
    const std::optional<LocalSearchResource> resource = searchResource(*kind);
    if (!resource.has_value()) {
      continue;
    }
    const QJsonObject item = itemValue.toObject();
    const std::optional<QString> id = requiredString(item, u"id", kMaximumIdLength);
    const std::optional<QString> owner = requiredString(item, u"account_id", kMaximumIdLength);
    const std::optional<QString> title = searchTitle(*kind, item);
    const std::optional<QString> detail = searchDetail(*kind, item);
    const std::optional<QString> scheduledAt = searchScheduledAt(*kind, item);
    if (!id.has_value() || !owner.has_value() || !title.has_value() || !detail.has_value() ||
        !scheduledAt.has_value() || !isIdentifier(*id) || *owner != accountId) {
      return invalidPayload();
    }
    results.append({.resource = *resource,
                    .id = *id,
                    .title = *title,
                    .detail = *detail,
                    .scheduledAt = *scheduledAt,
                    .score = score});
  }
  return results;
}

QHash<QString, QString>
PythonBridgeProjection::taskListTitles(const PythonBridgeWorkspaceSummary& summary) {
  QHash<QString, QString> titles;
  titles.reserve(summary.taskLists.size());
  for (const TaskListSummary& taskList : summary.taskLists) {
    titles.insert(taskList.id, taskList.title);
  }
  return titles;
}

} // namespace hcb
