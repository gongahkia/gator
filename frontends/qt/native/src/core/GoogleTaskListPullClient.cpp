#include "core/GoogleTaskListPullClient.h"

#include "core/GoogleHttpClient.h"

#include <QDateTime>
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

constexpr qsizetype kMaximumResponseBytes = 4 * 1024 * 1024;
constexpr qsizetype kMaximumTaskListCount = 2'000;
constexpr qsizetype kMaximumTaskListIdLength = 256;
constexpr qsizetype kMaximumTaskListTitleLength = 1'024;
constexpr qsizetype kMaximumEtagLength = 4'096;
constexpr qsizetype kMaximumPageTokenLength = 8'192;
constexpr int kMaximumPages = 2;

struct DecodedTaskListPage final {
  QList<GoogleTaskListMirror> taskLists;
  std::optional<QString> nextPageToken;
};

using DecodedTaskListPageOrError = std::variant<DecodedTaskListPage, GoogleApiError>;

[[nodiscard]] GoogleApiError invalidPayloadError() {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::InvalidPayload,
       .message = QStringLiteral("Google task-list response payload is invalid")});
}

[[nodiscard]] GoogleApiError transportError() {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::Transport,
       .message = QStringLiteral("Google task-list pull failed before completion")});
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumTaskListIdLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] std::optional<QString>
optionalString(const QJsonObject& object, QStringView key, qsizetype maximumLength) {
  const QJsonValue value = object.value(key);
  if (value.isUndefined() || value.isNull()) {
    return std::optional<QString>{};
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString text = value.toString();
  if (text.size() > maximumLength || text.contains(QChar::Null)) {
    return std::nullopt;
  }
  return text;
}

[[nodiscard]] std::optional<QString> normalizedTimestamp(const QJsonObject& object,
                                                         QStringView key) {
  const QJsonValue value = object.value(key);
  if (value.isUndefined() || value.isNull()) {
    return std::optional<QString>{};
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString timestamp = value.toString();
  if (timestamp.isEmpty() || timestamp.size() > 64 || timestamp.contains(QChar::Null)) {
    return std::nullopt;
  }
  const QDateTime parsed = QDateTime::fromString(timestamp, Qt::ISODate);
  return parsed.isValid() ? std::optional<QString>(parsed.toUTC().toString(Qt::ISODateWithMs))
                          : std::nullopt;
}

[[nodiscard]] std::optional<QString> normalizedTitle(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("title"));
  if (value.isUndefined() || value.isNull()) {
    return QStringLiteral("Untitled list");
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString title = value.toString();
  if (title.trimmed().isEmpty()) {
    return QStringLiteral("Untitled list");
  }
  return title.size() <= kMaximumTaskListTitleLength && !title.contains(QChar::Null)
             ? std::optional<QString>(title)
             : std::nullopt;
}

[[nodiscard]] DecodedTaskListPageOrError decodePage(const QByteArray& responseBody) {
  if (responseBody.size() > kMaximumResponseBytes) {
    return invalidPayloadError();
  }
  const QJsonDocument document = QJsonDocument::fromJson(responseBody);
  if (!document.isObject()) {
    return invalidPayloadError();
  }
  const QJsonObject response = document.object();
  const QJsonValue itemsValue = response.value(QStringLiteral("items"));
  if (!itemsValue.isUndefined() && !itemsValue.isNull() && !itemsValue.isArray()) {
    return invalidPayloadError();
  }
  const QJsonValue nextPageTokenValue = response.value(QStringLiteral("nextPageToken"));
  std::optional<QString> nextPageToken =
      optionalString(response, u"nextPageToken", kMaximumPageTokenLength);
  if (!nextPageToken.has_value() && !nextPageTokenValue.isUndefined() &&
      !nextPageTokenValue.isNull()) {
    return invalidPayloadError();
  }
  if (nextPageToken.has_value() && nextPageToken->isEmpty()) {
    nextPageToken.reset();
  }
  const QJsonArray items = itemsValue.isArray() ? itemsValue.toArray() : QJsonArray();
  if (items.size() > kMaximumTaskListCount) {
    return invalidPayloadError();
  }
  QSet<QString> seenIds;
  QList<GoogleTaskListMirror> taskLists;
  taskLists.reserve(items.size());
  for (const QJsonValue& itemValue : items) {
    if (!itemValue.isObject()) {
      return invalidPayloadError();
    }
    const QJsonObject item = itemValue.toObject();
    const QJsonValue idValue = item.value(QStringLiteral("id"));
    if (!idValue.isString() || !isValidIdentifier(idValue.toString()) ||
        seenIds.contains(idValue.toString())) {
      return invalidPayloadError();
    }
    const QJsonValue updatedValue = item.value(QStringLiteral("updated"));
    const QJsonValue etagValue = item.value(QStringLiteral("etag"));
    const std::optional<QString> title = normalizedTitle(item);
    const std::optional<QString> updatedAt = normalizedTimestamp(item, u"updated");
    const std::optional<QString> etag = optionalString(item, u"etag", kMaximumEtagLength);
    const bool hasInvalidUpdatedAt =
        !updatedAt.has_value() && !updatedValue.isUndefined() && !updatedValue.isNull();
    const bool hasInvalidEtag =
        !etag.has_value() && !etagValue.isUndefined() && !etagValue.isNull();
    if (!title.has_value() || hasInvalidUpdatedAt || hasInvalidEtag) {
      return invalidPayloadError();
    }
    seenIds.insert(idValue.toString());
    taskLists.append(
        {.id = idValue.toString(), .title = *title, .updatedAt = updatedAt, .etag = etag});
  }
  return DecodedTaskListPage{.taskLists = std::move(taskLists), .nextPageToken = nextPageToken};
}

[[nodiscard]] GoogleHttpRequest requestForPage(const std::optional<QString>& pageToken) {
  GoogleHttpRequest request;
  request.path = QStringLiteral("/tasks/v1/users/@me/lists");
  request.query = {{.name = QStringLiteral("maxResults"), .value = QStringLiteral("1000")},
                   {.name = QStringLiteral("fields"),
                    .value = QStringLiteral("nextPageToken,items(id,title,updated,etag)")}};
  if (pageToken.has_value()) {
    request.query.append({.name = QStringLiteral("pageToken"), .value = *pageToken});
  }
  return request;
}

} // namespace

GoogleTaskListPullClient::GoogleTaskListPullClient(GoogleHttpClient& httpClient)
    : httpClient_(httpClient) {}

std::future<GoogleTaskListPullResultOrError> GoogleTaskListPullClient::list(QString accessToken) {
  auto completion = std::make_shared<std::promise<GoogleTaskListPullResultOrError>>();
  std::future<GoogleTaskListPullResultOrError> future = completion->get_future();
  try {
    std::thread([this, accessToken = std::move(accessToken), completion] {
      try {
        QList<GoogleTaskListMirror> taskLists;
        taskLists.reserve(kMaximumTaskListCount);
        QSet<QString> seenIds;
        std::optional<QString> pageToken;
        std::optional<QString> serverDate;
        for (int page = 0; page < kMaximumPages; ++page) {
          GoogleHttpResult response =
              httpClient_.send(requestForPage(pageToken), accessToken).get();
          if (std::holds_alternative<GoogleApiError>(response)) {
            completion->set_value(std::get<GoogleApiError>(std::move(response)));
            return;
          }
          GoogleHttpResponse httpResponse = std::get<GoogleHttpResponse>(std::move(response));
          if (!serverDate.has_value()) {
            serverDate = std::move(httpResponse.serverDate);
          }
          DecodedTaskListPageOrError decoded = decodePage(httpResponse.body);
          if (std::holds_alternative<GoogleApiError>(decoded)) {
            completion->set_value(std::get<GoogleApiError>(std::move(decoded)));
            return;
          }
          DecodedTaskListPage pageData = std::get<DecodedTaskListPage>(std::move(decoded));
          if (taskLists.size() + pageData.taskLists.size() > kMaximumTaskListCount) {
            completion->set_value(invalidPayloadError());
            return;
          }
          for (GoogleTaskListMirror& taskList : pageData.taskLists) {
            if (seenIds.contains(taskList.id)) {
              completion->set_value(invalidPayloadError());
              return;
            }
            seenIds.insert(taskList.id);
            taskLists.append(std::move(taskList));
          }
          if (!pageData.nextPageToken.has_value()) {
            completion->set_value(GoogleTaskListPullResult{.taskLists = std::move(taskLists),
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
