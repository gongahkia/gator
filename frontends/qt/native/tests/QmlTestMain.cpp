#include <QtQuickTest>

#include <QFont>
#include <QFontDatabase>
#include <QLoggingCategory>
#include <QQuickStyle>

class QmlTestSetup final : public QObject {
  Q_OBJECT

public slots:
  void applicationAvailable() {
    QLoggingCategory::setFilterRules(QStringLiteral("qt.qpa.fonts.warning=false\n"));
    QQuickStyle::setStyle(QStringLiteral("Basic"));
    const QString systemFont = QFontDatabase::systemFont(QFontDatabase::GeneralFont).family();
    if (!systemFont.isEmpty()) {
      QFont::insertSubstitution(QStringLiteral("Sans Serif"), systemFont);
    }
  }
};

QUICK_TEST_MAIN_WITH_SETUP(hcb_qml_tests, QmlTestSetup)

#include "QmlTestMain.moc"
