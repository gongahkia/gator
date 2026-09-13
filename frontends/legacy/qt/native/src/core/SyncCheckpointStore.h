#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QJsonObject>
#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

enum class SyncCheckpointResourceType : std::uint8_t {
  TaskListWatermark,
  CalendarList,
  CalendarEvent
};

struct SyncCheckpointKey final {
  QString accountId;
  SyncCheckpointResourceType resourceType;
  QString resourceId;
};

struct SyncCheckpoint final {
  SyncCheckpointKey key;
  QString syncToken;
  QJsonObject metadata;
  QString lastSuccessfulSyncAt;
  QString updatedAt;
};

using SyncCheckpointLookupResult = std::variant<std::optional<SyncCheckpoint>, AppError>;
using SyncCheckpointSaveResult = std::variant<SyncCheckpoint, AppError>;
using SyncCheckpointEraseResult = std::variant<bool, AppError>;

class SyncCheckpointStore final {
public:
  SyncCheckpointStore(FilePath databasePath, const Clock& clock);
  SyncCheckpointStore(const SyncCheckpointStore&) = delete;
  SyncCheckpointStore& operator=(const SyncCheckpointStore&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<SyncCheckpointLookupResult> find(SyncCheckpointKey key);
  [[nodiscard]] std::future<SyncCheckpointSaveResult> save(SyncCheckpointKey key,
                                                           const QString& syncToken,
                                                           QJsonObject metadata = {});
  [[nodiscard]] std::future<SyncCheckpointEraseResult> erase(SyncCheckpointKey key);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
