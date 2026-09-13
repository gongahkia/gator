#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QList>
#include <QString>
#include <QStringList>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

enum class AccountConnectionState : std::uint8_t {
  SignedOut,
  Connected,
  ReauthRequired,
  SyncPaused
};

struct AccountStatus final {
  QString accountId;
  std::optional<QString> providerAccountId;
  std::optional<QString> email;
  std::optional<QString> displayName;
  std::optional<QString> avatarUrl;
  std::optional<QString> locale;
  std::optional<QString> timeZone;
  AccountConnectionState connectionState;
  QStringList grantedScopes;
  QStringList missingScopes;
  std::optional<QString> lastAuthenticatedAt;
  QString updatedAt;
};

struct AccountStatusInput final {
  QString accountId;
  std::optional<QString> providerAccountId;
  std::optional<QString> email;
  std::optional<QString> displayName;
  std::optional<QString> avatarUrl;
  std::optional<QString> locale;
  std::optional<QString> timeZone;
  AccountConnectionState connectionState;
  QStringList grantedScopes;
  std::optional<QString> lastAuthenticatedAt;
};

enum class AccountStatusMutationResult : std::uint8_t {
  Changed,
  Unchanged
};

struct AccountStatusSaveResult final {
  AccountStatus status;
  AccountStatusMutationResult mutation;
};

using AccountStatusLookupResult = std::variant<std::optional<AccountStatus>, AppError>;
using AccountStatusListResult = std::variant<QList<AccountStatus>, AppError>;
using AccountStatusSaveResultOrError = std::variant<AccountStatusSaveResult, AppError>;

class AccountStatusService final {
public:
  AccountStatusService(FilePath databasePath, const Clock& clock);
  AccountStatusService(const AccountStatusService&) = delete;
  AccountStatusService& operator=(const AccountStatusService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<AccountStatusLookupResult> find(QString accountId);
  [[nodiscard]] std::future<AccountStatusLookupResult> latest();
  [[nodiscard]] std::future<AccountStatusListResult> list();
  [[nodiscard]] std::future<AccountStatusSaveResultOrError> upsert(AccountStatusInput input);
  [[nodiscard]] std::future<AccountStatusSaveResultOrError> disconnect(QString accountId);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
