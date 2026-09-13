#pragma once

#include <QDir>
#include <QString>
#include <QStringView>

#include <optional>
#include <utility>

namespace hcb {

class FilePath final {
public:
  [[nodiscard]] static std::optional<FilePath> fromAbsolute(QString path) {
    path = QDir::cleanPath(path);
    if (!QDir::isAbsolutePath(path)) {
      return std::nullopt;
    }
    return FilePath(std::move(path));
  }

  [[nodiscard]] const QString& nativePath() const noexcept { return path_; }

  [[nodiscard]] std::optional<FilePath> child(QStringView component) const {
    if (component.isEmpty() || component == u"." || component == u".." ||
        component.contains(u'/') || component.contains(u'\\') ||
        QDir::isAbsolutePath(component.toString())) {
      return std::nullopt;
    }
    return fromAbsolute(QDir(path_).filePath(component.toString()));
  }

private:
  explicit FilePath(QString path) : path_(std::move(path)) {}

  QString path_;
};

} // namespace hcb
