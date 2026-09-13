#pragma once

#include "core/AppError.h"
#include "core/CalendarReadService.h"

#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

class GoogleCalendarEventPullClient;
class GoogleMirrorStore;

struct GoogleCalendarInstanceCacheRefreshResult final {
  std::int64_t requested{0};
  std::int64_t cached{0};
  std::int64_t failed{0};
  std::optional<QString> firstFailure;
};

using GoogleCalendarInstanceCacheRefreshResultOrError =
    std::variant<GoogleCalendarInstanceCacheRefreshResult, AppError>;

class GoogleCalendarInstanceCacheService final {
public:
  GoogleCalendarInstanceCacheService(GoogleCalendarEventPullClient& eventClient,
                                     CalendarReadService& calendarReadService,
                                     GoogleMirrorStore& mirrorStore);

  [[nodiscard]] std::future<GoogleCalendarInstanceCacheRefreshResultOrError>
  refresh(QString accountId,
          QString accessToken,
          CalendarRecurringInstanceCacheReadRequest request);

private:
  GoogleCalendarEventPullClient& eventClient_;
  CalendarReadService& calendarReadService_;
  GoogleMirrorStore& mirrorStore_;
};

} // namespace hcb
