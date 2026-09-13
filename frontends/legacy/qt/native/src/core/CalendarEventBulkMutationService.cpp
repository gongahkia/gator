#include "core/CalendarEventBulkMutationService.h"

#include <QDateTime>
#include <QHash>
#include <QSet>

#include <algorithm>
#include <deque>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumEventCount = 500;
constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr int kMaximumShiftMinutes = 366 * 24 * 60;
constexpr qsizetype kMaximumInFlightWrites = 4;

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumIdentifierLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isWritable(const std::optional<QString>& accessRole) {
  return !accessRole.has_value() || *accessRole == QStringLiteral("writer") ||
         *accessRole == QStringLiteral("owner");
}

[[nodiscard]] bool canMove(const std::optional<QString>& accessRole) {
  return !accessRole.has_value() || *accessRole == QStringLiteral("owner");
}

[[nodiscard]] bool isRecurring(const CalendarEventMutationSnapshot& event) {
  return event.recurringRemoteId.has_value() || event.recurrenceRule.has_value();
}

[[nodiscard]] bool isMutableEventType(const std::optional<QString>& eventType) {
  return !eventType.has_value() || *eventType == QStringLiteral("default");
}

[[nodiscard]] bool isValidVisibility(const QString& value) {
  return value == QStringLiteral("default") || value == QStringLiteral("public") ||
         value == QStringLiteral("private") || value == QStringLiteral("confidential");
}

[[nodiscard]] std::optional<AppError> validate(const CalendarEventBulkMutationInput& input) {
  if (input.eventIds.isEmpty() || input.eventIds.size() > kMaximumEventCount) {
    return validationError(QStringLiteral("Bulk event selection is invalid"));
  }
  QSet<QString> uniqueIds;
  for (const QString& eventId : input.eventIds) {
    if (!isValidIdentifier(eventId) || uniqueIds.contains(eventId)) {
      return validationError(QStringLiteral("Bulk event selection is invalid"));
    }
    uniqueIds.insert(eventId);
  }
  if (input.action == CalendarEventBulkAction::MoveToCalendar &&
      (!input.calendarId.has_value() || !isValidIdentifier(*input.calendarId))) {
    return validationError(QStringLiteral("Bulk event destination is invalid"));
  }
  if (input.action == CalendarEventBulkAction::SetColor &&
      (!input.colorId.has_value() || !isValidIdentifier(*input.colorId) ||
       input.colorId->size() > 32)) {
    return validationError(QStringLiteral("Bulk event color is invalid"));
  }
  if (input.action == CalendarEventBulkAction::SetAvailability && !input.available.has_value()) {
    return validationError(QStringLiteral("Bulk event availability is invalid"));
  }
  if (input.action == CalendarEventBulkAction::SetVisibility &&
      (!input.visibility.has_value() || !isValidVisibility(*input.visibility))) {
    return validationError(QStringLiteral("Bulk event visibility is invalid"));
  }
  if (input.action == CalendarEventBulkAction::ShiftTime &&
      (!input.shiftMinutes.has_value() || *input.shiftMinutes == 0 ||
       *input.shiftMinutes < -kMaximumShiftMinutes || *input.shiftMinutes > kMaximumShiftMinutes)) {
    return validationError(QStringLiteral("Bulk event shift is invalid"));
  }
  if (input.action == CalendarEventBulkAction::ReplaceText &&
      (input.findText.isEmpty() || input.textFields == 0 ||
       (input.textFields & ~static_cast<std::uint8_t>(CalendarEventBulkTextField::Title) &
            ~static_cast<std::uint8_t>(CalendarEventBulkTextField::Description) &
            ~static_cast<std::uint8_t>(CalendarEventBulkTextField::Location)) != 0 ||
       input.recurrenceScope < 0 || input.recurrenceScope > 3)) {
    return validationError(QStringLiteral("Bulk event text replacement is invalid"));
  }
  return std::nullopt;
}

[[nodiscard]] bool replacesField(const CalendarEventBulkMutationInput& input,
                                 CalendarEventBulkTextField field) {
  return (input.textFields & static_cast<std::uint8_t>(field)) != 0;
}

