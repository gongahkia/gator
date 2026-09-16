#pragma once

#include "core/AppError.h"
#include "core/Cancellation.h"

#include <QDate>
#include <QByteArray>
#include <QList>
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

  [[nodiscard]] static PythonBridgeConnectionOrError
  readConnectionDescriptor(const QString& descriptorPath);

  [[nodiscard]] std::future<PythonBridgeResult> workspace(const QString& accountId,
                                                          CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  authenticationState(const QString& accountId, CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> conflicts(const QString& accountId,
                                                          CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> savedSearches(const QString& accountId,
                                                              CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  createSavedSearch(const QString& accountId,
                    const QString& name,
                    const QString& query,
                    const QByteArray& idempotencyKey,
                    CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  updateSavedSearch(const QString& accountId,
                    const QString& savedSearchId,
                    std::optional<QString> name,
                    std::optional<QString> query,
                    const QByteArray& idempotencyKey,
                    CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  deleteSavedSearch(const QString& accountId,
                    const QString& savedSearchId,
                    const QByteArray& idempotencyKey,
                    CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  resolveConflict(const QString& accountId,
                  const QString& conflictId,
                  bool keepLocal,
                  const QByteArray& idempotencyKey,
                  CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  taskPage(const QString& accountId,
           int limit = 200,
           std::optional<QString> cursor = std::nullopt,
           std::optional<QString> listId = std::nullopt,
           CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> search(const QString& accountId,
                                                       const QString& query,
                                                       int limit = 50,
                                                       CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  eventRange(const QString& accountId,
             QDate start,
             QDate end,
             std::optional<QString> calendarId = std::nullopt,
             CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> createTask(const QString& accountId,
                                                           const QJsonObject& task,
                                                           const QByteArray& idempotencyKey,
                                                           CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> updateTask(const QString& accountId,
                                                           const QString& taskId,
                                                           const QJsonObject& changes,
                                                           const QByteArray& idempotencyKey,
                                                           CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> moveTask(const QString& accountId,
                                                         const QString& taskId,
                                                         const QJsonObject& move,
                                                         const QByteArray& idempotencyKey,
                                                         CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> completeTask(const QString& accountId,
                                                             const QString& taskId,
                                                             bool completed,
                                                             const QByteArray& idempotencyKey,
                                                             CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  bulkCompleteTasks(const QString& accountId,
                    const QList<QString>& taskIds,
                    bool completed,
                    const QByteArray& idempotencyKey,
                    CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  bulkDeleteTasks(const QString& accountId,
                  const QList<QString>& taskIds,
                  const QByteArray& idempotencyKey,
                  CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> bulkMoveTasks(const QString& accountId,
                                                              const QList<QString>& taskIds,
                                                              const QString& taskListId,
                                                              const QByteArray& idempotencyKey,
                                                              CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  stopTaskRecurrence(const QString& accountId,
                     const QString& taskId,
                     const QString& scope,
                     const QByteArray& idempotencyKey,
                     CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  splitTaskRecurrence(const QString& accountId,
                      const QString& taskId,
                      const QByteArray& idempotencyKey,
                      CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> deleteTask(const QString& accountId,
                                                           const QString& taskId,
                                                           const QByteArray& idempotencyKey,
                                                           CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> createTaskList(const QString& accountId,
                                                               const QJsonObject& taskList,
                                                               const QByteArray& idempotencyKey,
                                                               CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> updateTaskList(const QString& accountId,
                                                               const QString& taskListId,
                                                               const QJsonObject& changes,
                                                               const QByteArray& idempotencyKey,
                                                               CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> deleteTaskList(const QString& accountId,
                                                               const QString& taskListId,
                                                               const QByteArray& idempotencyKey,
                                                               CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> createEvent(const QString& accountId,
                                                            const QJsonObject& event,
                                                            const QByteArray& idempotencyKey,
                                                            CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> updateEvent(const QString& accountId,
                                                            const QString& eventId,
                                                            const QJsonObject& changes,
                                                            const QByteArray& idempotencyKey,
                                                            CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> deleteEvent(const QString& accountId,
                                                            const QString& eventId,
                                                            const QByteArray& idempotencyKey,
                                                            CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> createCalendar(const QString& accountId,
                                                               const QJsonObject& calendar,
                                                               const QByteArray& idempotencyKey,
                                                               CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> updateCalendar(const QString& accountId,
                                                               const QString& calendarId,
                                                               const QJsonObject& changes,
                                                               const QByteArray& idempotencyKey,
                                                               CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> deleteCalendar(const QString& accountId,
                                                               const QString& calendarId,
                                                               const QByteArray& idempotencyKey,
                                                               CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  subscribeCalendar(const QString& accountId,
                    const QJsonObject& subscription,
                    const QByteArray& idempotencyKey,
                    CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  unsubscribeCalendar(const QString& accountId,
                      const QString& calendarId,
                      const QByteArray& idempotencyKey,
                      CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> startSync(const QString& accountId,
                                                          CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> startOAuth(const QString& accountId,
                                                           const QString& expectedEmail,
                                                           CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult> operation(const QString& operationId,
                                                          CancellationToken cancellation = {});
  [[nodiscard]] std::future<PythonBridgeResult>
  cancelOperation(const QString& operationId, CancellationToken cancellation = {});

private:
  [[nodiscard]] std::future<PythonBridgeResult> get(QUrl path, CancellationToken cancellation);
  [[nodiscard]] std::future<PythonBridgeResult> request(QByteArray method,
                                                        QUrl path,
                                                        std::optional<QJsonObject> body,
                                                        std::optional<QByteArray> idempotencyKey,
                                                        CancellationToken cancellation);

  PythonBridgeConnection connection_;
  QNetworkAccessManager* manager_;
};

} // namespace hcb
