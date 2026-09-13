#pragma once

#include "core/AppError.h"
#include "core/UnifiedLocalSearchRanker.h"

#include <QDate>
#include <QList>
#include <QString>
#include <QStringList>

#include <optional>
#include <cstdint>
#include <variant>

namespace hcb {

enum class LocalSearchDateMatch : std::uint8_t {
  Any,
  None,
  Exact,
  Before,
  After,
  Range
};

struct LocalSearchDateFilter final {
  LocalSearchDateMatch match{LocalSearchDateMatch::Any};
  QDate first;
  QDate last;
};

struct LocalSearchParsedQuery final {
  QString plainText;
  QList<LocalSearchResource> resources;
  std::optional<QString> taskStatus;
  std::optional<LocalSearchDateFilter> due;
  std::optional<LocalSearchDateFilter> start;
  std::optional<QString> priority;
  std::optional<QString> taskList;
  std::optional<QString> calendar;
  std::optional<bool> hasBody;
  QStringList chips;
};

using LocalSearchQueryResult = std::variant<LocalSearchParsedQuery, AppError>;

class LocalSearchQuery final {
public:
  [[nodiscard]] static LocalSearchQueryResult parse(QString query,
                                                     QDate today = QDate::currentDate());
};

} // namespace hcb
