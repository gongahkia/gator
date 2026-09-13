#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QString>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

enum class MutationTelemetryPhase : std::uint8_t { Intent, RemoteApplied, RemoteFailed, Rollback };

struct MutationTelemetryInput final {
  QString mutationId;
  QString resource;
  QString operation;
  QString scope;
  bool allDay{false};
  std::optional<QString> targetStartAt;
  std::optional<QString> targetEndAt;
  MutationTelemetryPhase phase{MutationTelemetryPhase::Intent};
  std::optional<QString> remoteOutcome;
  std::optional<QString> errorCode;
  std::optional<QString> rollbackReason;
};

struct MutationTelemetryRecord final {
  QString id;
  QString mutationId;
  QString resource;
  QString operation;
  QString scope;
  bool allDay{false};
  std::optional<QString> targetStartAt;
  std::optional<QString> targetEndAt;
  MutationTelemetryPhase phase{MutationTelemetryPhase::Intent};
  std::optional<QString> remoteOutcome;
  std::optional<QString> errorCode;
  std::optional<QString> rollbackReason;
  QString createdAt;
};

using MutationTelemetryWriteResult = std::variant<MutationTelemetryRecord, AppError>;
using MutationTelemetryReadResult = std::variant<QList<MutationTelemetryRecord>, AppError>;

class MutationTelemetryStore final {
public:
  static constexpr int maximumRecords = 500;

  MutationTelemetryStore(FilePath databasePath, const Clock& clock);
  MutationTelemetryStore(const MutationTelemetryStore&) = delete;
  MutationTelemetryStore& operator=(const MutationTelemetryStore&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<MutationTelemetryWriteResult> record(MutationTelemetryInput input);
  [[nodiscard]] std::future<MutationTelemetryReadResult> recent(int limit = maximumRecords);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
