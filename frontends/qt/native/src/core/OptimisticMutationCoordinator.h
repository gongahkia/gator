#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QJsonObject>
#include <QList>
#include <QString>

#include <chrono>
#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

enum class PendingMutationResource : std::uint8_t {
  Task,
  TaskList,
  Event
};

enum class PendingMutationStatus : std::uint8_t {
  Pending,
  Applying,
  Failed,
  Applied,
  Cancelled
};

struct OptimisticMutationInput final {
  std::optional<QString> accountId;
  PendingMutationResource resource{PendingMutationResource::Task};
  QString resourceId;
  QString operation;
  QJsonObject payload;
  QJsonObject baseSnapshot;
  std::optional<QString> remoteEtag;
};

struct PendingMutation final {
  QString id;
  std::optional<QString> accountId;
  PendingMutationResource resource{PendingMutationResource::Task};
  QString resourceId;
  QString operation;
  QJsonObject payload;
  QJsonObject baseSnapshot;
  std::optional<QString> remoteEtag;
  PendingMutationStatus status{PendingMutationStatus::Pending};
  int attemptCount{0};
  std::optional<QString> nextRetryAt;
  std::optional<QString> leaseId;
  std::optional<QString> leaseExpiresAt;
  std::optional<QString> lastErrorCode;
  std::optional<QString> lastErrorMessage;
  QString createdAt;
  QString updatedAt;
  std::optional<QString> appliedAt;
};

struct MutationFailureInput final {
  QString mutationId;
  QString leaseId;
  QString errorCode;
  QString errorMessage;
  std::optional<QString> nextRetryAt;
};

struct MutationRebaseInput final {
  QString mutationId;
  std::optional<QString> leaseId;
  QJsonObject payload;
  QJsonObject baseSnapshot;
  std::optional<QString> remoteEtag;
};

using PendingMutationLookupResult = std::variant<std::optional<PendingMutation>, AppError>;
using PendingMutationListResult = std::variant<QList<PendingMutation>, AppError>;
using PendingMutationResult = std::variant<PendingMutation, AppError>;

class OptimisticMutationCoordinator final {
public:
  OptimisticMutationCoordinator(FilePath databasePath, const Clock& clock);
  OptimisticMutationCoordinator(const OptimisticMutationCoordinator&) = delete;
  OptimisticMutationCoordinator& operator=(const OptimisticMutationCoordinator&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<PendingMutationResult> enqueue(OptimisticMutationInput input);
  [[nodiscard]] std::future<PendingMutationLookupResult> find(QString mutationId);
  [[nodiscard]] std::future<PendingMutationListResult> listDue(int limit = 25);
  [[nodiscard]] std::future<PendingMutationListResult> listActive(int limit = 1'000);
  [[nodiscard]] std::future<PendingMutationResult> claim(QString mutationId,
                                                         std::chrono::seconds leaseDuration);
  [[nodiscard]] std::future<PendingMutationResult> markApplied(QString mutationId, QString leaseId);
  [[nodiscard]] std::future<PendingMutationResult> markFailed(MutationFailureInput input);
  [[nodiscard]] std::future<PendingMutationResult> rebase(MutationRebaseInput input);
  [[nodiscard]] std::future<PendingMutationResult> retry(QString mutationId);
  [[nodiscard]] std::future<PendingMutationResult> cancel(QString mutationId);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
