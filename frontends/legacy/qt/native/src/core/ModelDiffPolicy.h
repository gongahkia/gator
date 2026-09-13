#pragma once

#include <QList>

namespace hcb {

struct ModelDataChangeRange final {
  int firstRow{0};
  int lastRow{0};
};

struct ModelDiffPlan final {
  bool requiresReset{false};
  QList<ModelDataChangeRange> changedRanges;
};

class ModelDiffPolicy final {
public:
  template <typename Item, typename Key, typename Equivalent>
  [[nodiscard]] static ModelDiffPlan plan(const QList<Item>& current,
                                          const QList<Item>& next,
                                          const Key& key,
                                          const Equivalent& equivalent) {
    if (current.size() != next.size()) {
      return {.requiresReset = true};
    }
    ModelDiffPlan result;
    const int count = static_cast<int>(current.size());
    int firstChangedRow = -1;
    for (int row = 0; row < count; ++row) {
      if (key(current.at(row)) != key(next.at(row))) {
        return {.requiresReset = true};
      }
      if (!equivalent(current.at(row), next.at(row))) {
        if (firstChangedRow < 0) {
          firstChangedRow = row;
        }
      } else if (firstChangedRow >= 0) {
        result.changedRanges.append({.firstRow = firstChangedRow, .lastRow = row - 1});
        firstChangedRow = -1;
      }
    }
    if (firstChangedRow >= 0) {
      result.changedRanges.append({.firstRow = firstChangedRow, .lastRow = count - 1});
    }
    return result;
  }
};

} // namespace hcb
