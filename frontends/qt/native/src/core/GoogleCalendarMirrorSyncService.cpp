#include "core/GoogleCalendarMirrorSyncService.h"

#include "core/CalendarReadService.h"
#include "core/GoogleCalendarEventPullClient.h"
#include "core/GoogleCalendarListPullClient.h"
#include "core/GoogleMirrorStore.h"
#include "core/GoogleSyncRecoveryService.h"
#include "core/SyncCheckpointStore.h"

#include <QSet>

#include <algorithm>
#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr char kCalendarListResourceId[] = "calendar-list";
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

[[nodiscard]] QJsonObject calendarListRequestMetadata() {
  return {{QStringLiteral("fields"),
           QStringLiteral("nextPageToken,nextSyncToken,items(id,summary,summaryOverride,"
                          "description,timeZone,backgroundColor,foregroundColor,accessRole,"
                          "selected,hidden,primary,deleted,etag)")},
          {QStringLiteral("maxResults"), QStringLiteral("250")},
          {QStringLiteral("showDeleted"), true},
          {QStringLiteral("showHidden"), true}};
}

[[nodiscard]] QJsonObject calendarEventRequestMetadata() {
  return {{QStringLiteral("fields"),
           QStringLiteral("nextPageToken,nextSyncToken,items(id,status,summary,description,"
                          "location,start,end,recurringEventId,originalStartTime,recurrence,"
                          "colorId,transparency,visibility,eventType,attendees,reminders,etag,sequence,"
                          "updated)")},
          {QStringLiteral("maxResults"), QStringLiteral("250")},
          {QStringLiteral("showDeleted"), true},
          {QStringLiteral("showHiddenInvitations"), true},
          {QStringLiteral("singleEvents"), false}};
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] AppError cancelledError() {
  return AppError(AppErrorCode::Cancelled, QStringLiteral("Google calendar sync was cancelled"));
}

[[nodiscard]] bool validIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= 256 &&
         !value.contains(QChar::Null);
}

[[nodiscard]] std::variant<QList<QString>, AppError> selectedCalendarRemoteIds(
    CalendarReadService& service, const QString& accountId) {
  QList<QString> remoteIds;
  QSet<QString> seen;
  std::int64_t offset = 0;
  while (true) {
    CalendarListPageResult listed =
        service
            .listCalendars(
                {.accountId = accountId, .selectedOnly = true, .limit = 100, .offset = offset})
            .get();
    if (std::holds_alternative<AppError>(listed)) {
      return std::get<AppError>(std::move(listed));
    }
    CalendarListPage page = std::get<CalendarListPage>(std::move(listed));
    for (const CalendarSummary& calendar : page.items) {
      if (!seen.contains(calendar.remoteId)) {
        seen.insert(calendar.remoteId);
        remoteIds.append(calendar.remoteId);
      }
    }
    if (!page.nextOffset.has_value()) {
      return remoteIds;
    }
    offset = *page.nextOffset;
  }
}

[[nodiscard]] std::optional<AppError> saveCheckpoint(SyncCheckpointStore& checkpoints,
                                                      const SyncCheckpointKey& key,
                                                      const std::optional<QString>& token,
                                                      const QJsonObject& metadata) {
  if (!token.has_value()) {
    return validationError(QStringLiteral("Google incremental sync response lacks a sync token"));
  }
  SyncCheckpointSaveResult saved = checkpoints.save(key, *token, metadata).get();
  return std::holds_alternative<AppError>(saved)
             ? std::optional<AppError>(std::get<AppError>(std::move(saved)))
             : std::nullopt;
}

} // namespace

GoogleCalendarMirrorSyncService::GoogleCalendarMirrorSyncService(
    GoogleCalendarListPullClient& calendarListClient,
    GoogleCalendarEventPullClient& eventClient,
    CalendarReadService& calendarReadService,
    GoogleMirrorStore& mirrorStore,
    SyncCheckpointStore& checkpointStore,
    GoogleSyncRecoveryService& recoveryService)
    : GoogleCalendarMirrorSyncService(calendarListClient,
                                      eventClient,
                                      calendarReadService,
                                      mirrorStore,
                                      checkpointStore,
                                      recoveryService,
                                      mirrorBackoffPolicy()) {}

