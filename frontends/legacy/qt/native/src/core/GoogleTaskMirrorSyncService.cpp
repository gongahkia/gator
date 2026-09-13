#include "core/GoogleTaskMirrorSyncService.h"

#include "core/GoogleMirrorStore.h"
#include "core/GoogleTaskListPullClient.h"
#include "core/GoogleTaskPullClient.h"
#include "core/SyncCheckpointStore.h"
#include "core/TaskMutationService.h"

#include <QDateTime>
#include <QLocale>
#include <QTimeZone>

#include <algorithm>
#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr auto kTaskFullReconciliationInterval = std::chrono::hours(24);
constexpr auto kTaskWatermarkOverlap = std::chrono::minutes(2);
constexpr int kMaximumPullRetries = 2;

[[nodiscard]] SyncBackoffPolicy mirrorBackoffPolicy() {
  return SyncBackoffPolicy({.baseDelayMilliseconds = 1'000,
                            .maximumDelayMilliseconds = 30'000,
                            .jitterMilliseconds = 500,
                            .maximumAttempts = kMaximumPullRetries});
}

template <typename Result, typename Pull>
[[nodiscard]] Result pullWithRetry(Pull&& pull,
                                   const SyncBackoffPolicy& backoffPolicy,
                                   const CancellationToken& cancellation) {
  for (int attempt = 0;; ++attempt) {
    Result result = pull();
    if (!std::holds_alternative<GoogleApiError>(result) || attempt >= kMaximumPullRetries) {
      return result;
    }
    const std::optional<qint64> delay =
        backoffPolicy.retryDelayMilliseconds(std::get<GoogleApiError>(result), attempt);
    if (!delay.has_value()) {
      return result;
    }
    qint64 remaining = *delay;
    while (remaining > 0 && !cancellation.stop_requested()) {
      const qint64 slice = std::min<qint64>(remaining, 100);
      std::this_thread::sleep_for(std::chrono::milliseconds(slice));
      remaining -= slice;
    }
    if (cancellation.stop_requested()) {
      return result;
    }
  }
}

[[nodiscard]] QJsonObject taskRequestMetadata() {
  return {{QStringLiteral("fields"),
           QStringLiteral("nextPageToken,items(id,title,notes,status,due,completed,deleted,"
                          "hidden,parent,position,etag,updated,assignmentInfo)")},
          {QStringLiteral("maxResults"), QStringLiteral("100")},
          {QStringLiteral("showAssigned"), true},
          {QStringLiteral("showCompleted"), true},
          {QStringLiteral("showDeleted"), true},
          {QStringLiteral("showHidden"), true}};
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] AppError cancelledError() {
  return AppError(AppErrorCode::Cancelled, QStringLiteral("Google task sync was cancelled"));
}

[[nodiscard]] bool validIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= 256 &&
         !value.contains(QChar::Null);
}

[[nodiscard]] QDateTime clockDateTime(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC);
}

[[nodiscard]] std::optional<QDateTime> parseTimestamp(const QString& value) {
  QDateTime parsed = QDateTime::fromString(value, Qt::ISODateWithMs);
  if (!parsed.isValid()) {
    parsed = QDateTime::fromString(value, Qt::RFC2822Date);
  }
  if (!parsed.isValid()) {
    parsed = QLocale::c().toDateTime(value, QStringLiteral("ddd, dd MMM yyyy HH:mm:ss 'GMT'"));
    parsed.setTimeZone(QTimeZone::UTC);
  }
  return parsed.isValid() ? std::optional<QDateTime>(parsed.toUTC()) : std::nullopt;
}

[[nodiscard]] bool requiresFullReconciliation(const std::optional<SyncCheckpoint>& checkpoint,
                                               const Clock& clock) {
  if (!checkpoint.has_value()) {
    return true;
  }
  const std::optional<QDateTime> previous = parseTimestamp(checkpoint->lastSuccessfulSyncAt);
  return !previous.has_value() || previous->msecsTo(clockDateTime(clock)) >
                                     std::chrono::duration_cast<std::chrono::milliseconds>(
                                         kTaskFullReconciliationInterval)
                                         .count();
}

