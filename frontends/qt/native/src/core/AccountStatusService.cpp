#include "core/AccountStatusService.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonParseError>
#include <QString>
#include <QTimeZone>

#include <algorithm>
#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumAccountIdLength = 256;
constexpr qsizetype kMaximumEmailLength = 254;
constexpr qsizetype kMaximumDisplayNameLength = 256;
constexpr qsizetype kMaximumAvatarUrlLength = 2'048;
constexpr qsizetype kMaximumLocaleLength = 64;
constexpr qsizetype kMaximumTimeZoneLength = 128;
constexpr qsizetype kMaximumTimestampLength = 64;
constexpr qsizetype kMaximumScopeJsonBytes = 8'192;
constexpr qsizetype kMaximumScopeCount = 20;

[[nodiscard]] QString googleTasksScope() {
  return QStringLiteral("https://www.googleapis.com/auth/tasks");
}

[[nodiscard]] QString googleCalendarScope() {
  return QStringLiteral("https://www.googleapis.com/auth/calendar");
}

using AccountDecodeResult = std::variant<AccountStatus, AppError>;
using AccountStatusValidationResult = std::variant<AccountStatus, AppError>;

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] bool isValidRequiredText(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximumLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isValidOptionalText(const std::optional<QString>& value,
                                       qsizetype maximumLength) {
  return !value.has_value() || isValidRequiredText(*value, maximumLength);
}

[[nodiscard]] QString timestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] QString connectionStateText(AccountConnectionState state) {
  switch (state) {
  case AccountConnectionState::SignedOut:
    return QStringLiteral("signed_out");
  case AccountConnectionState::Connected:
    return QStringLiteral("connected");
  case AccountConnectionState::ReauthRequired:
    return QStringLiteral("reauth_required");
  case AccountConnectionState::SyncPaused:
    return QStringLiteral("sync_paused");
  }
  return {};
}

[[nodiscard]] std::optional<AccountConnectionState> connectionStateFromText(const QString& text) {
  if (text == QStringLiteral("signed_out")) {
    return AccountConnectionState::SignedOut;
  }
  if (text == QStringLiteral("connected")) {
    return AccountConnectionState::Connected;
  }
  if (text == QStringLiteral("reauth_required")) {
    return AccountConnectionState::ReauthRequired;
  }
  if (text == QStringLiteral("sync_paused")) {
    return AccountConnectionState::SyncPaused;
  }
  return std::nullopt;
}

[[nodiscard]] QStringList normalizeScopes(const QStringList& scopes) {
  QStringList normalized;
  normalized.reserve(scopes.size());
  for (const QString& scope : scopes) {
    const QString trimmed = scope.trimmed();
    if (!trimmed.isEmpty()) {
      normalized.append(trimmed);
    }
  }
  std::sort(normalized.begin(), normalized.end());
  normalized.erase(std::unique(normalized.begin(), normalized.end()), normalized.end());
  return normalized;
}

[[nodiscard]] QStringList missingRequiredScopes(const QStringList& grantedScopes) {
  QStringList missingScopes;
  if (!grantedScopes.contains(googleTasksScope())) {
    missingScopes.append(googleTasksScope());
  }
  if (!grantedScopes.contains(googleCalendarScope())) {
    missingScopes.append(googleCalendarScope());
  }
  return missingScopes;
}

[[nodiscard]] QString scopesJson(const QStringList& scopes) {
  QJsonArray array;
  for (const QString& scope : scopes) {
    array.append(scope);
  }
  return QString::fromUtf8(QJsonDocument(array).toJson(QJsonDocument::Compact));
}

