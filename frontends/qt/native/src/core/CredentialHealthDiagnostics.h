#pragma once

#include "core/AccountStatusService.h"
#include "core/OAuthCredentialStore.h"

#include <QString>
#include <QStringList>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

enum class CredentialStorageHealth : std::uint8_t {
  NotChecked,
  Missing,
  AccessOnly,
  Refreshable,
  Invalid
};

struct CredentialHealthDiagnostic final {
  QString accountId;
  std::optional<AccountConnectionState> connectionState;
  QStringList missingScopes;
  CredentialStorageHealth credentialStorage{CredentialStorageHealth::NotChecked};

  [[nodiscard]] bool isReady() const noexcept;
};

using CredentialHealthDiagnosticResult = std::variant<CredentialHealthDiagnostic, AppError>;

class CredentialHealthDiagnostics final {
public:
  CredentialHealthDiagnostics(AccountStatusService& accountStatuses,
                              OAuthCredentialStore& credentials);

  [[nodiscard]] std::future<CredentialHealthDiagnosticResult> inspect(QString accountId);

private:
  AccountStatusService& accountStatuses_;
  OAuthCredentialStore& credentials_;
};

} // namespace hcb
