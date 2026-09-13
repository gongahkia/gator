#pragma once

#include <QByteArray>
#include <QString>

#include <cstddef>
#include <cstdint>
#include <optional>
#include <utility>
#include <vector>

namespace hcb {

struct NativePerformanceFixtureCounts final {
  std::size_t tasks;
  std::size_t eventInstances;
  std::size_t notes;
  std::size_t recurrenceExceptions;
  std::size_t queuedMutations;
};

struct NativePerformanceTaskFixture final {
  QString id;
  QString taskListId;
  std::optional<QString> parentTaskId;
  QString title;
  QString status;
  std::optional<QString> dueAt;
  std::optional<QString> completedAt;
  QString updatedAt;
  std::size_t sortOrder;
};

struct NativePerformanceEventFixture final {
  QString id;
  QString calendarId;
  QString title;
  QString startsAt;
  QString endsAt;
  bool isAllDay;
  std::optional<QString> recurringEventId;
  std::optional<QString> originalStartAt;
  QString updatedAt;
};

struct NativePerformanceNoteFixture final {
  QString id;
  std::optional<QString> linkedResourceType;
  std::optional<QString> linkedResourceId;
  QString title;
  QString body;
  QString updatedAt;
};

struct NativePerformanceQueuedMutationFixture final {
  QString id;
  QString resourceType;
  QString resourceId;
};

struct NativePerformanceFixture final {
  std::uint32_t schemaVersion{1};
  QString size;
  QString seed;
  bool generatedDataOnly{true};
  QString baseTime;
  NativePerformanceFixtureCounts counts;
  std::vector<std::pair<QString, QString>> taskLists;
  std::vector<std::pair<QString, QString>> calendars;
  std::vector<NativePerformanceTaskFixture> tasks;
  std::vector<NativePerformanceEventFixture> eventInstances;
  std::vector<NativePerformanceNoteFixture> notes;
  std::vector<NativePerformanceQueuedMutationFixture> queuedMutations;
};

class NativePerformanceFixtureGenerator final {
public:
  [[nodiscard]] static NativePerformanceFixture small();
  [[nodiscard]] static NativePerformanceFixture medium();
  [[nodiscard]] static NativePerformanceFixture event15k();
  [[nodiscard]] static NativePerformanceFixture wrapperScale();
  [[nodiscard]] static QByteArray toJson(const NativePerformanceFixture& fixture);
};

} // namespace hcb