[[nodiscard]] std::variant<QStringList, AppError> parseScopesJson(const QString& json) {
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(json.toUtf8(), &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isArray()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored account scopes are invalid"));
  }
  QStringList scopes;
  const QJsonArray array = document.array();
  if (array.size() > kMaximumScopeCount) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored account scope count is invalid"));
  }
  scopes.reserve(array.size());
  for (const auto& value : array) {
    if (!value.isString()) {
      return AppError(AppErrorCode::Database, QStringLiteral("Stored account scope is invalid"));
    }
    scopes.append(value.toString());
  }
  return normalizeScopes(scopes);
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite account binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
bindOptionalText(sqlite3_stmt* statement, int index, const std::optional<QString>& value) {
  if (!value.has_value()) {
    const int result = sqlite3_bind_null(statement, index);
    if (result != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite account binding failed (%1)"), result);
    }
    return std::nullopt;
  }
  return bindText(statement, index, *value);
}

[[nodiscard]] std::optional<QString> optionalText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int byteCount = sqlite3_column_bytes(statement, index);
  if (value == nullptr || byteCount < 0) {
    return std::nullopt;
  }
  return QString::fromUtf8(value, byteCount);
}

[[nodiscard]] std::optional<QString> requiredText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) != SQLITE_TEXT) {
    return std::nullopt;
  }
  return optionalText(statement, index);
}

[[nodiscard]] AccountStatusValidationResult canonicalize(AccountStatusInput input,
                                                         const QString& updatedAt) {
  if (!isValidRequiredText(input.accountId, kMaximumAccountIdLength) ||
      !isValidOptionalText(input.providerAccountId, kMaximumAccountIdLength) ||
      !isValidOptionalText(input.email, kMaximumEmailLength) ||
      !isValidOptionalText(input.displayName, kMaximumDisplayNameLength) ||
      !isValidOptionalText(input.avatarUrl, kMaximumAvatarUrlLength) ||
      !isValidOptionalText(input.locale, kMaximumLocaleLength) ||
      !isValidOptionalText(input.timeZone, kMaximumTimeZoneLength) ||
      !isValidOptionalText(input.lastAuthenticatedAt, kMaximumTimestampLength)) {
    return validationError(QStringLiteral("Account status text input is invalid"));
  }
  const QStringList grantedScopes = normalizeScopes(input.grantedScopes);
  const QString grantedScopesJson = scopesJson(grantedScopes);
  if (grantedScopes.size() > kMaximumScopeCount ||
      grantedScopesJson.toUtf8().size() > kMaximumScopeJsonBytes ||
      std::any_of(grantedScopes.cbegin(), grantedScopes.cend(), [](const QString& scope) {
        return scope.contains(QChar::Null);
      })) {
    return validationError(QStringLiteral("Account status scopes are invalid"));
  }

  const QStringList missingScopes = missingRequiredScopes(grantedScopes);
  const AccountConnectionState connectionState =
      input.connectionState == AccountConnectionState::Connected && !missingScopes.isEmpty()
          ? AccountConnectionState::ReauthRequired
          : input.connectionState;
  return AccountStatus{.accountId = std::move(input.accountId),
                       .providerAccountId = std::move(input.providerAccountId),
                       .email = std::move(input.email),
                       .displayName = std::move(input.displayName),
                       .avatarUrl = std::move(input.avatarUrl),
                       .locale = std::move(input.locale),
                       .timeZone = std::move(input.timeZone),
                       .connectionState = connectionState,
                       .grantedScopes = grantedScopes,
                       .missingScopes = missingScopes,
                       .lastAuthenticatedAt = std::move(input.lastAuthenticatedAt),
                       .updatedAt = updatedAt};
}

