#include "core/OAuthTokenRefreshClient.h"

#include "core/OAuthTokenExchangeClient.h"
#include "core/SecretRedactor.h"

#include <QJsonDocument>
#include <QJsonObject>
#include <QMetaObject>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QRegularExpression>
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
  std::promise<OAuthTokenRefreshResult> promise;
};

void complete(const std::shared_ptr<Completion>& completion, OAuthTokenRefreshResult result) {
  bool expected = false;
  if (completion->completed.compare_exchange_strong(expected, true)) {
    completion->promise.set_value(std::move(result));
  }
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] AppError networkError(QString message) {
  return AppError(AppErrorCode::Network, std::move(message));
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] bool isValidText(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value.size() <= maximumLength && !value.contains(QChar::Null);
}

[[nodiscard]] bool isValidRequest(const OAuthTokenRefreshRequest& request) {
  return request.clientId.size() >= 10 && request.clientId.size() <= 500 &&
         !request.clientId.contains(QChar::Null) &&
         isValidText(request.refreshToken, kMaximumTokenLength) &&
         (!request.clientSecret.has_value() ||
          isValidText(*request.clientSecret, kMaximumTokenLength));
}

[[nodiscard]] std::optional<QString> oauthErrorCode(const QByteArray& responseBody) {
  const QJsonDocument document = QJsonDocument::fromJson(responseBody);
  if (!document.isObject()) {
    return std::nullopt;
  }
  const QString code = document.object().value(QStringLiteral("error")).toString();
  static const QRegularExpression validCode(
      QStringLiteral("^[A-Za-z][A-Za-z0-9_-]{0,79}$"));
  return validCode.match(code).hasMatch() ? std::optional<QString>(code) : std::nullopt;
}

[[nodiscard]] std::optional<QString> oauthErrorDescription(const QByteArray& responseBody) {
  const QJsonDocument document = QJsonDocument::fromJson(responseBody);
  if (!document.isObject()) {
    return std::nullopt;
  }
  const QString description = document.object().value(QStringLiteral("error_description")).toString();
  if (description.isEmpty() || description.size() > 500 || description.contains(QChar::Null)) {
    return std::nullopt;
  }
  const QString safeDescription = SecretRedactor::redactText(description, 240);
  return safeDescription.isEmpty() ? std::nullopt : std::optional<QString>(safeDescription);
}

[[nodiscard]] QString tokenRefreshFailureMessage(const QNetworkReply& reply,
                                                 int status,
                                                 const QByteArray& responseBody) {
  if (status >= 100 && status <= 599) {
    const std::optional<QString> code = oauthErrorCode(responseBody);
    const std::optional<QString> description = oauthErrorDescription(responseBody);
    if (code.has_value() && description.has_value()) {
      return QStringLiteral("OAuth token refresh failed (HTTP %1: %2 — %3)")
          .arg(status)
          .arg(*code, *description);
    }
    return code.has_value()
               ? QStringLiteral("OAuth token refresh failed (HTTP %1: %2)").arg(status).arg(*code)
               : QStringLiteral("OAuth token refresh failed (HTTP %1)").arg(status);
  }
  return QStringLiteral("OAuth token refresh failed (network error %1)")
      .arg(static_cast<int>(reply.error()));
}

} // namespace

OAuthTokenRefreshClient::OAuthTokenRefreshClient(QObject* parent, QNetworkAccessManager* manager)
    : QObject(parent), manager_(manager != nullptr ? manager : new QNetworkAccessManager(this)) {}

std::future<OAuthTokenRefreshResult>
OAuthTokenRefreshClient::refresh(OAuthTokenRefreshRequest request) {
  if (!isValidRequest(request)) {
    return readyFuture(OAuthTokenRefreshResult(
        validationError(QStringLiteral("OAuth token refresh input is invalid"))));
  }
  QUrlQuery form;
  form.addQueryItem(QStringLiteral("grant_type"), QStringLiteral("refresh_token"));
  form.addQueryItem(QStringLiteral("refresh_token"), request.refreshToken);
  form.addQueryItem(QStringLiteral("client_id"), request.clientId);
  if (request.clientSecret.has_value()) {
    form.addQueryItem(QStringLiteral("client_secret"), *request.clientSecret);
  }
  const QByteArray body = form.toString(QUrl::FullyEncoded).toUtf8();
  auto completion = std::make_shared<Completion>();
  std::future<OAuthTokenRefreshResult> future = completion->promise.get_future();
  connect(this, &QObject::destroyed, [completion] {
    complete(
        completion,
        OAuthTokenRefreshResult(networkError(QStringLiteral("OAuth token refresh was cancelled"))));
  });
  if (!QMetaObject::invokeMethod(
          this,
          [this, body, completion] {
            QNetworkRequest networkRequest(OAuthTokenExchangeClient::defaultTokenEndpoint());
            networkRequest.setHeader(QNetworkRequest::ContentTypeHeader,
                                     QStringLiteral("application/x-www-form-urlencoded"));
            networkRequest.setRawHeader("Accept", "application/json");
            QNetworkReply* const reply = manager_->post(networkRequest, body);
            if (reply == nullptr) {
              complete(completion,
                       OAuthTokenRefreshResult(
                           networkError(QStringLiteral("OAuth token refresh could not start"))));
              return;
            }
            connect(reply, &QNetworkReply::finished, this, [reply, completion] {
              const QByteArray responseBody = reply->readAll();
              const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
              const OAuthTokenRefreshResult result =
                  reply->error() == QNetworkReply::NoError && status >= 200 && status <= 299
                      ? OAuthTokenRefreshClient::decodeTokenResponse(responseBody)
                      : OAuthTokenRefreshResult(
                            networkError(tokenRefreshFailureMessage(*reply, status, responseBody)));
              complete(completion, result);
              reply->deleteLater();
            });
            connect(reply, &QObject::destroyed, [completion] {
              complete(completion,
                       OAuthTokenRefreshResult(
                           networkError(QStringLiteral("OAuth token refresh was cancelled"))));
            });
          },
          Qt::QueuedConnection)) {
    complete(
        completion,
        OAuthTokenRefreshResult(networkError(QStringLiteral("OAuth token refresh was cancelled"))));
  }
  return future;
}

OAuthTokenRefreshResult
OAuthTokenRefreshClient::decodeTokenResponse(const QByteArray& responseBody) {
  const OAuthTokenExchangeResult decoded =
      OAuthTokenExchangeClient::decodeTokenResponse(responseBody);
  if (std::holds_alternative<AppError>(decoded)) {
    return std::get<AppError>(decoded);
  }
  const OAuthTokenSet& tokens = std::get<OAuthTokenSet>(decoded);
  return OAuthRefreshedToken{.accessToken = tokens.accessToken,
                             .expiresInSeconds = tokens.expiresInSeconds,
                             .scope = tokens.scope,
                             .tokenType = tokens.tokenType};
}

} // namespace hcb
