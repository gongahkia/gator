#include "core/GoogleDriveFilePickerClient.h"

#include "core/GoogleHttpClient.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QUrl>

#include <future>
#include <optional>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumQueryLength = 256;
constexpr qsizetype kMaximumFileCount = 50;
constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumNameLength = 1'024;
constexpr qsizetype kMaximumMimeTypeLength = 256;
constexpr qsizetype kMaximumUrlLength = 2'048;

[[nodiscard]] GoogleApiError invalidInput() {
  return GoogleApiError({.kind = GoogleApiErrorKind::InvalidPayload,
                         .message = QStringLiteral("Google Drive picker input is invalid")});
}

[[nodiscard]] GoogleApiError invalidResponse() {
  return GoogleApiError({.kind = GoogleApiErrorKind::InvalidPayload,
                         .message = QStringLiteral("Google Drive picker response is invalid")});
}

[[nodiscard]] bool validText(const QString& value, qsizetype maximum) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximum &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool validUrl(const QString& value) {
  const QUrl url(value);
  return value.size() <= kMaximumUrlLength && url.isValid() &&
         url.scheme() == QStringLiteral("https") && !url.host().isEmpty();
}

[[nodiscard]] QString driveQueryLiteral(QString value) {
  value.replace(u'\\', QStringLiteral("\\\\"));
  value.replace(u'\'', QStringLiteral("\\'"));
  return value;
}

[[nodiscard]] std::optional<QList<GoogleDriveAttachmentCandidate>>
decode(const GoogleHttpResponse& response) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(response.body, &error);
  if (error.error != QJsonParseError::NoError || !document.isObject()) {
    return std::nullopt;
  }
  const QJsonValue filesValue = document.object().value(QStringLiteral("files"));
  if (!filesValue.isArray() || filesValue.toArray().size() > kMaximumFileCount) {
    return std::nullopt;
  }
  QList<GoogleDriveAttachmentCandidate> result;
  result.reserve(filesValue.toArray().size());
  for (const QJsonValue& fileValue : filesValue.toArray()) {
    if (!fileValue.isObject()) {
      return std::nullopt;
    }
    const QJsonObject file = fileValue.toObject();
    const QJsonValue id = file.value(QStringLiteral("id"));
    const QJsonValue name = file.value(QStringLiteral("name"));
    const QJsonValue mimeType = file.value(QStringLiteral("mimeType"));
    const QJsonValue webViewLink = file.value(QStringLiteral("webViewLink"));
    const QJsonValue iconLink = file.value(QStringLiteral("iconLink"));
    if (!id.isString() || !name.isString() || !mimeType.isString() || !webViewLink.isString() ||
        !validText(id.toString(), kMaximumIdentifierLength) ||
        !validText(name.toString(), kMaximumNameLength) ||
        !validText(mimeType.toString(), kMaximumMimeTypeLength) ||
        !validUrl(webViewLink.toString()) ||
        (!iconLink.isUndefined() && (!iconLink.isString() || !validUrl(iconLink.toString())))) {
      return std::nullopt;
    }
    result.append({.id = id.toString(),
                   .name = name.toString(),
                   .mimeType = mimeType.toString(),
                   .webViewLink = webViewLink.toString(),
                   .iconLink = iconLink.isString() ? std::optional<QString>(iconLink.toString())
                                                   : std::optional<QString>{}});
  }
  return result;
}

} // namespace

GoogleDriveFilePickerClient::GoogleDriveFilePickerClient(GoogleHttpClient& httpClient)
    : httpClient_(httpClient) {}

std::future<GoogleDriveAttachmentCandidatesOrError>
GoogleDriveFilePickerClient::search(QString query, QString accessToken) {
  query = query.trimmed();
  if (query.size() > kMaximumQueryLength || query.contains(QChar::Null)) {
    std::promise<GoogleDriveAttachmentCandidatesOrError> completion;
    std::future<GoogleDriveAttachmentCandidatesOrError> future = completion.get_future();
    completion.set_value(invalidInput());
    return future;
  }
  return std::async(
      std::launch::async, [this, query = std::move(query), accessToken = std::move(accessToken)] {
        GoogleHttpRequest request{
            .method = GoogleHttpMethod::Get,
            .path = QStringLiteral("/drive/v3/files"),
            .query = {{.name = QStringLiteral("pageSize"), .value = QStringLiteral("50")},
                      {.name = QStringLiteral("orderBy"),
                       .value = QStringLiteral("modifiedTime desc")},
                      {.name = QStringLiteral("fields"),
                       .value = QStringLiteral("files(id,name,mimeType,webViewLink,iconLink)")},
                      {.name = QStringLiteral("includeItemsFromAllDrives"),
                       .value = QStringLiteral("true")},
                      {.name = QStringLiteral("supportsAllDrives"),
                       .value = QStringLiteral("true")}},
        };
        QString filter = QStringLiteral("trashed = false");
        if (!query.isEmpty()) {
          filter.prepend(QStringLiteral("name contains '") + driveQueryLiteral(query) +
                         QStringLiteral("' and "));
        }
        request.query.append({.name = QStringLiteral("q"), .value = filter});
        GoogleHttpResult response = httpClient_.send(std::move(request), accessToken).get();
        if (std::holds_alternative<GoogleApiError>(response)) {
          return GoogleDriveAttachmentCandidatesOrError(
              std::get<GoogleApiError>(std::move(response)));
        }
        const std::optional<QList<GoogleDriveAttachmentCandidate>> files =
            decode(std::get<GoogleHttpResponse>(response));
        return files.has_value() ? GoogleDriveAttachmentCandidatesOrError(*files)
                                 : GoogleDriveAttachmentCandidatesOrError(invalidResponse());
      });
}

} // namespace hcb
