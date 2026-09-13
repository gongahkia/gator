#include "core/GoogleCalendarInstanceCacheService.h"

#include "core/GoogleCalendarEventPullClient.h"
#include "core/GoogleMirrorStore.h"

#include <QList>

#include <deque>
#include <future>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumConcurrentPulls = 4;

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= 256 &&
         !value.contains(QChar::Null);
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

struct PendingPull final {
  CalendarRecurringInstanceCacheTarget target;
  std::future<GoogleCalendarEventInstancesPullResultOrError> future;
};

} // namespace

GoogleCalendarInstanceCacheService::GoogleCalendarInstanceCacheService(
    GoogleCalendarEventPullClient& eventClient,
    CalendarReadService& calendarReadService,
    GoogleMirrorStore& mirrorStore)
    : eventClient_(eventClient), calendarReadService_(calendarReadService), mirrorStore_(mirrorStore) {}

std::future<GoogleCalendarInstanceCacheRefreshResultOrError>
GoogleCalendarInstanceCacheService::refresh(QString accountId,
                                             QString accessToken,
                                             CalendarRecurringInstanceCacheReadRequest request) {
  return std::async(std::launch::async,
                    [this,
                     accountId = std::move(accountId),
                     accessToken = std::move(accessToken),
                     request = std::move(request)] {
    if (!isValidIdentifier(accountId) || accessToken.isEmpty()) {
      return GoogleCalendarInstanceCacheRefreshResultOrError(
          validationError(QStringLiteral("Google recurrence-cache credentials are invalid")));
    }
    CalendarRecurringInstanceCacheTargetsResult listed =
        calendarReadService_.listUncachedRecurringInstances(request).get();
    if (std::holds_alternative<AppError>(listed)) {
      return GoogleCalendarInstanceCacheRefreshResultOrError(
          std::get<AppError>(std::move(listed)));
    }
    QList<CalendarRecurringInstanceCacheTarget> targets =
        std::get<QList<CalendarRecurringInstanceCacheTarget>>(std::move(listed));
    GoogleCalendarInstanceCacheRefreshResult result{.requested = targets.size()};
    std::deque<PendingPull> pending;
    qsizetype next = 0;
    while (next < targets.size() || !pending.empty()) {
      while (next < targets.size() && pending.size() < kMaximumConcurrentPulls) {
        CalendarRecurringInstanceCacheTarget target = targets.at(next);
        ++next;
        pending.push_back(
            {.target = target,
             .future = eventClient_.instances({.calendarId = target.calendarRemoteId,
                                               .recurringEventId = target.recurringRemoteId,
                                               .timeMin = request.startAt,
                                               .timeMax = request.endAt},
                                              accessToken)});
      }
      PendingPull pull = std::move(pending.front());
      pending.pop_front();
      GoogleCalendarEventInstancesPullResultOrError pulled = pull.future.get();
      if (std::holds_alternative<GoogleApiError>(pulled)) {
        ++result.failed;
        if (!result.firstFailure.has_value()) {
          result.firstFailure = std::get<GoogleApiError>(std::move(pulled)).message();
        }
        continue;
      }
      GoogleCalendarEventInstancesPullResult response =
          std::get<GoogleCalendarEventInstancesPullResult>(std::move(pulled));
      GoogleMirrorWriteResult cached =
          mirrorStore_
              .cacheCalendarInstances(accountId,
                                      pull.target.calendarRemoteId,
                                      pull.target.recurringRemoteId,
                                      request.startAt,
                                      request.endAt,
                                      std::move(response.events))
              .get();
      if (std::holds_alternative<AppError>(cached)) {
        ++result.failed;
        if (!result.firstFailure.has_value()) {
          result.firstFailure = std::get<AppError>(std::move(cached)).message();
        }
        continue;
      }
      ++result.cached;
    }
    return GoogleCalendarInstanceCacheRefreshResultOrError(std::move(result));
  });
}

} // namespace hcb
