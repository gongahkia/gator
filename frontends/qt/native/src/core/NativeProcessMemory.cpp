#include "core/NativeProcessMemory.h"

#if defined(Q_OS_MACOS)
#include <mach/mach.h>
#elif defined(Q_OS_WIN)
#include <windows.h>

#include <psapi.h>
#elif defined(Q_OS_LINUX)
#include <QFile>
#include <QRegularExpression>
#endif

namespace hcb {

std::optional<quint64> NativeProcessMemory::residentBytes() {
#if defined(Q_OS_MACOS)
  mach_task_basic_info info{};
  mach_msg_type_number_t count = MACH_TASK_BASIC_INFO_COUNT;
  const kern_return_t result = task_info(
      mach_task_self(), MACH_TASK_BASIC_INFO, reinterpret_cast<task_info_t>(&info), &count);
  if (result != KERN_SUCCESS) {
    return std::nullopt;
  }
  return static_cast<quint64>(info.resident_size);
#elif defined(Q_OS_WIN)
  PROCESS_MEMORY_COUNTERS_EX counters{};
  counters.cb = sizeof(counters);
  if (GetProcessMemoryInfo(GetCurrentProcess(),
                           reinterpret_cast<PROCESS_MEMORY_COUNTERS*>(&counters),
                           sizeof(counters)) == FALSE) {
    return std::nullopt;
  }
  return static_cast<quint64>(counters.WorkingSetSize);
#elif defined(Q_OS_LINUX)
  QFile status(QStringLiteral("/proc/self/status"));
  if (!status.open(QIODevice::ReadOnly | QIODevice::Text)) {
    return std::nullopt;
  }
  const QRegularExpression expression(QStringLiteral(R"(^VmRSS:\s+(\d+)\s+kB$)"),
                                      QRegularExpression::MultilineOption);
  const QRegularExpressionMatch match = expression.match(QString::fromUtf8(status.readAll()));
  bool valid = false;
  const quint64 kibibytes = match.captured(1).toULongLong(&valid);
  if (!valid) {
    return std::nullopt;
  }
  return kibibytes * 1'024U;
#else
  return std::nullopt;
#endif
}

} // namespace hcb