[[nodiscard]] AccountDecodeResult decodeAccount(sqlite3_stmt* statement) {
  const std::optional<QString> accountId = requiredText(statement, 0);
  const std::optional<QString> connectionStateValue = requiredText(statement, 7);
  const std::optional<QString> scopesJsonValue = requiredText(statement, 8);
  const std::optional<QString> updatedAt = requiredText(statement, 10);
  if (!accountId.has_value() || !connectionStateValue.has_value() || !scopesJsonValue.has_value() ||
      !updatedAt.has_value()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored account row is invalid"));
  }
  const std::optional<AccountConnectionState> connectionState =
      connectionStateFromText(*connectionStateValue);
  if (!connectionState.has_value()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored account state is invalid"));
  }
  const std::variant<QStringList, AppError> parsedScopes = parseScopesJson(*scopesJsonValue);
  if (std::holds_alternative<AppError>(parsedScopes)) {
    return std::get<AppError>(parsedScopes);
  }
  const QStringList grantedScopes = std::get<QStringList>(parsedScopes);
  const QStringList missingScopes = missingRequiredScopes(grantedScopes);
  return AccountStatus{.accountId = *accountId,
                       .providerAccountId = optionalText(statement, 1),
                       .email = optionalText(statement, 2),
                       .displayName = optionalText(statement, 3),
                       .avatarUrl = optionalText(statement, 4),
                       .locale = optionalText(statement, 5),
                       .timeZone = optionalText(statement, 6),
                       .connectionState = *connectionState == AccountConnectionState::Connected &&
                                                  !missingScopes.isEmpty()
                                              ? AccountConnectionState::ReauthRequired
                                              : *connectionState,
                       .grantedScopes = grantedScopes,
                       .missingScopes = missingScopes,
                       .lastAuthenticatedAt = optionalText(statement, 9),
                       .updatedAt = *updatedAt};
}

constexpr char accountProjectionSql[] = R"(
SELECT id, provider_account_id, email, display_name, avatar_url, locale, time_zone,
       connection_state, granted_scopes_json, last_authenticated_at, updated_at
FROM local_accounts
WHERE deleted_at IS NULL
)";

[[nodiscard]] AccountStatusLookupResult readStoredAccount(SqliteConnection& connection,
                                                          const QString& accountId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite account connection is unavailable"));
  }
  const QByteArray sql = QString::fromLatin1(accountProjectionSql)
                             .append(QStringLiteral(" AND id = ?1 LIMIT 1"))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite account read preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, accountId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    if (finalizeResult != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite account read finalization failed (%1)"),
                           finalizeResult);
    }
    return std::optional<AccountStatus>{};
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite account read failed (%1)"), stepResult);
  }
  const AccountDecodeResult decoded = decodeAccount(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (std::holds_alternative<AppError>(decoded)) {
    return std::get<AppError>(decoded);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite account read finalization failed (%1)"),
                         finalizeResult);
  }
  return std::optional<AccountStatus>(std::get<AccountStatus>(decoded));
}

[[nodiscard]] AccountStatusLookupResult readLatestStoredAccount(SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite account connection is unavailable"));
  }
  const QByteArray sql = QString::fromLatin1(accountProjectionSql)
                             .append(QStringLiteral(" ORDER BY updated_at DESC, id ASC LIMIT 1"))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite account latest preparation failed (%1)"),
                         prepareResult);
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    if (finalizeResult != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite account latest finalization failed (%1)"),
                           finalizeResult);
    }
    return std::optional<AccountStatus>{};
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite account latest read failed (%1)"), stepResult);
  }
  const AccountDecodeResult decoded = decodeAccount(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (std::holds_alternative<AppError>(decoded)) {
    return std::get<AppError>(decoded);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite account latest finalization failed (%1)"),
                         finalizeResult);
  }
  return std::optional<AccountStatus>(std::get<AccountStatus>(decoded));
}

[[nodiscard]] AccountStatusListResult readStoredAccounts(SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite account connection is unavailable"));
  }
  const QByteArray sql =
      QString::fromLatin1(accountProjectionSql)
          .append(QStringLiteral(
              " ORDER BY connection_state = 'connected' DESC, updated_at DESC, id ASC"))
          .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite account list preparation failed (%1)"),
                         prepareResult);
  }
  QList<AccountStatus> accounts;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite account list read failed (%1)"), stepResult);
    }
    const AccountDecodeResult decoded = decodeAccount(statement);
    if (std::holds_alternative<AppError>(decoded)) {
      sqlite3_finalize(statement);
      return std::get<AppError>(decoded);
    }
    accounts.append(std::get<AccountStatus>(decoded));
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite account list finalization failed (%1)"),
                         finalizeResult);
  }
  return accounts;
}

