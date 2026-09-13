#pragma once

#include "core/AppError.h"
#include "core/SyncBackoffPolicy.h"

#include <QString>

#include <future>
#include <variant>

namespace hcb {

class Clock;
class CalendarMutationService;
class GoogleHttpClient;
class GoogleSyncConflictResolver;
class MutationTelemetryStore;
class OptimisticMutationCoordinator;

struct GoogleCalendarEventMutationPushResult final {
  int applied{0};
  int failed{0};
  int skipped{0};
};

using GoogleCalendarEventMutationPushResultOrError =
    std::variant<GoogleCalendarEventMutationPushResult, AppError>;

class GoogleCalendarEventMutationPushService final {
public:
  GoogleCalendarEventMutationPushService(OptimisticMutationCoordinator& mutations,
                                         GoogleHttpClient& httpClient,
                                         const Clock& clock,
                                         SyncBackoffPolicy backoffPolicy,
                                         CalendarMutationService* calendarMutationService = nullptr,
                                         GoogleSyncConflictResolver* conflictResolver = nullptr,
                                         MutationTelemetryStore* mutationTelemetryStore = nullptr);

  [[nodiscard]] std::future<GoogleCalendarEventMutationPushResultOrError>
  pushDue(QString accessToken, int limit = 25);

private:
  OptimisticMutationCoordinator& mutations_;
  GoogleHttpClient& httpClient_;
  const Clock& clock_;
  SyncBackoffPolicy backoffPolicy_;
  CalendarMutationService* calendarMutationService_;
  GoogleSyncConflictResolver* conflictResolver_;
  MutationTelemetryStore* mutationTelemetryStore_;
};

} // namespace hcb
