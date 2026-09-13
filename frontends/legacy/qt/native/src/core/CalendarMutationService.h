#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QList>
#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

class ImportMutationService;

struct CalendarEventReminder final {
  QString method;
  int minutes{0};
};

struct CalendarEventReminderSettings final {
  bool useDefault{true};
  QList<CalendarEventReminder> overrides;
};

struct CalendarEventRichMetadata final {
  bool createGoogleMeet{false};
  QString attachmentsJson{QStringLiteral("[]")};
  QString guestPermissionsJson{QStringLiteral("{}")};
  QString eventType{QStringLiteral("default")};
  QString statusPropertiesJson{QStringLiteral("{}")};
  QString sendUpdates{QStringLiteral("all")};
};

enum class CalendarEventRecurrenceScope : std::uint8_t {
  ThisInstance,
  ThisAndFollowing,
  FullSeries
};

struct CalendarEventCreateInput final {
  QString calendarId;
  QString title;
  QString startAt;
  QString endAt;
  bool allDay{false};
  std::optional<QString> description;
  std::optional<QString> location;
  std::optional<QString> startTimeZone;
  std::optional<QString> endTimeZone;
  std::optional<QString> colorId;
  std::optional<QString> transparency;
  std::optional<QString> visibility;
  QList<QString> attendeeEmails;
  CalendarEventReminderSettings reminders;
  std::optional<QString> recurrenceRule;
  CalendarEventRichMetadata richMetadata;
};

struct CalendarEventUpdateInput final {
  QString eventId;
  std::optional<QString> calendarId;
  std::optional<QString> title;
  std::optional<std::optional<QString>> description;
  std::optional<std::optional<QString>> location;
  std::optional<QString> startAt;
  std::optional<QString> endAt;
  std::optional<bool> allDay;
  std::optional<std::optional<QString>> startTimeZone;
  std::optional<std::optional<QString>> endTimeZone;
  std::optional<std::optional<QString>> colorId;
  std::optional<QString> transparency;
  std::optional<QString> visibility;
  std::optional<QList<QString>> attendeeEmails;
  std::optional<CalendarEventReminderSettings> reminders;
  std::optional<std::optional<QString>> recurrenceRule;
  std::optional<bool> createGoogleMeet;
  std::optional<QString> attachmentsJson;
  std::optional<QString> guestPermissionsJson;
  std::optional<QString> statusPropertiesJson;
  std::optional<QString> sendUpdates;
  std::optional<QString> selfResponseStatus;
  std::optional<QString> selfResponseComment;
};

struct CalendarEventScopedUpdateInput final {
  CalendarEventUpdateInput update;
  CalendarEventRecurrenceScope scope{CalendarEventRecurrenceScope::ThisInstance};
};

struct CalendarEventScopedDeleteInput final {
  QString eventId;
  CalendarEventRecurrenceScope scope{CalendarEventRecurrenceScope::ThisInstance};
};

struct CalendarEventMutationReceipt final {
  QString eventId;
  QString updatedAt;
};

struct CalendarEventRemoteReconciliationInput final {
  QString localEventId;
  QString remoteEventId;
  std::optional<QString> remoteEtag;
};

struct CalendarEventMutationSnapshot final {
  QString eventId;
  QString accountId;
  QString calendarId;
  QString remoteId;
  std::optional<QString> calendarAccessRole;
  QString status;
  std::optional<QString> recurringRemoteId;
  std::optional<QString> originalStartAt;
  std::optional<QString> recurrenceRule;
  std::optional<QString> eventType;
  QString title;
  std::optional<QString> description;
  std::optional<QString> location;
  QString startAt;
  QString endAt;
  bool allDay{false};
  std::optional<QString> colorId;
  std::optional<QString> transparency;
  std::optional<QString> visibility;
  QString attendeeEmailsJson;
  QString attendeeDetailsJson;
  QString remindersJson;
  bool remindersUseDefault{true};
  std::optional<QString> conferenceJson;
  QString attachmentsJson;
  QString guestPermissionsJson;
  QString statusPropertiesJson;
};

using CalendarEventMutationResult = std::variant<CalendarEventMutationReceipt, AppError>;
using CalendarEventBatchMutationResult = std::variant<QList<CalendarEventMutationReceipt>, AppError>;
using CalendarEventMutationSnapshotResult =
    std::variant<QList<CalendarEventMutationSnapshot>, AppError>;

class CalendarMutationService final {
public:
  CalendarMutationService(FilePath databasePath, const Clock& clock);
  CalendarMutationService(const CalendarMutationService&) = delete;
  CalendarMutationService& operator=(const CalendarMutationService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] static std::variant<CalendarEventCreateInput, AppError>
  validateCreate(CalendarEventCreateInput input);
  [[nodiscard]] std::future<CalendarEventMutationResult> create(CalendarEventCreateInput input);
  [[nodiscard]] std::future<CalendarEventBatchMutationResult>
  createBatch(QList<CalendarEventCreateInput> inputs);
  [[nodiscard]] std::future<CalendarEventMutationResult> update(CalendarEventUpdateInput input);
  [[nodiscard]] std::future<CalendarEventMutationResult>
  respond(QString eventId, QString responseStatus, QString responseComment = {});
  [[nodiscard]] std::future<CalendarEventMutationResult>
  updateScoped(CalendarEventScopedUpdateInput input);
  [[nodiscard]] std::future<CalendarEventMutationResult> remove(QString eventId);
  [[nodiscard]] std::future<CalendarEventMutationResult> restore(QString eventId);
  [[nodiscard]] std::future<CalendarEventMutationResult>
  removeScoped(CalendarEventScopedDeleteInput input);
  [[nodiscard]] std::future<CalendarEventMutationSnapshotResult> inspect(QList<QString> eventIds);
  [[nodiscard]] std::future<CalendarEventMutationResult>
  reconcileGoogleEvent(CalendarEventRemoteReconciliationInput input);

private:
  friend class ImportMutationService;
  [[nodiscard]] static CalendarEventBatchMutationResult
  createBatchWithinTransaction(SqliteConnection& connection,
                               QList<CalendarEventCreateInput> inputs,
                               const QString& updatedAt);
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
