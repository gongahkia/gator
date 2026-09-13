#pragma once

#include "core/AppError.h"
#include "core/SyncBackoffPolicy.h"

#include <QString>

#include <future>
#include <variant>

namespace hcb {

class Clock;
class GoogleHttpClient;
class GoogleSyncConflictResolver;
class OptimisticMutationCoordinator;
class MutationTelemetryStore;
class TaskListMutationService;
class TaskMutationService;

struct GoogleTaskMutationPushResult final {
  int applied{0};
  int failed{0};
  int skipped{0};
};

using GoogleTaskMutationPushResultOrError = std::variant<GoogleTaskMutationPushResult, AppError>;

class GoogleTaskMutationPushService final {
public:
  GoogleTaskMutationPushService(OptimisticMutationCoordinator& mutations,
                                GoogleHttpClient& httpClient,
                                const Clock& clock,
                                SyncBackoffPolicy backoffPolicy,
                                 TaskMutationService* taskMutationService = nullptr,
                                 TaskListMutationService* taskListMutationService = nullptr,
                                 GoogleSyncConflictResolver* conflictResolver = nullptr,
                                 MutationTelemetryStore* mutationTelemetryStore = nullptr);

  [[nodiscard]] std::future<GoogleTaskMutationPushResultOrError> pushDue(QString accessToken,
                                                                         int limit = 25);

private:
  OptimisticMutationCoordinator& mutations_;
  GoogleHttpClient& httpClient_;
  const Clock& clock_;
  SyncBackoffPolicy backoffPolicy_;
  TaskMutationService* taskMutationService_;
  TaskListMutationService* taskListMutationService_;
  GoogleSyncConflictResolver* conflictResolver_;
  MutationTelemetryStore* mutationTelemetryStore_;
};

} // namespace hcb
