#include <QCoreApplication>
#include <QTextStream>

#include <optional>

#include "core/NativeLocalSearchBenchmark.h"

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  const std::optional<hcb::NativeLocalSearchBenchmarkResult> result =
      hcb::NativeLocalSearchBenchmark::run(5);
  if (!result.has_value()) {
    return 1;
  }
  QTextStream(stdout) << hcb::NativeLocalSearchBenchmark::toJson(*result) << '\n';
  return 0;
}
