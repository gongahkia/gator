#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "core/GoogleCalendarEventPullClient.h"
#include "core/GoogleCalendarListPullClient.h"
#include "core/GoogleTaskListPullClient.h"
#include "core/GoogleTaskPullClient.h"
#include "data/SqliteWriterQueue.h"

#include <QList>
#include <QString>

#include <future>
#include <variant>

namespace hcb {

using GoogleMirrorWriteResult = std::variant<std::monostate, AppError>;

class GoogleMirrorStore final {
public:
  GoogleMirrorStore(FilePath databasePath, const Clock& clock);
  GoogleMirrorStore(const GoogleMirrorStore&) = delete;
  GoogleMirrorStore& operator=(const GoogleMirrorStore&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<GoogleMirrorWriteResult> replaceTasks(
      QString accountId, QList<GoogleTaskListMirror> taskLists, QList<GoogleTaskMirror> tasks);
  [[nodiscard]] std::future<GoogleMirrorWriteResult>
  mergeTaskLists(QString accountId, QList<GoogleTaskListMirror> taskLists, bool fullReconciliation);
  [[nodiscard]] std::future<GoogleMirrorWriteResult>
  mergeTasks(QString accountId,
             QString taskListRemoteId,
             QList<GoogleTaskMirror> tasks,
             bool fullReconciliation);
  [[nodiscard]] std::future<GoogleMirrorWriteResult>
  replaceCalendars(QString accountId,
                   QList<GoogleCalendarMirror> calendars,
                   QList<GoogleCalendarEventMirror> events);
  [[nodiscard]] std::future<GoogleMirrorWriteResult>
  mergeCalendars(QString accountId,
                 QList<GoogleCalendarMirror> calendars,
                 bool fullReconciliation);
  [[nodiscard]] std::future<GoogleMirrorWriteResult>
  mergeCalendarEvents(QString accountId,
                      QString calendarRemoteId,
                      QList<GoogleCalendarEventMirror> events,
                      bool fullReconciliation);
  [[nodiscard]] std::future<GoogleMirrorWriteResult>
  cacheCalendarInstances(QString accountId,
                         QString calendarRemoteId,
                         QString recurringRemoteId,
                         QString rangeStartAt,
                         QString rangeEndAt,
                         QList<GoogleCalendarEventMirror> events);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
