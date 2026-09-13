#include "core/GoogleHttpClient.h"

#include <QLocale>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QTimer>
#include <QTimeZone>
#include <QUrlQuery>

#include <algorithm>
#include <atomic>
#include <limits>
#include <memory>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumAccessTokenLength = 8'192;
constexpr qsizetype kMaximumPathLength = 4'096;
constexpr qsizetype kMaximumQueryParameterCount = 100;
constexpr qsizetype kMaximumQueryTextLength = 8'192;
constexpr qsizetype kMaximumIfMatchLength = 8'192;
constexpr qsizetype kMaximumRequestBodyBytes = 10 * 1024 * 1024;
constexpr qsizetype kMaximumServerDateLength = 256;
constexpr qsizetype kMaximumHeaderValueLength = 1'024;
constexpr int kMinimumTimeoutMilliseconds = 1;
constexpr int kMaximumTimeoutMilliseconds = 120'000;

struct Completion final {
  std::atomic_bool completed{false};
  std::promise<GoogleHttpResult> promise;
};

[[nodiscard]] GoogleApiError clientError(QString message) {
  return GoogleApiError(
      {.kind = GoogleApiErrorKind::InvalidPayload, .message = std::move(message)});
}

[[nodiscard]] GoogleApiError transportError(QString message) {
  return GoogleApiError({.kind = GoogleApiErrorKind::Transport, .message = std::move(message)});
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

void complete(const std::shared_ptr<Completion>& completion, GoogleHttpResult result) {
  bool expected = false;
  if (completion->completed.compare_exchange_strong(expected, true)) {
    completion->promise.set_value(std::move(result));
  }
}

[[nodiscard]] bool containsWhitespace(const QString& value) {
  for (const QChar character : value) {
    if (character.isSpace()) {
      return true;
    }
  }
  return false;
}

[[nodiscard]] bool isValidAccessToken(const QString& accessToken) {
  return !accessToken.isEmpty() && accessToken.size() <= kMaximumAccessTokenLength &&
         !accessToken.contains(QChar::Null) && !containsWhitespace(accessToken);
}

[[nodiscard]] bool isValidPath(const QString& path) {
  return !path.isEmpty() && path.startsWith(u'/') && !path.startsWith(QStringLiteral("//")) &&
         path.size() <= kMaximumPathLength && !path.contains(QChar::Null) && !path.contains(u'?') &&
         !path.contains(u'#') && !path.contains(u'\\');
}

[[nodiscard]] bool isValidQueryText(const QString& value, bool required) {
  return (!required || !value.isEmpty()) && value.size() <= kMaximumQueryTextLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isValidIfMatch(const std::optional<QString>& ifMatch) {
  return !ifMatch.has_value() || (!ifMatch->isEmpty() && ifMatch->size() <= kMaximumIfMatchLength &&
                                  !ifMatch->contains(QChar::Null) && !ifMatch->contains(u'\r') &&
                                  !ifMatch->contains(u'\n'));
}

[[nodiscard]] bool isValidHeaderValue(const std::optional<QByteArray>& value) {
  return !value.has_value() ||
         (!value->isEmpty() && value->size() <= kMaximumHeaderValueLength &&
          !value->contains('\0') && !value->contains('\r') && !value->contains('\n'));
}

[[nodiscard]] bool isValidRequest(const GoogleHttpRequest& request) {
  if (!isValidPath(request.path) || request.query.size() > kMaximumQueryParameterCount ||
      !isValidIfMatch(request.ifMatch) ||
      !isValidHeaderValue(request.contentType) || !isValidHeaderValue(request.accept) ||
      (request.contentType.has_value() && !request.body.has_value()) ||
      (request.body.has_value() && request.body->size() > kMaximumRequestBodyBytes) ||
      request.timeoutMilliseconds < kMinimumTimeoutMilliseconds ||
      request.timeoutMilliseconds > kMaximumTimeoutMilliseconds) {
    return false;
  }
  if ((request.method == GoogleHttpMethod::Get || request.method == GoogleHttpMethod::Delete) &&
      request.body.has_value()) {
    return false;
  }
  for (const GoogleHttpQueryParameter& parameter : request.query) {
    if (!isValidQueryText(parameter.name, true) || !isValidQueryText(parameter.value, false)) {
      return false;
    }
  }
  return true;
}

[[nodiscard]] QByteArray methodName(GoogleHttpMethod method) {
  switch (method) {
  case GoogleHttpMethod::Get:
    return "GET";
  case GoogleHttpMethod::Post:
    return "POST";
  case GoogleHttpMethod::Patch:
    return "PATCH";
  case GoogleHttpMethod::Put:
    return "PUT";
  case GoogleHttpMethod::Delete:
    return "DELETE";
  }
  return {};
}

[[nodiscard]] std::optional<QString> serverDate(const QByteArray& header) {
  const QString value = QString::fromLatin1(header).trimmed();
  return value.isEmpty() || value.size() > kMaximumServerDateLength || value.contains(QChar::Null)
             ? std::nullopt
             : std::optional<QString>(value);
}

[[nodiscard]] std::optional<qint64> retryAfterMilliseconds(const QByteArray& header,
                                                           const QDateTime& now) {
  const QByteArray value = header.trimmed();
  if (value.isEmpty()) {
    return std::nullopt;
  }
  bool secondsValid = false;
  const qint64 seconds = QString::fromLatin1(value).toLongLong(&secondsValid);
  if (secondsValid && seconds >= 0) {
    return seconds > std::numeric_limits<qint64>::max() / 1'000
               ? std::optional<qint64>(std::numeric_limits<qint64>::max())
               : std::optional<qint64>(seconds * 1'000);
  }
  const QString dateText = QString::fromLatin1(value);
  QDateTime retryAt = QDateTime::fromString(dateText, Qt::RFC2822Date);
  if (!retryAt.isValid()) {
    retryAt = QLocale::c().toDateTime(dateText, QStringLiteral("ddd, dd MMM yyyy HH:mm:ss 'GMT'"));
    retryAt.setTimeZone(QTimeZone::UTC);
  }
  if (!retryAt.isValid() || !now.isValid()) {
    return std::nullopt;
  }
  return std::max<qint64>(0, now.msecsTo(retryAt.toUTC()));
}

} // namespace

GoogleHttpClient::GoogleHttpClient(QObject* parent, QNetworkAccessManager* manager)
    : QObject(parent), manager_(manager != nullptr ? manager : new QNetworkAccessManager(this)) {}

std::future<GoogleHttpResult> GoogleHttpClient::send(GoogleHttpRequest request,
                                                     QString accessToken,
                                                     CancellationToken cancellation) {
  if (cancellation.stop_requested()) {
    return readyFuture(
        GoogleHttpResult(transportError(QStringLiteral("Google HTTP request was cancelled"))));
  }
  if (!isValidRequest(request) || !isValidAccessToken(accessToken)) {
    return readyFuture(
        GoogleHttpResult(clientError(QStringLiteral("Google HTTP request input is invalid"))));
  }
  const std::optional<QUrl> url = buildUrl(request);
  if (!url.has_value()) {
    return readyFuture(
        GoogleHttpResult(clientError(QStringLiteral("Google HTTP request URL is invalid"))));
  }
  auto completion = std::make_shared<Completion>();
  std::future<GoogleHttpResult> future = completion->promise.get_future();
  connect(this, &QObject::destroyed, [completion] {
    complete(completion,
             GoogleHttpResult(transportError(QStringLiteral("Google HTTP request was cancelled"))));
  });
  if (!QMetaObject::invokeMethod(
          this,
          [this,
           request = std::move(request),
           accessToken = std::move(accessToken),
           url = *url,
           completion,
           cancellation] {
            QNetworkRequest networkRequest(url);
            networkRequest.setAttribute(QNetworkRequest::RedirectPolicyAttribute,
                                        QNetworkRequest::ManualRedirectPolicy);
            networkRequest.setRawHeader("Accept", request.accept.value_or("application/json"));
            networkRequest.setRawHeader("Authorization", "Bearer " + accessToken.toUtf8());
            if (request.ifMatch.has_value()) {
              networkRequest.setRawHeader("If-Match", request.ifMatch->toUtf8());
            }
            if (request.body.has_value()) {
              networkRequest.setRawHeader(
                  "Content-Type", request.contentType.value_or("application/json"));
            }
            const QByteArray body = request.body.value_or(QByteArray());
            QNetworkReply* reply = nullptr;
            switch (request.method) {
            case GoogleHttpMethod::Get:
              reply = manager_->get(networkRequest);
              break;
            case GoogleHttpMethod::Post:
              reply = manager_->post(networkRequest, body);
              break;
            case GoogleHttpMethod::Patch:
              reply = manager_->sendCustomRequest(networkRequest, methodName(request.method), body);
              break;
            case GoogleHttpMethod::Put:
              reply = manager_->put(networkRequest, body);
              break;
            case GoogleHttpMethod::Delete:
              reply = manager_->deleteResource(networkRequest);
              break;
            }
            if (reply == nullptr) {
              complete(completion,
                       GoogleHttpResult(
                           transportError(QStringLiteral("Google HTTP request could not start"))));
              return;
            }
            auto* deadline = new QTimer(reply);
            deadline->setSingleShot(true);
            deadline->start(request.timeoutMilliseconds);
            connect(deadline, &QTimer::timeout, reply, [reply, completion] {
              complete(completion,
                       GoogleHttpResult(
                           transportError(QStringLiteral("Google HTTP request timed out"))));
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
                         GoogleHttpResult(
                             transportError(QStringLiteral("Google HTTP request was cancelled"))));
                reply->abort();
              });
              cancellationPoll->start();
            }
            connect(reply, &QNetworkReply::finished, reply, [reply, completion] {
              const QByteArray responseBody = reply->readAll();
              const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
              const QByteArray retryAfterHeader = reply->rawHeader("Retry-After");
              const QByteArray serverDateHeader = reply->rawHeader("Date");
              complete(completion,
                       status >= 100 && status <= 599
                           ? GoogleHttpClient::decodeResponse(
                                 status, responseBody, retryAfterHeader, serverDateHeader)
                           : GoogleHttpResult(transportError(
                                 QStringLiteral("Google HTTP request failed before a response"))));
              reply->deleteLater();
            });
            connect(reply, &QObject::destroyed, [completion] {
              complete(completion,
                       GoogleHttpResult(
                           transportError(QStringLiteral("Google HTTP request was cancelled"))));
            });
          },
          Qt::QueuedConnection)) {
    complete(completion,
             GoogleHttpResult(transportError(QStringLiteral("Google HTTP request was cancelled"))));
  }
  return future;
}

