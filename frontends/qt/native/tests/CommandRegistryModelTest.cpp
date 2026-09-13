#include <QtTest>

#include "core/CommandRegistryModel.h"

class CommandRegistryModelTest final : public QObject {
  Q_OBJECT

private slots:
  void exposesNativeNavigationAndCreationCommands();
  void rejectsInvalidIndexesAndUnknownLabels();
  void filtersCommandsByIdAndLabel();
};

void CommandRegistryModelTest::exposesNativeNavigationAndCreationCommands() {
  hcb::CommandRegistryModel commands;

  QCOMPARE(commands.rowCount(), 9);
  QCOMPARE(commands.rowCount(commands.index(0, 0)), 0);
  QCOMPARE(commands.data(commands.index(0, 0), hcb::CommandRegistryModel::CommandIdRole).toString(),
           QStringLiteral("navigation.tasks"));
  QCOMPARE(
      commands.data(commands.index(0, 0), hcb::CommandRegistryModel::CommandLabelRole).toString(),
      QStringLiteral("Tasks"));
  QCOMPARE(commands.data(commands.index(1, 0), Qt::DisplayRole).toString(),
           QStringLiteral("Calendar"));
  QCOMPARE(commands.data(commands.index(4, 0), hcb::CommandRegistryModel::CommandIdRole).toString(),
           QStringLiteral("navigation.settings"));
  QCOMPARE(commands.data(commands.index(3, 0), hcb::CommandRegistryModel::CommandShortcutRole)
               .toString(),
           QStringLiteral("Ctrl+3"));
  QCOMPARE(commands.roleNames().value(hcb::CommandRegistryModel::CommandIdRole),
           QByteArrayLiteral("commandId"));
  QCOMPARE(commands.roleNames().value(hcb::CommandRegistryModel::CommandLabelRole),
           QByteArrayLiteral("commandLabel"));
  QCOMPARE(commands.roleNames().value(hcb::CommandRegistryModel::CommandShortcutRole),
           QByteArrayLiteral("commandShortcut"));
  QCOMPARE(commands.data(commands.index(5, 0), hcb::CommandRegistryModel::CommandIdRole).toString(),
           QStringLiteral("create.quickCapture"));
  QCOMPARE(commands.data(commands.index(6, 0), Qt::DisplayRole).toString(),
           QStringLiteral("Import Tasks and events"));
  QCOMPARE(commands.data(commands.index(8, 0), Qt::DisplayRole).toString(),
           QStringLiteral("New Event"));
}

void CommandRegistryModelTest::rejectsInvalidIndexesAndUnknownLabels() {
  hcb::CommandRegistryModel commands;

  QVERIFY(!commands.data(QModelIndex(), Qt::DisplayRole).isValid());
  QVERIFY(!commands.data(commands.index(9, 0), Qt::DisplayRole).isValid());
  QVERIFY(commands.containsLabel(QStringLiteral("Notes")));
  QVERIFY(!commands.containsLabel(QStringLiteral("Unsupported")));
  QVERIFY(!commands.containsLabel(QStringLiteral("notes")));
}

void CommandRegistryModelTest::filtersCommandsByIdAndLabel() {
  hcb::CommandRegistryModel commands;

  const QVariantList allCommands = commands.matchingCommands(QString());
  QCOMPARE(allCommands.size(), 9);
  const QVariantList labelMatch = commands.matchingCommands(QStringLiteral("notes"));
  QCOMPARE(labelMatch.size(), 1);
  QCOMPARE(labelMatch.constFirst().toMap().value(QStringLiteral("commandId")).toString(),
           QStringLiteral("navigation.notes"));
  const QVariantList idMatch = commands.matchingCommands(QStringLiteral("settings"));
  QCOMPARE(idMatch.size(), 1);
  QCOMPARE(idMatch.constFirst().toMap().value(QStringLiteral("commandShortcut")).toString(),
           QStringLiteral("Ctrl+,"));
  const QVariantList captureMatch = commands.matchingCommands(QStringLiteral("capture"));
  QCOMPARE(captureMatch.size(), 1);
  QCOMPARE(captureMatch.constFirst().toMap().value(QStringLiteral("commandId")).toString(),
           QStringLiteral("create.quickCapture"));
  QVERIFY(commands.matchingCommands(QStringLiteral("unavailable")).isEmpty());
}

QTEST_MAIN(CommandRegistryModelTest)
#include "CommandRegistryModelTest.moc"
