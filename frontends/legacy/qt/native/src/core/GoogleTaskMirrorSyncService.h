#pragma once

#include "core/AppError.h"
#include "core/Cancellation.h"
#include "core/Clock.h"
#include "core/GoogleApiError.h"
#include "core/SyncBackoffPolicy.h"

#include <QString>

#include <cstdint>
#include <future>
#include <variant>

namespace hcb {

class GoogleMirrorStore;
class GoogleTaskListPullClient;
class GoogleTaskPullClient;
class SyncCheckpointStore;
class TaskMutationService;

struct GoogleTaskMirrorSyncResult final {
  std::int64_t taskListCount{0};
  std::int64_t taskCount{0};
  std::int64_t fullReconciledListCount{0};
  std::int64_t generatedRecurringTaskCount{0};
  std::int64_t removedRecurringTaskDuplicateCount{0};
  std::int64_t divergentRecurringTaskDuplicateGroupCount{0};
};

using GoogleTaskMirrorSyncResultOrError =
    std::variant<GoogleTaskMirrorSyncResult, GoogleApiError, AppError>;

class GoogleTaskMirrorSyncService final {
public:
  GoogleTaskMirrorSyncService(GoogleTaskListPullClient& taskListClient,
                              GoogleTaskPullClient& taskClient,
                              GoogleMirrorStore& mirrorStore,
                              SyncCheckpointStore& checkpointStore,
                              const Clock& clock,
                              TaskMutationService* taskMutationService = nullptr);
  GoogleTaskMirrorSyncService(GoogleTaskListPullClient& taskListClient,
                              GoogleTaskPullClient& taskClient,
                              GoogleMirrorStore& mirrorStore,
                              SyncCheckpointStore& checkpointStore,
                              const Clock& clock,
                              SyncBackoffPolicy backoffPolicy,
                              TaskMutationService* taskMutationService = nullptr);

  [[nodiscard]] std::future<GoogleTaskMirrorSyncResultOrError>
  sync(QString accountId, QString accessToken, CancellationToken cancellation = {});

private:
  GoogleTaskListPullClient& taskListClient_;
  GoogleTaskPullClient& taskClient_;
  GoogleMirrorStore& mirrorStore_;
  SyncCheckpointStore& checkpointStore_;
  const Clock& clock_;
  SyncBackoffPolicy backoffPolicy_;
  TaskMutationService* taskMutationService_{nullptr};
};

} // namespace hcb
