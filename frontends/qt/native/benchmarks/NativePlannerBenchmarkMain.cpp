#include <QCoreApplication>
#include <QTextStream>

#include "core/NativePerformanceFixture.h"
#include "core/NativePlannerBenchmark.h"

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  const std::optional<hcb::NativePlannerBenchmarkResult> result =
      hcb::NativePlannerBenchmark::run(hcb::NativePerformanceFixtureGenerator::event15k());
  if (!result.has_value()) {
    return 1;
  }
  QTextStream(stdout) << hcb::NativePlannerBenchmark::toJson(*result) << '\n';
  return 0;
}
