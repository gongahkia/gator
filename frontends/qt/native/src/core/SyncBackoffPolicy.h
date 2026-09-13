#pragma once

#include "core/GoogleApiError.h"

#include <QtTypes>

#include <functional>
#include <optional>

namespace hcb {

struct SyncBackoffConstraintState {
  bool lowPowerMode{false};
  bool constrainedNetwork{false};
};

using SyncBackoffRandomProvider = std::function<double()>;
using SyncBackoffConstraintStateProvider = std::function<SyncBackoffConstraintState()>;

struct SyncBackoffPolicyOptions {
  qint64 baseDelayMilliseconds{90'000};
  qint64 maximumDelayMilliseconds{600'000};
  qint64 jitterMilliseconds{15'000};
  int maximumAttempts{6};
  SyncBackoffRandomProvider random;
  SyncBackoffConstraintStateProvider constraintState;
};

[[nodiscard]] double syncBackoffConstraintMultiplier(SyncBackoffConstraintState state) noexcept;
[[nodiscard]] SyncBackoffConstraintState defaultSyncBackoffConstraintState();

class SyncBackoffPolicy final {
public:
  explicit SyncBackoffPolicy(SyncBackoffPolicyOptions options = {});

  [[nodiscard]] qint64 delayMillisecondsForAttempt(int attempt) const;
  [[nodiscard]] std::optional<qint64> retryDelayMilliseconds(const GoogleApiError& error,
                                                             int attempt) const;
  [[nodiscard]] bool shouldBackOff(const GoogleApiError& error) const noexcept;

private:
  [[nodiscard]] qint64 applyConstraintMultiplier(qint64 delayMilliseconds) const;

  qint64 baseDelayMilliseconds_;
  qint64 maximumDelayMilliseconds_;
  qint64 jitterMilliseconds_;
  int maximumAttempts_;
  SyncBackoffRandomProvider random_;
  SyncBackoffConstraintStateProvider constraintState_;
};

} // namespace hcb
