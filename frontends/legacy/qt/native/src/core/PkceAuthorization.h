#pragma once

#include "core/Clock.h"

#include <QHash>
#include <QMutex>
#include <QString>
#include <QStringView>

#include <chrono>
#include <optional>

namespace hcb {

struct PkceAuthorizationRequest {
  QString state;
  QString codeChallenge;
};

enum class PkceStateValidationStatus {
  Accepted,
  Unrecognized,
  Expired
};

struct PkceStateValidationResult {
  PkceStateValidationStatus status;
  QString codeVerifier;
};

class PkceAuthorization final {
public:
  [[nodiscard]] static QString generateCodeVerifier();
  [[nodiscard]] static QString generateState();
  [[nodiscard]] static std::optional<QString> codeChallengeForVerifier(QStringView codeVerifier);
  [[nodiscard]] static bool isValidCodeVerifier(QStringView codeVerifier) noexcept;
};

class PkceStateRegistry final {
public:
  static constexpr std::chrono::minutes kStateTtl{10};

  explicit PkceStateRegistry(const Clock& clock) : clock_(clock) {}
  PkceStateRegistry(const PkceStateRegistry&) = delete;
  PkceStateRegistry& operator=(const PkceStateRegistry&) = delete;

  [[nodiscard]] PkceAuthorizationRequest begin();
  [[nodiscard]] PkceStateValidationResult consume(QStringView state);

private:
  struct PendingState {
    QString codeVerifier;
    MonotonicTimePoint expiresAt;
  };

  const Clock& clock_;
  QMutex mutex_;
  QHash<QString, PendingState> pendingStates_;
};

} // namespace hcb
