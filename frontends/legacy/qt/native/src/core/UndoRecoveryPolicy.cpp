#include "core/UndoRecoveryPolicy.h"

#include "data/LocalSchema.h"
#include "data/SqliteTransaction.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDateTime>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonParseError>
#include <QString>
#include <QTimeZone>
#include <QUuid>

#include <chrono>
#include <algorithm>
#include <future>
#include <initializer_list>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumActionKindLength = 128;
constexpr qsizetype kMaximumLabelLength = 512;
constexpr qsizetype kMaximumSnapshotBytes = 262'144;
constexpr int kMinimumRetentionDays = 1;
constexpr int kMaximumRetentionDays = 3650;
constexpr int kMinimumEntries = 50;
constexpr int kMaximumEntries = 1000;

enum class UndoStack : std::uint8_t {
  Undo,
  Redo
};

struct StoredUndoEntry final {
  QString id;
  QString actionKind;
  QString label;
  UndoResourceKind resource{UndoResourceKind::Task};
  QString resourceId;
  QJsonValue before;
  QJsonValue after;
};

using StoredEntryLookupResult = std::variant<std::optional<StoredUndoEntry>, AppError>;

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] bool isValidRequiredText(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximumLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] QString timestampAt(WallTimePoint point) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(point.time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] QString timestamp(const Clock& clock) { return timestampAt(clock.wallNow()); }

[[nodiscard]] QString stackText(UndoStack stack) {
  return stack == UndoStack::Undo ? QStringLiteral("undo") : QStringLiteral("redo");
}

[[nodiscard]] QString resourceText(UndoResourceKind resource) {
  switch (resource) {
  case UndoResourceKind::Task:
    return QStringLiteral("task");
  case UndoResourceKind::TaskList:
    return QStringLiteral("task_list");
  case UndoResourceKind::Event:
    return QStringLiteral("event");
  }
  return {};
}

[[nodiscard]] std::optional<UndoResourceKind> resourceFromText(const QString& value) {
  if (value == QStringLiteral("task")) {
    return UndoResourceKind::Task;
  }
  if (value == QStringLiteral("task_list")) {
    return UndoResourceKind::TaskList;
  }
  if (value == QStringLiteral("event")) {
    return UndoResourceKind::Event;
  }
  return std::nullopt;
}

[[nodiscard]] QByteArray snapshotBytes(const QJsonValue& snapshot) {
  return QJsonDocument(QJsonObject{{QStringLiteral("snapshot"), snapshot}})
      .toJson(QJsonDocument::Compact);
}

[[nodiscard]] std::optional<QJsonValue> snapshotFromBytes(const QString& value) {
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(value.toUtf8(), &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return std::nullopt;
  }
  const QJsonObject object = document.object();
  return object.contains(QStringLiteral("snapshot"))
             ? std::optional<QJsonValue>(object.value(QStringLiteral("snapshot")))
             : std::nullopt;
}

[[nodiscard]] bool isValidSnapshot(const QJsonValue& snapshot) {
  return !snapshot.isUndefined() && snapshotBytes(snapshot).size() <= kMaximumSnapshotBytes;
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  return result == SQLITE_OK ? std::nullopt
                             : std::optional<AppError>(databaseError(
                                   QStringLiteral("SQLite undo binding failed (%1)"), result));
}

