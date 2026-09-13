#include "core/SearchResultsModel.h"
#include "core/ModelDiffPolicy.h"

#include <utility>

namespace hcb {

namespace {

[[nodiscard]] QString resourceName(LocalSearchResource resource) {
  switch (resource) {
  case LocalSearchResource::TaskList:
    return QStringLiteral("taskList");
  case LocalSearchResource::Task:
    return QStringLiteral("task");
  case LocalSearchResource::Note:
    return QStringLiteral("note");
  case LocalSearchResource::Calendar:
    return QStringLiteral("calendar");
  case LocalSearchResource::Event:
    return QStringLiteral("event");
  }
  return {};
}

[[nodiscard]] bool equivalentResult(const LocalSearchRankedResult& left,
                                    const LocalSearchRankedResult& right) {
  return left.id == right.id && left.resource == right.resource && left.title == right.title &&
         left.detail == right.detail && left.scheduledAt == right.scheduledAt &&
         left.score == right.score;
}

} // namespace

SearchResultsModel::SearchResultsModel(QObject* parent) : QAbstractListModel(parent) {}

int SearchResultsModel::rowCount(const QModelIndex& parent) const {
  return parent.isValid() ? 0 : static_cast<int>(results_.size());
}

QVariant SearchResultsModel::data(const QModelIndex& index, int role) const {
  if (!index.isValid() || index.row() < 0 || index.row() >= static_cast<int>(results_.size())) {
    return {};
  }
  const LocalSearchRankedResult& result = results_.at(index.row());
  switch (role) {
  case Qt::DisplayRole:
  case TitleRole:
    return result.title;
  case IdRole:
    return result.id;
  case ResourceRole:
    return resourceName(result.resource);
  case DetailRole:
    return result.detail;
  case ScheduledAtRole:
    return result.scheduledAt;
  case ScoreRole:
    return result.score;
  default:
    return {};
  }
}

QHash<int, QByteArray> SearchResultsModel::roleNames() const {
  return {{IdRole, "id"},
          {ResourceRole, "resource"},
          {TitleRole, "title"},
          {DetailRole, "detail"},
          {ScheduledAtRole, "scheduledAt"},
          {ScoreRole, "score"}};
}

void SearchResultsModel::setResults(QList<LocalSearchRankedResult> results) {
  const ModelDiffPlan plan = ModelDiffPolicy::plan(
      results_,
      results,
      [](const LocalSearchRankedResult& result) -> const QString& { return result.id; },
      equivalentResult);
  if (plan.requiresReset) {
    beginResetModel();
    results_ = std::move(results);
    endResetModel();
    return;
  }
  results_ = std::move(results);
  for (const ModelDataChangeRange& range : plan.changedRanges) {
    emit dataChanged(index(range.firstRow, 0), index(range.lastRow, 0));
  }
}

} // namespace hcb
