#include "core/GoogleCalendarListPullClient.h"

#include "core/GoogleHttpClient.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSet>

#include <future>
#include <memory>
#include <thread>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumResponseBytes = static_cast<qsizetype>(4) * 1024 * 1024;
constexpr qsizetype kMaximumCalendarCount = 20'000;
constexpr qsizetype kMaximumCalendarIdLength = 256;
constexpr qsizetype kMaximumCalendarTitleLength = 500;
constexpr qsizetype kMaximumDescriptionLength = 20'000;
constexpr qsizetype kMaximumTimeZoneLength = 120;
constexpr qsizetype kMaximumColorLength = 7;
constexpr qsizetype kMaximumColorIdLength = 32;
constexpr qsizetype kMaximumEtagLength = 4'096;
constexpr qsizetype kMaximumPageTokenLength = 8'192;
constexpr qsizetype kMaximumSyncTokenLength = 8'192;
constexpr int kMaximumPages = 100;
constexpr int kMaximumReminderMinutes = 40'320;

struct DecodedCalendarListPage final {
  QList<GoogleCalendarMirror> calendars;
  std::optional<QString> nextPageToken;
  std::optional<QString> nextSyncToken;
};

using DecodedCalendarListPageOrError = std::variant<DecodedCalendarListPage, GoogleApiError>;

[[nodiscard]] GoogleApiError invalidPayloadError() {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::InvalidPayload,
       .message = QStringLiteral("Google calendar-list response payload is invalid")});
}

[[nodiscard]] GoogleApiError transportError() {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::Transport,
       .message = QStringLiteral("Google calendar-list pull failed before completion")});
}

[[nodiscard]] GoogleApiError invalidRequestError() {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::InvalidPayload,
       .message = QStringLiteral("Google calendar-list pull request is invalid")});
}

[[nodiscard]] bool isPresent(const QJsonValue& value) {
  return !value.isUndefined() && !value.isNull();
}

