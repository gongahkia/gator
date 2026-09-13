#pragma once

#include "core/SyncConflictPolicy.h"
#include "core/SyncConflictStore.h"

#include <QJsonObject>
#include <QList>
#include <QString>

namespace hcb {

enum class SyncMergeDecision : std::uint8_t {
  ReapplyLocal,
  KeepRemote,
  RequireUser
};

struct SyncFieldConflict final {
  QString field;
  QJsonValue base;
  QJsonValue local;
  QJsonValue remote;
};

struct SyncThreeWayMergeInput final {
  SyncConflictResource resource{SyncConflictResource::Task};
  QString operation;
  QJsonObject baseSnapshot;
  QJsonObject localIntent;
  QJsonObject remoteSnapshot;
  SyncConflictPolicy policy{SyncConflictPolicy::PreferGoogle};
};

struct SyncThreeWayMergeResult final {
  SyncMergeDecision decision{SyncMergeDecision::RequireUser};
  QJsonObject reapplyIntent;
  QList<SyncFieldConflict> conflicts;
  bool structural{false};
};

class SyncThreeWayMerge final {
public:
  [[nodiscard]] static SyncThreeWayMergeResult merge(SyncThreeWayMergeInput input);
};

} // namespace hcb
