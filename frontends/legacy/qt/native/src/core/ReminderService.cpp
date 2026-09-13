#include "core/ReminderService.h"

#include "app/NativeReminderNotifier.h"
#include "data/LocalSchema.h"
#include "data/SqliteConnection.h"

#include "sqlite3.h"

#include <QCryptographicHash>
#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocale>
#include <QSet>
#include <QTimer>
#include <QTimeZone>

#include <chrono>
#include <algorithm>
#include <functional>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr int kRefreshIntervalMilliseconds = 5 * 60 * 1'000;
constexpr int kMaximumReminderMinutes = 40'320;
constexpr int kMaximumScheduledDays = 35;

struct ReminderCandidate final {
  QString identifier;
  QString eventId;
  QString title;
  QString body;
  QDateTime triggerAt;
};

struct ReminderState final {
  std::optional<QDateTime> snoozedUntil;
  bool dismissed{false};
};

[[nodiscard]] QDateTime now(const Clock& clock) {
  const auto milliseconds = std::chrono::duration_cast<std::chrono::milliseconds>(
      clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC);
}

[[nodiscard]] std::optional<QDateTime> parseDateTime(const QString& value) {
  QDateTime result = QDateTime::fromString(value, Qt::ISODateWithMs);
  if (!result.isValid()) {
    result = QDateTime::fromString(value, Qt::ISODate);
  }
  return result.isValid() ? std::optional<QDateTime>(result.toUTC()) : std::nullopt;
}

[[nodiscard]] std::optional<QDateTime>
reminderStartAt(const QString& startAt,
                bool allDay,
                const std::optional<QString>& eventTimeZone,
                const std::optional<QString>& calendarTimeZone) {
  const std::optional<QDateTime> parsed = parseDateTime(startAt);
  if (!parsed.has_value() || !allDay) {
    return parsed;
  }
  QTimeZone timeZone = QTimeZone::systemTimeZone();
  for (const std::optional<QString>& identifier : {eventTimeZone, calendarTimeZone}) {
    if (!identifier.has_value()) {
      continue;
    }
    const QTimeZone candidate(identifier->toUtf8());
    if (candidate.isValid()) {
      timeZone = candidate;
      break;
    }
  }
  const QDateTime localStart(parsed->date(), QTime(0, 0), timeZone);
  return localStart.isValid() ? std::optional<QDateTime>(localStart.toUTC()) : std::nullopt;
}

[[nodiscard]] std::optional<QString> textColumn(sqlite3_stmt* statement, int column) {
  const auto* text = reinterpret_cast<const char*>(sqlite3_column_text(statement, column));
  const int size = sqlite3_column_bytes(statement, column);
  if (text == nullptr || size < 0) {
    return std::nullopt;
  }
  return QString::fromUtf8(text, size);
}

[[nodiscard]] QString identifierFor(const QString& eventId, const QDateTime& triggerAt) {
  const QByteArray source = eventId.toUtf8() + '\0' +
                            triggerAt.toUTC().toString(Qt::ISODateWithMs).toUtf8();
  return QStringLiteral("hcb.reminder.") +
         QString::fromLatin1(QCryptographicHash::hash(source,
                                                        QCryptographicHash::Sha256)
                                    .toHex());
}

[[nodiscard]] QList<int> reminderMinutes(const QString& json) {
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(json.toUtf8(), &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isArray()) {
    return {};
  }
  QSet<int> unique;
  for (const QJsonValue& value : document.array()) {
    if (!value.isObject()) {
      return {};
    }
    const QJsonObject reminder = value.toObject();
    if (reminder.value(QStringLiteral("method")).toString() != QStringLiteral("popup")) {
      continue;
    }
    const QJsonValue minutes = reminder.value(QStringLiteral("minutes"));
    if (!minutes.isDouble()) {
      return {};
    }
    const int valueMinutes = minutes.toInt(-1);
    if (valueMinutes < 0 || valueMinutes > kMaximumReminderMinutes) {
      return {};
    }
    unique.insert(valueMinutes);
  }
  QList<int> result = unique.values();
  std::sort(result.begin(), result.end(), std::greater<>());
  return result;
}

