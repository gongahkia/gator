#include <QGuiApplication>
#include <QQmlComponent>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickView>
#include <QtTest/QTest>

#include "core/SearchResultsModel.h"

namespace {

[[nodiscard]] int countDelegates(const QQuickItem* item) {
  int count = item->objectName() == QStringLiteral("searchResultDelegate") ? 1 : 0;
  for (const QQuickItem* child : item->childItems()) {
    count += countDelegates(child);
  }
  return count;
}

} // namespace

class ModelVirtualizationTest final : public QObject {
  Q_OBJECT

private slots:
  void listViewVirtualizesLargeSearchResultModels();
};

void ModelVirtualizationTest::listViewVirtualizesLargeSearchResultModels() {
  hcb::SearchResultsModel model;
  QList<hcb::LocalSearchRankedResult> results;
  constexpr int resultCount = 15'000;
  results.reserve(resultCount);
  for (int index = 0; index < resultCount; ++index) {
    results.append({.resource = hcb::LocalSearchResource::Task,
                    .id = QStringLiteral("task-%1").arg(index),
                    .title = QStringLiteral("Result %1").arg(index),
                    .detail = QStringLiteral("Detail %1").arg(index),
                    .score = resultCount - index});
  }
  model.setResults(std::move(results));
  QCOMPARE(model.rowCount(), resultCount);
  QCOMPARE(
      model.data(model.index(resultCount - 1, 0), hcb::SearchResultsModel::TitleRole).toString(),
      QStringLiteral("Result 14999"));

  QQuickView view;
  view.rootContext()->setContextProperty(QStringLiteral("searchResults"), &model);
  QQmlComponent component(view.engine());
  component.setData(R"(
import QtQuick

Item {
  width: 320
  height: 240

  ListView {
    objectName: "searchResultsView"
    anchors.fill: parent
    model: searchResults
    delegate: Item {
      objectName: "searchResultDelegate"
      width: ListView.view.width
      height: 24

      property string boundTitle: title
    }
  }
}
)",
                    QUrl(QStringLiteral("qrc:/ModelVirtualization.qml")));
  QVERIFY2(component.isReady(), qPrintable(component.errorString()));
  QObject* root = component.create(view.rootContext());
  QVERIFY2(root != nullptr, qPrintable(component.errorString()));
  view.setContent(component.url(), &component, root);
  view.show();

  QQuickItem* const listView = root->findChild<QQuickItem*>(QStringLiteral("searchResultsView"));
  QVERIFY(listView != nullptr);
  QQuickItem* const rootItem = qobject_cast<QQuickItem*>(root);
  QVERIFY(rootItem != nullptr);
  QTRY_COMPARE(listView->property("count").toInt(), resultCount);
  const auto delegateCount = [rootItem] { return countDelegates(rootItem); };
  QTRY_VERIFY(delegateCount() > 0);
  QVERIFY(delegateCount() < 100);

  listView->setProperty("contentY", resultCount * 24 - 240);
  QTRY_VERIFY(delegateCount() > 0);
  QVERIFY(delegateCount() < 100);
}

int main(int argc, char* argv[]) {
  QGuiApplication application(argc, argv);
  ModelVirtualizationTest test;
  return QTest::qExec(&test, argc, argv);
}

#include "ModelVirtualizationTest.moc"
