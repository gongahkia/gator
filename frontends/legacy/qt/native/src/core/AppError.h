#pragma once

#include <QString>

#include <utility>

namespace hcb {

enum class AppErrorCode {
  Cancelled,
  Configuration,
  Database,
  Network,
  Validation
};

class AppError final {
public:
  AppError(AppErrorCode code, QString message) : code_(code), message_(std::move(message)) {}

  [[nodiscard]] AppErrorCode code() const noexcept { return code_; }
  [[nodiscard]] const QString& message() const noexcept { return message_; }

private:
  AppErrorCode code_;
  QString message_;
};

} // namespace hcb
