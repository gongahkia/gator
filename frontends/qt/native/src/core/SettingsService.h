#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

enum class SettingsMutationResult : std::uint8_t {
  Changed,
  Unchanged
};

using SettingsJsonReadResult = std::variant<std::optional<QString>, AppError>;
using SettingsMutationResultOrError = std::variant<SettingsMutationResult, AppError>;

class SettingsService final {
public:
  SettingsService(FilePath databasePath, const Clock& clock);
  SettingsService(const SettingsService&) = delete;
  SettingsService& operator=(const SettingsService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<SettingsJsonReadResult> readJson(QString scope, QString key);
  [[nodiscard]] std::future<SettingsMutationResultOrError>
  writeJson(QString scope, QString key, QString valueJson);
  [[nodiscard]] std::future<SettingsMutationResultOrError> erase(QString scope, QString key);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
