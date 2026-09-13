#pragma once

#include "core/AppError.h"
#include "core/GoogleApiError.h"
#include "core/OptimisticMutationCoordinator.h"
#include "core/SyncConflictPolicy.h"
#include "core/SyncConflictStore.h"

#include <QString>

#include <atomic>
#include <cstdint>
#include <optional>
#include <variant>

namespace hcb {

class GoogleHttpClient;

enum class GoogleSyncConflictOutcome : std::uint8_t {
  KeptRemote,
  ReappliedLocal,
  AwaitingUser
};

using GoogleSyncConflictResult =
    std::variant<GoogleSyncConflictOutcome, GoogleApiError, AppError>;

class GoogleSyncConflictResolver final {
public:
  GoogleSyncConflictResolver(OptimisticMutationCoordinator& mutations,
                             SyncConflictStore& conflicts,
                             GoogleHttpClient& httpClient);

  void setPolicy(SyncConflictPolicy policy) noexcept;
  [[nodiscard]] SyncConflictPolicy policy() const noexcept;
  [[nodiscard]] GoogleSyncConflictResult handle(PendingMutation mutation,
                                                QString errorCode,
                                                QString errorMessage,
                                                QString accessToken);
  [[nodiscard]] std::future<std::optional<AppError>>
  resolve(QString conflictId, SyncConflictResolution resolution);

private:
  OptimisticMutationCoordinator& mutations_;
  SyncConflictStore& conflicts_;
  GoogleHttpClient& httpClient_;
  std::atomic<SyncConflictPolicy> policy_{SyncConflictPolicy::PreferGoogle};
};

} // namespace hcb