[[nodiscard]] std::optional<QJsonArray> defaultReminders(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("defaultReminders"));
  if (!isPresent(value)) {
    return QJsonArray();
  }
  if (!value.isArray() || value.toArray().size() > 5) {
    return std::nullopt;
  }
  const QJsonArray reminders = value.toArray();
  for (const QJsonValue& reminderValue : reminders) {
    if (!reminderValue.isObject()) {
      return std::nullopt;
    }
    const QJsonObject reminder = reminderValue.toObject();
    const QJsonValue method = reminder.value(QStringLiteral("method"));
    const QJsonValue minutes = reminder.value(QStringLiteral("minutes"));
    if (!method.isString() || (method.toString() != QStringLiteral("popup") &&
                               method.toString() != QStringLiteral("email")) ||
        !minutes.isDouble() || minutes.toInt(-1) < 0 ||
        minutes.toInt(-1) > kMaximumReminderMinutes) {
      return std::nullopt;
    }
  }
  return reminders;
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumCalendarIdLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] std::optional<QString>
optionalString(const QJsonObject& object, QStringView key, qsizetype maximumLength) {
  const QJsonValue value = object.value(key);
  if (!isPresent(value)) {
    return std::optional<QString>{};
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString text = value.toString();
  return text.size() <= maximumLength && !text.contains(QChar::Null) ? std::optional<QString>(text)
                                                                     : std::nullopt;
}

[[nodiscard]] bool isValidSyncToken(const std::optional<QString>& token) {
  return !token.has_value() || (!token->isEmpty() && token->size() <= kMaximumSyncTokenLength &&
                                !token->contains(QChar::Null));
}

[[nodiscard]] std::optional<QString> normalizedTitle(const QJsonObject& object) {
  const QJsonValue overrideValue = object.value(QStringLiteral("summaryOverride"));
  const QJsonValue summaryValue = object.value(QStringLiteral("summary"));
  const QJsonValue titleValue = isPresent(overrideValue) ? overrideValue : summaryValue;
  if (!isPresent(titleValue)) {
    return QStringLiteral("Untitled calendar");
  }
  if (!titleValue.isString()) {
    return std::nullopt;
  }
  const QString title = titleValue.toString();
  if (title.trimmed().isEmpty()) {
    return QStringLiteral("Untitled calendar");
  }
  return title.size() <= kMaximumCalendarTitleLength && !title.contains(QChar::Null)
             ? std::optional<QString>(title)
             : std::nullopt;
}

[[nodiscard]] std::optional<bool>
booleanOrDefault(const QJsonObject& object, QStringView key, bool defaultValue) {
  const QJsonValue value = object.value(key);
  if (!isPresent(value)) {
    return defaultValue;
  }
  return value.isBool() ? std::optional<bool>(value.toBool()) : std::nullopt;
}

[[nodiscard]] std::optional<GoogleCalendarAccessRole>
calendarAccessRole(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("accessRole"));
  if (!isPresent(value)) {
    return std::optional<GoogleCalendarAccessRole>{};
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString role = value.toString();
  if (role == QStringLiteral("freeBusyReader")) {
    return GoogleCalendarAccessRole::FreeBusyReader;
  }
  if (role == QStringLiteral("reader")) {
    return GoogleCalendarAccessRole::Reader;
  }
  if (role == QStringLiteral("writer")) {
    return GoogleCalendarAccessRole::Writer;
  }
  if (role == QStringLiteral("owner")) {
    return GoogleCalendarAccessRole::Owner;
  }
  return std::nullopt;
}

[[nodiscard]] bool isValidColor(const std::optional<QString>& color) {
  if (!color.has_value()) {
    return true;
  }
  if (color->size() != kMaximumColorLength || !color->startsWith(u'#')) {
    return false;
  }
  for (const QChar character : color->sliced(1)) {
    if (!character.isDigit() && !(character >= u'a' && character <= u'f') &&
        !(character >= u'A' && character <= u'F')) {
      return false;
    }
  }
  return true;
}

[[nodiscard]] DecodedCalendarListPageOrError decodePage(const QByteArray& responseBody) {
  if (responseBody.size() > kMaximumResponseBytes) {
    return invalidPayloadError();
  }
  const QJsonDocument document = QJsonDocument::fromJson(responseBody);
  if (!document.isObject()) {
    return invalidPayloadError();
  }
  const QJsonObject response = document.object();
  const QJsonValue itemsValue = response.value(QStringLiteral("items"));
  if (isPresent(itemsValue) && !itemsValue.isArray()) {
    return invalidPayloadError();
  }
  std::optional<QString> nextPageToken =
      optionalString(response, u"nextPageToken", kMaximumPageTokenLength);
  std::optional<QString> nextSyncToken =
      optionalString(response, u"nextSyncToken", kMaximumSyncTokenLength);
  if ((!nextPageToken.has_value() && isPresent(response.value(QStringLiteral("nextPageToken")))) ||
      (!nextSyncToken.has_value() && isPresent(response.value(QStringLiteral("nextSyncToken")))) ||
      (nextPageToken.has_value() && nextPageToken->isEmpty()) ||
      (nextSyncToken.has_value() && nextSyncToken->isEmpty()) ||
      (nextPageToken.has_value() && nextSyncToken.has_value())) {
    return invalidPayloadError();
  }
  const QJsonArray items = itemsValue.isArray() ? itemsValue.toArray() : QJsonArray();
  if (items.size() > 250) {
    return invalidPayloadError();
  }
  QSet<QString> seenIds;
  QList<GoogleCalendarMirror> calendars;
  calendars.reserve(items.size());
  for (const auto& itemValue : items) {
    if (!itemValue.isObject()) {
      return invalidPayloadError();
    }
    const QJsonObject item = itemValue.toObject();
    const QJsonValue idValue = item.value(QStringLiteral("id"));
    if (!idValue.isString() || !isValidIdentifier(idValue.toString()) ||
        seenIds.contains(idValue.toString())) {
      return invalidPayloadError();
    }
    const std::optional<QString> title = normalizedTitle(item);
    const std::optional<QString> description =
        optionalString(item, u"description", kMaximumDescriptionLength);
    const std::optional<QString> timeZone =
        optionalString(item, u"timeZone", kMaximumTimeZoneLength);
    const std::optional<QString> colorId = optionalString(item, u"colorId", kMaximumColorIdLength);
    const std::optional<QString> backgroundColor =
        optionalString(item, u"backgroundColor", kMaximumColorLength);
    const std::optional<QString> foregroundColor =
        optionalString(item, u"foregroundColor", kMaximumColorLength);
    const std::optional<QString> etag = optionalString(item, u"etag", kMaximumEtagLength);
    const std::optional<QJsonArray> reminders = defaultReminders(item);
    const std::optional<GoogleCalendarAccessRole> accessRole = calendarAccessRole(item);
    const std::optional<bool> selected = booleanOrDefault(item, u"selected", true);
    const std::optional<bool> hidden = booleanOrDefault(item, u"hidden", false);
    const std::optional<bool> primary = booleanOrDefault(item, u"primary", false);
    const std::optional<bool> deleted = booleanOrDefault(item, u"deleted", false);
    if (!title.has_value() ||
        (!description.has_value() && isPresent(item.value(QStringLiteral("description")))) ||
        (!timeZone.has_value() && isPresent(item.value(QStringLiteral("timeZone")))) ||
        (!colorId.has_value() && isPresent(item.value(QStringLiteral("colorId")))) ||
        (!backgroundColor.has_value() &&
         isPresent(item.value(QStringLiteral("backgroundColor")))) ||
        (!foregroundColor.has_value() &&
         isPresent(item.value(QStringLiteral("foregroundColor")))) ||
        (!etag.has_value() && isPresent(item.value(QStringLiteral("etag")))) ||
        (!accessRole.has_value() && isPresent(item.value(QStringLiteral("accessRole")))) ||
        !selected.has_value() || !hidden.has_value() || !primary.has_value() ||
        !deleted.has_value() || !reminders.has_value() || !isValidColor(backgroundColor) ||
        !isValidColor(foregroundColor)) {
      return invalidPayloadError();
    }
    seenIds.insert(idValue.toString());
    calendars.append({.id = idValue.toString(),
                      .title = *title,
                      .description = description,
                      .timeZone = timeZone,
                      .colorId = colorId,
                      .backgroundColor = backgroundColor,
                      .foregroundColor = foregroundColor,
                      .accessRole = accessRole,
                      .selected = *selected,
                      .hidden = *hidden,
                      .primary = *primary,
                      .deleted = *deleted,
                      .etag = etag,
                      .defaultReminders = *reminders});
  }
  return DecodedCalendarListPage{.calendars = std::move(calendars),
                                 .nextPageToken = nextPageToken,
                                 .nextSyncToken = nextSyncToken};
}

[[nodiscard]] GoogleHttpRequest requestForPage(const GoogleCalendarListPullRequest& request,
                                               const std::optional<QString>& pageToken) {
  GoogleHttpRequest httpRequest;
  httpRequest.path = QStringLiteral("/calendar/v3/users/me/calendarList");
  httpRequest.query = {
      {.name = QStringLiteral("maxResults"), .value = QStringLiteral("250")},
      {.name = QStringLiteral("showDeleted"), .value = QStringLiteral("true")},
      {.name = QStringLiteral("showHidden"), .value = QStringLiteral("true")},
      {.name = QStringLiteral("fields"),
       .value = QStringLiteral(
           "nextPageToken,nextSyncToken,items(id,summary,summaryOverride,description,timeZone,colorId,"
           "backgroundColor,foregroundColor,accessRole,selected,hidden,primary,deleted,etag,"
           "defaultReminders)")}};
  if (request.syncToken.has_value()) {
    httpRequest.query.append({.name = QStringLiteral("syncToken"), .value = *request.syncToken});
  }
  if (pageToken.has_value()) {
    httpRequest.query.append({.name = QStringLiteral("pageToken"), .value = *pageToken});
  }
  return httpRequest;
}

} // namespace

