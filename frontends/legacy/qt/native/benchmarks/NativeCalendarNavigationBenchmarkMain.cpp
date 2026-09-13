#include <QGuiApplication>
#include <QTextStream>

#include <optional>

#include "core/NativeCalendarNavigationBenchmark.h"

int main(int argc, char* argv[]) {
  QGuiApplication application(argc, argv);
  const std::optional<hcb::NativeCalendarNavigationBenchmarkResult> result =
      hcb::NativeCalendarNavigationBenchmark::run(5);
  if (!result.has_value()) {
    return 1;
  }
  QTextStream(stdout) << hcb::NativeCalendarNavigationBenchmark::toJson(*result) << '\n';
  return 0;
}
