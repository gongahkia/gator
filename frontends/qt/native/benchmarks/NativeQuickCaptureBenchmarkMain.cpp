#include <QCoreApplication>
#include <QTextStream>

#include <optional>

#include "core/NativeQuickCaptureBenchmark.h"

int main(int argc, char* argv[]) {
  QCoreApplication application(argc, argv);
  const std::optional<hcb::NativeQuickCaptureBenchmarkResult> result =
      hcb::NativeQuickCaptureBenchmark::run(5);
  if (!result.has_value()) {
    return 1;
  }
  QTextStream(stdout) << hcb::NativeQuickCaptureBenchmark::toJson(*result) << '\n';
  return 0;
}
