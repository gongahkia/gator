#include <QCoreApplication>
#include <QProcess>
#include <QProcessEnvironment>
#include <QtTest>

#include <utility>

namespace {

constexpr int kStartTimeoutMilliseconds = 5'000;
constexpr int kExitTimeoutMilliseconds = 15'000;

class NativeQmlShellSmokeTest final : public QObject {
  Q_OBJECT

public:
  explicit NativeQmlShellSmokeTest(QString executable) : executable_(std::move(executable)) {}

private slots:
  void loadsCompiledQmlModuleOffscreen();
  void routesLaunchDeepLinkOffscreen();

private:
  QString executable_;
};

void NativeQmlShellSmokeTest::loadsCompiledQmlModuleOffscreen() {
  QProcess process;
  QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
  environment.insert(QStringLiteral("HCB_BENCHMARK_EXIT_AFTER_LOAD"), QStringLiteral("1"));
  process.setProcessEnvironment(environment);
  process.setProcessChannelMode(QProcess::SeparateChannels);
  process.start(executable_, {QStringLiteral("-platform"), QStringLiteral("offscreen")});

  if (!process.waitForStarted(kStartTimeoutMilliseconds)) {
    QFAIL(qPrintable(process.errorString()));
    return;
  }
  if (!process.waitForFinished(kExitTimeoutMilliseconds)) {
    process.kill();
    process.waitForFinished();
    QFAIL(qPrintable(process.errorString()));
    return;
  }

  const QByteArray standardError = process.readAllStandardError();
  QCOMPARE(process.exitStatus(), QProcess::NormalExit);
  QVERIFY2(process.exitCode() == 0, standardError.constData());
}

void NativeQmlShellSmokeTest::routesLaunchDeepLinkOffscreen() {
  QProcess process;
  QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
  environment.insert(QStringLiteral("HCB_BENCHMARK_EXIT_AFTER_LOAD"), QStringLiteral("1"));
  process.setProcessEnvironment(environment);
  process.setProcessChannelMode(QProcess::SeparateChannels);
  process.start(executable_,
                {QStringLiteral("-platform"),
                 QStringLiteral("offscreen"),
                 QStringLiteral("hotcrossbuns://settings")});

  if (!process.waitForStarted(kStartTimeoutMilliseconds)) {
    QFAIL(qPrintable(process.errorString()));
    return;
  }
  if (!process.waitForFinished(kExitTimeoutMilliseconds)) {
    process.kill();
    process.waitForFinished();
    QFAIL(qPrintable(process.errorString()));
    return;
  }

  const QByteArray standardError = process.readAllStandardError();
  QCOMPARE(process.exitStatus(), QProcess::NormalExit);
  QVERIFY2(process.exitCode() == 0, standardError.constData());
  QVERIFY2(!standardError.contains("No such method"), standardError.constData());
}

} // namespace

int main(int argc, char* argv[]) {
  if (argc != 2) {
    return 2;
  }
  QCoreApplication application(argc, argv);
  NativeQmlShellSmokeTest test(QString::fromLocal8Bit(argv[1]));
  return QTest::qExec(&test, 1, argv);
}

#include "NativeQmlShellSmokeTest.moc"