QUrl GoogleHttpClient::defaultApiEndpoint() {
  return QUrl(QStringLiteral("https://www.googleapis.com"));
}

std::optional<QUrl> GoogleHttpClient::buildUrl(const GoogleHttpRequest& request) {
  if (!isValidRequest(request)) {
    return std::nullopt;
  }
  QUrl url = defaultApiEndpoint();
  url.setPath(request.path, QUrl::DecodedMode);
  QUrlQuery query;
  for (const GoogleHttpQueryParameter& parameter : request.query) {
    query.addQueryItem(parameter.name, parameter.value);
  }
  url.setQuery(query);
  return url.isValid() && url.scheme() == QStringLiteral("https") &&
                 url.host() == QStringLiteral("www.googleapis.com") && url.userInfo().isEmpty() &&
                 url.fragment().isEmpty()
             ? std::optional<QUrl>(url)
             : std::nullopt;
}

GoogleHttpResult GoogleHttpClient::decodeResponse(int status,
                                                  QByteArray responseBody,
                                                  QByteArray retryAfterHeader,
                                                  QByteArray serverDateHeader,
                                                  QDateTime now) {
  if (status >= 200 && status <= 299) {
    return GoogleHttpResponse{.status = status,
                              .body = std::move(responseBody),
                              .serverDate = serverDate(serverDateHeader)};
  }
  return GoogleApiError::fromHttpStatus(
      status, QString::fromUtf8(responseBody), retryAfterMilliseconds(retryAfterHeader, now));
}

} // namespace hcb