GoogleCalendarListPullClient::GoogleCalendarListPullClient(GoogleHttpClient& httpClient)
    : httpClient_(httpClient) {}

std::future<GoogleCalendarListPullResultOrError>
GoogleCalendarListPullClient::list(GoogleCalendarListPullRequest request,
                                   const QString& accessToken) {
  if (!isValidSyncToken(request.syncToken)) {
    std::promise<GoogleCalendarListPullResultOrError> completion;
    std::future<GoogleCalendarListPullResultOrError> future = completion.get_future();
    completion.set_value(invalidRequestError());
    return future;
  }
  auto completion = std::make_shared<std::promise<GoogleCalendarListPullResultOrError>>();
  std::future<GoogleCalendarListPullResultOrError> future = completion->get_future();
  try {
    std::thread([this, request = std::move(request), accessToken, completion] {
      try {
        QList<GoogleCalendarMirror> calendars;
        calendars.reserve(kMaximumCalendarCount);
        QSet<QString> seenIds;
        std::optional<QString> pageToken;
        std::optional<QString> serverDate;
        for (int page = 0; page < kMaximumPages; ++page) {
          GoogleHttpResult response =
              httpClient_.send(requestForPage(request, pageToken), accessToken).get();
          if (std::holds_alternative<GoogleApiError>(response)) {
            completion->set_value(std::get<GoogleApiError>(std::move(response)));
            return;
          }
          GoogleHttpResponse httpResponse = std::get<GoogleHttpResponse>(std::move(response));
          if (!serverDate.has_value()) {
            serverDate = std::move(httpResponse.serverDate);
          }
          DecodedCalendarListPageOrError decoded = decodePage(httpResponse.body);
          if (std::holds_alternative<GoogleApiError>(decoded)) {
            completion->set_value(std::get<GoogleApiError>(std::move(decoded)));
            return;
          }
          DecodedCalendarListPage pageData = std::get<DecodedCalendarListPage>(std::move(decoded));
          if (calendars.size() + pageData.calendars.size() > kMaximumCalendarCount) {
            completion->set_value(invalidPayloadError());
            return;
          }
          for (GoogleCalendarMirror& calendar : pageData.calendars) {
            if (seenIds.contains(calendar.id)) {
              completion->set_value(invalidPayloadError());
              return;
            }
            seenIds.insert(calendar.id);
            calendars.append(std::move(calendar));
          }
          if (!pageData.nextPageToken.has_value()) {
            completion->set_value(
                GoogleCalendarListPullResult{.calendars = std::move(calendars),
                                             .nextSyncToken = pageData.nextSyncToken,
                                             .serverDate = serverDate});
            return;
          }
          pageToken = std::move(pageData.nextPageToken);
        }
        completion->set_value(invalidPayloadError());
      } catch (...) {
        completion->set_value(transportError());
      }
    }).detach();
  } catch (...) {
    completion->set_value(transportError());
  }
  return future;
}

} // namespace hcb
