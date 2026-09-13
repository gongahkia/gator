#pragma once

#include "core/DiagnosticsSnapshot.h"

#include <QByteArray>

namespace hcb {

class DiagnosticsJsonExporter final {
public:
  [[nodiscard]] static QByteArray exportSnapshot(const DiagnosticsSnapshot& snapshot);
};

} // namespace hcb
