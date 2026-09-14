#include "core/PythonBridgeProjection.h"

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

[[nodiscard]] std::optional<QString> requiredString(const QJsonObject& object,
                                                     QStringView key,
                                                     qsizetype maximumLength) {
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

[[nodiscard]] std::optional<TaskModelTask>
bridgeTask(const QJsonObject& object, const QHash<QString, QString>& taskListTitles, int sortOrder) {
  const std::optional<QString> id = requiredString(object, u"id", kMaximumIdLength);
  const std::optional<QString> accountId = requiredString(object, u"account_id", kMaximumIdLength);
  const std::optional<QString> listId = requiredString(object, u"list_id", kMaximumIdLength);
  const std::optional<QString> title = requiredString(object, u"title", kMaximumTitleLength);
  const std::optional<TaskPriority> priority = taskPriority(object.value(QStringLiteral("priority")));
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
      !isValidOptionalString(object, u"due_time_zone", 120) ||
      !isIdentifier(*id) || !isIdentifier(*accountId) || !isIdentifier(*listId) ||
      !taskListTitles.contains(*listId) ||
      (parentId.has_value() && !isIdentifier(*parentId)) ||
      (status.toString() != QStringLiteral("needsAction") && status.toString() != QStringLiteral("completed"))) {
    return std::nullopt;
  }
  return TaskModelTask{.id = *id,
                       .taskListId = *listId,
                       .taskListTitle = taskListTitles.value(*listId),
                       .parentTaskId = parentId,
                       .title = *title,
                       .notes = notes,
                       .due = due.has_value() ? std::optional<TaskDue>(
                                                  TaskDue{.at = due, .timeZone = dueTimeZone})
                                            : std::optional<TaskDue>{},
                       .priority = *priority,
                       .completed = status.toString() == QStringLiteral("completed"),
                       .sortOrder = static_cast<std::int64_t>(sortOrder)};
}

[[nodiscard]] std::optional<CalendarEventSummary> bridgeEvent(const QJsonObject& object) {
  const std::optional<QString> id = requiredString(object, u"id", kMaximumIdLength);
  const std::optional<QString> calendarId = requiredString(object, u"calendar_id", kMaximumIdLength);
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
  const std::optional<QString> description = optionalString(object, u"description", kMaximumTextLength);
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
  if (!startKind.has_value() || !startAt.has_value() || !endKind.has_value() || !endAt.has_value() ||
      !isValidOptionalString(start, u"time_zone", 120) ||
      !isValidOptionalString(end, u"time_zone", 120) ||
      !isValidOptionalString(object, u"description", kMaximumTextLength) ||
      !isValidOptionalString(object, u"location", kMaximumTextLength) ||
      !isValidOptionalString(object, u"color_id", 32) ||
      !isValidOptionalString(object, u"transparency", 32) ||
      !isValidOptionalString(object, u"visibility", 32) ||
      !isValidOptionalString(object, u"event_type", 64) ||
      !isValidOptionalString(object, u"remote_id", kMaximumIdLength) ||
      (!etag.has_value() && !object.value(QStringLiteral("metadata")).toObject()
                                .value(QStringLiteral("etag")).isNull()) ||
      !updatedAt.has_value() ||
      !attendeesValue.isArray() || !remindersValue.isArray() || !attachmentsValue.isArray() ||
      (*startKind != QStringLiteral("date") && *startKind != QStringLiteral("dateTime")) ||
      *startKind != *endKind) {
    return std::nullopt;
  }
  const QJsonArray attendees = attendeesValue.toArray();
  const QJsonArray reminders = remindersValue.toArray();
  const QJsonArray attachments = attachmentsValue.toArray();
  return CalendarEventSummary{.id = *id,
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
                              .attendeeDetailsJson = QString::fromUtf8(
                                  QJsonDocument(attendees).toJson(QJsonDocument::Compact)),
                              .remindersJson = QString::fromUtf8(
                                  QJsonDocument(reminders).toJson(QJsonDocument::Compact)),
                              .attachmentsJson = QString::fromUtf8(
                                  QJsonDocument(attachments).toJson(QJsonDocument::Compact)),
                              .etag = etag,
                              .updatedAt = *updatedAt};
}

} // namespace

PythonBridgeWorkspaceSummaryOrError PythonBridgeProjection::workspaceSummary(const QJsonObject& data) {
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
      !accountEmail->contains(u'@') || !pending.isDouble() ||
      pending.toInt(-1) < 0 || !taskListsValue.isArray() || !calendarsValue.isArray() ||
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
    if (!id.has_value() || !owner.has_value() || !title.has_value() ||
        !isValidOptionalString(object, u"remote_id", kMaximumIdLength) ||
        (!etag.has_value() && !object.value(QStringLiteral("metadata")).toObject()
                              .value(QStringLiteral("etag")).isNull()) ||
        !updatedAt.has_value() || !position.isDouble() || !isIdentifier(*id) || *owner != *accountId) {
      return invalidPayload();
    }
    taskLists.append({.id = *id,
                      .accountId = *owner,
                      .remoteId = remoteId.value_or(QString()),
                      .title = *title,
                      .etag = etag,
                      .sortOrder = position.toInteger(),
                      .selected = true,
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
    const std::optional<QString> description = optionalString(object, u"description", kMaximumTextLength);
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
        (!etag.has_value() && !object.value(QStringLiteral("metadata")).toObject()
                              .value(QStringLiteral("etag")).isNull()) ||
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
  return PythonBridgeWorkspaceSummary{*accountId,
                                      *accountEmail,
                                      std::move(taskLists),
                                      std::move(calendars),
                                      pending.toInt()};
}

PythonBridgeTaskPageOrError PythonBridgeProjection::taskPage(
    const QJsonObject& data, const QHash<QString, QString>& taskListTitles) {
  const QJsonObject page = data.value(QStringLiteral("page")).toObject();
  const QJsonValue tasksValue = page.value(QStringLiteral("tasks"));
  const QJsonValue cursorValue = page.value(QStringLiteral("next_cursor"));
  if (page.isEmpty() || !tasksValue.isArray() || tasksValue.toArray().size() > kMaximumTasksPerPage ||
      (!cursorValue.isNull() && !cursorValue.isString())) {
    return invalidPayload();
  }
  QList<TaskModelTask> tasks;
  int sortOrder = 0;
  for (const QJsonValue& value : tasksValue.toArray()) {
    if (!value.isObject()) {
      return invalidPayload();
    }
    const std::optional<TaskModelTask> task = bridgeTask(value.toObject(), taskListTitles, sortOrder++);
    if (!task.has_value()) {
      return invalidPayload();
    }
    tasks.append(*task);
  }
  return PythonBridgeTaskPage{std::move(tasks),
                              cursorValue.isString() ? std::optional<QString>(cursorValue.toString())
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
