#include "core/SyncBackoffPolicy.h"

#include <QRandomGenerator>

#include <algorithm>
#include <cmath>
#include <limits>
#include <utility>

namespace hcb {
namespace {

[[nodiscard]] double clampedRandomValue(double value) noexcept {
  if (std::isnan(value)) {
    return 0.0;
  }
  return std::clamp(value, 0.0, 1.0);
}

[[nodiscard]] qint64 saturatedAdd(qint64 left, qint64 right) noexcept {
  if (right > 0 && left > std::numeric_limits<qint64>::max() - right) {
    return std::numeric_limits<qint64>::max();
  }
  return left + right;
}

[[nodiscard]] qint64 roundedJitter(qint64 jitterMilliseconds, double randomValue) noexcept {
  const long double jitter = static_cast<long double>(jitterMilliseconds) *
                             static_cast<long double>(clampedRandomValue(randomValue));
  return static_cast<qint64>(std::llround(jitter));
}

} // namespace

double syncBackoffConstraintMultiplier(SyncBackoffConstraintState state) noexcept {
  return (state.lowPowerMode ? 1.5 : 1.0) * (state.constrainedNetwork ? 2.0 : 1.0);
}

SyncBackoffConstraintState defaultSyncBackoffConstraintState() {
  return {
      .lowPowerMode = qEnvironmentVariable("HCB_LOW_POWER_MODE") == "1",
      .constrainedNetwork = qEnvironmentVariable("HCB_CONSTRAINED_NETWORK") == "1",
  };
}

SyncBackoffPolicy::SyncBackoffPolicy(SyncBackoffPolicyOptions options)
    : baseDelayMilliseconds_(std::max<qint64>(0, options.baseDelayMilliseconds)),
      maximumDelayMilliseconds_(std::max<qint64>(0, options.maximumDelayMilliseconds)),
      jitterMilliseconds_(std::max<qint64>(0, options.jitterMilliseconds)),
      maximumAttempts_(std::max(0, options.maximumAttempts)),
      random_(options.random ? std::move(options.random) : SyncBackoffRandomProvider([] {
        return QRandomGenerator::global()->generateDouble();
      })),
      constraintState_(options.constraintState ? std::move(options.constraintState)
                                               : SyncBackoffConstraintStateProvider(
                                                     defaultSyncBackoffConstraintState)) {}

qint64 SyncBackoffPolicy::delayMillisecondsForAttempt(int attempt) const {
  const int clampedAttempt = std::clamp(attempt, 0, maximumAttempts_);
  qint64 exponentialDelay = baseDelayMilliseconds_;
  for (int index = 0; index < clampedAttempt && exponentialDelay < maximumDelayMilliseconds_;
       ++index) {
    exponentialDelay = exponentialDelay > maximumDelayMilliseconds_ / 2
                           ? maximumDelayMilliseconds_
                           : std::min(exponentialDelay * 2, maximumDelayMilliseconds_);
  }

  const qint64 cappedDelay = std::min(exponentialDelay, maximumDelayMilliseconds_);
  const qint64 jitter = roundedJitter(jitterMilliseconds_, random_());
  const qint64 cappedWithJitter =
      std::min(saturatedAdd(cappedDelay, jitter),
               saturatedAdd(maximumDelayMilliseconds_, jitterMilliseconds_));
  return applyConstraintMultiplier(cappedWithJitter);
}

std::optional<qint64> SyncBackoffPolicy::retryDelayMilliseconds(const GoogleApiError& error,
                                                                int attempt) const {
  if (!shouldBackOff(error)) {
    return std::nullopt;
  }
  if (error.retryAfterMilliseconds().has_value()) {
    return applyConstraintMultiplier(*error.retryAfterMilliseconds());
  }
  return delayMillisecondsForAttempt(attempt);
}

bool SyncBackoffPolicy::shouldBackOff(const GoogleApiError& error) const noexcept {
  if (error.quotaExceeded()) {
    return false;
  }
  return error.kind() == GoogleApiErrorKind::RateLimited ||
         error.kind() == GoogleApiErrorKind::Server;
}

qint64 SyncBackoffPolicy::applyConstraintMultiplier(qint64 delayMilliseconds) const {
  const long double adjustedDelay =
      static_cast<long double>(delayMilliseconds) *
      static_cast<long double>(syncBackoffConstraintMultiplier(constraintState_()));
  return adjustedDelay >= static_cast<long double>(std::numeric_limits<qint64>::max())
             ? std::numeric_limits<qint64>::max()
             : static_cast<qint64>(std::llround(adjustedDelay));
}

} // namespace hcb
