#pragma once

#include "core/OAuthCredentialStore.h"

namespace hcb {

class MacOSCredentialAdapter final : public OAuthCredentialStore {
public:
  [[nodiscard]] std::future<OAuthCredentialReadResult> read(QString accountId) override;
  [[nodiscard]] std::future<OAuthCredentialSaveResult>
  save(QString accountId, OAuthStoredCredential credential) override;
  [[nodiscard]] std::future<OAuthCredentialDeleteResult> erase(QString accountId) override;
};

} // namespace hcb
