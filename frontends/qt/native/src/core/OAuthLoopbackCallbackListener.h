#pragma once

#include "core/AppError.h"

#include <QByteArray>
#include <QHash>
#include <QObject>
#include <QString>
#include <QUrl>

#include <cstdint>
#include <optional>
#include <variant>

class QTcpServer;
class QTcpSocket;

namespace hcb {

struct OAuthLoopbackCallback final {
  std::uint64_t requestId;
  std::optional<QString> code;
  std::optional<QString> state;
  std::optional<QString> error;
};

using OAuthLoopbackListenerStartResult = std::variant<QUrl, AppError>;

class OAuthLoopbackCallbackListener final : public QObject {
  Q_OBJECT

public:
  explicit OAuthLoopbackCallbackListener(QObject* parent = nullptr);
  OAuthLoopbackCallbackListener(const OAuthLoopbackCallbackListener&) = delete;
  OAuthLoopbackCallbackListener& operator=(const OAuthLoopbackCallbackListener&) = delete;

  [[nodiscard]] OAuthLoopbackListenerStartResult start();
  void stop();
  [[nodiscard]] bool isListening() const;
  [[nodiscard]] QUrl redirectUri() const;
  [[nodiscard]] bool respond(std::uint64_t requestId, int statusCode, QString message);

signals:
  void callbackReceived(hcb::OAuthLoopbackCallback callback);

private:
  void handleNewConnection();
  void handleReadyRead(QTcpSocket* socket);
  void handleDisconnected(QTcpSocket* socket);
  void sendResponse(QTcpSocket* socket, int statusCode, const QString& message) const;

  QTcpServer* server_;
  QHash<QTcpSocket*, QByteArray> pendingRequests_;
  QHash<std::uint64_t, QTcpSocket*> pendingCallbacks_;
  std::uint64_t nextRequestId_{1};
};

} // namespace hcb

Q_DECLARE_METATYPE(hcb::OAuthLoopbackCallback)
