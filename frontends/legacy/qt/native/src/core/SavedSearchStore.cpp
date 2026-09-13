#include "core/SavedSearchStore.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>

#include <future>
#include <optional>
#include <utility>

namespace hcb {
namespace {

constexpr char kSearchSettingsScope[] = "search";
constexpr char kSavedSearchesSettingsKey[] = "saved_searches";
constexpr qsizetype kMaximumSavedSearches = 100;
constexpr qsizetype kMaximumSavedSearchNameLength = 128;
constexpr qsizetype kMaximumSavedSearchQueryLength = 4096;

[[nodiscard]] std::optional<AppError> validate(const QList<SavedSearch>& searches) {
  if (searches.size() > kMaximumSavedSearches) {
    return AppError(AppErrorCode::Validation, QStringLiteral("Too many saved searches"));
  }
  QStringList ids;
  QStringList names;
  for (const SavedSearch& search : searches) {
    if (search.id.isEmpty() || search.id != search.id.trimmed() ||
        search.id.size() > kMaximumSavedSearchNameLength || search.name.trimmed().isEmpty() ||
        search.name != search.name.trimmed() || search.name.size() > kMaximumSavedSearchNameLength ||
        search.query.trimmed().isEmpty() || search.query.size() > kMaximumSavedSearchQueryLength ||
        ids.contains(search.id) || names.contains(search.name, Qt::CaseInsensitive)) {
      return AppError(AppErrorCode::Validation, QStringLiteral("Saved search is invalid"));
    }
    ids.append(search.id);
    names.append(search.name);
  }
  return std::nullopt;
}

[[nodiscard]] SavedSearchListResult decode(const QString& json) {
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(json.toUtf8(), &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isArray()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Saved searches are invalid"));
  }
  QList<SavedSearch> searches;
  const QJsonArray values = document.array();
  searches.reserve(values.size());
  for (const QJsonValue& value : values) {
    if (!value.isObject()) {
      return AppError(AppErrorCode::Database, QStringLiteral("Saved searches are invalid"));
    }
    const QJsonObject object = value.toObject();
    if (!object.value(QStringLiteral("id")).isString() ||
        !object.value(QStringLiteral("name")).isString() ||
        !object.value(QStringLiteral("query")).isString()) {
      return AppError(AppErrorCode::Database, QStringLiteral("Saved searches are invalid"));
    }
    searches.append({.id = object.value(QStringLiteral("id")).toString(),
                     .name = object.value(QStringLiteral("name")).toString(),
                     .query = object.value(QStringLiteral("query")).toString()});
  }
  if (const std::optional<AppError> error = validate(searches); error.has_value()) {
    return *error;
  }
  return searches;
}

[[nodiscard]] QString encode(const QList<SavedSearch>& searches) {
  QJsonArray values;
  for (const SavedSearch& search : searches) {
    values.append(QJsonObject{{QStringLiteral("id"), search.id},
                              {QStringLiteral("name"), search.name},
                              {QStringLiteral("query"), search.query}});
  }
  return QString::fromUtf8(QJsonDocument(values).toJson(QJsonDocument::Compact));
}

} // namespace

SavedSearchStore::SavedSearchStore(SettingsService& settingsService) : settingsService_(settingsService) {}

std::future<SavedSearchListResult> SavedSearchStore::load() {
  std::future<SettingsJsonReadResult> read = settingsService_.readJson(
      QString::fromLatin1(kSearchSettingsScope), QString::fromLatin1(kSavedSearchesSettingsKey));
  return std::async(std::launch::async, [read = std::move(read)]() mutable -> SavedSearchListResult {
    SettingsJsonReadResult result = read.get();
    if (std::holds_alternative<AppError>(result)) {
      return std::get<AppError>(std::move(result));
    }
    const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
    return stored.has_value() ? decode(*stored) : SavedSearchListResult(QList<SavedSearch>{});
  });
}

std::future<SavedSearchMutationResult> SavedSearchStore::save(QList<SavedSearch> searches) {
  if (const std::optional<AppError> error = validate(searches); error.has_value()) {
    std::promise<SavedSearchMutationResult> completion;
    std::future<SavedSearchMutationResult> future = completion.get_future();
    completion.set_value(*error);
    return future;
  }
  std::future<SettingsMutationResultOrError> write = settingsService_.writeJson(
      QString::fromLatin1(kSearchSettingsScope),
      QString::fromLatin1(kSavedSearchesSettingsKey),
      encode(searches));
  return std::async(std::launch::async, [write = std::move(write)]() mutable -> SavedSearchMutationResult {
    SettingsMutationResultOrError result = write.get();
    return std::holds_alternative<AppError>(result)
               ? SavedSearchMutationResult(std::get<AppError>(std::move(result)))
               : SavedSearchMutationResult(std::get<SettingsMutationResult>(result));
  });
}

} // namespace hcb
