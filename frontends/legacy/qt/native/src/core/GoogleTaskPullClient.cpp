#include "core/GoogleTaskPullClient.h"

#include "core/GoogleHttpClient.h"

#include <QDate>
#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSet>
#include <QTime>
#include <QTimeZone>

#include <future>
#include <memory>
#include <thread>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumResponseBytes = 2 * 1024 * 1024;
constexpr qsizetype kMaximumTaskCount = 100'000;
constexpr qsizetype kMaximumTaskIdLength = 256;
constexpr qsizetype kMaximumTaskTitleLength = 1'024;
constexpr qsizetype kMaximumTaskNotesLength = 8'192;
constexpr qsizetype kMaximumPositionLength = 256;
constexpr qsizetype kMaximumEtagLength = 4'096;
constexpr qsizetype kMaximumPageTokenLength = 8'192;
constexpr int kMaximumPages = 1'000;

struct DecodedTaskPage final {
  QList<GoogleTaskMirror> tasks;
  std::optional<QString> nextPageToken;
};

using DecodedTaskPageOrError = std::variant<DecodedTaskPage, GoogleApiError>;

[[nodiscard]] GoogleApiError invalidPayloadError() {
  return GoogleApiError({.kind = GoogleApiErrorKind::InvalidPayload,
                         .message = QStringLiteral("Google task response payload is invalid")});
}

[[nodiscard]] GoogleApiError invalidRequestError() {
  return GoogleApiError({.kind = GoogleApiErrorKind::InvalidPayload,
                         .message = QStringLiteral("Google task pull request is invalid")});
}