[[nodiscard]] bool hasReplacement(const CalendarEventBulkMutationInput& input,
                                  const CalendarEventMutationSnapshot& event) {
  return (replacesField(input, CalendarEventBulkTextField::Title) &&
          event.title.contains(input.findText)) ||
         (replacesField(input, CalendarEventBulkTextField::Description) &&
          event.description.value_or(QString()).contains(input.findText)) ||
         (replacesField(input, CalendarEventBulkTextField::Location) &&
          event.location.value_or(QString()).contains(input.findText));
}

[[nodiscard]] std::optional<QPair<QString, QString>>
shiftedTimes(const CalendarEventMutationSnapshot& event, int shiftMinutes) {
  if (event.allDay && shiftMinutes % (24 * 60) != 0) {
    return std::nullopt;
  }
  const QDateTime start = QDateTime::fromString(event.startAt, Qt::ISODateWithMs);
  const QDateTime end = QDateTime::fromString(event.endAt, Qt::ISODateWithMs);
  if (!start.isValid() || !end.isValid()) {
    return std::nullopt;
  }
  const QDateTime shiftedStart = start.addSecs(static_cast<qint64>(shiftMinutes) * 60);
  const QDateTime shiftedEnd = end.addSecs(static_cast<qint64>(shiftMinutes) * 60);
  if (!shiftedStart.isValid() || !shiftedEnd.isValid() || shiftedEnd <= shiftedStart) {
    return std::nullopt;
  }
  return QPair<QString, QString>{shiftedStart.toUTC().toString(Qt::ISODateWithMs),
                                 shiftedEnd.toUTC().toString(Qt::ISODateWithMs)};
}

[[nodiscard]] std::optional<QString>
ineligibility(const CalendarEventBulkMutationInput& input,
              const CalendarEventMutationSnapshot& event) {
  if (!isWritable(event.calendarAccessRole)) {
    return QStringLiteral("Calendar is read-only");
  }
  if (!isMutableEventType(event.eventType)) {
    return QStringLiteral("Event type is immutable");
  }
  if (isRecurring(event) && input.action != CalendarEventBulkAction::ReplaceText) {
    return QStringLiteral("Recurring events require the series editor");
  }
  if (isRecurring(event) && input.action == CalendarEventBulkAction::ReplaceText) {
    if (input.recurrenceScope == 0) {
      return QStringLiteral("Recurring event skipped by rewrite scope");
    }
    if (input.recurrenceScope == 1 && event.recurrenceRule.has_value()) {
      return QStringLiteral("Recurring event master requires full-series scope");
    }
    if (input.recurrenceScope == 2 && !event.recurringRemoteId.has_value()) {
      return QStringLiteral("Current-and-future scope requires a recurring instance");
    }
  }
  if (event.status != QStringLiteral("confirmed") && event.status != QStringLiteral("tentative")) {
    return QStringLiteral("Event is unavailable");
  }
  switch (input.action) {
  case CalendarEventBulkAction::Delete:
    return std::nullopt;
  case CalendarEventBulkAction::MoveToCalendar:
    if (!canMove(event.calendarAccessRole)) {
      return QStringLiteral("Only owner-calendar events can move");
    }
    return event.calendarId == *input.calendarId
               ? std::optional<QString>(QStringLiteral("Event is already in that calendar"))
               : std::nullopt;
  case CalendarEventBulkAction::SetColor:
    return event.colorId == input.colorId
               ? std::optional<QString>(QStringLiteral("Event already has that color"))
               : std::nullopt;
  case CalendarEventBulkAction::SetAvailability: {
    const QString transparency = *input.available ? QStringLiteral("transparent")
                                                   : QStringLiteral("opaque");
    return event.transparency == transparency
               ? std::optional<QString>(QStringLiteral("Event already has that availability"))
               : std::nullopt;
  }
  case CalendarEventBulkAction::SetVisibility:
    return event.visibility == input.visibility
               ? std::optional<QString>(QStringLiteral("Event already has that visibility"))
               : std::nullopt;
  case CalendarEventBulkAction::ShiftTime:
    return shiftedTimes(event, *input.shiftMinutes).has_value()
               ? std::nullopt
               : std::optional<QString>(QStringLiteral("Event cannot shift by that interval"));
  case CalendarEventBulkAction::ReplaceText:
    return hasReplacement(input, event)
               ? std::nullopt
               : std::optional<QString>(QStringLiteral("No selected field matches the find text"));
  }
  return QStringLiteral("Bulk event action is invalid");
}

