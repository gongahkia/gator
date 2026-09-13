#pragma once

#include "core/GoogleApiError.h"

#include <QHash>
#include <QList>
#include <QString>

#include <future>
#include <variant>

namespace hcb {

class GoogleHttpClient;

struct GoogleCalendarBusyInterval final {
  QString startAt;
  QString endAt;
};

struct GoogleCalendarFreeBusyRequest final {
  QString startAt;
  QString endAt;
  QList<QString> calendarIds;
};

struct GoogleCalendarFreeBusyResult final {
  QHash<QString, QList<GoogleCalendarBusyInterval>> intervalsByCalendar;
};

using GoogleCalendarFreeBusyResultOrError =
    std::variant<GoogleCalendarFreeBusyResult, GoogleApiError>;

class GoogleCalendarFreeBusyClient final {
public:
  explicit GoogleCalendarFreeBusyClient(GoogleHttpClient& httpClient);

  [[nodiscard]] std::future<GoogleCalendarFreeBusyResultOrError>
  query(GoogleCalendarFreeBusyRequest request, QString accessToken);

private:
  GoogleHttpClient& httpClient_;
};

} // namespace hcb
