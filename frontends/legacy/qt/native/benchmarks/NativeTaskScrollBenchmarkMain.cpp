#include <QGuiApplication>
#include <QTextStream>

#include <optional>

#include "core/NativeTaskScrollBenchmark.h"

int main(int argc, char* argv[]) {
  QGuiApplication application(argc, argv);
  const std::optional<hcb::NativeTaskScrollBenchmarkResult> result =
      hcb::NativeTaskScrollBenchmark::run(5);
  if (!result.has_value()) {
    return 1;
  }
  QTextStream(stdout) << hcb::NativeTaskScrollBenchmark::toJson(*result) << '\n';
  return 0;
}
