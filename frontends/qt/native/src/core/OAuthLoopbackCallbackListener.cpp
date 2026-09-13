#include "core/OAuthLoopbackCallbackListener.h"

#include <QHostAddress>
#include <QSet>
#include <QTcpServer>
#include <QTcpSocket>
#include <QUrlQuery>

#include <algorithm>
#include <iterator>
#include <limits>
#include <utility>

namespace hcb {
namespace {

constexpr auto kCallbackPath = "/oauth/google/callback";
constexpr qsizetype kMaximumRequestBytes = 16'384;
constexpr qsizetype kMaximumCodeLength = 8'192;
constexpr qsizetype kMaximumStateLength = 512;
constexpr qsizetype kMaximumErrorLength = 1'024;
constexpr qsizetype kMaximumResponseMessageLength = 1'024;

[[nodiscard]] AppError listenerError(QString message) {
  return AppError(AppErrorCode::Network, std::move(message));
}

[[nodiscard]] QString escapeHtml(const QString& value) {
  QString escaped = value;
  escaped.replace(u'&', QStringLiteral("&amp;"));
  escaped.replace(u'<', QStringLiteral("&lt;"));
  escaped.replace(u'>', QStringLiteral("&gt;"));
  escaped.replace(u'\"', QStringLiteral("&quot;"));
  return escaped;
}

[[nodiscard]] QString renderHtml(const QString& message) {
  return QStringLiteral("<!doctype html><meta charset=\"utf-8\"><title>Hot Cross Buns</title>"
                        "<body><main style=\"font-family:system-ui,sans-serif;max-width:36rem;"
                        "margin:4rem auto;line-height:1.5\"><h1>Hot Cross Buns</h1><p>%1</p>"
                        "</main></body>")
      .arg(escapeHtml(message));
}

[[nodiscard]] bool isValidValue(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value.size() <= maximumLength && !value.contains(QChar::Null);
}

[[nodiscard]] std::optional<QString>
singleQueryValue(const QUrlQuery& query, QStringView key, qsizetype maximumLength) {
  const QList<QString> values = query.allQueryItemValues(key.toString());
  if (values.size() != 1 || !isValidValue(values.front(), maximumLength)) {
    return std::nullopt;
  }
  return values.front();
}

[[nodiscard]] bool isValidStatusCode(int statusCode) {
  return statusCode >= 200 && statusCode <= 599;
}

} // namespace

OAuthLoopbackCallbackListener::OAuthLoopbackCallbackListener(QObject* parent)
    : QObject(parent), server_(new QTcpServer(this)) {
  connect(server_,
          &QTcpServer::newConnection,
          this,
          &OAuthLoopbackCallbackListener::handleNewConnection);
}

OAuthLoopbackListenerStartResult OAuthLoopbackCallbackListener::start() {
  stop();
  if (!server_->listen(QHostAddress::LocalHost, 0)) {
    return listenerError(
        QStringLiteral("OAuth loopback listener could not bind: %1").arg(server_->errorString()));
  }
  const QUrl uri(QStringLiteral("http://127.0.0.1:%1%2")
                     .arg(server_->serverPort())
                     .arg(QString::fromLatin1(kCallbackPath)));
  if (!uri.isValid()) {
    stop();
    return listenerError(QStringLiteral("OAuth loopback listener URI is invalid"));
  }
  return uri;
}

void OAuthLoopbackCallbackListener::stop() {
  server_->close();
  QSet<QTcpSocket*> sockets(pendingRequests_.keyBegin(), pendingRequests_.keyEnd());
  for (QTcpSocket* socket : pendingCallbacks_) {
    sockets.insert(socket);
  }
  pendingRequests_.clear();
  pendingCallbacks_.clear();
  for (QTcpSocket* socket : std::as_const(sockets)) {
    if (socket != nullptr) {
      socket->disconnectFromHost();
      socket->deleteLater();
    }
  }
}

bool OAuthLoopbackCallbackListener::isListening() const { return server_->isListening(); }

QUrl OAuthLoopbackCallbackListener::redirectUri() const {
  if (!server_->isListening()) {
    return {};
  }
  return QUrl(QStringLiteral("http://127.0.0.1:%1%2")
                  .arg(server_->serverPort())
                  .arg(QString::fromLatin1(kCallbackPath)));
}

bool OAuthLoopbackCallbackListener::respond(std::uint64_t requestId,
                                            int statusCode,
                                            QString message) {
  if (!isValidStatusCode(statusCode) || message.size() > kMaximumResponseMessageLength ||
      message.contains(QChar::Null)) {
    return false;
  }
  const auto callback = pendingCallbacks_.find(requestId);
  if (callback == pendingCallbacks_.end() || callback.value() == nullptr) {
    return false;
  }
  QTcpSocket* const socket = callback.value();
  pendingCallbacks_.erase(callback);
  sendResponse(socket, statusCode, message);
  return true;
}

void OAuthLoopbackCallbackListener::handleNewConnection() {
  while (server_->hasPendingConnections()) {
    QTcpSocket* const socket = server_->nextPendingConnection();
    if (socket == nullptr) {
      continue;
    }
    pendingRequests_.insert(socket, {});
    connect(socket, &QTcpSocket::readyRead, this, [this, socket] { handleReadyRead(socket); });
    connect(
        socket, &QTcpSocket::disconnected, this, [this, socket] { handleDisconnected(socket); });
  }
}

void OAuthLoopbackCallbackListener::handleReadyRead(QTcpSocket* socket) {
  const auto pending = pendingRequests_.find(socket);
  if (pending == pendingRequests_.end()) {
    return;
  }
  pending.value().append(socket->readAll());
  if (pending.value().size() > kMaximumRequestBytes) {
    pendingRequests_.erase(pending);
    sendResponse(socket, 413, QStringLiteral("OAuth callback request is too large."));
    return;
  }
  const qsizetype headersEnd = pending.value().indexOf("\r\n\r\n");
  if (headersEnd < 0) {
    return;
  }
  const QList<QByteArray> requestLine = pending.value().left(headersEnd).split('\n');
  pendingRequests_.erase(pending);
  if (requestLine.isEmpty()) {
    sendResponse(socket, 400, QStringLiteral("OAuth callback request is invalid."));
    return;
  }
  const QList<QByteArray> components = requestLine.front().trimmed().split(' ');
  if (components.size() != 3 || components.at(0) != "GET" ||
      (components.at(2) != "HTTP/1.1" && components.at(2) != "HTTP/1.0")) {
    sendResponse(socket, 405, QStringLiteral("OAuth callback requires GET."));
    return;
  }
  const QByteArray target = components.at(1);
  if (!target.startsWith('/') || target.startsWith("//")) {
    sendResponse(socket, 400, QStringLiteral("OAuth callback request target is invalid."));
    return;
  }
  const QUrl url = QUrl::fromEncoded(target, QUrl::StrictMode);
  if (!url.isValid() || url.path() != QString::fromLatin1(kCallbackPath) || url.hasFragment()) {
    sendResponse(socket, 404, QStringLiteral("Not Found"));
    return;
  }
  const QUrlQuery query(url);
  const std::optional<QString> code = singleQueryValue(query, u"code", kMaximumCodeLength);
  const std::optional<QString> state = singleQueryValue(query, u"state", kMaximumStateLength);
  const std::optional<QString> error = singleQueryValue(query, u"error", kMaximumErrorLength);
  if ((!code.has_value() || !state.has_value()) && !error.has_value()) {
    sendResponse(socket, 400, QStringLiteral("OAuth callback is missing required fields."));
    return;
  }
  if (error.has_value() && code.has_value()) {
    sendResponse(socket, 400, QStringLiteral("OAuth callback fields are invalid."));
    return;
  }
  const std::uint64_t requestId = nextRequestId_;
  nextRequestId_ =
      nextRequestId_ == std::numeric_limits<std::uint64_t>::max() ? 1 : nextRequestId_ + 1;
  pendingCallbacks_.insert(requestId, socket);
  emit callbackReceived(
      OAuthLoopbackCallback{.requestId = requestId, .code = code, .state = state, .error = error});
}

void OAuthLoopbackCallbackListener::handleDisconnected(QTcpSocket* socket) {
  pendingRequests_.remove(socket);
  for (auto callback = pendingCallbacks_.begin(); callback != pendingCallbacks_.end();) {
    callback = callback.value() == socket ? pendingCallbacks_.erase(callback) : std::next(callback);
  }
  socket->deleteLater();
}

void OAuthLoopbackCallbackListener::sendResponse(QTcpSocket* socket,
                                                 int statusCode,
                                                 const QString& message) const {
  if (socket == nullptr) {
    return;
  }
  const QByteArray body = renderHtml(message).toUtf8();
  QByteArray response = "HTTP/1.1 ";
  response.append(QByteArray::number(statusCode));
  response.append("\r\nContent-Type: text/html; charset=utf-8\r\nCache-Control: no-store\r\n"
                  "Connection: close\r\nContent-Length: ");
  response.append(QByteArray::number(body.size()));
  response.append("\r\n\r\n");
  response.append(body);
  socket->write(response);
  socket->disconnectFromHost();
}

} // namespace hcb
