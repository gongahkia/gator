#include <QCoreApplication>
#include <QTextStream>

#include <optional>

#include "core/NativeSyncApplyBenchmark.h"

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  const std::optional<hcb::NativeSyncApplyBenchmarkResult> result =
      hcb::NativeSyncApplyBenchmark::run();
  if (!result.has_value()) {
    return 1;
  }
  QTextStream(stdout) << hcb::NativeSyncApplyBenchmark::toJson(*result) << '\n';
  return 0;
}
