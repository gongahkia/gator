#include <QtTest/QTest>

#include "app/PythonBridgeProcessLauncher.h"

#include <QFile>
#include <QTemporaryDir>

namespace {

constexpr auto kToken = "test_desktop_bridge_token_123456";

} // namespace

class PythonBridgeProcessLauncherTest final : public QObject {
  Q_OBJECT

private slots:
  void launchesAndStopsAPrivateBridgeProcess();
  void rejectsAnUnavailableExecutable();
};

void PythonBridgeProcessLauncherTest::launchesAndStopsAPrivateBridgeProcess() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const QString command = directory.filePath(QStringLiteral("fake-hcb"));
  QFile file(command);
  QVERIFY(file.open(QIODevice::WriteOnly | QIODevice::Text));
  const QByteArray script = QByteArrayLiteral(R"sh(#!/bin/sh
ready_file=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--ready-file" ]; then
    ready_file="$2"
    shift 2
  else
    shift
  fi
done
printf '%s' '{"api_version":1,"url":"http://127.0.0.1:4242","token":"test_desktop_bridge_token_123456"}' > "$ready_file"
chmod 600 "$ready_file"
trap 'rm -f "$ready_file"; exit 0' INT TERM
while :; do sleep 1; done
)sh");
  QCOMPARE(file.write(script), qint64(script.size()));
  file.close();
  QVERIFY(file.setPermissions(QFile::ReadOwner | QFile::WriteOwner | QFile::ExeOwner));

  QString descriptor;
  {
    hcb::PythonBridgeProcessLauncher launcher(command);
    const hcb::PythonBridgeLaunchResult result = launcher.start();
    QVERIFY(std::holds_alternative<hcb::PythonBridgeConnection>(result));
    QCOMPARE(std::get<hcb::PythonBridgeConnection>(result).bearerToken, QByteArray(kToken));
    descriptor = launcher.descriptorPath();
    QVERIFY(QFile::exists(descriptor));
  }
  QVERIFY(!QFile::exists(descriptor));
}

void PythonBridgeProcessLauncherTest::rejectsAnUnavailableExecutable() {
  hcb::PythonBridgeProcessLauncher launcher(QStringLiteral("/definitely/not/hcb"));
  const hcb::PythonBridgeLaunchResult result = launcher.start();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
}

QTEST_GUILESS_MAIN(PythonBridgeProcessLauncherTest)

#include "PythonBridgeProcessLauncherTest.moc"
