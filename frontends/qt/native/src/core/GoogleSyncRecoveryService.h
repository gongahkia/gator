#pragma once

#include "core/GoogleApiError.h"
#include "core/SyncCheckpointStore.h"

#include <cstdint>
#include <future>
#include <variant>

namespace hcb {

enum class GoogleSyncRecoveryResult : std::uint8_t {
  NotRequired,
  ReadyForFullResync
};

using GoogleSyncRecoveryResultOrError = std::variant<GoogleSyncRecoveryResult, AppError>;

class GoogleSyncRecoveryService final {
public:
  explicit GoogleSyncRecoveryService(SyncCheckpointStore& checkpointStore);

  [[nodiscard]] std::future<GoogleSyncRecoveryResultOrError> recover(SyncCheckpointKey key,
                                                                     const GoogleApiError& error);

private:
  SyncCheckpointStore& checkpointStore_;
};

} // namespace hcb