[[nodiscard]] std::future<CalendarEventMutationResult>
submit(CalendarMutationService& service,
       const CalendarEventBulkMutationInput& input,
       const CalendarEventMutationSnapshot& event) {
  switch (input.action) {
  case CalendarEventBulkAction::Delete:
    return service.remove(event.eventId);
  case CalendarEventBulkAction::MoveToCalendar:
    return service.update({.eventId = event.eventId, .calendarId = input.calendarId});
  case CalendarEventBulkAction::SetColor:
    return service.update(
        {.eventId = event.eventId, .colorId = std::optional<std::optional<QString>>(input.colorId)});
  case CalendarEventBulkAction::SetAvailability:
    return service.update({.eventId = event.eventId,
                           .transparency = *input.available ? QStringLiteral("transparent")
                                                            : QStringLiteral("opaque")});
  case CalendarEventBulkAction::SetVisibility:
    return service.update({.eventId = event.eventId, .visibility = input.visibility});
  case CalendarEventBulkAction::ShiftTime: {
    const std::optional<QPair<QString, QString>> times = shiftedTimes(event, *input.shiftMinutes);
    if (!times.has_value()) {
      return readyFuture(CalendarEventMutationResult(
          validationError(QStringLiteral("Bulk event shift is invalid"))));
    }
    return service.update(
        {.eventId = event.eventId, .startAt = times->first, .endAt = times->second});
  }
  case CalendarEventBulkAction::ReplaceText: {
    CalendarEventUpdateInput update{.eventId = event.eventId};
    if (replacesField(input, CalendarEventBulkTextField::Title) &&
        event.title.contains(input.findText)) {
      update.title = event.title;
      update.title->replace(input.findText, input.replaceText, Qt::CaseSensitive);
    }
    if (replacesField(input, CalendarEventBulkTextField::Description) &&
        event.description.value_or(QString()).contains(input.findText)) {
      QString value = event.description.value_or(QString());
      value.replace(input.findText, input.replaceText, Qt::CaseSensitive);
      update.description = std::optional<std::optional<QString>>(std::move(value));
    }
    if (replacesField(input, CalendarEventBulkTextField::Location) &&
        event.location.value_or(QString()).contains(input.findText)) {
      QString value = event.location.value_or(QString());
      value.replace(input.findText, input.replaceText, Qt::CaseSensitive);
      update.location = std::optional<std::optional<QString>>(std::move(value));
    }
    if (!isRecurring(event)) {
      return service.update(std::move(update));
    }
    const auto scope = input.recurrenceScope == 1 ? CalendarEventRecurrenceScope::ThisInstance
                       : input.recurrenceScope == 2 ? CalendarEventRecurrenceScope::ThisAndFollowing
                                                    : CalendarEventRecurrenceScope::FullSeries;
    return service.updateScoped({.update = std::move(update), .scope = scope});
  }
  }
  return readyFuture(
      CalendarEventMutationResult(validationError(QStringLiteral("Bulk event action is invalid"))));
}

void recordResult(CalendarEventBulkMutationSummary& summary,
                  CalendarEventBulkMutationItem& item,
                  CalendarEventMutationResult result) {
  if (std::holds_alternative<CalendarEventMutationReceipt>(result)) {
    item.outcome = CalendarEventBulkItemOutcome::Queued;
    item.message = QStringLiteral("Queued for Google sync");
    ++summary.queued;
    return;
  }
  const AppError& error = std::get<AppError>(result);
  if (error.code() == AppErrorCode::Validation || error.code() == AppErrorCode::Cancelled) {
    item.outcome = CalendarEventBulkItemOutcome::Skipped;
    item.message = error.message();
    ++summary.skipped;
    return;
  }
  item.outcome = CalendarEventBulkItemOutcome::Failed;
  item.message = error.message();
  ++summary.failed;
}

} // namespace

CalendarEventBulkMutationService::CalendarEventBulkMutationService(
    CalendarMutationService& calendarMutationService)
    : calendarMutationService_(calendarMutationService) {}