GoogleCalendarMirrorSyncService::GoogleCalendarMirrorSyncService(
    GoogleCalendarListPullClient& calendarListClient,
    GoogleCalendarEventPullClient& eventClient,
    CalendarReadService& calendarReadService,
    GoogleMirrorStore& mirrorStore,
    SyncCheckpointStore& checkpointStore,
    GoogleSyncRecoveryService& recoveryService,
    SyncBackoffPolicy backoffPolicy)
    : calendarListClient_(calendarListClient), eventClient_(eventClient),
      calendarReadService_(calendarReadService), mirrorStore_(mirrorStore),
      checkpointStore_(checkpointStore), recoveryService_(recoveryService),
      backoffPolicy_(std::move(backoffPolicy)) {}

std::future<GoogleCalendarMirrorSyncResultOrError>
GoogleCalendarMirrorSyncService::sync(QString accountId,
                                      QString accessToken,
                                      CancellationToken cancellation) {
  return std::async(std::launch::async,
                    [this,
                     accountId = std::move(accountId),
                     accessToken = std::move(accessToken),
                     cancellation] {
    if (!validIdentifier(accountId)) {
      return GoogleCalendarMirrorSyncResultOrError(
          validationError(QStringLiteral("Google calendar sync account is invalid")));
    }
    if (cancellation.stop_requested()) {
      return GoogleCalendarMirrorSyncResultOrError(cancelledError());
    }
    const SyncCheckpointKey listCheckpointKey{
        .accountId = accountId,
        .resourceType = SyncCheckpointResourceType::CalendarList,
        .resourceId = QString::fromLatin1(kCalendarListResourceId)};
    SyncCheckpointLookupResult storedListCheckpoint = checkpointStore_.find(listCheckpointKey).get();
    if (std::holds_alternative<AppError>(storedListCheckpoint)) {
      return GoogleCalendarMirrorSyncResultOrError(
          std::get<AppError>(std::move(storedListCheckpoint)));
    }
    std::optional<SyncCheckpoint> listCheckpoint =
        std::get<std::optional<SyncCheckpoint>>(std::move(storedListCheckpoint));
    const QJsonObject listRequestMetadata = calendarListRequestMetadata();
    const QJsonObject eventRequestMetadata = calendarEventRequestMetadata();
    bool fullCalendarList = !listCheckpoint.has_value() ||
                            listCheckpoint->metadata != listRequestMetadata;
    GoogleCalendarListPullResultOrError pulledList =
        pullWithRetry<GoogleCalendarListPullResultOrError>(
            [&] {
              return calendarListClient_
                  .list({.syncToken = !fullCalendarList
                                       ? std::optional<QString>(listCheckpoint->syncToken)
                                       : std::optional<QString>{}},
                        accessToken)
                  .get();
            },
            backoffPolicy_,
            cancellation);
    if (cancellation.stop_requested()) {
      return GoogleCalendarMirrorSyncResultOrError(cancelledError());
    }
    if (std::holds_alternative<GoogleApiError>(pulledList)) {
      GoogleApiError error = std::get<GoogleApiError>(std::move(pulledList));
      GoogleSyncRecoveryResultOrError recovered =
          recoveryService_.recover(listCheckpointKey, error).get();
      if (std::holds_alternative<AppError>(recovered)) {
        return GoogleCalendarMirrorSyncResultOrError(std::get<AppError>(std::move(recovered)));
      }
      if (std::get<GoogleSyncRecoveryResult>(recovered) !=
          GoogleSyncRecoveryResult::ReadyForFullResync) {
        return GoogleCalendarMirrorSyncResultOrError(std::move(error));
      }
      fullCalendarList = true;
      pulledList = pullWithRetry<GoogleCalendarListPullResultOrError>(
          [&] { return calendarListClient_.list({}, accessToken).get(); }, backoffPolicy_, cancellation);
      if (cancellation.stop_requested()) {
        return GoogleCalendarMirrorSyncResultOrError(cancelledError());
      }
      if (std::holds_alternative<GoogleApiError>(pulledList)) {
        return GoogleCalendarMirrorSyncResultOrError(
            std::get<GoogleApiError>(std::move(pulledList)));
      }
    }
    GoogleCalendarListPullResult calendars =
        std::get<GoogleCalendarListPullResult>(std::move(pulledList));
    GoogleMirrorWriteResult calendarWrite =
        mirrorStore_.mergeCalendars(accountId, calendars.calendars, fullCalendarList).get();
    if (std::holds_alternative<AppError>(calendarWrite)) {
      return GoogleCalendarMirrorSyncResultOrError(std::get<AppError>(std::move(calendarWrite)));
    }
    if (const std::optional<AppError> error =
            saveCheckpoint(checkpointStore_, listCheckpointKey, calendars.nextSyncToken,
                           listRequestMetadata);
        error.has_value()) {
      return GoogleCalendarMirrorSyncResultOrError(*error);
    }
    const auto selected = selectedCalendarRemoteIds(calendarReadService_, accountId);
    if (std::holds_alternative<AppError>(selected)) {
      return GoogleCalendarMirrorSyncResultOrError(std::get<AppError>(std::move(selected)));
    }
    GoogleCalendarMirrorSyncResult result{
        .calendarCount = static_cast<std::int64_t>(calendars.calendars.size())};
    for (const QString& calendarId : std::get<QList<QString>>(selected)) {
      if (cancellation.stop_requested()) {
        return GoogleCalendarMirrorSyncResultOrError(cancelledError());
      }
      const SyncCheckpointKey eventCheckpointKey{.accountId = accountId,
                                                 .resourceType = SyncCheckpointResourceType::CalendarEvent,
                                                 .resourceId = calendarId};
      SyncCheckpointLookupResult storedEventCheckpoint = checkpointStore_.find(eventCheckpointKey).get();
      if (std::holds_alternative<AppError>(storedEventCheckpoint)) {
        return GoogleCalendarMirrorSyncResultOrError(
            std::get<AppError>(std::move(storedEventCheckpoint)));
      }
      std::optional<SyncCheckpoint> eventCheckpoint =
          std::get<std::optional<SyncCheckpoint>>(std::move(storedEventCheckpoint));
      bool fullCalendar = !eventCheckpoint.has_value() ||
                          eventCheckpoint->metadata != eventRequestMetadata;
      GoogleCalendarEventPullResultOrError pulledEvents =
          pullWithRetry<GoogleCalendarEventPullResultOrError>(
              [&] {
                return eventClient_
                    .list({.calendarId = calendarId,
                           .syncToken = !fullCalendar
                                            ? std::optional<QString>(eventCheckpoint->syncToken)
                                            : std::optional<QString>{}},
                          accessToken)
                    .get();
              },
              backoffPolicy_,
              cancellation);
      if (cancellation.stop_requested()) {
        return GoogleCalendarMirrorSyncResultOrError(cancelledError());
      }
      if (std::holds_alternative<GoogleApiError>(pulledEvents)) {
        GoogleApiError error = std::get<GoogleApiError>(std::move(pulledEvents));
        GoogleSyncRecoveryResultOrError recovered =
            recoveryService_.recover(eventCheckpointKey, error).get();
        if (std::holds_alternative<AppError>(recovered)) {
          return GoogleCalendarMirrorSyncResultOrError(std::get<AppError>(std::move(recovered)));
        }
        if (std::get<GoogleSyncRecoveryResult>(recovered) !=
            GoogleSyncRecoveryResult::ReadyForFullResync) {
          return GoogleCalendarMirrorSyncResultOrError(std::move(error));
        }
        fullCalendar = true;
        pulledEvents = pullWithRetry<GoogleCalendarEventPullResultOrError>(
            [&] { return eventClient_.list({.calendarId = calendarId}, accessToken).get(); },
            backoffPolicy_,
            cancellation);
        if (cancellation.stop_requested()) {
          return GoogleCalendarMirrorSyncResultOrError(cancelledError());
        }
        if (std::holds_alternative<GoogleApiError>(pulledEvents)) {
          return GoogleCalendarMirrorSyncResultOrError(
              std::get<GoogleApiError>(std::move(pulledEvents)));
        }
      }
      GoogleCalendarEventPullResult events =
          std::get<GoogleCalendarEventPullResult>(std::move(pulledEvents));
      GoogleMirrorWriteResult eventWrite =
          mirrorStore_.mergeCalendarEvents(accountId, calendarId, events.events, fullCalendar).get();
      if (std::holds_alternative<AppError>(eventWrite)) {
        return GoogleCalendarMirrorSyncResultOrError(std::get<AppError>(std::move(eventWrite)));
      }
      if (const std::optional<AppError> error =
              saveCheckpoint(checkpointStore_, eventCheckpointKey, events.nextSyncToken,
                             eventRequestMetadata);
          error.has_value()) {
        return GoogleCalendarMirrorSyncResultOrError(*error);
      }
      result.eventCount += static_cast<std::int64_t>(events.events.size());
      result.fullReconciledCalendarCount += fullCalendar ? 1 : 0;
    }
    return GoogleCalendarMirrorSyncResultOrError(result);
  });
}

} // namespace hcb