[[nodiscard]] std::optional<AppError>
bindAll(sqlite3_stmt* statement, const std::initializer_list<std::optional<AppError>>& results) {
  for (const std::optional<AppError>& result : results) {
    if (result.has_value()) {
      sqlite3_finalize(statement);
      return result;
    }
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<QString> requiredText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int size = sqlite3_column_bytes(statement, index);
  return value == nullptr || size < 0 ? std::nullopt
                                      : std::optional<QString>(QString::fromUtf8(value, size));
}

[[nodiscard]] std::variant<UndoChangeInput, AppError> canonicalize(UndoChangeInput input) {
  if (!isValidRequiredText(input.actionKind, kMaximumActionKindLength) ||
      !isValidRequiredText(input.label, kMaximumLabelLength) ||
      resourceText(input.resource).isEmpty() ||
      !isValidRequiredText(input.resourceId, kMaximumIdentifierLength) ||
      !isValidSnapshot(input.before) || !isValidSnapshot(input.after)) {
    return validationError(QStringLiteral("Undo change input is invalid"));
  }
  return input;
}

[[nodiscard]] std::optional<AppError> executeDelete(sqlite3* handle,
                                                    const char* sql,
                                                    const QString& first,
                                                    const std::optional<QString>& second,
                                                    int* deletedCount) {
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite undo cleanup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, first); error.has_value()) {
    sqlite3_finalize(statement);
    return error;
  }
  if (second.has_value()) {
    if (const std::optional<AppError> error = bindText(statement, 2, *second); error.has_value()) {
      sqlite3_finalize(statement);
      return error;
    }
  }
  const int stepResult = sqlite3_step(statement);
  const int changes = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite undo cleanup failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite undo cleanup finalization failed (%1)"),
                         finalizeResult);
  }
  *deletedCount += changes;
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError> cleanupStoredEntries(SqliteConnection& connection,
                                                           const QString& sessionId,
                                                           const QString& staleBefore,
                                                           int maximumEntries,
                                                           int* deletedCount) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite undo connection is unavailable"));
  }
  constexpr char currentSessionSql[] = R"(
DELETE FROM local_undo_entries
WHERE session_id = ?1
  AND id NOT IN (
    SELECT id
    FROM local_undo_entries
    WHERE session_id = ?1
    ORDER BY ordinal DESC, id DESC
    LIMIT ?2
  )
)";
  if (const std::optional<AppError> error =
          executeDelete(handle,
                        currentSessionSql,
                        sessionId,
                        QString::number(maximumEntries),
                        deletedCount);
      error.has_value()) {
    return error;
  }
  constexpr char staleSessionSql[] = R"(
DELETE FROM local_undo_entries
WHERE session_id <> ?1 AND created_at < ?2
)";
  if (const std::optional<AppError> error =
          executeDelete(handle, staleSessionSql, sessionId, staleBefore, deletedCount);
      error.has_value()) {
    return error;
  }
  constexpr char otherSessionSql[] = R"(
DELETE FROM local_undo_entries
WHERE session_id <> ?1
  AND id NOT IN (
    SELECT id
    FROM local_undo_entries
    WHERE session_id <> ?1
    ORDER BY updated_at DESC, ordinal DESC, id DESC
    LIMIT 1000
  )
)";
  return executeDelete(handle, otherSessionSql, sessionId, std::nullopt, deletedCount);
}

[[nodiscard]] StoredEntryLookupResult
topStoredEntry(SqliteConnection& connection, const QString& sessionId, UndoStack stack) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite undo connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT id, action_kind, label, resource_type, resource_id, before_json, after_json
FROM local_undo_entries
WHERE session_id = ?1 AND stack = ?2
ORDER BY ordinal DESC, id DESC
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite undo lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(
          statement, {bindText(statement, 1, sessionId), bindText(statement, 2, stackText(stack))});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? StoredEntryLookupResult(std::optional<StoredUndoEntry>{})
               : StoredEntryLookupResult(
                     databaseError(QStringLiteral("SQLite undo lookup finalization failed (%1)"),
                                   finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite undo lookup failed (%1)"), stepResult);
  }
  const std::optional<QString> id = requiredText(statement, 0);
  const std::optional<QString> actionKind = requiredText(statement, 1);
  const std::optional<QString> label = requiredText(statement, 2);
  const std::optional<QString> resourceType = requiredText(statement, 3);
  const std::optional<QString> resourceId = requiredText(statement, 4);
  const std::optional<QString> beforeJson = requiredText(statement, 5);
  const std::optional<QString> afterJson = requiredText(statement, 6);
  const std::optional<UndoResourceKind> resource =
      resourceType.has_value() ? resourceFromText(*resourceType) : std::nullopt;
  const std::optional<QJsonValue> before =
      beforeJson.has_value() ? snapshotFromBytes(*beforeJson) : std::nullopt;
  const std::optional<QJsonValue> after =
      afterJson.has_value() ? snapshotFromBytes(*afterJson) : std::nullopt;
  const int finalizeResult = sqlite3_finalize(statement);
  if (!id.has_value() || !actionKind.has_value() || !label.has_value() || !resource.has_value() ||
      !resourceId.has_value() || !before.has_value() || !after.has_value()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored undo entry is invalid"));
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite undo lookup finalization failed (%1)"),
                         finalizeResult);
  }
  return std::optional<StoredUndoEntry>(StoredUndoEntry{.id = *id,
                                                        .actionKind = *actionKind,
                                                        .label = *label,
                                                        .resource = *resource,
                                                        .resourceId = *resourceId,
                                                        .before = *before,
                                                        .after = *after});
}

