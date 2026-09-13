#include "core/MutationTelemetryStore.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QDateTime>
#include <QTimeZone>
#include <QUuid>

#include <array>
#include <algorithm>
#include <chrono>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 128;
constexpr qsizetype kMaximumDateLength = 64;

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] QString timestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] QString phaseName(MutationTelemetryPhase phase) {
  switch (phase) {
  case MutationTelemetryPhase::Intent:
    return QStringLiteral("intent");
  case MutationTelemetryPhase::RemoteApplied:
    return QStringLiteral("remote_applied");
  case MutationTelemetryPhase::RemoteFailed:
    return QStringLiteral("remote_failed");
  case MutationTelemetryPhase::Rollback:
    return QStringLiteral("rollback");
  }
  return QStringLiteral("intent");
}

[[nodiscard]] std::optional<MutationTelemetryPhase> parsePhase(const QString& value) {
  if (value == QStringLiteral("intent")) return MutationTelemetryPhase::Intent;
  if (value == QStringLiteral("remote_applied")) return MutationTelemetryPhase::RemoteApplied;
  if (value == QStringLiteral("remote_failed")) return MutationTelemetryPhase::RemoteFailed;
  if (value == QStringLiteral("rollback")) return MutationTelemetryPhase::Rollback;
  return std::nullopt;
}

[[nodiscard]] bool validIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumIdentifierLength &&
         std::all_of(value.cbegin(), value.cend(), [](QChar character) {
           return character.isLetterOrNumber() || character == u'_' || character == u'-' ||
                  character == u'.' || character == u':';
         });
}

[[nodiscard]] bool validOptionalDate(const std::optional<QString>& value) {
  return !value.has_value() || (!value->isEmpty() && value->size() <= kMaximumDateLength &&
                                QDateTime::fromString(*value, Qt::ISODateWithMs).isValid());
}

[[nodiscard]] bool validOptionalIdentifier(const std::optional<QString>& value) {
  return !value.has_value() || validIdentifier(*value);
}

[[nodiscard]] std::optional<AppError> bindText(sqlite3_stmt* statement,
                                                int index,
                                                const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(statement, index, utf8.constData(), static_cast<int>(utf8.size()),
                                       SQLITE_TRANSIENT);
  return result == SQLITE_OK ? std::nullopt
                             : std::optional<AppError>(databaseError(
                                   QStringLiteral("SQLite telemetry binding failed (%1)"), result));
}

[[nodiscard]] std::optional<AppError> bindOptionalText(sqlite3_stmt* statement,
                                                        int index,
                                                        const std::optional<QString>& value) {
  if (!value.has_value()) {
    const int result = sqlite3_bind_null(statement, index);
    return result == SQLITE_OK ? std::nullopt
                               : std::optional<AppError>(databaseError(
                                     QStringLiteral("SQLite telemetry binding failed (%1)"), result));
  }
  return bindText(statement, index, *value);
}

[[nodiscard]] MutationTelemetryWriteResult validate(MutationTelemetryInput input) {
  const auto allowed = [](const QString& value, const auto& values) {
    return std::find(values.cbegin(), values.cend(), value) != values.cend();
  };
  const std::array<QString, 3> resources = {QStringLiteral("task"),
                                             QStringLiteral("task_list"),
                                             QStringLiteral("event")};
  const std::array<QString, 4> scopes = {QStringLiteral("none"),
                                         QStringLiteral("this_instance"),
                                         QStringLiteral("this_and_following"),
                                         QStringLiteral("full_series")};
  if (!validIdentifier(input.resource) || !validIdentifier(input.operation) ||
      !validIdentifier(input.scope) || (!input.mutationId.isEmpty() && !validIdentifier(input.mutationId)) ||
      !validOptionalDate(input.targetStartAt) || !validOptionalDate(input.targetEndAt) ||
      !validOptionalIdentifier(input.remoteOutcome) || !validOptionalIdentifier(input.errorCode) ||
      !validOptionalIdentifier(input.rollbackReason) ||
      !allowed(input.resource, resources) || !allowed(input.scope, scopes)) {
    return AppError(AppErrorCode::Validation, QStringLiteral("Mutation telemetry input is invalid"));
  }
  return MutationTelemetryRecord{};
}

[[nodiscard]] MutationTelemetryReadResult readRecent(SqliteConnection& connection, int limit) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database, QStringLiteral("SQLite telemetry connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepare = sqlite3_prepare_v3(
      handle,
      "SELECT id, mutation_id, resource, operation, scope, all_day, target_start_at, target_end_at, "
      "phase, remote_outcome, error_code, rollback_reason, created_at "
      "FROM local_mutation_telemetry ORDER BY created_at DESC, id DESC LIMIT ?1",
      -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepare != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite telemetry read preparation failed (%1)"), prepare);
  }
  sqlite3_bind_int(statement, 1, limit);
  QList<MutationTelemetryRecord> records;
  for (;;) {
    const int step = sqlite3_step(statement);
    if (step == SQLITE_DONE) break;
    if (step != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite telemetry read failed (%1)"), step);
    }
    const auto text = [statement](int column) {
      const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, column));
      return value == nullptr ? QString() : QString::fromUtf8(value);
    };
    const std::optional<MutationTelemetryPhase> phase = parsePhase(text(8));
    if (!phase.has_value()) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database, QStringLiteral("SQLite telemetry phase is invalid"));
    }
    const auto optional = [statement, &text](int column) -> std::optional<QString> {
      return sqlite3_column_type(statement, column) == SQLITE_NULL ? std::nullopt
                                                                     : std::optional<QString>(text(column));
    };
    records.append({.id = text(0), .mutationId = text(1), .resource = text(2), .operation = text(3),
                    .scope = text(4), .allDay = sqlite3_column_int(statement, 5) != 0,
                    .targetStartAt = optional(6), .targetEndAt = optional(7), .phase = *phase,
                    .remoteOutcome = optional(9), .errorCode = optional(10),
                    .rollbackReason = optional(11), .createdAt = text(12)});
  }
  sqlite3_finalize(statement);
  return records;
}

} // namespace

