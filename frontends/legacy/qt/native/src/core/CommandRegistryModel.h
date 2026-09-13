#pragma once

#include <QAbstractListModel>
#include <QHash>
#include <QString>
#include <QVariantList>

#include <cstdint>

namespace hcb {

class CommandRegistryModel final : public QAbstractListModel {
  Q_OBJECT

public:
  enum Role : std::int32_t {
    CommandIdRole = Qt::UserRole + 1,
    CommandLabelRole,
    CommandShortcutRole
  };
  Q_ENUM(Role)

  explicit CommandRegistryModel(QObject* parent = nullptr);

  [[nodiscard]] int rowCount(const QModelIndex& parent = QModelIndex()) const override;
  [[nodiscard]] QVariant data(const QModelIndex& index, int role = Qt::DisplayRole) const override;
  [[nodiscard]] QHash<int, QByteArray> roleNames() const override;

  Q_INVOKABLE bool containsLabel(const QString& label) const;
  Q_INVOKABLE QVariantList matchingCommands(const QString& query) const;

private:
  struct Command final {
    QString id;
    QString label;
    QString shortcut;
  };

  QList<Command> commands_;
};

} // namespace hcb
