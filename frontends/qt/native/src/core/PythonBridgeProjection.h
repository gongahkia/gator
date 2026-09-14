#pragma once

#include "core/AppError.h"
#include "core/CalendarReadService.h"
#include "core/TaskListReadService.h"
#include "core/TaskModel.h"

#include <QHash>
#include <QJsonObject>
#include <QList>
#include <QString>

#include <optional>
#include <variant>

namespace hcb {

struct PythonBridgeWorkspaceSummary final {
  QString accountId;
  QList<TaskListSummary> taskLists;
  QList<CalendarSummary> calendars;
  int pending;
};

struct PythonBridgeTaskPage final {
  QList<TaskModelTask> tasks;
  std::optional<QString> nextCursor;
};

using PythonBridgeWorkspaceSummaryOrError = std::variant<PythonBridgeWorkspaceSummary, AppError>;
using PythonBridgeTaskPageOrError = std::variant<PythonBridgeTaskPage, AppError>;
using PythonBridgeEventRangeOrError = std::variant<QList<CalendarEventSummary>, AppError>;

class PythonBridgeProjection final {
public:
  [[nodiscard]] static PythonBridgeWorkspaceSummaryOrError
  workspaceSummary(const QJsonObject& data);
  [[nodiscard]] static PythonBridgeTaskPageOrError
  taskPage(const QJsonObject& data, const QHash<QString, QString>& taskListTitles);
  [[nodiscard]] static PythonBridgeEventRangeOrError eventRange(const QJsonObject& data);
  [[nodiscard]] static QHash<QString, QString>
  taskListTitles(const PythonBridgeWorkspaceSummary& summary);
};

} // namespace hcb
