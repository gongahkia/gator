#pragma once

#include "core/AppError.h"
#include "core/Cancellation.h"
#include "core/FilePath.h"
#include "core/UnifiedLocalSearchRanker.h"
#include "data/SqliteReadConnectionPool.h"

#include <QList>
#include <QString>

#include <future>
#include <memory>
#include <variant>

namespace hcb {

struct LocalSearchRequest final {
  QString query;
  int offset{0};
  int limit{25};
};

struct LocalSearchPage final {
  QList<LocalSearchRankedResult> items;
  int totalKnown{0};
  bool hasMore{false};
};

using LocalSearchPageResult = std::variant<LocalSearchPage, AppError>;

class LocalSearchService final {
public:
  explicit LocalSearchService(FilePath databasePath, std::size_t connectionCount = 2);
  LocalSearchService(const LocalSearchService&) = delete;
  LocalSearchService& operator=(const LocalSearchService&) = delete;

  [[nodiscard]] std::shared_future<std::optional<AppError>> ready() const;
  [[nodiscard]] std::future<LocalSearchPageResult>
  search(LocalSearchRequest request, const CancellationToken& cancellation = {});

private:
  UnifiedLocalSearchRanker ranker_;
  std::unique_ptr<SqliteReadConnectionPool> readPool_;
};

} // namespace hcb