[[nodiscard]] QString nextWatermark(const std::optional<QString>& serverDate,
                                    const std::optional<SyncCheckpoint>& previous,
                                    const Clock& clock) {
  const std::optional<QDateTime> parsed =
      serverDate.has_value() ? parseTimestamp(*serverDate) : std::optional<QDateTime>{};
  const QDateTime candidate =
      parsed.value_or(clockDateTime(clock))
          .addMSecs(-std::chrono::duration_cast<std::chrono::milliseconds>(kTaskWatermarkOverlap)
                        .count())
          .toUTC();
  const std::optional<QDateTime> prior =
      previous.has_value() ? parseTimestamp(previous->syncToken) : std::optional<QDateTime>{};
  return prior.has_value() && *prior > candidate ? prior->toString(Qt::ISODateWithMs)
                                                   : candidate.toString(Qt::ISODateWithMs);
}

} // namespace

GoogleTaskMirrorSyncService::GoogleTaskMirrorSyncService(
    GoogleTaskListPullClient& taskListClient,
    GoogleTaskPullClient& taskClient,
    GoogleMirrorStore& mirrorStore,
    SyncCheckpointStore& checkpointStore,
    const Clock& clock,
    TaskMutationService* taskMutationService)
    : GoogleTaskMirrorSyncService(
          taskListClient,
          taskClient,
          mirrorStore,
          checkpointStore,
          clock,
          mirrorBackoffPolicy(),
          taskMutationService) {}

GoogleTaskMirrorSyncService::GoogleTaskMirrorSyncService(
    GoogleTaskListPullClient& taskListClient,
    GoogleTaskPullClient& taskClient,
    GoogleMirrorStore& mirrorStore,
    SyncCheckpointStore& checkpointStore,
    const Clock& clock,
    SyncBackoffPolicy backoffPolicy,
    TaskMutationService* taskMutationService)
    : taskListClient_(taskListClient), taskClient_(taskClient), mirrorStore_(mirrorStore),
      checkpointStore_(checkpointStore), clock_(clock), backoffPolicy_(std::move(backoffPolicy)),
      taskMutationService_(taskMutationService) {}

