#include "core/GoogleApiError.h"

#include "core/SecretRedactor.h"

namespace hcb {
namespace {

[[nodiscard]] bool isQuotaExceededBody(QStringView body) {
  const QString normalizedBody = body.toString();
  return normalizedBody.contains(QStringLiteral("quotaexceeded"), Qt::CaseInsensitive) ||
         normalizedBody.contains(QStringLiteral("dailylimitexceeded"), Qt::CaseInsensitive) ||
         normalizedBody.contains(QStringLiteral("usage limits"), Qt::CaseInsensitive) ||
         normalizedBody.contains(QStringLiteral("quota exceeded"), Qt::CaseInsensitive) ||
         normalizedBody.contains(QStringLiteral("daily limit"), Qt::CaseInsensitive);
}

[[nodiscard]] GoogleApiErrorKind kindForStatus(int status, bool quotaExceeded) noexcept {
  if (status == 401) {
    return GoogleApiErrorKind::Unauthorized;
  }
  if (status == 403) {
    return GoogleApiErrorKind::Forbidden;
  }
  if (status == 404) {
    return GoogleApiErrorKind::NotFound;
  }
  if (status == 409) {
    return GoogleApiErrorKind::Conflict;
  }
  if (status == 410) {
    return GoogleApiErrorKind::InvalidSyncToken;
  }
  if (status == 412) {
    return GoogleApiErrorKind::PreconditionFailed;
  }
  if (status == 429 || quotaExceeded) {
    return GoogleApiErrorKind::RateLimited;
  }
  if (status >= 500 && status <= 599) {
    return GoogleApiErrorKind::Server;
  }
  if (status == 400) {
    return GoogleApiErrorKind::InvalidPayload;
  }
  return GoogleApiErrorKind::Transport;
}

[[nodiscard]] QString messageForStatus(int status, GoogleApiErrorKind kind) {
  switch (kind) {
  case GoogleApiErrorKind::Unauthorized:
    return QStringLiteral("Google account reauthentication is required");
  case GoogleApiErrorKind::Forbidden:
    return QStringLiteral("Google denied access to the requested resource");
  case GoogleApiErrorKind::NotFound:
    return QStringLiteral("Google resource was not found");
  case GoogleApiErrorKind::Conflict:
    return QStringLiteral("Google resource changed before the operation completed");
  case GoogleApiErrorKind::PreconditionFailed:
    return QStringLiteral("Google resource precondition failed");
  case GoogleApiErrorKind::InvalidSyncToken:
    return QStringLiteral("Google sync token is invalid and requires a full resync");
  case GoogleApiErrorKind::RateLimited:
    return QStringLiteral("Google rate limit was reached");
  case GoogleApiErrorKind::Server:
    return QStringLiteral("Google service is temporarily unavailable");
  case GoogleApiErrorKind::InvalidPayload:
    return QStringLiteral("Google rejected the request payload");
  case GoogleApiErrorKind::Transport:
    return QStringLiteral("Google request failed with status %1").arg(status);
  }
  return QStringLiteral("Google request failed");
}

} // namespace

GoogleApiError::GoogleApiError(const GoogleApiErrorOptions& options)
    : kind_(options.kind), message_(SecretRedactor::redactText(options.message)),
      status_(options.status), retryAfterMilliseconds_(options.retryAfterMilliseconds),
      responseBodyBytes_(options.responseBodyBytes), quotaExceeded_(options.quotaExceeded) {}

GoogleApiError GoogleApiError::fromHttpStatus(int status,
                                              QStringView body,
                                              std::optional<qint64> retryAfterMilliseconds) {
  const bool quotaExceeded = isQuotaExceededBody(body);
  const GoogleApiErrorKind kind = kindForStatus(status, quotaExceeded);
  GoogleApiErrorOptions options;
  options.kind = kind;
  options.message = messageForStatus(status, kind);
  options.status = status;
  options.retryAfterMilliseconds = retryAfterMilliseconds;
  options.responseBodyBytes = body.toString().toUtf8().size();
  options.quotaExceeded = quotaExceeded;
  return GoogleApiError(options);
}

GoogleApiErrorKind GoogleApiError::kind() const noexcept { return kind_; }

const QString& GoogleApiError::message() const noexcept { return message_; }

const std::optional<int>& GoogleApiError::status() const noexcept { return status_; }

const std::optional<qint64>& GoogleApiError::retryAfterMilliseconds() const noexcept {
  return retryAfterMilliseconds_;
}

const std::optional<qsizetype>& GoogleApiError::responseBodyBytes() const noexcept {
  return responseBodyBytes_;
}

bool GoogleApiError::quotaExceeded() const noexcept { return quotaExceeded_; }

} // namespace hcb
