#pragma once

#include "core/GoogleApiError.h"

#include <QList>
#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

class GoogleHttpClient;

enum class GoogleTaskStatus : std::uint8_t {
  NeedsAction,
  Completed
};

struct GoogleTaskMirror final {
  QString id;
  QString taskListId;
  std::optional<QString> parentId;
  QString title;
  std::optional<QString> notes;
  GoogleTaskStatus status;
  std::optional<QString> dueAt;
  std::optional<QString> completedAt;
  bool deleted;
  bool hidden;
  bool isAssigned;
  std::optional<QString> position;
  std::optional<QString> etag;
  std::optional<QString> updatedAt;
};

struct GoogleTaskPullRequest final {
  QString taskListId;
  std::optional<QString> updatedMin;
  std::optional<QString> completedMin;
  bool showCompleted{true};
};

struct GoogleTaskPullResult final {
  QList<GoogleTaskMirror> tasks;
  std::optional<QString> serverDate;
};

using GoogleTaskPullResultOrError = std::variant<GoogleTaskPullResult, GoogleApiError>;

class GoogleTaskPullClient final {
public:
  explicit GoogleTaskPullClient(GoogleHttpClient& httpClient);

  [[nodiscard]] std::future<GoogleTaskPullResultOrError> list(GoogleTaskPullRequest request,
                                                              QString accessToken);

private:
  GoogleHttpClient& httpClient_;
};

} // namespace hcb
