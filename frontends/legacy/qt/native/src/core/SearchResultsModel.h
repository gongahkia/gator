#pragma once

#include "core/UnifiedLocalSearchRanker.h"

#include <QAbstractListModel>

#include <cstdint>

namespace hcb {

class SearchResultsModel final : public QAbstractListModel {
  Q_OBJECT

public:
  enum Role : std::int32_t {
    IdRole = Qt::UserRole + 1,
    ResourceRole,
    TitleRole,
    DetailRole,
    ScheduledAtRole,
    ScoreRole
  };
  Q_ENUM(Role)

  explicit SearchResultsModel(QObject* parent = nullptr);

  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;

  void setResults(QList<LocalSearchRankedResult> results);

private:
  QList<LocalSearchRankedResult> results_;
};

} // namespace hcb