MutationTelemetryStore::MutationTelemetryStore(FilePath databasePath, const Clock& clock)
    : clock_(clock), writerQueue_(std::move(databasePath)),
      initialization_(writerQueue_.enqueue([](SqliteConnection& connection) {
        const auto result = LocalSchema::initialize(connection);
        return std::holds_alternative<AppError>(result)
                   ? std::optional<AppError>(std::get<AppError>(result))
                   : std::nullopt;
      }).share()) {}

std::shared_future<SqliteWriteResult> MutationTelemetryStore::ready() const { return initialization_; }

std::future<MutationTelemetryWriteResult> MutationTelemetryStore::record(MutationTelemetryInput input) {
  const MutationTelemetryWriteResult validation = validate(input);
  if (std::holds_alternative<AppError>(validation)) {
    std::promise<MutationTelemetryWriteResult> completion;
    auto future = completion.get_future();
    completion.set_value(std::get<AppError>(validation));
    return future;
  }
  MutationTelemetryRecord record{.id = QUuid::createUuid().toString(QUuid::WithoutBraces),
                                  .mutationId = std::move(input.mutationId),
                                  .resource = std::move(input.resource),
                                  .operation = std::move(input.operation),
                                  .scope = std::move(input.scope),
                                  .allDay = input.allDay,
                                  .targetStartAt = std::move(input.targetStartAt),
                                  .targetEndAt = std::move(input.targetEndAt),
                                  .phase = input.phase,
                                  .remoteOutcome = std::move(input.remoteOutcome),
                                  .errorCode = std::move(input.errorCode),
                                  .rollbackReason = std::move(input.rollbackReason),
                                  .createdAt = timestamp(clock_)};
  return writerQueue_.enqueueResult([record = std::move(record)](SqliteConnection& connection)
                                        -> MutationTelemetryWriteResult {
    sqlite3* const handle = connection.nativeHandle();
    if (handle == nullptr) {
      return AppError(AppErrorCode::Database, QStringLiteral("SQLite telemetry connection is unavailable"));
    }
    sqlite3_stmt* statement = nullptr;
    const int prepare = sqlite3_prepare_v3(
        handle,
        "INSERT INTO local_mutation_telemetry (id, mutation_id, resource, operation, scope, all_day, "
        "target_start_at, target_end_at, phase, remote_outcome, error_code, rollback_reason, created_at) "
        "VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13)",
        -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
    if (prepare != SQLITE_OK) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite telemetry write preparation failed (%1)"), prepare);
    }
    const std::array<std::optional<AppError>, 13> bindings = {
        bindText(statement, 1, record.id), bindText(statement, 2, record.mutationId),
        bindText(statement, 3, record.resource), bindText(statement, 4, record.operation),
        bindText(statement, 5, record.scope),
        sqlite3_bind_int(statement, 6, record.allDay ? 1 : 0) == SQLITE_OK
            ? std::nullopt
            : std::optional<AppError>(databaseError(
                  QStringLiteral("SQLite telemetry binding failed (%1)"), SQLITE_ERROR)),
        bindOptionalText(statement, 7, record.targetStartAt), bindOptionalText(statement, 8, record.targetEndAt),
        bindText(statement, 9, phaseName(record.phase)), bindOptionalText(statement, 10, record.remoteOutcome),
        bindOptionalText(statement, 11, record.errorCode), bindOptionalText(statement, 12, record.rollbackReason),
        bindText(statement, 13, record.createdAt)};
    for (const auto& error : bindings) {
      if (error.has_value()) {
        sqlite3_finalize(statement);
        return *error;
      }
    }
    const int step = sqlite3_step(statement);
    const int finalize = sqlite3_finalize(statement);
    if (step != SQLITE_DONE || finalize != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite telemetry write failed (%1)"), step != SQLITE_DONE ? step : finalize);
    }
    const char trimSql[] =
        "DELETE FROM local_mutation_telemetry WHERE id IN (SELECT id FROM local_mutation_telemetry "
        "ORDER BY created_at DESC, id DESC LIMIT -1 OFFSET 500)";
    const int trim = sqlite3_exec(handle, trimSql, nullptr, nullptr, nullptr);
    return trim == SQLITE_OK ? MutationTelemetryWriteResult(record)
                             : MutationTelemetryWriteResult(databaseError(
                                   QStringLiteral("SQLite telemetry trim failed (%1)"), trim));
  });
}

std::future<MutationTelemetryReadResult> MutationTelemetryStore::recent(int limit) {
  return writerQueue_.enqueueResult([limit = std::clamp(limit, 1, maximumRecords)](SqliteConnection& connection) {
    return readRecent(connection, limit);
  });
}

} // namespace hcb
