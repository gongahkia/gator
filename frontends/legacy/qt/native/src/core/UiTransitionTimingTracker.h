#pragma once

#include "core/Clock.h"
#include "core/StructuredLogger.h"

#include <chrono>
#include <cstddef>
#include <vector>

#include <QObject>

namespace hcb {

struct UiTransitionTimingSpan final {
  QString name;
  std::chrono::milliseconds elapsed;
};

class UiTransitionTimingTracker final : public QObject {
  Q_OBJECT

public:
  UiTransitionTimingTracker(const Clock& clock,
                            StructuredLogger& logger,
                            std::size_t maximumActiveTransitions = 8,
                            std::size_t maximumSpans = 64,
                            QObject* parent = nullptr);

  Q_INVOKABLE bool begin(const QString& name);
  Q_INVOKABLE bool complete(const QString& name);
  [[nodiscard]] std::vector<UiTransitionTimingSpan> spans() const;

private:
  struct ActiveTransition final {
    QString name;
    MonotonicTimePoint startedAt;
  };

  [[nodiscard]] static QString safeName(const QString& name);

  const Clock& clock_;
  StructuredLogger& logger_;
  const std::size_t maximumActiveTransitions_;
  const std::size_t maximumSpans_;
  std::vector<ActiveTransition> activeTransitions_;
  std::vector<UiTransitionTimingSpan> spans_;
};

} // namespace hcb