[[nodiscard]] UndoStatusResult readStoredStatus(SqliteConnection& connection,
                                                const QString& sessionId) {
  const StoredEntryLookupResult undo = topStoredEntry(connection, sessionId, UndoStack::Undo);
  if (std::holds_alternative<AppError>(undo)) {
    return std::get<AppError>(undo);
  }
  const StoredEntryLookupResult redo = topStoredEntry(connection, sessionId, UndoStack::Redo);
  if (std::holds_alternative<AppError>(redo)) {
    return std::get<AppError>(redo);
  }
  const std::optional<StoredUndoEntry>& undoEntry = std::get<std::optional<StoredUndoEntry>>(undo);
  const std::optional<StoredUndoEntry>& redoEntry = std::get<std::optional<StoredUndoEntry>>(redo);
  return UndoStatus{
      .undoLabel = undoEntry.has_value() ? std::optional<QString>(undoEntry->label) : std::nullopt,
      .redoLabel = redoEntry.has_value() ? std::optional<QString>(redoEntry->label) : std::nullopt};
}

[[nodiscard]] std::optional<AppError> recordStoredEntry(SqliteConnection& connection,
                                                        const QString& sessionId,
                                                        const UndoChangeInput& input,
                                                        const QString& now,
                                                        const QString& staleBefore,
                                                        int maximumEntries) {
  SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
  if (std::holds_alternative<AppError>(transactionResult)) {
    return std::get<AppError>(transactionResult);
  }
  SqliteTransaction transaction = std::move(std::get<SqliteTransaction>(transactionResult));
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite undo connection is unavailable"));
  }
  constexpr char clearRedoSql[] =
      "DELETE FROM local_undo_entries WHERE session_id = ?1 AND stack = 'redo'";
  sqlite3_stmt* clearRedo = nullptr;
  const int clearPrepareResult =
      sqlite3_prepare_v3(handle, clearRedoSql, -1, SQLITE_PREPARE_PERSISTENT, &clearRedo, nullptr);
  if (clearPrepareResult != SQLITE_OK) {
    sqlite3_finalize(clearRedo);
    return databaseError(QStringLiteral("SQLite undo redo-clear preparation failed (%1)"),
                         clearPrepareResult);
  }
  if (const std::optional<AppError> error = bindText(clearRedo, 1, sessionId); error.has_value()) {
    sqlite3_finalize(clearRedo);
    return error;
  }
  const int clearStepResult = sqlite3_step(clearRedo);
  const int clearFinalizeResult = sqlite3_finalize(clearRedo);
  if (clearStepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite undo redo-clear failed (%1)"), clearStepResult);
  }
  if (clearFinalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite undo redo-clear finalization failed (%1)"),
                         clearFinalizeResult);
  }
  constexpr char insertSql[] = R"(