[[nodiscard]] GoogleApiError transportError() {
  return GoogleApiError({.kind = GoogleApiErrorKind::Transport,
                         .message = QStringLiteral("Google task pull failed before completion")});
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumTaskIdLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isPresent(const QJsonValue& value) {
  return !value.isUndefined() && !value.isNull();
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
  if (text.size() > maximumLength || text.contains(QChar::Null)) {
    return std::nullopt;
  }
  return text;
}

[[nodiscard]] std::optional<QString> normalizedTimestamp(const QJsonObject& object,
                                                         QStringView key) {
  const QJsonValue value = object.value(key);
  if (!isPresent(value)) {
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

[[nodiscard]] std::optional<QString> normalizedDueDate(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("due"));
  if (!isPresent(value)) {
    return std::optional<QString>{};
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString due = value.toString();
  if (due.size() < 10 || due.size() > 64 || due.contains(QChar::Null)) {
    return std::nullopt;
  }
  const QDate date = QDate::fromString(due.left(10), Qt::ISODate);
  return date.isValid()
             ? std::optional<QString>(
                   QDateTime(date, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs))
             : std::nullopt;
}

[[nodiscard]] std::optional<QString> normalizedTitle(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("title"));
  if (!isPresent(value)) {
    return QStringLiteral("Untitled task");
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  const QString title = value.toString();
  if (title.trimmed().isEmpty()) {
    return QStringLiteral("Untitled task");
  }
  return title.size() <= kMaximumTaskTitleLength && !title.contains(QChar::Null)
             ? std::optional<QString>(title)
             : std::nullopt;
}

[[nodiscard]] std::optional<bool> optionalBoolean(const QJsonObject& object, QStringView key) {
  const QJsonValue value = object.value(key);
  if (!isPresent(value)) {
    return false;
  }
  return value.isBool() ? std::optional<bool>(value.toBool()) : std::nullopt;
}

[[nodiscard]] std::optional<bool> isAssignedTask(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("assignmentInfo"));
  if (!isPresent(value)) {
    return false;
  }
  return value.isObject() ? std::optional<bool>(true) : std::nullopt;
}

[[nodiscard]] std::optional<GoogleTaskStatus> taskStatus(const QJsonObject& object) {
  const QJsonValue value = object.value(QStringLiteral("status"));
  if (!isPresent(value)) {
    return GoogleTaskStatus::NeedsAction;
  }
  if (!value.isString()) {
    return std::nullopt;
  }
  if (value.toString() == QStringLiteral("needsAction")) {
    return GoogleTaskStatus::NeedsAction;
  }
  if (value.toString() == QStringLiteral("completed")) {
    return GoogleTaskStatus::Completed;
  }
  return std::nullopt;
}

[[nodiscard]] DecodedTaskPageOrError decodePage(const QByteArray& responseBody,
                                                const QString& taskListId) {
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
  const QJsonValue nextPageTokenValue = response.value(QStringLiteral("nextPageToken"));
  std::optional<QString> nextPageToken =
      optionalString(response, u"nextPageToken", kMaximumPageTokenLength);
  if (!nextPageToken.has_value() && isPresent(nextPageTokenValue)) {
    return invalidPayloadError();
  }
  if (nextPageToken.has_value() && nextPageToken->isEmpty()) {
    nextPageToken.reset();
  }
  const QJsonArray items = itemsValue.isArray() ? itemsValue.toArray() : QJsonArray();
  if (items.size() > 100) {
    return invalidPayloadError();
  }
  QSet<QString> seenIds;
  QList<GoogleTaskMirror> tasks;
  tasks.reserve(items.size());
  for (const QJsonValue& itemValue : items) {
    if (!itemValue.isObject()) {
      return invalidPayloadError();
    }
    const QJsonObject item = itemValue.toObject();
    const QJsonValue idValue = item.value(QStringLiteral("id"));
    const QJsonValue parentValue = item.value(QStringLiteral("parent"));
    const QJsonValue notesValue = item.value(QStringLiteral("notes"));
    const QJsonValue positionValue = item.value(QStringLiteral("position"));
    const QJsonValue etagValue = item.value(QStringLiteral("etag"));
    if (!idValue.isString() || !isValidIdentifier(idValue.toString()) ||
        seenIds.contains(idValue.toString())) {
      return invalidPayloadError();
    }
    const std::optional<QString> parentId = optionalString(item, u"parent", kMaximumTaskIdLength);
    const std::optional<QString> title = normalizedTitle(item);
    const std::optional<QString> notes = optionalString(item, u"notes", kMaximumTaskNotesLength);
    const std::optional<GoogleTaskStatus> status = taskStatus(item);
    const std::optional<QString> dueAt = normalizedDueDate(item);
    const std::optional<QString> completedAt = normalizedTimestamp(item, u"completed");
    const std::optional<bool> deleted = optionalBoolean(item, u"deleted");
    const std::optional<bool> hidden = optionalBoolean(item, u"hidden");
    const std::optional<bool> isAssigned = isAssignedTask(item);
    const std::optional<QString> position =
        optionalString(item, u"position", kMaximumPositionLength);
    const std::optional<QString> etag = optionalString(item, u"etag", kMaximumEtagLength);
    const std::optional<QString> updatedAt = normalizedTimestamp(item, u"updated");
    const bool hasInvalidParent = !parentId.has_value() && isPresent(parentValue);
    const bool hasInvalidNotes = !notes.has_value() && isPresent(notesValue);
    const bool hasInvalidPosition = !position.has_value() && isPresent(positionValue);
    const bool hasInvalidEtag = !etag.has_value() && isPresent(etagValue);
    const bool hasInvalidCompletedAt =
        !completedAt.has_value() && isPresent(item.value(QStringLiteral("completed")));
    const bool hasInvalidDueAt = !dueAt.has_value() && isPresent(item.value(QStringLiteral("due")));
    const bool hasInvalidUpdatedAt =
        !updatedAt.has_value() && isPresent(item.value(QStringLiteral("updated")));
    if ((parentId.has_value() && !isValidIdentifier(*parentId)) || !title.has_value() ||
        !status.has_value() || !deleted.has_value() || !hidden.has_value() || !isAssigned.has_value() ||
        hasInvalidParent ||
        hasInvalidNotes || hasInvalidPosition || hasInvalidEtag || hasInvalidCompletedAt ||
        hasInvalidDueAt || hasInvalidUpdatedAt) {
      return invalidPayloadError();
    }
    seenIds.insert(idValue.toString());
    tasks.append({.id = idValue.toString(),
                  .taskListId = taskListId,
                  .parentId = parentId,
                  .title = *title,
                  .notes = notes,
                  .status = *status,
                  .dueAt = dueAt,
                  .completedAt = completedAt,
                  .deleted = *deleted,
                  .hidden = *hidden,
                  .isAssigned = *isAssigned,
                  .position = position,
                  .etag = etag,
                  .updatedAt = updatedAt});
  }
  return DecodedTaskPage{.tasks = std::move(tasks), .nextPageToken = nextPageToken};
}

[[nodiscard]] bool isValidTimestamp(const std::optional<QString>& value) {
  return !value.has_value() ||
         (!value->isEmpty() && value->size() <= 64 && !value->contains(QChar::Null) &&
          QDateTime::fromString(*value, Qt::ISODate).isValid());
}

[[nodiscard]] bool isValidRequest(const GoogleTaskPullRequest& request) {
  return isValidIdentifier(request.taskListId) && isValidTimestamp(request.updatedMin) &&
         isValidTimestamp(request.completedMin);
}

[[nodiscard]] GoogleHttpRequest requestForPage(const GoogleTaskPullRequest& request,
                                               const std::optional<QString>& pageToken) {
  GoogleHttpRequest httpRequest;
  httpRequest.path =
      QStringLiteral("/tasks/v1/lists/") + request.taskListId + QStringLiteral("/tasks");
  httpRequest.query = {
      {.name = QStringLiteral("showAssigned"), .value = QStringLiteral("true")},
      {.name = QStringLiteral("showCompleted"),
       .value = request.showCompleted ? QStringLiteral("true") : QStringLiteral("false")},
      {.name = QStringLiteral("showDeleted"), .value = QStringLiteral("true")},
      {.name = QStringLiteral("showHidden"), .value = QStringLiteral("true")},
      {.name = QStringLiteral("maxResults"), .value = QStringLiteral("100")},
      {.name = QStringLiteral("fields"),
       .value = QStringLiteral("nextPageToken,items(id,title,notes,status,due,completed,deleted,"
                               "hidden,parent,position,etag,updated,assignmentInfo)")}};
  if (request.updatedMin.has_value()) {
    httpRequest.query.append({.name = QStringLiteral("updatedMin"), .value = *request.updatedMin});
  } else if (request.showCompleted && request.completedMin.has_value()) {
    httpRequest.query.append(
        {.name = QStringLiteral("completedMin"), .value = *request.completedMin});
  }
  if (pageToken.has_value()) {
    httpRequest.query.append({.name = QStringLiteral("pageToken"), .value = *pageToken});
  }
  return httpRequest;
}

} // namespace

GoogleTaskPullClient::GoogleTaskPullClient(GoogleHttpClient& httpClient)
    : httpClient_(httpClient) {}

std::future<GoogleTaskPullResultOrError> GoogleTaskPullClient::list(GoogleTaskPullRequest request,
                                                                    QString accessToken) {
  if (!isValidRequest(request)) {
    std::promise<GoogleTaskPullResultOrError> completion;
    std::future<GoogleTaskPullResultOrError> future = completion.get_future();
    completion.set_value(invalidRequestError());
    return future;
  }
  auto completion = std::make_shared<std::promise<GoogleTaskPullResultOrError>>();
  std::future<GoogleTaskPullResultOrError> future = completion->get_future();
  try {
    std::thread([this,
                 request = std::move(request),
                 accessToken = std::move(accessToken),
                 completion] {
      try {
        QList<GoogleTaskMirror> tasks;
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
          DecodedTaskPageOrError decoded = decodePage(httpResponse.body, request.taskListId);
          if (std::holds_alternative<GoogleApiError>(decoded)) {
            completion->set_value(std::get<GoogleApiError>(std::move(decoded)));
            return;
          }
          DecodedTaskPage pageData = std::get<DecodedTaskPage>(std::move(decoded));
          if (tasks.size() + pageData.tasks.size() > kMaximumTaskCount) {
            completion->set_value(invalidPayloadError());
            return;
          }
          for (GoogleTaskMirror& task : pageData.tasks) {
            if (seenIds.contains(task.id)) {
              completion->set_value(invalidPayloadError());
              return;
            }
            seenIds.insert(task.id);
            tasks.append(std::move(task));
          }
          if (!pageData.nextPageToken.has_value()) {
            completion->set_value(
                GoogleTaskPullResult{.tasks = std::move(tasks), .serverDate = serverDate});
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
