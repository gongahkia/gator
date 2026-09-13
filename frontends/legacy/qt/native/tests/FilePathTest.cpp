#include <QtTest>

#include <QDir>

#include "core/FilePath.h"

class FilePathTest final : public QObject {
  Q_OBJECT

private slots:
  void normalizesAbsolutePaths();
  void rejectsInvalidRootsAndChildren();
};

void FilePathTest::normalizesAbsolutePaths() {
  const std::optional<hcb::FilePath> root = hcb::FilePath::fromAbsolute(QDir::tempPath());
  QVERIFY(root.has_value());
  if (!root.has_value()) {
    return;
  }

  const std::optional<hcb::FilePath> child = root->child(u"settings.sqlite");
  QVERIFY(child.has_value());
  if (!child.has_value()) {
    return;
  }
  QCOMPARE(child->nativePath(),
           QDir(root->nativePath()).filePath(QStringLiteral("settings.sqlite")));
}

void FilePathTest::rejectsInvalidRootsAndChildren() {
  QVERIFY(!hcb::FilePath::fromAbsolute(QString{}).has_value());
  QVERIFY(!hcb::FilePath::fromAbsolute(QStringLiteral("relative/path")).has_value());

  const std::optional<hcb::FilePath> root = hcb::FilePath::fromAbsolute(QDir::tempPath());
  QVERIFY(root.has_value());
  if (!root.has_value()) {
    return;
  }
  QVERIFY(!root->child(QStringView{}).has_value());
  QVERIFY(!root->child(u".").has_value());
  QVERIFY(!root->child(u"..").has_value());
  QVERIFY(!root->child(u"nested/file").has_value());
  QVERIFY(!root->child(u"nested\\file").has_value());
}

QTEST_MAIN(FilePathTest)
#include "FilePathTest.moc"