[[nodiscard]] AccountStatusSaveResultOrError writeStoredAccount(SqliteConnection& connection,
                                                                const AccountStatus& account) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite account connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle,
      "INSERT INTO local_accounts (id, provider, provider_account_id, email, display_name, "
      "avatar_url, "
      "locale, time_zone, connection_state, granted_scopes_json, missing_scopes_json, "
      "last_authenticated_at, updated_at) "
      "VALUES (?1, 'google', ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12) "
      "ON CONFLICT(id) DO UPDATE SET "
      "provider = excluded.provider, provider_account_id = excluded.provider_account_id, "
      "email = excluded.email, display_name = excluded.display_name, avatar_url = "
      "excluded.avatar_url, "
      "locale = excluded.locale, time_zone = excluded.time_zone, "
      "connection_state = excluded.connection_state, granted_scopes_json = "
      "excluded.granted_scopes_json, "
      "missing_scopes_json = excluded.missing_scopes_json, "
      "last_authenticated_at = excluded.last_authenticated_at, updated_at = excluded.updated_at, "
      "deleted_at = NULL "
      "WHERE local_accounts.provider IS NOT excluded.provider "
      "OR local_accounts.provider_account_id IS NOT excluded.provider_account_id "
      "OR local_accounts.email IS NOT excluded.email "
      "OR local_accounts.display_name IS NOT excluded.display_name "
      "OR local_accounts.avatar_url IS NOT excluded.avatar_url "
      "OR local_accounts.locale IS NOT excluded.locale "
      "OR local_accounts.time_zone IS NOT excluded.time_zone "
      "OR local_accounts.connection_state IS NOT excluded.connection_state "
      "OR local_accounts.granted_scopes_json IS NOT excluded.granted_scopes_json "
      "OR local_accounts.missing_scopes_json IS NOT excluded.missing_scopes_json "
      "OR local_accounts.last_authenticated_at IS NOT excluded.last_authenticated_at "
      "OR local_accounts.deleted_at IS NOT NULL",
      -1,
      SQLITE_PREPARE_PERSISTENT,
      &statement,
      nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite account write preparation failed (%1)"),
                         prepareResult);
  }
  const QString connectionState = connectionStateText(account.connectionState);
  const QString grantedScopesJson = scopesJson(account.grantedScopes);
  const QString missingScopesJson = scopesJson(account.missingScopes);
  std::optional<AppError> error;
  const auto bindRequired = [&](int index, const QString& value) {
    error = bindText(statement, index, value);
    return !error.has_value();
  };
  const auto bindOptional = [&](int index, const std::optional<QString>& value) {
    error = bindOptionalText(statement, index, value);
    return !error.has_value();
  };
  if (!bindRequired(1, account.accountId) || !bindOptional(2, account.providerAccountId) ||
      !bindOptional(3, account.email) || !bindOptional(4, account.displayName) ||
      !bindOptional(5, account.avatarUrl) || !bindOptional(6, account.locale) ||
      !bindOptional(7, account.timeZone) || !bindRequired(8, connectionState) ||
      !bindRequired(9, grantedScopesJson) || !bindRequired(10, missingScopesJson) ||
      !bindOptional(11, account.lastAuthenticatedAt) || !bindRequired(12, account.updatedAt)) {
    sqlite3_finalize(statement);
    if (error.has_value()) {
      return *error;
    }
    return AppError(AppErrorCode::Database, QStringLiteral("SQLite account binding failed"));
  }
  const int stepResult = sqlite3_step(statement);
  const int changes = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite account write failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite account write finalization failed (%1)"),
                         finalizeResult);
  }
  const AccountStatusLookupResult saved = readStoredAccount(connection, account.accountId);
  if (std::holds_alternative<AppError>(saved)) {
    return std::get<AppError>(saved);
  }
  const std::optional<AccountStatus>& status = std::get<std::optional<AccountStatus>>(saved);
  if (!status.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite account write was not persisted"));
  }
  return AccountStatusSaveResult{.status = *status,
                                 .mutation = changes == 0 ? AccountStatusMutationResult::Unchanged
                                                          : AccountStatusMutationResult::Changed};
}

