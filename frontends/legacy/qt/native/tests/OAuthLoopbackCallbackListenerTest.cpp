#include <QSignalSpy>
#include <QTcpSocket>
#include <QtTest/QTest>

#include "core/OAuthLoopbackCallbackListener.h"

#include <optional>
#include <variant>

class OAuthLoopbackCallbackListenerTest final : public QObject {
  Q_OBJECT

private slots:
  void dispatchesCallbackAndWritesSanitizedReply();
  void rejectsInvalidRequests();
};

namespace {

[[nodiscard]] std::optional<QUrl> start(hcb::OAuthLoopbackCallbackListener& listener) {
  const hcb::OAuthLoopbackListenerStartResult result = listener.start();
  if (!std::holds_alternative<QUrl>(result)) {
    return std::nullopt;
  }
  return std::get<QUrl>(result);
}

void connectClient(QTcpSocket& client, const QUrl& uri) {
  client.connectToHost(uri.host(), static_cast<quint16>(uri.port()));
  QVERIFY(client.waitForConnected(1'000));
}

[[nodiscard]] QByteArray awaitResponse(QTcpSocket& client) {
  for (int attempt = 0; attempt < 100 && client.bytesAvailable() == 0 &&
                        client.state() != QAbstractSocket::UnconnectedState;
       ++attempt) {
    QTest::qWait(10);
  }
  QByteArray response = client.readAll();
  for (int attempt = 0; attempt < 100 && client.state() != QAbstractSocket::UnconnectedState;
       ++attempt) {
    QTest::qWait(10);
  }
  response.append(client.readAll());
  return response;
}

} // namespace

void OAuthLoopbackCallbackListenerTest::dispatchesCallbackAndWritesSanitizedReply() {
  qRegisterMetaType<hcb::OAuthLoopbackCallback>();
  hcb::OAuthLoopbackCallbackListener listener;
  const std::optional<QUrl> uri = start(listener);
  QVERIFY(uri.has_value());
  if (!uri.has_value()) {
    return;
  }
  QCOMPARE(uri->host(), QStringLiteral("127.0.0.1"));
  QCOMPARE(uri->path(), QStringLiteral("/oauth/google/callback"));
  QVERIFY(listener.isListening());
  QSignalSpy callbacks(&listener, &hcb::OAuthLoopbackCallbackListener::callbackReceived);

  QTcpSocket client;
  connectClient(client, *uri);
  client.write("GET /oauth/google/callback?code=oauth-code&state=pkce-state HTTP/1.1\r\n"
               "Host: 127.0.0.1\r\n\r\n");
  QVERIFY(client.flush());
  QTRY_COMPARE_WITH_TIMEOUT(callbacks.count(), 1, 1'000);
  const hcb::OAuthLoopbackCallback callback =
      qvariant_cast<hcb::OAuthLoopbackCallback>(callbacks.at(0).at(0));
  QCOMPARE(callback.code, std::optional<QString>(QStringLiteral("oauth-code")));
  QCOMPARE(callback.state, std::optional<QString>(QStringLiteral("pkce-state")));
  QVERIFY(!callback.error.has_value());
  QVERIFY(listener.respond(callback.requestId, 200, QStringLiteral("Completed <safely>.")));
  const QByteArray response = awaitResponse(client);
  QVERIFY(response.startsWith("HTTP/1.1 200"));
  QVERIFY(response.contains("Cache-Control: no-store"));
  QVERIFY(response.contains("Completed &lt;safely&gt;."));
  QVERIFY(!response.contains("oauth-code"));
  QVERIFY(!listener.respond(callback.requestId, 200, QStringLiteral("Completed.")));

  listener.stop();
  QVERIFY(!listener.isListening());
  QVERIFY(!listener.redirectUri().isValid());
}

void OAuthLoopbackCallbackListenerTest::rejectsInvalidRequests() {
  qRegisterMetaType<hcb::OAuthLoopbackCallback>();
  hcb::OAuthLoopbackCallbackListener listener;
  const std::optional<QUrl> uri = start(listener);
  QVERIFY(uri.has_value());
  if (!uri.has_value()) {
    return;
  }
  QSignalSpy callbacks(&listener, &hcb::OAuthLoopbackCallbackListener::callbackReceived);

  QTcpSocket missingState;
  connectClient(missingState, *uri);
  missingState.write(
      "GET /oauth/google/callback?code=oauth-code HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n");
  QVERIFY(missingState.flush());
  const QByteArray missingResponse = awaitResponse(missingState);
  QVERIFY(missingResponse.startsWith("HTTP/1.1 400"));
  QCOMPARE(callbacks.count(), 0);

  QTcpSocket wrongMethod;
  connectClient(wrongMethod, *uri);
  wrongMethod.write("POST /oauth/google/callback HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n");
  QVERIFY(wrongMethod.flush());
  const QByteArray methodResponse = awaitResponse(wrongMethod);
  QVERIFY(methodResponse.startsWith("HTTP/1.1 405"));

  QTcpSocket wrongPath;
  connectClient(wrongPath, *uri);
  wrongPath.write("GET /wrong HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n");
  QVERIFY(wrongPath.flush());
  const QByteArray pathResponse = awaitResponse(wrongPath);
  QVERIFY(pathResponse.startsWith("HTTP/1.1 404"));
  QCOMPARE(callbacks.count(), 0);
}

QTEST_GUILESS_MAIN(OAuthLoopbackCallbackListenerTest)

#include "OAuthLoopbackCallbackListenerTest.moc"
