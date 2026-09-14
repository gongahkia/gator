#include "core/PythonBridgeClient.h"

#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QTimer>
#include <QUrlQuery>

#include <atomic>
#include <memory>
#include <utility>

namespace hcb {
namespace {

constexpr int kApiVersion = 1;
constexpr qsizetype kMaximumDescriptorBytes = 16 * 1024;
constexpr qsizetype kMaximumResponseBytes = 8 * 1024 * 1024;
constexpr qsizetype kMaximumAccountIdLength = 256;
constexpr qsizetype kMaximumCursorLength = 128;
constexpr qsizetype kMaximumListIdLength = 256;
constexpr int kRequestTimeoutMilliseconds = 10'000;

struct Completion final {
  std::atomic_bool completed{false};
  std::promise<PythonBridgeResult> promise;
};

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

void complete(const std::shared_ptr<Completion>& completion, PythonBridgeResult result) {
  bool expected = false;
  if (completion->completed.compare_exchange_strong(expected, true)) {
    completion->promise.set_value(std::move(result));
  }
}

[[nodiscard]] AppError configurationError(QString message) {
  return AppError(AppErrorCode::Configuration, std::move(message));
}

[[nodiscard]] AppError networkError(QString message) {
  return AppError(AppErrorCode::Network, std::move(message));
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] bool isValidIdentifier(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximumLength &&
         !value.contains(QChar::Null) && !value.contains(u'/') && !value.contains(u'\\');
}

[[nodiscard]] bool isValidToken(const QString& value) {
  if (value.size() < 20 || value.size() > 128 || value.contains(QChar::Null)) {
    return false;
  }
  for (const QChar character : value) {
    if (!(character.isLetterOrNumber() || character == u'-' || character == u'_')) {
      return false;
    }
  }
  return true;
}

[[nodiscard]] bool isPrivateDescriptor(const QFileInfo& fileInfo) {
  const QFile::Permissions permissions = fileInfo.permissions();
  return permissions.testFlag(QFile::ReadOwner) && !permissions.testFlag(QFile::ReadGroup) &&
         !permissions.testFlag(QFile::ReadOther) && !permissions.testFlag(QFile::WriteGroup) &&
         !permissions.testFlag(QFile::WriteOther);
}

[[nodiscard]] bool isLoopbackEndpoint(const QUrl& endpoint) {
  return endpoint.isValid() && endpoint.scheme() == QStringLiteral("http") &&
         endpoint.host() == QStringLiteral("127.0.0.1") && endpoint.port() > 0 &&
         endpoint.userInfo().isEmpty() &&
         (endpoint.path().isEmpty() || endpoint.path() == QStringLiteral("/")) &&
         endpoint.query().isEmpty() && endpoint.fragment().isEmpty();
}

[[nodiscard]] std::optional<QUrl> accountPath(const QString& accountId, QStringView suffix) {
  if (!isValidIdentifier(accountId, kMaximumAccountIdLength)) {
    return std::nullopt;
  }
  return QUrl(QStringLiteral("/v1/accounts/") +
              QString::fromLatin1(QUrl::toPercentEncoding(accountId)) + suffix);
}

[[nodiscard]] PythonBridgeResult decodeResponse(int status, const QByteArray& body) {
  if (body.size() > kMaximumResponseBytes) {
    return networkError(QStringLiteral("HCB bridge response exceeded the local size limit"));
  }
  const QJsonDocument document = QJsonDocument::fromJson(body);
  if (!document.isObject()) {
    return networkError(QStringLiteral("HCB bridge returned malformed JSON"));
  }
  const QJsonObject envelope = document.object();
  if (envelope.value(QStringLiteral("api_version")).toInt(-1) != kApiVersion) {
    return networkError(QStringLiteral("HCB bridge protocol version is unsupported"));
  }
  if (status < 200 || status >= 300) {
    const QJsonObject error = envelope.value(QStringLiteral("error")).toObject();
    const QString message = error.value(QStringLiteral("message")).toString();
    return networkError(message.isEmpty() ? QStringLiteral("HCB bridge request failed") : message);
  }
  const QJsonValue data = envelope.value(QStringLiteral("data"));
  if (!data.isObject()) {
    return networkError(QStringLiteral("HCB bridge response has no data object"));
  }
  return data.toObject();
}

} // namespace

PythonBridgeClient::PythonBridgeClient(PythonBridgeConnection connection,
                                       QObject* parent,
                                       QNetworkAccessManager* manager)
    : QObject(parent), connection_(std::move(connection)),
      manager_(manager != nullptr ? manager : new QNetworkAccessManager(this)) {}

PythonBridgeConnectionOrError
PythonBridgeClient::readConnectionDescriptor(const QString& descriptorPath) {
  const QFileInfo fileInfo(descriptorPath);
  if (!fileInfo.exists() || !fileInfo.isFile() || fileInfo.isSymLink() ||
      fileInfo.size() > kMaximumDescriptorBytes || !isPrivateDescriptor(fileInfo)) {
    return configurationError(QStringLiteral("HCB bridge descriptor is unavailable or not private"));
  }
  QFile descriptor(descriptorPath);
  if (!descriptor.open(QIODevice::ReadOnly)) {
    return configurationError(QStringLiteral("HCB bridge descriptor cannot be read"));
  }
  const QJsonDocument document = QJsonDocument::fromJson(descriptor.readAll());
  if (!document.isObject()) {
    return configurationError(QStringLiteral("HCB bridge descriptor is malformed"));
  }
  const QJsonObject object = document.object();
  const QJsonValue version = object.value(QStringLiteral("api_version"));
  const QJsonValue url = object.value(QStringLiteral("url"));
  const QJsonValue token = object.value(QStringLiteral("token"));
  const QUrl endpoint(url.toString());
  if (!version.isDouble() || version.toInt(-1) != kApiVersion || !url.isString() ||
      !token.isString() || !isLoopbackEndpoint(endpoint) || !isValidToken(token.toString())) {
    return configurationError(QStringLiteral("HCB bridge descriptor is invalid"));
  }
  return PythonBridgeConnection{endpoint, token.toString().toUtf8()};
}

std::future<PythonBridgeResult>
PythonBridgeClient::workspace(const QString& accountId, CancellationToken cancellation) {
  const std::optional<QUrl> path = accountPath(accountId, u"/workspace");
  if (!path.has_value()) {
    return readyFuture(PythonBridgeResult(validationError(QStringLiteral("account id is invalid"))));
  }
  return get(*path, cancellation);
}

std::future<PythonBridgeResult>
PythonBridgeClient::taskPage(const QString& accountId,
                             int limit,
                             std::optional<QString> cursor,
                             std::optional<QString> listId,
                             CancellationToken cancellation) {
  const std::optional<QUrl> path = accountPath(accountId, u"/tasks");
  if (!path.has_value() || limit < 1 || limit > 500 ||
      (cursor.has_value() && !isValidIdentifier(*cursor, kMaximumCursorLength)) ||
      (listId.has_value() && !isValidIdentifier(*listId, kMaximumListIdLength))) {
    return readyFuture(PythonBridgeResult(validationError(QStringLiteral("task page request is invalid"))));
  }
  QUrl result = *path;
  QUrlQuery query;
  query.addQueryItem(QStringLiteral("limit"), QString::number(limit));
  if (cursor.has_value()) {
    query.addQueryItem(QStringLiteral("cursor"), *cursor);
  }
  if (listId.has_value()) {
    query.addQueryItem(QStringLiteral("list_id"), *listId);
  }
  result.setQuery(query);
  return get(result, cancellation);
}

std::future<PythonBridgeResult>
PythonBridgeClient::eventRange(const QString& accountId,
                               QDate start,
                               QDate end,
                               std::optional<QString> calendarId,
                               CancellationToken cancellation) {
  const std::optional<QUrl> path = accountPath(accountId, u"/workspace");
  if (!path.has_value() || !start.isValid() || !end.isValid() || end <= start ||
      (calendarId.has_value() && !isValidIdentifier(*calendarId, kMaximumListIdLength))) {
    return readyFuture(PythonBridgeResult(validationError(QStringLiteral("event range request is invalid"))));
  }
  QUrl result = *path;
  QUrlQuery query;
  query.addQueryItem(QStringLiteral("include"), QStringLiteral("events"));
  query.addQueryItem(QStringLiteral("start"), start.toString(Qt::ISODate));
  query.addQueryItem(QStringLiteral("end"), end.toString(Qt::ISODate));
  if (calendarId.has_value()) {
    query.addQueryItem(QStringLiteral("calendar_id"), *calendarId);
  }
  result.setQuery(query);
  return get(result, cancellation);
}

std::future<PythonBridgeResult> PythonBridgeClient::get(QUrl path, CancellationToken cancellation) {
  if (cancellation.stop_requested()) {
    return readyFuture(PythonBridgeResult(networkError(QStringLiteral("HCB bridge request was cancelled"))));
  }
  if (!path.isValid() || !path.path().startsWith(QStringLiteral("/v1/")) ||
      path.fragment().size() > 0) {
    return readyFuture(PythonBridgeResult(validationError(QStringLiteral("HCB bridge path is invalid"))));
  }
  auto completion = std::make_shared<Completion>();
  std::future<PythonBridgeResult> future = completion->promise.get_future();
  if (!QMetaObject::invokeMethod(
          this,
          [this, path = std::move(path), cancellation, completion] {
            QUrl requestUrl = connection_.endpoint;
            requestUrl.setPath(path.path());
            requestUrl.setQuery(path.query());
            QNetworkRequest request(requestUrl);
            request.setRawHeader("Authorization", "Bearer " + connection_.bearerToken);
            request.setRawHeader("Accept", "application/json");
            request.setAttribute(QNetworkRequest::RedirectPolicyAttribute,
                                 QNetworkRequest::ManualRedirectPolicy);
            QNetworkReply* reply = manager_->get(request);
            if (reply == nullptr) {
              complete(completion,
                       PythonBridgeResult(networkError(QStringLiteral("HCB bridge request could not start"))));
              return;
            }
            auto* deadline = new QTimer(reply);
            deadline->setSingleShot(true);
            deadline->start(kRequestTimeoutMilliseconds);
            connect(deadline, &QTimer::timeout, reply, [reply, completion] {
              complete(completion,
                       PythonBridgeResult(networkError(QStringLiteral("HCB bridge request timed out"))));
              reply->abort();
            });
            if (cancellation.stop_possible()) {
              auto* cancellationPoll = new QTimer(reply);
              cancellationPoll->setInterval(25);
              connect(cancellationPoll, &QTimer::timeout, reply, [reply, completion, cancellation] {
                if (!cancellation.stop_requested()) {
                  return;
                }
                complete(completion,
                         PythonBridgeResult(networkError(QStringLiteral("HCB bridge request was cancelled"))));
                reply->abort();
              });
              cancellationPoll->start();
            }
            connect(reply, &QNetworkReply::finished, reply, [reply, completion] {
              const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
              const QByteArray body = reply->readAll();
              if (reply->error() != QNetworkReply::NoError && status == 0) {
                complete(completion,
                         PythonBridgeResult(networkError(QStringLiteral("HCB bridge transport failed"))));
              } else {
                complete(completion, decodeResponse(status, body));
              }
              reply->deleteLater();
            });
          },
          Qt::QueuedConnection)) {
    complete(completion,
             PythonBridgeResult(networkError(QStringLiteral("HCB bridge request could not be queued"))));
  }
  return future;
}

} // namespace hcb
