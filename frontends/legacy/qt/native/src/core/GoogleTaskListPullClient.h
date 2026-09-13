#pragma once

#include "core/GoogleApiError.h"

#include <QList>
#include <QString>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

class GoogleHttpClient;

struct GoogleTaskListMirror final {
  QString id;
  QString title;
  std::optional<QString> updatedAt;
  std::optional<QString> etag;
};

struct GoogleTaskListPullResult final {
  QList<GoogleTaskListMirror> taskLists;
  std::optional<QString> serverDate;
};

using GoogleTaskListPullResultOrError = std::variant<GoogleTaskListPullResult, GoogleApiError>;

class GoogleTaskListPullClient final {
public:
  explicit GoogleTaskListPullClient(GoogleHttpClient& httpClient);

  [[nodiscard]] std::future<GoogleTaskListPullResultOrError> list(QString accessToken);

private:
  GoogleHttpClient& httpClient_;
};

} // namespace hcb
