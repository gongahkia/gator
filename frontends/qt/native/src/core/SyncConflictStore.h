#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "core/SyncConflictPolicy.h"
#include "data/SqliteWriterQueue.h"

#include <QJsonObject>
#include <QList>
#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

enum class SyncConflictResource : std::uint8_t {
  Task,
  TaskList,
  Event
};

enum class SyncConflictResolution : std::uint8_t {
  KeepLocal,
  KeepRemote
};

struct SyncConflictInput final {
  std::optional<QString> accountId;
  SyncConflictResource resource{SyncConflictResource::Task};
  QString resourceId;
  QString mutationId;
  QString errorCode;
  QString errorMessage;
  QJsonObject baseSnapshot;
  QJsonObject localPayload;
  QJsonObject remoteSnapshot;
  std::optional<QString> remoteEtag;
  SyncConflictPolicy policy{SyncConflictPolicy::PreferGoogle};
};

struct SyncConflict final {
  QString id;
  std::optional<QString> accountId;
  SyncConflictResource resource{SyncConflictResource::Task};
  QString resourceId;
  QString mutationId;
  QString errorCode;
  QString errorMessage;
  QJsonObject baseSnapshot;
  QJsonObject localPayload;
  QJsonObject remoteSnapshot;
  std::optional<QString> remoteEtag;
  SyncConflictPolicy policy{SyncConflictPolicy::PreferGoogle};
  QString createdAt;
  QString updatedAt;
  std::optional<SyncConflictResolution> resolution;
  std::optional<QString> resolvedAt;
};

using SyncConflictResult = std::variant<SyncConflict, AppError>;
using SyncConflictListResult = std::variant<QList<SyncConflict>, AppError>;

class SyncConflictStore final {
public:
  SyncConflictStore(FilePath databasePath, const Clock& clock);
  SyncConflictStore(const SyncConflictStore&) = delete;
  SyncConflictStore& operator=(const SyncConflictStore&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<SyncConflictResult> record(SyncConflictInput input);
  [[nodiscard]] std::future<SyncConflictListResult> listUnresolved(int limit = 50);
  [[nodiscard]] std::future<SyncConflictListResult> listResolved(int limit = 20);
  [[nodiscard]] std::future<SyncConflictResult> resolve(QString conflictId,
                                                        SyncConflictResolution resolution);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
