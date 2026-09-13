#pragma once

#include "core/GoogleApiError.h"

#include <QString>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

class GoogleHttpClient;

struct GoogleCalendarCreateRequest final {
  QString title;
  std::optional<QString> description;
  std::optional<QString> timeZone;
};

struct GoogleCalendarSubscribeRequest final {
  QString calendarId;
  bool selected{true};
  bool hidden{false};
  std::optional<QString> colorId;
};

struct GoogleCalendarUpdateRequest final {
  QString calendarId;
  QString title;
  std::optional<QString> description;
  std::optional<QString> timeZone;
};

struct GoogleCalendarListUpdateRequest final {
  QString calendarId;
  bool selected{true};
  bool hidden{false};
  std::optional<QString> colorId;
};

struct GoogleCalendarManagementResult final {
  QString calendarId;
};

using GoogleCalendarManagementResultOrError =
    std::variant<GoogleCalendarManagementResult, GoogleApiError>;

class GoogleCalendarManagementClient final {
public:
  explicit GoogleCalendarManagementClient(GoogleHttpClient& httpClient);

  [[nodiscard]] std::future<GoogleCalendarManagementResultOrError>
  create(GoogleCalendarCreateRequest request, QString accessToken);
  [[nodiscard]] std::future<GoogleCalendarManagementResultOrError>
  subscribe(GoogleCalendarSubscribeRequest request, QString accessToken);
  [[nodiscard]] std::future<GoogleCalendarManagementResultOrError>
  update(GoogleCalendarUpdateRequest request, QString accessToken);
  [[nodiscard]] std::future<GoogleCalendarManagementResultOrError>
  remove(QString calendarId, QString accessToken);
  [[nodiscard]] std::future<GoogleCalendarManagementResultOrError>
  updateListEntry(GoogleCalendarListUpdateRequest request, QString accessToken);
  [[nodiscard]] std::future<GoogleCalendarManagementResultOrError>
  removeListEntry(QString calendarId, QString accessToken);

private:
  GoogleHttpClient& httpClient_;
};

} // namespace hcb