std::future<GoogleTaskMirrorSyncResultOrError>
GoogleTaskMirrorSyncService::sync(QString accountId,
                                  QString accessToken,
                                  CancellationToken cancellation) {
  return std::async(std::launch::async,
                    [this,
                     accountId = std::move(accountId),
                     accessToken = std::move(accessToken),
                     cancellation] {
    if (!validIdentifier(accountId)) {
      return GoogleTaskMirrorSyncResultOrError(
          validationError(QStringLiteral("Google task sync account is invalid")));
    }
    if (cancellation.stop_requested()) {
      return GoogleTaskMirrorSyncResultOrError(cancelledError());
    }
    GoogleTaskListPullResultOrError pulledLists = pullWithRetry<GoogleTaskListPullResultOrError>(
        [&] { return taskListClient_.list(accessToken).get(); }, backoffPolicy_, cancellation);
    if (cancellation.stop_requested()) {
      return GoogleTaskMirrorSyncResultOrError(cancelledError());
    }
    if (std::holds_alternative<GoogleApiError>(pulledLists)) {
      return GoogleTaskMirrorSyncResultOrError(std::get<GoogleApiError>(std::move(pulledLists)));
    }
    GoogleTaskListPullResult taskLists = std::get<GoogleTaskListPullResult>(std::move(pulledLists));
    GoogleMirrorWriteResult listWrite =
        mirrorStore_.mergeTaskLists(accountId, taskLists.taskLists, true).get();
    if (std::holds_alternative<AppError>(listWrite)) {
      return GoogleTaskMirrorSyncResultOrError(std::get<AppError>(std::move(listWrite)));
    }
    const QJsonObject requestMetadata = taskRequestMetadata();
    GoogleTaskMirrorSyncResult result{
        .taskListCount = static_cast<std::int64_t>(taskLists.taskLists.size())};
    for (const GoogleTaskListMirror& taskList : taskLists.taskLists) {
      if (cancellation.stop_requested()) {
        return GoogleTaskMirrorSyncResultOrError(cancelledError());
      }
      const SyncCheckpointKey checkpointKey{.accountId = accountId,
                                            .resourceType =
                                                SyncCheckpointResourceType::TaskListWatermark,
                                            .resourceId = taskList.id};
      SyncCheckpointLookupResult storedCheckpoint = checkpointStore_.find(checkpointKey).get();
      if (std::holds_alternative<AppError>(storedCheckpoint)) {
        return GoogleTaskMirrorSyncResultOrError(
            std::get<AppError>(std::move(storedCheckpoint)));
      }
      const std::optional<SyncCheckpoint> checkpoint =
          std::get<std::optional<SyncCheckpoint>>(std::move(storedCheckpoint));
      const bool fullReconciliation = requiresFullReconciliation(checkpoint, clock_) ||
                                      checkpoint->metadata != requestMetadata;
      GoogleTaskPullResultOrError pulledTasks = pullWithRetry<GoogleTaskPullResultOrError>(
          [&] {
            return taskClient_
                .list({.taskListId = taskList.id,
                       .updatedMin = fullReconciliation
                                         ? std::optional<QString>{}
                                         : std::optional<QString>(checkpoint->syncToken),
                       .showCompleted = true},
                      accessToken)
                .get();
          },
          backoffPolicy_,
          cancellation);
      if (cancellation.stop_requested()) {
        return GoogleTaskMirrorSyncResultOrError(cancelledError());
      }
      if (std::holds_alternative<GoogleApiError>(pulledTasks)) {
        return GoogleTaskMirrorSyncResultOrError(
            std::get<GoogleApiError>(std::move(pulledTasks)));
      }
      GoogleTaskPullResult taskPage = std::get<GoogleTaskPullResult>(std::move(pulledTasks));
      GoogleMirrorWriteResult taskWrite =
          mirrorStore_
              .mergeTasks(accountId, taskList.id, taskPage.tasks, fullReconciliation)
              .get();
      if (std::holds_alternative<AppError>(taskWrite)) {
        return GoogleTaskMirrorSyncResultOrError(std::get<AppError>(std::move(taskWrite)));
      }
      if (taskMutationService_ != nullptr) {
        TaskRecurrenceReconciliationResult recurrenceResult =
            taskMutationService_->reconcileManagedRecurrences(accountId, taskList.id).get();
        if (std::holds_alternative<AppError>(recurrenceResult)) {
          return GoogleTaskMirrorSyncResultOrError(
              std::get<AppError>(std::move(recurrenceResult)));
        }
        const TaskRecurrenceReconciliation reconciliation =
            std::get<TaskRecurrenceReconciliation>(std::move(recurrenceResult));
        result.generatedRecurringTaskCount += reconciliation.createdSuccessorCount;
        result.removedRecurringTaskDuplicateCount += reconciliation.removedDuplicateCount;
        result.divergentRecurringTaskDuplicateGroupCount +=
            reconciliation.divergentDuplicateGroupCount;
      }
      const QString watermark = nextWatermark(taskPage.serverDate, checkpoint, clock_);
      SyncCheckpointSaveResult saved =
          checkpointStore_.save(checkpointKey, watermark, requestMetadata).get();
      if (std::holds_alternative<AppError>(saved)) {
        return GoogleTaskMirrorSyncResultOrError(std::get<AppError>(std::move(saved)));
      }
      result.taskCount += static_cast<std::int64_t>(taskPage.tasks.size());
      result.fullReconciledListCount += fullReconciliation ? 1 : 0;
    }
    return GoogleTaskMirrorSyncResultOrError(result);
  });
}

} // namespace hcb