std::future<CalendarEventBulkMutationResult>
CalendarEventBulkMutationService::execute(CalendarEventBulkMutationInput input) {
  if (const std::optional<AppError> error = validate(input); error.has_value()) {
    return readyFuture(CalendarEventBulkMutationResult(*error));
  }
  try {
    return std::async(std::launch::async,
                      [this, input = std::move(input)]() mutable -> CalendarEventBulkMutationResult {
                        CalendarEventMutationSnapshotResult inspected =
                            calendarMutationService_.inspect(input.eventIds).get();
                        if (std::holds_alternative<AppError>(inspected)) {
                          return std::get<AppError>(std::move(inspected));
                        }
                        QHash<QString, CalendarEventMutationSnapshot> snapshots;
                        for (CalendarEventMutationSnapshot& event :
                             std::get<QList<CalendarEventMutationSnapshot>>(inspected)) {
                          snapshots.insert(event.eventId, std::move(event));
                        }
                        CalendarEventBulkMutationSummary summary{
                            .requested = static_cast<int>(input.eventIds.size())};
                        summary.items.reserve(input.eventIds.size());
                        struct PendingWrite final {
                          qsizetype itemIndex;
                          std::future<CalendarEventMutationResult> future;
                        };
                        std::deque<PendingWrite> pending;
                        const auto collectFirst = [&summary, &pending] {
                          PendingWrite write = std::move(pending.front());
                          pending.pop_front();
                          recordResult(summary, summary.items[write.itemIndex], write.future.get());
                        };
                        QList<QString> targetIds = input.eventIds;
                        if (input.action == CalendarEventBulkAction::ReplaceText &&
                            input.recurrenceScope >= 2) {
                          QHash<QString, QString> representativeBySeries;
                          for (const QString& eventId : input.eventIds) {
                            const auto event = snapshots.constFind(eventId);
                            if (event == snapshots.cend() || !isRecurring(*event)) {
                              continue;
                            }
                            const QString seriesId = event->calendarId + QChar::Null +
                                                     event->recurringRemoteId.value_or(event->remoteId);
                            const auto current = representativeBySeries.constFind(seriesId);
                            if (current == representativeBySeries.cend()) {
                              representativeBySeries.insert(seriesId, eventId);
                              continue;
                            }
                            const auto prior = snapshots.constFind(*current);
                            if (input.recurrenceScope == 2 && prior != snapshots.cend() &&
                                event->originalStartAt.value_or(QString()) <
                                    prior->originalStartAt.value_or(QString())) {
                              representativeBySeries[seriesId] = eventId;
                            }
                          }
                          QSet<QString> targets(targetIds.cbegin(), targetIds.cend());
                          for (auto selected = representativeBySeries.cbegin();
                               selected != representativeBySeries.cend(); ++selected) {
                            for (const QString& eventId : input.eventIds) {
                              const auto event = snapshots.constFind(eventId);
                              if (event == snapshots.cend() || !isRecurring(*event)) {
                                continue;
                              }
                              const QString seriesId = event->calendarId + QChar::Null +
                                                       event->recurringRemoteId.value_or(event->remoteId);
                              if (seriesId == selected.key() && eventId != selected.value()) {
                                targets.remove(eventId);
                              }
                            }
                          }
                          targetIds = targets.values();
                          std::sort(targetIds.begin(), targetIds.end());
                        }
                        for (const QString& eventId : targetIds) {
                          CalendarEventBulkMutationItem item{.eventId = eventId};
                          const auto event = snapshots.constFind(eventId);
                          if (event == snapshots.cend()) {
                            item.message = QStringLiteral("Event is no longer available");
                            ++summary.skipped;
                            summary.items.append(std::move(item));
                            continue;
                          }
                          if (const std::optional<QString> reason = ineligibility(input, *event);
                              reason.has_value()) {
                            item.message = *reason;
                            ++summary.skipped;
                            summary.items.append(std::move(item));
                            continue;
                          }
                          const qsizetype itemIndex = summary.items.size();
                          summary.items.append(std::move(item));
                          ++summary.eligible;
                          if (input.previewOnly) {
                            summary.items[itemIndex].message = QStringLiteral("Matches find text");
                            continue;
                          }
                          pending.push_back({.itemIndex = itemIndex,
                                             .future = submit(calendarMutationService_, input, *event)});
                          if (pending.size() >= static_cast<std::size_t>(kMaximumInFlightWrites)) {
                            collectFirst();
                          }
                        }
                        while (!pending.empty()) {
                          collectFirst();
                        }
                        return summary;
                      });
  } catch (...) {
    return readyFuture(CalendarEventBulkMutationResult(
        AppError(AppErrorCode::Database, QStringLiteral("Bulk event mutation could not start"))));
  }
}

} // namespace hcb
