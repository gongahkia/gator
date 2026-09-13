#include "core/GoogleCalendarFreeBusyClient.h"

#include "core/GoogleHttpClient.h"

#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSet>

#include <future>
#include <optional>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumCalendarCount = 50;
constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumTimestampLength = 64;

[[nodiscard]] GoogleApiError invalidInput() {
  return GoogleApiError({.kind = GoogleApiErrorKind::InvalidPayload,
                         .message = QStringLiteral("Google free-busy input is invalid")});
}

[[nodiscard]] GoogleApiError invalidResponse() {
  return GoogleApiError({.kind = GoogleApiErrorKind::InvalidPayload,
                         .message = QStringLiteral("Google free-busy response is invalid")});
}

[[nodiscard]] bool validIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumIdentifierLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] std::optional<QDateTime> timestamp(const QString& value) {
  if (value.size() > kMaximumTimestampLength || !value.contains(u'T')) {
    return std::nullopt;
  }
  const QDateTime parsed = QDateTime::fromString(value, Qt::ISODateWithMs);
  return parsed.isValid() ? std::optional<QDateTime>(parsed.toUTC()) : std::nullopt;
}

[[nodiscard]] std::optional<GoogleCalendarFreeBusyResult>
decode(const GoogleHttpResponse& response) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(response.body, &error);
  if (error.error != QJsonParseError::NoError || !document.isObject()) {
    return std::nullopt;
  }
  const QJsonValue calendarsValue = document.object().value(QStringLiteral("calendars"));
  if (!calendarsValue.isObject()) {
    return std::nullopt;
  }
  const QJsonObject calendars = calendarsValue.toObject();
  GoogleCalendarFreeBusyResult result;
  for (auto calendar = calendars.constBegin(); calendar != calendars.constEnd(); ++calendar) {
    if (!validIdentifier(calendar.key()) || !calendar.value().isObject()) {
      return std::nullopt;
    }
    const QJsonValue busyValue = calendar.value().toObject().value(QStringLiteral("busy"));
    if (!busyValue.isArray()) {
      return std::nullopt;
    }
    QList<GoogleCalendarBusyInterval> intervals;
    intervals.reserve(busyValue.toArray().size());
    for (const QJsonValue& intervalValue : busyValue.toArray()) {
      if (!intervalValue.isObject()) {
        return std::nullopt;
      }
      const QJsonObject interval = intervalValue.toObject();
      const QJsonValue start = interval.value(QStringLiteral("start"));
      const QJsonValue end = interval.value(QStringLiteral("end"));
      const std::optional<QDateTime> startAt =
          start.isString() ? timestamp(start.toString()) : std::nullopt;
      const std::optional<QDateTime> endAt =
          end.isString() ? timestamp(end.toString()) : std::nullopt;
      if (!startAt.has_value() || !endAt.has_value() || *endAt <= *startAt) {
        return std::nullopt;
      }
      intervals.append({.startAt = startAt->toString(Qt::ISODateWithMs),
                        .endAt = endAt->toString(Qt::ISODateWithMs)});
    }
    result.intervalsByCalendar.insert(calendar.key(), std::move(intervals));
  }
  return result;
}

} // namespace

GoogleCalendarFreeBusyClient::GoogleCalendarFreeBusyClient(GoogleHttpClient& httpClient)
    : httpClient_(httpClient) {}

std::future<GoogleCalendarFreeBusyResultOrError>
GoogleCalendarFreeBusyClient::query(GoogleCalendarFreeBusyRequest request, QString accessToken) {
  const std::optional<QDateTime> startAt = timestamp(request.startAt);
  const std::optional<QDateTime> endAt = timestamp(request.endAt);
  QSet<QString> uniqueIds;
  for (const QString& calendarId : request.calendarIds) {
    if (!validIdentifier(calendarId) || uniqueIds.contains(calendarId)) {
      std::promise<GoogleCalendarFreeBusyResultOrError> completion;
      std::future<GoogleCalendarFreeBusyResultOrError> future = completion.get_future();
      completion.set_value(invalidInput());
      return future;
    }
    uniqueIds.insert(calendarId);
  }
  if (!startAt.has_value() || !endAt.has_value() || *endAt <= *startAt ||
      request.calendarIds.isEmpty() || request.calendarIds.size() > kMaximumCalendarCount) {
    std::promise<GoogleCalendarFreeBusyResultOrError> completion;
    std::future<GoogleCalendarFreeBusyResultOrError> future = completion.get_future();
    completion.set_value(invalidInput());
    return future;
  }
  return std::async(
      std::launch::async,
      [this, request = std::move(request), accessToken = std::move(accessToken)] {
        QJsonArray items;
        for (const QString& calendarId : request.calendarIds) {
          items.append(QJsonObject{{QStringLiteral("id"), calendarId}});
        }
        const QJsonObject body{{QStringLiteral("timeMin"), request.startAt},
                               {QStringLiteral("timeMax"), request.endAt},
                               {QStringLiteral("items"), items}};
        GoogleHttpRequest httpRequest{.method = GoogleHttpMethod::Post,
                                      .path = QStringLiteral("/calendar/v3/freeBusy"),
                                      .body = QJsonDocument(body).toJson(QJsonDocument::Compact)};
        GoogleHttpResult response = httpClient_.send(std::move(httpRequest), accessToken).get();
        if (std::holds_alternative<GoogleApiError>(response)) {
          return GoogleCalendarFreeBusyResultOrError(std::get<GoogleApiError>(std::move(response)));
        }
        const std::optional<GoogleCalendarFreeBusyResult> result =
            decode(std::get<GoogleHttpResponse>(response));
        return result.has_value() ? GoogleCalendarFreeBusyResultOrError(*result)
                                  : GoogleCalendarFreeBusyResultOrError(invalidResponse());
      });
}

} // namespace hcb
