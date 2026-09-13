#include <QCoreApplication>
#include <QTextStream>

#include "core/NativePerformanceFixture.h"

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  QTextStream(stdout) << hcb::NativePerformanceFixtureGenerator::toJson(
                             hcb::NativePerformanceFixtureGenerator::event15k())
                      << '\n';
  return 0;
}
