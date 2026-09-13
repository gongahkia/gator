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

struct OAuthClientConfiguration final {
  QString clientId;
  QString clientSecret;
  QString updatedAt;
};

enum class OAuthClientConfigurationMutationResult : std::uint8_t {
  Changed,
  Unchanged
};

using OAuthClientConfigurationReadResult =
    std::variant<std::optional<OAuthClientConfiguration>, AppError>;
using OAuthClientConfigurationMutationResultOrError =
    std::variant<OAuthClientConfigurationMutationResult, AppError>;

class OAuthClientConfigurationStore final {
public:
  OAuthClientConfigurationStore(FilePath databasePath, const Clock& clock);
  OAuthClientConfigurationStore(const OAuthClientConfigurationStore&) = delete;
  OAuthClientConfigurationStore& operator=(const OAuthClientConfigurationStore&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<OAuthClientConfigurationReadResult> load();
  [[nodiscard]] std::future<OAuthClientConfigurationMutationResultOrError>
  save(QString clientId, QString clientSecret = {});
  [[nodiscard]] std::future<OAuthClientConfigurationMutationResultOrError> clear();

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
