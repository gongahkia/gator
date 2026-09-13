#include <QtTest>

#include "app/AppPaths.h"
#include "app/LinuxPathsAdapter.h"
#include "app/MacOSPathsAdapter.h"
#include "app/WindowsPathsAdapter.h"

#include <QStandardPaths>

class AppPathsTest final : public QObject {
  Q_OBJECT

private slots:
  void discoversStableApplicationDirectories();
  void discoversMacOSApplicationDirectories();
  void discoversLinuxApplicationDirectories();
  void discoversWindowsApplicationDirectories();
};

void AppPathsTest::discoversStableApplicationDirectories() {
  const std::optional<hcb::AppPaths> discoveredPaths = hcb::AppPaths::discover();
  QVERIFY(discoveredPaths.has_value());
  if (!discoveredPaths.has_value()) {
    return;
  }
  const hcb::AppPaths& paths = *discoveredPaths;

  QVERIFY(!paths.dataDirectory().nativePath().isEmpty());
  QVERIFY(!paths.cacheDirectory().nativePath().isEmpty());
}

void AppPathsTest::discoversMacOSApplicationDirectories() {
#if defined(Q_OS_MACOS)
  const std::optional<hcb::MacOSPathLocations> locations = hcb::MacOSPathsAdapter::discover();
  QVERIFY(locations.has_value());
  if (!locations.has_value()) {
    return;
  }
  QCOMPARE(locations->dataDirectory,
           QStandardPaths::writableLocation(QStandardPaths::AppDataLocation));
  QCOMPARE(locations->cacheDirectory,
           QStandardPaths::writableLocation(QStandardPaths::CacheLocation));

  const std::optional<hcb::AppPaths> paths = hcb::AppPaths::discover();
  QVERIFY(paths.has_value());
  if (!paths.has_value()) {
    return;
  }
  QCOMPARE(paths->dataDirectory().nativePath(), locations->dataDirectory);
  QCOMPARE(paths->cacheDirectory().nativePath(), locations->cacheDirectory);
#else
  QSKIP("macOS-only adapter");
#endif
}

void AppPathsTest::discoversLinuxApplicationDirectories() {
#if defined(Q_OS_LINUX)
  const std::optional<hcb::LinuxPathLocations> locations = hcb::LinuxPathsAdapter::discover();
  QVERIFY(locations.has_value());
  if (!locations.has_value()) {
    return;
  }
  QCOMPARE(locations->dataDirectory,
           QStandardPaths::writableLocation(QStandardPaths::AppDataLocation));
  QCOMPARE(locations->cacheDirectory,
           QStandardPaths::writableLocation(QStandardPaths::CacheLocation));

  const std::optional<hcb::AppPaths> paths = hcb::AppPaths::discover();
  QVERIFY(paths.has_value());
  if (!paths.has_value()) {
    return;
  }
  QCOMPARE(paths->dataDirectory().nativePath(), locations->dataDirectory);
  QCOMPARE(paths->cacheDirectory().nativePath(), locations->cacheDirectory);
#else
  QSKIP("Linux-only adapter");
#endif
}

void AppPathsTest::discoversWindowsApplicationDirectories() {
#if defined(Q_OS_WIN)
  const std::optional<hcb::WindowsPathLocations> locations = hcb::WindowsPathsAdapter::discover();
  QVERIFY(locations.has_value());
  if (!locations.has_value()) {
    return;
  }
  QCOMPARE(locations->dataDirectory,
           QStandardPaths::writableLocation(QStandardPaths::AppDataLocation));
  QCOMPARE(locations->cacheDirectory,
           QStandardPaths::writableLocation(QStandardPaths::CacheLocation));

  const std::optional<hcb::AppPaths> paths = hcb::AppPaths::discover();
  QVERIFY(paths.has_value());
  if (!paths.has_value()) {
    return;
  }
  QCOMPARE(paths->dataDirectory().nativePath(), locations->dataDirectory);
  QCOMPARE(paths->cacheDirectory().nativePath(), locations->cacheDirectory);
#else
  QSKIP("Windows-only adapter");
#endif
}

QTEST_MAIN(AppPathsTest)
#include "AppPathsTest.moc"
