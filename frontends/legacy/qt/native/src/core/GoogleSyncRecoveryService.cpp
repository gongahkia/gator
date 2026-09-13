#include "core/GoogleSyncRecoveryService.h"

#include <future>
#include <utility>
#include <variant>

namespace hcb {

GoogleSyncRecoveryService::GoogleSyncRecoveryService(SyncCheckpointStore& checkpointStore)
    : checkpointStore_(checkpointStore) {}

std::future<GoogleSyncRecoveryResultOrError>
GoogleSyncRecoveryService::recover(SyncCheckpointKey key, const GoogleApiError& error) {
  if (error.kind() != GoogleApiErrorKind::InvalidSyncToken) {
    std::promise<GoogleSyncRecoveryResultOrError> completion;
    std::future<GoogleSyncRecoveryResultOrError> future = completion.get_future();
    completion.set_value(GoogleSyncRecoveryResult::NotRequired);
    return future;
  }
  return std::async(std::launch::async, [this, key = std::move(key)]() mutable {
    SyncCheckpointEraseResult erased = checkpointStore_.erase(std::move(key)).get();
    if (std::holds_alternative<AppError>(erased)) {
      return GoogleSyncRecoveryResultOrError(std::get<AppError>(std::move(erased)));
    }
    return GoogleSyncRecoveryResultOrError(GoogleSyncRecoveryResult::ReadyForFullResync);
  });
}

} // namespace hcb
