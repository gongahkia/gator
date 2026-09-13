#pragma once

#include <QDate>
#include <QDateTime>
#include <QList>
#include <QString>
#include <QStringList>
#include <QTime>
#include <QTimeZone>

#include <cstdint>
#include <optional>

namespace hcb {

enum class QuickCaptureKind : std::int32_t {
  Task = 0,
  Event = 1,
};

struct QuickCaptureAliases final {
  QStringList task;
  QStringList event;
  QStringList highPriority;
  QStringList mediumPriority;
  QStringList lowPriority;
};

struct QuickCaptureRecognition final {
  QString id;
  QString label;
  bool removable{true};
};

struct QuickCaptureRecurrence final {
  bool enabled{false};
  std::int32_t frequency{-1};
  std::int32_t interval{1};
  QString rrule;
};

struct QuickCaptureParseRequest final {
  QString text;
  QuickCaptureKind kind{QuickCaptureKind::Event};
  QDateTime now;
  QTimeZone timeZone;
  std::int32_t defaultEventDurationMinutes{30};
  QuickCaptureAliases aliases;
  QStringList disabledRecognitionIds;
};

struct QuickCaptureParseResult final {
  QuickCaptureKind kind{QuickCaptureKind::Event};
  QString rawTitle;
  QString parsedTitle;
  std::optional<QDate> date;
  std::optional<QTime> time;
  bool allDay{false};
  std::int32_t eventDurationMinutes{30};
  std::int32_t taskPriority{0};
  QuickCaptureRecurrence recurrence;
  QList<QuickCaptureRecognition> recognitions;
  bool eventReady{false};
};

class QuickCaptureParser final {
public:
  [[nodiscard]] static QuickCaptureAliases defaultAliases();
  [[nodiscard]] static QuickCaptureParseResult parse(const QuickCaptureParseRequest& request);
};

} // namespace hcb
