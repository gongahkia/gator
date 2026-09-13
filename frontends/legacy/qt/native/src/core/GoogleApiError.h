#pragma once

#include <QString>
#include <QStringView>

#include <optional>

namespace hcb {

enum class GoogleApiErrorKind {
  Unauthorized,
  Forbidden,
  NotFound,
  Conflict,
  PreconditionFailed,
  InvalidSyncToken,
  RateLimited,
  Server,
  InvalidPayload,
  Transport
};

struct GoogleApiErrorOptions {
  GoogleApiErrorKind kind{GoogleApiErrorKind::Transport};
  QString message;
  std::optional<int> status;
  std::optional<qint64> retryAfterMilliseconds;
  std::optional<qsizetype> responseBodyBytes;
  bool quotaExceeded{false};
};

class GoogleApiError final {
public:
  explicit GoogleApiError(const GoogleApiErrorOptions& options);

  [[nodiscard]] static GoogleApiError fromHttpStatus(
      int status, QStringView body, std::optional<qint64> retryAfterMilliseconds = std::nullopt);

  [[nodiscard]] GoogleApiErrorKind kind() const noexcept;
  [[nodiscard]] const QString& message() const noexcept;
  [[nodiscard]] const std::optional<int>& status() const noexcept;
  [[nodiscard]] const std::optional<qint64>& retryAfterMilliseconds() const noexcept;
  [[nodiscard]] const std::optional<qsizetype>& responseBodyBytes() const noexcept;
  [[nodiscard]] bool quotaExceeded() const noexcept;

private:
  GoogleApiErrorKind kind_;
  QString message_;
  std::optional<int> status_;
  std::optional<qint64> retryAfterMilliseconds_;
  std::optional<qsizetype> responseBodyBytes_;
  bool quotaExceeded_;
};

} // namespace hcb