[[nodiscard]] std::optional<ReminderState> readState(sqlite3* handle, const QString& identifier) {
  constexpr char sql[] = "SELECT snoozed_until, dismissed_at FROM local_reminder_state WHERE identifier = ?1";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const QByteArray utf8 = identifier.toUtf8();
  if (sqlite3_bind_text(statement, 1, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  const int stepped = sqlite3_step(statement);
  if (stepped == SQLITE_DONE) {
    sqlite3_finalize(statement);
    return ReminderState{};
  }
  if (stepped != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return std::nullopt;
  }
  ReminderState state;
  if (sqlite3_column_type(statement, 0) != SQLITE_NULL) {
    const std::optional<QString> text = textColumn(statement, 0);
    if (!text.has_value()) {
      sqlite3_finalize(statement);
      return std::nullopt;
    }
    state.snoozedUntil = parseDateTime(*text);
    if (!state.snoozedUntil.has_value()) {
      sqlite3_finalize(statement);
      return std::nullopt;
    }
  }
  state.dismissed = sqlite3_column_type(statement, 1) != SQLITE_NULL;
  const bool done = sqlite3_finalize(statement) == SQLITE_OK;
  return done ? std::optional<ReminderState>(std::move(state)) : std::nullopt;
}

[[nodiscard]] bool saveCandidateState(sqlite3* handle,
                                      const ReminderCandidate& candidate,
                                      const QDateTime& current) {
  constexpr char sql[] = R"(
INSERT INTO local_reminder_state(identifier, event_id, trigger_at, updated_at)
VALUES(?1, ?2, ?3, ?4)
ON CONFLICT(identifier) DO UPDATE SET
  event_id = excluded.event_id, trigger_at = excluded.trigger_at, updated_at = excluded.updated_at
)";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return false;
  }
  const QList<QByteArray> values{candidate.identifier.toUtf8(), candidate.eventId.toUtf8(),
                                 candidate.triggerAt.toUTC().toString(Qt::ISODateWithMs).toUtf8(),
                                 current.toUTC().toString(Qt::ISODateWithMs).toUtf8()};
  for (int index = 0; index < values.size(); ++index) {
    if (sqlite3_bind_text(statement,
                          index + 1,
                          values.at(index).constData(),
                          static_cast<int>(values.at(index).size()),
                          SQLITE_TRANSIENT) != SQLITE_OK) {
      sqlite3_finalize(statement);
      return false;
    }
  }
  const bool saved = sqlite3_step(statement) == SQLITE_DONE;
  return sqlite3_finalize(statement) == SQLITE_OK && saved;
}

[[nodiscard]] QList<QString> activeStateIdentifiers(sqlite3* handle) {
  constexpr char sql[] = "SELECT identifier FROM local_reminder_state WHERE dismissed_at IS NULL";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return {};
  }
  QList<QString> values;
  while (sqlite3_step(statement) == SQLITE_ROW) {
    const std::optional<QString> identifier = textColumn(statement, 0);
    if (!identifier.has_value()) {
      sqlite3_finalize(statement);
      return {};
    }
    values.append(*identifier);
  }
  const bool valid = sqlite3_finalize(statement) == SQLITE_OK;
  return valid ? values : QList<QString>{};
}

[[nodiscard]] bool updateState(sqlite3* handle,
                               const QString& identifier,
                               const std::optional<QDateTime>& snoozedUntil,
                               bool dismissed,
                               const QDateTime& current) {
  constexpr char sql[] = R"(
UPDATE local_reminder_state
SET snoozed_until = ?2, dismissed_at = ?3, updated_at = ?4
WHERE identifier = ?1
)";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return false;
  }
  const QByteArray identifierUtf8 = identifier.toUtf8();
  const QByteArray snoozeUtf8 = snoozedUntil.has_value()
                                     ? snoozedUntil->toUTC().toString(Qt::ISODateWithMs).toUtf8()
                                     : QByteArray();
  const QByteArray dismissedUtf8 = current.toUTC().toString(Qt::ISODateWithMs).toUtf8();
  const QByteArray currentUtf8 = current.toUTC().toString(Qt::ISODateWithMs).toUtf8();
  const bool bound = sqlite3_bind_text(statement, 1, identifierUtf8.constData(), static_cast<int>(identifierUtf8.size()),
                                       SQLITE_TRANSIENT) == SQLITE_OK &&
                     (snoozedUntil.has_value()
                          ? sqlite3_bind_text(statement, 2, snoozeUtf8.constData(), static_cast<int>(snoozeUtf8.size()), SQLITE_TRANSIENT)
                          : sqlite3_bind_null(statement, 2)) == SQLITE_OK &&
                     (dismissed
                          ? sqlite3_bind_text(statement, 3, dismissedUtf8.constData(), static_cast<int>(dismissedUtf8.size()), SQLITE_TRANSIENT)
                          : sqlite3_bind_null(statement, 3)) == SQLITE_OK &&
                     sqlite3_bind_text(statement, 4, currentUtf8.constData(), static_cast<int>(currentUtf8.size()), SQLITE_TRANSIENT) == SQLITE_OK;
  const bool updated = bound && sqlite3_step(statement) == SQLITE_DONE && sqlite3_changes(handle) == 1;
  return sqlite3_finalize(statement) == SQLITE_OK && updated;
}

} // namespace

