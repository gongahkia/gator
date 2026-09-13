#pragma once

#include "core/AppError.h"
#include "core/Cancellation.h"
#include "core/GoogleApiError.h"
#include "core/SyncBackoffPolicy.h"

#include <QString>

#include <cstdint>
#include <future>
#include <variant>

namespace hcb {

class CalendarReadService;
class GoogleCalendarEventPullClient;
class GoogleCalendarListPullClient;
class GoogleMirrorStore;
class GoogleSyncRecoveryService;
class SyncCheckpointStore;

struct GoogleCalendarMirrorSyncResult final {
  std::int64_t calendarCount{0};
  std::int64_t eventCount{0};
  std::int64_t fullReconciledCalendarCount{0};
};

using GoogleCalendarMirrorSyncResultOrError =
    std::variant<GoogleCalendarMirrorSyncResult, GoogleApiError, AppError>;

class GoogleCalendarMirrorSyncService final {
public:
  GoogleCalendarMirrorSyncService(GoogleCalendarListPullClient& calendarListClient,
                                  GoogleCalendarEventPullClient& eventClient,
                                  CalendarReadService& calendarReadService,
                                  GoogleMirrorStore& mirrorStore,
                                  SyncCheckpointStore& checkpointStore,
                                  GoogleSyncRecoveryService& recoveryService);
  GoogleCalendarMirrorSyncService(GoogleCalendarListPullClient& calendarListClient,
                                  GoogleCalendarEventPullClient& eventClient,
                                  CalendarReadService& calendarReadService,
                                  GoogleMirrorStore& mirrorStore,
                                  SyncCheckpointStore& checkpointStore,
                                  GoogleSyncRecoveryService& recoveryService,
                                  SyncBackoffPolicy backoffPolicy);

  [[nodiscard]] std::future<GoogleCalendarMirrorSyncResultOrError>
  sync(QString accountId, QString accessToken, CancellationToken cancellation = {});

private:
  GoogleCalendarListPullClient& calendarListClient_;
  GoogleCalendarEventPullClient& eventClient_;
  CalendarReadService& calendarReadService_;
  GoogleMirrorStore& mirrorStore_;
  SyncCheckpointStore& checkpointStore_;
  GoogleSyncRecoveryService& recoveryService_;
  SyncBackoffPolicy backoffPolicy_;
};

} // namespace hcb
