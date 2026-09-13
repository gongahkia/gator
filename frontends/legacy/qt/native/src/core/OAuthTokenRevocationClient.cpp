#include "core/OAuthTokenRevocationClient.h"

#include <QJsonDocument>
#include <QJsonObject>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QUrlQuery>

#include <atomic>
#include <future>
#include <memory>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumTokenLength = 8'192;

struct Completion final {
  std::atomic_bool completed{false};
  std::promise<OAuthTokenRevocationResult> promise;
};

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] AppError networkError(QString message) {
  return AppError(AppErrorCode::Network, std::move(message));
}

[[nodiscard]] AppError cancelledError(QString message) {
  return AppError(AppErrorCode::Cancelled, std::move(message));
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

void complete(const std::shared_ptr<Completion>& completion, OAuthTokenRevocationResult result) {
  bool expected = false;
  if (completion->completed.compare_exchange_strong(expected, true)) {
    completion->promise.set_value(std::move(result));
  }
}

[[nodiscard]] bool isValidToken(const QString& token) {
  return !token.isEmpty() && token.size() <= kMaximumTokenLength && !token.contains(QChar::Null);
}

[[nodiscard]] bool isAlreadyInvalidated(int status, const QByteArray& responseBody) {
  if (status != 400) {
    return false;
  }
  const QJsonDocument document = QJsonDocument::fromJson(responseBody);
  return document.isObject() && document.object().value(QStringLiteral("error")).toString() ==
                                    QStringLiteral("invalid_token");
}

} // namespace

OAuthTokenRevocationClient::OAuthTokenRevocationClient(QObject* parent,
                                                       QNetworkAccessManager* manager)
    : QObject(parent), manager_(manager != nullptr ? manager : new QNetworkAccessManager(this)) {}

std::future<OAuthTokenRevocationResult> OAuthTokenRevocationClient::revoke(QString token) {
  if (!isValidToken(token)) {
    return readyFuture(OAuthTokenRevocationResult(
        validationError(QStringLiteral("OAuth token revocation input is invalid"))));
  }
  auto completion = std::make_shared<Completion>();
  std::future<OAuthTokenRevocationResult> future = completion->promise.get_future();
  connect(this, &QObject::destroyed, [completion] {
    complete(completion,
             OAuthTokenRevocationResult(
                 cancelledError(QStringLiteral("OAuth token revocation was cancelled"))));
  });
  if (!QMetaObject::invokeMethod(
          this,
          [this, token = std::move(token), completion] {
            QUrlQuery form;
            form.addQueryItem(QStringLiteral("token"), token);
            QNetworkRequest request(defaultRevocationEndpoint());
            request.setHeader(QNetworkRequest::ContentTypeHeader,
                              QStringLiteral("application/x-www-form-urlencoded"));
            request.setRawHeader("Accept", "application/json");
            QNetworkReply* const reply =
                manager_->post(request, form.toString(QUrl::FullyEncoded).toUtf8());
            connect(reply, &QNetworkReply::finished, reply, [reply, completion] {
              const QByteArray responseBody = reply->readAll();
              const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
              complete(completion,
                       reply->error() == QNetworkReply::NoError || status == 400
                           ? decodeResponse(status, responseBody)
                           : OAuthTokenRevocationResult(
                                 networkError(QStringLiteral("OAuth token revocation failed"))));
              reply->deleteLater();
            });
            connect(reply, &QObject::destroyed, [completion] {
              complete(completion,
                       OAuthTokenRevocationResult(
                           cancelledError(QStringLiteral("OAuth token revocation was cancelled"))));
            });
          },
          Qt::QueuedConnection)) {
    complete(completion,
             OAuthTokenRevocationResult(
                 cancelledError(QStringLiteral("OAuth token revocation was cancelled"))));
  }
  return future;
}

QUrl OAuthTokenRevocationClient::defaultRevocationEndpoint() {
  return QUrl(QStringLiteral("https://oauth2.googleapis.com/revoke"));
}

OAuthTokenRevocationResult
OAuthTokenRevocationClient::decodeResponse(int status, const QByteArray& responseBody) {
  if (status == 200 || isAlreadyInvalidated(status, responseBody)) {
    return std::monostate{};
  }
  return networkError(QStringLiteral("OAuth token revocation failed"));
}

} // namespace hcb
