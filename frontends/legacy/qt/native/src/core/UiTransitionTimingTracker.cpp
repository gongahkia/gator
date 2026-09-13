#include "core/UiTransitionTimingTracker.h"

#include "core/SecretRedactor.h"

#include <algorithm>
#include <utility>

namespace hcb {

UiTransitionTimingTracker::UiTransitionTimingTracker(const Clock& clock,
                                                     StructuredLogger& logger,
                                                     std::size_t maximumActiveTransitions,
                                                     std::size_t maximumSpans,
                                                     QObject* parent)
    : QObject(parent), clock_(clock), logger_(logger),
      maximumActiveTransitions_(std::max<std::size_t>(maximumActiveTransitions, 1)),
      maximumSpans_(std::max<std::size_t>(maximumSpans, 1)) {
  activeTransitions_.reserve(maximumActiveTransitions_);
  spans_.reserve(maximumSpans_);
}

bool UiTransitionTimingTracker::begin(const QString& name) {
  const QString transitionName = safeName(name);
  if (transitionName.isEmpty() || activeTransitions_.size() >= maximumActiveTransitions_ ||
      std::any_of(activeTransitions_.cbegin(),
                  activeTransitions_.cend(),
                  [&transitionName](const ActiveTransition& transition) {
                    return transition.name == transitionName;
                  })) {
    return false;
  }

  activeTransitions_.push_back(ActiveTransition{transitionName, clock_.monotonicNow()});
  return true;
}

bool UiTransitionTimingTracker::complete(const QString& name) {
  const QString transitionName = safeName(name);
  const auto transition = std::find_if(activeTransitions_.cbegin(),
                                       activeTransitions_.cend(),
                                       [&transitionName](const ActiveTransition& candidate) {
                                         return candidate.name == transitionName;
                                       });
  if (transition == activeTransitions_.cend()) {
    return false;
  }

  std::chrono::milliseconds elapsed = std::chrono::duration_cast<std::chrono::milliseconds>(
      clock_.monotonicNow() - transition->startedAt);
  if (elapsed < std::chrono::milliseconds::zero()) {
    elapsed = std::chrono::milliseconds::zero();
  }
  activeTransitions_.erase(transition);
  if (spans_.size() == maximumSpans_) {
    spans_.erase(spans_.begin());
  }
  spans_.push_back(UiTransitionTimingSpan{transitionName, elapsed});
  logger_.log(LogLevel::Info,
              {u"ui.transition", u"ui transition completed"},
              {{QStringLiteral("span"), transitionName},
               {QStringLiteral("elapsed_ms"), QString::number(elapsed.count())}});
  return true;
}

std::vector<UiTransitionTimingSpan> UiTransitionTimingTracker::spans() const { return spans_; }

QString UiTransitionTimingTracker::safeName(const QString& name) {
  return SecretRedactor::redactText(name, 120);
}

} // namespace hcb