INSERT INTO local_undo_entries (
  id, session_id, stack, ordinal, action_kind, label, resource_type, resource_id, before_json,
  after_json, created_at, updated_at, applied_at
) VALUES (
  ?1, ?2, 'undo', COALESCE((SELECT MAX(ordinal) + 1
                             FROM local_undo_entries
                             WHERE session_id = ?2 AND stack = 'undo'), 0),
  ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?9, NULL
)
)";
  sqlite3_stmt* insert = nullptr;
  const int insertPrepareResult =
      sqlite3_prepare_v3(handle, insertSql, -1, SQLITE_PREPARE_PERSISTENT, &insert, nullptr);
  if (insertPrepareResult != SQLITE_OK) {
    sqlite3_finalize(insert);
    return databaseError(QStringLiteral("SQLite undo record preparation failed (%1)"),
                         insertPrepareResult);
  }
  const QString id =
      QStringLiteral("undo:%1").arg(QUuid::createUuid().toString(QUuid::WithoutBraces));
  const QString before = QString::fromUtf8(snapshotBytes(input.before));
  const QString after = QString::fromUtf8(snapshotBytes(input.after));
  if (const std::optional<AppError> error =
          bindAll(insert,
                  {bindText(insert, 1, id),
                   bindText(insert, 2, sessionId),
                   bindText(insert, 3, input.actionKind),
                   bindText(insert, 4, input.label),
                   bindText(insert, 5, resourceText(input.resource)),
                   bindText(insert, 6, input.resourceId),
                   bindText(insert, 7, before),
                   bindText(insert, 8, after),
                   bindText(insert, 9, now)});
      error.has_value()) {
    return error;
  }
  const int insertStepResult = sqlite3_step(insert);
  const int insertFinalizeResult = sqlite3_finalize(insert);
  if (insertStepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite undo record failed (%1)"), insertStepResult);
  }
  if (insertFinalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite undo record finalization failed (%1)"),
                         insertFinalizeResult);
  }
  int discardedEntries = 0;
  if (const std::optional<AppError> error =
          cleanupStoredEntries(connection, sessionId, staleBefore, maximumEntries, &discardedEntries);
      error.has_value()) {
    return error;
  }
  return transaction.commit();
}

[[nodiscard]] UndoReplayResult moveStoredTopEntry(SqliteConnection& connection,
                                                  const QString& sessionId,
                                                  UndoStack stack,
                                                  const QJsonValue& currentSnapshot,
                                                  const QString& now) {
  const StoredEntryLookupResult found = topStoredEntry(connection, sessionId, stack);
  if (std::holds_alternative<AppError>(found)) {
    return std::get<AppError>(found);
  }
  const std::optional<StoredUndoEntry>& entry = std::get<std::optional<StoredUndoEntry>>(found);
  if (!entry.has_value()) {
    return validationError(stack == UndoStack::Undo ? QStringLiteral("Nothing to undo")
                                                    : QStringLiteral("Nothing to redo"));
  }
  const QJsonValue& expected = stack == UndoStack::Undo ? entry->after : entry->before;
  if (snapshotBytes(currentSnapshot) != snapshotBytes(expected)) {
    return validationError(stack == UndoStack::Undo
                               ? QStringLiteral("Undo is unavailable because the item changed")
                               : QStringLiteral("Redo is unavailable because the item changed"));
  }
  SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
  if (std::holds_alternative<AppError>(transactionResult)) {
    return std::get<AppError>(transactionResult);
  }
  SqliteTransaction transaction = std::move(std::get<SqliteTransaction>(transactionResult));
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite undo connection is unavailable"));
  }
  const UndoStack nextStack = stack == UndoStack::Undo ? UndoStack::Redo : UndoStack::Undo;
  constexpr char updateSql[] = R"(
UPDATE local_undo_entries
SET stack = ?2,
    ordinal = COALESCE((SELECT MAX(ordinal) + 1
                        FROM local_undo_entries
                        WHERE session_id = ?3 AND stack = ?2), 0),
    updated_at = ?4,
    applied_at = ?4
