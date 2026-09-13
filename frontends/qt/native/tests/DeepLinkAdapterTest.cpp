#include <QtTest>

#include "app/DeepLinkAdapter.h"

#include <vector>

class DeepLinkAdapterTest final : public QObject {
  Q_OBJECT

private slots:
  void parsesSupportedRoutes();
  void rejectsUnsupportedOrMalformedRoutes();
  void parsesLaunchArgumentsOnce();
  void queuesRoutesUntilHandlerIsReady();
};

void DeepLinkAdapterTest::parsesSupportedRoutes() {
  const std::optional<hcb::DeepLink> task = hcb::DeepLinkAdapter::parse(
      QUrl(QStringLiteral("hotcrossbuns://task/task-1"), QUrl::StrictMode));
  QVERIFY(task.has_value());
  QCOMPARE(task->destination, hcb::DeepLinkDestination::Tasks);
  QCOMPARE(task->entityId, QStringLiteral("task-1"));
  QCOMPARE(hcb::DeepLinkAdapter::pageName(task->destination), QStringLiteral("Tasks"));

  const std::optional<hcb::DeepLink> calendar = hcb::DeepLinkAdapter::parse(
      QUrl(QStringLiteral("hotcrossbuns://calendar/event-1"), QUrl::StrictMode));
  QVERIFY(calendar.has_value());
  QCOMPARE(calendar->destination, hcb::DeepLinkDestination::Calendar);
  QCOMPARE(calendar->entityId, QStringLiteral("event-1"));

  const std::optional<hcb::DeepLink> settings = hcb::DeepLinkAdapter::parse(
      QUrl(QStringLiteral("hotcrossbuns://settings"), QUrl::StrictMode));
  QVERIFY(settings.has_value());
  QCOMPARE(settings->destination, hcb::DeepLinkDestination::Settings);
  QVERIFY(settings->entityId.isEmpty());
}

void DeepLinkAdapterTest::rejectsUnsupportedOrMalformedRoutes() {
  const QStringList rejected{QStringLiteral("https://task/task-1"),
                             QStringLiteral("hotcrossbuns://search?q=invoice"),
                             QStringLiteral("hotcrossbuns://settings/advanced"),
                             QStringLiteral("hotcrossbuns://task/task-1/extra"),
                             QStringLiteral("hotcrossbuns://task/%E0%A4%A")};
  for (const QString& raw : rejected) {
    QVERIFY(!hcb::DeepLinkAdapter::parse(QUrl(raw, QUrl::StrictMode)).has_value());
  }
}

void DeepLinkAdapterTest::parsesLaunchArgumentsOnce() {
  const std::vector<hcb::DeepLink> links =
      hcb::DeepLinkAdapter::parseLaunchArguments({QStringLiteral("Hot Cross Buns"),
                                                  QStringLiteral("hotcrossbuns://task/task-1"),
                                                  QStringLiteral("hotcrossbuns://task/task-1"),
                                                  QStringLiteral("--other"),
                                                  QStringLiteral("hotcrossbuns://notes/note-1")});
  QCOMPARE(links.size(), std::size_t{2});
  QCOMPARE(links[0].destination, hcb::DeepLinkDestination::Tasks);
  QCOMPARE(links[1].destination, hcb::DeepLinkDestination::Notes);
}

void DeepLinkAdapterTest::queuesRoutesUntilHandlerIsReady() {
  hcb::DeepLinkDispatcher dispatcher;
  QVERIFY(
      dispatcher.handle(QUrl(QStringLiteral("hotcrossbuns://calendar/event-1"), QUrl::StrictMode)));

  std::vector<hcb::DeepLinkDestination> destinations;
  dispatcher.setHandler(
      [&destinations](const hcb::DeepLink& link) { destinations.push_back(link.destination); });
  QVERIFY(destinations ==
          std::vector<hcb::DeepLinkDestination>{hcb::DeepLinkDestination::Calendar});

  QVERIFY(dispatcher.handle(QUrl(QStringLiteral("hotcrossbuns://notes/note-1"), QUrl::StrictMode)));
  QVERIFY((destinations == std::vector<hcb::DeepLinkDestination>{hcb::DeepLinkDestination::Calendar,
                                                                 hcb::DeepLinkDestination::Notes}));
}

QTEST_APPLESS_MAIN(DeepLinkAdapterTest)

#include "DeepLinkAdapterTest.moc"