[[nodiscard]] AccountStatusInput disconnectInput(QString accountId,
                                                 const std::optional<AccountStatus>& prior) {
  AccountStatusInput input{.accountId = std::move(accountId),
                           .connectionState = AccountConnectionState::SignedOut};
  if (!prior.has_value()) {
    return input;
  }
  input.providerAccountId = prior->providerAccountId;
  input.email = prior->email;
  input.displayName = prior->displayName;
  input.avatarUrl = prior->avatarUrl;
  input.locale = prior->locale;
  input.timeZone = prior->timeZone;
  return input;
}

} // namespace

AccountStatusService::AccountStatusService(FilePath databasePath, const Clock& clock)
    : clock_(clock), writerQueue_(std::move(databasePath)),
      initialization_(writerQueue_
                          .enqueue([](SqliteConnection& connection) -> SqliteWriteResult {
                            const SqliteMigrationRunResultOrError result =
                                LocalSchema::initialize(connection);
                            return std::holds_alternative<AppError>(result)
                                       ? std::optional<AppError>(std::get<AppError>(result))
                                       : std::nullopt;
                          })
                          .share()) {}

std::shared_future<SqliteWriteResult> AccountStatusService::ready() const {
  return initialization_;
}

std::future<AccountStatusLookupResult> AccountStatusService::find(QString accountId) {
  if (!isValidRequiredText(accountId, kMaximumAccountIdLength)) {
    return readyFuture(AccountStatusLookupResult(
        validationError(QStringLiteral("Account identifier is invalid"))));
  }
  return writerQueue_.enqueueResult(
      [accountId = std::move(accountId)](SqliteConnection& connection) {
        return readStoredAccount(connection, accountId);
      });
}

std::future<AccountStatusLookupResult> AccountStatusService::latest() {
  return writerQueue_.enqueueResult(
      [](SqliteConnection& connection) { return readLatestStoredAccount(connection); });
}

std::future<AccountStatusListResult> AccountStatusService::list() {
  return writerQueue_.enqueueResult(
      [](SqliteConnection& connection) { return readStoredAccounts(connection); });
}

std::future<AccountStatusSaveResultOrError> AccountStatusService::upsert(AccountStatusInput input) {
  const AccountStatusValidationResult canonical = canonicalize(std::move(input), timestamp(clock_));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(AccountStatusSaveResultOrError(std::get<AppError>(canonical)));
  }
  return writerQueue_.enqueueResult(
      [account = std::get<AccountStatus>(canonical)](SqliteConnection& connection) {
        return writeStoredAccount(connection, account);
      });
}

std::future<AccountStatusSaveResultOrError> AccountStatusService::disconnect(QString accountId) {
  if (!isValidRequiredText(accountId, kMaximumAccountIdLength)) {
    return readyFuture(AccountStatusSaveResultOrError(
        validationError(QStringLiteral("Account identifier is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [accountId = std::move(accountId), updatedAt](SqliteConnection& connection) {
        const AccountStatusLookupResult prior = readStoredAccount(connection, accountId);
        if (std::holds_alternative<AppError>(prior)) {
          return AccountStatusSaveResultOrError(std::get<AppError>(prior));
        }
        const AccountStatusValidationResult canonical = canonicalize(
            disconnectInput(accountId, std::get<std::optional<AccountStatus>>(prior)), updatedAt);
        if (std::holds_alternative<AppError>(canonical)) {
          return AccountStatusSaveResultOrError(std::get<AppError>(canonical));
        }
        return writeStoredAccount(connection, std::get<AccountStatus>(canonical));
      });
}

} // namespace hcb