WHERE id = ?1 AND session_id = ?3 AND stack = ?5
)";
  sqlite3_stmt* update = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, updateSql, -1, SQLITE_PREPARE_PERSISTENT, &update, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(update);
    return databaseError(QStringLiteral("SQLite undo move preparation failed (%1)"), prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(update,
                                                    {bindText(update, 1, entry->id),
                                                     bindText(update, 2, stackText(nextStack)),
                                                     bindText(update, 3, sessionId),
                                                     bindText(update, 4, now),
                                                     bindText(update, 5, stackText(stack))});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(update);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(update);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite undo move failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite undo move finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Undo entry changed before it could be applied"));
  }
  if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
    return *error;
  }
  return UndoReplay{.action = stack == UndoStack::Undo ? UndoAction::Undo : UndoAction::Redo,
                    .label = entry->label,
                    .resource = entry->resource,
                    .resourceId = entry->resourceId,
                    .target = stack == UndoStack::Undo ? entry->before : entry->after};
}

[[nodiscard]] UndoEntryResult readStoredTopEntry(SqliteConnection& connection,
                                                  const QString& sessionId,
                                                  UndoStack stack) {
  const StoredEntryLookupResult found = topStoredEntry(connection, sessionId, stack);
  if (std::holds_alternative<AppError>(found)) {
    return std::get<AppError>(found);
  }
  const std::optional<StoredUndoEntry>& entry = std::get<std::optional<StoredUndoEntry>>(found);
  if (!entry.has_value()) {
    return validationError(stack == UndoStack::Undo ? QStringLiteral("Nothing to undo")
                                                    : QStringLiteral("Nothing to redo"));
  }
  return UndoEntry{.action = stack == UndoStack::Undo ? UndoAction::Undo : UndoAction::Redo,
                   .actionKind = entry->actionKind,
                   .label = entry->label,
                   .resource = entry->resource,
                   .resourceId = entry->resourceId,
                   .expected = stack == UndoStack::Undo ? entry->after : entry->before,
                   .target = stack == UndoStack::Undo ? entry->before : entry->after};
}

[[nodiscard]] UndoRecoveryResult recoverStoredEntries(SqliteConnection& connection,
                                                      const QString& sessionId,
                                                      const QString& staleBefore,
                                                      int maximumEntries) {
  SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
  if (std::holds_alternative<AppError>(transactionResult)) {
    return std::get<AppError>(transactionResult);
  }
  SqliteTransaction transaction = std::move(std::get<SqliteTransaction>(transactionResult));
  int discardedEntries = 0;
  if (const std::optional<AppError> error =
          cleanupStoredEntries(connection, sessionId, staleBefore, maximumEntries, &discardedEntries);
      error.has_value()) {
    return *error;
  }
  const UndoStatusResult status = readStoredStatus(connection, sessionId);
  if (std::holds_alternative<AppError>(status)) {
    return std::get<AppError>(status);
  }
  if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
    return *error;
  }
  return UndoRecoveryReport{.status = std::get<UndoStatus>(status),
                            .discardedEntries = discardedEntries};
}

} // namespace

UndoRecoveryPolicy::UndoRecoveryPolicy(FilePath databasePath, const Clock& clock, QString sessionId)
    : clock_(clock),
      sessionId_(
          sessionId.isEmpty()
              ? QStringLiteral("session:%1").arg(QUuid::createUuid().toString(QUuid::WithoutBraces))
              : std::move(sessionId)),
      writerQueue_(std::move(databasePath)),
      initialization_(writerQueue_
                          .enqueue([](SqliteConnection& connection) -> SqliteWriteResult {
                            const SqliteMigrationRunResultOrError result =
                                LocalSchema::initialize(connection);
                            return std::holds_alternative<AppError>(result)
                                       ? std::optional<AppError>(std::get<AppError>(result))
                                       : std::nullopt;
                          })
                          .share()) {}

std::shared_future<SqliteWriteResult> UndoRecoveryPolicy::ready() const { return initialization_; }

const QString& UndoRecoveryPolicy::sessionId() const noexcept { return sessionId_; }

void UndoRecoveryPolicy::configure(UndoHistoryConfiguration configuration) {
  configuration.retentionDays = std::clamp(configuration.retentionDays,
                                           kMinimumRetentionDays,
                                           kMaximumRetentionDays);
  configuration.maximumEntries = std::clamp(configuration.maximumEntries,
                                            kMinimumEntries,
                                            kMaximumEntries);
  std::scoped_lock lock(configurationMutex_);
  configuration_ = configuration;
}

