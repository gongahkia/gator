#include "core/SyncThreeWayMerge.h"

#include <QJsonValue>

#include <algorithm>
#include <array>
#include <utility>

namespace hcb {
namespace {

[[nodiscard]] bool equalValues(const QJsonValue& left, const QJsonValue& right) {
  const bool leftAbsent = left.isUndefined() || left.isNull();
  const bool rightAbsent = right.isUndefined() || right.isNull();
  return leftAbsent || rightAbsent ? leftAbsent == rightAbsent : left == right;
}

[[nodiscard]] bool isStructural(const SyncThreeWayMergeInput& input) {
  if (input.remoteSnapshot.value(QStringLiteral("_deleted")).toBool() ||
      input.remoteSnapshot.value(QStringLiteral("_recurrence")).toBool()) {
    return true;
  }
  if (input.operation.endsWith(QStringLiteral(".delete")) ||
      input.operation.contains(QStringLiteral("move"), Qt::CaseInsensitive)) {
    return true;
  }
  if (input.resource == SyncConflictResource::Task) {
    return input.localIntent.contains(QStringLiteral("parentTaskId")) ||
           input.localIntent.contains(QStringLiteral("previousTaskId"));
  }
  if (input.resource == SyncConflictResource::Event) {
    const QJsonObject event = input.localIntent.value(QStringLiteral("event")).toObject();
    return input.localIntent.contains(QStringLiteral("recurrenceScope")) ||
           input.localIntent.contains(QStringLiteral("recurringEventId")) ||
           event.contains(QStringLiteral("recurrence"));
  }
  return true;
}

[[nodiscard]] const QJsonObject&
payloadFor(const SyncThreeWayMergeInput& input, QStringView key, QJsonObject& empty) {
  const QJsonValue payload = input.localIntent.value(key);
  if (payload.isObject()) {
    empty = payload.toObject();
    return empty;
  }
  return empty;
}

[[nodiscard]] QJsonObject
wrapIntent(const SyncThreeWayMergeInput& input, QStringView key, QJsonObject fields) {
  QJsonObject result = input.localIntent;
  result.insert(key.toString(), std::move(fields));
  return result;
}

[[nodiscard]] SyncThreeWayMergeResult structuralResult(const SyncThreeWayMergeInput& input) {
  switch (input.policy) {
  case SyncConflictPolicy::PreferGoogle:
    return {.decision = SyncMergeDecision::KeepRemote, .structural = true};
  case SyncConflictPolicy::PreferHcb:
    return {.decision = SyncMergeDecision::ReapplyLocal,
            .reapplyIntent = input.localIntent,
            .structural = true};
  case SyncConflictPolicy::AskEachTime:
    return {.decision = SyncMergeDecision::RequireUser, .structural = true};
  }
  return {.decision = SyncMergeDecision::RequireUser, .structural = true};
}

[[nodiscard]] SyncThreeWayMergeResult mergeFields(const SyncThreeWayMergeInput& input,
                                                  QStringView intentKey,
                                                  const QList<QStringView>& fields) {
  QJsonObject localFields;
  const QJsonObject& local = payloadFor(input, intentKey, localFields);
  QJsonObject reapply;
  QList<SyncFieldConflict> conflicts;
  for (const QStringView field : fields) {
    const QJsonValue localValue = local.value(field);
    if (localValue.isUndefined()) {
      continue;
    }
    const QJsonValue baseValue = input.baseSnapshot.value(field);
    const QJsonValue remoteValue = input.remoteSnapshot.value(field);
    if (equalValues(localValue, baseValue)) {
      continue;
    }
    if (!equalValues(remoteValue, baseValue) && !equalValues(localValue, remoteValue)) {
      conflicts.append({.field = field.toString(),
                        .base = baseValue,
                        .local = localValue,
                        .remote = remoteValue});
      continue;
    }
    if (!equalValues(localValue, remoteValue)) {
      reapply.insert(field.toString(), localValue);
    }
  }
  if (conflicts.isEmpty()) {
    return {.decision =
                reapply.isEmpty() ? SyncMergeDecision::KeepRemote : SyncMergeDecision::ReapplyLocal,
            .reapplyIntent =
                reapply.isEmpty() ? QJsonObject() : wrapIntent(input, intentKey, reapply)};
  }
  switch (input.policy) {
  case SyncConflictPolicy::PreferGoogle:
    return {.decision =
                reapply.isEmpty() ? SyncMergeDecision::KeepRemote : SyncMergeDecision::ReapplyLocal,
            .reapplyIntent =
                reapply.isEmpty() ? QJsonObject() : wrapIntent(input, intentKey, reapply),
            .conflicts = std::move(conflicts)};
  case SyncConflictPolicy::PreferHcb:
    for (const SyncFieldConflict& conflict : conflicts) {
      reapply.insert(conflict.field, conflict.local);
    }
    return {.decision = SyncMergeDecision::ReapplyLocal,
            .reapplyIntent = wrapIntent(input, intentKey, std::move(reapply)),
            .conflicts = std::move(conflicts)};
  case SyncConflictPolicy::AskEachTime:
    return {.decision = SyncMergeDecision::RequireUser, .conflicts = std::move(conflicts)};
  }
  return {.decision = SyncMergeDecision::RequireUser, .conflicts = std::move(conflicts)};
}

} // namespace

SyncThreeWayMergeResult SyncThreeWayMerge::merge(SyncThreeWayMergeInput input) {
  if (isStructural(input)) {
    return structuralResult(input);
  }
  if (input.resource == SyncConflictResource::Task) {
    return mergeFields(input, u"task", {u"title", u"notes", u"status", u"due"});
  }
  if (input.resource == SyncConflictResource::Event) {
    return mergeFields(
        input,
        u"event",
        {u"summary", u"description", u"location", u"start", u"end", u"colorId",
         u"transparency", u"visibility", u"attendees", u"reminders"});
  }
  return structuralResult(input);
}

} // namespace hcb
