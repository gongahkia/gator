#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QJsonValue>
#include <QString>

#include <cstdint>
#include <future>
#include <mutex>
#include <optional>
#include <variant>

namespace hcb {

enum class UndoResourceKind : std::uint8_t {
  Task,
  TaskList,
  Event
};

enum class UndoAction : std::uint8_t {
  Undo,
  Redo
};

struct UndoHistoryConfiguration final {
  int retentionDays{30};
  int maximumEntries{200};
};

struct UndoChangeInput final {
  QString actionKind;
  QString label;
  UndoResourceKind resource{UndoResourceKind::Task};
  QString resourceId;
  QJsonValue before;
  QJsonValue after;
};

struct UndoStatus final {
  std::optional<QString> undoLabel;
  std::optional<QString> redoLabel;
};

struct UndoReplay final {
  UndoAction action{UndoAction::Undo};
  QString label;
  UndoResourceKind resource{UndoResourceKind::Task};
  QString resourceId;
  QJsonValue target;
};

struct UndoEntry final {
  UndoAction action{UndoAction::Undo};
  QString actionKind;
  QString label;
  UndoResourceKind resource{UndoResourceKind::Task};
  QString resourceId;
  QJsonValue expected;
  QJsonValue target;
};

struct UndoRecoveryReport final {
  UndoStatus status;
  int discardedEntries{0};
};

using UndoStatusResult = std::variant<UndoStatus, AppError>;
using UndoReplayResult = std::variant<UndoReplay, AppError>;
using UndoEntryResult = std::variant<UndoEntry, AppError>;
using UndoRecoveryResult = std::variant<UndoRecoveryReport, AppError>;

class UndoRecoveryPolicy final {
public:
  UndoRecoveryPolicy(FilePath databasePath, const Clock& clock, QString sessionId = {});
  UndoRecoveryPolicy(const UndoRecoveryPolicy&) = delete;
  UndoRecoveryPolicy& operator=(const UndoRecoveryPolicy&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] const QString& sessionId() const noexcept;
  void configure(UndoHistoryConfiguration configuration);
  [[nodiscard]] UndoHistoryConfiguration configuration() const;
  [[nodiscard]] std::future<UndoStatusResult> status();
  [[nodiscard]] std::future<UndoEntryResult> nextUndo();
  [[nodiscard]] std::future<UndoEntryResult> nextRedo();
  [[nodiscard]] std::future<std::optional<AppError>> record(UndoChangeInput input);
  [[nodiscard]] std::future<UndoReplayResult> undo(QJsonValue currentSnapshot);
  [[nodiscard]] std::future<UndoReplayResult> redo(QJsonValue currentSnapshot);
  [[nodiscard]] std::future<UndoRecoveryResult> recover();

private:
  const Clock& clock_;
  QString sessionId_;
  mutable std::mutex configurationMutex_;
  UndoHistoryConfiguration configuration_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
