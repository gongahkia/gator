#pragma once

#include "data/SqliteConnection.h"

#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>

#include <memory>
#include <optional>
#include <variant>

namespace hcb::test {

class TemporarySqliteDatabase final {
public:
  TemporarySqliteDatabase(const TemporarySqliteDatabase&) = delete;
  TemporarySqliteDatabase& operator=(const TemporarySqliteDatabase&) = delete;

  [[nodiscard]] static std::variant<std::unique_ptr<TemporarySqliteDatabase>, AppError> create() {
    auto directory = std::make_unique<QTemporaryDir>(
        QDir(QDir::tempPath()).filePath(QStringLiteral("hot-cross-buns-XXXXXX")));
    if (!directory->isValid()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Temporary SQLite directory creation failed"));
    }
    const QString rootPath = QFileInfo(directory->path()).canonicalFilePath();
    const std::optional<FilePath> databasePath =
        FilePath::fromAbsolute(QDir(rootPath).filePath(QStringLiteral("hot-cross-buns.sqlite")));
    if (!databasePath.has_value()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Temporary SQLite database path is invalid"));
    }
    return std::unique_ptr<TemporarySqliteDatabase>(
        new TemporarySqliteDatabase(std::move(directory), rootPath, *databasePath));
  }

  [[nodiscard]] const QString& rootPath() const noexcept { return rootPath_; }
  [[nodiscard]] const FilePath& databasePath() const noexcept { return databasePath_; }
  [[nodiscard]] SqliteConnectionResult open(SqliteOpenMode mode) const {
    return SqliteConnectionFactory::open(databasePath_, mode);
  }

private:
  TemporarySqliteDatabase(std::unique_ptr<QTemporaryDir> directory,
                          QString rootPath,
                          FilePath databasePath)
      : directory_(std::move(directory)), rootPath_(std::move(rootPath)),
        databasePath_(std::move(databasePath)) {}

  std::unique_ptr<QTemporaryDir> directory_;
  QString rootPath_;
  FilePath databasePath_;
};

using TemporarySqliteDatabaseResult =
    std::variant<std::unique_ptr<TemporarySqliteDatabase>, AppError>;

} // namespace hcb::test
