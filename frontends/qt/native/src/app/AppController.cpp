#include "app/AppController.h"

#include "app/LinuxCredentialAdapter.h"
#include "app/MacOSCredentialAdapter.h"
#include "app/WindowsCredentialAdapter.h"

#include "core/AgendaModel.h"
#include "core/CalendarSourceModel.h"
#include "core/LocalSearchQuery.h"
#include "core/MonthGridModel.h"
#include "core/NotesModel.h"
#include "core/SearchResultsModel.h"
#include "core/ReminderService.h"
#include "core/SecretRedactor.h"
#include "core/TaskListModel.h"
#include "core/TaskModel.h"
#include "core/TaskRecurrenceMarker.h"
#include "core/TimelineModel.h"

#include <QDate>
#include <QDateTime>
#include <QDebug>
#include <QColor>
#include <QCoreApplication>
#include <QDesktopServices>
#include <QElapsedTimer>
#include <QFile>
#include <QFileDialog>
#include <QFileInfo>
#include <QFontMetrics>
#include <QFontDatabase>
#include <QJsonArray>
#include <QJsonDocument>
#include <QLocale>
#include <QMetaType>
#include <QProcess>
#include <QRegularExpression>
#include <QSaveFile>
#include <QSet>
#include <QTimeZone>
#include <QTimer>
#include <QThread>
#include <QUrlQuery>
#include <QUuid>
#include <QVariantMap>

#include <algorithm>
#include <chrono>
#include <memory>
#include <optional>
#include <thread>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr int kCalendarTimelineDays = 7;
constexpr int kVisibleAllDayLaneCount = 2;
constexpr char kGoogleAccountId[] = "google";
constexpr char kSyncSettingsScope[] = "sync";
constexpr char kConflictPolicySettingsKey[] = "conflict_policy";
constexpr char kPresentationSettingsScope[] = "presentation";
constexpr char kNotesEnabledSettingsKey[] = "notes_enabled";
constexpr char kNotesProjectionSettingsKey[] = "notes_projection";
constexpr char kAppearanceModeSettingsKey[] = "appearance_mode";
constexpr char kVisualDensitySettingsKey[] = "visual_density";
constexpr char kTaskListPaneWidthSettingsKey[] = "task_list_pane_width";
constexpr char kPaletteModeSettingsKey[] = "palette_mode";
constexpr char kAccentColorSettingsKey[] = "accent_color";
constexpr char kFontFamilySettingsKey[] = "font_family";
constexpr char kFontScaleSettingsKey[] = "font_scale";
constexpr char kBulkTextRecurrenceScopeSettingsKey[] = "bulk_text_recurrence_scope";
constexpr char kQuickCaptureDefaultTaskListSettingsKey[] = "quick_capture_default_task_list";
constexpr char kQuickCaptureDefaultCalendarSettingsKey[] = "quick_capture_default_calendar";
constexpr char kQuickCaptureEventDurationSettingsKey[] = "quick_capture_event_duration_minutes";
constexpr char kQuickCaptureRemoveParsedTextSettingsKey[] = "quick_capture_remove_parsed_text";
constexpr char kQuickCaptureTaskAliasesSettingsKey[] = "quick_capture_task_aliases";
constexpr char kQuickCaptureEventAliasesSettingsKey[] = "quick_capture_event_aliases";
constexpr char kQuickCaptureHighPriorityAliasesSettingsKey[] =
    "quick_capture_high_priority_aliases";
constexpr char kQuickCaptureMediumPriorityAliasesSettingsKey[] =
    "quick_capture_medium_priority_aliases";
constexpr char kQuickCaptureLowPriorityAliasesSettingsKey[] = "quick_capture_low_priority_aliases";
constexpr char kWeekStartDaySettingsKey[] = "week_start_day";
constexpr char kUse24HourTimeSettingsKey[] = "use_24_hour_time";
constexpr char kDisplayTimeZoneSettingsKey[] = "display_time_zone";
constexpr char kWorkdayStartHourSettingsKey[] = "workday_start_hour";
constexpr char kUndoRetentionDaysSettingsKey[] = "undo_retention_days";
constexpr char kUndoMaximumEntriesSettingsKey[] = "undo_maximum_entries";
constexpr char kCalendarDragCreateHintSeenSettingsKey[] = "calendar_drag_create_hint_seen";
constexpr char kWorkdayEndHourSettingsKey[] = "workday_end_hour";
constexpr char kCalendarVisibilitySettingsKey[] = "calendar_visibility";
constexpr char kSidebarTabIdsSettingsKey[] = "sidebar_tab_ids";
constexpr char kExternalBrowserSettingsKey[] = "external_browser";
constexpr int kNotesOnlyProjection = 0;
constexpr int kMirrorNotesProjection = 1;
constexpr auto kGoogleSyncInterval = std::chrono::minutes(5);
constexpr int kSearchDebounceMilliseconds = 180;
constexpr int kMinimumTaskListPaneWidth = 200;
constexpr int kMaximumTaskListPaneWidth = 480;
constexpr char kBridgeAcceptanceInitialTaskTitle[] = "Qt bridge interaction task initial";
constexpr char kBridgeAcceptanceUpdatedTaskTitle[] = "Qt bridge interaction task updated";
constexpr char kBridgeAcceptanceInitialTaskNotes[] = "Created by the isolated Qt bridge acceptance";
constexpr char kBridgeAcceptanceUpdatedTaskNotes[] = "Updated by the isolated Qt bridge acceptance";
constexpr char kBridgeAcceptanceRecurringTaskTitle[] = "Qt bridge recurring task";
constexpr char kBridgeAcceptanceUpdatedRecurringTaskTitle[] = "Qt bridge recurring task updated";
constexpr char kBridgeAcceptanceSplitRecurringTaskTitle[] = "Qt bridge split recurring task";
constexpr char kBridgeAcceptanceHierarchyParentTaskTitle[] = "Qt bridge hierarchy parent";
constexpr char kBridgeAcceptanceHierarchyFirstChildTaskTitle[] = "Qt bridge hierarchy first child";
constexpr char kBridgeAcceptanceHierarchySecondChildTaskTitle[] =
    "Qt bridge hierarchy second child";
constexpr char kBridgeAcceptanceBulkFirstTaskTitle[] = "Qt bridge bulk first task";
constexpr char kBridgeAcceptanceBulkSecondTaskTitle[] = "Qt bridge bulk second task";
constexpr char kBridgeAcceptanceSavedSearchName[] = "Qt bridge saved search";
constexpr char kBridgeAcceptanceRenamedSavedSearchName[] = "Qt bridge saved search renamed";
constexpr char kBridgeAcceptanceDeletedSavedSearchName[] = "Qt bridge deleted saved search";
constexpr char kBridgeAcceptanceSavedSearchQuery[] = "release-marker";
constexpr char kBridgeAcceptanceDeletedSavedSearchQuery[] = "type:task release-marker";
constexpr char kBridgeAcceptanceInitialEventTitle[] = "Qt bridge interaction event initial";
constexpr char kBridgeAcceptanceUpdatedEventTitle[] = "Qt bridge interaction event updated";
constexpr char kBridgeAcceptanceInitialEventDescription[] =
    "Created by the isolated Qt bridge acceptance";
constexpr char kBridgeAcceptanceUpdatedEventDescription[] =
    "Updated by the isolated Qt bridge acceptance";
constexpr char kBridgeAcceptanceInitialEventLocation[] = "Initial acceptance location";
constexpr char kBridgeAcceptanceUpdatedEventLocation[] = "Updated acceptance location";
constexpr char kBridgeAcceptanceInitialTaskListTitle[] = "Qt bridge acceptance list initial";
constexpr char kBridgeAcceptanceUpdatedTaskListTitle[] = "Qt bridge acceptance list updated";
constexpr char kBridgeAcceptanceHiddenTaskTitle[] = "Qt bridge acceptance hidden-list task";
constexpr char kBridgeAcceptanceInitialCalendarTitle[] = "Qt bridge acceptance calendar initial";
constexpr char kBridgeAcceptanceUpdatedCalendarTitle[] = "Qt bridge acceptance calendar updated";
constexpr char kBridgeAcceptanceUpdatedCalendarDescription[] =
    "Updated by the isolated Qt bridge calendar acceptance";
constexpr char kBridgeAcceptanceSubscriptionRemoteId[] = "qt-bridge-acceptance-subscription";
constexpr qsizetype kMaximumBridgeOperationIdLength = 256;
constexpr qsizetype kMaximumBridgeOperationProgressItems = 32;
constexpr qsizetype kMaximumBridgeOperationProgressLength = 512;

[[nodiscard]] std::unique_ptr<OAuthCredentialStore> makeCredentialStore() {
#if defined(Q_OS_MACOS)
  return std::make_unique<MacOSCredentialAdapter>();
#elif defined(Q_OS_LINUX)
  return std::make_unique<LinuxCredentialAdapter>();
#elif defined(Q_OS_WIN)
  return std::make_unique<WindowsCredentialAdapter>();
#else
  return {};
#endif
}

[[nodiscard]] QStringList requiredGoogleScopes() {
  return {QStringLiteral("https://www.googleapis.com/auth/tasks"),
          QStringLiteral("https://www.googleapis.com/auth/calendar"),
          QStringLiteral("https://www.googleapis.com/auth/drive.metadata.readonly")};
}

[[nodiscard]] QString authenticationTimestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] QString errorMessage(const AppError& error) { return error.message(); }

[[nodiscard]] bool isValidAppearanceMode(int value) { return value >= 0 && value <= 2; }

[[nodiscard]] bool isValidVisualDensity(int value) { return value >= 0 && value <= 2; }

[[nodiscard]] bool isValidTaskListPaneWidth(int value) {
  return value >= kMinimumTaskListPaneWidth && value <= kMaximumTaskListPaneWidth;
}

[[nodiscard]] bool isValidPaletteMode(int value) { return value >= 0 && value <= 5; }

[[nodiscard]] bool isValidFontScale(int value) { return value >= 0 && value <= 3; }

[[nodiscard]] bool isValidExternalBrowser(const QString& browser) {
  return browser.size() <= 128 && !browser.contains(QChar::Null);
}

[[nodiscard]] bool openExternalUrl(const QUrl& url, const QString& browser) {
  if (browser.isEmpty()) {
    return QDesktopServices::openUrl(url);
  }
#if defined(Q_OS_MACOS)
  return QProcess::startDetached(QStringLiteral("/usr/bin/open"),
                                 {QStringLiteral("-a"), browser, url.toString(QUrl::FullyEncoded)});
#else
  return QDesktopServices::openUrl(url);
#endif
}

[[nodiscard]] bool isValidBulkTextRecurrenceScope(int value) { return value >= 0 && value <= 3; }

[[nodiscard]] bool isValidQuickCaptureKind(int value) {
  return value == static_cast<int>(QuickCaptureKind::Task) ||
         value == static_cast<int>(QuickCaptureKind::Event);
}

[[nodiscard]] bool isValidQuickCaptureDuration(int value) { return value >= 1 && value <= 1'440; }

[[nodiscard]] bool isValidQuickCaptureDestination(const QString& value) {
  return value.isEmpty() ||
         (value == value.trimmed() && value.size() <= 256 && !value.contains(QChar::Null));
}

[[nodiscard]] bool isValidBridgeOperationId(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() &&
         value.size() <= kMaximumBridgeOperationIdLength && !value.contains(QChar::Null) &&
         !value.contains(u'/') && !value.contains(u'\\');
}

[[nodiscard]] bool isValidBridgeOperationState(const QString& value) {
  return value == QStringLiteral("queued") || value == QStringLiteral("running") ||
         value == QStringLiteral("succeeded") || value == QStringLiteral("failed") ||
         value == QStringLiteral("cancelled");
}

[[nodiscard]] bool isValidBridgeOperationProgress(const QJsonValue& value) {
  if (!value.isArray()) {
    return false;
  }
  const QJsonArray progress = value.toArray();
  if (progress.size() > kMaximumBridgeOperationProgressItems) {
    return false;
  }
  return std::all_of(progress.cbegin(), progress.cend(), [](const QJsonValue& item) {
    return item.isString() && item.toString().size() <= kMaximumBridgeOperationProgressLength &&
           !item.toString().contains(QChar::Null);
  });
}

[[nodiscard]] bool
isValidBridgeOperation(const QJsonObject& operation,
                       const QString& expectedAccountId,
                       const std::optional<QString>& expectedOperationId = std::nullopt) {
  const QString operationId = operation.value(QStringLiteral("id")).toString();
  const QString accountId = operation.value(QStringLiteral("account_id")).toString();
  const QString state = operation.value(QStringLiteral("state")).toString();
  return isValidBridgeOperationId(operationId) &&
         (!expectedOperationId.has_value() || operationId == *expectedOperationId) &&
         accountId == expectedAccountId && isValidBridgeOperationState(state) &&
         isValidBridgeOperationProgress(operation.value(QStringLiteral("progress")));
}

[[nodiscard]] bool isValidQuickCaptureAlias(const QString& value) {
  static const QRegularExpression expression(QStringLiteral("^[A-Za-z][A-Za-z0-9_-]{0,31}$"));
  return expression.match(value).hasMatch();
}

[[nodiscard]] std::optional<QStringList> quickCaptureAliasesFromText(const QString& text) {
  QStringList aliases;
  QSet<QString> seen;
  const QStringList parts = text.split(',', Qt::SkipEmptyParts);
  for (const QString& part : parts) {
    const QString alias = part.trimmed();
    if (!isValidQuickCaptureAlias(alias) || seen.contains(alias.toCaseFolded())) {
      return std::nullopt;
    }
    seen.insert(alias.toCaseFolded());
    aliases.append(alias);
  }
  return aliases.isEmpty() ? std::nullopt : std::optional<QStringList>(aliases);
}

[[nodiscard]] bool quickCaptureAliasesAreDistinct(const QStringList& task,
                                                  const QStringList& event,
                                                  const QStringList& high,
                                                  const QStringList& medium,
                                                  const QStringList& low) {
  QSet<QString> seen;
  for (const QStringList* values : {&task, &event, &high, &medium, &low}) {
    for (const QString& value : *values) {
      const QString normalized = value.toCaseFolded();
      if (seen.contains(normalized)) {
        return false;
      }
      seen.insert(normalized);
    }
  }
  return true;
}

[[nodiscard]] QString quickCaptureAliasesToText(const QStringList& aliases) {
  return aliases.join(QStringLiteral(", "));
}

[[nodiscard]] bool isValidAccentColor(const QString& value) {
  if (value.isEmpty()) {
    return true;
  }
  if (value.size() != 7 || value.front() != u'#') {
    return false;
  }
  const QColor color(value);
  return color.isValid() && color.name(QColor::HexRgb).compare(value, Qt::CaseInsensitive) == 0;
}

[[nodiscard]] QStringList installedFontFamilies() {
  QStringList families = QFontDatabase::families();
  families.removeDuplicates();
  std::sort(families.begin(), families.end(), [](const QString& left, const QString& right) {
    return left.compare(right, Qt::CaseInsensitive) < 0;
  });
  return families;
}

[[nodiscard]] bool isUsableTextFontFamily(const QString& family) {
  if (family.isEmpty()) {
    return true;
  }
  const QFontMetrics metrics{QFont(family)};
  return metrics.inFont(QChar::fromLatin1('A')) && metrics.inFont(QChar::fromLatin1('a')) &&
         metrics.inFont(QChar::fromLatin1('0'));
}

[[nodiscard]] bool isValidWeekStartDay(int value) { return value == 0 || value == 1; }

[[nodiscard]] bool isValidWorkdayHours(int startHour, int endHour) {
  return startHour >= 0 && startHour <= 23 && endHour >= 1 && endHour <= 24 && startHour < endHour;
}

[[nodiscard]] QTimeZone resolvedTimeZone(const QString& value) {
  const QTimeZone zone(value.toUtf8());
  return zone.isValid() ? zone : QTimeZone::systemTimeZone();
}

[[nodiscard]] std::optional<QString> jsonArrayString(const QString& value) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(value.toUtf8(), &error);
  if (error.error != QJsonParseError::NoError || !document.isArray() ||
      document.array().size() != 1 || !document.array().at(0).isString()) {
    return std::nullopt;
  }
  return document.array().at(0).toString();
}

[[nodiscard]] QString jsonStringArray(const QString& value) {
  return QString::fromUtf8(QJsonDocument(QJsonArray{value}).toJson(QJsonDocument::Compact));
}

[[nodiscard]] std::optional<QStringList> jsonStringList(const QString& value) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(value.toUtf8(), &error);
  if (error.error != QJsonParseError::NoError || !document.isArray() ||
      document.array().isEmpty() || document.array().size() > 32) {
    return std::nullopt;
  }
  QStringList values;
  values.reserve(document.array().size());
  for (const QJsonValue& item : document.array()) {
    if (!item.isString()) {
      return std::nullopt;
    }
    values.append(item.toString());
  }
  return values;
}

[[nodiscard]] QString jsonStringList(const QStringList& values) {
  QJsonArray array;
  for (const QString& value : values) {
    array.append(value);
  }
  return QString::fromUtf8(QJsonDocument(array).toJson(QJsonDocument::Compact));
}

[[nodiscard]] bool isValidSidebarTabId(const QString& value) {
  return value == QStringLiteral("tasks") || value == QStringLiteral("calendar") ||
         value == QStringLiteral("invitations") || value == QStringLiteral("notes");
}

[[nodiscard]] std::optional<QStringList> sidebarTabIdsFromJson(const QString& value) {
  QJsonParseError error;
  const QJsonDocument document = QJsonDocument::fromJson(value.toUtf8(), &error);
  if (error.error != QJsonParseError::NoError || !document.isArray() ||
      document.array().size() > 4) {
    return std::nullopt;
  }
  QStringList ids;
  QSet<QString> seen;
  for (const QJsonValue& item : document.array()) {
    if (!item.isString() || !isValidSidebarTabId(item.toString()) ||
        seen.contains(item.toString())) {
      return std::nullopt;
    }
    seen.insert(item.toString());
    ids.append(item.toString());
  }
  return ids;
}

[[nodiscard]] QString localizedCalendarDate(const QDate& date, const QString& format) {
  return date.isValid() ? QLocale().toString(date, format) : QString();
}

[[nodiscard]] std::optional<TaskPriority> priorityForValue(int value) {
  switch (value) {
  case static_cast<int>(TaskPriority::None):
    return TaskPriority::None;
  case static_cast<int>(TaskPriority::Low):
    return TaskPriority::Low;
  case static_cast<int>(TaskPriority::Medium):
    return TaskPriority::Medium;
  case static_cast<int>(TaskPriority::High):
    return TaskPriority::High;
  default:
    return std::nullopt;
  }
}

[[nodiscard]] QString priorityText(TaskPriority priority) {
  switch (priority) {
  case TaskPriority::None:
    return QStringLiteral("none");
  case TaskPriority::Low:
    return QStringLiteral("low");
  case TaskPriority::Medium:
    return QStringLiteral("medium");
  case TaskPriority::High:
    return QStringLiteral("high");
  }
  return {};
}

[[nodiscard]] std::optional<TaskPriority> importPriority(const std::optional<QString>& value) {
  if (!value.has_value() || value->trimmed().isEmpty() ||
      value->toCaseFolded() == QStringLiteral("none")) {
    return TaskPriority::None;
  }
  if (value->toCaseFolded() == QStringLiteral("low"))
    return TaskPriority::Low;
  if (value->toCaseFolded() == QStringLiteral("medium"))
    return TaskPriority::Medium;
  if (value->toCaseFolded() == QStringLiteral("high"))
    return TaskPriority::High;
  return std::nullopt;
}

[[nodiscard]] QVariantList importPreviewRowVariants(const ImportParseResult& parsed) {
  QVariantList rows;
  rows.reserve(parsed.rows.size());
  for (const ImportPreviewRow& row : parsed.rows) {
    rows.append(QVariantMap{
        {QStringLiteral("line"), row.sourceLine},
        {QStringLiteral("kind"),
         row.kind == ImportItemKind::Task ? QStringLiteral("Task") : QStringLiteral("Event")},
        {QStringLiteral("title"), row.title},
        {QStringLiteral("accepted"), row.accepted},
        {QStringLiteral("message"), row.message}});
  }
  return rows;
}

template <typename Summary>
[[nodiscard]] std::optional<QString> resolveImportTarget(const std::optional<QString>& name,
                                                         const QString& defaultId,
                                                         const QList<Summary>& candidates) {
  if (!name.has_value()) {
    const auto match =
        std::find_if(candidates.cbegin(), candidates.cend(), [&defaultId](const Summary& item) {
          return item.id == defaultId;
        });
    return match == candidates.cend() ? std::nullopt : std::optional<QString>(match->id);
  }
  std::optional<QString> found;
  for (const Summary& candidate : candidates) {
    if (candidate.title != *name)
      continue;
    if (found.has_value())
      return std::nullopt;
    found = candidate.id;
  }
  return found;
}

[[nodiscard]] std::optional<QList<QString>> recurrenceDatesFromText(const QString& value);

[[nodiscard]] std::optional<QList<QString>>
importRecurrenceDates(const std::optional<QString>& text) {
  if (!text.has_value())
    return QList<QString>{};
  return recurrenceDatesFromText(*text);
}

struct ManagedTaskRecurrenceConfiguration final {
  TaskRecurrenceFrequency frequency;
  std::int32_t interval;
  TaskRecurrenceEndCondition end;
  QString defaultRule;
};

[[nodiscard]] std::optional<ManagedTaskRecurrenceConfiguration> managedTaskRecurrenceConfiguration(
    int frequency, int interval, int endKind, const QString& endUntil, int endCount) {
  std::optional<TaskRecurrenceFrequency> parsedFrequency;
  switch (frequency) {
  case 0:
    parsedFrequency = TaskRecurrenceFrequency::Daily;
    break;
  case 1:
    parsedFrequency = TaskRecurrenceFrequency::Weekly;
    break;
  case 2:
    parsedFrequency = TaskRecurrenceFrequency::Monthly;
    break;
  case 3:
    parsedFrequency = TaskRecurrenceFrequency::Yearly;
    break;
  case 4:
    parsedFrequency = TaskRecurrenceFrequency::Daily;
    break;
  default:
    return std::nullopt;
  }
  if (interval < 1 || interval > 1'000) {
    return std::nullopt;
  }
  TaskRecurrenceEndCondition end;
  switch (endKind) {
  case 0:
    end.kind = TaskRecurrenceEndKind::Never;
    break;
  case 1: {
    const QDate until = QDate::fromString(endUntil.trimmed(), Qt::ISODate);
    if (!until.isValid()) {
      return std::nullopt;
    }
    end.kind = TaskRecurrenceEndKind::Until;
    end.untilDate = until.toString(Qt::ISODate);
    break;
  }
  case 2:
    if (endCount < 1 || endCount > 10'000) {
      return std::nullopt;
    }
    end.kind = TaskRecurrenceEndKind::Count;
    end.count = static_cast<std::int32_t>(endCount);
    break;
  default:
    return std::nullopt;
  }
  return ManagedTaskRecurrenceConfiguration{
      .frequency = *parsedFrequency,
      .interval = static_cast<std::int32_t>(interval),
      .end = std::move(end),
      .defaultRule =
          frequency == 4
              ? QStringLiteral("FREQ=DAILY;INTERVAL=%1;BYDAY=MO,TU,WE,TH,FR").arg(interval)
              : QString()};
}

[[nodiscard]] QString bridgeRecurrenceFrequency(TaskRecurrenceFrequency frequency) {
  switch (frequency) {
  case TaskRecurrenceFrequency::Daily:
    return QStringLiteral("daily");
  case TaskRecurrenceFrequency::Weekly:
    return QStringLiteral("weekly");
  case TaskRecurrenceFrequency::Monthly:
    return QStringLiteral("monthly");
  case TaskRecurrenceFrequency::Yearly:
    return QStringLiteral("yearly");
  }
  return {};
}

[[nodiscard]] QJsonArray bridgeRecurrenceDates(const QList<QString>& dates) {
  QJsonArray result;
  for (const QString& date : dates) {
    result.append(date);
  }
  return result;
}

[[nodiscard]] QJsonObject
bridgeTaskRecurrence(const ManagedTaskRecurrenceConfiguration& configuration,
                     const QString& recurrenceRule,
                     const QList<QString>& exclusionDates,
                     const QList<QString>& additionDates) {
  QJsonObject end{{QStringLiteral("kind"), QStringLiteral("never")}};
  switch (configuration.end.kind) {
  case TaskRecurrenceEndKind::Never:
    break;
  case TaskRecurrenceEndKind::Until:
    end.insert(QStringLiteral("kind"), QStringLiteral("until"));
    end.insert(QStringLiteral("until_date"), configuration.end.untilDate.value_or(QString()));
    break;
  case TaskRecurrenceEndKind::Count:
    end.insert(QStringLiteral("kind"), QStringLiteral("count"));
    end.insert(QStringLiteral("count"), configuration.end.count.value_or(0));
    break;
  }
  return {{QStringLiteral("frequency"), bridgeRecurrenceFrequency(configuration.frequency)},
          {QStringLiteral("interval"), configuration.interval},
          {QStringLiteral("end"), end},
          {QStringLiteral("recurrence_rule"), recurrenceRule},
          {QStringLiteral("exclusion_dates"), bridgeRecurrenceDates(exclusionDates)},
          {QStringLiteral("addition_dates"), bridgeRecurrenceDates(additionDates)}};
}

[[nodiscard]] std::optional<QList<QString>> recurrenceDatesFromText(const QString& value) {
  QList<QString> dates;
  QSet<QString> seen;
  for (const QString& token : value.split(u',', Qt::SkipEmptyParts)) {
    const QDate date = QDate::fromString(token.trimmed(), Qt::ISODate);
    const QString normalized = date.toString(Qt::ISODate);
    if (!date.isValid() || seen.contains(normalized)) {
      return std::nullopt;
    }
    seen.insert(normalized);
    dates.append(normalized);
  }
  return dates;
}

[[nodiscard]] std::optional<QList<QString>> taskIdsFromVariantList(const QVariantList& values) {
  QList<QString> taskIds;
  taskIds.reserve(values.size());
  for (const QVariant& value : values) {
    if (value.metaType().id() != QMetaType::QString) {
      return std::nullopt;
    }
    taskIds.append(value.toString());
  }
  return taskIds;
}

[[nodiscard]] std::optional<QList<QString>>
eventAttendeesFromVariantList(const QVariantList& values) {
  QList<QString> attendees;
  attendees.reserve(values.size());
  for (const QVariant& value : values) {
    if (value.metaType().id() != QMetaType::QString) {
      return std::nullopt;
    }
    attendees.append(value.toString());
  }
  return attendees;
}

[[nodiscard]] std::optional<CalendarEventReminderSettings>
eventRemindersFromVariantList(bool useDefault, const QVariantList& values) {
  CalendarEventReminderSettings settings{.useDefault = useDefault};
  settings.overrides.reserve(values.size());
  for (const QVariant& value : values) {
    if (!value.canConvert<QVariantMap>()) {
      return std::nullopt;
    }
    const QVariantMap reminder = value.toMap();
    const QVariant method = reminder.value(QStringLiteral("method"));
    const QVariant minutes = reminder.value(QStringLiteral("minutes"));
    if (method.metaType().id() != QMetaType::QString || !minutes.canConvert<int>()) {
      return std::nullopt;
    }
    settings.overrides.append({.method = method.toString(), .minutes = minutes.toInt()});
  }
  return settings;
}

[[nodiscard]] std::optional<QString> normalizedDueAt(QString value) {
  value = value.trimmed();
  if (value.isEmpty()) {
    return std::nullopt;
  }
  const QDate date = QDate::fromString(value, Qt::ISODate);
  if (date.isValid()) {
    return QDateTime(date, QTime(0, 0), QTimeZone::UTC).toString(Qt::ISODateWithMs);
  }
  const QDateTime dateTime = QDateTime::fromString(value, Qt::ISODate);
  return dateTime.isValid() ? std::optional<QString>(dateTime.toUTC().toString(Qt::ISODateWithMs))
                            : std::nullopt;
}

[[nodiscard]] std::optional<QString> bridgeDueDate(const std::optional<QString>& due) {
  if (!due.has_value()) {
    return std::optional<QString>{};
  }
  const QDateTime dateTime = QDateTime::fromString(*due, Qt::ISODate);
  if (!dateTime.isValid()) {
    return std::nullopt;
  }
  return dateTime.date().toString(Qt::ISODate);
}

[[nodiscard]] std::optional<QJsonObject>
bridgeEventTime(const QString& value, bool allDay, const QString& timeZone) {
  if (allDay) {
    const QDate date = QDate::fromString(value, Qt::ISODate);
    if (!date.isValid()) {
      return std::nullopt;
    }
    return QJsonObject{{QStringLiteral("kind"), QStringLiteral("date")},
                       {QStringLiteral("value"), date.toString(Qt::ISODate)}};
  }
  const QDateTime dateTime = QDateTime::fromString(value, Qt::ISODate);
  if (!dateTime.isValid() || dateTime.timeSpec() == Qt::LocalTime) {
    return std::nullopt;
  }
  QJsonObject result{{QStringLiteral("kind"), QStringLiteral("dateTime")},
                     {QStringLiteral("value"), dateTime.toUTC().toString(Qt::ISODateWithMs)}};
  if (!timeZone.trimmed().isEmpty()) {
    result.insert(QStringLiteral("time_zone"), timeZone.trimmed());
  }
  return result;
}

[[nodiscard]] QString bulkTaskSummaryMessage(const TaskBulkMutationSummary& summary) {
  return QStringLiteral("%1 selected · %2 eligible · applied %3 · queued %4 · conflicted %5 · "
                        "failed %6 · skipped %7. Remote sync pending.")
      .arg(summary.requested)
      .arg(summary.eligible)
      .arg(summary.applied)
      .arg(summary.queued)
      .arg(summary.conflicted)
      .arg(summary.failed)
      .arg(summary.skipped);
}

[[nodiscard]] QString bulkEventSummaryMessage(const CalendarEventBulkMutationSummary& summary) {
  return QStringLiteral("%1 selected · %2 eligible · applied %3 · queued %4 · conflicted %5 · "
                        "failed %6 · skipped %7. Remote sync pending.")
      .arg(summary.requested)
      .arg(summary.eligible)
      .arg(summary.applied)
      .arg(summary.queued)
      .arg(summary.conflicted)
      .arg(summary.failed)
      .arg(summary.skipped);
}

[[nodiscard]] std::optional<SyncConflictPolicy> conflictPolicyForValue(int value) {
  switch (value) {
  case static_cast<int>(SyncConflictPolicy::PreferGoogle):
    return SyncConflictPolicy::PreferGoogle;
  case static_cast<int>(SyncConflictPolicy::PreferHcb):
    return SyncConflictPolicy::PreferHcb;
  case static_cast<int>(SyncConflictPolicy::AskEachTime):
    return SyncConflictPolicy::AskEachTime;
  default:
    return std::nullopt;
  }
}

[[nodiscard]] bool isValidNotesProjectionMode(int value) {
  return value == kNotesOnlyProjection || value == kMirrorNotesProjection;
}

[[nodiscard]] bool isUndatedTask(const TaskModelTask& task) {
  return !task.due.has_value() || !task.due->at.has_value();
}

[[nodiscard]] QList<TaskModelTask> taskPresentation(const QList<TaskModelTask>& tasks,
                                                    bool notesOnly) {
  if (!notesOnly) {
    return tasks;
  }
  QList<TaskModelTask> visible;
  visible.reserve(tasks.size());
  QSet<QString> visibleIds;
  for (const TaskModelTask& task : tasks) {
    if (!isUndatedTask(task)) {
      visible.append(task);
      visibleIds.insert(task.id);
    }
  }
  for (TaskModelTask& task : visible) {
    if (task.parentTaskId.has_value() && !visibleIds.contains(*task.parentTaskId)) {
      task.parentTaskId.reset();
    }
  }
  return visible;
}

[[nodiscard]] QList<LocalSearchRankedResult>
searchPresentation(QList<LocalSearchRankedResult> results, bool notesEnabled, int notesMode) {
  results.erase(std::remove_if(results.begin(),
                               results.end(),
                               [notesEnabled, notesMode](const LocalSearchRankedResult& result) {
                                 if (result.resource == LocalSearchResource::Note) {
                                   return !notesEnabled;
                                 }
                                 return notesEnabled && notesMode == kNotesOnlyProjection &&
                                        result.resource == LocalSearchResource::Task &&
                                        result.isUndatedTask;
                               }),
                results.end());
  return results;
}

[[nodiscard]] QString conflictResourceText(SyncConflictResource resource) {
  switch (resource) {
  case SyncConflictResource::Task:
    return QStringLiteral("Task");
  case SyncConflictResource::TaskList:
    return QStringLiteral("Task list");
  case SyncConflictResource::Event:
    return QStringLiteral("Calendar event");
  }
  return QStringLiteral("Google resource");
}

[[nodiscard]] QVariantList conflictRows(QList<SyncConflict> conflicts) {
  QVariantList rows;
  rows.reserve(conflicts.size());
  for (const SyncConflict& conflict : conflicts) {
    QVariantMap row;
    row.insert(QStringLiteral("id"), conflict.id);
    row.insert(QStringLiteral("resource"), conflictResourceText(conflict.resource));
    row.insert(QStringLiteral("message"), conflict.errorMessage);
    row.insert(QStringLiteral("canKeepLocal"),
               conflict.remoteEtag.has_value() &&
                   !conflict.remoteSnapshot.value(QStringLiteral("_deleted")).toBool());
    row.insert(QStringLiteral("canKeepRemote"), true);
    row.insert(QStringLiteral("resolution"),
               !conflict.resolution.has_value() ? QString()
               : *conflict.resolution == SyncConflictResolution::KeepLocal
                   ? QStringLiteral("Kept HCB")
                   : QStringLiteral("Kept Google"));
    row.insert(QStringLiteral("resolvedAt"), conflict.resolvedAt.value_or(QString()));
    rows.append(std::move(row));
  }
  return rows;
}

[[nodiscard]] std::optional<QVariantMap> bridgeConflictRow(const QJsonObject& conflict) {
  const QJsonValue idValue = conflict.value(QStringLiteral("id"));
  const QJsonValue resourceValue = conflict.value(QStringLiteral("resource"));
  const QJsonValue statusValue = conflict.value(QStringLiteral("status"));
  const QJsonValue messageValue = conflict.value(QStringLiteral("message"));
  const QJsonValue keepLocalValue = conflict.value(QStringLiteral("can_keep_local"));
  const QJsonValue keepRemoteValue = conflict.value(QStringLiteral("can_keep_remote"));
  const QJsonValue resolvedAtValue = conflict.value(QStringLiteral("resolved_at"));
  if (!idValue.isString() || !resourceValue.isString() || !statusValue.isString() ||
      !messageValue.isString() || !keepLocalValue.isBool() || !keepRemoteValue.isBool() ||
      (!resolvedAtValue.isNull() && !resolvedAtValue.isString())) {
    return std::nullopt;
  }
  const QString id = idValue.toString();
  const QString resource = resourceValue.toString();
  const QString status = statusValue.toString();
  const QString message = messageValue.toString();
  bool parsed = false;
  const qlonglong parsedId = id.toLongLong(&parsed);
  if (!parsed || parsedId < 1 || QString::number(parsedId) != id || resource.isEmpty() ||
      resource.size() > 64 || message.isEmpty() || message.size() > 512 ||
      (status != QStringLiteral("open") && status != QStringLiteral("keep_local") &&
       status != QStringLiteral("keep_remote"))) {
    return std::nullopt;
  }
  const QString resolvedAt = resolvedAtValue.isString() ? resolvedAtValue.toString() : QString();
  if (resolvedAt.size() > 64 || (status == QStringLiteral("open") && !resolvedAt.isEmpty())) {
    return std::nullopt;
  }
  const QString resolution = status == QStringLiteral("keep_local") ? QStringLiteral("Kept HCB")
                             : status == QStringLiteral("keep_remote")
                                 ? QStringLiteral("Kept Google")
                                 : QString();
  return QVariantMap{{QStringLiteral("id"), id},
                     {QStringLiteral("resource"), resource},
                     {QStringLiteral("message"), message},
                     {QStringLiteral("canKeepLocal"), keepLocalValue.toBool()},
                     {QStringLiteral("canKeepRemote"), keepRemoteValue.toBool()},
                     {QStringLiteral("resolution"), resolution},
                     {QStringLiteral("resolvedAt"), resolvedAt}};
}

[[nodiscard]] std::optional<QVariantList> bridgeConflictRows(const QJsonObject& data) {
  const QJsonValue conflictsValue = data.value(QStringLiteral("conflicts"));
  if (!conflictsValue.isArray() || conflictsValue.toArray().size() > 200) {
    return std::nullopt;
  }
  QVariantList rows;
  rows.reserve(conflictsValue.toArray().size());
  QSet<QString> ids;
  for (const QJsonValue& value : conflictsValue.toArray()) {
    if (!value.isObject()) {
      return std::nullopt;
    }
    const std::optional<QVariantMap> row = bridgeConflictRow(value.toObject());
    if (!row.has_value() || ids.contains(row->value(QStringLiteral("id")).toString())) {
      return std::nullopt;
    }
    ids.insert(row->value(QStringLiteral("id")).toString());
    rows.append(*row);
  }
  return rows;
}

[[nodiscard]] std::optional<SavedSearch> bridgeSavedSearch(const QJsonObject& search) {
  const QJsonValue idValue = search.value(QStringLiteral("id"));
  const QJsonValue nameValue = search.value(QStringLiteral("name"));
  const QJsonValue queryValue = search.value(QStringLiteral("query"));
  const QJsonValue createdAtValue = search.value(QStringLiteral("created_at"));
  if (!idValue.isString() || !nameValue.isString() || !queryValue.isString() ||
      !createdAtValue.isString()) {
    return std::nullopt;
  }
  const QString id = idValue.toString();
  const QString name = nameValue.toString();
  const QString query = queryValue.toString();
  const QString createdAt = createdAtValue.toString();
  if (id.isEmpty() || id != id.trimmed() || id.size() > 128 || id.contains(QChar::Null) ||
      id.contains(u'/') || id.contains(u'\\') || name.isEmpty() || name != name.trimmed() ||
      name.size() > 128 || name.contains(QChar::Null) || query.isEmpty() ||
      query != query.trimmed() || query.size() > 4 * 1024 || query.contains(QChar::Null) ||
      createdAt.isEmpty() || createdAt.size() > 64) {
    return std::nullopt;
  }
  return SavedSearch{.id = id, .name = name, .query = query};
}

[[nodiscard]] std::optional<QList<SavedSearch>> bridgeSavedSearches(const QJsonObject& data) {
  const QJsonValue searchesValue = data.value(QStringLiteral("saved_searches"));
  if (!searchesValue.isArray() || searchesValue.toArray().size() > 100) {
    return std::nullopt;
  }
  QList<SavedSearch> searches;
  searches.reserve(searchesValue.toArray().size());
  QSet<QString> ids;
  for (const QJsonValue& value : searchesValue.toArray()) {
    if (!value.isObject()) {
      return std::nullopt;
    }
    const std::optional<SavedSearch> search = bridgeSavedSearch(value.toObject());
    if (!search.has_value() || ids.contains(search->id)) {
      return std::nullopt;
    }
    ids.insert(search->id);
    searches.append(*search);
  }
  return searches;
}

[[nodiscard]] QVariantList savedSearchRows(const QList<SavedSearch>& searches) {
  QVariantList rows;
  rows.reserve(searches.size());
  for (const SavedSearch& search : searches) {
    rows.append(QVariantMap{{QStringLiteral("id"), search.id},
                            {QStringLiteral("name"), search.name},
                            {QStringLiteral("query"), search.query}});
  }
  return rows;
}

[[nodiscard]] QDate weekStart(const QDate& date, int firstDay) {
  const int firstQtDay = firstDay == 0 ? 7 : 1;
  return date.addDays(-(date.dayOfWeek() - firstQtDay + 7) % 7);
}

[[nodiscard]] QDate monthGridStart(const QDate& date, int firstDayValue) {
  const QDate firstDay(date.year(), date.month(), 1);
  const int firstQtDay = firstDayValue == 0 ? 7 : 1;
  return firstDay.addDays(-(firstDay.dayOfWeek() - firstQtDay + 7) % 7);
}

[[nodiscard]] QString calendarRangeStart(const QDate& date, int firstDay) {
  return QDateTime(monthGridStart(date, firstDay), QTime(0, 0), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] QString calendarRangeEnd(const QDate& date, int firstDay) {
  return QDateTime(monthGridStart(date, firstDay).addDays(42), QTime(0, 0), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

struct CalendarViewLayouts final {
  QList<CalendarEventSummary> agendaEvents;
  TimelineModel::Layout timeline;
  MonthGridModel::Layout month;
};

[[nodiscard]] QList<CalendarEventSummary> calendarPresentation(QList<CalendarEventSummary> events) {
  QList<CalendarEventSummary> presentation;
  for (CalendarEventSummary& event : events) {
    if (event.recurringRemoteId.has_value() && event.originalStartAt.has_value()) {
      if (event.status != QStringLiteral("cancelled")) {
        presentation.append(std::move(event));
      }
    } else if (event.recurrenceRule.has_value()) {
    } else if (event.status != QStringLiteral("cancelled")) {
      presentation.append(std::move(event));
    }
  }
  std::sort(presentation.begin(),
            presentation.end(),
            [](const CalendarEventSummary& left, const CalendarEventSummary& right) {
              return left.startAt == right.startAt ? left.id < right.id
                                                   : left.startAt < right.startAt;
            });
  return presentation;
}

[[nodiscard]] CalendarViewLayouts buildCalendarViewLayouts(QDate date,
                                                           QList<CalendarEventSummary> events,
                                                           const QTimeZone& displayTimeZone,
                                                           int firstDay) {
  events = calendarPresentation(std::move(events));
  return {.agendaEvents = events,
          .timeline = TimelineModel::buildLayout(weekStart(date, firstDay),
                                                 kCalendarTimelineDays,
                                                 events,
                                                 displayTimeZone,
                                                 kVisibleAllDayLaneCount),
          .month = MonthGridModel::buildLayout(date, events, displayTimeZone, firstDay)};
}

[[nodiscard]] QJsonObject taskDueSnapshot(const TaskMutationSnapshot& task) {
  return {{QStringLiteral("taskId"), task.taskId},
          {QStringLiteral("dueAt"),
           task.dueAt.has_value() ? QJsonValue(*task.dueAt) : QJsonValue(QJsonValue::Null)},
          {QStringLiteral("dueTimeZone"),
           task.dueTimeZone.has_value() ? QJsonValue(*task.dueTimeZone)
                                        : QJsonValue(QJsonValue::Null)}};
}

[[nodiscard]] QJsonObject existenceSnapshot(bool exists) {
  return {{QStringLiteral("exists"), exists}};
}

[[nodiscard]] std::optional<bool> existenceFromSnapshot(const QJsonValue& value) {
  if (!value.isObject()) {
    return std::nullopt;
  }
  const QJsonValue exists = value.toObject().value(QStringLiteral("exists"));
  return exists.isBool() ? std::optional<bool>(exists.toBool()) : std::nullopt;
}

[[nodiscard]] std::optional<TaskDue> taskDueFromSnapshot(const QJsonValue& snapshot,
                                                         const QString& taskId) {
  if (!snapshot.isObject()) {
    return std::nullopt;
  }
  const QJsonObject object = snapshot.toObject();
  if (object.value(QStringLiteral("taskId")).toString() != taskId) {
    return std::nullopt;
  }
  const QJsonValue dueAt = object.value(QStringLiteral("dueAt"));
  const QJsonValue dueTimeZone = object.value(QStringLiteral("dueTimeZone"));
  if ((!dueAt.isNull() && !dueAt.isString()) ||
      (!dueTimeZone.isNull() && !dueTimeZone.isString())) {
    return std::nullopt;
  }
  return TaskDue{.at = dueAt.isNull() ? std::optional<QString>{}
                                      : std::optional<QString>(dueAt.toString()),
                 .timeZone = dueTimeZone.isNull() ? std::optional<QString>{}
                                                  : std::optional<QString>(dueTimeZone.toString())};
}

[[nodiscard]] QJsonObject eventTimingSnapshot(const CalendarEventMutationSnapshot& event) {
  return {{QStringLiteral("eventId"), event.eventId},
          {QStringLiteral("startAt"), event.startAt},
          {QStringLiteral("endAt"), event.endAt},
          {QStringLiteral("allDay"), event.allDay}};
}

struct EventTiming final {
  QString startAt;
  QString endAt;
  bool allDay{false};
};

[[nodiscard]] std::optional<EventTiming> eventTimingFromSnapshot(const QJsonValue& snapshot,
                                                                 const QString& eventId) {
  if (!snapshot.isObject()) {
    return std::nullopt;
  }
  const QJsonObject object = snapshot.toObject();
  const QJsonValue allDay = object.value(QStringLiteral("allDay"));
  const QString startAt = object.value(QStringLiteral("startAt")).toString();
  const QString endAt = object.value(QStringLiteral("endAt")).toString();
  if (object.value(QStringLiteral("eventId")).toString() != eventId || startAt.isEmpty() ||
      endAt.isEmpty() || !allDay.isBool()) {
    return std::nullopt;
  }
  return EventTiming{.startAt = startAt, .endAt = endAt, .allDay = allDay.toBool()};
}

} // namespace

AppController::AppController(FilePath databasePath,
                             Clock& clock,
                             AgendaModel& agendaModel,
                             CalendarSourceModel& calendarSourceModel,
                             MonthGridModel& monthGridModel,
                             NotesModel& notesModel,
                             TaskListModel& taskListModel,
                             TaskModel& taskModel,
                             TimelineModel& timelineModel,
                             std::optional<PythonBridgeConnection> bridgeConnection,
                             QString bridgeAccountId,
                             QObject* parent)
    : QObject(parent), clock_(clock), agendaModel_(agendaModel),
      calendarSourceModel_(calendarSourceModel), monthGridModel_(monthGridModel),
      notesModel_(notesModel), taskListModel_(taskListModel), taskModel_(taskModel),
      timelineModel_(timelineModel), scheduledTaskDateIndex_(this),
      oauthConfigurationStore_(databasePath, clock), accountStatusService_(databasePath, clock),
      credentialStore_(makeCredentialStore()), oauthLoopbackListener_(this),
      oauthTokenExchangeClient_(this), oauthTokenRefreshClient_(this), pkceStateRegistry_(clock),
      googleHttpClient_(this), googleTaskListPullClient_(googleHttpClient_),
      googleTaskPullClient_(googleHttpClient_), googleCalendarListPullClient_(googleHttpClient_),
      googleCalendarManagementClient_(googleHttpClient_),
      googleCalendarFreeBusyClient_(googleHttpClient_),
      googleDriveFilePickerClient_(googleHttpClient_),
      googleCalendarEventPullClient_(googleHttpClient_), googleMirrorStore_(databasePath, clock),
      settingsService_(databasePath, clock), savedSearchStore_(settingsService_),
      optimisticMutationCoordinator_(databasePath, clock),
      mutationTelemetryStore_(databasePath, clock),
      undoRecoveryPolicy_(databasePath, clock, QStringLiteral("undo:google")),
      syncCheckpointStore_(databasePath, clock), syncConflictStore_(databasePath, clock),
      googleSyncConflictResolver_(
          optimisticMutationCoordinator_, syncConflictStore_, googleHttpClient_),
      taskMutationService_(databasePath, clock), taskBulkMutationService_(taskMutationService_),
      taskListMutationService_(databasePath, clock), calendarMutationService_(databasePath, clock),
      importMutationService_(databasePath, clock),
      calendarEventBulkMutationService_(calendarMutationService_),
      googleSyncRecoveryService_(syncCheckpointStore_),
      googleTaskMutationPushService_(optimisticMutationCoordinator_,
                                     googleHttpClient_,
                                     clock,
                                     SyncBackoffPolicy(),
                                     &taskMutationService_,
                                     &taskListMutationService_,
                                     &googleSyncConflictResolver_,
                                     &mutationTelemetryStore_),
      googleCalendarEventMutationPushService_(optimisticMutationCoordinator_,
                                              googleHttpClient_,
                                              clock,
                                              SyncBackoffPolicy(),
                                              &calendarMutationService_,
                                              &googleSyncConflictResolver_,
                                              &mutationTelemetryStore_),
      taskListReadService_(databasePath), taskReadService_(databasePath),
      calendarReadService_(databasePath),
      googleCalendarInstanceCacheService_(
          googleCalendarEventPullClient_, calendarReadService_, googleMirrorStore_),
      localSearchService_(databasePath), googleTaskMirrorSyncService_(googleTaskListPullClient_,
                                                                      googleTaskPullClient_,
                                                                      googleMirrorStore_,
                                                                      syncCheckpointStore_,
                                                                      clock,
                                                                      &taskMutationService_),
      googleCalendarMirrorSyncService_(googleCalendarListPullClient_,
                                       googleCalendarEventPullClient_,
                                       calendarReadService_,
                                       googleMirrorStore_,
                                       syncCheckpointStore_,
                                       googleSyncRecoveryService_),
      syncScheduler_([this](const SyncSchedulerRequest& request) { return runGoogleSync(request); },
                     clock) {
  searchResultsModelPointer_ = new SearchResultsModel(this);
  searchDebounce_.setSingleShot(true);
  searchDebounce_.setInterval(kSearchDebounceMilliseconds);
  connect(&searchDebounce_, &QTimer::timeout, this, &AppController::runSearch);
  connect(&oauthLoopbackListener_,
          &OAuthLoopbackCallbackListener::callbackReceived,
          this,
          &AppController::handleOAuthCallback);
  if (bridgeConnection.has_value()) {
    pythonBridgeClient_ = std::make_unique<PythonBridgeClient>(std::move(*bridgeConnection), this);
    pythonBridgeAccountId_ = std::move(bridgeAccountId);
  }
}

AppController::~AppController() {
  if (searchCancellation_ != nullptr) {
    static_cast<void>(searchCancellation_->requestStop());
  }
  syncScheduler_.stop();
}

QString AppController::clientId() const { return clientId_; }

bool AppController::hasClientSecret() const { return !clientSecret_.isEmpty(); }

bool AppController::googleConnected() const { return googleConnected_; }

QString AppController::statusMessage() const { return statusMessage_; }

QString AppController::taskListErrorMessage() const { return taskListErrorMessage_; }

QString AppController::syncStatus() const { return syncStatus_; }

int AppController::conflictPolicy() const { return conflictPolicy_; }

QVariantList AppController::unresolvedConflicts() const { return unresolvedConflicts_; }

QVariantList AppController::resolvedConflicts() const { return resolvedConflicts_; }

QString AppController::searchQuery() const { return searchQuery_; }

QString AppController::searchErrorMessage() const { return searchErrorMessage_; }

QVariantList AppController::searchFilterChips() const { return searchFilterChips_; }

QVariantList AppController::savedSearches() const { return savedSearchRows_; }

bool AppController::searchLoading() const { return searchLoading_; }

QString AppController::bulkTaskStatusMessage() const { return bulkTaskStatusMessage_; }

QString AppController::bulkEventStatusMessage() const { return bulkEventStatusMessage_; }

QString AppController::bulkTaskPreviewMessage() const { return bulkTaskPreviewMessage_; }

QString AppController::bulkEventPreviewMessage() const { return bulkEventPreviewMessage_; }

int AppController::bulkTaskPreviewRequestToken() const { return bulkTaskPreviewRequestToken_; }

int AppController::bulkEventPreviewRequestToken() const { return bulkEventPreviewRequestToken_; }

int AppController::bulkTextRecurrenceScope() const { return bulkTextRecurrenceScope_; }

QString AppController::calendarDate() const { return calendarDate_.toString(Qt::ISODate); }

QString AppController::calendarDateLabel() const {
  return localizedCalendarDate(calendarDate_, QStringLiteral("d MMM yyyy"));
}

QString AppController::calendarMonthLabel() const {
  return calendarDate_.isValid() ? QLocale::c().toString(calendarDate_, QStringLiteral("MMMM yyyy"))
                                 : QString();
}

QString AppController::calendarDayHeading() const {
  return localizedCalendarDate(calendarDate_, QStringLiteral("dddd, d MMMM"));
}

QVariantList AppController::calendarWeekLabels() const {
  QVariantList labels;
  if (!calendarDate_.isValid()) {
    return labels;
  }
  const int firstQtDay = weekStartDay_ == 0 ? 7 : 1;
  const QDate start = calendarDate_.addDays(-(calendarDate_.dayOfWeek() - firstQtDay + 7) % 7);
  labels.reserve(7);
  for (int offset = 0; offset < 7; ++offset) {
    labels.append(localizedCalendarDate(start.addDays(offset), QStringLiteral("ddd d MMM")));
  }
  return labels;
}

int AppController::appearanceMode() const { return appearanceMode_; }

int AppController::visualDensity() const { return visualDensity_; }

int AppController::taskListPaneWidth() const { return taskListPaneWidth_; }

int AppController::paletteMode() const { return paletteMode_; }

QString AppController::accentColor() const { return accentColor_; }

QString AppController::fontFamily() const { return fontFamily_; }

QVariantList AppController::availableFontFamilies() const {
  QVariantList families;
  const QStringList installed = installedFontFamilies();
  families.reserve(installed.size());
  for (const QString& family : installed) {
    families.append(family);
  }
  return families;
}

int AppController::fontScale() const { return fontScale_; }

QString AppController::externalBrowser() const { return externalBrowser_; }

QString AppController::quickCaptureDefaultTaskListId() const {
  return quickCaptureDefaultTaskListId_;
}

QString AppController::quickCaptureDefaultCalendarId() const {
  return quickCaptureDefaultCalendarId_;
}

int AppController::quickCaptureEventDurationMinutes() const {
  return quickCaptureEventDurationMinutes_;
}

bool AppController::quickCaptureRemoveParsedText() const { return quickCaptureRemoveParsedText_; }

QString AppController::quickCaptureTaskAliases() const {
  return quickCaptureAliasesToText(quickCaptureTaskAliases_);
}

QString AppController::quickCaptureEventAliases() const {
  return quickCaptureAliasesToText(quickCaptureEventAliases_);
}

QString AppController::quickCaptureHighPriorityAliases() const {
  return quickCaptureAliasesToText(quickCaptureHighPriorityAliases_);
}

QString AppController::quickCaptureMediumPriorityAliases() const {
  return quickCaptureAliasesToText(quickCaptureMediumPriorityAliases_);
}

QString AppController::quickCaptureLowPriorityAliases() const {
  return quickCaptureAliasesToText(quickCaptureLowPriorityAliases_);
}

int AppController::weekStartDay() const { return weekStartDay_; }

bool AppController::use24HourTime() const { return use24HourTime_; }

QString AppController::displayTimeZone() const { return displayTimeZone_; }

QVariantList AppController::availableTimeZones() const {
  QVariantList zones;
  const QList<QByteArray> identifiers = QTimeZone::availableTimeZoneIds();
  zones.reserve(identifiers.size() + 1);
  zones.append(QString());
  for (const QByteArray& identifier : identifiers) {
    zones.append(QString::fromUtf8(identifier));
  }
  return zones;
}

int AppController::workdayStartHour() const { return workdayStartHour_; }

int AppController::workdayEndHour() const { return workdayEndHour_; }

QVariantList AppController::visibleCalendarIds() const { return visibleCalendarIds_; }

bool AppController::calendarVisibilityConfigured() const { return calendarVisibilityConfigured_; }

QVariantList AppController::calendarManagementRows() const { return calendarManagementRows_; }

QVariantList AppController::importPreviewRows() const { return importPreviewRows_; }

QString AppController::importSourceName() const { return importSourceName_; }

bool AppController::importReadyToCommit() const { return importReadyToCommit_; }

bool AppController::notesEnabled() const { return notesEnabled_; }

int AppController::notesProjectionMode() const { return notesProjectionMode_; }

QVariantList AppController::sidebarTabIds() const {
  QVariantList ids;
  ids.reserve(sidebarTabIds_.size());
  for (const QString& id : sidebarTabIds_) {
    ids.append(id);
  }
  return ids;
}

QVariantList AppController::freeBusyIntervals() const { return freeBusyIntervals_; }

QVariantList AppController::driveAttachmentCandidates() const { return driveAttachmentCandidates_; }

QVariantList AppController::invitations() const { return invitations_; }

int AppController::pendingInvitationCount() const { return static_cast<int>(invitations_.size()); }

QVariantList AppController::scheduledTasks() const { return scheduledTaskRows_; }

QObject* AppController::scheduledTaskDateIndex() { return &scheduledTaskDateIndex_; }

namespace {

[[nodiscard]] QVariantList scheduledTaskRows(const QList<TaskModelTask>& tasks) {
  QVariantList result;
  for (const TaskModelTask& task : tasks) {
    if (task.completed || !task.due.has_value() || !task.due->at.has_value()) {
      continue;
    }
    const QString dueAt = *task.due->at;
    if (dueAt.size() < 10) {
      continue;
    }
    result.append(
        QVariantMap{{QStringLiteral("id"), task.id},
                    {QStringLiteral("taskListId"), task.taskListId},
                    {QStringLiteral("taskListTitle"), task.taskListTitle},
                    {QStringLiteral("title"), task.title},
                    {QStringLiteral("notes"), task.notes.value_or(QString())},
                    {QStringLiteral("dueAt"), dueAt},
                    {QStringLiteral("dueTimeZone"), task.due->timeZone.value_or(QString())},
                    {QStringLiteral("priority"), static_cast<int>(task.priority)},
                    {QStringLiteral("managedRecurrence"), task.managedRecurrence},
                    {QStringLiteral("recurrenceSummary"), task.recurrenceSummary}});
  }
  return result;
}

} // namespace

QVariantList AppController::unscheduledTasks() const {
  QVariantList result;
  for (const TaskModelTask& task : taskProjectionTasks_) {
    if (task.completed || task.parentTaskId.has_value() || task.due.has_value()) {
      continue;
    }
    result.append(QVariantMap{{QStringLiteral("id"), task.id},
                              {QStringLiteral("taskListId"), task.taskListId},
                              {QStringLiteral("taskListTitle"), task.taskListTitle},
                              {QStringLiteral("title"), task.title},
                              {QStringLiteral("notes"), task.notes.value_or(QString())},
                              {QStringLiteral("priority"), static_cast<int>(task.priority)}});
  }
  return result;
}

QString AppController::undoLabel() const { return undoLabel_; }

QString AppController::redoLabel() const { return redoLabel_; }

int AppController::undoRetentionDays() const { return undoRetentionDays_; }

int AppController::undoMaximumEntries() const { return undoMaximumEntries_; }

bool AppController::calendarDragCreateHintSeen() const { return calendarDragCreateHintSeen_; }

int AppController::pendingSyncCount() const { return pendingSyncCount_; }

QString AppController::reminderStatusMessage() const { return reminderStatusMessage_; }

bool AppController::busy() const { return busy_; }

bool AppController::bridgeMode() const { return pythonBridgeClient_ != nullptr; }

bool AppController::bridgeOperationActive() const {
  return pythonBridgeOperationStarting_ || !pythonBridgeOperationId_.isEmpty();
}

QString AppController::bridgeOperationKind() const { return pythonBridgeOperationKind_; }

SearchResultsModel& AppController::searchResultsModel() { return *searchResultsModelPointer_; }

void AppController::initialize() {
  if (bridgeMode()) {
    initializeBridge();
    return;
  }
  loadSavedSearches();
  watch(
      undoRecoveryPolicy_.recover(),
      [this](UndoRecoveryResult result) {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(result)));
          return;
        }
        refreshUndoStatus();
      },
      false);
  const auto loadUndoSetting = [this](const char* key, bool retention) {
    watch(
        settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(key)),
        [this, retention](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          bool parsed = false;
          const int value = stored->toInt(&parsed);
          const bool valid =
              retention ? value >= 1 && value <= 3'650 : value >= 50 && value <= 1'000;
          if (!parsed || !valid) {
            setStatus(QStringLiteral("Stored undo history setting is invalid"));
            return;
          }
          if (retention) {
            undoRetentionDays_ = value;
          } else {
            undoMaximumEntries_ = value;
          }
          undoRecoveryPolicy_.configure(
              {.retentionDays = undoRetentionDays_, .maximumEntries = undoMaximumEntries_});
          emit undoHistorySettingsChanged();
        },
        false);
  };
  loadUndoSetting(kUndoRetentionDaysSettingsKey, true);
  loadUndoSetting(kUndoMaximumEntriesSettingsKey, false);
  refreshPendingSyncCount();
  watch(settingsService_.readJson(QString::fromLatin1(kSyncSettingsScope),
                                  QString::fromLatin1(kConflictPolicySettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          bool valid = false;
          const int storedValue = stored->toInt(&valid);
          const std::optional<SyncConflictPolicy> policy =
              valid ? conflictPolicyForValue(storedValue) : std::nullopt;
          if (!policy.has_value()) {
            setStatus(QStringLiteral("Stored sync conflict policy is invalid"));
            return;
          }
          conflictPolicy_ = storedValue;
          googleSyncConflictResolver_.setPolicy(*policy);
          emit conflictPolicyChanged();
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kNotesEnabledSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          const std::optional<bool> enabled =
              *stored == QStringLiteral("true")    ? std::optional<bool>(true)
              : *stored == QStringLiteral("false") ? std::optional<bool>(false)
                                                   : std::nullopt;
          if (!enabled.has_value()) {
            setStatus(QStringLiteral("Stored Notes setting is invalid"));
            return;
          }
          if (notesEnabled_ != *enabled) {
            notesEnabled_ = *enabled;
            applyTaskProjections(taskProjectionTasks_);
            refreshSearchProjection();
            emit notesEnabledChanged();
          }
          ensureNotesSidebarTab();
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kTaskListPaneWidthSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          bool valid = false;
          const int width = stored->toInt(&valid);
          if (!valid || !isValidTaskListPaneWidth(width)) {
            setStatus(QStringLiteral("Stored task Lists pane width is invalid"));
            return;
          }
          if (taskListPaneWidth_ != width) {
            taskListPaneWidth_ = width;
            emit taskListPaneWidthChanged();
          }
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kNotesProjectionSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          bool valid = false;
          const int mode = stored->toInt(&valid);
          if (!valid || !isValidNotesProjectionMode(mode)) {
            setStatus(QStringLiteral("Stored Notes projection is invalid"));
            return;
          }
          if (notesProjectionMode_ != mode) {
            notesProjectionMode_ = mode;
            applyTaskProjections(taskProjectionTasks_);
            refreshSearchProjection();
            emit notesProjectionModeChanged();
          }
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kSidebarTabIdsSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          const std::optional<QStringList> ids = sidebarTabIdsFromJson(*stored);
          if (!ids.has_value()) {
            setStatus(QStringLiteral("Stored sidebar tabs are invalid"));
            return;
          }
          if (sidebarTabIds_ != *ids) {
            sidebarTabIds_ = *ids;
            emit sidebarTabIdsChanged();
          }
          ensureNotesSidebarTab();
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kExternalBrowserSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          const std::optional<QString> browser = jsonArrayString(*stored);
          if (!browser.has_value() || !isValidExternalBrowser(*browser)) {
            setStatus(QStringLiteral("Stored external browser is invalid"));
            return;
          }
          if (externalBrowser_ != *browser) {
            externalBrowser_ = *browser;
            emit externalBrowserChanged();
          }
        });
  const auto loadPresentationInt =
      [this](const char* key, auto valid, auto apply, QString invalidMessage) {
        watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                        QString::fromLatin1(key)),
              [this, valid, apply, invalidMessage = std::move(invalidMessage)](
                  SettingsJsonReadResult result) mutable {
                if (std::holds_alternative<AppError>(result)) {
                  setStatus(errorMessage(std::get<AppError>(result)));
                  return;
                }
                const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
                if (!stored.has_value()) {
                  return;
                }
                bool parsed = false;
                const int value = stored->toInt(&parsed);
                if (!parsed || !valid(value)) {
                  setStatus(std::move(invalidMessage));
                  return;
                }
                apply(value);
              });
      };
  loadPresentationInt(
      kAppearanceModeSettingsKey,
      isValidAppearanceMode,
      [this](int value) {
        if (appearanceMode_ != value) {
          appearanceMode_ = value;
          emit appearanceModeChanged();
        }
      },
      QStringLiteral("Stored appearance mode is invalid"));
  loadPresentationInt(
      kVisualDensitySettingsKey,
      isValidVisualDensity,
      [this](int value) {
        if (visualDensity_ != value) {
          visualDensity_ = value;
          emit visualDensityChanged();
        }
      },
      QStringLiteral("Stored visual density is invalid"));
  loadPresentationInt(
      kPaletteModeSettingsKey,
      isValidPaletteMode,
      [this](int value) {
        if (paletteMode_ != value) {
          paletteMode_ = value;
          emit paletteModeChanged();
        }
      },
      QStringLiteral("Stored palette is invalid"));
  loadPresentationInt(
      kFontScaleSettingsKey,
      isValidFontScale,
      [this](int value) {
        if (fontScale_ != value) {
          fontScale_ = value;
          emit fontScaleChanged();
        }
      },
      QStringLiteral("Stored font scale is invalid"));
  loadPresentationInt(
      kBulkTextRecurrenceScopeSettingsKey,
      isValidBulkTextRecurrenceScope,
      [this](int value) {
        if (bulkTextRecurrenceScope_ != value) {
          bulkTextRecurrenceScope_ = value;
          emit bulkTextRecurrenceScopeChanged();
        }
      },
      QStringLiteral("Stored bulk text recurrence scope is invalid"));
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kAccentColorSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          const std::optional<QString> value = jsonArrayString(*stored);
          if (!value.has_value() || !isValidAccentColor(*value)) {
            setStatus(QStringLiteral("Stored accent color is invalid"));
            return;
          }
          const QString normalized = value->toUpper();
          if (accentColor_ != normalized) {
            accentColor_ = normalized;
            emit accentColorChanged();
          }
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kFontFamilySettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          const std::optional<QString> value = jsonArrayString(*stored);
          if (!value.has_value() ||
              (!value->isEmpty() &&
               (!installedFontFamilies().contains(*value) || !isUsableTextFontFamily(*value)))) {
            setStatus(QStringLiteral("Stored font family is unavailable"));
            return;
          }
          if (fontFamily_ != *value) {
            fontFamily_ = *value;
            emit fontFamilyChanged();
          }
        });
  const auto loadQuickCaptureString =
      [this](const char* key, auto valid, auto apply, QString message) {
        watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                        QString::fromLatin1(key)),
              [this, valid, apply, message = std::move(message)](
                  SettingsJsonReadResult result) mutable {
                if (std::holds_alternative<AppError>(result)) {
                  setStatus(errorMessage(std::get<AppError>(result)));
                  return;
                }
                const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
                if (!stored.has_value()) {
                  return;
                }
                const std::optional<QString> value = jsonArrayString(*stored);
                if (!value.has_value() || !valid(*value)) {
                  setStatus(std::move(message));
                  return;
                }
                apply(*value);
              });
      };
  loadQuickCaptureString(
      kQuickCaptureDefaultTaskListSettingsKey,
      isValidQuickCaptureDestination,
      [this](const QString& value) {
        if (quickCaptureDefaultTaskListId_ != value) {
          quickCaptureDefaultTaskListId_ = value;
          emit quickCaptureDefaultTaskListIdChanged();
        }
      },
      QStringLiteral("Stored quick capture task list is invalid"));
  loadQuickCaptureString(
      kQuickCaptureDefaultCalendarSettingsKey,
      isValidQuickCaptureDestination,
      [this](const QString& value) {
        if (quickCaptureDefaultCalendarId_ != value) {
          quickCaptureDefaultCalendarId_ = value;
          emit quickCaptureDefaultCalendarIdChanged();
        }
      },
      QStringLiteral("Stored quick capture calendar is invalid"));
  loadPresentationInt(
      kQuickCaptureEventDurationSettingsKey,
      isValidQuickCaptureDuration,
      [this](int value) {
        if (quickCaptureEventDurationMinutes_ != value) {
          quickCaptureEventDurationMinutes_ = value;
          emit quickCaptureEventDurationMinutesChanged();
        }
      },
      QStringLiteral("Stored quick capture event duration is invalid"));
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kQuickCaptureRemoveParsedTextSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          const std::optional<bool> value =
              *stored == QStringLiteral("true")    ? std::optional<bool>(true)
              : *stored == QStringLiteral("false") ? std::optional<bool>(false)
                                                   : std::nullopt;
          if (!value.has_value()) {
            setStatus(QStringLiteral("Stored quick capture parsed-text preference is invalid"));
          } else if (quickCaptureRemoveParsedText_ != *value) {
            quickCaptureRemoveParsedText_ = *value;
            emit quickCaptureRemoveParsedTextChanged();
          }
        });
  const auto loadQuickCaptureAliases = [this](const char* key, auto apply, QString message) {
    watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                    QString::fromLatin1(key)),
          [this, apply, message = std::move(message)](SettingsJsonReadResult result) mutable {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(result)));
              return;
            }
            const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
            if (!stored.has_value()) {
              return;
            }
            const std::optional<QStringList> values = jsonStringList(*stored);
            if (!values.has_value() ||
                std::any_of(values->cbegin(), values->cend(), [](const QString& value) {
                  return !isValidQuickCaptureAlias(value);
                })) {
              setStatus(std::move(message));
              return;
            }
            apply(*values);
          });
  };
  const auto aliasesChanged = [this](QStringList values, QStringList& destination) {
    if (destination != values) {
      destination = std::move(values);
      emit quickCaptureAliasesChanged();
    }
  };
  loadQuickCaptureAliases(
      kQuickCaptureTaskAliasesSettingsKey,
      [this, aliasesChanged](const QStringList& values) mutable {
        aliasesChanged(values, quickCaptureTaskAliases_);
      },
      QStringLiteral("Stored quick capture task aliases are invalid"));
  loadQuickCaptureAliases(
      kQuickCaptureEventAliasesSettingsKey,
      [this, aliasesChanged](const QStringList& values) mutable {
        aliasesChanged(values, quickCaptureEventAliases_);
      },
      QStringLiteral("Stored quick capture event aliases are invalid"));
  loadQuickCaptureAliases(
      kQuickCaptureHighPriorityAliasesSettingsKey,
      [this, aliasesChanged](const QStringList& values) mutable {
        aliasesChanged(values, quickCaptureHighPriorityAliases_);
      },
      QStringLiteral("Stored quick capture high-priority aliases are invalid"));
  loadQuickCaptureAliases(
      kQuickCaptureMediumPriorityAliasesSettingsKey,
      [this, aliasesChanged](const QStringList& values) mutable {
        aliasesChanged(values, quickCaptureMediumPriorityAliases_);
      },
      QStringLiteral("Stored quick capture medium-priority aliases are invalid"));
  loadQuickCaptureAliases(
      kQuickCaptureLowPriorityAliasesSettingsKey,
      [this, aliasesChanged](const QStringList& values) mutable {
        aliasesChanged(values, quickCaptureLowPriorityAliases_);
      },
      QStringLiteral("Stored quick capture low-priority aliases are invalid"));
  loadPresentationInt(
      kWeekStartDaySettingsKey,
      isValidWeekStartDay,
      [this](int value) {
        if (weekStartDay_ != value) {
          weekStartDay_ = value;
          emit weekStartDayChanged();
          emit calendarLabelsChanged();
          refreshCalendar();
        }
      },
      QStringLiteral("Stored week start is invalid"));
  loadPresentationInt(
      kWorkdayStartHourSettingsKey,
      [](int value) { return value >= 0 && value <= 23; },
      [this](int value) {
        if (isValidWorkdayHours(value, workdayEndHour_) && workdayStartHour_ != value) {
          workdayStartHour_ = value;
          emit workdayStartHourChanged();
        }
      },
      QStringLiteral("Stored workday start is invalid"));
  loadPresentationInt(
      kWorkdayEndHourSettingsKey,
      [](int value) { return value >= 1 && value <= 24; },
      [this](int value) {
        if (isValidWorkdayHours(workdayStartHour_, value) && workdayEndHour_ != value) {
          workdayEndHour_ = value;
          emit workdayEndHourChanged();
        }
      },
      QStringLiteral("Stored workday end is invalid"));
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kUse24HourTimeSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          const std::optional<bool> value =
              !stored.has_value()                  ? std::optional<bool>{}
              : *stored == QStringLiteral("true")  ? std::optional<bool>(true)
              : *stored == QStringLiteral("false") ? std::optional<bool>(false)
                                                   : std::nullopt;
          if (!value.has_value() && stored.has_value()) {
            setStatus(QStringLiteral("Stored time format is invalid"));
          } else if (value.has_value() && use24HourTime_ != *value) {
            use24HourTime_ = *value;
            emit use24HourTimeChanged();
          }
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kCalendarDragCreateHintSeenSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          if (*stored != QStringLiteral("true") && *stored != QStringLiteral("false")) {
            setStatus(QStringLiteral("Stored calendar drag hint setting is invalid"));
            return;
          }
          const bool seen = *stored == QStringLiteral("true");
          if (calendarDragCreateHintSeen_ != seen) {
            calendarDragCreateHintSeen_ = seen;
            emit calendarDragCreateHintSeenChanged();
          }
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kDisplayTimeZoneSettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          const std::optional<QString> value = jsonArrayString(*stored);
          if (!value.has_value() || !QTimeZone(value->toUtf8()).isValid()) {
            setStatus(QStringLiteral("Stored display time zone is invalid"));
            return;
          }
          if (displayTimeZone_ != *value) {
            displayTimeZone_ = *value;
            emit displayTimeZoneChanged();
            refreshCalendar();
          }
        });
  watch(settingsService_.readJson(QString::fromLatin1(kPresentationSettingsScope),
                                  QString::fromLatin1(kCalendarVisibilitySettingsKey)),
        [this](SettingsJsonReadResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          QVariantList values;
          const std::optional<QString>& stored = std::get<std::optional<QString>>(result);
          if (!stored.has_value()) {
            return;
          }
          QJsonParseError error;
          const QJsonDocument document = QJsonDocument::fromJson(stored->toUtf8(), &error);
          if (error.error != QJsonParseError::NoError || !document.isArray() ||
              document.array().size() > 20'000) {
            setStatus(QStringLiteral("Stored calendar visibility is invalid"));
            return;
          }
          QSet<QString> seen;
          for (const QJsonValue& item : document.array()) {
            if (!item.isString() || item.toString().isEmpty() || item.toString().size() > 256 ||
                item.toString() != item.toString().trimmed() || seen.contains(item.toString())) {
              setStatus(QStringLiteral("Stored calendar visibility is invalid"));
              return;
            }
            seen.insert(item.toString());
            values.append(item.toString());
          }
          if (visibleCalendarIds_ != values) {
            visibleCalendarIds_ = std::move(values);
            emit visibleCalendarIdsChanged();
          }
          if (!calendarVisibilityConfigured_) {
            calendarVisibilityConfigured_ = true;
            emit calendarVisibilityConfiguredChanged();
          }
        });
  watch(syncConflictStore_.listUnresolved(), [this](SyncConflictListResult result) {
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
      return;
    }
    setUnresolvedConflicts(std::get<QList<SyncConflict>>(std::move(result)));
  });
  watch(syncConflictStore_.listResolved(), [this](SyncConflictListResult result) {
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
      return;
    }
    setResolvedConflicts(std::get<QList<SyncConflict>>(std::move(result)));
  });
  watch(oauthConfigurationStore_.load(), [this](OAuthClientConfigurationReadResult result) {
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
    } else if (const std::optional<OAuthClientConfiguration>& configuration =
                   std::get<std::optional<OAuthClientConfiguration>>(result);
               configuration.has_value()) {
      clientId_ = configuration->clientId;
      const bool secretChanged = clientSecret_ != configuration->clientSecret;
      clientSecret_ = configuration->clientSecret;
      {
        std::lock_guard<std::mutex> lock(syncConfigurationMutex_);
        syncClientId_ = clientId_;
        syncClientSecret_ = clientSecret_;
      }
      emit clientIdChanged();
      if (secretChanged) {
        emit clientSecretChanged();
      }
      if (googleConnected_) {
        requestGoogleSync(SyncScheduleTrigger::Startup);
        startPeriodicGoogleSync();
      }
    }
  });
  watch(accountStatusService_.find(QString::fromLatin1(kGoogleAccountId)),
        [this](AccountStatusLookupResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const std::optional<AccountStatus>& account =
              std::get<std::optional<AccountStatus>>(result);
          if (account.has_value() &&
              account->connectionState == AccountConnectionState::Connected) {
            googleConnected_ = true;
            emit googleConnectedChanged();
            if (!clientId_.isEmpty()) {
              requestGoogleSync(SyncScheduleTrigger::Startup);
              startPeriodicGoogleSync();
            }
          }
        });
  refresh();
}

void AppController::initializeBridge() {
  if (pythonBridgeAccountId_.isEmpty()) {
    setStatus(QStringLiteral("An HCB bridge account is required"));
    return;
  }
  if (googleConnected_) {
    googleConnected_ = false;
    emit googleConnectedChanged();
  }
  setSyncStatus(QStringLiteral("managed by HCB core"));
  setReminderStatusMessage(QStringLiteral("Calendar reminders are managed by HCB core"));
  loadBridgeAuthentication();
  refreshBridge();
}

void AppController::loadBridgeAuthentication() {
  if (pythonBridgeClient_ == nullptr || pythonBridgeAccountId_.isEmpty()) {
    return;
  }
  watch(
      pythonBridgeClient_->authenticationState(pythonBridgeAccountId_),
      [this](PythonBridgeResult result) {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(std::move(result))));
          return;
        }
        const QJsonObject authentication = std::get<QJsonObject>(std::move(result))
                                               .value(QStringLiteral("authentication"))
                                               .toObject();
        const QJsonValue connected = authentication.value(QStringLiteral("connected"));
        if (!connected.isBool()) {
          setStatus(QStringLiteral("HCB bridge returned an invalid authentication state"));
          return;
        }
        if (googleConnected_ != connected.toBool()) {
          googleConnected_ = connected.toBool();
          emit googleConnectedChanged();
        }
      },
      false);
}

void AppController::reportBridgeUnsupportedAction() {
  setStatus(QStringLiteral("This action is not implemented in the HCB bridge preview"));
}

void AppController::refreshBridge() {
  if (pythonBridgeClient_ == nullptr || pythonBridgeAccountId_.isEmpty()) {
    setStatus(QStringLiteral("An HCB bridge account is required"));
    return;
  }
  const std::uint64_t generation = ++pythonBridgeRefreshGeneration_;
  ++pythonBridgeConflictsGeneration_;
  ++pythonBridgeSavedSearchesGeneration_;
  pythonBridgeTasksReady_ = false;
  pythonBridgeCalendarReady_ = false;
  pythonBridgeSavedSearchesReady_ = false;
  setStatus(QStringLiteral("Loading HCB core bridge"));
  watch(pythonBridgeClient_->workspace(pythonBridgeAccountId_),
        [this, generation](PythonBridgeResult result) {
          if (generation != pythonBridgeRefreshGeneration_) {
            return;
          }
          if (std::holds_alternative<AppError>(result)) {
            const QString message = errorMessage(std::get<AppError>(std::move(result)));
            setStatus(message);
            setTaskListError(message);
            return;
          }
          PythonBridgeWorkspaceSummaryOrError decoded =
              PythonBridgeProjection::workspaceSummary(std::get<QJsonObject>(std::move(result)));
          if (std::holds_alternative<AppError>(decoded)) {
            const QString message = errorMessage(std::get<AppError>(std::move(decoded)));
            setStatus(message);
            setTaskListError(message);
            return;
          }
          PythonBridgeWorkspaceSummary summary =
              std::get<PythonBridgeWorkspaceSummary>(std::move(decoded));
          if (summary.accountId != pythonBridgeAccountId_) {
            setStatus(QStringLiteral("HCB bridge returned a different account"));
            setTaskListError(statusMessage_);
            return;
          }
          pythonBridgeExpectedEmail_ = summary.accountEmail;
          setTaskListError({});
          applyBridgeTaskLists(std::move(summary.taskLists));
          applyBridgeCalendars(std::move(summary.calendars), false);
          loadBridgeConflicts(generation);
          loadBridgeSavedSearches(generation);
          loadBridgeTaskPage(generation, std::nullopt, {}, false);
          loadBridgeCalendar(generation);
        });
}

void AppController::loadBridgeConflicts(std::uint64_t generation) {
  if (generation != pythonBridgeRefreshGeneration_ || pythonBridgeClient_ == nullptr) {
    return;
  }
  const std::uint64_t conflictsGeneration = ++pythonBridgeConflictsGeneration_;
  watch(
      pythonBridgeClient_->conflicts(pythonBridgeAccountId_),
      [this, generation, conflictsGeneration](PythonBridgeResult result) {
        if (generation != pythonBridgeRefreshGeneration_ ||
            conflictsGeneration != pythonBridgeConflictsGeneration_) {
          return;
        }
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(std::move(result))));
          return;
        }
        const std::optional<QVariantList> rows =
            bridgeConflictRows(std::get<QJsonObject>(std::move(result)));
        if (!rows.has_value()) {
          setStatus(QStringLiteral("HCB bridge returned invalid conflicts"));
          return;
        }
        if (unresolvedConflicts_ != *rows) {
          unresolvedConflicts_ = *rows;
          emit unresolvedConflictsChanged();
        }
      },
      false);
}

void AppController::loadBridgeSavedSearches(std::uint64_t generation) {
  if (generation != pythonBridgeRefreshGeneration_ || pythonBridgeClient_ == nullptr) {
    return;
  }
  const std::uint64_t searchesGeneration = ++pythonBridgeSavedSearchesGeneration_;
  watch(
      pythonBridgeClient_->savedSearches(pythonBridgeAccountId_),
      [this, generation, searchesGeneration](PythonBridgeResult result) {
        if (generation != pythonBridgeRefreshGeneration_ ||
            searchesGeneration != pythonBridgeSavedSearchesGeneration_) {
          return;
        }
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(std::move(result))));
          return;
        }
        const std::optional<QList<SavedSearch>> searches =
            bridgeSavedSearches(std::get<QJsonObject>(std::move(result)));
        if (!searches.has_value()) {
          setStatus(QStringLiteral("HCB bridge returned invalid saved searches"));
          return;
        }
        setSavedSearches(*searches);
        pythonBridgeSavedSearchesReady_ = true;
        completeBridgeReadyProbe();
      },
      false);
}

void AppController::loadBridgeTaskPage(std::uint64_t generation,
                                       std::optional<QString> cursor,
                                       QList<TaskModelTask> accumulated,
                                       bool firstPageApplied) {
  if (generation != pythonBridgeRefreshGeneration_ || pythonBridgeClient_ == nullptr) {
    return;
  }
  watch(pythonBridgeClient_->taskPage(pythonBridgeAccountId_, 500, std::move(cursor)),
        [this, generation, accumulated = std::move(accumulated), firstPageApplied](
            PythonBridgeResult result) mutable {
          if (generation != pythonBridgeRefreshGeneration_) {
            return;
          }
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          PythonBridgeTaskPageOrError decoded = PythonBridgeProjection::taskPage(
              std::get<QJsonObject>(std::move(result)), pythonBridgeTaskListTitles_);
          if (std::holds_alternative<AppError>(decoded)) {
            setStatus(errorMessage(std::get<AppError>(std::move(decoded))));
            return;
          }
          PythonBridgeTaskPage page = std::get<PythonBridgeTaskPage>(std::move(decoded));
          accumulated.append(std::move(page.tasks));
          const bool applyNow = !firstPageApplied || !page.nextCursor.has_value();
          if (applyNow) {
            pythonBridgeTasks_ = accumulated;
            applyTaskProjections(visibleBridgeTasks());
          }
          if (page.nextCursor.has_value()) {
            setStatus(QStringLiteral("Loading HCB core tasks (%1 loaded)").arg(accumulated.size()));
            loadBridgeTaskPage(
                generation, std::move(page.nextCursor), std::move(accumulated), true);
            return;
          }
          setStatus(QStringLiteral("HCB core bridge loaded %1 tasks").arg(accumulated.size()));
          pythonBridgeTasksReady_ = true;
          completeBridgeReadyProbe();
        });
}

void AppController::loadBridgeCalendar(std::uint64_t generation) {
  if (generation != pythonBridgeRefreshGeneration_ || pythonBridgeClient_ == nullptr) {
    return;
  }
  const QDate rangeStart = monthGridStart(calendarDate_, weekStartDay_);
  const QDate rangeEnd = rangeStart.addDays(42);
  watch(pythonBridgeClient_->eventRange(pythonBridgeAccountId_, rangeStart, rangeEnd),
        [this, generation](PythonBridgeResult result) {
          if (generation != pythonBridgeRefreshGeneration_) {
            return;
          }
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          PythonBridgeEventRangeOrError decoded =
              PythonBridgeProjection::eventRange(std::get<QJsonObject>(std::move(result)));
          if (std::holds_alternative<AppError>(decoded)) {
            setStatus(errorMessage(std::get<AppError>(std::move(decoded))));
            return;
          }
          applyBridgeCalendarEvents(generation,
                                    std::get<QList<CalendarEventSummary>>(std::move(decoded)));
        });
}

void AppController::applyBridgeCalendarEvents(std::uint64_t generation,
                                              QList<CalendarEventSummary> events) {
  if (generation != pythonBridgeRefreshGeneration_) {
    return;
  }
  QSet<QString> visibleCalendars;
  for (const CalendarSummary& calendar : pythonBridgeCalendars_) {
    if (calendar.selected && !calendar.hidden) {
      visibleCalendars.insert(calendar.id);
    }
  }
  events.erase(std::remove_if(events.begin(),
                              events.end(),
                              [&visibleCalendars](const CalendarEventSummary& event) {
                                return !visibleCalendars.contains(event.calendarId);
                              }),
               events.end());
  const QDate date = calendarDate_;
  const QTimeZone displayTimeZone(displayTimeZone_.toUtf8());
  const int firstDay = weekStartDay_;
  pythonBridgeCalendarEvents_ = events;
  watch(std::async(std::launch::async,
                   [date, events = std::move(events), displayTimeZone, firstDay]() mutable {
                     return buildCalendarViewLayouts(
                         date, std::move(events), displayTimeZone, firstDay);
                   }),
        [this, generation](CalendarViewLayouts layouts) {
          if (generation != pythonBridgeRefreshGeneration_) {
            return;
          }
          agendaModel_.setEvents(std::move(layouts.agendaEvents));
          timelineModel_.applyLayout(std::move(layouts.timeline));
          monthGridModel_.applyLayout(std::move(layouts.month));
          pythonBridgeCalendarReady_ = true;
          completeBridgeReadyProbe();
        });
}

void AppController::applyBridgeTaskLists(QList<TaskListSummary> taskLists) {
  pythonBridgeTaskLists_ = std::move(taskLists);
  pythonBridgeTaskListTitles_.clear();
  pythonBridgeTaskListTitles_.reserve(pythonBridgeTaskLists_.size());
  for (const TaskListSummary& taskList : pythonBridgeTaskLists_) {
    pythonBridgeTaskListTitles_.insert(taskList.id, taskList.title);
  }
  taskListModel_.setTaskLists(pythonBridgeTaskLists_);
  for (TaskModelTask& task : pythonBridgeTasks_) {
    const auto title = pythonBridgeTaskListTitles_.constFind(task.taskListId);
    if (title != pythonBridgeTaskListTitles_.cend()) {
      task.taskListTitle = *title;
    }
  }
  applyTaskProjections(visibleBridgeTasks());
}

QList<TaskModelTask> AppController::visibleBridgeTasks() const {
  QSet<QString> selectedTaskLists;
  for (const TaskListSummary& taskList : pythonBridgeTaskLists_) {
    if (taskList.selected) {
      selectedTaskLists.insert(taskList.id);
    }
  }
  QList<TaskModelTask> visible;
  visible.reserve(pythonBridgeTasks_.size());
  for (const TaskModelTask& task : pythonBridgeTasks_) {
    if (selectedTaskLists.contains(task.taskListId)) {
      visible.append(task);
    }
  }
  return visible;
}

void AppController::applyBridgeCalendars(QList<CalendarSummary> calendars,
                                         bool reloadCalendarRange) {
  pythonBridgeCalendars_ = std::move(calendars);
  QStringList visibleIds;
  QVariantList managementRows;
  managementRows.reserve(pythonBridgeCalendars_.size());
  for (const CalendarSummary& calendar : pythonBridgeCalendars_) {
    if (calendar.selected && !calendar.hidden) {
      visibleIds.append(calendar.id);
    }
    managementRows.append(QVariantMap{
        {QStringLiteral("id"), calendar.id},
        {QStringLiteral("title"), calendar.title},
        {QStringLiteral("description"), calendar.description.value_or(QString())},
        {QStringLiteral("timeZone"), calendar.timeZone.value_or(QString())},
        {QStringLiteral("colorId"), calendar.colorId.value_or(QString())},
        {QStringLiteral("backgroundColor"), calendar.backgroundColor.value_or(QString())},
        {QStringLiteral("accessRole"), QString()},
        {QStringLiteral("selected"), calendar.selected},
        {QStringLiteral("hidden"), calendar.hidden},
        {QStringLiteral("primary"), calendar.primary}});
  }
  calendarSourceModel_.setCalendars(pythonBridgeCalendars_);
  monthGridModel_.setVisibleCalendarIds(visibleIds);
  visibleCalendarIds_.clear();
  visibleCalendarIds_.reserve(visibleIds.size());
  for (const QString& id : visibleIds) {
    visibleCalendarIds_.append(id);
  }
  if (!calendarVisibilityConfigured_) {
    calendarVisibilityConfigured_ = true;
    emit calendarVisibilityConfiguredChanged();
  }
  emit visibleCalendarIdsChanged();
  setCalendarManagementRows(std::move(managementRows));
  if (reloadCalendarRange) {
    pythonBridgeCalendarReady_ = false;
    loadBridgeCalendar(pythonBridgeRefreshGeneration_);
  }
}

bool AppController::applyBridgeTaskResponse(const QJsonObject& data) {
  const QJsonObject task = data.value(QStringLiteral("task")).toObject();
  if (task.isEmpty()) {
    setStatus(QStringLiteral("HCB bridge mutation returned an invalid task"));
    return false;
  }
  const QString id = task.value(QStringLiteral("id")).toString();
  const bool deleted =
      task.value(QStringLiteral("metadata")).toObject().value(QStringLiteral("deleted")).toBool();
  const auto existing =
      std::find_if(pythonBridgeTasks_.begin(),
                   pythonBridgeTasks_.end(),
                   [&id](const TaskModelTask& candidate) { return candidate.id == id; });
  if (deleted) {
    if (existing != pythonBridgeTasks_.end()) {
      pythonBridgeTasks_.erase(existing);
      applyTaskProjections(visibleBridgeTasks());
      refreshSearchProjection();
    }
    return true;
  }
  QJsonObject page{{QStringLiteral("tasks"), QJsonArray{task}},
                   {QStringLiteral("next_cursor"), QJsonValue::Null}};
  const PythonBridgeTaskPageOrError decoded = PythonBridgeProjection::taskPage(
      QJsonObject{{QStringLiteral("page"), page}}, pythonBridgeTaskListTitles_);
  if (std::holds_alternative<AppError>(decoded)) {
    setStatus(errorMessage(std::get<AppError>(decoded)));
    return false;
  }
  TaskModelTask projected = std::get<PythonBridgeTaskPage>(decoded).tasks.first();
  if (existing == pythonBridgeTasks_.end()) {
    projected.sortOrder = pythonBridgeTasks_.size();
    pythonBridgeTasks_.append(std::move(projected));
  } else {
    projected.sortOrder = existing->sortOrder;
    *existing = std::move(projected);
  }
  applyTaskProjections(visibleBridgeTasks());
  refreshSearchProjection();
  return true;
}

bool AppController::applyBridgeTaskBatchResponse(const QJsonObject& data) {
  const QJsonValue tasksValue = data.value(QStringLiteral("tasks"));
  const QJsonValue successorsValue = data.value(QStringLiteral("successors"));
  if (!tasksValue.isArray() || tasksValue.toArray().isEmpty() ||
      (!successorsValue.isUndefined() && !successorsValue.isArray())) {
    setStatus(QStringLiteral("HCB bridge bulk mutation returned invalid tasks"));
    return false;
  }
  const QList<QJsonArray> batches{
      tasksValue.toArray(), successorsValue.isArray() ? successorsValue.toArray() : QJsonArray{}};
  QHash<QString, TaskModelTask> projectedTasks;
  QHash<QString, bool> deletedById;
  for (const QJsonArray& batch : batches) {
    QJsonObject page{{QStringLiteral("tasks"), batch},
                     {QStringLiteral("next_cursor"), QJsonValue::Null}};
    const PythonBridgeTaskPageOrError decoded = PythonBridgeProjection::taskPage(
        QJsonObject{{QStringLiteral("page"), page}}, pythonBridgeTaskListTitles_);
    if (std::holds_alternative<AppError>(decoded)) {
      setStatus(errorMessage(std::get<AppError>(decoded)));
      return false;
    }
    const QList<TaskModelTask>& projected = std::get<PythonBridgeTaskPage>(decoded).tasks;
    for (qsizetype index = 0; index < batch.size(); ++index) {
      const QJsonObject raw = batch.at(index).toObject();
      const QJsonObject metadata = raw.value(QStringLiteral("metadata")).toObject();
      const QJsonValue deleted = metadata.value(QStringLiteral("deleted"));
      const QString id = projected.at(index).id;
      if (metadata.isEmpty() || !deleted.isBool() || projectedTasks.contains(id)) {
        setStatus(QStringLiteral("HCB bridge bulk mutation returned invalid tasks"));
        return false;
      }
      projectedTasks.insert(id, projected.at(index));
      deletedById.insert(id, deleted.toBool());
    }
  }

  QList<TaskModelTask> updatedTasks = pythonBridgeTasks_;
  for (auto iterator = projectedTasks.cbegin(); iterator != projectedTasks.cend(); ++iterator) {
    const QString& id = iterator.key();
    const auto existing = std::find_if(updatedTasks.begin(),
                                       updatedTasks.end(),
                                       [&id](const TaskModelTask& task) { return task.id == id; });
    if (deletedById.value(id)) {
      if (existing != updatedTasks.end()) {
        updatedTasks.erase(existing);
      }
      continue;
    }
    TaskModelTask projected = iterator.value();
    if (existing == updatedTasks.end()) {
      projected.sortOrder = updatedTasks.size();
      updatedTasks.append(std::move(projected));
    } else {
      projected.sortOrder = existing->sortOrder;
      *existing = std::move(projected);
    }
  }
  pythonBridgeTasks_ = std::move(updatedTasks);
  applyTaskProjections(visibleBridgeTasks());
  refreshSearchProjection();
  return true;
}

bool AppController::applyBridgeEventResponse(const QJsonObject& data) {
  const QJsonObject event = data.value(QStringLiteral("event")).toObject();
  if (event.isEmpty()) {
    setStatus(QStringLiteral("HCB bridge mutation returned an invalid event"));
    return false;
  }
  const QString id = event.value(QStringLiteral("id")).toString();
  const bool deleted =
      event.value(QStringLiteral("metadata")).toObject().value(QStringLiteral("deleted")).toBool();
  const auto existing =
      std::find_if(pythonBridgeCalendarEvents_.begin(),
                   pythonBridgeCalendarEvents_.end(),
                   [&id](const CalendarEventSummary& candidate) { return candidate.id == id; });
  if (deleted) {
    if (existing != pythonBridgeCalendarEvents_.end()) {
      pythonBridgeCalendarEvents_.erase(existing);
      applyBridgeCalendarEvents(pythonBridgeRefreshGeneration_, pythonBridgeCalendarEvents_);
      refreshSearchProjection();
    }
    return true;
  }
  QJsonObject workspace;
  workspace.insert(QStringLiteral("events"), QJsonArray{event});
  const PythonBridgeEventRangeOrError decoded =
      PythonBridgeProjection::eventRange(QJsonObject{{QStringLiteral("workspace"), workspace}});
  if (std::holds_alternative<AppError>(decoded)) {
    setStatus(errorMessage(std::get<AppError>(decoded)));
    return false;
  }
  CalendarEventSummary projected = std::get<QList<CalendarEventSummary>>(decoded).first();
  if (existing == pythonBridgeCalendarEvents_.end()) {
    pythonBridgeCalendarEvents_.append(std::move(projected));
  } else {
    *existing = std::move(projected);
  }
  applyBridgeCalendarEvents(pythonBridgeRefreshGeneration_, pythonBridgeCalendarEvents_);
  refreshSearchProjection();
  return true;
}

bool AppController::applyBridgeTaskListResponse(const QJsonObject& data) {
  const QJsonObject taskList = data.value(QStringLiteral("task_list")).toObject();
  if (taskList.isEmpty() || pythonBridgeExpectedEmail_.isEmpty()) {
    setStatus(QStringLiteral("HCB bridge mutation returned an invalid task list"));
    return false;
  }
  const QJsonObject workspace{{QStringLiteral("account"),
                               QJsonObject{{QStringLiteral("id"), pythonBridgeAccountId_},
                                           {QStringLiteral("email"), pythonBridgeExpectedEmail_}}},
                              {QStringLiteral("pending"), 0},
                              {QStringLiteral("task_lists"), QJsonArray{taskList}},
                              {QStringLiteral("calendars"), QJsonArray{}}};
  const PythonBridgeWorkspaceSummaryOrError decoded = PythonBridgeProjection::workspaceSummary(
      QJsonObject{{QStringLiteral("workspace"), workspace}});
  if (std::holds_alternative<AppError>(decoded)) {
    setStatus(errorMessage(std::get<AppError>(decoded)));
    return false;
  }
  const TaskListSummary projected =
      std::get<PythonBridgeWorkspaceSummary>(decoded).taskLists.first();
  const bool deleted = taskList.value(QStringLiteral("metadata"))
                           .toObject()
                           .value(QStringLiteral("deleted"))
                           .toBool();
  const auto existing = std::find_if(
      pythonBridgeTaskLists_.begin(),
      pythonBridgeTaskLists_.end(),
      [&projected](const TaskListSummary& candidate) { return candidate.id == projected.id; });
  if (deleted) {
    if (existing != pythonBridgeTaskLists_.end()) {
      pythonBridgeTaskLists_.erase(existing);
    }
    pythonBridgeTasks_.erase(std::remove_if(pythonBridgeTasks_.begin(),
                                            pythonBridgeTasks_.end(),
                                            [&projected](const TaskModelTask& task) {
                                              return task.taskListId == projected.id;
                                            }),
                             pythonBridgeTasks_.end());
  } else if (existing == pythonBridgeTaskLists_.end()) {
    pythonBridgeTaskLists_.append(projected);
  } else {
    *existing = projected;
  }
  applyBridgeTaskLists(pythonBridgeTaskLists_);
  refreshSearchProjection();
  return true;
}

bool AppController::applyBridgeCalendarResponse(const QJsonObject& data) {
  const QJsonObject calendar = data.value(QStringLiteral("calendar")).toObject();
  if (calendar.isEmpty() || pythonBridgeExpectedEmail_.isEmpty()) {
    setStatus(QStringLiteral("HCB bridge mutation returned an invalid calendar"));
    return false;
  }
  const QJsonObject workspace{{QStringLiteral("account"),
                               QJsonObject{{QStringLiteral("id"), pythonBridgeAccountId_},
                                           {QStringLiteral("email"), pythonBridgeExpectedEmail_}}},
                              {QStringLiteral("pending"), 0},
                              {QStringLiteral("task_lists"), QJsonArray{}},
                              {QStringLiteral("calendars"), QJsonArray{calendar}}};
  const PythonBridgeWorkspaceSummaryOrError decoded = PythonBridgeProjection::workspaceSummary(
      QJsonObject{{QStringLiteral("workspace"), workspace}});
  if (std::holds_alternative<AppError>(decoded)) {
    setStatus(errorMessage(std::get<AppError>(decoded)));
    return false;
  }
  const CalendarSummary projected =
      std::get<PythonBridgeWorkspaceSummary>(decoded).calendars.first();
  const bool deleted = calendar.value(QStringLiteral("metadata"))
                           .toObject()
                           .value(QStringLiteral("deleted"))
                           .toBool();
  const auto existing = std::find_if(
      pythonBridgeCalendars_.begin(),
      pythonBridgeCalendars_.end(),
      [&projected](const CalendarSummary& candidate) { return candidate.id == projected.id; });
  if (deleted) {
    if (existing != pythonBridgeCalendars_.end()) {
      pythonBridgeCalendars_.erase(existing);
    }
  } else if (existing == pythonBridgeCalendars_.end()) {
    pythonBridgeCalendars_.append(projected);
  } else {
    *existing = projected;
  }
  applyBridgeCalendars(pythonBridgeCalendars_, true);
  refreshSearchProjection();
  return true;
}

bool AppController::applyBridgeSavedSearchResponse(const QJsonObject& data) {
  const std::optional<SavedSearch> search =
      bridgeSavedSearch(data.value(QStringLiteral("saved_search")).toObject());
  if (!search.has_value()) {
    setStatus(QStringLiteral("HCB bridge mutation returned an invalid saved search"));
    return false;
  }
  QList<SavedSearch> next = savedSearches_;
  const auto existing =
      std::find_if(next.begin(), next.end(), [&search](const SavedSearch& candidate) {
        return candidate.id == search->id;
      });
  if (existing == next.end()) {
    next.append(*search);
  } else {
    *existing = *search;
  }
  std::sort(next.begin(), next.end(), [](const SavedSearch& left, const SavedSearch& right) {
    return left.name < right.name;
  });
  setSavedSearches(std::move(next));
  return true;
}

void AppController::startBridgeOperation(QString kind, std::future<PythonBridgeResult> future) {
  if (bridgeOperationActive()) {
    setStatus(QStringLiteral("An HCB bridge operation is already in progress"));
    return;
  }
  pythonBridgeOperationStarting_ = true;
  pythonBridgeOperationKind_ = kind;
  emit bridgeOperationChanged();
  setSyncStatus(kind + QStringLiteral(" starting"));
  watch(std::move(future), [this, kind = std::move(kind)](PythonBridgeResult result) {
    if (!pythonBridgeOperationStarting_ || pythonBridgeOperationKind_ != kind) {
      return;
    }
    pythonBridgeOperationStarting_ = false;
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(std::move(result))));
      pythonBridgeOperationKind_.clear();
      setSyncStatus(kind + QStringLiteral(" failed"));
      emit bridgeOperationChanged();
      return;
    }
    const QJsonObject operation =
        std::get<QJsonObject>(std::move(result)).value(QStringLiteral("operation")).toObject();
    const QString operationId = operation.value(QStringLiteral("id")).toString();
    if (!isValidBridgeOperation(operation, pythonBridgeAccountId_)) {
      setStatus(QStringLiteral("HCB bridge returned an invalid operation"));
      pythonBridgeOperationKind_.clear();
      setSyncStatus(kind + QStringLiteral(" failed"));
      emit bridgeOperationChanged();
      return;
    }
    pythonBridgeOperationId_ = operationId;
    emit bridgeOperationChanged();
    setSyncStatus(kind + QStringLiteral(" in progress"));
    pollBridgeOperation(operationId);
  });
}

void AppController::pollBridgeOperation(QString operationId) {
  if (pythonBridgeClient_ == nullptr || operationId != pythonBridgeOperationId_) {
    return;
  }
  watch(
      pythonBridgeClient_->operation(operationId),
      [this, operationId](PythonBridgeResult result) {
        if (operationId != pythonBridgeOperationId_) {
          return;
        }
        const QString kind = pythonBridgeOperationKind_;
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(std::move(result))));
          pythonBridgeOperationId_.clear();
          pythonBridgeOperationKind_.clear();
          setSyncStatus(kind + QStringLiteral(" failed"));
          emit bridgeOperationChanged();
          return;
        }
        const QJsonObject operation =
            std::get<QJsonObject>(std::move(result)).value(QStringLiteral("operation")).toObject();
        const QString state = operation.value(QStringLiteral("state")).toString();
        const QJsonValue progressValue = operation.value(QStringLiteral("progress"));
        const QJsonArray progress = progressValue.toArray();
        if (!isValidBridgeOperation(operation, pythonBridgeAccountId_, operationId)) {
          setStatus(QStringLiteral("HCB bridge returned an invalid operation response"));
          pythonBridgeOperationId_.clear();
          pythonBridgeOperationKind_.clear();
          setSyncStatus(kind + QStringLiteral(" failed"));
          emit bridgeOperationChanged();
          return;
        }
        if (!progress.isEmpty() && progress.last().isString()) {
          setStatus(progress.last().toString());
        }
        if (state == QStringLiteral("queued") || state == QStringLiteral("running")) {
          QTimer::singleShot(250, this, [this, operationId] { pollBridgeOperation(operationId); });
          return;
        }
        pythonBridgeOperationId_.clear();
        pythonBridgeOperationKind_.clear();
        emit bridgeOperationChanged();
        if (state == QStringLiteral("succeeded")) {
          if (kind == QStringLiteral("Google authorization") && !googleConnected_) {
            googleConnected_ = true;
            emit googleConnectedChanged();
          }
          setSyncStatus(kind + QStringLiteral(" complete"));
          setStatus(kind + QStringLiteral(" complete"));
          refreshBridge();
          return;
        }
        if (state == QStringLiteral("cancelled")) {
          setSyncStatus(kind + QStringLiteral(" cancelled"));
          setStatus(kind + QStringLiteral(" cancelled"));
          return;
        }
        const QString message = operation.value(QStringLiteral("error"))
                                    .toObject()
                                    .value(QStringLiteral("message"))
                                    .toString();
        setSyncStatus(kind + QStringLiteral(" failed"));
        setStatus(message.isEmpty() ? kind + QStringLiteral(" failed") : message);
      },
      false);
}

void AppController::cancelBridgeOperation() {
  if (pythonBridgeClient_ == nullptr) {
    return;
  }
  if (pythonBridgeOperationStarting_) {
    setStatus(QStringLiteral("HCB bridge operation is still starting"));
    return;
  }
  if (pythonBridgeOperationId_.isEmpty()) {
    return;
  }
  const QString operationId = pythonBridgeOperationId_;
  watch(pythonBridgeClient_->cancelOperation(operationId),
        [this, operationId](PythonBridgeResult result) {
          if (operationId != pythonBridgeOperationId_) {
            return;
          }
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            setSyncStatus(pythonBridgeOperationKind_ + QStringLiteral(" still in progress"));
            return;
          }
          setStatus(QStringLiteral("Cancelling HCB bridge operation"));
          pollBridgeOperation(operationId);
        });
}

void AppController::completeBridgeReadyProbe() {
  const QString readyPath = qEnvironmentVariable("HCB_BRIDGE_READY_FILE");
  const QString interactionPath = qEnvironmentVariable("HCB_BRIDGE_INTERACTION_ACCEPTANCE_FILE");
  const QString failurePath = qEnvironmentVariable("HCB_BRIDGE_FAILURE_ACCEPTANCE_FILE");
  if ((readyPath.isEmpty() && interactionPath.isEmpty() && failurePath.isEmpty()) ||
      !pythonBridgeTasksReady_ || !pythonBridgeCalendarReady_ || !pythonBridgeSavedSearchesReady_) {
    return;
  }
  if (!failurePath.isEmpty()) {
    if (bridgeFailureAcceptanceStarted_) {
      return;
    }
    bridgeFailureAcceptanceStarted_ = true;
    runBridgeFailureAcceptance(failurePath);
    return;
  }
  if (!interactionPath.isEmpty()) {
    if (bridgeInteractionAcceptanceStarted_) {
      return;
    }
    bridgeInteractionAcceptanceStarted_ = true;
    runBridgeInteractionAcceptance(interactionPath);
    return;
  }
  QSaveFile readyFile(readyPath);
  if (!readyFile.open(QIODevice::WriteOnly)) {
    return;
  }
  const QJsonObject payload{{QStringLiteral("tasks"), taskProjectionTasks_.size()},
                            {QStringLiteral("events"), pythonBridgeCalendarEvents_.size()},
                            {QStringLiteral("saved_searches"), savedSearches_.size()}};
  const QByteArray encoded = QJsonDocument(payload).toJson(QJsonDocument::Compact);
  if (readyFile.write(encoded) != encoded.size() || !readyFile.commit()) {
    return;
  }
  QCoreApplication::quit();
}

void AppController::runBridgeInteractionAcceptance(QString reportPath) {
  struct State final {
    QString reportPath;
    QString taskId;
    QString hiddenTaskId;
    QString eventId;
    QString taskListId;
    QString calendarId;
    QString subscriptionId;
    QString recurrenceTaskId;
    QString recurrenceSuccessorId;
    QString splitRecurrenceTaskId;
    QString splitRecurrenceSuccessorId;
    QString recurrenceSeriesId;
    QString splitRecurrenceSeriesId;
    QString hierarchyParentId;
    QString hierarchyFirstChildId;
    QString hierarchySecondChildId;
    QString bulkFirstTaskId;
    QString bulkSecondTaskId;
    QString conflictId;
    QString savedSearchId;
    QString deletedSavedSearchId;
    QString eventDate;
    int stage{0};
    int initialSearchResults{0};
    int refreshedSearchResults{0};
    QElapsedTimer elapsed;
  };

  const auto state = std::make_shared<State>();
  state->reportPath = std::move(reportPath);
  state->eventDate = calendarDate_.addDays(1).toString(Qt::ISODate);
  state->elapsed.start();

  auto* timer = new QTimer(this);
  timer->setInterval(20);
  const auto finish = [this, timer, state](bool success, QString error = {}) {
    QJsonObject payload{
        {QStringLiteral("ok"), success},
        {QStringLiteral("stage"), state->stage},
        {QStringLiteral("task_id"), state->taskId},
        {QStringLiteral("hidden_task_id"), state->hiddenTaskId},
        {QStringLiteral("event_id"), state->eventId},
        {QStringLiteral("task_list_id"), state->taskListId},
        {QStringLiteral("calendar_id"), state->calendarId},
        {QStringLiteral("recurrence_task_id"), state->recurrenceTaskId},
        {QStringLiteral("recurrence_successor_id"), state->recurrenceSuccessorId},
        {QStringLiteral("split_recurrence_task_id"), state->splitRecurrenceTaskId},
        {QStringLiteral("split_recurrence_successor_id"), state->splitRecurrenceSuccessorId},
        {QStringLiteral("hierarchy_parent_id"), state->hierarchyParentId},
        {QStringLiteral("hierarchy_first_child_id"), state->hierarchyFirstChildId},
        {QStringLiteral("hierarchy_second_child_id"), state->hierarchySecondChildId},
        {QStringLiteral("bulk_first_task_id"), state->bulkFirstTaskId},
        {QStringLiteral("bulk_second_task_id"), state->bulkSecondTaskId},
        {QStringLiteral("conflict_id"), state->conflictId},
        {QStringLiteral("saved_search_id"), state->savedSearchId},
        {QStringLiteral("event_date"), state->eventDate},
        {QStringLiteral("initial_search_results"), state->initialSearchResults},
        {QStringLiteral("refreshed_search_results"), state->refreshedSearchResults}};
    if (!error.isEmpty()) {
      payload.insert(QStringLiteral("error"), std::move(error));
      payload.insert(QStringLiteral("status"), statusMessage_);
    }
    QSaveFile report(state->reportPath);
    const QByteArray encoded = QJsonDocument(payload).toJson(QJsonDocument::Compact);
    const bool wrote = report.open(QIODevice::WriteOnly) &&
                       report.write(encoded) == encoded.size() && report.commit();
    timer->stop();
    timer->deleteLater();
    QCoreApplication::exit(success && wrote ? 0 : 3);
  };

  connect(timer, &QTimer::timeout, this, [this, state, finish] {
    const auto taskById = [this, state]() -> const TaskModelTask* {
      const auto found =
          std::find_if(taskProjectionTasks_.cbegin(),
                       taskProjectionTasks_.cend(),
                       [state](const TaskModelTask& task) { return task.id == state->taskId; });
      return found == taskProjectionTasks_.cend() ? nullptr : &*found;
    };
    const auto eventById = [this, state]() -> const CalendarEventSummary* {
      const auto found = std::find_if(
          pythonBridgeCalendarEvents_.cbegin(),
          pythonBridgeCalendarEvents_.cend(),
          [state](const CalendarEventSummary& event) { return event.id == state->eventId; });
      return found == pythonBridgeCalendarEvents_.cend() ? nullptr : &*found;
    };
    const auto countSearchTasks = [this](const QString& id = {}) {
      int matches = 0;
      SearchResultsModel& model = searchResultsModel();
      for (int row = 0; row < model.rowCount(); ++row) {
        const QModelIndex index = model.index(row, 0);
        if (model.data(index, SearchResultsModel::ResourceRole).toString() ==
                QStringLiteral("task") &&
            (id.isEmpty() || model.data(index, SearchResultsModel::IdRole).toString() == id)) {
          ++matches;
        }
      }
      return matches;
    };
    if (state->elapsed.elapsed() > 60'000) {
      finish(false, QStringLiteral("timed out waiting for Qt bridge interaction stage"));
      return;
    }

    switch (state->stage) {
    case 0:
      setSearchQuery(QStringLiteral("release-marker"));
      state->stage = 1;
      return;
    case 1:
      state->initialSearchResults = countSearchTasks();
      if (state->initialSearchResults == 0) {
        return;
      }
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QString::fromLatin1(kBridgeAcceptanceInitialTaskTitle),
                         QString::fromLatin1(kBridgeAcceptanceInitialTaskNotes),
                         calendarDate_.toString(Qt::ISODate),
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 2;
      return;
    case 2: {
      const auto created = std::find_if(
          taskProjectionTasks_.cbegin(),
          taskProjectionTasks_.cend(),
          [](const TaskModelTask& task) {
            return task.title == QString::fromLatin1(kBridgeAcceptanceInitialTaskTitle);
          });
      if (created == taskProjectionTasks_.cend()) {
        return;
      }
      state->taskId = created->id;
      updateTaskDetailed(state->taskId,
                         QString::fromLatin1(kBridgeAcceptanceUpdatedTaskTitle),
                         QString::fromLatin1(kBridgeAcceptanceUpdatedTaskNotes),
                         state->eventDate,
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 3;
      return;
    }
    case 3: {
      const TaskModelTask* updated = taskById();
      if (updated == nullptr ||
          updated->title != QString::fromLatin1(kBridgeAcceptanceUpdatedTaskTitle) ||
          updated->notes.value_or(QString()) !=
              QString::fromLatin1(kBridgeAcceptanceUpdatedTaskNotes)) {
        return;
      }
      setTaskCompleted(state->taskId, true);
      state->stage = 4;
      return;
    }
    case 4: {
      const TaskModelTask* completed = taskById();
      if (completed == nullptr || !completed->completed) {
        return;
      }
      createEvent(QStringLiteral("primary"),
                  QString::fromLatin1(kBridgeAcceptanceInitialEventTitle),
                  state->eventDate + QStringLiteral("T09:00:00.000Z"),
                  state->eventDate + QStringLiteral("T10:00:00.000Z"),
                  false,
                  QString::fromLatin1(kBridgeAcceptanceInitialEventDescription),
                  QString::fromLatin1(kBridgeAcceptanceInitialEventLocation));
      state->stage = 5;
      return;
    }
    case 5: {
      const auto created = std::find_if(
          pythonBridgeCalendarEvents_.cbegin(),
          pythonBridgeCalendarEvents_.cend(),
          [](const CalendarEventSummary& event) {
            return event.title == QString::fromLatin1(kBridgeAcceptanceInitialEventTitle);
          });
      if (created == pythonBridgeCalendarEvents_.cend()) {
        return;
      }
      state->eventId = created->id;
      updateEvent(state->eventId,
                  QStringLiteral("primary"),
                  QString::fromLatin1(kBridgeAcceptanceUpdatedEventTitle),
                  state->eventDate + QStringLiteral("T10:00:00.000Z"),
                  state->eventDate + QStringLiteral("T11:00:00.000Z"),
                  false,
                  QString::fromLatin1(kBridgeAcceptanceUpdatedEventDescription),
                  QString::fromLatin1(kBridgeAcceptanceUpdatedEventLocation));
      state->stage = 6;
      return;
    }
    case 6: {
      const CalendarEventSummary* updated = eventById();
      if (updated == nullptr ||
          updated->title != QString::fromLatin1(kBridgeAcceptanceUpdatedEventTitle) ||
          updated->description.value_or(QString()) !=
              QString::fromLatin1(kBridgeAcceptanceUpdatedEventDescription) ||
          updated->location.value_or(QString()) !=
              QString::fromLatin1(kBridgeAcceptanceUpdatedEventLocation)) {
        return;
      }
      setSearchQuery(QString::fromLatin1(kBridgeAcceptanceUpdatedTaskTitle));
      state->stage = 7;
      return;
    }
    case 7:
      state->refreshedSearchResults = countSearchTasks(state->taskId);
      if (state->refreshedSearchResults == 0) {
        return;
      }
      createTaskList(QString::fromLatin1(kBridgeAcceptanceInitialTaskListTitle));
      state->stage = 8;
      return;
    case 8: {
      const auto created = std::find_if(
          pythonBridgeTaskLists_.cbegin(),
          pythonBridgeTaskLists_.cend(),
          [](const TaskListSummary& taskList) {
            return taskList.title == QString::fromLatin1(kBridgeAcceptanceInitialTaskListTitle);
          });
      if (created == pythonBridgeTaskLists_.cend()) {
        return;
      }
      state->taskListId = created->id;
      renameTaskList(state->taskListId, QString::fromLatin1(kBridgeAcceptanceUpdatedTaskListTitle));
      state->stage = 9;
      return;
    }
    case 9: {
      const auto updated = std::find_if(
          pythonBridgeTaskLists_.cbegin(),
          pythonBridgeTaskLists_.cend(),
          [state](const TaskListSummary& taskList) {
            return taskList.id == state->taskListId &&
                   taskList.title == QString::fromLatin1(kBridgeAcceptanceUpdatedTaskListTitle);
          });
      if (updated == pythonBridgeTaskLists_.cend()) {
        return;
      }
      createTaskDetailed(state->taskListId,
                         {},
                         QString::fromLatin1(kBridgeAcceptanceHiddenTaskTitle),
                         {},
                         {},
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 10;
      return;
    }
    case 10: {
      const auto created =
          std::find_if(pythonBridgeTasks_.cbegin(),
                       pythonBridgeTasks_.cend(),
                       [state](const TaskModelTask& task) {
                         return task.taskListId == state->taskListId &&
                                task.title == QString::fromLatin1(kBridgeAcceptanceHiddenTaskTitle);
                       });
      if (created == pythonBridgeTasks_.cend()) {
        return;
      }
      state->hiddenTaskId = created->id;
      setTaskListSelected(state->taskListId, false);
      state->stage = 11;
      return;
    }
    case 11: {
      const auto updated =
          std::find_if(pythonBridgeTaskLists_.cbegin(),
                       pythonBridgeTaskLists_.cend(),
                       [state](const TaskListSummary& taskList) {
                         return taskList.id == state->taskListId && !taskList.selected;
                       });
      if (updated == pythonBridgeTaskLists_.cend()) {
        return;
      }
      const bool retainedInBridgeCache = std::any_of(
          pythonBridgeTasks_.cbegin(),
          pythonBridgeTasks_.cend(),
          [state](const TaskModelTask& task) { return task.id == state->hiddenTaskId; });
      const bool visibleInTaskProjection = std::any_of(
          taskProjectionTasks_.cbegin(),
          taskProjectionTasks_.cend(),
          [state](const TaskModelTask& task) { return task.id == state->hiddenTaskId; });
      if (!retainedInBridgeCache || visibleInTaskProjection) {
        finish(false, QStringLiteral("task-list selection did not hide its retained task"));
        return;
      }
      createGoogleCalendar(QString::fromLatin1(kBridgeAcceptanceInitialCalendarTitle),
                           QStringLiteral("Initial acceptance calendar"),
                           QStringLiteral("UTC"));
      state->stage = 12;
      return;
    }
    case 12: {
      const auto created = std::find_if(
          pythonBridgeCalendars_.cbegin(),
          pythonBridgeCalendars_.cend(),
          [](const CalendarSummary& calendar) {
            return calendar.title == QString::fromLatin1(kBridgeAcceptanceInitialCalendarTitle);
          });
      if (created == pythonBridgeCalendars_.cend()) {
        return;
      }
      state->calendarId = created->id;
      saveGoogleCalendarSettings(state->calendarId,
                                 QString::fromLatin1(kBridgeAcceptanceUpdatedCalendarTitle),
                                 QString::fromLatin1(kBridgeAcceptanceUpdatedCalendarDescription),
                                 QStringLiteral("UTC"),
                                 false,
                                 true,
                                 QStringLiteral("#123456"));
      state->stage = 13;
      return;
    }
    case 13: {
      const auto updated = std::find_if(
          pythonBridgeCalendars_.cbegin(),
          pythonBridgeCalendars_.cend(),
          [state](const CalendarSummary& calendar) {
            return calendar.id == state->calendarId && !calendar.selected && calendar.hidden &&
                   calendar.title == QString::fromLatin1(kBridgeAcceptanceUpdatedCalendarTitle) &&
                   calendar.description.value_or(QString()) ==
                       QString::fromLatin1(kBridgeAcceptanceUpdatedCalendarDescription) &&
                   calendar.backgroundColor.value_or(QString()) == QStringLiteral("#123456");
          });
      if (updated == pythonBridgeCalendars_.cend()) {
        return;
      }
      subscribeGoogleCalendar(QString::fromLatin1(kBridgeAcceptanceSubscriptionRemoteId));
      state->stage = 14;
      return;
    }
    case 14: {
      const auto subscribed = std::find_if(
          pythonBridgeCalendars_.cbegin(),
          pythonBridgeCalendars_.cend(),
          [](const CalendarSummary& calendar) {
            return calendar.remoteId == QString::fromLatin1(kBridgeAcceptanceSubscriptionRemoteId);
          });
      if (subscribed == pythonBridgeCalendars_.cend()) {
        return;
      }
      state->subscriptionId = subscribed->id;
      unsubscribeGoogleCalendar(state->subscriptionId);
      state->stage = 15;
      return;
    }
    case 15:
      if (std::any_of(pythonBridgeCalendars_.cbegin(),
                      pythonBridgeCalendars_.cend(),
                      [state](const CalendarSummary& calendar) {
                        return calendar.id == state->subscriptionId;
                      })) {
        return;
      }
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QString::fromLatin1(kBridgeAcceptanceRecurringTaskTitle),
                         QStringLiteral("Recurring bridge acceptance context"),
                         state->eventDate,
                         QStringLiteral("UTC"),
                         0,
                         true,
                         0,
                         1,
                         2,
                         {},
                         3);
      state->stage = 16;
      return;
    case 16: {
      const auto created = std::find_if(
          pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), [](const TaskModelTask& task) {
            return task.title == QString::fromLatin1(kBridgeAcceptanceRecurringTaskTitle) &&
                   task.managedRecurrence;
          });
      if (created == pythonBridgeTasks_.cend()) {
        return;
      }
      state->recurrenceTaskId = created->id;
      state->recurrenceSeriesId = created->recurrenceSeriesId;
      updateTaskDetailed(state->recurrenceTaskId,
                         QString::fromLatin1(kBridgeAcceptanceUpdatedRecurringTaskTitle),
                         QStringLiteral("Updated recurrence configuration"),
                         state->eventDate,
                         QStringLiteral("UTC"),
                         0,
                         true,
                         1,
                         2,
                         2,
                         {},
                         3);
      state->stage = 17;
      return;
    }
    case 17: {
      const auto reconfigured = std::find_if(
          pythonBridgeTasks_.cbegin(),
          pythonBridgeTasks_.cend(),
          [state](const TaskModelTask& task) {
            return task.id == state->recurrenceTaskId && task.managedRecurrence &&
                   task.title == QString::fromLatin1(kBridgeAcceptanceUpdatedRecurringTaskTitle) &&
                   task.recurrenceFrequency == 1 && task.recurrenceInterval == 2 &&
                   task.recurrenceSeriesId == state->recurrenceSeriesId;
          });
      if (reconfigured == pythonBridgeTasks_.cend()) {
        return;
      }
      setTaskCompleted(state->recurrenceTaskId, true);
      state->stage = 18;
      return;
    }
    case 18: {
      const auto successor =
          std::find_if(pythonBridgeTasks_.cbegin(),
                       pythonBridgeTasks_.cend(),
                       [state](const TaskModelTask& task) {
                         return task.managedRecurrence &&
                                task.recurrenceSeriesId == state->recurrenceSeriesId &&
                                task.id != state->recurrenceTaskId &&
                                task.recurrenceFrequency == 1 && task.recurrenceInterval == 2;
                       });
      if (successor == pythonBridgeTasks_.cend()) {
        return;
      }
      state->recurrenceSuccessorId = successor->id;
      stopTaskRecurrence(state->recurrenceSuccessorId, 0);
      state->stage = 19;
      return;
    }
    case 19: {
      const auto stopped = std::find_if(
          pythonBridgeTasks_.cbegin(),
          pythonBridgeTasks_.cend(),
          [state](const TaskModelTask& task) { return task.id == state->recurrenceSuccessorId; });
      const bool continuingOccurrence = std::any_of(
          pythonBridgeTasks_.cbegin(),
          pythonBridgeTasks_.cend(),
          [state](const TaskModelTask& task) {
            return task.managedRecurrence && task.recurrenceSeriesId == state->recurrenceSeriesId &&
                   task.id != state->recurrenceTaskId && task.id != state->recurrenceSuccessorId;
          });
      if (stopped == pythonBridgeTasks_.cend() || stopped->managedRecurrence ||
          !continuingOccurrence) {
        return;
      }
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QString::fromLatin1(kBridgeAcceptanceSplitRecurringTaskTitle),
                         {},
                         state->eventDate,
                         QStringLiteral("UTC"),
                         0,
                         true,
                         0,
                         1,
                         2,
                         {},
                         3);
      state->stage = 20;
      return;
    }
    case 20: {
      const auto created = std::find_if(
          pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), [](const TaskModelTask& task) {
            return task.title == QString::fromLatin1(kBridgeAcceptanceSplitRecurringTaskTitle) &&
                   task.managedRecurrence;
          });
      if (created == pythonBridgeTasks_.cend()) {
        return;
      }
      state->splitRecurrenceTaskId = created->id;
      state->splitRecurrenceSeriesId = created->recurrenceSeriesId;
      setTaskCompleted(state->splitRecurrenceTaskId, true);
      state->stage = 21;
      return;
    }
    case 21: {
      const auto successor =
          std::find_if(pythonBridgeTasks_.cbegin(),
                       pythonBridgeTasks_.cend(),
                       [state](const TaskModelTask& task) {
                         return task.managedRecurrence &&
                                task.recurrenceSeriesId == state->splitRecurrenceSeriesId &&
                                task.id != state->splitRecurrenceTaskId;
                       });
      if (successor == pythonBridgeTasks_.cend()) {
        return;
      }
      state->splitRecurrenceSuccessorId = successor->id;
      splitTaskRecurrence(state->splitRecurrenceSuccessorId);
      state->stage = 22;
      return;
    }
    case 22: {
      const auto split =
          std::find_if(pythonBridgeTasks_.cbegin(),
                       pythonBridgeTasks_.cend(),
                       [state](const TaskModelTask& task) {
                         return task.id == state->splitRecurrenceSuccessorId &&
                                task.managedRecurrence &&
                                task.recurrenceSeriesId != state->splitRecurrenceSeriesId &&
                                task.recurrenceOccurrenceId.endsWith(QStringLiteral(":0"));
                       });
      if (split == pythonBridgeTasks_.cend()) {
        return;
      }
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QString::fromLatin1(kBridgeAcceptanceHierarchyParentTaskTitle),
                         {},
                         {},
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 23;
      return;
    }
    case 23: {
      const auto parent = std::find_if(
          pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), [](const TaskModelTask& task) {
            return task.title == QString::fromLatin1(kBridgeAcceptanceHierarchyParentTaskTitle);
          });
      if (parent == pythonBridgeTasks_.cend()) {
        return;
      }
      state->hierarchyParentId = parent->id;
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QString::fromLatin1(kBridgeAcceptanceHierarchyFirstChildTaskTitle),
                         {},
                         {},
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 24;
      return;
    }
    case 24: {
      const auto child = std::find_if(
          pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), [](const TaskModelTask& task) {
            return task.title == QString::fromLatin1(kBridgeAcceptanceHierarchyFirstChildTaskTitle);
          });
      if (child == pythonBridgeTasks_.cend()) {
        return;
      }
      state->hierarchyFirstChildId = child->id;
      reparentTask(state->hierarchyFirstChildId, state->hierarchyParentId);
      state->stage = 25;
      return;
    }
    case 25: {
      const auto child = std::find_if(pythonBridgeTasks_.cbegin(),
                                      pythonBridgeTasks_.cend(),
                                      [state](const TaskModelTask& task) {
                                        return task.id == state->hierarchyFirstChildId &&
                                               task.parentTaskId == state->hierarchyParentId;
                                      });
      if (child == pythonBridgeTasks_.cend()) {
        return;
      }
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QString::fromLatin1(kBridgeAcceptanceHierarchySecondChildTaskTitle),
                         {},
                         {},
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 26;
      return;
    }
    case 26: {
      const auto child = std::find_if(
          pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), [](const TaskModelTask& task) {
            return task.title ==
                   QString::fromLatin1(kBridgeAcceptanceHierarchySecondChildTaskTitle);
          });
      if (child == pythonBridgeTasks_.cend()) {
        return;
      }
      state->hierarchySecondChildId = child->id;
      reparentTask(state->hierarchySecondChildId, state->hierarchyParentId);
      state->stage = 27;
      return;
    }
    case 27: {
      const auto first = std::find_if(pythonBridgeTasks_.cbegin(),
                                      pythonBridgeTasks_.cend(),
                                      [state](const TaskModelTask& task) {
                                        return task.id == state->hierarchyFirstChildId &&
                                               task.parentTaskId == state->hierarchyParentId;
                                      });
      const auto second = std::find_if(pythonBridgeTasks_.cbegin(),
                                       pythonBridgeTasks_.cend(),
                                       [state](const TaskModelTask& task) {
                                         return task.id == state->hierarchySecondChildId &&
                                                task.parentTaskId == state->hierarchyParentId;
                                       });
      if (first == pythonBridgeTasks_.cend() || second == pythonBridgeTasks_.cend()) {
        return;
      }
      reorderTask(state->hierarchyFirstChildId, true);
      state->stage = 28;
      return;
    }
    case 28: {
      const auto first = std::find_if(pythonBridgeTasks_.cbegin(),
                                      pythonBridgeTasks_.cend(),
                                      [state](const TaskModelTask& task) {
                                        return task.id == state->hierarchyFirstChildId &&
                                               task.parentTaskId == state->hierarchyParentId;
                                      });
      const auto second = std::find_if(pythonBridgeTasks_.cbegin(),
                                       pythonBridgeTasks_.cend(),
                                       [state](const TaskModelTask& task) {
                                         return task.id == state->hierarchySecondChildId &&
                                                task.parentTaskId == state->hierarchyParentId;
                                       });
      if (first == pythonBridgeTasks_.cend() || second == pythonBridgeTasks_.cend() ||
          first->sortOrder >= second->sortOrder) {
        return;
      }
      moveTask(state->hierarchyFirstChildId, state->taskListId);
      state->stage = 29;
      return;
    }
    case 29: {
      const auto moved = std::find_if(pythonBridgeTasks_.cbegin(),
                                      pythonBridgeTasks_.cend(),
                                      [state](const TaskModelTask& task) {
                                        return task.id == state->hierarchyFirstChildId &&
                                               task.taskListId == state->taskListId &&
                                               !task.parentTaskId.has_value();
                                      });
      if (moved == pythonBridgeTasks_.cend()) {
        return;
      }
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QString::fromLatin1(kBridgeAcceptanceBulkFirstTaskTitle),
                         {},
                         {},
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 30;
      return;
    }
    case 30: {
      const auto first = std::find_if(
          pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), [](const TaskModelTask& task) {
            return task.title == QString::fromLatin1(kBridgeAcceptanceBulkFirstTaskTitle);
          });
      if (first == pythonBridgeTasks_.cend()) {
        return;
      }
      state->bulkFirstTaskId = first->id;
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QString::fromLatin1(kBridgeAcceptanceBulkSecondTaskTitle),
                         {},
                         {},
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 31;
      return;
    }
    case 31: {
      const auto second = std::find_if(
          pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), [](const TaskModelTask& task) {
            return task.title == QString::fromLatin1(kBridgeAcceptanceBulkSecondTaskTitle);
          });
      if (second == pythonBridgeTasks_.cend()) {
        return;
      }
      state->bulkSecondTaskId = second->id;
      bulkSetTaskCompleted(QVariantList{state->bulkFirstTaskId, state->bulkSecondTaskId}, true);
      state->stage = 32;
      return;
    }
    case 32: {
      const auto completed = [state](const TaskModelTask& task) {
        return (task.id == state->bulkFirstTaskId || task.id == state->bulkSecondTaskId) &&
               task.completed;
      };
      if (std::count_if(pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), completed) != 2) {
        return;
      }
      bulkMoveTasks(QVariantList{state->bulkFirstTaskId, state->bulkSecondTaskId},
                    state->taskListId);
      state->stage = 33;
      return;
    }
    case 33: {
      const auto moved = [state](const TaskModelTask& task) {
        return (task.id == state->bulkFirstTaskId || task.id == state->bulkSecondTaskId) &&
               task.taskListId == state->taskListId && !task.parentTaskId.has_value();
      };
      if (std::count_if(pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), moved) != 2) {
        return;
      }
      bulkDeleteTasks(QVariantList{state->bulkFirstTaskId, state->bulkSecondTaskId});
      state->stage = 34;
      return;
    }
    case 34: {
      const auto deleted = [state](const TaskModelTask& task) {
        return task.id == state->bulkFirstTaskId || task.id == state->bulkSecondTaskId;
      };
      if (std::find_if(pythonBridgeTasks_.cbegin(), pythonBridgeTasks_.cend(), deleted) !=
          pythonBridgeTasks_.cend()) {
        return;
      }
      const auto conflict = std::find_if(
          unresolvedConflicts_.cbegin(), unresolvedConflicts_.cend(), [](const QVariant& row) {
            const QVariantMap values = row.toMap();
            return values.value(QStringLiteral("canKeepRemote")).toBool();
          });
      if (conflict == unresolvedConflicts_.cend()) {
        return;
      }
      state->conflictId = conflict->toMap().value(QStringLiteral("id")).toString();
      resolveSyncConflict(state->conflictId, false);
      state->stage = 35;
      return;
    }
    case 35: {
      const bool remainsOpen = std::any_of(
          unresolvedConflicts_.cbegin(), unresolvedConflicts_.cend(), [state](const QVariant& row) {
            return row.toMap().value(QStringLiteral("id")).toString() == state->conflictId;
          });
      if (remainsOpen) {
        return;
      }
      saveSearch(QString::fromLatin1(kBridgeAcceptanceSavedSearchName),
                 QString::fromLatin1(kBridgeAcceptanceSavedSearchQuery));
      state->stage = 36;
      return;
    }
    case 36: {
      const auto saved = std::find_if(
          savedSearches_.cbegin(), savedSearches_.cend(), [](const SavedSearch& search) {
            return search.name == QString::fromLatin1(kBridgeAcceptanceSavedSearchName) &&
                   search.query == QString::fromLatin1(kBridgeAcceptanceSavedSearchQuery);
          });
      if (saved == savedSearches_.cend()) {
        return;
      }
      state->savedSearchId = saved->id;
      applySavedSearch(state->savedSearchId);
      state->stage = 37;
      return;
    }
    case 37:
      if (searchQuery_ != QString::fromLatin1(kBridgeAcceptanceSavedSearchQuery) ||
          countSearchTasks() == 0) {
        return;
      }
      renameSavedSearch(state->savedSearchId,
                        QString::fromLatin1(kBridgeAcceptanceRenamedSavedSearchName));
      state->stage = 38;
      return;
    case 38: {
      const auto renamed = std::find_if(
          savedSearches_.cbegin(), savedSearches_.cend(), [state](const SavedSearch& search) {
            return search.id == state->savedSearchId &&
                   search.name == QString::fromLatin1(kBridgeAcceptanceRenamedSavedSearchName) &&
                   search.query == QString::fromLatin1(kBridgeAcceptanceSavedSearchQuery);
          });
      if (renamed == savedSearches_.cend()) {
        return;
      }
      saveSearch(QString::fromLatin1(kBridgeAcceptanceDeletedSavedSearchName),
                 QString::fromLatin1(kBridgeAcceptanceDeletedSavedSearchQuery));
      state->stage = 39;
      return;
    }
    case 39: {
      const auto deleted = std::find_if(
          savedSearches_.cbegin(), savedSearches_.cend(), [](const SavedSearch& search) {
            return search.name == QString::fromLatin1(kBridgeAcceptanceDeletedSavedSearchName) &&
                   search.query == QString::fromLatin1(kBridgeAcceptanceDeletedSavedSearchQuery);
          });
      if (deleted == savedSearches_.cend()) {
        return;
      }
      state->deletedSavedSearchId = deleted->id;
      deleteSavedSearch(state->deletedSavedSearchId);
      state->stage = 40;
      return;
    }
    case 40: {
      const bool retained = std::any_of(
          savedSearches_.cbegin(), savedSearches_.cend(), [state](const SavedSearch& search) {
            return search.id == state->savedSearchId &&
                   search.name == QString::fromLatin1(kBridgeAcceptanceRenamedSavedSearchName) &&
                   search.query == QString::fromLatin1(kBridgeAcceptanceSavedSearchQuery);
          });
      const bool deleted = std::none_of(
          savedSearches_.cbegin(), savedSearches_.cend(), [state](const SavedSearch& search) {
            return search.id == state->deletedSavedSearchId;
          });
      if (!retained || !deleted) {
        return;
      }
      finish(true);
      return;
    }
    default:
      finish(false, QStringLiteral("invalid Qt bridge interaction stage"));
      return;
    }
  });
  timer->start();
}

void AppController::runBridgeFailureAcceptance(QString reportPath) {
  struct State final {
    QString reportPath;
    int stage{0};
    qsizetype initialTasks{0};
    qsizetype initialTaskLists{0};
    qsizetype initialCalendars{0};
    bool pollingWasCancellable{false};
    bool cancellationCompleted{false};
    bool mutationFailuresPreservedModels{false};
    bool malformedOperationRejected{false};
    bool malformedStartRejected{false};
    bool shutdownWithInFlightPoll{false};
    QElapsedTimer elapsed;
  };

  const auto state = std::make_shared<State>();
  state->reportPath = std::move(reportPath);
  state->initialTasks = pythonBridgeTasks_.size();
  state->initialTaskLists = pythonBridgeTaskLists_.size();
  state->initialCalendars = pythonBridgeCalendars_.size();
  state->elapsed.start();

  auto* timer = new QTimer(this);
  timer->setInterval(20);
  const auto finish = [this, timer, state](bool success, QString error = {}) {
    QJsonObject payload{
        {QStringLiteral("ok"), success},
        {QStringLiteral("stage"), state->stage},
        {QStringLiteral("polling_was_cancellable"), state->pollingWasCancellable},
        {QStringLiteral("cancellation_completed"), state->cancellationCompleted},
        {QStringLiteral("mutation_failures_preserved_models"),
         state->mutationFailuresPreservedModels},
        {QStringLiteral("malformed_operation_rejected"), state->malformedOperationRejected},
        {QStringLiteral("malformed_start_rejected"), state->malformedStartRejected},
        {QStringLiteral("shutdown_with_in_flight_poll"), state->shutdownWithInFlightPoll}};
    if (!error.isEmpty()) {
      payload.insert(QStringLiteral("error"), std::move(error));
      payload.insert(QStringLiteral("status"), statusMessage_);
      payload.insert(QStringLiteral("sync_status"), syncStatus_);
    }
    QSaveFile report(state->reportPath);
    const QByteArray encoded = QJsonDocument(payload).toJson(QJsonDocument::Compact);
    const bool wrote = report.open(QIODevice::WriteOnly) &&
                       report.write(encoded) == encoded.size() && report.commit();
    timer->stop();
    timer->deleteLater();
    QCoreApplication::exit(success && wrote ? 0 : 3);
  };

  connect(timer, &QTimer::timeout, this, [this, state, finish] {
    const auto modelsUnchanged = [this, state] {
      return pythonBridgeTasks_.size() == state->initialTasks &&
             pythonBridgeTaskLists_.size() == state->initialTaskLists &&
             pythonBridgeCalendars_.size() == state->initialCalendars;
    };
    if (state->elapsed.elapsed() > 20'000) {
      finish(false, QStringLiteral("timed out waiting for Qt bridge failure stage"));
      return;
    }

    switch (state->stage) {
    case 0:
      syncGoogle();
      state->stage = 1;
      return;
    case 1:
      if (!bridgeOperationActive() || busy_) {
        return;
      }
      state->pollingWasCancellable = true;
      syncGoogle();
      state->stage = 2;
      return;
    case 2:
      if (bridgeOperationActive()) {
        return;
      }
      if (syncStatus_ != QStringLiteral("HCB sync cancelled")) {
        finish(false, QStringLiteral("cancelled bridge operation did not report cancellation"));
        return;
      }
      state->cancellationCompleted = true;
      createTaskDetailed(QStringLiteral("inbox"),
                         {},
                         QStringLiteral("rejected bridge task"),
                         {},
                         {},
                         QStringLiteral("UTC"),
                         0,
                         false,
                         0,
                         1,
                         0,
                         {},
                         0);
      state->stage = 3;
      return;
    case 3:
      if (busy_) {
        return;
      }
      if (statusMessage_ != QStringLiteral("synthetic task rejection") || !modelsUnchanged()) {
        finish(false, QStringLiteral("rejected task mutation changed the bridge models"));
        return;
      }
      createTaskList(QStringLiteral("invalid bridge task list"));
      state->stage = 4;
      return;
    case 4:
      if (busy_) {
        return;
      }
      if (statusMessage_ != QStringLiteral("HCB bridge mutation returned an invalid task list") ||
          taskListErrorMessage_ != statusMessage_ || !modelsUnchanged()) {
        finish(false, QStringLiteral("invalid task-list mutation was not retained as an error"));
        return;
      }
      createGoogleCalendar(QStringLiteral("rejected bridge calendar"), {}, QStringLiteral("UTC"));
      state->stage = 5;
      return;
    case 5:
      if (busy_) {
        return;
      }
      if (statusMessage_ != QStringLiteral("synthetic calendar rejection") || !modelsUnchanged()) {
        finish(false, QStringLiteral("rejected calendar mutation changed the bridge models"));
        return;
      }
      state->mutationFailuresPreservedModels = true;
      syncGoogle();
      state->stage = 6;
      return;
    case 6:
      if (!bridgeOperationActive() || busy_) {
        return;
      }
      state->stage = 7;
      return;
    case 7:
      if (bridgeOperationActive()) {
        return;
      }
      if (statusMessage_ != QStringLiteral("HCB bridge returned an invalid operation response") ||
          syncStatus_ != QStringLiteral("HCB sync failed")) {
        finish(false, QStringLiteral("mismatched bridge operation response was accepted"));
        return;
      }
      state->malformedOperationRejected = true;
      syncGoogle();
      state->stage = 8;
      return;
    case 8:
      if (bridgeOperationActive() || busy_) {
        return;
      }
      if (statusMessage_ != QStringLiteral("HCB bridge returned an invalid operation") ||
          syncStatus_ != QStringLiteral("HCB sync failed")) {
        finish(false, QStringLiteral("malformed bridge operation start was accepted"));
        return;
      }
      state->malformedStartRejected = true;
      syncGoogle();
      state->stage = 9;
      return;
    case 9:
      if (!bridgeOperationActive() || busy_) {
        return;
      }
      state->shutdownWithInFlightPoll = true;
      finish(true);
      return;
    default:
      finish(false, QStringLiteral("invalid Qt bridge failure stage"));
      return;
    }
  });
  timer->start();
}

void AppController::setReminderService(ReminderService* service) {
  if (reminderService_ == service) {
    return;
  }
  if (reminderService_ != nullptr) {
    disconnect(reminderService_, nullptr, this, nullptr);
  }
  reminderService_ = service;
  if (reminderService_ == nullptr) {
    setReminderStatusMessage(QStringLiteral("Calendar reminders are unavailable"));
    return;
  }
  connect(reminderService_, &ReminderService::statusMessageChanged, this, [this] {
    setReminderStatusMessage(reminderService_->statusMessage());
  });
  setReminderStatusMessage(reminderService_->statusMessage());
}

void AppController::setPlatformReminderStatus(QString message) {
  setReminderStatusMessage(std::move(message));
}

void AppController::refresh() {
  if (bridgeMode()) {
    refreshBridge();
    return;
  }
  refreshTasks();
  refreshCalendar();
}

void AppController::setCalendarDate(QString date) {
  const QDate parsed = QDate::fromString(date, Qt::ISODate);
  if (!parsed.isValid()) {
    setStatus(QStringLiteral("Calendar date is invalid"));
    return;
  }
  if (calendarDate_ == parsed) {
    return;
  }
  calendarDate_ = parsed;
  emit calendarDateChanged();
  emit calendarLabelsChanged();
  if (bridgeMode()) {
    refreshBridge();
    return;
  }
  refreshCalendar();
}

void AppController::saveAppearanceMode(int mode) {
  if (!isValidAppearanceMode(mode)) {
    setStatus(QStringLiteral("Appearance mode is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kAppearanceModeSettingsKey),
                                   QString::number(mode)),
        [this, mode](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (appearanceMode_ != mode) {
            appearanceMode_ = mode;
            emit appearanceModeChanged();
          }
        });
}

void AppController::saveVisualDensity(int density) {
  if (!isValidVisualDensity(density)) {
    setStatus(QStringLiteral("Visual density is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kVisualDensitySettingsKey),
                                   QString::number(density)),
        [this, density](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (visualDensity_ != density) {
            visualDensity_ = density;
            emit visualDensityChanged();
          }
        });
}

void AppController::saveTaskListPaneWidth(int width) {
  width = std::clamp(width, kMinimumTaskListPaneWidth, kMaximumTaskListPaneWidth);
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kTaskListPaneWidthSettingsKey),
                                   QString::number(width)),
        [this, width](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (taskListPaneWidth_ != width) {
            taskListPaneWidth_ = width;
            emit taskListPaneWidthChanged();
          }
        });
}

void AppController::savePaletteMode(int mode) {
  if (!isValidPaletteMode(mode)) {
    setStatus(QStringLiteral("Palette is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kPaletteModeSettingsKey),
                                   QString::number(mode)),
        [this, mode](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (paletteMode_ != mode) {
            paletteMode_ = mode;
            emit paletteModeChanged();
          }
        });
}

void AppController::saveAccentColor(QString color) {
  color = color.trimmed().toUpper();
  if (!isValidAccentColor(color)) {
    setStatus(QStringLiteral("Accent color must be #RRGGBB"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kAccentColorSettingsKey),
                                   jsonStringArray(color)),
        [this, color = std::move(color)](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (accentColor_ != color) {
            accentColor_ = color;
            emit accentColorChanged();
          }
        });
}

void AppController::saveFontFamily(QString family) {
  family = family.trimmed();
  if (!family.isEmpty() &&
      (!installedFontFamilies().contains(family) || !isUsableTextFontFamily(family))) {
    setStatus(QStringLiteral("Font family is unavailable"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kFontFamilySettingsKey),
                                   jsonStringArray(family)),
        [this, family = std::move(family)](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (fontFamily_ != family) {
            fontFamily_ = family;
            emit fontFamilyChanged();
          }
        });
}

void AppController::saveFontScale(int scale) {
  if (!isValidFontScale(scale)) {
    setStatus(QStringLiteral("Font scale is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kFontScaleSettingsKey),
                                   QString::number(scale)),
        [this, scale](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (fontScale_ != scale) {
            fontScale_ = scale;
            emit fontScaleChanged();
          }
        });
}

void AppController::saveBulkTextRecurrenceScope(int scope) {
  if (!isValidBulkTextRecurrenceScope(scope)) {
    setStatus(QStringLiteral("Bulk text recurrence scope is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kBulkTextRecurrenceScopeSettingsKey),
                                   QString::number(scope)),
        [this, scope](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (bulkTextRecurrenceScope_ != scope) {
            bulkTextRecurrenceScope_ = scope;
            emit bulkTextRecurrenceScopeChanged();
          }
        });
}

void AppController::saveExternalBrowser(QString browser) {
  browser = browser.trimmed();
  if (!isValidExternalBrowser(browser)) {
    setStatus(QStringLiteral("External browser is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kExternalBrowserSettingsKey),
                                   jsonStringArray(browser)),
        [this, browser = std::move(browser)](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (externalBrowser_ != browser) {
            externalBrowser_ = browser;
            emit externalBrowserChanged();
          }
        });
}

void AppController::openExternalLink(QString value) {
  const QUrl url = QUrl::fromUserInput(value.trimmed());
  if (!url.isValid() || url.host().isEmpty() ||
      (url.scheme() != QStringLiteral("https") && url.scheme() != QStringLiteral("http"))) {
    setStatus(QStringLiteral("Only valid HTTP and HTTPS links can be opened"));
    return;
  }
  if (!openExternalUrl(url, externalBrowser_)) {
    setStatus(externalBrowser_.isEmpty()
                  ? QStringLiteral("System browser could not be opened")
                  : QStringLiteral("Configured browser could not be opened"));
  }
}

void AppController::resetVisualPreferences() {
  saveAppearanceMode(0);
  saveVisualDensity(1);
  savePaletteMode(0);
  saveAccentColor({});
  saveFontFamily({});
  saveFontScale(1);
}

QuickCaptureAliases AppController::quickCaptureAliasesConfiguration() const {
  return {.task = quickCaptureTaskAliases_,
          .event = quickCaptureEventAliases_,
          .highPriority = quickCaptureHighPriorityAliases_,
          .mediumPriority = quickCaptureMediumPriorityAliases_,
          .lowPriority = quickCaptureLowPriorityAliases_};
}

QuickCaptureParseResult AppController::quickCaptureParse(
    QString text, int kind, QVariantList disabledRecognitionIds) const {
  QStringList disabled;
  disabled.reserve(disabledRecognitionIds.size());
  for (const QVariant& value : disabledRecognitionIds) {
    const QString id = value.toString();
    if (!id.isEmpty() && id.size() <= 128) {
      disabled.append(id);
    }
  }
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock_.wallNow().time_since_epoch());
  return QuickCaptureParser::parse(
      {.text = std::move(text),
       .kind = isValidQuickCaptureKind(kind) ? static_cast<QuickCaptureKind>(kind)
                                             : QuickCaptureKind::Event,
       .now = QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC),
       .timeZone = resolvedTimeZone(displayTimeZone_),
       .defaultEventDurationMinutes = quickCaptureEventDurationMinutes_,
       .aliases = quickCaptureAliasesConfiguration(),
       .disabledRecognitionIds = std::move(disabled)});
}

QVariantMap AppController::previewQuickCapture(QString text,
                                               int kind,
                                               QVariantList disabledRecognitionIds) const {
  QVariantMap preview;
  if (!isValidQuickCaptureKind(kind)) {
    preview.insert(QStringLiteral("error"), QStringLiteral("Quick capture type is invalid"));
    return preview;
  }
  const QuickCaptureParseResult parsed =
      quickCaptureParse(std::move(text), kind, std::move(disabledRecognitionIds));
  QVariantList recognitions;
  recognitions.reserve(parsed.recognitions.size());
  for (const QuickCaptureRecognition& recognition : parsed.recognitions) {
    recognitions.append(QVariantMap{{QStringLiteral("id"), recognition.id},
                                    {QStringLiteral("label"), recognition.label},
                                    {QStringLiteral("removable"), recognition.removable}});
  }
  preview.insert(QStringLiteral("kind"), static_cast<int>(parsed.kind));
  preview.insert(QStringLiteral("rawTitle"), parsed.rawTitle);
  preview.insert(QStringLiteral("parsedTitle"), parsed.parsedTitle);
  preview.insert(QStringLiteral("savedTitle"),
                 quickCaptureRemoveParsedText_ ? parsed.parsedTitle : parsed.rawTitle);
  preview.insert(QStringLiteral("date"),
                 parsed.date.has_value() ? parsed.date->toString(Qt::ISODate) : QString());
  preview.insert(QStringLiteral("time"),
                 parsed.time.has_value() ? parsed.time->toString(QStringLiteral("HH:mm"))
                                         : QString());
  preview.insert(QStringLiteral("allDay"), parsed.allDay);
  preview.insert(QStringLiteral("eventDurationMinutes"), parsed.eventDurationMinutes);
  preview.insert(QStringLiteral("taskPriority"), parsed.taskPriority);
  preview.insert(QStringLiteral("recurrenceEnabled"), parsed.recurrence.enabled);
  preview.insert(QStringLiteral("recurrenceFrequency"), parsed.recurrence.frequency);
  preview.insert(QStringLiteral("recurrenceInterval"), parsed.recurrence.interval);
  preview.insert(QStringLiteral("recurrenceRule"), parsed.recurrence.rrule);
  preview.insert(QStringLiteral("eventReady"), parsed.eventReady);
  preview.insert(QStringLiteral("recognitions"), std::move(recognitions));
  return preview;
}

void AppController::createQuickCapture(QString text,
                                       int kind,
                                       QString destinationId,
                                       QVariantList disabledRecognitionIds) {
  if (!isValidQuickCaptureKind(kind)) {
    setStatus(QStringLiteral("Quick capture type is invalid"));
    return;
  }
  destinationId = destinationId.trimmed();
  if (destinationId.isEmpty()) {
    setStatus(QStringLiteral("Quick capture needs a destination"));
    return;
  }
  const QuickCaptureParseResult parsed =
      quickCaptureParse(std::move(text), kind, std::move(disabledRecognitionIds));
  const QString title =
      (quickCaptureRemoveParsedText_ ? parsed.parsedTitle : parsed.rawTitle).trimmed();
  if (title.isEmpty()) {
    setStatus(QStringLiteral("Quick capture needs a title after parsing"));
    return;
  }
  if (parsed.kind == QuickCaptureKind::Task) {
    const QVariantList selectedLists = taskListModel_.selectedTaskLists();
    const bool exists = std::any_of(
        selectedLists.cbegin(), selectedLists.cend(), [&destinationId](const QVariant& row) {
          return row.toMap().value(QStringLiteral("id")).toString() == destinationId;
        });
    if (!exists) {
      setStatus(QStringLiteral("Choose an active Google Task list for Quick Capture"));
      return;
    }
    createTaskDetailed(destinationId,
                       {},
                       title,
                       {},
                       parsed.date.has_value() ? parsed.date->toString(Qt::ISODate) : QString(),
                       displayTimeZone_,
                       parsed.taskPriority,
                       parsed.recurrence.enabled,
                       parsed.recurrence.frequency,
                       parsed.recurrence.interval,
                       0,
                       {},
                       1,
                       parsed.recurrence.rrule,
                       {},
                       {});
    return;
  }

  if (!calendarSourceModel_.calendarIds().contains(destinationId)) {
    setStatus(QStringLiteral("Choose an available Google Calendar for Quick Capture"));
    return;
  }
  if (!parsed.eventReady || !parsed.date.has_value()) {
    setStatus(QStringLiteral("Events need a date or time; switch to Task for an unscheduled item"));
    return;
  }
  const QTimeZone timeZone = resolvedTimeZone(displayTimeZone_);
  const QDateTime start = QDateTime(
      *parsed.date, parsed.allDay ? QTime(0, 0) : parsed.time.value_or(QTime(0, 0)), timeZone);
  const QDateTime end =
      parsed.allDay ? start.addDays(1) : start.addSecs(parsed.eventDurationMinutes * 60);
  if (!start.isValid() || !end.isValid() || end <= start) {
    setStatus(QStringLiteral("Quick capture event time is invalid in the selected time zone"));
    return;
  }
  createEventDetailed(destinationId,
                      title,
                      start.toUTC().toString(Qt::ISODateWithMs),
                      end.toUTC().toString(Qt::ISODateWithMs),
                      parsed.allDay,
                      {},
                      {},
                      QString::fromUtf8(timeZone.id()),
                      {},
                      false,
                      QStringLiteral("default"),
                      {},
                      true,
                      {},
                      parsed.recurrence.enabled ? parsed.recurrence.rrule : QString(),
                      false,
                      QStringLiteral("[]"),
                      QStringLiteral("{}"),
                      QStringLiteral("default"),
                      QStringLiteral("{}"),
                      QStringLiteral("all"));
}

void AppController::saveQuickCaptureDefaultTaskListId(QString taskListId) {
  taskListId = taskListId.trimmed();
  if (!isValidQuickCaptureDestination(taskListId)) {
    setStatus(QStringLiteral("Quick capture task list is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kQuickCaptureDefaultTaskListSettingsKey),
                                   jsonStringArray(taskListId)),
        [this, taskListId = std::move(taskListId)](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (quickCaptureDefaultTaskListId_ != taskListId) {
            quickCaptureDefaultTaskListId_ = taskListId;
            emit quickCaptureDefaultTaskListIdChanged();
          }
        });
}

void AppController::saveQuickCaptureDefaultCalendarId(QString calendarId) {
  calendarId = calendarId.trimmed();
  if (!isValidQuickCaptureDestination(calendarId)) {
    setStatus(QStringLiteral("Quick capture calendar is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kQuickCaptureDefaultCalendarSettingsKey),
                                   jsonStringArray(calendarId)),
        [this, calendarId = std::move(calendarId)](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (quickCaptureDefaultCalendarId_ != calendarId) {
            quickCaptureDefaultCalendarId_ = calendarId;
            emit quickCaptureDefaultCalendarIdChanged();
          }
        });
}

void AppController::saveQuickCaptureEventDurationMinutes(int minutes) {
  if (!isValidQuickCaptureDuration(minutes)) {
    setStatus(QStringLiteral("Quick capture event duration must be between 1 and 1440 minutes"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kQuickCaptureEventDurationSettingsKey),
                                   QString::number(minutes)),
        [this, minutes](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (quickCaptureEventDurationMinutes_ != minutes) {
            quickCaptureEventDurationMinutes_ = minutes;
            emit quickCaptureEventDurationMinutesChanged();
          }
        });
}

void AppController::saveQuickCaptureRemoveParsedText(bool enabled) {
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kQuickCaptureRemoveParsedTextSettingsKey),
                                   enabled ? QStringLiteral("true") : QStringLiteral("false")),
        [this, enabled](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (quickCaptureRemoveParsedText_ != enabled) {
            quickCaptureRemoveParsedText_ = enabled;
            emit quickCaptureRemoveParsedTextChanged();
          }
        });
}

void AppController::saveQuickCaptureAliases(QString taskAliases,
                                            QString eventAliases,
                                            QString highPriorityAliases,
                                            QString mediumPriorityAliases,
                                            QString lowPriorityAliases) {
  const std::optional<QStringList> task = quickCaptureAliasesFromText(taskAliases);
  const std::optional<QStringList> event = quickCaptureAliasesFromText(eventAliases);
  const std::optional<QStringList> high = quickCaptureAliasesFromText(highPriorityAliases);
  const std::optional<QStringList> medium = quickCaptureAliasesFromText(mediumPriorityAliases);
  const std::optional<QStringList> low = quickCaptureAliasesFromText(lowPriorityAliases);
  if (!task.has_value() || !event.has_value() || !high.has_value() || !medium.has_value() ||
      !low.has_value() || !quickCaptureAliasesAreDistinct(*task, *event, *high, *medium, *low)) {
    setStatus(QStringLiteral("Quick capture aliases must be unique words of up to 32 characters"));
    return;
  }
  const auto save =
      [this](const char* key, QStringList values, QStringList AppController::* property) {
        watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                         QString::fromLatin1(key),
                                         jsonStringList(values)),
              [this, values = std::move(values), property](SettingsMutationResultOrError result) {
                if (std::holds_alternative<AppError>(result)) {
                  setStatus(errorMessage(std::get<AppError>(result)));
                } else if ((this->*property) != values) {
                  (this->*property) = values;
                  emit quickCaptureAliasesChanged();
                }
              });
      };
  save(kQuickCaptureTaskAliasesSettingsKey, *task, &AppController::quickCaptureTaskAliases_);
  save(kQuickCaptureEventAliasesSettingsKey, *event, &AppController::quickCaptureEventAliases_);
  save(kQuickCaptureHighPriorityAliasesSettingsKey,
       *high,
       &AppController::quickCaptureHighPriorityAliases_);
  save(kQuickCaptureMediumPriorityAliasesSettingsKey,
       *medium,
       &AppController::quickCaptureMediumPriorityAliases_);
  save(kQuickCaptureLowPriorityAliasesSettingsKey,
       *low,
       &AppController::quickCaptureLowPriorityAliases_);
}

void AppController::saveWeekStartDay(int day) {
  if (!isValidWeekStartDay(day)) {
    setStatus(QStringLiteral("Week start is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kWeekStartDaySettingsKey),
                                   QString::number(day)),
        [this, day](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (weekStartDay_ != day) {
            weekStartDay_ = day;
            emit weekStartDayChanged();
            emit calendarLabelsChanged();
            refreshCalendar();
          }
        });
}

void AppController::saveSidebarTabIds(QVariantList ids) {
  QStringList parsed;
  QSet<QString> seen;
  parsed.reserve(ids.size());
  for (const QVariant& value : ids) {
    if (value.metaType().id() != QMetaType::QString || !isValidSidebarTabId(value.toString()) ||
        seen.contains(value.toString())) {
      setStatus(QStringLiteral("Sidebar tabs are invalid"));
      return;
    }
    seen.insert(value.toString());
    parsed.append(value.toString());
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kSidebarTabIdsSettingsKey),
                                   jsonStringList(parsed)),
        [this, parsed = std::move(parsed)](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (sidebarTabIds_ != parsed) {
            sidebarTabIds_ = parsed;
            emit sidebarTabIdsChanged();
          }
        });
}

void AppController::saveUse24HourTime(bool enabled) {
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kUse24HourTimeSettingsKey),
                                   enabled ? QStringLiteral("true") : QStringLiteral("false")),
        [this, enabled](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (use24HourTime_ != enabled) {
            use24HourTime_ = enabled;
            emit use24HourTimeChanged();
          }
        });
}

void AppController::saveDisplayTimeZone(QString timeZone) {
  timeZone = timeZone.trimmed();
  if (!QTimeZone(timeZone.toUtf8()).isValid()) {
    setStatus(QStringLiteral("Display time zone is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kDisplayTimeZoneSettingsKey),
                                   jsonStringArray(timeZone)),
        [this, timeZone = std::move(timeZone)](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (displayTimeZone_ != timeZone) {
            displayTimeZone_ = timeZone;
            emit displayTimeZoneChanged();
            refreshCalendar();
          }
        });
}

QVariantMap AppController::dateTimeComponents(QString value, QString timeZone) const {
  const QDateTime parsed = QDateTime::fromString(value, Qt::ISODate);
  if (!parsed.isValid()) {
    return {};
  }
  const QDateTime local = parsed.toTimeZone(resolvedTimeZone(timeZone));
  return {{QStringLiteral("year"), local.date().year()},
          {QStringLiteral("month"), local.date().month()},
          {QStringLiteral("day"), local.date().day()},
          {QStringLiteral("hour"), local.time().hour()},
          {QStringLiteral("minute"), local.time().minute()}};
}

QString AppController::dateTimeFromComponents(
    int year, int month, int day, int hour, int minute, QString timeZone) const {
  const QDate date(year, month, day);
  const QTime time(hour, minute);
  const QDateTime local(date, time, resolvedTimeZone(timeZone));
  return local.isValid() ? local.toUTC().toString(Qt::ISODateWithMs) : QString();
}

void AppController::saveWorkdayHours(int startHour, int endHour) {
  if (!isValidWorkdayHours(startHour, endHour)) {
    setStatus(QStringLiteral("Workday hours are invalid"));
    return;
  }
  const QString start = QString::number(startHour);
  const QString end = QString::number(endHour);
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kWorkdayStartHourSettingsKey),
                                   start),
        [this, startHour](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (workdayStartHour_ != startHour) {
            workdayStartHour_ = startHour;
            emit workdayStartHourChanged();
          }
        });
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kWorkdayEndHourSettingsKey),
                                   end),
        [this, endHour](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (workdayEndHour_ != endHour) {
            workdayEndHour_ = endHour;
            emit workdayEndHourChanged();
          }
        });
}

void AppController::saveCalendarVisibility(QVariantList calendarIds) {
  if (calendarIds.size() > 20'000) {
    setStatus(QStringLiteral("Calendar visibility is invalid"));
    return;
  }
  QJsonArray stored;
  QVariantList values;
  QSet<QString> seen;
  for (const QVariant& value : calendarIds) {
    if (!value.canConvert<QString>()) {
      setStatus(QStringLiteral("Calendar visibility is invalid"));
      return;
    }
    const QString id = value.toString();
    if (id.isEmpty() || id.size() > 256 || id != id.trimmed() || id.contains(QChar::Null) ||
        seen.contains(id)) {
      setStatus(QStringLiteral("Calendar visibility is invalid"));
      return;
    }
    seen.insert(id);
    stored.append(id);
    values.append(id);
  }
  const QString json = QString::fromUtf8(QJsonDocument(stored).toJson(QJsonDocument::Compact));
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kCalendarVisibilitySettingsKey),
                                   json),
        [this, values = std::move(values)](SettingsMutationResultOrError result) mutable {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          if (visibleCalendarIds_ != values) {
            visibleCalendarIds_ = std::move(values);
            emit visibleCalendarIdsChanged();
          }
          if (!calendarVisibilityConfigured_) {
            calendarVisibilityConfigured_ = true;
            emit calendarVisibilityConfiguredChanged();
          }
        });
}

void AppController::createGoogleCalendar(QString title, QString description, QString timeZone) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || title.trimmed().isEmpty()) {
      setStatus(QStringLiteral("Calendar title is required"));
      return;
    }
    const QString trimmedTimeZone = timeZone.trimmed();
    watch(pythonBridgeClient_->createCalendar(
              pythonBridgeAccountId_,
              QJsonObject{
                  {QStringLiteral("summary"), title.trimmed()},
                  {QStringLiteral("description"),
                   description.trimmed().isEmpty() ? QJsonValue::Null : QJsonValue(description)},
                  {QStringLiteral("time_zone"),
                   trimmedTimeZone.isEmpty() ? QJsonValue::Null : QJsonValue(trimmedTimeZone)}},
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeCalendarResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Calendar saved in HCB core"));
            }
          });
    return;
  }
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before creating a calendar"));
    return;
  }
  const GoogleCalendarCreateRequest request{
      .title = std::move(title),
      .description = description.trimmed().isEmpty()
                         ? std::optional<QString>{}
                         : std::optional<QString>(std::move(description)),
      .timeZone = timeZone.trimmed().isEmpty() ? std::optional<QString>{}
                                               : std::optional<QString>(std::move(timeZone))};
  watch(std::async(
            std::launch::async,
            [this, request]() -> std::variant<GoogleCalendarManagementResult, AppError> {
              OAuthCredentialReadResult read =
                  credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
              if (std::holds_alternative<AppError>(read)) {
                return std::get<AppError>(std::move(read));
              }
              const std::optional<OAuthStoredCredential>& credential =
                  std::get<std::optional<OAuthStoredCredential>>(read);
              if (!credential.has_value() || credential->accessToken.isEmpty()) {
                return AppError(AppErrorCode::Configuration,
                                QStringLiteral("Google authorization must be renewed"));
              }
              GoogleCalendarManagementResultOrError created =
                  googleCalendarManagementClient_.create(request, credential->accessToken).get();
              return std::holds_alternative<GoogleApiError>(created)
                         ? std::variant<GoogleCalendarManagementResult, AppError>(
                               AppError(AppErrorCode::Network,
                                        std::get<GoogleApiError>(std::move(created)).message()))
                         : std::variant<GoogleCalendarManagementResult, AppError>(
                               std::get<GoogleCalendarManagementResult>(std::move(created)));
            }),
        [this](std::variant<GoogleCalendarManagementResult, AppError> result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          setStatus(QStringLiteral("Google calendar created"));
          requestGoogleSync(SyncScheduleTrigger::Manual);
        });
}

void AppController::subscribeGoogleCalendar(QString calendarId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || calendarId.trimmed().isEmpty()) {
      setStatus(QStringLiteral("Calendar ID is required"));
      return;
    }
    watch(pythonBridgeClient_->subscribeCalendar(
              pythonBridgeAccountId_,
              QJsonObject{{QStringLiteral("remote_calendar_id"), calendarId.trimmed()}},
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeCalendarResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Calendar subscription queued in HCB core"));
            }
          });
    return;
  }
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before subscribing to a calendar"));
    return;
  }
  const GoogleCalendarSubscribeRequest request{.calendarId = std::move(calendarId)};
  watch(std::async(
            std::launch::async,
            [this, request]() -> std::variant<GoogleCalendarManagementResult, AppError> {
              OAuthCredentialReadResult read =
                  credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
              if (std::holds_alternative<AppError>(read)) {
                return std::get<AppError>(std::move(read));
              }
              const std::optional<OAuthStoredCredential>& credential =
                  std::get<std::optional<OAuthStoredCredential>>(read);
              if (!credential.has_value() || credential->accessToken.isEmpty()) {
                return AppError(AppErrorCode::Configuration,
                                QStringLiteral("Google authorization must be renewed"));
              }
              GoogleCalendarManagementResultOrError subscribed =
                  googleCalendarManagementClient_.subscribe(request, credential->accessToken).get();
              return std::holds_alternative<GoogleApiError>(subscribed)
                         ? std::variant<GoogleCalendarManagementResult, AppError>(
                               AppError(AppErrorCode::Network,
                                        std::get<GoogleApiError>(std::move(subscribed)).message()))
                         : std::variant<GoogleCalendarManagementResult, AppError>(
                               std::get<GoogleCalendarManagementResult>(std::move(subscribed)));
            }),
        [this](std::variant<GoogleCalendarManagementResult, AppError> result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          setStatus(QStringLiteral("Google calendar subscribed"));
          requestGoogleSync(SyncScheduleTrigger::Manual);
        });
}

void AppController::updateGoogleCalendar(QString calendarId,
                                         QString title,
                                         QString description,
                                         QString timeZone) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || calendarId.isEmpty() || title.trimmed().isEmpty()) {
      setStatus(QStringLiteral("Calendar title is required"));
      return;
    }
    const QString trimmedTimeZone = timeZone.trimmed();
    watch(pythonBridgeClient_->updateCalendar(
              pythonBridgeAccountId_,
              calendarId,
              QJsonObject{
                  {QStringLiteral("summary"), title.trimmed()},
                  {QStringLiteral("description"), QJsonValue(description)},
                  {QStringLiteral("time_zone"),
                   trimmedTimeZone.isEmpty() ? QJsonValue::Null : QJsonValue(trimmedTimeZone)}},
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeCalendarResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Calendar saved in HCB core"));
            }
          });
    return;
  }
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before editing a calendar"));
    return;
  }
  watch(
      calendarReadService_.findCalendar(std::move(calendarId)),
      [this,
       title = std::move(title),
       description = std::move(description),
       timeZone = std::move(timeZone)](CalendarLookupResult result) mutable {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(std::move(result))));
          return;
        }
        const std::optional<CalendarSummary>& calendar =
            std::get<std::optional<CalendarSummary>>(result);
        if (!calendar.has_value() || calendar->primary ||
            calendar->accessRole.value_or(QString()) != QStringLiteral("owner")) {
          setStatus(QStringLiteral("Only owned secondary calendars can be edited"));
          return;
        }
        const GoogleCalendarUpdateRequest request{
            .calendarId = calendar->remoteId,
            .title = std::move(title),
            .description = std::optional<QString>(std::move(description)),
            .timeZone = timeZone.trimmed().isEmpty() ? std::optional<QString>{}
                                                     : std::optional<QString>(std::move(timeZone))};
        watch(std::async(
                  std::launch::async,
                  [this, request]() -> std::variant<GoogleCalendarManagementResult, AppError> {
                    OAuthCredentialReadResult read =
                        credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
                    if (std::holds_alternative<AppError>(read)) {
                      return std::get<AppError>(std::move(read));
                    }
                    const std::optional<OAuthStoredCredential>& credential =
                        std::get<std::optional<OAuthStoredCredential>>(read);
                    if (!credential.has_value() || credential->accessToken.isEmpty()) {
                      return AppError(AppErrorCode::Configuration,
                                      QStringLiteral("Google authorization must be renewed"));
                    }
                    GoogleCalendarManagementResultOrError updated =
                        googleCalendarManagementClient_.update(request, credential->accessToken)
                            .get();
                    return std::holds_alternative<GoogleApiError>(updated)
                               ? std::variant<GoogleCalendarManagementResult, AppError>(AppError(
                                     AppErrorCode::Network,
                                     std::get<GoogleApiError>(std::move(updated)).message()))
                               : std::variant<GoogleCalendarManagementResult, AppError>(
                                     std::get<GoogleCalendarManagementResult>(std::move(updated)));
                  }),
              [this](std::variant<GoogleCalendarManagementResult, AppError> updated) {
                if (std::holds_alternative<AppError>(updated)) {
                  setStatus(errorMessage(std::get<AppError>(std::move(updated))));
                  return;
                }
                setStatus(QStringLiteral("Google calendar updated"));
                requestGoogleSync(SyncScheduleTrigger::Manual);
              });
      });
}

void AppController::deleteGoogleCalendar(QString calendarId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || calendarId.isEmpty()) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(pythonBridgeClient_->deleteCalendar(
              pythonBridgeAccountId_,
              calendarId,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeCalendarResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Calendar deleted in HCB core"));
            }
          });
    return;
  }
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before deleting a calendar"));
    return;
  }
  watch(calendarReadService_.findCalendar(std::move(calendarId)),
        [this](CalendarLookupResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          const std::optional<CalendarSummary>& calendar =
              std::get<std::optional<CalendarSummary>>(result);
          if (!calendar.has_value() || calendar->primary ||
              calendar->accessRole.value_or(QString()) != QStringLiteral("owner")) {
            setStatus(QStringLiteral("Only owned secondary calendars can be deleted"));
            return;
          }
          const QString remoteId = calendar->remoteId;
          watch(
              std::async(
                  std::launch::async,
                  [this, remoteId]() -> std::variant<GoogleCalendarManagementResult, AppError> {
                    OAuthCredentialReadResult read =
                        credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
                    if (std::holds_alternative<AppError>(read)) {
                      return std::get<AppError>(std::move(read));
                    }
                    const std::optional<OAuthStoredCredential>& credential =
                        std::get<std::optional<OAuthStoredCredential>>(read);
                    if (!credential.has_value() || credential->accessToken.isEmpty()) {
                      return AppError(AppErrorCode::Configuration,
                                      QStringLiteral("Google authorization must be renewed"));
                    }
                    GoogleCalendarManagementResultOrError removed =
                        googleCalendarManagementClient_.remove(remoteId, credential->accessToken)
                            .get();
                    return std::holds_alternative<GoogleApiError>(removed)
                               ? std::variant<GoogleCalendarManagementResult, AppError>(AppError(
                                     AppErrorCode::Network,
                                     std::get<GoogleApiError>(std::move(removed)).message()))
                               : std::variant<GoogleCalendarManagementResult, AppError>(
                                     std::get<GoogleCalendarManagementResult>(std::move(removed)));
                  }),
              [this](std::variant<GoogleCalendarManagementResult, AppError> removed) {
                if (std::holds_alternative<AppError>(removed)) {
                  setStatus(errorMessage(std::get<AppError>(std::move(removed))));
                  return;
                }
                setStatus(QStringLiteral("Google calendar deleted"));
                requestGoogleSync(SyncScheduleTrigger::Manual);
              });
        });
}

void AppController::updateGoogleCalendarListEntry(QString calendarId,
                                                  bool selected,
                                                  bool hidden,
                                                  QString colorId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || calendarId.isEmpty()) {
      reportBridgeUnsupportedAction();
      return;
    }
    const QString trimmedColor = colorId.trimmed();
    watch(pythonBridgeClient_->updateCalendar(
              pythonBridgeAccountId_,
              calendarId,
              QJsonObject{{QStringLiteral("selected"), selected},
                          {QStringLiteral("hidden"), hidden},
                          {QStringLiteral("color"),
                           trimmedColor.isEmpty() ? QJsonValue::Null : QJsonValue(trimmedColor)}},
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeCalendarResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Calendar preferences saved in HCB core"));
            }
          });
    return;
  }
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before editing calendar preferences"));
    return;
  }
  watch(
      calendarReadService_.findCalendar(std::move(calendarId)),
      [this, selected, hidden, colorId = std::move(colorId)](CalendarLookupResult result) mutable {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(std::move(result))));
          return;
        }
        const std::optional<CalendarSummary>& calendar =
            std::get<std::optional<CalendarSummary>>(result);
        if (!calendar.has_value()) {
          setStatus(QStringLiteral("Calendar is unavailable"));
          return;
        }
        const GoogleCalendarListUpdateRequest request{
            .calendarId = calendar->remoteId,
            .selected = selected,
            .hidden = hidden,
            .colorId = colorId.trimmed().isEmpty() ? std::optional<QString>{}
                                                   : std::optional<QString>(std::move(colorId))};
        watch(std::async(
                  std::launch::async,
                  [this, request]() -> std::variant<GoogleCalendarManagementResult, AppError> {
                    OAuthCredentialReadResult read =
                        credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
                    if (std::holds_alternative<AppError>(read)) {
                      return std::get<AppError>(std::move(read));
                    }
                    const std::optional<OAuthStoredCredential>& credential =
                        std::get<std::optional<OAuthStoredCredential>>(read);
                    if (!credential.has_value() || credential->accessToken.isEmpty()) {
                      return AppError(AppErrorCode::Configuration,
                                      QStringLiteral("Google authorization must be renewed"));
                    }
                    GoogleCalendarManagementResultOrError updated =
                        googleCalendarManagementClient_
                            .updateListEntry(request, credential->accessToken)
                            .get();
                    return std::holds_alternative<GoogleApiError>(updated)
                               ? std::variant<GoogleCalendarManagementResult, AppError>(AppError(
                                     AppErrorCode::Network,
                                     std::get<GoogleApiError>(std::move(updated)).message()))
                               : std::variant<GoogleCalendarManagementResult, AppError>(
                                     std::get<GoogleCalendarManagementResult>(std::move(updated)));
                  }),
              [this](std::variant<GoogleCalendarManagementResult, AppError> updated) {
                if (std::holds_alternative<AppError>(updated)) {
                  setStatus(errorMessage(std::get<AppError>(std::move(updated))));
                  return;
                }
                setStatus(QStringLiteral("Google calendar preferences updated"));
                requestGoogleSync(SyncScheduleTrigger::Manual);
              });
      });
}

void AppController::saveGoogleCalendarSettings(QString calendarId,
                                               QString title,
                                               QString description,
                                               QString timeZone,
                                               bool selected,
                                               bool hidden,
                                               QString colorId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || calendarId.isEmpty() || title.trimmed().isEmpty()) {
      setStatus(QStringLiteral("Calendar title is required"));
      return;
    }
    const QString trimmedTimeZone = timeZone.trimmed();
    const QString trimmedColor = colorId.trimmed();
    watch(pythonBridgeClient_->updateCalendar(
              pythonBridgeAccountId_,
              calendarId,
              QJsonObject{
                  {QStringLiteral("summary"), title.trimmed()},
                  {QStringLiteral("description"), QJsonValue(description)},
                  {QStringLiteral("time_zone"),
                   trimmedTimeZone.isEmpty() ? QJsonValue::Null : QJsonValue(trimmedTimeZone)},
                  {QStringLiteral("selected"), selected},
                  {QStringLiteral("hidden"), hidden},
                  {QStringLiteral("color"),
                   trimmedColor.isEmpty() ? QJsonValue::Null : QJsonValue(trimmedColor)}},
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeCalendarResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Calendar settings saved in HCB core"));
            }
          });
    return;
  }
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before editing calendar settings"));
    return;
  }
  watch(
      calendarReadService_.findCalendar(std::move(calendarId)),
      [this,
       title = std::move(title),
       description = std::move(description),
       timeZone = std::move(timeZone),
       selected,
       hidden,
       colorId = std::move(colorId)](CalendarLookupResult result) mutable {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(std::move(result))));
          return;
        }
        const std::optional<CalendarSummary>& calendar =
            std::get<std::optional<CalendarSummary>>(result);
        if (!calendar.has_value()) {
          setStatus(QStringLiteral("Calendar is unavailable"));
          return;
        }
        const bool updateDetails = !calendar->primary && calendar->accessRole.value_or(QString()) ==
                                                             QStringLiteral("owner");
        const GoogleCalendarUpdateRequest calendarRequest{
            .calendarId = calendar->remoteId,
            .title = std::move(title),
            .description = std::optional<QString>(std::move(description)),
            .timeZone = timeZone.trimmed().isEmpty() ? std::optional<QString>{}
                                                     : std::optional<QString>(std::move(timeZone))};
        const GoogleCalendarListUpdateRequest listRequest{
            .calendarId = calendar->remoteId,
            .selected = selected,
            .hidden = hidden,
            .colorId = colorId.trimmed().isEmpty() ? std::optional<QString>{}
                                                   : std::optional<QString>(std::move(colorId))};
        watch(std::async(
                  std::launch::async,
                  [this, updateDetails, calendarRequest, listRequest]()
                      -> std::variant<GoogleCalendarManagementResult, AppError> {
                    OAuthCredentialReadResult read =
                        credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
                    if (std::holds_alternative<AppError>(read)) {
                      return std::get<AppError>(std::move(read));
                    }
                    const std::optional<OAuthStoredCredential>& credential =
                        std::get<std::optional<OAuthStoredCredential>>(read);
                    if (!credential.has_value() || credential->accessToken.isEmpty()) {
                      return AppError(AppErrorCode::Configuration,
                                      QStringLiteral("Google authorization must be renewed"));
                    }
                    if (updateDetails) {
                      GoogleCalendarManagementResultOrError updated =
                          googleCalendarManagementClient_
                              .update(calendarRequest, credential->accessToken)
                              .get();
                      if (std::holds_alternative<GoogleApiError>(updated)) {
                        return AppError(AppErrorCode::Network,
                                        QStringLiteral("Google calendar details failed: ") +
                                            std::get<GoogleApiError>(std::move(updated)).message());
                      }
                    }
                    GoogleCalendarManagementResultOrError preferences =
                        googleCalendarManagementClient_
                            .updateListEntry(listRequest, credential->accessToken)
                            .get();
                    if (std::holds_alternative<GoogleApiError>(preferences)) {
                      return AppError(
                          AppErrorCode::Network,
                          (updateDetails ? QStringLiteral("Calendar details updated; Google "
                                                          "display preferences failed: ")
                                         : QStringLiteral("Google display preferences failed: ")) +
                              std::get<GoogleApiError>(std::move(preferences)).message());
                    }
                    return std::get<GoogleCalendarManagementResult>(std::move(preferences));
                  }),
              [this, updateDetails](std::variant<GoogleCalendarManagementResult, AppError> saved) {
                if (std::holds_alternative<AppError>(saved)) {
                  setStatus(errorMessage(std::get<AppError>(std::move(saved))));
                  requestGoogleSync(SyncScheduleTrigger::Manual);
                  return;
                }
                setStatus(updateDetails
                              ? QStringLiteral("Google calendar details and preferences updated")
                              : QStringLiteral("Google calendar preferences updated"));
                requestGoogleSync(SyncScheduleTrigger::Manual);
              });
      });
}

void AppController::unsubscribeGoogleCalendar(QString calendarId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || calendarId.isEmpty()) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(pythonBridgeClient_->unsubscribeCalendar(
              pythonBridgeAccountId_,
              calendarId,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeCalendarResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Calendar unsubscribed in HCB core"));
            }
          });
    return;
  }
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before unsubscribing from a calendar"));
    return;
  }
  watch(calendarReadService_.findCalendar(std::move(calendarId)),
        [this](CalendarLookupResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          const std::optional<CalendarSummary>& calendar =
              std::get<std::optional<CalendarSummary>>(result);
          if (!calendar.has_value() || calendar->primary) {
            setStatus(QStringLiteral("The primary calendar cannot be unsubscribed"));
            return;
          }
          const QString remoteId = calendar->remoteId;
          watch(
              std::async(
                  std::launch::async,
                  [this, remoteId]() -> std::variant<GoogleCalendarManagementResult, AppError> {
                    OAuthCredentialReadResult read =
                        credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
                    if (std::holds_alternative<AppError>(read)) {
                      return std::get<AppError>(std::move(read));
                    }
                    const std::optional<OAuthStoredCredential>& credential =
                        std::get<std::optional<OAuthStoredCredential>>(read);
                    if (!credential.has_value() || credential->accessToken.isEmpty()) {
                      return AppError(AppErrorCode::Configuration,
                                      QStringLiteral("Google authorization must be renewed"));
                    }
                    GoogleCalendarManagementResultOrError removed =
                        googleCalendarManagementClient_
                            .removeListEntry(remoteId, credential->accessToken)
                            .get();
                    return std::holds_alternative<GoogleApiError>(removed)
                               ? std::variant<GoogleCalendarManagementResult, AppError>(AppError(
                                     AppErrorCode::Network,
                                     std::get<GoogleApiError>(std::move(removed)).message()))
                               : std::variant<GoogleCalendarManagementResult, AppError>(
                                     std::get<GoogleCalendarManagementResult>(std::move(removed)));
                  }),
              [this](std::variant<GoogleCalendarManagementResult, AppError> removed) {
                if (std::holds_alternative<AppError>(removed)) {
                  setStatus(errorMessage(std::get<AppError>(std::move(removed))));
                  return;
                }
                setStatus(QStringLiteral("Google calendar unsubscribed"));
                requestGoogleSync(SyncScheduleTrigger::Manual);
              });
        });
}

void AppController::chooseImportFile() {
  const QString path = QFileDialog::getOpenFileName(
      nullptr,
      QStringLiteral("Import Tasks and events"),
      {},
      QStringLiteral("HCB delimited import (*.hcb *.hcbimport *.txt);;CSV (*.csv);;All files (*)"));
  if (path.isEmpty()) {
    return;
  }
  QFile file(path);
  if (!file.open(QIODevice::ReadOnly)) {
    setStatus(QStringLiteral("Import file could not be read"));
    return;
  }
  setParsedImport(ImportService::parse(ImportService::detectFormat(path), file.readAll()),
                  QFileInfo(path).fileName());
}

void AppController::previewDelimitedImport(QString text) {
  setParsedImport(ImportService::parse(ImportFormat::Delimited, text.toUtf8()),
                  QStringLiteral("Pasted delimited text"));
}

void AppController::invalidateImportValidation() {
  setImportReadyToCommit(false);
  importTasks_.clear();
  importEvents_.clear();
}

void AppController::cancelImport() {
  importItems_.clear();
  importTasks_.clear();
  importEvents_.clear();
  setImportPreviewRows({});
  setImportSourceName({});
  setImportReadyToCommit(false);
  importDefaultTaskListId_.clear();
  importDefaultCalendarId_.clear();
  setStatus(QStringLiteral("Import cancelled"));
}

void AppController::setParsedImport(ImportParseResult parsed, QString sourceName) {
  importItems_ = parsed.items;
  importTasks_.clear();
  importEvents_.clear();
  setImportReadyToCommit(false);
  setImportPreviewRows(importPreviewRowVariants(parsed));
  setImportSourceName(std::move(sourceName));
  const qsizetype accepted = static_cast<qsizetype>(parsed.items.size());
  const qsizetype skipped = parsed.rows.size() - accepted;
  setStatus(QStringLiteral("Import parsed: %1 rows need destination validation, %2 skipped")
                .arg(accepted)
                .arg(skipped));
}

void AppController::runImport(QString defaultTaskListId, QString defaultCalendarId) {
  if (importItems_.isEmpty()) {
    setStatus(QStringLiteral("Paste or choose an import with at least one valid row"));
    return;
  }
  if (importReadyToCommit_ && defaultTaskListId == importDefaultTaskListId_ &&
      defaultCalendarId == importDefaultCalendarId_) {
    setImportReadyToCommit(false);
    watch(importMutationService_.create(std::move(importTasks_), std::move(importEvents_)),
          [this](ImportMutationResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            const ImportMutationReceipt receipt = std::get<ImportMutationReceipt>(result);
            refreshTasks();
            refreshCalendar();
            setStatus(QStringLiteral(
                          "Imported %1 task(s) and %2 calendar event(s); remote sync is queued")
                          .arg(receipt.taskCount)
                          .arg(receipt.eventCount));
            importItems_.clear();
            importDefaultTaskListId_.clear();
            importDefaultCalendarId_.clear();
          });
    return;
  }
  loadImportTaskLists(std::move(defaultTaskListId), std::move(defaultCalendarId), 0, {});
}

void AppController::loadImportTaskLists(QString defaultTaskListId,
                                        QString defaultCalendarId,
                                        std::int64_t offset,
                                        QList<TaskListSummary> taskLists) {
  watch(taskListReadService_.list({.limit = 100, .offset = offset}),
        [this,
         defaultTaskListId = std::move(defaultTaskListId),
         defaultCalendarId = std::move(defaultCalendarId),
         taskLists = std::move(taskLists)](TaskListPageResult result) mutable {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          TaskListPage page = std::get<TaskListPage>(std::move(result));
          taskLists.append(std::move(page.items));
          if (page.nextOffset.has_value()) {
            loadImportTaskLists(std::move(defaultTaskListId),
                                std::move(defaultCalendarId),
                                *page.nextOffset,
                                std::move(taskLists));
            return;
          }
          loadImportCalendars(std::move(defaultTaskListId),
                              std::move(defaultCalendarId),
                              0,
                              std::move(taskLists),
                              {});
        });
}

void AppController::loadImportCalendars(QString defaultTaskListId,
                                        QString defaultCalendarId,
                                        std::int64_t offset,
                                        QList<TaskListSummary> taskLists,
                                        QList<CalendarSummary> calendars) {
  watch(calendarReadService_.listCalendars({.includeHidden = true, .limit = 100, .offset = offset}),
        [this,
         defaultTaskListId = std::move(defaultTaskListId),
         defaultCalendarId = std::move(defaultCalendarId),
         taskLists = std::move(taskLists),
         calendars = std::move(calendars)](CalendarListPageResult result) mutable {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          CalendarListPage page = std::get<CalendarListPage>(std::move(result));
          calendars.append(std::move(page.items));
          if (page.nextOffset.has_value()) {
            loadImportCalendars(std::move(defaultTaskListId),
                                std::move(defaultCalendarId),
                                *page.nextOffset,
                                std::move(taskLists),
                                std::move(calendars));
            return;
          }
          executeImport(defaultTaskListId, defaultCalendarId, taskLists, calendars);
        });
}

void AppController::executeImport(QString defaultTaskListId,
                                  QString defaultCalendarId,
                                  const QList<TaskListSummary>& taskLists,
                                  const QList<CalendarSummary>& calendars) {
  QVariantList rows = importPreviewRows_;
  const auto update = [&rows](const ImportItem& item, bool accepted, QString message) {
    for (QVariant& value : rows) {
      QVariantMap row = value.toMap();
      if (row.value(QStringLiteral("line")).toInt() != item.sourceLine) {
        continue;
      }
      row.insert(QStringLiteral("accepted"), accepted);
      row.insert(QStringLiteral("message"), std::move(message));
      value = std::move(row);
      return;
    }
  };
  const auto reject = [&update](const ImportItem& item, QString message) {
    update(item, false, std::move(message));
  };
  const auto accept = [&update](const ImportItem& item, QString message) {
    update(item, true, std::move(message));
  };
  QList<TaskCreateInput> tasks;
  QList<CalendarEventCreateInput> events;
  tasks.reserve(importItems_.size());
  events.reserve(importItems_.size());
  for (const ImportItem& item : importItems_) {
    if (item.kind == ImportItemKind::Task) {
      const std::optional<QString> taskListId =
          resolveImportTarget(item.taskList, defaultTaskListId, taskLists);
      if (!taskListId.has_value()) {
        reject(item, QStringLiteral("task list is missing or ambiguous"));
        continue;
      }
      const auto taskList = std::find_if(
          taskLists.cbegin(), taskLists.cend(), [&taskListId](const TaskListSummary& value) {
            return value.id == *taskListId;
          });
      if (taskList == taskLists.cend()) {
        reject(item, QStringLiteral("task list is unavailable"));
        continue;
      }
      const std::optional<TaskPriority> priority = importPriority(item.taskPriority);
      const std::optional<QString> due =
          item.taskDue.has_value() ? normalizedDueAt(*item.taskDue) : std::optional<QString>{};
      if (!priority.has_value() || (item.taskDue.has_value() && !due.has_value())) {
        reject(item, QStringLiteral("task due date or priority is invalid"));
        continue;
      }
      TaskCreateInput input{.taskListId = *taskListId,
                            .title = item.title,
                            .notes = item.taskNotes,
                            .due = due.has_value() ? std::optional<TaskDue>(TaskDue{.at = due})
                                                   : std::optional<TaskDue>{},
                            .priority = *priority};
      if (item.taskRecurrenceRule.has_value()) {
        const std::optional<TaskRecurrenceRuleInfo> rule =
            parseTaskRecurrenceRule(*item.taskRecurrenceRule);
        const std::optional<QList<QString>> excluded =
            importRecurrenceDates(item.taskExclusionDates);
        const std::optional<QList<QString>> added = importRecurrenceDates(item.taskAdditionDates);
        const QDate anchor =
            due.has_value() ? QDateTime::fromString(*due, Qt::ISODate).date() : QDate();
        if (!rule.has_value() || !excluded.has_value() || !added.has_value() || !anchor.isValid()) {
          reject(item,
                 QStringLiteral("task recurrence requires a valid due date and supported rule"));
          continue;
        }
        TaskRecurrenceEndCondition end;
        if (item.taskRecurrenceUntil.has_value()) {
          const QDate until = QDate::fromString(*item.taskRecurrenceUntil, Qt::ISODate);
          if (!until.isValid()) {
            reject(item, QStringLiteral("task recurrence until date is invalid"));
            continue;
          }
          end = {.kind = TaskRecurrenceEndKind::Until, .untilDate = until.toString(Qt::ISODate)};
        } else if (item.taskRecurrenceCount.has_value()) {
          end = {.kind = TaskRecurrenceEndKind::Count,
                 .count = static_cast<std::int32_t>(*item.taskRecurrenceCount)};
        }
        const QString timeZone = QString::fromUtf8(QTimeZone::systemTimeZoneId());
        const QString seriesId = QUuid::createUuid().toString(QUuid::WithoutBraces);
        TaskRecurrenceMarker marker{.seriesId = seriesId,
                                    .occurrenceId = seriesId + QStringLiteral(":0"),
                                    .frequency = rule->frequency,
                                    .interval = rule->interval,
                                    .anchorDate = anchor.toString(Qt::ISODate),
                                    .timeZone = timeZone,
                                    .end = std::move(end),
                                    .recurrenceRule = *item.taskRecurrenceRule,
                                    .exclusionDates = *excluded,
                                    .additionDates = *added,
                                    .templateTitle = input.title,
                                    .templateDueDate = anchor.toString(Qt::ISODate),
                                    .templatePriority = priorityText(*priority)};
        const TaskRecurrenceSerializationResult serialized =
            serializeTaskRecurrenceNotes(input.notes.value_or(QString()), marker);
        if (serialized.error.has_value()) {
          reject(item, *serialized.error);
          continue;
        }
        input.notes = serialized.notes;
        input.due = TaskDue{.at = due, .timeZone = timeZone};
      }
      std::variant<TaskCreateInput, AppError> validated =
          TaskMutationService::validateCreate(std::move(input));
      if (std::holds_alternative<AppError>(validated)) {
        reject(item, QStringLiteral("task fields exceed supported limits"));
        continue;
      }
      const TaskCreateInput& ready = std::get<TaskCreateInput>(validated);
      accept(item,
             QStringLiteral("ready → Task list “%1”%2")
                 .arg(taskList->title,
                      ready.due.has_value() && ready.due->at.has_value()
                          ? QStringLiteral(" · due %1").arg(*ready.due->at)
                          : QString()));
      tasks.append(std::get<TaskCreateInput>(std::move(validated)));
      continue;
    }
    const std::optional<QString> calendarId =
        resolveImportTarget(item.calendar, defaultCalendarId, calendars);
    const auto calendar = calendarId.has_value()
                              ? std::find_if(calendars.cbegin(),
                                             calendars.cend(),
                                             [&calendarId](const CalendarSummary& value) {
                                               return value.id == *calendarId;
                                             })
                              : calendars.cend();
    if (!calendarId.has_value() || calendar == calendars.cend() ||
        (calendar->accessRole.has_value() && calendar->accessRole != QStringLiteral("writer") &&
         calendar->accessRole != QStringLiteral("owner"))) {
      reject(item, QStringLiteral("calendar is missing, ambiguous, or read-only"));
      continue;
    }
    std::variant<CalendarEventCreateInput, AppError> validated =
        CalendarMutationService::validateCreate({.calendarId = *calendarId,
                                                 .title = item.title,
                                                 .startAt = item.eventStart,
                                                 .endAt = item.eventEnd,
                                                 .allDay = item.eventAllDay,
                                                 .description = item.eventDescription,
                                                 .location = item.eventLocation,
                                                 .startTimeZone = item.eventTimeZone,
                                                 .endTimeZone = item.eventTimeZone,
                                                 .recurrenceRule = item.eventRecurrence});
    if (std::holds_alternative<AppError>(validated)) {
      reject(item,
             QStringLiteral("event dates, time zone, recurrence, or field limits are invalid"));
      continue;
    }
    const CalendarEventCreateInput& ready = std::get<CalendarEventCreateInput>(validated);
    accept(item,
           QStringLiteral("ready → Calendar “%1” · %2 → %3%4")
               .arg(calendar->title,
                    ready.startAt,
                    ready.endAt,
                    ready.recurrenceRule.has_value() ? QStringLiteral(" · recurring") : QString()));
    events.append(std::get<CalendarEventCreateInput>(std::move(validated)));
  }
  setImportPreviewRows(std::move(rows));
  if (tasks.isEmpty() && events.isEmpty()) {
    importTasks_.clear();
    importEvents_.clear();
    setImportReadyToCommit(false);
    setStatus(QStringLiteral("No import rows are eligible after destination validation"));
    return;
  }
  importTasks_ = std::move(tasks);
  importEvents_ = std::move(events);
  importDefaultTaskListId_ = std::move(defaultTaskListId);
  importDefaultCalendarId_ = std::move(defaultCalendarId);
  setImportReadyToCommit(true);
  setStatus(QStringLiteral("Import validated: %1 task(s), %2 event(s) ready; confirm to create")
                .arg(importTasks_.size())
                .arg(importEvents_.size()));
}

void AppController::queryGoogleFreeBusy(QVariantList calendarIds, QString startAt, QString endAt) {
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before checking availability"));
    return;
  }
  QList<QString> ids;
  ids.reserve(calendarIds.size());
  for (const QVariant& value : calendarIds) {
    if (!value.canConvert<QString>()) {
      setStatus(QStringLiteral("Free-busy calendars are invalid"));
      return;
    }
    ids.append(value.toString());
  }
  watch(std::async(
            std::launch::async,
            [this, ids = std::move(ids), startAt = std::move(startAt), endAt = std::move(endAt)]()
                -> std::variant<QVariantList, AppError> {
              QList<QString> remoteIds;
              remoteIds.reserve(ids.size());
              for (const QString& id : ids) {
                CalendarLookupResult lookup = calendarReadService_.findCalendar(id).get();
                if (std::holds_alternative<AppError>(lookup)) {
                  return std::get<AppError>(std::move(lookup));
                }
                const std::optional<CalendarSummary>& calendar =
                    std::get<std::optional<CalendarSummary>>(lookup);
                if (!calendar.has_value() || calendar->remoteId.isEmpty()) {
                  return AppError(AppErrorCode::Validation,
                                  QStringLiteral("Free-busy calendar is unavailable"));
                }
                remoteIds.append(calendar->remoteId);
              }
              OAuthCredentialReadResult read =
                  credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
              if (std::holds_alternative<AppError>(read)) {
                return std::get<AppError>(std::move(read));
              }
              const std::optional<OAuthStoredCredential>& credential =
                  std::get<std::optional<OAuthStoredCredential>>(read);
              if (!credential.has_value() || credential->accessToken.isEmpty()) {
                return AppError(AppErrorCode::Configuration,
                                QStringLiteral("Google authorization must be renewed"));
              }
              GoogleCalendarFreeBusyResultOrError response =
                  googleCalendarFreeBusyClient_
                      .query(
                          {.startAt = startAt, .endAt = endAt, .calendarIds = std::move(remoteIds)},
                          credential->accessToken)
                      .get();
              if (std::holds_alternative<GoogleApiError>(response)) {
                return AppError(AppErrorCode::Network,
                                std::get<GoogleApiError>(std::move(response)).message());
              }
              QVariantList intervals;
              const GoogleCalendarFreeBusyResult result =
                  std::get<GoogleCalendarFreeBusyResult>(std::move(response));
              for (auto calendar = result.intervalsByCalendar.constBegin();
                   calendar != result.intervalsByCalendar.constEnd();
                   ++calendar) {
                for (const GoogleCalendarBusyInterval& interval : calendar.value()) {
                  intervals.append(QVariantMap{{QStringLiteral("calendarId"), calendar.key()},
                                               {QStringLiteral("startAt"), interval.startAt},
                                               {QStringLiteral("endAt"), interval.endAt}});
                }
              }
              return intervals;
            }),
        [this](std::variant<QVariantList, AppError> result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          const QVariantList intervals = std::get<QVariantList>(std::move(result));
          if (freeBusyIntervals_ != intervals) {
            freeBusyIntervals_ = intervals;
            emit freeBusyIntervalsChanged();
          }
        });
}

void AppController::searchGoogleDriveAttachments(QString query) {
  if (!googleConnected_ || credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Connect Google before searching Drive attachments"));
    return;
  }
  watch(std::async(std::launch::async,
                   [this, query = std::move(query)]() -> std::variant<QVariantList, AppError> {
                     OAuthCredentialReadResult read =
                         credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
                     if (std::holds_alternative<AppError>(read)) {
                       return std::get<AppError>(std::move(read));
                     }
                     const std::optional<OAuthStoredCredential>& credential =
                         std::get<std::optional<OAuthStoredCredential>>(read);
                     if (!credential.has_value() || credential->accessToken.isEmpty()) {
                       return AppError(
                           AppErrorCode::Configuration,
                           QStringLiteral("Google authorization must be renewed for Drive access"));
                     }
                     GoogleDriveAttachmentCandidatesOrError response =
                         googleDriveFilePickerClient_.search(query, credential->accessToken).get();
                     if (std::holds_alternative<GoogleApiError>(response)) {
                       const GoogleApiError error = std::get<GoogleApiError>(std::move(response));
                       return AppError(AppErrorCode::Network,
                                       QStringLiteral("Drive API access failed: ") +
                                           error.message());
                     }
                     QVariantList rows;
                     const QList<GoogleDriveAttachmentCandidate> candidates =
                         std::get<QList<GoogleDriveAttachmentCandidate>>(std::move(response));
                     rows.reserve(candidates.size());
                     for (const GoogleDriveAttachmentCandidate& candidate : candidates) {
                       rows.append(QVariantMap{
                           {QStringLiteral("id"), candidate.id},
                           {QStringLiteral("name"), candidate.name},
                           {QStringLiteral("mimeType"), candidate.mimeType},
                           {QStringLiteral("fileUrl"), candidate.webViewLink},
                           {QStringLiteral("iconLink"), candidate.iconLink.value_or(QString())}});
                     }
                     return rows;
                   }),
        [this](std::variant<QVariantList, AppError> result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          const QVariantList candidates = std::get<QVariantList>(std::move(result));
          if (driveAttachmentCandidates_ != candidates) {
            driveAttachmentCandidates_ = candidates;
            emit driveAttachmentCandidatesChanged();
          }
        });
}

void AppController::setSearchQuery(QString query) {
  if (query == searchQuery_) {
    return;
  }
  ++searchGeneration_;
  searchDebounce_.stop();
  if (searchCancellation_ != nullptr) {
    static_cast<void>(searchCancellation_->requestStop());
  }
  searchQuery_ = std::move(query);
  emit searchQueryChanged();
  if (bridgeMode()) {
    setSearchError({});
    setSearchFilterChips({});
    if (searchQuery_.trimmed().isEmpty()) {
      searchResultsModel().setResults({});
      setSearchLoading(false);
      return;
    }
    searchDebounce_.start();
    return;
  }
  const LocalSearchQueryResult parsed = LocalSearchQuery::parse(searchQuery_);
  if (std::holds_alternative<AppError>(parsed)) {
    setSearchError(errorMessage(std::get<AppError>(parsed)));
    setSearchFilterChips({});
    searchResultsModel().setResults({});
    setSearchLoading(false);
    return;
  }
  const LocalSearchParsedQuery& value = std::get<LocalSearchParsedQuery>(parsed);
  setSearchError({});
  setSearchFilterChips(value.chips);
  if (value.plainText.isEmpty() && value.chips.isEmpty()) {
    searchResultsModel().setResults({});
    setSearchLoading(false);
    return;
  }
  searchDebounce_.start();
}

void AppController::refreshSearchProjection() {
  if (searchQuery_.trimmed().isEmpty()) {
    return;
  }
  ++searchGeneration_;
  searchDebounce_.stop();
  if (searchCancellation_ != nullptr) {
    static_cast<void>(searchCancellation_->requestStop());
  }
  searchResultsModel().setResults({});
  setSearchLoading(false);
  searchDebounce_.start();
}

void AppController::applySavedSearch(QString savedSearchId) {
  const auto found = std::find_if(
      savedSearches_.cbegin(), savedSearches_.cend(), [&savedSearchId](const SavedSearch& search) {
        return search.id == savedSearchId;
      });
  if (found == savedSearches_.cend()) {
    setStatus(QStringLiteral("Saved search was not found"));
    return;
  }
  setSearchQuery(found->query);
}

void AppController::saveSearch(QString name, QString query) {
  if (bridgeMode()) {
    name = name.trimmed();
    query = query.trimmed();
    if (name.isEmpty() || query.isEmpty()) {
      setStatus(QStringLiteral("Saved search name and query are required"));
      return;
    }
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(pythonBridgeClient_->createSavedSearch(
              pythonBridgeAccountId_,
              std::move(name),
              std::move(query),
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeSavedSearchResponse(std::get<QJsonObject>(std::move(result)))) {
              loadBridgeSavedSearches(pythonBridgeRefreshGeneration_);
              setStatus(QStringLiteral("Saved search stored in HCB core"));
            }
          });
    return;
  }
  name = name.trimmed();
  query = query.trimmed();
  if (name.isEmpty() || query.isEmpty()) {
    setStatus(QStringLiteral("Saved search name and query are required"));
    return;
  }
  const LocalSearchQueryResult parsed = LocalSearchQuery::parse(query);
  if (std::holds_alternative<AppError>(parsed)) {
    setStatus(errorMessage(std::get<AppError>(parsed)));
    return;
  }
  if (std::any_of(
          savedSearches_.cbegin(), savedSearches_.cend(), [&name](const SavedSearch& search) {
            return search.name.compare(name, Qt::CaseInsensitive) == 0;
          })) {
    setStatus(QStringLiteral("Saved search name already exists"));
    return;
  }
  QList<SavedSearch> next = savedSearches_;
  next.append({.id = QUuid::createUuid().toString(QUuid::WithoutBraces),
               .name = std::move(name),
               .query = std::move(query)});
  watch(savedSearchStore_.save(next),
        [this, next = std::move(next)](SavedSearchMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          setSavedSearches(next);
        });
}

void AppController::renameSavedSearch(QString savedSearchId, QString name) {
  if (bridgeMode()) {
    name = name.trimmed();
    if (name.isEmpty()) {
      setStatus(QStringLiteral("Saved search name is required"));
      return;
    }
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const auto found = std::find_if(
        savedSearches_.cbegin(),
        savedSearches_.cend(),
        [&savedSearchId](const SavedSearch& search) { return search.id == savedSearchId; });
    if (found == savedSearches_.cend()) {
      setStatus(QStringLiteral("Saved search was not found"));
      return;
    }
    const QString targetId = savedSearchId;
    watch(pythonBridgeClient_->updateSavedSearch(
              pythonBridgeAccountId_,
              targetId,
              std::optional<QString>{std::move(name)},
              std::nullopt,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeSavedSearchResponse(std::get<QJsonObject>(std::move(result)))) {
              loadBridgeSavedSearches(pythonBridgeRefreshGeneration_);
              setStatus(QStringLiteral("Saved search renamed in HCB core"));
            }
          });
    return;
  }
  name = name.trimmed();
  if (name.isEmpty()) {
    setStatus(QStringLiteral("Saved search name is required"));
    return;
  }
  QList<SavedSearch> next = savedSearches_;
  const auto found =
      std::find_if(next.begin(), next.end(), [&savedSearchId](const SavedSearch& search) {
        return search.id == savedSearchId;
      });
  if (found == next.end()) {
    setStatus(QStringLiteral("Saved search was not found"));
    return;
  }
  if (std::any_of(next.cbegin(), next.cend(), [&savedSearchId, &name](const SavedSearch& search) {
        return search.id != savedSearchId && search.name.compare(name, Qt::CaseInsensitive) == 0;
      })) {
    setStatus(QStringLiteral("Saved search name already exists"));
    return;
  }
  found->name = std::move(name);
  watch(savedSearchStore_.save(next),
        [this, next = std::move(next)](SavedSearchMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          setSavedSearches(next);
        });
}

void AppController::deleteSavedSearch(QString savedSearchId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const auto found = std::find_if(
        savedSearches_.cbegin(),
        savedSearches_.cend(),
        [&savedSearchId](const SavedSearch& search) { return search.id == savedSearchId; });
    if (found == savedSearches_.cend()) {
      setStatus(QStringLiteral("Saved search was not found"));
      return;
    }
    const QString targetId = savedSearchId;
    watch(pythonBridgeClient_->deleteSavedSearch(
              pythonBridgeAccountId_,
              targetId,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this, targetId](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            const QJsonObject data = std::get<QJsonObject>(std::move(result));
            if (data.value(QStringLiteral("deleted")).toString() != targetId) {
              setStatus(QStringLiteral("HCB bridge mutation returned an invalid saved search"));
              return;
            }
            QList<SavedSearch> next = savedSearches_;
            next.erase(std::remove_if(next.begin(),
                                      next.end(),
                                      [&targetId](const SavedSearch& search) {
                                        return search.id == targetId;
                                      }),
                       next.end());
            setSavedSearches(std::move(next));
            loadBridgeSavedSearches(pythonBridgeRefreshGeneration_);
            setStatus(QStringLiteral("Saved search deleted in HCB core"));
          });
    return;
  }
  QList<SavedSearch> next = savedSearches_;
  const auto found =
      std::find_if(next.begin(), next.end(), [&savedSearchId](const SavedSearch& search) {
        return search.id == savedSearchId;
      });
  if (found == next.end()) {
    setStatus(QStringLiteral("Saved search was not found"));
    return;
  }
  next.erase(found);
  watch(savedSearchStore_.save(next),
        [this, next = std::move(next)](SavedSearchMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          setSavedSearches(next);
        });
}

void AppController::saveClientId(QString clientId, QString clientSecret) {
  clientSecret = clientSecret.trimmed();
  if (clientSecret.isEmpty() && clientId.trimmed() == clientId_) {
    clientSecret = clientSecret_;
  }
  watch(oauthConfigurationStore_.save(std::move(clientId), std::move(clientSecret)),
        [this](OAuthClientConfigurationMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            watch(oauthConfigurationStore_.load(),
                  [this](OAuthClientConfigurationReadResult loaded) {
                    if (std::holds_alternative<AppError>(loaded)) {
                      setStatus(errorMessage(std::get<AppError>(loaded)));
                    } else if (const std::optional<OAuthClientConfiguration>& configuration =
                                   std::get<std::optional<OAuthClientConfiguration>>(loaded);
                               configuration.has_value()) {
                      const bool secretChanged = clientSecret_ != configuration->clientSecret;
                      clientId_ = configuration->clientId;
                      clientSecret_ = configuration->clientSecret;
                      {
                        std::lock_guard<std::mutex> lock(syncConfigurationMutex_);
                        syncClientId_ = clientId_;
                        syncClientSecret_ = clientSecret_;
                      }
                      emit clientIdChanged();
                      if (secretChanged) {
                        emit clientSecretChanged();
                      }
                      setStatus(QStringLiteral("Google client configuration saved"));
                    }
                  });
          }
        });
}

void AppController::connectGoogle() {
  if (bridgeMode()) {
    if (bridgeOperationActive()) {
      cancelBridgeOperation();
    } else if (pythonBridgeClient_ == nullptr || pythonBridgeExpectedEmail_.isEmpty()) {
      setStatus(QStringLiteral("HCB bridge account details have not loaded"));
    } else {
      startBridgeOperation(
          QStringLiteral("Google authorization"),
          pythonBridgeClient_->startOAuth(pythonBridgeAccountId_, pythonBridgeExpectedEmail_));
    }
    return;
  }
  if (clientId_.isEmpty()) {
    setStatus(QStringLiteral("Save a desktop OAuth client ID before connecting Google"));
    return;
  }
  if (credentialStore_ == nullptr) {
    setStatus(QStringLiteral("Secure credential storage is unavailable on this platform"));
    return;
  }
  if (oauthLoopbackListener_.isListening()) {
    setStatus(QStringLiteral("Google authorization is already in progress"));
    return;
  }
  const OAuthLoopbackListenerStartResult listenerStart = oauthLoopbackListener_.start();
  if (std::holds_alternative<AppError>(listenerStart)) {
    setStatus(errorMessage(std::get<AppError>(listenerStart)));
    return;
  }
  const QUrl redirectUri = std::get<QUrl>(listenerStart);
  const PkceAuthorizationRequest pkce = pkceStateRegistry_.begin();
  QUrl authorizationUrl(QStringLiteral("https://accounts.google.com/o/oauth2/v2/auth"));
  QUrlQuery query;
  query.addQueryItem(QStringLiteral("client_id"), clientId_);
  query.addQueryItem(QStringLiteral("redirect_uri"), redirectUri.toString(QUrl::FullyEncoded));
  query.addQueryItem(QStringLiteral("response_type"), QStringLiteral("code"));
  query.addQueryItem(QStringLiteral("scope"), requiredGoogleScopes().join(u' '));
  query.addQueryItem(QStringLiteral("state"), pkce.state);
  query.addQueryItem(QStringLiteral("code_challenge"), pkce.codeChallenge);
  query.addQueryItem(QStringLiteral("code_challenge_method"), QStringLiteral("S256"));
  query.addQueryItem(QStringLiteral("access_type"), QStringLiteral("offline"));
  query.addQueryItem(QStringLiteral("prompt"), QStringLiteral("consent"));
  authorizationUrl.setQuery(query);
  const OAuthBrowserAuthorizationLaunchResult launch =
      oauthBrowserAuthorizationLauncher_.launch(authorizationUrl);
  if (std::holds_alternative<AppError>(launch)) {
    oauthLoopbackListener_.stop();
    setStatus(errorMessage(std::get<AppError>(launch)));
    return;
  }
  setStatus(QStringLiteral("Complete Google authorization in your browser"));
}

void AppController::handleOAuthCallback(OAuthLoopbackCallback callback) {
  if (callback.error.has_value()) {
    static_cast<void>(oauthLoopbackListener_.respond(
        callback.requestId, 400, QStringLiteral("Google authorization was not completed.")));
    oauthLoopbackListener_.stop();
    setStatus(QStringLiteral("Google authorization was not completed"));
    return;
  }
  if (!callback.code.has_value() || !callback.state.has_value()) {
    static_cast<void>(oauthLoopbackListener_.respond(
        callback.requestId, 400, QStringLiteral("Google authorization response is invalid.")));
    oauthLoopbackListener_.stop();
    setStatus(QStringLiteral("Google authorization response is invalid"));
    return;
  }
  const PkceStateValidationResult state = pkceStateRegistry_.consume(*callback.state);
  if (state.status != PkceStateValidationStatus::Accepted) {
    static_cast<void>(oauthLoopbackListener_.respond(
        callback.requestId,
        400,
        QStringLiteral("Google authorization state is invalid or expired.")));
    oauthLoopbackListener_.stop();
    setStatus(QStringLiteral("Google authorization state is invalid or expired"));
    return;
  }
  const QUrl redirectUri = oauthLoopbackListener_.redirectUri();
  if (!redirectUri.isValid()) {
    static_cast<void>(oauthLoopbackListener_.respond(
        callback.requestId, 500, QStringLiteral("Google authorization could not be completed.")));
    oauthLoopbackListener_.stop();
    setStatus(QStringLiteral("Google authorization listener is unavailable"));
    return;
  }
  watch(oauthTokenExchangeClient_.exchange(
            {.code = *callback.code,
             .codeVerifier = state.codeVerifier,
             .redirectUri = redirectUri,
             .clientId = clientId_,
             .clientSecret =
                 clientSecret_.isEmpty() ? std::nullopt : std::optional<QString>(clientSecret_)}),
        [this, requestId = callback.requestId](OAuthTokenExchangeResult result) {
          if (std::holds_alternative<AppError>(result)) {
            const QString diagnostic =
                SecretRedactor::redactText(errorMessage(std::get<AppError>(result)), 240);
            qWarning().noquote() << "oauth.token_exchange_failed" << diagnostic;
            static_cast<void>(oauthLoopbackListener_.respond(requestId, 500, diagnostic));
            oauthLoopbackListener_.stop();
            setStatus(diagnostic);
            return;
          }
          finishOAuthConnection(requestId, std::get<OAuthTokenSet>(std::move(result)));
        });
}

void AppController::finishOAuthConnection(std::uint64_t requestId, OAuthTokenSet tokenSet) {
  if (!tokenSet.refreshToken.has_value() || credentialStore_ == nullptr) {
    static_cast<void>(oauthLoopbackListener_.respond(
        requestId, 500, QStringLiteral("Google did not return a reusable authorization.")));
    oauthLoopbackListener_.stop();
    setStatus(QStringLiteral("Google did not return a reusable authorization"));
    return;
  }
  const QStringList scopes = tokenSet.scope.has_value()
                                 ? tokenSet.scope->split(u' ', Qt::SkipEmptyParts)
                                 : requiredGoogleScopes();
  watch(
      credentialStore_->save(QString::fromLatin1(kGoogleAccountId),
                             {.accessToken = std::move(tokenSet.accessToken),
                              .refreshToken = std::move(tokenSet.refreshToken)}),
      [this, requestId, scopes](OAuthCredentialSaveResult result) {
        if (std::holds_alternative<AppError>(result)) {
          static_cast<void>(oauthLoopbackListener_.respond(
              requestId, 500, QStringLiteral("Google authorization could not be saved.")));
          oauthLoopbackListener_.stop();
          setStatus(errorMessage(std::get<AppError>(result)));
          return;
        }
        watch(
            accountStatusService_.upsert({.accountId = QString::fromLatin1(kGoogleAccountId),
                                          .connectionState = AccountConnectionState::Connected,
                                          .grantedScopes = scopes,
                                          .lastAuthenticatedAt = authenticationTimestamp(clock_)}),
            [this, requestId](AccountStatusSaveResultOrError saved) {
              if (std::holds_alternative<AppError>(saved)) {
                static_cast<void>(oauthLoopbackListener_.respond(
                    requestId, 500, QStringLiteral("Google authorization could not be saved.")));
                oauthLoopbackListener_.stop();
                setStatus(errorMessage(std::get<AppError>(saved)));
                return;
              }
              static_cast<void>(oauthLoopbackListener_.respond(
                  requestId, 200, QStringLiteral("Google connected. You can close this page.")));
              oauthLoopbackListener_.stop();
              if (!googleConnected_) {
                googleConnected_ = true;
                emit googleConnectedChanged();
              }
              setStatus(QStringLiteral("Google connected"));
              syncGoogle();
            });
      });
}

void AppController::syncGoogle() {
  if (bridgeMode()) {
    if (bridgeOperationActive()) {
      cancelBridgeOperation();
    } else if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
    } else {
      startBridgeOperation(QStringLiteral("HCB sync"),
                           pythonBridgeClient_->startSync(pythonBridgeAccountId_));
    }
    return;
  }
  if (!googleConnected_ || clientId_.isEmpty() || credentialStore_ == nullptr) {
    return;
  }
  requestGoogleSync(SyncScheduleTrigger::Manual);
  startPeriodicGoogleSync();
}

void AppController::saveConflictPolicy(int policyValue) {
  if (bridgeMode()) {
    reportBridgeUnsupportedAction();
    return;
  }
  const std::optional<SyncConflictPolicy> policy = conflictPolicyForValue(policyValue);
  if (!policy.has_value()) {
    setStatus(QStringLiteral("Sync conflict policy is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kSyncSettingsScope),
                                   QString::fromLatin1(kConflictPolicySettingsKey),
                                   QString::number(policyValue)),
        [this, policyValue, policy](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          if (conflictPolicy_ != policyValue) {
            conflictPolicy_ = policyValue;
            emit conflictPolicyChanged();
          }
          googleSyncConflictResolver_.setPolicy(*policy);
          setStatus(QStringLiteral("Sync conflict policy saved"));
        });
}

void AppController::saveNotesEnabled(bool enabled) {
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kNotesEnabledSettingsKey),
                                   enabled ? QStringLiteral("true") : QStringLiteral("false")),
        [this, enabled](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          if (notesEnabled_ != enabled) {
            notesEnabled_ = enabled;
            applyTaskProjections(taskProjectionTasks_);
            refreshSearchProjection();
            emit notesEnabledChanged();
          }
          ensureNotesSidebarTab();
          setStatus(QStringLiteral("Notes presentation saved"));
        });
}

void AppController::ensureNotesSidebarTab() {
  if (!notesEnabled_ || sidebarTabIds_.contains(QStringLiteral("notes"))) {
    return;
  }
  sidebarTabIds_.append(QStringLiteral("notes"));
  emit sidebarTabIdsChanged();
  const QStringList persistedIds = sidebarTabIds_;
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kSidebarTabIdsSettingsKey),
                                   jsonStringList(persistedIds)),
        [this](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          }
        });
}

void AppController::saveNotesProjectionMode(int mode) {
  if (!isValidNotesProjectionMode(mode)) {
    setStatus(QStringLiteral("Notes projection is invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kNotesProjectionSettingsKey),
                                   QString::number(mode)),
        [this, mode](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          if (notesProjectionMode_ != mode) {
            notesProjectionMode_ = mode;
            applyTaskProjections(taskProjectionTasks_);
            refreshSearchProjection();
            emit notesProjectionModeChanged();
          }
          setStatus(QStringLiteral("Notes presentation saved"));
        });
}

void AppController::resolveSyncConflict(QString conflictId, bool keepLocal) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const QString resolvedConflictId = conflictId;
    watch(pythonBridgeClient_->resolveConflict(
              pythonBridgeAccountId_,
              resolvedConflictId,
              keepLocal,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this, conflictId = resolvedConflictId](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            const QJsonObject data = std::get<QJsonObject>(std::move(result));
            const std::optional<QVariantMap> resolved =
                bridgeConflictRow(data.value(QStringLiteral("conflict")).toObject());
            if (!resolved.has_value() ||
                resolved->value(QStringLiteral("id")).toString() != conflictId ||
                resolved->value(QStringLiteral("resolution")).toString().isEmpty()) {
              setStatus(QStringLiteral("HCB bridge returned an invalid conflict resolution"));
              return;
            }
            QVariantList unresolved;
            unresolved.reserve(unresolvedConflicts_.size());
            for (const QVariant& row : unresolvedConflicts_) {
              if (row.toMap().value(QStringLiteral("id")).toString() != conflictId) {
                unresolved.append(row);
              }
            }
            if (unresolvedConflicts_ != unresolved) {
              unresolvedConflicts_ = std::move(unresolved);
              emit unresolvedConflictsChanged();
            }
            QVariantList recent = resolvedConflicts_;
            recent.prepend(*resolved);
            while (recent.size() > 20) {
              recent.removeLast();
            }
            if (resolvedConflicts_ != recent) {
              resolvedConflicts_ = std::move(recent);
              emit resolvedConflictsChanged();
            }
            loadBridgeConflicts(pythonBridgeRefreshGeneration_);
            setStatus(QStringLiteral("Sync conflict resolved in HCB core"));
          });
    return;
  }
  const SyncConflictResolution resolution =
      keepLocal ? SyncConflictResolution::KeepLocal : SyncConflictResolution::KeepRemote;
  watch(googleSyncConflictResolver_.resolve(std::move(conflictId), resolution),
        [this](std::optional<AppError> result) {
          if (result.has_value()) {
            setStatus(errorMessage(*result));
            return;
          }
          setStatus(QStringLiteral("Sync conflict resolved"));
          syncGoogle();
        });
}

void AppController::requestGoogleSync(SyncScheduleTrigger trigger) {
  if (!googleConnected_ || clientId_.isEmpty() || credentialStore_ == nullptr) {
    return;
  }
  setSyncStatus(QStringLiteral("pulling"));
  watch(syncScheduler_.request(trigger), [this](SyncSchedulerResult result) {
    if (std::holds_alternative<AppError>(result)) {
      const AppError& error = std::get<AppError>(result);
      setSyncStatus(error.code() == AppErrorCode::Configuration ? QStringLiteral("auth-required")
                                                                : QStringLiteral("retrying"));
      const QString diagnostic = SecretRedactor::redactText(errorMessage(error), 240);
      qWarning().noquote() << "google.sync_failed" << diagnostic;
      setStatus(diagnostic);
      return;
    }
    if (syncStatus_ == QStringLiteral("pulling") || syncStatus_ == QStringLiteral("pushing")) {
      setSyncStatus(QStringLiteral("idle"));
    }
    refresh();
  });
}

void AppController::startPeriodicGoogleSync() {
  if (googleConnected_ && !clientId_.isEmpty()) {
    static_cast<void>(syncScheduler_.startPeriodic(kGoogleSyncInterval));
  }
}

std::optional<AppError> AppController::runGoogleSync(const SyncSchedulerRequest&) {
  const auto fail = [this](AppError error) {
    setSyncStatus(error.code() == AppErrorCode::Configuration ? QStringLiteral("auth-required")
                                                              : QStringLiteral("retrying"));
    return std::optional<AppError>(std::move(error));
  };
  setSyncStatus(QStringLiteral("pulling"));
  QString clientId;
  QString clientSecret;
  {
    std::lock_guard<std::mutex> lock(syncConfigurationMutex_);
    clientId = syncClientId_;
    clientSecret = syncClientSecret_;
  }
  if (credentialStore_ == nullptr || clientId.isEmpty()) {
    return fail(AppError(AppErrorCode::Configuration,
                         QStringLiteral("Google authorization must be renewed")));
  }
  if (const std::optional<AppError> initialization = optimisticMutationCoordinator_.ready().get();
      initialization.has_value()) {
    return fail(*initialization);
  }
  if (const std::optional<AppError> initialization = syncCheckpointStore_.ready().get();
      initialization.has_value()) {
    return fail(*initialization);
  }
  if (const std::optional<AppError> initialization = syncConflictStore_.ready().get();
      initialization.has_value()) {
    return fail(*initialization);
  }
  OAuthCredentialReadResult credential =
      credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
  if (std::holds_alternative<AppError>(credential)) {
    return fail(std::get<AppError>(std::move(credential)));
  }
  const std::optional<OAuthStoredCredential>& stored =
      std::get<std::optional<OAuthStoredCredential>>(credential);
  if (!stored.has_value() || !stored->refreshToken.has_value()) {
    return fail(AppError(AppErrorCode::Configuration,
                         QStringLiteral("Google authorization must be renewed")));
  }
  const QString refreshToken = *stored->refreshToken;
  OAuthTokenRefreshResult refreshed =
      oauthTokenRefreshClient_
          .refresh({.clientId = std::move(clientId),
                    .refreshToken = refreshToken,
                    .clientSecret = clientSecret.isEmpty() ? std::nullopt
                                                           : std::optional<QString>(clientSecret)})
          .get();
  if (std::holds_alternative<AppError>(refreshed)) {
    return fail(std::get<AppError>(std::move(refreshed)));
  }
  const QString accessToken = std::get<OAuthRefreshedToken>(std::move(refreshed)).accessToken;
  OAuthCredentialSaveResult saved =
      credentialStore_
          ->save(QString::fromLatin1(kGoogleAccountId),
                 {.accessToken = accessToken, .refreshToken = refreshToken})
          .get();
  if (std::holds_alternative<AppError>(saved)) {
    return fail(std::get<AppError>(std::move(saved)));
  }
  setSyncStatus(QStringLiteral("pushing"));
  GoogleTaskMutationPushResultOrError taskPush =
      googleTaskMutationPushService_.pushDue(accessToken).get();
  if (std::holds_alternative<AppError>(taskPush)) {
    return fail(std::get<AppError>(std::move(taskPush)));
  }
  GoogleCalendarEventMutationPushResultOrError eventPush =
      googleCalendarEventMutationPushService_.pushDue(accessToken).get();
  if (std::holds_alternative<AppError>(eventPush)) {
    return fail(std::get<AppError>(std::move(eventPush)));
  }
  const bool hasDeferredMutations =
      std::get<GoogleTaskMutationPushResult>(taskPush).failed > 0 ||
      std::get<GoogleCalendarEventMutationPushResult>(eventPush).failed > 0;
  SyncConflictListResult conflicts = syncConflictStore_.listUnresolved().get();
  if (std::holds_alternative<AppError>(conflicts)) {
    return fail(std::get<AppError>(std::move(conflicts)));
  }
  const QList<SyncConflict>& unresolved = std::get<QList<SyncConflict>>(conflicts);
  setUnresolvedConflicts(unresolved);
  SyncConflictListResult history = syncConflictStore_.listResolved().get();
  if (std::holds_alternative<AppError>(history)) {
    return fail(std::get<AppError>(std::move(history)));
  }
  setResolvedConflicts(std::get<QList<SyncConflict>>(std::move(history)));
  setSyncStatus(QStringLiteral("pulling"));
  GoogleMirrorWriteResult pulled = pullGoogleData(accessToken).get();
  if (std::holds_alternative<AppError>(pulled)) {
    return fail(std::get<AppError>(std::move(pulled)));
  }
  if (!unresolved.isEmpty()) {
    setSyncStatus(QStringLiteral("conflict"));
  } else if (hasDeferredMutations) {
    setSyncStatus(QStringLiteral("retrying"));
  } else {
    setSyncStatus(QStringLiteral("idle"));
  }
  return std::nullopt;
}

std::future<GoogleMirrorWriteResult> AppController::pullGoogleData(QString accessToken) {
  auto completion = std::make_shared<std::promise<GoogleMirrorWriteResult>>();
  std::future<GoogleMirrorWriteResult> future = completion->get_future();
  auto pull = [this, accessToken = std::move(accessToken)] {
    GoogleTaskMirrorSyncResultOrError taskSync =
        googleTaskMirrorSyncService_.sync(QString::fromLatin1(kGoogleAccountId), accessToken).get();
    if (std::holds_alternative<AppError>(taskSync)) {
      return GoogleMirrorWriteResult(std::get<AppError>(std::move(taskSync)));
    }
    if (std::holds_alternative<GoogleApiError>(taskSync)) {
      return GoogleMirrorWriteResult(
          AppError(AppErrorCode::Network, std::get<GoogleApiError>(std::move(taskSync)).message()));
    }
    GoogleCalendarMirrorSyncResultOrError calendarSync =
        googleCalendarMirrorSyncService_.sync(QString::fromLatin1(kGoogleAccountId), accessToken)
            .get();
    if (std::holds_alternative<AppError>(calendarSync)) {
      return GoogleMirrorWriteResult(std::get<AppError>(std::move(calendarSync)));
    }
    if (std::holds_alternative<GoogleApiError>(calendarSync)) {
      return GoogleMirrorWriteResult(AppError(
          AppErrorCode::Network, std::get<GoogleApiError>(std::move(calendarSync)).message()));
    }
    return GoogleMirrorWriteResult(std::monostate{});
  };
  try {
    std::thread([completion, pull = std::move(pull)]() mutable {
      try {
        completion->set_value(pull());
      } catch (...) {
        completion->set_value(
            AppError(AppErrorCode::Network, QStringLiteral("Google sync failed unexpectedly")));
      }
    }).detach();
  } catch (...) {
    completion->set_value(
        AppError(AppErrorCode::Network, QStringLiteral("Google sync could not start")));
  }
  return future;
}

void AppController::createTask(QString taskListId, QString parentTaskId, QString title) {
  watch(
      taskMutationService_.create(
          {.taskListId = std::move(taskListId),
           .parentTaskId = parentTaskId.isEmpty() ? std::nullopt
                                                  : std::optional<QString>(std::move(parentTaskId)),
           .title = std::move(title)}),
      [this](TaskMutationResult result) {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(result)));
        } else {
          recordExistenceHistory(UndoResourceKind::Task,
                                 std::get<TaskMutationReceipt>(result).taskId,
                                 QStringLiteral("task.create"),
                                 QStringLiteral("Create task"),
                                 false,
                                 true);
          refreshTasks();
          refreshPendingSyncCount();
        }
      });
}

void AppController::createTaskDetailed(QString taskListId,
                                       QString parentTaskId,
                                       QString title,
                                       QString notes,
                                       QString dueAt,
                                       QString dueTimeZone,
                                       int priority,
                                       bool managedRecurrence,
                                       int recurrenceFrequency,
                                       int recurrenceInterval,
                                       int recurrenceEndKind,
                                       QString recurrenceEndUntil,
                                       int recurrenceEndCount,
                                       QString recurrenceRule,
                                       QString exclusionDates,
                                       QString additionDates) {
  const std::optional<TaskPriority> parsedPriority = priorityForValue(priority);
  const bool clearingDue = dueAt.trimmed().isEmpty();
  const std::optional<QString> normalizedDue =
      clearingDue ? std::optional<QString>{} : normalizedDueAt(std::move(dueAt));
  if (!parsedPriority.has_value() || (!normalizedDue.has_value() && !clearingDue)) {
    setStatus(QStringLiteral("Task creation input is invalid"));
    return;
  }
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const std::optional<QString> due = bridgeDueDate(normalizedDue);
    if (!due.has_value() && normalizedDue.has_value()) {
      setStatus(QStringLiteral("Task due date is invalid"));
      return;
    }
    QJsonObject request{{QStringLiteral("list_id"), taskListId},
                        {QStringLiteral("title"), title.trimmed()},
                        {QStringLiteral("priority"), priorityText(*parsedPriority)}};
    if (!notes.isEmpty()) {
      request.insert(QStringLiteral("notes"), notes);
    }
    if (due.has_value()) {
      request.insert(QStringLiteral("due"), *due);
      if (!dueTimeZone.trimmed().isEmpty()) {
        request.insert(QStringLiteral("due_time_zone"), dueTimeZone.trimmed());
      }
    }
    if (managedRecurrence) {
      const std::optional<ManagedTaskRecurrenceConfiguration> recurrence =
          managedTaskRecurrenceConfiguration(recurrenceFrequency,
                                             recurrenceInterval,
                                             recurrenceEndKind,
                                             recurrenceEndUntil,
                                             recurrenceEndCount);
      const std::optional<QList<QString>> excluded = recurrenceDatesFromText(exclusionDates);
      const std::optional<QList<QString>> added = recurrenceDatesFromText(additionDates);
      const QString timeZone = dueTimeZone.trimmed().isEmpty()
                                   ? QString::fromUtf8(QTimeZone::systemTimeZoneId())
                                   : dueTimeZone.trimmed();
      if (!recurrence.has_value() || !due.has_value() || !parentTaskId.isEmpty() ||
          !excluded.has_value() || !added.has_value() || !QTimeZone(timeZone.toUtf8()).isValid()) {
        setStatus(
            QStringLiteral("Managed recurrence requires a top-level task with valid due details"));
        return;
      }
      request.insert(QStringLiteral("due_time_zone"), timeZone);
      request.insert(QStringLiteral("recurrence"),
                     bridgeTaskRecurrence(*recurrence,
                                          recurrenceRule.trimmed().isEmpty()
                                              ? recurrence->defaultRule
                                              : recurrenceRule.trimmed(),
                                          *excluded,
                                          *added));
    }
    watch(pythonBridgeClient_->createTask(
              pythonBridgeAccountId_,
              request,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeTaskResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Task saved in HCB core"));
            }
          });
    return;
  }
  TaskCreateInput input{
      .taskListId = std::move(taskListId),
      .parentTaskId = parentTaskId.isEmpty() ? std::optional<QString>{}
                                             : std::optional<QString>(std::move(parentTaskId)),
      .title = title.trimmed(),
      .notes = std::move(notes),
      .due = normalizedDue.has_value()
                 ? std::optional<TaskDue>(
                       TaskDue{.at = normalizedDue,
                               .timeZone = dueTimeZone.trimmed().isEmpty()
                                               ? std::optional<QString>{}
                                               : std::optional<QString>(dueTimeZone.trimmed())})
                 : std::optional<TaskDue>{},
      .priority = *parsedPriority};
  if (managedRecurrence) {
    const std::optional<ManagedTaskRecurrenceConfiguration> recurrence =
        managedTaskRecurrenceConfiguration(recurrenceFrequency,
                                           recurrenceInterval,
                                           recurrenceEndKind,
                                           recurrenceEndUntil,
                                           recurrenceEndCount);
    if (!recurrence.has_value() || !normalizedDue.has_value() || input.parentTaskId.has_value()) {
      setStatus(
          QStringLiteral("Managed recurrence requires a top-level task with a valid due date"));
      return;
    }
    const std::optional<QList<QString>> excluded = recurrenceDatesFromText(exclusionDates);
    const std::optional<QList<QString>> added = recurrenceDatesFromText(additionDates);
    if (!excluded.has_value() || !added.has_value()) {
      setStatus(QStringLiteral("Task recurrence dates must be unique ISO dates"));
      return;
    }
    const QDate anchor = QDateTime::fromString(*normalizedDue, Qt::ISODate).date();
    const QString timeZone = dueTimeZone.trimmed().isEmpty()
                                 ? QString::fromUtf8(QTimeZone::systemTimeZoneId())
                                 : dueTimeZone.trimmed();
    if (!anchor.isValid() || !QTimeZone(timeZone.toUtf8()).isValid()) {
      setStatus(QStringLiteral("Managed recurrence time zone is invalid"));
      return;
    }
    const QString seriesId = QUuid::createUuid().toString(QUuid::WithoutBraces);
    TaskRecurrenceMarker marker{.seriesId = seriesId,
                                .occurrenceId = seriesId + QStringLiteral(":0"),
                                .frequency = recurrence->frequency,
                                .interval = recurrence->interval,
                                .anchorDate = anchor.toString(Qt::ISODate),
                                .timeZone = timeZone,
                                .end = recurrence->end,
                                .recurrenceRule = recurrenceRule.trimmed().isEmpty()
                                                      ? recurrence->defaultRule
                                                      : recurrenceRule.trimmed(),
                                .exclusionDates = *excluded,
                                .additionDates = *added,
                                .templateTitle = input.title,
                                .templateDueDate = anchor.toString(Qt::ISODate),
                                .templatePriority = priorityText(*parsedPriority)};
    const TaskRecurrenceSerializationResult serialized =
        serializeTaskRecurrenceNotes(input.notes.value_or(QString()), marker);
    if (serialized.error.has_value()) {
      setStatus(*serialized.error);
      return;
    }
    input.notes = serialized.notes;
    input.due = TaskDue{.at = normalizedDue, .timeZone = timeZone};
  }
  watch(taskMutationService_.create(std::move(input)), [this](TaskMutationResult result) {
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
    } else {
      recordExistenceHistory(UndoResourceKind::Task,
                             std::get<TaskMutationReceipt>(result).taskId,
                             QStringLiteral("task.create"),
                             QStringLiteral("Create task"),
                             false,
                             true);
      refreshTasks();
      refreshPendingSyncCount();
    }
  });
}

void AppController::saveNoteTask(QString taskId, QString taskListId, QString title, QString notes) {
  if (taskId.isEmpty()) {
    watch(taskMutationService_.create({.taskListId = std::move(taskListId),
                                       .title = std::move(title),
                                       .notes = std::move(notes)}),
          [this](TaskMutationResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(result)));
            } else {
              recordExistenceHistory(UndoResourceKind::Task,
                                     std::get<TaskMutationReceipt>(result).taskId,
                                     QStringLiteral("task.create"),
                                     QStringLiteral("Create task"),
                                     false,
                                     true);
              refreshTasks();
              refreshPendingSyncCount();
            }
          });
    return;
  }
  const auto current =
      std::find_if(taskProjectionTasks_.cbegin(),
                   taskProjectionTasks_.cend(),
                   [&taskId](const TaskModelTask& task) { return task.id == taskId; });
  if (current == taskProjectionTasks_.cend()) {
    setStatus(QStringLiteral("Note task is unavailable"));
    return;
  }
  const bool needsMove = current->taskListId != taskListId;
  watch(taskMutationService_.update(
            {.taskId = taskId, .title = std::move(title), .notes = std::move(notes)}),
        [this, taskId = std::move(taskId), taskListId = std::move(taskListId), needsMove](
            TaskMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          if (!needsMove) {
            refreshTasks();
            return;
          }
          watch(taskMutationService_.moveToTaskList(std::move(taskId), std::move(taskListId)),
                [this](TaskMutationResult moveResult) {
                  if (std::holds_alternative<AppError>(moveResult)) {
                    setStatus(errorMessage(std::get<AppError>(moveResult)));
                  }
                  refreshTasks();
                });
        });
}

void AppController::createTaskList(QString title) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || title.trimmed().isEmpty()) {
      setStatus(QStringLiteral("Task list title is required"));
      return;
    }
    watch(pythonBridgeClient_->createTaskList(
              pythonBridgeAccountId_,
              QJsonObject{{QStringLiteral("title"), title.trimmed()}},
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              const QString message = errorMessage(std::get<AppError>(std::move(result)));
              setStatus(message);
              setTaskListError(message);
              return;
            }
            if (!applyBridgeTaskListResponse(std::get<QJsonObject>(std::move(result)))) {
              setTaskListError(statusMessage_);
              return;
            }
            setTaskListError({});
            setStatus(QStringLiteral("Task list saved in HCB core"));
          });
    return;
  }
  watch(taskListMutationService_.create(
            {.accountId = QString::fromLatin1(kGoogleAccountId), .title = std::move(title)}),
        [this](TaskListMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            const QString message = errorMessage(std::get<AppError>(result));
            setStatus(message);
            setTaskListError(message);
          } else {
            setTaskListError({});
            refreshTasks();
          }
        });
}

void AppController::renameTaskList(QString taskListId, QString title) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || taskListId.isEmpty() || title.trimmed().isEmpty()) {
      setStatus(QStringLiteral("Task list title is required"));
      return;
    }
    watch(pythonBridgeClient_->updateTaskList(
              pythonBridgeAccountId_,
              taskListId,
              QJsonObject{{QStringLiteral("title"), title.trimmed()}},
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              const QString message = errorMessage(std::get<AppError>(std::move(result)));
              setStatus(message);
              setTaskListError(message);
              return;
            }
            if (!applyBridgeTaskListResponse(std::get<QJsonObject>(std::move(result)))) {
              setTaskListError(statusMessage_);
              return;
            }
            setTaskListError({});
            setStatus(QStringLiteral("Task list saved in HCB core"));
          });
    return;
  }
  watch(taskListMutationService_.update(
            {.taskListId = std::move(taskListId), .title = std::move(title)}),
        [this](TaskListMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            const QString message = errorMessage(std::get<AppError>(result));
            setStatus(message);
            setTaskListError(message);
          } else {
            setTaskListError({});
            refreshTasks();
          }
        });
}

void AppController::setTaskListSelected(QString taskListId, bool selected) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || taskListId.isEmpty()) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(pythonBridgeClient_->updateTaskList(
              pythonBridgeAccountId_,
              taskListId,
              QJsonObject{{QStringLiteral("selected"), selected}},
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              const QString message = errorMessage(std::get<AppError>(std::move(result)));
              setStatus(message);
              setTaskListError(message);
              return;
            }
            if (!applyBridgeTaskListResponse(std::get<QJsonObject>(std::move(result)))) {
              setTaskListError(statusMessage_);
              return;
            }
            setTaskListError({});
            setStatus(QStringLiteral("Task list visibility saved in HCB core"));
          });
    return;
  }
  watch(taskListMutationService_.setSelected(
            {.taskListId = std::move(taskListId), .selected = selected}),
        [this](TaskListMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            const QString message = errorMessage(std::get<AppError>(result));
            setStatus(message);
            setTaskListError(message);
          } else {
            setTaskListError({});
            refreshTasks();
          }
        });
}

void AppController::deleteTaskList(QString taskListId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr || taskListId.isEmpty()) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(pythonBridgeClient_->deleteTaskList(
              pythonBridgeAccountId_,
              taskListId,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              const QString message = errorMessage(std::get<AppError>(std::move(result)));
              setStatus(message);
              setTaskListError(message);
              return;
            }
            if (!applyBridgeTaskListResponse(std::get<QJsonObject>(std::move(result)))) {
              setTaskListError(statusMessage_);
              return;
            }
            setTaskListError({});
            setStatus(QStringLiteral("Task list deleted in HCB core"));
          });
    return;
  }
  watch(taskListMutationService_.remove(std::move(taskListId)),
        [this](TaskListMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            const QString message = errorMessage(std::get<AppError>(result));
            setStatus(message);
            setTaskListError(message);
          } else {
            setTaskListError({});
            refreshTasks();
          }
        });
}

void AppController::updateTask(QString taskId,
                               QString title,
                               QString notes,
                               QString dueAt,
                               QString dueTimeZone,
                               int priority) {
  const std::optional<TaskPriority> parsedPriority = priorityForValue(priority);
  if (!parsedPriority.has_value()) {
    setStatus(QStringLiteral("Task priority is invalid"));
    return;
  }
  const bool clearingDue = dueAt.trimmed().isEmpty();
  const std::optional<QString> normalizedDue =
      clearingDue ? std::optional<QString>{} : normalizedDueAt(std::move(dueAt));
  if (!normalizedDue.has_value() && !clearingDue) {
    setStatus(QStringLiteral("Task due date is invalid"));
    return;
  }
  watch(taskMutationService_.update(
            {.taskId = std::move(taskId),
             .title = std::move(title),
             .notes = std::move(notes),
             .due = TaskDue{.at = normalizedDue,
                            .timeZone = normalizedDue.has_value() && !dueTimeZone.isEmpty()
                                            ? std::optional<QString>(std::move(dueTimeZone))
                                            : std::nullopt},
             .priority = *parsedPriority}),
        [this](TaskMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshTasks();
          }
        });
}

void AppController::updateTaskDetailed(QString taskId,
                                       QString title,
                                       QString notes,
                                       QString dueAt,
                                       QString dueTimeZone,
                                       int priority,
                                       bool managedRecurrence,
                                       int recurrenceFrequency,
                                       int recurrenceInterval,
                                       int recurrenceEndKind,
                                       QString recurrenceEndUntil,
                                       int recurrenceEndCount,
                                       QString recurrenceRule,
                                       QString exclusionDates,
                                       QString additionDates) {
  const std::optional<TaskPriority> parsedPriority = priorityForValue(priority);
  const bool clearingDue = dueAt.trimmed().isEmpty();
  const std::optional<QString> normalizedDue =
      clearingDue ? std::optional<QString>{} : normalizedDueAt(std::move(dueAt));
  const std::optional<ManagedTaskRecurrenceConfiguration> recurrence =
      managedRecurrence ? managedTaskRecurrenceConfiguration(recurrenceFrequency,
                                                             recurrenceInterval,
                                                             recurrenceEndKind,
                                                             recurrenceEndUntil,
                                                             recurrenceEndCount)
                        : std::optional<ManagedTaskRecurrenceConfiguration>{};
  if (!parsedPriority.has_value() || (!normalizedDue.has_value() && !clearingDue) ||
      (managedRecurrence && (!recurrence.has_value() || !normalizedDue.has_value()))) {
    setStatus(QStringLiteral("Task recurrence input is invalid"));
    return;
  }
  const std::optional<QList<QString>> excluded =
      managedRecurrence ? recurrenceDatesFromText(exclusionDates) : std::optional<QList<QString>>{};
  const std::optional<QList<QString>> added =
      managedRecurrence ? recurrenceDatesFromText(additionDates) : std::optional<QList<QString>>{};
  if (managedRecurrence && (!excluded.has_value() || !added.has_value())) {
    setStatus(QStringLiteral("Task recurrence dates must be unique ISO dates"));
    return;
  }
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const std::optional<QString> due = bridgeDueDate(normalizedDue);
    if (!due.has_value() && normalizedDue.has_value()) {
      setStatus(QStringLiteral("Task due date is invalid"));
      return;
    }
    QJsonObject request{
        {QStringLiteral("title"), title.trimmed()},
        {QStringLiteral("notes"), notes},
        {QStringLiteral("priority"), priorityText(*parsedPriority)},
        {QStringLiteral("due"), due.has_value() ? QJsonValue(*due) : QJsonValue::Null}};
    if (due.has_value() && !dueTimeZone.trimmed().isEmpty()) {
      request.insert(QStringLiteral("due_time_zone"), dueTimeZone.trimmed());
    }
    if (managedRecurrence) {
      const QString timeZone = dueTimeZone.trimmed().isEmpty()
                                   ? QString::fromUtf8(QTimeZone::systemTimeZoneId())
                                   : dueTimeZone.trimmed();
      if (!recurrence.has_value() || !excluded.has_value() || !added.has_value() ||
          !QTimeZone(timeZone.toUtf8()).isValid()) {
        setStatus(QStringLiteral("Task recurrence input is invalid"));
        return;
      }
      request.insert(QStringLiteral("due_time_zone"), timeZone);
      request.insert(QStringLiteral("recurrence"),
                     bridgeTaskRecurrence(*recurrence,
                                          recurrenceRule.trimmed().isEmpty()
                                              ? recurrence->defaultRule
                                              : recurrenceRule.trimmed(),
                                          *excluded,
                                          *added));
    } else {
      request.insert(QStringLiteral("recurrence"), QJsonValue::Null);
    }
    watch(pythonBridgeClient_->updateTask(
              pythonBridgeAccountId_,
              taskId,
              request,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeTaskResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Task saved in HCB core"));
            }
          });
    return;
  }
  const QString selectedRule = managedRecurrence
                                   ? (recurrenceRule.trimmed().isEmpty() ? recurrence->defaultRule
                                                                         : recurrenceRule.trimmed())
                                   : QString();
  const QString selectedTaskId = taskId;
  watch(
      taskMutationService_.update(
          {.taskId = std::move(taskId),
           .title = std::move(title),
           .notes = std::move(notes),
           .due = TaskDue{.at = normalizedDue,
                          .timeZone = normalizedDue.has_value() && !dueTimeZone.isEmpty()
                                          ? std::optional<QString>(std::move(dueTimeZone))
                                          : std::nullopt},
           .priority = *parsedPriority}),
      [this, selectedTaskId, recurrence, selectedRule, excluded, added](TaskMutationResult result) {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(result)));
          return;
        }
        if (!recurrence.has_value()) {
          refreshTasks();
          return;
        }
        watch(taskMutationService_.reconfigureManagedRecurrence(selectedTaskId,
                                                                recurrence->frequency,
                                                                recurrence->interval,
                                                                recurrence->end,
                                                                selectedRule,
                                                                *excluded,
                                                                *added),
              [this](TaskMutationResult reconfigured) {
                if (std::holds_alternative<AppError>(reconfigured)) {
                  setStatus(errorMessage(std::get<AppError>(reconfigured)));
                } else {
                  refreshTasks();
                }
              });
      });
}

void AppController::setTaskCompleted(QString taskId, bool completed) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(pythonBridgeClient_->completeTask(
              pythonBridgeAccountId_,
              taskId,
              completed,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            const QJsonObject data = std::get<QJsonObject>(std::move(result));
            const QJsonValue successor = data.value(QStringLiteral("successor"));
            if (!successor.isUndefined() && !successor.isNull() && !successor.isObject()) {
              setStatus(QStringLiteral("HCB bridge completion returned an invalid successor"));
              return;
            }
            if (applyBridgeTaskResponse(data)) {
              if (successor.isObject()) {
                refreshBridge();
              }
              setStatus(QStringLiteral("Task saved in HCB core"));
            }
          });
    return;
  }
  watch(taskMutationService_.setCompleted(std::move(taskId), completed),
        [this](TaskMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshTasks();
          }
        });
}

void AppController::stopTaskRecurrence(QString taskId, int recurrenceScope) {
  if (recurrenceScope < 0 || recurrenceScope > 2) {
    setStatus(QStringLiteral("Task recurrence scope is invalid"));
    return;
  }
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const QString scope = recurrenceScope == 0   ? QStringLiteral("this")
                          : recurrenceScope == 1 ? QStringLiteral("following")
                                                 : QStringLiteral("series");
    watch(pythonBridgeClient_->stopTaskRecurrence(
              pythonBridgeAccountId_,
              taskId,
              scope,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            const QJsonObject data = std::get<QJsonObject>(std::move(result));
            if (!data.value(QStringLiteral("tasks")).isArray()) {
              setStatus(QStringLiteral("HCB bridge recurrence stop returned an invalid task set"));
              return;
            }
            refreshBridge();
            setStatus(QStringLiteral("Task recurrence stopped in HCB core"));
          });
    return;
  }
  const auto scope = recurrenceScope == 0   ? TaskRecurrenceScope::ThisOccurrence
                     : recurrenceScope == 1 ? TaskRecurrenceScope::ThisAndFollowing
                                            : TaskRecurrenceScope::EntireSeries;
  watch(taskMutationService_.stopManagedRecurrence(std::move(taskId), scope),
        [this](TaskMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshTasks();
          }
        });
}

void AppController::splitTaskRecurrence(QString taskId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(pythonBridgeClient_->splitTaskRecurrence(
              pythonBridgeAccountId_,
              taskId,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            const QJsonObject data = std::get<QJsonObject>(std::move(result));
            if (!data.value(QStringLiteral("tasks")).isArray()) {
              setStatus(QStringLiteral("HCB bridge recurrence split returned an invalid task set"));
              return;
            }
            refreshBridge();
            setStatus(QStringLiteral("Task recurrence split in HCB core"));
          });
    return;
  }
  watch(taskMutationService_.splitManagedRecurrence(std::move(taskId)),
        [this](TaskMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshTasks();
          }
        });
}

void AppController::deleteTask(QString taskId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(pythonBridgeClient_->deleteTask(
              pythonBridgeAccountId_,
              taskId,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeTaskResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Task deleted in HCB core"));
            }
          });
    return;
  }
  watch(taskMutationService_.inspect({taskId}),
        [this, taskId = std::move(taskId)](TaskMutationSnapshotResult inspection) {
          if (std::holds_alternative<AppError>(inspection) ||
              std::get<QList<TaskMutationSnapshot>>(inspection).size() != 1) {
            setStatus(std::holds_alternative<AppError>(inspection)
                          ? errorMessage(std::get<AppError>(inspection))
                          : QStringLiteral("Task is unavailable for deletion"));
            return;
          }
          watch(taskMutationService_.remove(taskId), [this, taskId](TaskMutationResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(result)));
            } else {
              recordExistenceHistory(UndoResourceKind::Task,
                                     taskId,
                                     QStringLiteral("task.delete"),
                                     QStringLiteral("Delete task"),
                                     true,
                                     false);
              refreshTasks();
              refreshPendingSyncCount();
            }
          });
        });
}

void AppController::moveTask(QString taskId, QString taskListId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    watch(
        pythonBridgeClient_->moveTask(pythonBridgeAccountId_,
                                      taskId,
                                      QJsonObject{{QStringLiteral("list_id"), taskListId}},
                                      QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
        [this](PythonBridgeResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          if (applyBridgeTaskResponse(std::get<QJsonObject>(std::move(result)))) {
            refreshBridge();
            setStatus(QStringLiteral("Task moved in HCB core"));
          }
        });
    return;
  }
  watch(taskMutationService_.moveToTaskList(std::move(taskId), std::move(taskListId)),
        [this](TaskMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshTasks();
          }
        });
}

void AppController::reparentTask(QString taskId, QString parentTaskId) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const QJsonValue parent = parentTaskId.isEmpty() ? QJsonValue::Null : QJsonValue(parentTaskId);
    watch(
        pythonBridgeClient_->moveTask(pythonBridgeAccountId_,
                                      taskId,
                                      QJsonObject{{QStringLiteral("parent_id"), parent}},
                                      QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
        [this](PythonBridgeResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          if (applyBridgeTaskResponse(std::get<QJsonObject>(std::move(result)))) {
            refreshBridge();
            setStatus(QStringLiteral("Task hierarchy updated in HCB core"));
          }
        });
    return;
  }
  const std::optional<std::optional<QString>> parent =
      parentTaskId.isEmpty() ? std::optional<std::optional<QString>>(std::optional<QString>{})
                             : std::optional<std::optional<QString>>(std::move(parentTaskId));
  watch(taskMutationService_.update({.taskId = std::move(taskId), .parentTaskId = parent}),
        [this](TaskMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshTasks();
          }
        });
}

void AppController::bulkSetTaskCompleted(QVariantList taskIds, bool completed) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk task selection is invalid"));
    return;
  }
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    runBridgeTaskBulkMutation(completed ? QStringLiteral("Completed") : QStringLiteral("Reopened"),
                              pythonBridgeClient_->bulkCompleteTasks(
                                  pythonBridgeAccountId_,
                                  *ids,
                                  completed,
                                  QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()));
    return;
  }
  runBulkTaskMutation(
      {.action = completed ? TaskBulkAction::Complete : TaskBulkAction::Reopen, .taskIds = *ids});
}

void AppController::bulkDeleteTasks(QVariantList taskIds) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk task selection is invalid"));
    return;
  }
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    runBridgeTaskBulkMutation(QStringLiteral("Deleted"),
                              pythonBridgeClient_->bulkDeleteTasks(
                                  pythonBridgeAccountId_,
                                  *ids,
                                  QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()));
    return;
  }
  runBulkTaskMutation({.action = TaskBulkAction::Delete, .taskIds = *ids});
}

void AppController::bulkMoveTasks(QVariantList taskIds, QString taskListId) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk task selection is invalid"));
    return;
  }
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    runBridgeTaskBulkMutation(QStringLiteral("Moved"),
                              pythonBridgeClient_->bulkMoveTasks(
                                  pythonBridgeAccountId_,
                                  *ids,
                                  taskListId,
                                  QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()));
    return;
  }
  runBulkTaskMutation(
      {.action = TaskBulkAction::MoveToList, .taskIds = *ids, .taskListId = std::move(taskListId)});
}

void AppController::bulkSetTaskDue(QVariantList taskIds, QString dueAt) {
  if (bridgeMode()) {
    reportBridgeUnsupportedAction();
    return;
  }
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  const std::optional<QString> normalizedDue = normalizedDueAt(std::move(dueAt));
  if (!ids.has_value() || !normalizedDue.has_value()) {
    setStatus(QStringLiteral("Bulk task due date is invalid"));
    return;
  }
  watch(taskMutationService_.inspect(*ids),
        [this, ids = *ids, due = *normalizedDue](TaskMutationSnapshotResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const QList<TaskMutationSnapshot> before =
              std::get<QList<TaskMutationSnapshot>>(std::move(result));
          runBulkTaskMutation(
              {.action = TaskBulkAction::SetDue, .taskIds = ids, .due = TaskDue{.at = due}},
              [this, before, due](const TaskBulkMutationSummary&) {
                recordTaskDueHistory(before, QStringLiteral("Schedule task"));
                static_cast<void>(
                    mutationTelemetryStore_.record({.resource = QStringLiteral("task"),
                                                    .operation = QStringLiteral("task.schedule"),
                                                    .scope = QStringLiteral("none"),
                                                    .targetStartAt = due,
                                                    .phase = MutationTelemetryPhase::Intent}));
              });
        });
}

void AppController::bulkClearTaskDue(QVariantList taskIds) {
  if (bridgeMode()) {
    reportBridgeUnsupportedAction();
    return;
  }
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk task selection is invalid"));
    return;
  }
  watch(taskMutationService_.inspect(*ids), [this, ids = *ids](TaskMutationSnapshotResult result) {
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
      return;
    }
    const QList<TaskMutationSnapshot> before =
        std::get<QList<TaskMutationSnapshot>>(std::move(result));
    runBulkTaskMutation({.action = TaskBulkAction::ClearDue, .taskIds = ids},
                        [this, before](const TaskBulkMutationSummary&) {
                          recordTaskDueHistory(before, QStringLiteral("Unschedule task"));
                          static_cast<void>(mutationTelemetryStore_.record(
                              {.resource = QStringLiteral("task"),
                               .operation = QStringLiteral("task.unschedule"),
                               .scope = QStringLiteral("none"),
                               .phase = MutationTelemetryPhase::Intent}));
                        });
  });
}

void AppController::bulkSetTaskPriority(QVariantList taskIds, int priority) {
  if (bridgeMode()) {
    reportBridgeUnsupportedAction();
    return;
  }
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  const std::optional<TaskPriority> parsedPriority = priorityForValue(priority);
  if (!ids.has_value() || !parsedPriority.has_value()) {
    setStatus(QStringLiteral("Bulk task priority is invalid"));
    return;
  }
  runBulkTaskMutation(
      {.action = TaskBulkAction::SetPriority, .taskIds = *ids, .priority = *parsedPriority});
}

void AppController::undo() { replayHistory(UndoAction::Undo); }

void AppController::redo() { replayHistory(UndoAction::Redo); }

void AppController::saveUndoHistorySettings(int retentionDays, int maximumEntries) {
  if (retentionDays < 1 || retentionDays > 3'650 || maximumEntries < 50 || maximumEntries > 1'000) {
    setStatus(QStringLiteral("Undo history settings are invalid"));
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kUndoRetentionDaysSettingsKey),
                                   QString::number(retentionDays)),
        [this, retentionDays, maximumEntries](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                           QString::fromLatin1(kUndoMaximumEntriesSettingsKey),
                                           QString::number(maximumEntries)),
                [this, retentionDays, maximumEntries](SettingsMutationResultOrError maximumResult) {
                  if (std::holds_alternative<AppError>(maximumResult)) {
                    setStatus(errorMessage(std::get<AppError>(maximumResult)));
                    return;
                  }
                  undoRetentionDays_ = retentionDays;
                  undoMaximumEntries_ = maximumEntries;
                  undoRecoveryPolicy_.configure(
                      {.retentionDays = retentionDays, .maximumEntries = maximumEntries});
                  emit undoHistorySettingsChanged();
                  watch(
                      undoRecoveryPolicy_.recover(),
                      [this](UndoRecoveryResult recoveryResult) {
                        if (std::holds_alternative<AppError>(recoveryResult)) {
                          setStatus(errorMessage(std::get<AppError>(recoveryResult)));
                          return;
                        }
                        refreshUndoStatus();
                        setStatus(QStringLiteral("Undo history saved"));
                      },
                      false);
                });
        });
}

void AppController::dismissCalendarDragCreateHint() {
  if (calendarDragCreateHintSeen_) {
    return;
  }
  watch(settingsService_.writeJson(QString::fromLatin1(kPresentationSettingsScope),
                                   QString::fromLatin1(kCalendarDragCreateHintSeenSettingsKey),
                                   QStringLiteral("true")),
        [this](SettingsMutationResultOrError result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else if (!calendarDragCreateHintSeen_) {
            calendarDragCreateHintSeen_ = true;
            emit calendarDragCreateHintSeenChanged();
          }
        });
}

void AppController::bulkReparentTasks(QVariantList taskIds, QString parentTaskId) {
  if (bridgeMode()) {
    reportBridgeUnsupportedAction();
    return;
  }
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk task selection is invalid"));
    return;
  }
  runBulkTaskMutation({.action = TaskBulkAction::Reparent,
                       .taskIds = *ids,
                       .parentTaskId = parentTaskId.trimmed().isEmpty()
                                           ? std::optional<QString>{}
                                           : std::optional<QString>(std::move(parentTaskId))});
}

void AppController::bulkReplaceTaskText(
    QVariantList taskIds, QString findText, QString replaceText, int fields, int recurrenceScope) {
  if (bridgeMode()) {
    reportBridgeUnsupportedAction();
    return;
  }
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  if (!ids.has_value() || findText.isEmpty() || fields <= 0 || fields > 3 ||
      !isValidBulkTextRecurrenceScope(recurrenceScope)) {
    setStatus(QStringLiteral("Bulk task text replacement is invalid"));
    return;
  }
  runBulkTaskMutation({.action = TaskBulkAction::ReplaceText,
                       .taskIds = *ids,
                       .findText = std::move(findText),
                       .replaceText = std::move(replaceText),
                       .textFields = static_cast<std::uint8_t>(fields),
                       .recurrenceScope = recurrenceScope});
}

void AppController::previewBulkTaskText(
    QVariantList taskIds, QString findText, int fields, int recurrenceScope, int requestToken) {
  if (bridgeMode()) {
    setBulkTaskPreviewMessage(
        QStringLiteral("This action is not implemented in the HCB bridge preview"), requestToken);
    return;
  }
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(taskIds);
  if (!ids.has_value() || findText.isEmpty() || fields <= 0 || fields > 3 ||
      !isValidBulkTextRecurrenceScope(recurrenceScope)) {
    setBulkTaskPreviewMessage(QStringLiteral("Enter find text and at least one field"),
                              requestToken);
    return;
  }
  previewBulkTaskMutation({.action = TaskBulkAction::ReplaceText,
                           .taskIds = *ids,
                           .findText = std::move(findText),
                           .textFields = static_cast<std::uint8_t>(fields),
                           .recurrenceScope = recurrenceScope,
                           .previewOnly = true},
                          requestToken);
}

void AppController::createEvent(QString calendarId,
                                QString title,
                                QString startAt,
                                QString endAt,
                                bool allDay,
                                QString description,
                                QString location) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const std::optional<QJsonObject> start = bridgeEventTime(startAt, allDay, {});
    const std::optional<QJsonObject> end = bridgeEventTime(endAt, allDay, {});
    if (!start.has_value() || !end.has_value()) {
      setStatus(QStringLiteral("Calendar event times are invalid"));
      return;
    }
    QJsonObject request{
        {QStringLiteral("calendar_id"), calendarId},
        {QStringLiteral("summary"), title.trimmed()},
        {QStringLiteral("start"), *start},
        {QStringLiteral("end"), *end},
        {QStringLiteral("description"),
         description.isEmpty() ? QJsonValue::Null : QJsonValue(description)},
        {QStringLiteral("location"), location.isEmpty() ? QJsonValue::Null : QJsonValue(location)}};
    watch(pythonBridgeClient_->createEvent(
              pythonBridgeAccountId_,
              request,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeEventResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Event saved in HCB core"));
            }
          });
    return;
  }
  watch(calendarMutationService_.create(
            {.calendarId = std::move(calendarId),
             .title = std::move(title),
             .startAt = std::move(startAt),
             .endAt = std::move(endAt),
             .allDay = allDay,
             .description = description.isEmpty() ? std::nullopt
                                                  : std::optional<QString>(std::move(description)),
             .location =
                 location.isEmpty() ? std::nullopt : std::optional<QString>(std::move(location))}),
        [this](CalendarEventMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            recordExistenceHistory(UndoResourceKind::Event,
                                   std::get<CalendarEventMutationReceipt>(result).eventId,
                                   QStringLiteral("event.create"),
                                   QStringLiteral("Create event"),
                                   false,
                                   true);
            refreshCalendar();
            refreshPendingSyncCount();
          }
        });
}

void AppController::updateEvent(QString eventId,
                                QString calendarId,
                                QString title,
                                QString startAt,
                                QString endAt,
                                bool allDay,
                                QString description,
                                QString location) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const std::optional<QJsonObject> start = bridgeEventTime(startAt, allDay, {});
    const std::optional<QJsonObject> end = bridgeEventTime(endAt, allDay, {});
    if (!start.has_value() || !end.has_value()) {
      setStatus(QStringLiteral("Calendar event times are invalid"));
      return;
    }
    QJsonObject request{
        {QStringLiteral("summary"), title.trimmed()},
        {QStringLiteral("start"), *start},
        {QStringLiteral("end"), *end},
        {QStringLiteral("description"),
         description.isEmpty() ? QJsonValue::Null : QJsonValue(description)},
        {QStringLiteral("location"), location.isEmpty() ? QJsonValue::Null : QJsonValue(location)}};
    watch(pythonBridgeClient_->updateEvent(
              pythonBridgeAccountId_,
              eventId,
              request,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeEventResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Event saved in HCB core"));
            }
          });
    return;
  }
  watch(calendarMutationService_.update(
            {.eventId = std::move(eventId),
             .calendarId = std::move(calendarId),
             .title = std::move(title),
             .description = std::optional<std::optional<QString>>(
                 description.isEmpty() ? std::optional<QString>{}
                                       : std::optional<QString>(std::move(description))),
             .location = std::optional<std::optional<QString>>(
                 location.isEmpty() ? std::optional<QString>{}
                                    : std::optional<QString>(std::move(location))),
             .startAt = std::move(startAt),
             .endAt = std::move(endAt),
             .allDay = allDay}),
        [this](CalendarEventMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshCalendar();
            refreshPendingSyncCount();
          }
        });
}

void AppController::createEventDetailed(QString calendarId,
                                        QString title,
                                        QString startAt,
                                        QString endAt,
                                        bool allDay,
                                        QString description,
                                        QString location,
                                        QString timeZone,
                                        QString colorId,
                                        bool available,
                                        QString visibility,
                                        QVariantList attendees,
                                        bool remindersUseDefault,
                                        QVariantList reminders,
                                        QString recurrenceRule,
                                        bool createGoogleMeet,
                                        QString attachmentsJson,
                                        QString guestPermissionsJson,
                                        QString eventType,
                                        QString statusPropertiesJson,
                                        QString sendUpdates) {
  const std::optional<QList<QString>> parsedAttendees = eventAttendeesFromVariantList(attendees);
  const std::optional<CalendarEventReminderSettings> parsedReminders =
      eventRemindersFromVariantList(remindersUseDefault, reminders);
  if (!parsedAttendees.has_value() || !parsedReminders.has_value()) {
    setStatus(QStringLiteral("Calendar event metadata is invalid"));
    return;
  }
  if (bridgeMode()) {
    const bool unsupported =
        !colorId.trimmed().isEmpty() || !available || !visibility.trimmed().isEmpty() ||
        !attendees.isEmpty() || !remindersUseDefault || !reminders.isEmpty() || createGoogleMeet ||
        (!attachmentsJson.trimmed().isEmpty() && attachmentsJson.trimmed() != "{}") ||
        (!guestPermissionsJson.trimmed().isEmpty() && guestPermissionsJson.trimmed() != "{}") ||
        (!eventType.trimmed().isEmpty() && eventType.trimmed() != "default") ||
        (!statusPropertiesJson.trimmed().isEmpty() && statusPropertiesJson.trimmed() != "{}") ||
        (!sendUpdates.trimmed().isEmpty() && sendUpdates.trimmed() != "all");
    if (unsupported || pythonBridgeClient_ == nullptr) {
      setStatus(QStringLiteral("This event metadata is not supported by the HCB bridge preview"));
      return;
    }
    const std::optional<QJsonObject> start = bridgeEventTime(startAt, allDay, timeZone);
    const std::optional<QJsonObject> end = bridgeEventTime(endAt, allDay, timeZone);
    if (!start.has_value() || !end.has_value()) {
      setStatus(QStringLiteral("Calendar event times are invalid"));
      return;
    }
    QJsonObject request{
        {QStringLiteral("calendar_id"), calendarId},
        {QStringLiteral("summary"), title.trimmed()},
        {QStringLiteral("start"), *start},
        {QStringLiteral("end"), *end},
        {QStringLiteral("description"),
         description.isEmpty() ? QJsonValue::Null : QJsonValue(description)},
        {QStringLiteral("location"), location.isEmpty() ? QJsonValue::Null : QJsonValue(location)}};
    if (!recurrenceRule.trimmed().isEmpty()) {
      request.insert(QStringLiteral("recurrence"), QJsonArray{recurrenceRule.trimmed()});
    }
    watch(pythonBridgeClient_->createEvent(
              pythonBridgeAccountId_,
              request,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeEventResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Event saved in HCB core"));
            }
          });
    return;
  }
  const std::optional<QString> eventTimeZone = timeZone.trimmed().isEmpty()
                                                   ? std::optional<QString>{}
                                                   : std::optional<QString>(timeZone.trimmed());
  watch(calendarMutationService_.create(
            {.calendarId = std::move(calendarId),
             .title = std::move(title),
             .startAt = std::move(startAt),
             .endAt = std::move(endAt),
             .allDay = allDay,
             .description = description.isEmpty() ? std::nullopt
                                                  : std::optional<QString>(std::move(description)),
             .location =
                 location.isEmpty() ? std::nullopt : std::optional<QString>(std::move(location)),
             .startTimeZone = eventTimeZone,
             .endTimeZone = eventTimeZone,
             .colorId = colorId.trimmed().isEmpty() ? std::optional<QString>{}
                                                    : std::optional<QString>(colorId.trimmed()),
             .transparency = available ? std::optional<QString>(QStringLiteral("transparent"))
                                       : std::optional<QString>(QStringLiteral("opaque")),
             .visibility = visibility.trimmed().isEmpty()
                               ? std::optional<QString>{}
                               : std::optional<QString>(visibility.trimmed()),
             .attendeeEmails = *parsedAttendees,
             .reminders = *parsedReminders,
             .recurrenceRule = recurrenceRule.trimmed().isEmpty()
                                   ? std::optional<QString>{}
                                   : std::optional<QString>(recurrenceRule.trimmed()),
             .richMetadata = {.createGoogleMeet = createGoogleMeet,
                              .attachmentsJson = std::move(attachmentsJson),
                              .guestPermissionsJson = std::move(guestPermissionsJson),
                              .eventType = std::move(eventType),
                              .statusPropertiesJson = std::move(statusPropertiesJson),
                              .sendUpdates = std::move(sendUpdates)}}),
        [this](CalendarEventMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            recordExistenceHistory(UndoResourceKind::Event,
                                   std::get<CalendarEventMutationReceipt>(result).eventId,
                                   QStringLiteral("event.create"),
                                   QStringLiteral("Create event"),
                                   false,
                                   true);
            refreshCalendar();
            refreshPendingSyncCount();
          }
        });
}

void AppController::updateEventDetailed(QString eventId,
                                        QString calendarId,
                                        QString title,
                                        QString startAt,
                                        QString endAt,
                                        bool allDay,
                                        QString description,
                                        QString location,
                                        QString timeZone,
                                        QString colorId,
                                        bool available,
                                        QString visibility,
                                        QVariantList attendees,
                                        bool remindersUseDefault,
                                        QVariantList reminders,
                                        QString recurrenceRule,
                                        int recurrenceScope,
                                        bool createGoogleMeet,
                                        QString attachmentsJson,
                                        QString guestPermissionsJson,
                                        QString statusPropertiesJson,
                                        QString sendUpdates) {
  const std::optional<QList<QString>> parsedAttendees = eventAttendeesFromVariantList(attendees);
  const std::optional<CalendarEventReminderSettings> parsedReminders =
      eventRemindersFromVariantList(remindersUseDefault, reminders);
  if (!parsedAttendees.has_value() || !parsedReminders.has_value()) {
    setStatus(QStringLiteral("Calendar event metadata is invalid"));
    return;
  }
  if (bridgeMode()) {
    const bool unsupported =
        !colorId.trimmed().isEmpty() || !available || !visibility.trimmed().isEmpty() ||
        !attendees.isEmpty() || !remindersUseDefault || !reminders.isEmpty() ||
        !recurrenceRule.trimmed().isEmpty() || createGoogleMeet ||
        (!attachmentsJson.trimmed().isEmpty() && attachmentsJson.trimmed() != "{}") ||
        (!guestPermissionsJson.trimmed().isEmpty() && guestPermissionsJson.trimmed() != "{}") ||
        (!statusPropertiesJson.trimmed().isEmpty() && statusPropertiesJson.trimmed() != "{}") ||
        (!sendUpdates.trimmed().isEmpty() && sendUpdates.trimmed() != "all") ||
        recurrenceScope != 2;
    const auto existing =
        std::find_if(pythonBridgeCalendarEvents_.cbegin(),
                     pythonBridgeCalendarEvents_.cend(),
                     [&eventId](const CalendarEventSummary& event) { return event.id == eventId; });
    if (unsupported || pythonBridgeClient_ == nullptr ||
        existing == pythonBridgeCalendarEvents_.cend() || existing->calendarId != calendarId) {
      setStatus(QStringLiteral("This event edit is not supported by the HCB bridge preview"));
      return;
    }
    const std::optional<QJsonObject> start = bridgeEventTime(startAt, allDay, timeZone);
    const std::optional<QJsonObject> end = bridgeEventTime(endAt, allDay, timeZone);
    if (!start.has_value() || !end.has_value()) {
      setStatus(QStringLiteral("Calendar event times are invalid"));
      return;
    }
    QJsonObject request{
        {QStringLiteral("summary"), title.trimmed()},
        {QStringLiteral("start"), *start},
        {QStringLiteral("end"), *end},
        {QStringLiteral("description"),
         description.isEmpty() ? QJsonValue::Null : QJsonValue(description)},
        {QStringLiteral("location"), location.isEmpty() ? QJsonValue::Null : QJsonValue(location)}};
    watch(pythonBridgeClient_->updateEvent(
              pythonBridgeAccountId_,
              eventId,
              request,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeEventResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Event saved in HCB core"));
            }
          });
    return;
  }
  const std::optional<QString> eventTimeZone = timeZone.trimmed().isEmpty()
                                                   ? std::optional<QString>{}
                                                   : std::optional<QString>(timeZone.trimmed());
  const auto scope = recurrenceScope == 0   ? CalendarEventRecurrenceScope::ThisInstance
                     : recurrenceScope == 1 ? CalendarEventRecurrenceScope::ThisAndFollowing
                                            : CalendarEventRecurrenceScope::FullSeries;
  if (recurrenceScope < 0 || recurrenceScope > 2) {
    setStatus(QStringLiteral("Calendar recurrence scope is invalid"));
    return;
  }
  watch(
      calendarMutationService_.updateScoped(
          {.update = {.eventId = std::move(eventId),
                      .calendarId = std::move(calendarId),
                      .title = std::move(title),
                      .description = std::optional<std::optional<QString>>(
                          description.isEmpty() ? std::optional<QString>{}
                                                : std::optional<QString>(std::move(description))),
                      .location = std::optional<std::optional<QString>>(
                          location.isEmpty() ? std::optional<QString>{}
                                             : std::optional<QString>(std::move(location))),
                      .startAt = std::move(startAt),
                      .endAt = std::move(endAt),
                      .allDay = allDay,
                      .startTimeZone = std::optional<std::optional<QString>>(eventTimeZone),
                      .endTimeZone = std::optional<std::optional<QString>>(eventTimeZone),
                      .colorId = std::optional<std::optional<QString>>(
                          colorId.trimmed().isEmpty() ? std::optional<QString>{}
                                                      : std::optional<QString>(colorId.trimmed())),
                      .transparency = available
                                          ? std::optional<QString>(QStringLiteral("transparent"))
                                          : std::optional<QString>(QStringLiteral("opaque")),
                      .visibility = visibility.trimmed().isEmpty()
                                        ? std::optional<QString>{}
                                        : std::optional<QString>(visibility.trimmed()),
                      .attendeeEmails = *parsedAttendees,
                      .reminders = *parsedReminders,
                      .recurrenceRule = std::optional<std::optional<QString>>(
                          recurrenceRule.trimmed().isEmpty()
                              ? std::optional<QString>{}
                              : std::optional<QString>(recurrenceRule.trimmed())),
                      .createGoogleMeet = createGoogleMeet,
                      .attachmentsJson = std::move(attachmentsJson),
                      .guestPermissionsJson = std::move(guestPermissionsJson),
                      .statusPropertiesJson = std::move(statusPropertiesJson),
                      .sendUpdates = std::move(sendUpdates)},
           .scope = scope}),
      [this](CalendarEventMutationResult result) {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(result)));
        } else {
          refreshCalendar();
        }
      });
}

void AppController::deleteEvent(QString eventId, int recurrenceScope) {
  if (bridgeMode()) {
    if (recurrenceScope != 2 || pythonBridgeClient_ == nullptr) {
      setStatus(
          QStringLiteral("Only full-series event deletion is supported by the HCB bridge preview"));
      return;
    }
    watch(pythonBridgeClient_->deleteEvent(
              pythonBridgeAccountId_,
              eventId,
              QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
          [this](PythonBridgeResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(std::move(result))));
              return;
            }
            if (applyBridgeEventResponse(std::get<QJsonObject>(std::move(result)))) {
              setStatus(QStringLiteral("Event deleted in HCB core"));
            }
          });
    return;
  }
  if (recurrenceScope < 0 || recurrenceScope > 2) {
    setStatus(QStringLiteral("Calendar recurrence scope is invalid"));
    return;
  }
  const auto scope = recurrenceScope == 0   ? CalendarEventRecurrenceScope::ThisInstance
                     : recurrenceScope == 1 ? CalendarEventRecurrenceScope::ThisAndFollowing
                                            : CalendarEventRecurrenceScope::FullSeries;
  watch(
      calendarMutationService_.inspect({eventId}),
      [this, eventId = std::move(eventId), scope](CalendarEventMutationSnapshotResult inspection) {
        if (std::holds_alternative<AppError>(inspection)) {
          setStatus(errorMessage(std::get<AppError>(inspection)));
          return;
        }
        const QList<CalendarEventMutationSnapshot> events =
            std::get<QList<CalendarEventMutationSnapshot>>(std::move(inspection));
        const bool undoable = events.size() == 1 && !events.front().recurrenceRule.has_value() &&
                              !events.front().recurringRemoteId.has_value();
        watch(calendarMutationService_.removeScoped({.eventId = eventId, .scope = scope}),
              [this, eventId, undoable](CalendarEventMutationResult result) {
                if (std::holds_alternative<AppError>(result)) {
                  setStatus(errorMessage(std::get<AppError>(result)));
                } else {
                  if (undoable) {
                    recordExistenceHistory(UndoResourceKind::Event,
                                           eventId,
                                           QStringLiteral("event.delete"),
                                           QStringLiteral("Delete event"),
                                           true,
                                           false);
                  }
                  refreshCalendar();
                  refreshPendingSyncCount();
                }
              });
      });
}

void AppController::respondToEvent(QString eventId,
                                   QString responseStatus,
                                   QString responseComment) {
  watch(calendarMutationService_.respond(
            std::move(eventId), std::move(responseStatus), std::move(responseComment)),
        [this](CalendarEventMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshCalendar();
          }
        });
}

void AppController::moveEvent(QString eventId, QString startAt, QString endAt, bool allDay) {
  watch(calendarMutationService_.inspect({eventId}),
        [this,
         eventId = std::move(eventId),
         startAt = std::move(startAt),
         endAt = std::move(endAt),
         allDay](CalendarEventMutationSnapshotResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const QList<CalendarEventMutationSnapshot> snapshots =
              std::get<QList<CalendarEventMutationSnapshot>>(std::move(result));
          if (snapshots.size() != 1) {
            setStatus(QStringLiteral("Calendar event is unavailable for move"));
            return;
          }
          const CalendarEventMutationSnapshot before = snapshots.front();
          watch(calendarMutationService_.update(
                    {.eventId = eventId, .startAt = startAt, .endAt = endAt, .allDay = allDay}),
                [this, before, startAt, endAt, allDay](CalendarEventMutationResult updateResult) {
                  if (std::holds_alternative<AppError>(updateResult)) {
                    setStatus(errorMessage(std::get<AppError>(updateResult)));
                    return;
                  }
                  recordEventTimingHistory(before, QStringLiteral("Move event"));
                  static_cast<void>(
                      mutationTelemetryStore_.record({.resource = QStringLiteral("event"),
                                                      .operation = QStringLiteral("event.move"),
                                                      .scope = QStringLiteral("none"),
                                                      .allDay = allDay,
                                                      .targetStartAt = startAt,
                                                      .targetEndAt = endAt,
                                                      .phase = MutationTelemetryPhase::Intent}));
                  refreshCalendar();
                  refreshPendingSyncCount();
                });
        });
}

void AppController::resizeEvent(QString eventId, QString endAt) {
  watch(calendarMutationService_.inspect({eventId}),
        [this, eventId = std::move(eventId), endAt = std::move(endAt)](
            CalendarEventMutationSnapshotResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const QList<CalendarEventMutationSnapshot> snapshots =
              std::get<QList<CalendarEventMutationSnapshot>>(std::move(result));
          if (snapshots.size() != 1) {
            setStatus(QStringLiteral("Calendar event is unavailable for resize"));
            return;
          }
          const CalendarEventMutationSnapshot before = snapshots.front();
          watch(calendarMutationService_.update({.eventId = eventId, .endAt = endAt}),
                [this, before, endAt](CalendarEventMutationResult updateResult) {
                  if (std::holds_alternative<AppError>(updateResult)) {
                    setStatus(errorMessage(std::get<AppError>(updateResult)));
                    return;
                  }
                  recordEventTimingHistory(before, QStringLiteral("Resize event"));
                  static_cast<void>(
                      mutationTelemetryStore_.record({.resource = QStringLiteral("event"),
                                                      .operation = QStringLiteral("event.resize"),
                                                      .scope = QStringLiteral("none"),
                                                      .allDay = before.allDay,
                                                      .targetStartAt = before.startAt,
                                                      .targetEndAt = endAt,
                                                      .phase = MutationTelemetryPhase::Intent}));
                  refreshCalendar();
                  refreshPendingSyncCount();
                });
        });
}

void AppController::moveEventScoped(
    QString eventId, QString startAt, QString endAt, bool allDay, int recurrenceScope) {
  if (recurrenceScope < 0 || recurrenceScope > 2) {
    setStatus(QStringLiteral("Calendar recurrence scope is invalid"));
    return;
  }
  const auto scope = recurrenceScope == 0   ? CalendarEventRecurrenceScope::ThisInstance
                     : recurrenceScope == 1 ? CalendarEventRecurrenceScope::ThisAndFollowing
                                            : CalendarEventRecurrenceScope::FullSeries;
  const QString telemetryScope = recurrenceScope == 0   ? QStringLiteral("this_instance")
                                 : recurrenceScope == 1 ? QStringLiteral("this_and_following")
                                                        : QStringLiteral("full_series");
  watch(calendarMutationService_.updateScoped(
            {.update = {.eventId = eventId, .startAt = startAt, .endAt = endAt, .allDay = allDay},
             .scope = scope}),
        [this, startAt, endAt, allDay, telemetryScope](CalendarEventMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            static_cast<void>(
                mutationTelemetryStore_.record({.resource = QStringLiteral("event"),
                                                .operation = QStringLiteral("event.move"),
                                                .scope = telemetryScope,
                                                .allDay = allDay,
                                                .targetStartAt = startAt,
                                                .targetEndAt = endAt,
                                                .phase = MutationTelemetryPhase::Intent}));
            refreshCalendar();
          }
        });
}

void AppController::resizeEventScoped(QString eventId, QString endAt, int recurrenceScope) {
  if (recurrenceScope < 0 || recurrenceScope > 2) {
    setStatus(QStringLiteral("Calendar recurrence scope is invalid"));
    return;
  }
  const auto scope = recurrenceScope == 0   ? CalendarEventRecurrenceScope::ThisInstance
                     : recurrenceScope == 1 ? CalendarEventRecurrenceScope::ThisAndFollowing
                                            : CalendarEventRecurrenceScope::FullSeries;
  const QString telemetryScope = recurrenceScope == 0   ? QStringLiteral("this_instance")
                                 : recurrenceScope == 1 ? QStringLiteral("this_and_following")
                                                        : QStringLiteral("full_series");
  watch(calendarMutationService_.updateScoped(
            {.update = {.eventId = eventId, .endAt = endAt}, .scope = scope}),
        [this, endAt, telemetryScope](CalendarEventMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            static_cast<void>(
                mutationTelemetryStore_.record({.resource = QStringLiteral("event"),
                                                .operation = QStringLiteral("event.resize"),
                                                .scope = telemetryScope,
                                                .targetEndAt = endAt,
                                                .phase = MutationTelemetryPhase::Intent}));
            refreshCalendar();
          }
        });
}

void AppController::bulkDeleteEvents(QVariantList eventIds) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(eventIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk event selection is invalid"));
    return;
  }
  runBulkEventMutation({.action = CalendarEventBulkAction::Delete, .eventIds = *ids});
}

void AppController::bulkMoveEvents(QVariantList eventIds, QString calendarId) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(eventIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk event selection is invalid"));
    return;
  }
  runBulkEventMutation({.action = CalendarEventBulkAction::MoveToCalendar,
                        .eventIds = *ids,
                        .calendarId = std::move(calendarId)});
}

void AppController::bulkSetEventColor(QVariantList eventIds, QString colorId) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(eventIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk event selection is invalid"));
    return;
  }
  runBulkEventMutation({.action = CalendarEventBulkAction::SetColor,
                        .eventIds = *ids,
                        .colorId = std::move(colorId)});
}

void AppController::bulkSetEventAvailability(QVariantList eventIds, bool available) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(eventIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk event selection is invalid"));
    return;
  }
  runBulkEventMutation({.action = CalendarEventBulkAction::SetAvailability,
                        .eventIds = *ids,
                        .available = available});
}

void AppController::bulkSetEventVisibility(QVariantList eventIds, QString visibility) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(eventIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk event selection is invalid"));
    return;
  }
  runBulkEventMutation({.action = CalendarEventBulkAction::SetVisibility,
                        .eventIds = *ids,
                        .visibility = std::move(visibility)});
}

void AppController::bulkShiftEventTimes(QVariantList eventIds, int shiftMinutes) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(eventIds);
  if (!ids.has_value()) {
    setStatus(QStringLiteral("Bulk event selection is invalid"));
    return;
  }
  runBulkEventMutation({.action = CalendarEventBulkAction::ShiftTime,
                        .eventIds = *ids,
                        .shiftMinutes = shiftMinutes});
}

void AppController::bulkReplaceEventText(
    QVariantList eventIds, QString findText, QString replaceText, int fields, int recurrenceScope) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(eventIds);
  if (!ids.has_value() || findText.isEmpty() || fields <= 0 || fields > 7 ||
      !isValidBulkTextRecurrenceScope(recurrenceScope)) {
    setStatus(QStringLiteral("Bulk event text replacement is invalid"));
    return;
  }
  runBulkEventMutation({.action = CalendarEventBulkAction::ReplaceText,
                        .eventIds = *ids,
                        .findText = std::move(findText),
                        .replaceText = std::move(replaceText),
                        .textFields = static_cast<std::uint8_t>(fields),
                        .recurrenceScope = recurrenceScope});
}

void AppController::previewBulkEventText(
    QVariantList eventIds, QString findText, int fields, int recurrenceScope, int requestToken) {
  const std::optional<QList<QString>> ids = taskIdsFromVariantList(eventIds);
  if (!ids.has_value() || findText.isEmpty() || fields <= 0 || fields > 7 ||
      !isValidBulkTextRecurrenceScope(recurrenceScope)) {
    setBulkEventPreviewMessage(QStringLiteral("Enter find text and at least one field"),
                               requestToken);
    return;
  }
  previewBulkEventMutation({.action = CalendarEventBulkAction::ReplaceText,
                            .eventIds = *ids,
                            .findText = std::move(findText),
                            .textFields = static_cast<std::uint8_t>(fields),
                            .recurrenceScope = recurrenceScope,
                            .previewOnly = true},
                           requestToken);
}

void AppController::runSearch() {
  if (bridgeMode()) {
    if (searchQuery_.trimmed().isEmpty() || pythonBridgeClient_ == nullptr) {
      setSearchLoading(false);
      return;
    }
    searchCancellation_ = std::make_unique<CancellationSource>();
    const std::uint64_t generation = searchGeneration_;
    setSearchLoading(true);
    watch(
        pythonBridgeClient_->search(
            pythonBridgeAccountId_, searchQuery_, 50, searchCancellation_->token()),
        [this, generation](PythonBridgeResult result) {
          if (generation != searchGeneration_) {
            return;
          }
          setSearchLoading(false);
          if (std::holds_alternative<AppError>(result)) {
            const AppError& error = std::get<AppError>(result);
            if (error.code() != AppErrorCode::Cancelled) {
              setSearchError(errorMessage(error));
            }
            return;
          }
          PythonBridgeSearchOrError decoded = PythonBridgeProjection::searchResults(
              std::get<QJsonObject>(std::move(result)), pythonBridgeAccountId_);
          if (std::holds_alternative<AppError>(decoded)) {
            setSearchError(errorMessage(std::get<AppError>(std::move(decoded))));
            return;
          }
          setSearchError({});
          searchResultsModel().setResults(
              std::get<QList<LocalSearchRankedResult>>(std::move(decoded)));
        },
        false);
    return;
  }
  const LocalSearchQueryResult parsed = LocalSearchQuery::parse(searchQuery_);
  if (std::holds_alternative<AppError>(parsed)) {
    setSearchError(errorMessage(std::get<AppError>(parsed)));
    setSearchLoading(false);
    return;
  }
  const LocalSearchParsedQuery& value = std::get<LocalSearchParsedQuery>(parsed);
  if (value.plainText.isEmpty() && value.chips.isEmpty()) {
    setSearchLoading(false);
    return;
  }
  searchCancellation_ = std::make_unique<CancellationSource>();
  const std::uint64_t generation = searchGeneration_;
  setSearchLoading(true);
  watch(
      localSearchService_.search({.query = searchQuery_}, searchCancellation_->token()),
      [this, generation](LocalSearchPageResult result) {
        if (generation != searchGeneration_) {
          return;
        }
        setSearchLoading(false);
        if (std::holds_alternative<AppError>(result)) {
          const AppError& error = std::get<AppError>(result);
          if (error.code() != AppErrorCode::Cancelled) {
            setSearchError(errorMessage(error));
          }
          return;
        }
        setSearchError({});
        searchResultsModel().setResults(
            searchPresentation(std::get<LocalSearchPage>(std::move(result)).items,
                               notesEnabled_,
                               notesProjectionMode_));
      },
      false);
}

void AppController::loadSavedSearches() {
  watch(savedSearchStore_.load(), [this](SavedSearchListResult result) {
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
      return;
    }
    setSavedSearches(std::get<QList<SavedSearch>>(std::move(result)));
  });
}

void AppController::runBridgeTaskBulkMutation(QString action,
                                              std::future<PythonBridgeResult> future) {
  if (bridgeTaskBulkMutationInFlight_) {
    const QString message = QStringLiteral("Another HCB core bulk task mutation is still running");
    setBulkTaskStatusMessage(message);
    setStatus(message);
    return;
  }
  bridgeTaskBulkMutationInFlight_ = true;
  watch(std::move(future), [this, action = std::move(action)](PythonBridgeResult result) {
    bridgeTaskBulkMutationInFlight_ = false;
    if (std::holds_alternative<AppError>(result)) {
      const QString message = errorMessage(std::get<AppError>(std::move(result)));
      setBulkTaskStatusMessage(message);
      setStatus(message);
      return;
    }
    const QJsonObject data = std::get<QJsonObject>(std::move(result));
    if (!applyBridgeTaskBatchResponse(data)) {
      return;
    }
    const QJsonArray tasks = data.value(QStringLiteral("tasks")).toArray();
    const QString message = QStringLiteral("%1 %2 task%3 in HCB core")
                                .arg(action)
                                .arg(tasks.size())
                                .arg(tasks.size() == 1 ? QString() : QStringLiteral("s"));
    setBulkTaskStatusMessage(message);
    refreshBridge();
    setStatus(message);
  });
}

void AppController::runBulkTaskMutation(
    TaskBulkMutationInput input, std::function<void(const TaskBulkMutationSummary&)> onSuccess) {
  watch(taskBulkMutationService_.execute(std::move(input)),
        [this, onSuccess = std::move(onSuccess)](TaskBulkMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            const QString message = errorMessage(std::get<AppError>(result));
            setBulkTaskStatusMessage(message);
            setStatus(message);
            return;
          }
          const TaskBulkMutationSummary& summary = std::get<TaskBulkMutationSummary>(result);
          const QString message = bulkTaskSummaryMessage(summary);
          setBulkTaskStatusMessage(message);
          setStatus(message);
          if (summary.queued > 0) {
            refreshTasks();
            refreshPendingSyncCount();
            if (onSuccess) {
              onSuccess(summary);
            }
          }
        });
}

void AppController::runBulkEventMutation(CalendarEventBulkMutationInput input) {
  watch(calendarEventBulkMutationService_.execute(std::move(input)),
        [this](CalendarEventBulkMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            const QString message = errorMessage(std::get<AppError>(result));
            setBulkEventStatusMessage(message);
            setStatus(message);
            return;
          }
          const CalendarEventBulkMutationSummary& summary =
              std::get<CalendarEventBulkMutationSummary>(result);
          const QString message = bulkEventSummaryMessage(summary);
          setBulkEventStatusMessage(message);
          setStatus(message);
          if (summary.queued > 0) {
            refreshCalendar();
          }
        });
}

void AppController::previewBulkTaskMutation(TaskBulkMutationInput input, int requestToken) {
  latestTaskPreviewRequestToken_ = requestToken;
  watch(taskBulkMutationService_.execute(std::move(input)),
        [this, requestToken](TaskBulkMutationResult result) {
          if (requestToken != latestTaskPreviewRequestToken_) {
            return;
          }
          if (std::holds_alternative<AppError>(result)) {
            setBulkTaskPreviewMessage(errorMessage(std::get<AppError>(result)), requestToken);
            return;
          }
          const TaskBulkMutationSummary& summary = std::get<TaskBulkMutationSummary>(result);
          setBulkTaskPreviewMessage(QStringLiteral("Preview: %1 records will change; %2 skipped.")
                                        .arg(summary.eligible)
                                        .arg(summary.skipped),
                                    requestToken);
        });
}

void AppController::previewBulkEventMutation(CalendarEventBulkMutationInput input,
                                             int requestToken) {
  latestEventPreviewRequestToken_ = requestToken;
  watch(calendarEventBulkMutationService_.execute(std::move(input)),
        [this, requestToken](CalendarEventBulkMutationResult result) {
          if (requestToken != latestEventPreviewRequestToken_) {
            return;
          }
          if (std::holds_alternative<AppError>(result)) {
            setBulkEventPreviewMessage(errorMessage(std::get<AppError>(result)), requestToken);
            return;
          }
          const CalendarEventBulkMutationSummary& summary =
              std::get<CalendarEventBulkMutationSummary>(result);
          setBulkEventPreviewMessage(QStringLiteral("Preview: %1 records will change; %2 skipped.")
                                         .arg(summary.eligible)
                                         .arg(summary.skipped),
                                     requestToken);
        });
}

void AppController::schedulePoll() {
  if (pollScheduled_) {
    return;
  }
  pollScheduled_ = true;
  QTimer::singleShot(10, this, &AppController::pollPending);
}

void AppController::pollPending() {
  pollScheduled_ = false;
  std::vector<std::unique_ptr<PendingOperation>> existing = std::move(pending_);
  for (std::unique_ptr<PendingOperation>& operation : existing) {
    if (!operation->poll()) {
      pending_.push_back(std::move(operation));
    }
  }
  setBusy(std::any_of(
      pending_.cbegin(), pending_.cend(), [](const std::unique_ptr<PendingOperation>& operation) {
        return operation->affectsBusy();
      }));
  if (!pending_.empty()) {
    schedulePoll();
  }
}

void AppController::refreshTasks() {
  watch(taskListReadService_.list(), [this](TaskListPageResult result) {
    if (std::holds_alternative<AppError>(result)) {
      const QString message = errorMessage(std::get<AppError>(result));
      setStatus(message);
      setTaskListError(message);
      return;
    }
    setTaskListError({});
    taskListModel_.setTaskLists(std::get<TaskListPage>(std::move(result)).items);
  });
  watch(taskReadService_.list({.selectedListsOnly = true}), [this](TaskReadResult result) {
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
      return;
    }
    applyTaskProjections(std::get<QList<TaskModelTask>>(std::move(result)));
  });
}

void AppController::applyTaskProjections(QList<TaskModelTask> tasks) {
  taskProjectionTasks_ = std::move(tasks);
  taskModel_.setTasks(taskPresentation(
      taskProjectionTasks_, notesEnabled_ && notesProjectionMode_ == kNotesOnlyProjection));
  notesModel_.setTasks(taskProjectionTasks_);
  scheduledTaskRows_ = scheduledTaskRows(taskProjectionTasks_);
  scheduledTaskDateIndex_.setTasks(scheduledTaskRows_);
  emit scheduledTasksChanged();
  emit unscheduledTasksChanged();
}

void AppController::refreshUndoStatus() {
  watch(
      undoRecoveryPolicy_.status(),
      [this](UndoStatusResult result) {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(result)));
          return;
        }
        const UndoStatus status = std::get<UndoStatus>(std::move(result));
        const QString undoLabel = status.undoLabel.value_or(QString());
        const QString redoLabel = status.redoLabel.value_or(QString());
        if (undoLabel_ != undoLabel || redoLabel_ != redoLabel) {
          undoLabel_ = undoLabel;
          redoLabel_ = redoLabel;
          emit undoStateChanged();
        }
      },
      false);
}

void AppController::refreshPendingSyncCount() {
  watch(
      optimisticMutationCoordinator_.listActive(),
      [this](PendingMutationListResult result) {
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(result)));
          return;
        }
        const int count =
            static_cast<int>(std::get<QList<PendingMutation>>(std::move(result)).size());
        if (pendingSyncCount_ != count) {
          pendingSyncCount_ = count;
          emit pendingSyncCountChanged();
        }
      },
      false);
}

void AppController::recordExistenceHistory(UndoResourceKind resource,
                                           QString resourceId,
                                           QString actionKind,
                                           QString label,
                                           bool beforeExists,
                                           bool afterExists) {
  watch(
      undoRecoveryPolicy_.record({.actionKind = std::move(actionKind),
                                  .label = std::move(label),
                                  .resource = resource,
                                  .resourceId = std::move(resourceId),
                                  .before = existenceSnapshot(beforeExists),
                                  .after = existenceSnapshot(afterExists)}),
      [this](std::optional<AppError> recordResult) {
        if (recordResult.has_value()) {
          setStatus(errorMessage(*recordResult));
          return;
        }
        refreshUndoStatus();
      },
      false);
}

void AppController::recordTaskDueHistory(QList<TaskMutationSnapshot> before, QString label) {
  if (before.isEmpty()) {
    return;
  }
  QList<QString> ids;
  ids.reserve(before.size());
  for (const TaskMutationSnapshot& task : before) {
    ids.append(task.taskId);
  }
  watch(taskMutationService_.inspect(std::move(ids)),
        [this, before = std::move(before), label = std::move(label)](
            TaskMutationSnapshotResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const QList<TaskMutationSnapshot> after =
              std::get<QList<TaskMutationSnapshot>>(std::move(result));
          for (const TaskMutationSnapshot& beforeTask : before) {
            const auto afterTask = std::find_if(
                after.cbegin(), after.cend(), [&beforeTask](const TaskMutationSnapshot& candidate) {
                  return candidate.taskId == beforeTask.taskId;
                });
            if (afterTask == after.cend()) {
              continue;
            }
            watch(
                undoRecoveryPolicy_.record({.actionKind = QStringLiteral("task.due"),
                                            .label = label,
                                            .resource = UndoResourceKind::Task,
                                            .resourceId = beforeTask.taskId,
                                            .before = taskDueSnapshot(beforeTask),
                                            .after = taskDueSnapshot(*afterTask)}),
                [this](std::optional<AppError> recordResult) {
                  if (recordResult.has_value()) {
                    setStatus(errorMessage(*recordResult));
                    return;
                  }
                  refreshUndoStatus();
                },
                false);
          }
        });
}

void AppController::recordEventTimingHistory(CalendarEventMutationSnapshot before, QString label) {
  watch(calendarMutationService_.inspect({before.eventId}),
        [this, before = std::move(before), label = std::move(label)](
            CalendarEventMutationSnapshotResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
            return;
          }
          const QList<CalendarEventMutationSnapshot> after =
              std::get<QList<CalendarEventMutationSnapshot>>(std::move(result));
          if (after.size() != 1) {
            return;
          }
          watch(
              undoRecoveryPolicy_.record({.actionKind = QStringLiteral("event.timing"),
                                          .label = std::move(label),
                                          .resource = UndoResourceKind::Event,
                                          .resourceId = before.eventId,
                                          .before = eventTimingSnapshot(before),
                                          .after = eventTimingSnapshot(after.front())}),
              [this](std::optional<AppError> recordResult) {
                if (recordResult.has_value()) {
                  setStatus(errorMessage(*recordResult));
                  return;
                }
                refreshUndoStatus();
              },
              false);
        });
}

void AppController::replayHistory(UndoAction action) {
  const auto onEntry = [this](UndoEntryResult result) {
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
      return;
    }
    replayHistoryEntry(std::get<UndoEntry>(std::move(result)));
  };
  if (action == UndoAction::Undo) {
    watch(undoRecoveryPolicy_.nextUndo(), onEntry, false);
  } else {
    watch(undoRecoveryPolicy_.nextRedo(), onEntry, false);
  }
}

void AppController::replayHistoryEntry(UndoEntry entry) {
  if (entry.resource == UndoResourceKind::Task &&
      (entry.actionKind == QStringLiteral("task.create") ||
       entry.actionKind == QStringLiteral("task.delete"))) {
    watch(taskMutationService_.inspect({entry.resourceId}),
          [this, entry = std::move(entry)](TaskMutationSnapshotResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(result)));
              return;
            }
            const bool exists =
                std::get<QList<TaskMutationSnapshot>>(std::move(result)).size() == 1;
            const std::optional<bool> expected = existenceFromSnapshot(entry.expected);
            if (!expected.has_value() || exists != *expected) {
              setStatus(QStringLiteral("Undo is unavailable because the task changed"));
              return;
            }
            const QJsonObject currentSnapshot = existenceSnapshot(exists);
            const auto applyHistory = [this](UndoReplayResult replayResult) {
              if (std::holds_alternative<AppError>(replayResult)) {
                setStatus(errorMessage(std::get<AppError>(replayResult)));
                return;
              }
              const UndoReplay replay = std::get<UndoReplay>(std::move(replayResult));
              const std::optional<bool> target = existenceFromSnapshot(replay.target);
              if (!target.has_value()) {
                setStatus(QStringLiteral("Stored undo task snapshot is invalid"));
                return;
              }
              const QJsonObject targetSnapshot = existenceSnapshot(*target);
              const auto complete = [this, replay, targetSnapshot](TaskMutationResult mutation) {
                if (std::holds_alternative<AppError>(mutation)) {
                  const AppError failure = std::get<AppError>(mutation);
                  auto rollback = replay.action == UndoAction::Undo
                                      ? undoRecoveryPolicy_.redo(targetSnapshot)
                                      : undoRecoveryPolicy_.undo(targetSnapshot);
                  watch(
                      std::move(rollback),
                      [this, failure](UndoReplayResult) {
                        refreshUndoStatus();
                        setStatus(errorMessage(failure));
                      },
                      false);
                  return;
                }
                refreshTasks();
                refreshPendingSyncCount();
                refreshUndoStatus();
                setStatus(QStringLiteral("%1 queued for Google sync")
                              .arg(replay.action == UndoAction::Undo
                                       ? QStringLiteral("Undid %1").arg(replay.label)
                                       : QStringLiteral("Redid %1").arg(replay.label)));
              };
              if (*target) {
                watch(taskMutationService_.restore(replay.resourceId), complete);
              } else {
                watch(taskMutationService_.remove(replay.resourceId), complete);
              }
            };
            if (entry.action == UndoAction::Undo) {
              watch(undoRecoveryPolicy_.undo(currentSnapshot), applyHistory, false);
            } else {
              watch(undoRecoveryPolicy_.redo(currentSnapshot), applyHistory, false);
            }
          });
    return;
  }
  if (entry.resource == UndoResourceKind::Event &&
      (entry.actionKind == QStringLiteral("event.create") ||
       entry.actionKind == QStringLiteral("event.delete"))) {
    watch(calendarMutationService_.inspect({entry.resourceId}),
          [this, entry = std::move(entry)](CalendarEventMutationSnapshotResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(result)));
              return;
            }
            const bool exists =
                std::get<QList<CalendarEventMutationSnapshot>>(std::move(result)).size() == 1;
            const std::optional<bool> expected = existenceFromSnapshot(entry.expected);
            if (!expected.has_value() || exists != *expected) {
              setStatus(QStringLiteral("Undo is unavailable because the event changed"));
              return;
            }
            const QJsonObject currentSnapshot = existenceSnapshot(exists);
            const auto applyHistory = [this](UndoReplayResult replayResult) {
              if (std::holds_alternative<AppError>(replayResult)) {
                setStatus(errorMessage(std::get<AppError>(replayResult)));
                return;
              }
              const UndoReplay replay = std::get<UndoReplay>(std::move(replayResult));
              const std::optional<bool> target = existenceFromSnapshot(replay.target);
              if (!target.has_value()) {
                setStatus(QStringLiteral("Stored undo event snapshot is invalid"));
                return;
              }
              const QJsonObject targetSnapshot = existenceSnapshot(*target);
              const auto complete =
                  [this, replay, targetSnapshot](CalendarEventMutationResult mutation) {
                    if (std::holds_alternative<AppError>(mutation)) {
                      const AppError failure = std::get<AppError>(mutation);
                      auto rollback = replay.action == UndoAction::Undo
                                          ? undoRecoveryPolicy_.redo(targetSnapshot)
                                          : undoRecoveryPolicy_.undo(targetSnapshot);
                      watch(
                          std::move(rollback),
                          [this, failure](UndoReplayResult) {
                            refreshUndoStatus();
                            setStatus(errorMessage(failure));
                          },
                          false);
                      return;
                    }
                    refreshCalendar();
                    refreshPendingSyncCount();
                    refreshUndoStatus();
                    setStatus(QStringLiteral("%1 queued for Google sync")
                                  .arg(replay.action == UndoAction::Undo
                                           ? QStringLiteral("Undid %1").arg(replay.label)
                                           : QStringLiteral("Redid %1").arg(replay.label)));
                  };
              if (*target) {
                watch(calendarMutationService_.restore(replay.resourceId), complete);
              } else {
                watch(calendarMutationService_.remove(replay.resourceId), complete);
              }
            };
            if (entry.action == UndoAction::Undo) {
              watch(undoRecoveryPolicy_.undo(currentSnapshot), applyHistory, false);
            } else {
              watch(undoRecoveryPolicy_.redo(currentSnapshot), applyHistory, false);
            }
          });
    return;
  }
  if (entry.resource == UndoResourceKind::Task && entry.actionKind == QStringLiteral("task.due")) {
    watch(taskMutationService_.inspect({entry.resourceId}),
          [this, entry = std::move(entry)](TaskMutationSnapshotResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(result)));
              return;
            }
            const QList<TaskMutationSnapshot> current =
                std::get<QList<TaskMutationSnapshot>>(std::move(result));
            if (current.size() != 1 || taskDueSnapshot(current.front()) != entry.expected) {
              setStatus(QStringLiteral("Undo is unavailable because the task changed"));
              return;
            }
            const QJsonObject currentSnapshot = taskDueSnapshot(current.front());
            const auto moveHistory = [this](UndoReplayResult replayResult) {
              if (std::holds_alternative<AppError>(replayResult)) {
                setStatus(errorMessage(std::get<AppError>(replayResult)));
                return;
              }
              const UndoReplay replay = std::get<UndoReplay>(std::move(replayResult));
              const std::optional<TaskDue> target =
                  taskDueFromSnapshot(replay.target, replay.resourceId);
              if (!target.has_value()) {
                setStatus(QStringLiteral("Stored undo task snapshot is invalid"));
                return;
              }
              watch(taskMutationService_.update({.taskId = replay.resourceId, .due = *target}),
                    [this, replay, target = *target](TaskMutationResult updateResult) {
                      if (std::holds_alternative<AppError>(updateResult)) {
                        auto restore =
                            replay.action == UndoAction::Undo
                                ? undoRecoveryPolicy_.redo(taskDueSnapshot(
                                      TaskMutationSnapshot{.taskId = replay.resourceId,
                                                           .dueAt = target.at,
                                                           .dueTimeZone = target.timeZone}))
                                : undoRecoveryPolicy_.undo(taskDueSnapshot(
                                      TaskMutationSnapshot{.taskId = replay.resourceId,
                                                           .dueAt = target.at,
                                                           .dueTimeZone = target.timeZone}));
                        watch(
                            std::move(restore),
                            [this, updateResult](UndoReplayResult) {
                              refreshUndoStatus();
                              setStatus(errorMessage(std::get<AppError>(updateResult)));
                            },
                            false);
                        return;
                      }
                      refreshTasks();
                      refreshPendingSyncCount();
                      refreshUndoStatus();
                      setStatus(QStringLiteral("%1 queued for Google sync")
                                    .arg(replay.action == UndoAction::Undo
                                             ? QStringLiteral("Undid %1").arg(replay.label)
                                             : QStringLiteral("Redid %1").arg(replay.label)));
                    });
            };
            if (entry.action == UndoAction::Undo) {
              watch(undoRecoveryPolicy_.undo(currentSnapshot), moveHistory, false);
            } else {
              watch(undoRecoveryPolicy_.redo(currentSnapshot), moveHistory, false);
            }
          });
    return;
  }
  if (entry.resource == UndoResourceKind::Event &&
      entry.actionKind == QStringLiteral("event.timing")) {
    watch(calendarMutationService_.inspect({entry.resourceId}),
          [this, entry = std::move(entry)](CalendarEventMutationSnapshotResult result) {
            if (std::holds_alternative<AppError>(result)) {
              setStatus(errorMessage(std::get<AppError>(result)));
              return;
            }
            const QList<CalendarEventMutationSnapshot> current =
                std::get<QList<CalendarEventMutationSnapshot>>(std::move(result));
            if (current.size() != 1 || eventTimingSnapshot(current.front()) != entry.expected) {
              setStatus(QStringLiteral("Undo is unavailable because the event changed"));
              return;
            }
            const QJsonObject currentSnapshot = eventTimingSnapshot(current.front());
            const auto moveHistory = [this](UndoReplayResult replayResult) {
              if (std::holds_alternative<AppError>(replayResult)) {
                setStatus(errorMessage(std::get<AppError>(replayResult)));
                return;
              }
              const UndoReplay replay = std::get<UndoReplay>(std::move(replayResult));
              const std::optional<EventTiming> target =
                  eventTimingFromSnapshot(replay.target, replay.resourceId);
              if (!target.has_value()) {
                setStatus(QStringLiteral("Stored undo event snapshot is invalid"));
                return;
              }
              watch(calendarMutationService_.update({.eventId = replay.resourceId,
                                                     .startAt = target->startAt,
                                                     .endAt = target->endAt,
                                                     .allDay = target->allDay}),
                    [this, replay, target = *target](CalendarEventMutationResult updateResult) {
                      if (std::holds_alternative<AppError>(updateResult)) {
                        const QJsonObject targetSnapshot{
                            {QStringLiteral("eventId"), replay.resourceId},
                            {QStringLiteral("startAt"), target.startAt},
                            {QStringLiteral("endAt"), target.endAt},
                            {QStringLiteral("allDay"), target.allDay}};
                        auto restore = replay.action == UndoAction::Undo
                                           ? undoRecoveryPolicy_.redo(targetSnapshot)
                                           : undoRecoveryPolicy_.undo(targetSnapshot);
                        watch(
                            std::move(restore),
                            [this, updateResult](UndoReplayResult) {
                              refreshUndoStatus();
                              setStatus(errorMessage(std::get<AppError>(updateResult)));
                            },
                            false);
                        return;
                      }
                      refreshCalendar();
                      refreshPendingSyncCount();
                      refreshUndoStatus();
                      setStatus(QStringLiteral("%1 queued for Google sync")
                                    .arg(replay.action == UndoAction::Undo
                                             ? QStringLiteral("Undid %1").arg(replay.label)
                                             : QStringLiteral("Redid %1").arg(replay.label)));
                    });
            };
            if (entry.action == UndoAction::Undo) {
              watch(undoRecoveryPolicy_.undo(currentSnapshot), moveHistory, false);
            } else {
              watch(undoRecoveryPolicy_.redo(currentSnapshot), moveHistory, false);
            }
          });
    return;
  }
  setStatus(QStringLiteral("Stored undo action is unavailable"));
}

void AppController::reorderTask(QString taskId, bool earlier) {
  if (bridgeMode()) {
    if (pythonBridgeClient_ == nullptr) {
      reportBridgeUnsupportedAction();
      return;
    }
    const auto selected =
        std::find_if(pythonBridgeTasks_.cbegin(),
                     pythonBridgeTasks_.cend(),
                     [&taskId](const TaskModelTask& task) { return task.id == taskId; });
    if (selected == pythonBridgeTasks_.cend()) {
      setStatus(QStringLiteral("Task is unavailable for reordering"));
      return;
    }
    QList<TaskModelTask> siblings;
    for (const TaskModelTask& candidate : pythonBridgeTasks_) {
      if (candidate.taskListId == selected->taskListId &&
          candidate.parentTaskId == selected->parentTaskId) {
        siblings.append(candidate);
      }
    }
    std::sort(siblings.begin(),
              siblings.end(),
              [](const TaskModelTask& left, const TaskModelTask& right) {
                return left.sortOrder != right.sortOrder ? left.sortOrder < right.sortOrder
                                                         : left.id < right.id;
              });
    const auto current =
        std::find_if(siblings.cbegin(), siblings.cend(), [&taskId](const TaskModelTask& task) {
          return task.id == taskId;
        });
    if (current == siblings.cend()) {
      setStatus(QStringLiteral("Task is unavailable for reordering"));
      return;
    }
    const qsizetype index = static_cast<qsizetype>(std::distance(siblings.cbegin(), current));
    if ((earlier && index == 0) || (!earlier && index + 1 >= siblings.size())) {
      setStatus(QStringLiteral("Task is already at the requested position"));
      return;
    }
    const std::optional<QString> previous =
        earlier ? (index > 1 ? std::optional<QString>(siblings.at(index - 2).id) : std::nullopt)
                : std::optional<QString>(siblings.at(index + 1).id);
    const QJsonValue previousValue =
        previous.has_value() ? QJsonValue(*previous) : QJsonValue::Null;
    watch(
        pythonBridgeClient_->moveTask(pythonBridgeAccountId_,
                                      taskId,
                                      QJsonObject{{QStringLiteral("previous_id"), previousValue}},
                                      QUuid::createUuid().toString(QUuid::WithoutBraces).toUtf8()),
        [this](PythonBridgeResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(std::move(result))));
            return;
          }
          if (applyBridgeTaskResponse(std::get<QJsonObject>(std::move(result)))) {
            refreshBridge();
            setStatus(QStringLiteral("Task reordered in HCB core"));
          }
        });
    return;
  }
  watch(taskMutationService_.reorder(std::move(taskId),
                                     earlier ? TaskReorderDirection::Earlier
                                             : TaskReorderDirection::Later),
        [this](TaskMutationResult result) {
          if (std::holds_alternative<AppError>(result)) {
            setStatus(errorMessage(std::get<AppError>(result)));
          } else {
            refreshTasks();
          }
        });
}

void AppController::refreshCalendar() {
  if (reminderService_ != nullptr) {
    reminderService_->refresh();
  }
  const std::uint64_t generation = ++calendarRefreshGeneration_;
  refreshInvitations();
  loadCalendarManagementRows(generation, 0, {});
  watch(calendarReadService_.listCalendars(), [this, generation](CalendarListPageResult result) {
    if (generation != calendarRefreshGeneration_) {
      return;
    }
    if (std::holds_alternative<AppError>(result)) {
      setStatus(errorMessage(std::get<AppError>(result)));
      return;
    }
    QList<CalendarSummary> calendars = std::get<CalendarListPage>(std::move(result)).items;
    QList<QString> ids;
    ids.reserve(calendars.size());
    for (const CalendarSummary& calendar : calendars) {
      ids.append(calendar.id);
    }
    calendarSourceModel_.setCalendars(std::move(calendars));
    refreshCalendarEvents(std::move(ids), generation);
  });
}

void AppController::loadCalendarManagementRows(std::uint64_t generation,
                                               std::int64_t offset,
                                               QVariantList rows) {
  watch(
      calendarReadService_.listCalendars({.includeHidden = true, .limit = 100, .offset = offset}),
      [this, generation, rows = std::move(rows)](CalendarListPageResult result) mutable {
        if (generation != calendarRefreshGeneration_) {
          return;
        }
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(std::move(result))));
          return;
        }
        CalendarListPage page = std::get<CalendarListPage>(std::move(result));
        rows.reserve(rows.size() + page.items.size());
        for (const CalendarSummary& calendar : page.items) {
          QVariantMap row;
          row.insert(QStringLiteral("id"), calendar.id);
          row.insert(QStringLiteral("title"), calendar.title);
          row.insert(QStringLiteral("description"), calendar.description.value_or(QString()));
          row.insert(QStringLiteral("timeZone"), calendar.timeZone.value_or(QString()));
          row.insert(QStringLiteral("colorId"), calendar.colorId.value_or(QString()));
          row.insert(QStringLiteral("backgroundColor"),
                     calendar.backgroundColor.value_or(QString()));
          row.insert(QStringLiteral("accessRole"), calendar.accessRole.value_or(QString()));
          row.insert(QStringLiteral("selected"), calendar.selected);
          row.insert(QStringLiteral("hidden"), calendar.hidden);
          row.insert(QStringLiteral("primary"), calendar.primary);
          rows.append(std::move(row));
        }
        if (page.nextOffset.has_value()) {
          loadCalendarManagementRows(generation, *page.nextOffset, std::move(rows));
          return;
        }
        setCalendarManagementRows(std::move(rows));
      },
      false);
}

void AppController::refreshInvitations() {
  const std::uint64_t generation = ++invitationRefreshGeneration_;
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock_.wallNow().time_since_epoch());
  const QDateTime current = QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC);
  watch(
      calendarReadService_.listEvents({.startAt = current.addDays(-1).toString(Qt::ISODateWithMs),
                                       .endAt = current.addDays(366).toString(Qt::ISODateWithMs),
                                       .limit = 25'000}),
      [this, generation](CalendarEventPageResult result) {
        if (generation != invitationRefreshGeneration_) {
          return;
        }
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(result)));
          return;
        }
        QVariantList rows;
        const CalendarEventPage page = std::get<CalendarEventPage>(std::move(result));
        for (const CalendarEventSummary& event : page.items) {
          QJsonParseError error;
          const QJsonDocument attendees =
              QJsonDocument::fromJson(event.attendeeDetailsJson.toUtf8(), &error);
          if (error.error != QJsonParseError::NoError || !attendees.isArray()) {
            continue;
          }
          QString response;
          QString comment;
          bool self = false;
          for (const QJsonValue& attendeeValue : attendees.array()) {
            if (!attendeeValue.isObject()) {
              continue;
            }
            const QJsonObject attendee = attendeeValue.toObject();
            if (attendee.value(QStringLiteral("self")).toBool()) {
              self = true;
              response = attendee.value(QStringLiteral("responseStatus")).toString();
              comment = attendee.value(QStringLiteral("comment")).toString();
              break;
            }
          }
          if (!self || response != QStringLiteral("needsAction")) {
            continue;
          }
          rows.append(QVariantMap{{QStringLiteral("eventId"), event.id},
                                  {QStringLiteral("calendarId"), event.calendarId},
                                  {QStringLiteral("title"), event.title},
                                  {QStringLiteral("startAt"), event.startAt},
                                  {QStringLiteral("allDay"), event.allDay},
                                  {QStringLiteral("comment"), comment}});
        }
        setInvitations(std::move(rows));
      },
      false);
}

void AppController::refreshCalendarEvents(QList<QString> calendarIds, std::uint64_t generation) {
  if (generation != calendarRefreshGeneration_) {
    return;
  }
  const QDate date = calendarDate_;
  const QTimeZone displayTimeZone(displayTimeZone_.toUtf8());
  const int firstDay = weekStartDay_;
  const auto applyLayouts = [this, generation](CalendarViewLayouts layouts) {
    if (generation != calendarRefreshGeneration_) {
      return;
    }
    agendaModel_.setEvents(std::move(layouts.agendaEvents));
    timelineModel_.applyLayout(std::move(layouts.timeline));
    monthGridModel_.applyLayout(std::move(layouts.month));
  };
  if (calendarIds.isEmpty()) {
    const QList<CalendarEventSummary> events;
    watch(std::async(std::launch::async,
                     [date, events, displayTimeZone, firstDay]() mutable {
                       return buildCalendarViewLayouts(
                           date, std::move(events), displayTimeZone, firstDay);
                     }),
          applyLayouts);
    return;
  }
  const QList<QString> cacheCalendarIds = calendarIds;
  watch(
      calendarReadService_.listEvents({.calendarIds = std::move(calendarIds),
                                       .startAt = calendarRangeStart(date, firstDay),
                                       .endAt = calendarRangeEnd(date, firstDay),
                                       .limit = 25'000}),
      [this, generation, date, displayTimeZone, firstDay, applyLayouts, cacheCalendarIds](
          CalendarEventPageResult result) {
        if (generation != calendarRefreshGeneration_) {
          return;
        }
        if (std::holds_alternative<AppError>(result)) {
          setStatus(errorMessage(std::get<AppError>(result)));
        } else {
          CalendarEventPage page = std::get<CalendarEventPage>(std::move(result));
          if (page.nextOffset.has_value()) {
            setStatus(QStringLiteral("Calendar range is limited to the first %1 events")
                          .arg(page.items.size()));
          }
          QList<CalendarEventSummary> events = std::move(page.items);
          const qsizetype uncachedSeries =
              std::count_if(events.cbegin(), events.cend(), [](const CalendarEventSummary& event) {
                return event.recurrenceRule.has_value() && !event.instanceRangeCached;
              });
          watch(std::async(std::launch::async,
                           [date, events = std::move(events), displayTimeZone, firstDay]() mutable {
                             return buildCalendarViewLayouts(
                                 date, std::move(events), displayTimeZone, firstDay);
                           }),
                applyLayouts);
          if (uncachedSeries > 0 && !page.nextOffset.has_value()) {
            setStatus(
                googleConnected_
                    ? QStringLiteral("Loading Google recurrence for %1 series").arg(uncachedSeries)
                    : QStringLiteral("Google recurrence cache may be stale for %1 series")
                          .arg(uncachedSeries));
          }
          refreshCalendarInstanceCache(cacheCalendarIds, date, generation);
        }
      });
}

void AppController::refreshCalendarInstanceCache(QList<QString> calendarIds,
                                                 QDate date,
                                                 std::uint64_t generation) {
  if (generation != calendarRefreshGeneration_ || !googleConnected_ ||
      credentialStore_ == nullptr) {
    return;
  }
  const QString rangeStartAt = calendarRangeStart(date, weekStartDay_);
  const QString rangeEndAt = calendarRangeEnd(date, weekStartDay_);
  QList<QString> refreshCalendarIds = calendarIds;
  QList<QString> resultCalendarIds = std::move(calendarIds);
  watch(
      std::async(std::launch::async,
                 [this,
                  calendarIds = std::move(refreshCalendarIds),
                  rangeStartAt,
                  rangeEndAt]() mutable -> GoogleCalendarInstanceCacheRefreshResultOrError {
                   OAuthCredentialReadResult read =
                       credentialStore_->read(QString::fromLatin1(kGoogleAccountId)).get();
                   if (std::holds_alternative<AppError>(read)) {
                     return std::get<AppError>(std::move(read));
                   }
                   std::optional<OAuthStoredCredential> stored =
                       std::get<std::optional<OAuthStoredCredential>>(std::move(read));
                   if (!stored.has_value() || stored->accessToken.isEmpty()) {
                     return AppError(AppErrorCode::Configuration,
                                     QStringLiteral("Google authorization must be renewed"));
                   }
                   return googleCalendarInstanceCacheService_
                       .refresh(QString::fromLatin1(kGoogleAccountId),
                                std::move(stored->accessToken),
                                {.calendarIds = std::move(calendarIds),
                                 .startAt = rangeStartAt,
                                 .endAt = rangeEndAt,
                                 .limit = 1'000})
                       .get();
                 }),
      [this, calendarIds = std::move(resultCalendarIds), date, generation](
          GoogleCalendarInstanceCacheRefreshResultOrError result) mutable {
        if (generation != calendarRefreshGeneration_) {
          return;
        }
        if (std::holds_alternative<AppError>(result)) {
          setStatus(QStringLiteral("Google recurrence cache: %1")
                        .arg(errorMessage(std::get<AppError>(std::move(result)))));
          return;
        }
        GoogleCalendarInstanceCacheRefreshResult refreshed =
            std::get<GoogleCalendarInstanceCacheRefreshResult>(std::move(result));
        if (refreshed.failed > 0) {
          setStatus(QStringLiteral("Google recurrence cache: %1 of %2 series unavailable%3")
                        .arg(refreshed.failed)
                        .arg(refreshed.requested)
                        .arg(refreshed.firstFailure.has_value()
                                 ? QStringLiteral(" (%1)").arg(*refreshed.firstFailure)
                                 : QString()));
        }
        if (refreshed.cached > 0 && date == calendarDate_) {
          refreshCalendarEvents(std::move(calendarIds), generation);
        }
      },
      false);
}

void AppController::setStatus(QString message) {
  if (statusMessage_ == message) {
    return;
  }
  statusMessage_ = std::move(message);
  emit statusMessageChanged();
}

void AppController::setTaskListError(QString message) {
  if (taskListErrorMessage_ == message) {
    return;
  }
  taskListErrorMessage_ = std::move(message);
  emit taskListErrorMessageChanged();
}

void AppController::setSyncStatus(QString status) {
  if (QThread::currentThread() != thread()) {
    static_cast<void>(QMetaObject::invokeMethod(
        this,
        [this, status = std::move(status)]() mutable { setSyncStatus(std::move(status)); },
        Qt::QueuedConnection));
    return;
  }
  if (syncStatus_ == status) {
    return;
  }
  syncStatus_ = std::move(status);
  emit syncStatusChanged();
  refreshPendingSyncCount();
}

void AppController::setUnresolvedConflicts(QList<SyncConflict> conflicts) {
  if (QThread::currentThread() != thread()) {
    static_cast<void>(QMetaObject::invokeMethod(
        this,
        [this, conflicts = std::move(conflicts)]() mutable {
          setUnresolvedConflicts(std::move(conflicts));
        },
        Qt::QueuedConnection));
    return;
  }
  QVariantList rows = conflictRows(std::move(conflicts));
  if (unresolvedConflicts_ == rows) {
    return;
  }
  unresolvedConflicts_ = std::move(rows);
  emit unresolvedConflictsChanged();
}

void AppController::setResolvedConflicts(QList<SyncConflict> conflicts) {
  if (QThread::currentThread() != thread()) {
    static_cast<void>(QMetaObject::invokeMethod(
        this,
        [this, conflicts = std::move(conflicts)]() mutable {
          setResolvedConflicts(std::move(conflicts));
        },
        Qt::QueuedConnection));
    return;
  }
  QVariantList rows = conflictRows(std::move(conflicts));
  if (resolvedConflicts_ == rows) {
    return;
  }
  resolvedConflicts_ = std::move(rows);
  emit resolvedConflictsChanged();
}

void AppController::setSearchError(QString message) {
  if (searchErrorMessage_ == message) {
    return;
  }
  searchErrorMessage_ = std::move(message);
  emit searchErrorMessageChanged();
}

void AppController::setSearchFilterChips(QStringList chips) {
  QVariantList values;
  values.reserve(chips.size());
  for (QString& chip : chips) {
    values.append(std::move(chip));
  }
  if (searchFilterChips_ == values) {
    return;
  }
  searchFilterChips_ = std::move(values);
  emit searchFilterChipsChanged();
}

void AppController::setSavedSearches(QList<SavedSearch> searches) {
  QVariantList rows = savedSearchRows(searches);
  if (savedSearchRows_ == rows) {
    return;
  }
  savedSearches_ = std::move(searches);
  savedSearchRows_ = std::move(rows);
  emit savedSearchesChanged();
}

void AppController::setSearchLoading(bool loading) {
  if (searchLoading_ == loading) {
    return;
  }
  searchLoading_ = loading;
  emit searchLoadingChanged();
}

void AppController::setBulkTaskStatusMessage(QString message) {
  if (bulkTaskStatusMessage_ == message) {
    return;
  }
  bulkTaskStatusMessage_ = std::move(message);
  emit bulkTaskStatusMessageChanged();
}

void AppController::setBulkEventStatusMessage(QString message) {
  if (bulkEventStatusMessage_ == message) {
    return;
  }
  bulkEventStatusMessage_ = std::move(message);
  emit bulkEventStatusMessageChanged();
}

void AppController::setBulkTaskPreviewMessage(QString message, int requestToken) {
  if (bulkTaskPreviewMessage_ != message) {
    bulkTaskPreviewMessage_ = std::move(message);
    emit bulkTaskPreviewMessageChanged();
  }
  if (bulkTaskPreviewRequestToken_ != requestToken) {
    bulkTaskPreviewRequestToken_ = requestToken;
    emit bulkTaskPreviewRequestTokenChanged();
  }
}

void AppController::setBulkEventPreviewMessage(QString message, int requestToken) {
  if (bulkEventPreviewMessage_ != message) {
    bulkEventPreviewMessage_ = std::move(message);
    emit bulkEventPreviewMessageChanged();
  }
  if (bulkEventPreviewRequestToken_ != requestToken) {
    bulkEventPreviewRequestToken_ = requestToken;
    emit bulkEventPreviewRequestTokenChanged();
  }
}

void AppController::setReminderStatusMessage(QString message) {
  if (reminderStatusMessage_ == message) {
    return;
  }
  reminderStatusMessage_ = std::move(message);
  emit reminderStatusMessageChanged();
}

void AppController::setInvitations(QVariantList invitations) {
  if (invitations_ == invitations) {
    return;
  }
  invitations_ = std::move(invitations);
  emit invitationsChanged();
}

void AppController::setCalendarManagementRows(QVariantList rows) {
  if (calendarManagementRows_ == rows) {
    return;
  }
  calendarManagementRows_ = std::move(rows);
  emit calendarManagementRowsChanged();
}

void AppController::setImportPreviewRows(QVariantList rows) {
  if (importPreviewRows_ == rows) {
    return;
  }
  importPreviewRows_ = std::move(rows);
  emit importPreviewRowsChanged();
}

void AppController::setImportSourceName(QString sourceName) {
  if (importSourceName_ == sourceName) {
    return;
  }
  importSourceName_ = std::move(sourceName);
  emit importSourceNameChanged();
}

void AppController::setImportReadyToCommit(bool ready) {
  if (importReadyToCommit_ == ready) {
    return;
  }
  importReadyToCommit_ = ready;
  emit importReadyToCommitChanged();
}

void AppController::setBusy(bool busy) {
  if (busy_ == busy) {
    return;
  }
  busy_ = busy;
  emit busyChanged();
}

} // namespace hcb