ReminderService::ReminderService(FilePath databasePath,
                                 Clock& clock,
                                 NativeReminderNotifier& notifier,
                                 QObject* parent)
    : QObject(parent), databasePath_(std::move(databasePath)), clock_(clock), notifier_(notifier) {
  refreshTimer_ = new QTimer(this);
  refreshTimer_->setInterval(kRefreshIntervalMilliseconds);
  QObject::connect(refreshTimer_, &QTimer::timeout, this, &ReminderService::refresh);
  QObject::connect(&notifier_, &NativeReminderNotifier::actionRequested, this,
                   &ReminderService::handleAction);
  QObject::connect(&notifier_, &NativeReminderNotifier::statusChanged, this,
                   &ReminderService::setStatusMessage);
}

ReminderService::~ReminderService() = default;

void ReminderService::start() {
  notifier_.requestAuthorization();
  refreshTimer_->start();
  refresh();
}

void ReminderService::refresh() {
  SqliteConnectionResult opened =
      SqliteConnectionFactory::open(databasePath_, SqliteOpenMode::ReadWriteCreate);
  if (std::holds_alternative<AppError>(opened)) {
    setStatusMessage(QStringLiteral("Calendar reminders are unavailable"));
    return;
  }
  SqliteConnection connection = std::move(std::get<SqliteConnection>(opened));
  if (std::holds_alternative<AppError>(LocalSchema::initialize(connection))) {
    setStatusMessage(QStringLiteral("Calendar reminders are unavailable"));
    return;
  }
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    setStatusMessage(QStringLiteral("Calendar reminders are unavailable"));
    return;
  }
  const QDateTime current = now(clock_);
  const QDateTime upperBound = current.addDays(kMaximumScheduledDays);
  constexpr char sql[] = R"(
SELECT events.id, events.title, events.start_at, events.start_time_zone, events.is_all_day,
       events.reminders_json, events.reminders_use_default, calendars.time_zone,
       calendars.default_reminders_json
