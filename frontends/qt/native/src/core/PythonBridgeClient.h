#pragma once

#include "core/AppError.h"
#include "core/Cancellation.h"

#include <QDate>
#include <QByteArray>
#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QObject>
#include <QString>
#include <QUrl>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct PythonBridgeConnection final {
  QUrl endpoint;
  QByteArray bearerToken;
};

using PythonBridgeConnectionOrError = std::variant<PythonBridgeConnection, AppError>;
using PythonBridgeResult = std::variant<QJsonObject, AppError>;

class PythonBridgeClient final : public QObject {
  Q_OBJECT

public:
  explicit PythonBridgeClient(PythonBridgeConnection connection,
                              QObject* parent = nullptr,
                              QNetworkAccessManager* manager = nullptr);

  [[nodiscard]] static PythonBridgeConnectionOrError readConnectionDescriptor(
      const QString& descriptorPath);

  [[nodiscard]] std::future<PythonBridgeResult>
  workspace(const QString& accountId, CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  taskPage(const QString& accountId,
           int limit = 200,
           std::optional<QString> cursor = std::nullopt,
           std::optional<QString> listId = std::nullopt,
           CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  eventRange(const QString& accountId,
             QDate start,
             QDate end,
             std::optional<QString> calendarId = std::nullopt,
             CancellationToken cancellation = {});

private:
  [[nodiscard]] std::future<PythonBridgeResult>
  get(QUrl path, CancellationToken cancellation);

  PythonBridgeConnection connection_;
  QNetworkAccessManager* manager_;
};

} // namespace hcb
