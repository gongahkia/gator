#pragma once

#include "core/AppError.h"
#include "core/SettingsService.h"

#include <QList>
#include <QString>

#include <future>
#include <variant>

namespace hcb {

struct SavedSearch final {
  QString id;
  QString name;
  QString query;
};

using SavedSearchListResult = std::variant<QList<SavedSearch>, AppError>;
using SavedSearchMutationResult = std::variant<SettingsMutationResult, AppError>;

class SavedSearchStore final {
public:
  explicit SavedSearchStore(SettingsService& settingsService);
  SavedSearchStore(const SavedSearchStore&) = delete;
  SavedSearchStore& operator=(const SavedSearchStore&) = delete;

  [[nodiscard]] std::future<SavedSearchListResult> load();
  [[nodiscard]] std::future<SavedSearchMutationResult> save(QList<SavedSearch> searches);

private:
  SettingsService& settingsService_;
};

} // namespace hcb
