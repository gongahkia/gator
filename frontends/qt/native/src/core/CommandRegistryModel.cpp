#include "core/CommandRegistryModel.h"

#include <algorithm>

#include <QVariantMap>

namespace hcb {

CommandRegistryModel::CommandRegistryModel(QObject* parent)
    : QAbstractListModel(parent),
      commands_{
          {QStringLiteral("navigation.tasks"), QStringLiteral("Tasks"), QStringLiteral("Ctrl+1")},
          {QStringLiteral("navigation.calendar"),
           QStringLiteral("Calendar"),
           QStringLiteral("Ctrl+2")},
          {QStringLiteral("navigation.invitations"),
           QStringLiteral("Invitations"),
           QStringLiteral("Ctrl+4")},
          {QStringLiteral("navigation.notes"), QStringLiteral("Notes"), QStringLiteral("Ctrl+3")},
          {QStringLiteral("navigation.settings"),
           QStringLiteral("Settings"),
           QStringLiteral("Ctrl+,")},
          {QStringLiteral("create.quickCapture"),
           QStringLiteral("Quick Capture"),
           QStringLiteral("Ctrl+Shift+N")},
          {QStringLiteral("import.items"), QStringLiteral("Import Tasks and events"), QString()},
          {QStringLiteral("create.task"), QStringLiteral("New Task"), QString()},
          {QStringLiteral("create.event"), QStringLiteral("New Event"), QString()}} {}

int CommandRegistryModel::rowCount(const QModelIndex& parent) const {
  return parent.isValid() ? 0 : static_cast<int>(commands_.size());
}

QVariant CommandRegistryModel::data(const QModelIndex& index, int role) const {
  if (!index.isValid() || index.row() < 0 || index.row() >= static_cast<int>(commands_.size())) {
    return {};
  }

  const Command& command = commands_.at(index.row());
  switch (role) {
  case Qt::DisplayRole:
  case CommandLabelRole:
    return command.label;
  case CommandIdRole:
    return command.id;
  case CommandShortcutRole:
    return command.shortcut;
  default:
    return {};
  }
}

QHash<int, QByteArray> CommandRegistryModel::roleNames() const {
  return {{CommandIdRole, "commandId"},
          {CommandLabelRole, "commandLabel"},
          {CommandShortcutRole, "commandShortcut"}};
}

bool CommandRegistryModel::containsLabel(const QString& label) const {
  return std::any_of(commands_.cbegin(), commands_.cend(), [&label](const Command& command) {
    return command.label == label;
  });
}

QVariantList CommandRegistryModel::matchingCommands(const QString& query) const {
  const QString normalizedQuery = query.trimmed().toCaseFolded();
  QVariantList matches;
  matches.reserve(commands_.size());
  for (const Command& command : commands_) {
    if (!normalizedQuery.isEmpty() && !command.id.toCaseFolded().contains(normalizedQuery) &&
        !command.label.toCaseFolded().contains(normalizedQuery)) {
      continue;
    }
    matches.append(QVariantMap{{QStringLiteral("commandId"), command.id},
                               {QStringLiteral("commandLabel"), command.label},
                               {QStringLiteral("commandShortcut"), command.shortcut}});
  }
  return matches;
}

} // namespace hcb