FROM local_calendar_events AS events
INNER JOIN local_calendars AS calendars ON calendars.id = events.calendar_id
WHERE events.deleted_at IS NULL AND events.status != 'cancelled'
  AND events.start_at >= ?1 AND events.start_at <= ?2
)";
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) != SQLITE_OK) {
    sqlite3_finalize(statement);
    setStatusMessage(QStringLiteral("Calendar reminders could not read local events"));
    return;
  }
  const QByteArray lower = current.addDays(-1).toUTC().toString(Qt::ISODateWithMs).toUtf8();
  const QByteArray upper = upperBound.toUTC().toString(Qt::ISODateWithMs).toUtf8();
  if (sqlite3_bind_text(statement, 1, lower.constData(), static_cast<int>(lower.size()), SQLITE_TRANSIENT) != SQLITE_OK ||
      sqlite3_bind_text(statement, 2, upper.constData(), static_cast<int>(upper.size()), SQLITE_TRANSIENT) != SQLITE_OK) {
    sqlite3_finalize(statement);
    setStatusMessage(QStringLiteral("Calendar reminders could not read local events"));
    return;
  }
  QList<ReminderCandidate> candidates;
  bool valid = true;
  while (sqlite3_step(statement) == SQLITE_ROW) {
    const std::optional<QString> eventId = textColumn(statement, 0);
    const std::optional<QString> title = textColumn(statement, 1);
    const std::optional<QString> startAt = textColumn(statement, 2);
    const std::optional<QString> eventTimeZone = textColumn(statement, 3);
    const std::optional<QString> overrides = textColumn(statement, 5);
    const std::optional<QString> calendarTimeZone = textColumn(statement, 7);
    const std::optional<QString> defaults = textColumn(statement, 8);
    const std::optional<QDateTime> start =
        startAt.has_value() ? reminderStartAt(*startAt,
                                               sqlite3_column_int(statement, 4) != 0,
                                               eventTimeZone,
                                               calendarTimeZone)
                            : std::nullopt;
    if (!eventId.has_value() || !title.has_value() || !overrides.has_value() || !defaults.has_value() ||
        !start.has_value()) {
      valid = false;
      break;
    }
    const QString& source = sqlite3_column_int(statement, 6) != 0 ? *defaults : *overrides;
    for (const int minutes : reminderMinutes(source)) {
      const QDateTime triggerAt = start->addSecs(-minutes * 60);
      if (triggerAt > upperBound) {
        continue;
      }
      candidates.append({.identifier = identifierFor(*eventId, triggerAt),
                         .eventId = *eventId,
                         .title = *title,
                         .body = QStringLiteral("Starts %1").arg(QLocale().toString(start->toLocalTime(), QLocale::ShortFormat)),
                         .triggerAt = triggerAt});
    }
  }
  valid = sqlite3_finalize(statement) == SQLITE_OK && valid;
  if (!valid) {
    setStatusMessage(QStringLiteral("Calendar reminders could not decode local events"));
    return;
  }
  QSet<QString> expected;
  int scheduled = 0;
  for (const ReminderCandidate& candidate : candidates) {
    expected.insert(candidate.identifier);
    if (!saveCandidateState(handle, candidate, current)) {
      continue;
    }
    const std::optional<ReminderState> state = readState(handle, candidate.identifier);
    if (!state.has_value() || state->dismissed) {
      continue;
    }
    QDateTime deliverAt = state->snoozedUntil.value_or(candidate.triggerAt);
    if (deliverAt <= current) {
      if (!state->snoozedUntil.has_value()) {
        continue;
      }
      deliverAt = current.addSecs(1);
    }
    notifier_.schedule({.identifier = candidate.identifier,
                        .title = candidate.title,
                        .body = candidate.body,
                        .deliverAt = deliverAt});
    ++scheduled;
  }
  for (const QString& identifier : activeStateIdentifiers(handle)) {
    if (!expected.contains(identifier)) {
      notifier_.cancel(identifier);
      static_cast<void>(updateState(handle, identifier, std::nullopt, true, current));
    }
  }
  setStatusMessage(scheduled == 0 ? QStringLiteral("No upcoming desktop calendar reminders")
                                  : QStringLiteral("%1 upcoming desktop calendar reminder(s)").arg(scheduled));
}

void ReminderService::dismiss(QString identifier) {
  if (!identifier.startsWith(QStringLiteral("hcb.reminder."))) {
    return;
  }
  SqliteConnectionResult opened =
      SqliteConnectionFactory::open(databasePath_, SqliteOpenMode::ReadWriteCreate);
  if (std::holds_alternative<AppError>(opened)) {
    return;
  }
  SqliteConnection connection = std::move(std::get<SqliteConnection>(opened));
  if (updateState(connection.nativeHandle(), identifier, std::nullopt, true, now(clock_))) {
    notifier_.cancel(identifier);
    setStatusMessage(QStringLiteral("Calendar reminder dismissed"));
  }
}

void ReminderService::snooze(QString identifier, int minutes) {
  if (!identifier.startsWith(QStringLiteral("hcb.reminder.")) || minutes < 1 || minutes > 24 * 60) {
    return;
  }
  SqliteConnectionResult opened =
      SqliteConnectionFactory::open(databasePath_, SqliteOpenMode::ReadWriteCreate);
  if (std::holds_alternative<AppError>(opened)) {
    return;
  }
  const QDateTime current = now(clock_);
  SqliteConnection connection = std::move(std::get<SqliteConnection>(opened));
  if (updateState(connection.nativeHandle(), identifier, current.addSecs(minutes * 60), false, current)) {
    notifier_.cancel(identifier);
    setStatusMessage(QStringLiteral("Calendar reminder snoozed for %1 minutes").arg(minutes));
    refresh();
  }
}

QString ReminderService::statusMessage() const { return statusMessage_; }

void ReminderService::handleAction(QString identifier, ReminderAction action) {
  if (action == ReminderAction::SnoozeTenMinutes) {
    snooze(std::move(identifier));
    return;
  }
  dismiss(std::move(identifier));
}

void ReminderService::setStatusMessage(QString message) {
  if (message == statusMessage_) {
    return;
  }
  statusMessage_ = std::move(message);
  emit statusMessageChanged();
}

} // namespace hcb
