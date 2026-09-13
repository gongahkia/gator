#pragma once

#include <QByteArray>
#include <QString>
#include <QStringList>
#include <QtGlobal>

#include <chrono>
#include <optional>

namespace hcb {

class NativeIdleRssBenchmark final {
public:
  [[nodiscard]] static std::optional<quint64> measure(const QString& executable,
                                                      const QStringList& arguments,
                                                      std::chrono::milliseconds idleDuration,
                                                      std::chrono::milliseconds timeout,
                                                      QString* error);
  [[nodiscard]] static std::optional<quint64> parseResidentBytes(const QByteArray& output);
};

} // namespace hcb