UndoHistoryConfiguration UndoRecoveryPolicy::configuration() const {
  std::scoped_lock lock(configurationMutex_);
  return configuration_;
}

std::future<UndoStatusResult> UndoRecoveryPolicy::status() {
  return writerQueue_.enqueueResult(
      [sessionId = sessionId_](SqliteConnection& connection) -> UndoStatusResult {
        return readStoredStatus(connection, sessionId);
      });
}

std::future<UndoEntryResult> UndoRecoveryPolicy::nextUndo() {
  return writerQueue_.enqueueResult(
      [sessionId = sessionId_](SqliteConnection& connection) -> UndoEntryResult {
        return readStoredTopEntry(connection, sessionId, UndoStack::Undo);
      });
}

std::future<UndoEntryResult> UndoRecoveryPolicy::nextRedo() {
  return writerQueue_.enqueueResult(
      [sessionId = sessionId_](SqliteConnection& connection) -> UndoEntryResult {
        return readStoredTopEntry(connection, sessionId, UndoStack::Redo);
      });
}

std::future<std::optional<AppError>> UndoRecoveryPolicy::record(UndoChangeInput input) {
  const std::variant<UndoChangeInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(std::optional<AppError>(std::get<AppError>(canonical)));
  }
  const UndoChangeInput storedInput = std::get<UndoChangeInput>(canonical);
  if (snapshotBytes(storedInput.before) == snapshotBytes(storedInput.after)) {
    return readyFuture(std::optional<AppError>{});
  }
  const UndoHistoryConfiguration configuration = this->configuration();
  const WallTimePoint nowPoint = clock_.wallNow();
  const QString now = timestampAt(nowPoint);
  const QString staleBefore = timestampAt(nowPoint - std::chrono::hours(24 * configuration.retentionDays));
  return writerQueue_.enqueueResult([sessionId = sessionId_, input = storedInput, now, staleBefore,
                                     maximumEntries = configuration.maximumEntries](
                                        SqliteConnection& connection) -> std::optional<AppError> {
    return recordStoredEntry(connection, sessionId, input, now, staleBefore, maximumEntries);
  });
}

std::future<UndoReplayResult> UndoRecoveryPolicy::undo(QJsonValue currentSnapshot) {
  if (!isValidSnapshot(currentSnapshot)) {
    return readyFuture(
        UndoReplayResult(validationError(QStringLiteral("Undo snapshot is invalid"))));
  }
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult([sessionId = sessionId_,
                                     currentSnapshot = std::move(currentSnapshot),
                                     now](SqliteConnection& connection) -> UndoReplayResult {
    return moveStoredTopEntry(connection, sessionId, UndoStack::Undo, currentSnapshot, now);
  });
}

std::future<UndoReplayResult> UndoRecoveryPolicy::redo(QJsonValue currentSnapshot) {
  if (!isValidSnapshot(currentSnapshot)) {
    return readyFuture(
        UndoReplayResult(validationError(QStringLiteral("Redo snapshot is invalid"))));
  }
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult([sessionId = sessionId_,
                                     currentSnapshot = std::move(currentSnapshot),
                                     now](SqliteConnection& connection) -> UndoReplayResult {
    return moveStoredTopEntry(connection, sessionId, UndoStack::Redo, currentSnapshot, now);
  });
}

std::future<UndoRecoveryResult> UndoRecoveryPolicy::recover() {
  const UndoHistoryConfiguration configuration = this->configuration();
  const WallTimePoint nowPoint = clock_.wallNow();
  const QString staleBefore = timestampAt(nowPoint - std::chrono::hours(24 * configuration.retentionDays));
  return writerQueue_.enqueueResult(
      [sessionId = sessionId_, staleBefore, maximumEntries = configuration.maximumEntries](SqliteConnection& connection) -> UndoRecoveryResult {
        return recoverStoredEntries(connection, sessionId, staleBefore, maximumEntries);
      });
}

} // namespace hcb
