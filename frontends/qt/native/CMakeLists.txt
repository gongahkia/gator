set(HCB_MACOS_CREDENTIAL_SOURCES
  src/app/MacOSCredentialAdapter.cpp
  src/app/MacOSCredentialAdapter.h
)
set(HCB_WINDOWS_CREDENTIAL_SOURCES
  src/app/WindowsCredentialAdapter.cpp
  src/app/WindowsCredentialAdapter.h
)
set(HCB_LINUX_CREDENTIAL_SOURCES
  src/app/LinuxCredentialAdapter.cpp
  src/app/LinuxCredentialAdapter.h
)
set(HCB_SYSTEM_TRAY_SOURCES
  src/app/SystemTrayAdapter.cpp
  src/app/SystemTrayAdapter.h
)
set(HCB_NOTIFICATION_SOURCES
  src/app/NotificationAdapter.cpp
  src/app/NotificationAdapter.h
)
set(HCB_DEEP_LINK_SOURCES
  src/app/DeepLinkAdapter.cpp
  src/app/DeepLinkAdapter.h
)
set(HCB_SYSTEM_TRAY_ICON "${CMAKE_CURRENT_SOURCE_DIR}/../assets/brand/buns-app-icon.png")
set_source_files_properties("${HCB_SYSTEM_TRAY_ICON}" PROPERTIES QT_RESOURCE_ALIAS "tray-icon.png")
if(APPLE)
  find_library(HCB_SECURITY_FRAMEWORK Security REQUIRED)
  find_library(HCB_USER_NOTIFICATIONS_FRAMEWORK UserNotifications REQUIRED)
  set(HCB_REMINDER_SOURCES
    src/app/NativeReminderNotifier.mm
    src/app/NativeReminderNotifier.h
  )
  set_source_files_properties(src/app/NativeReminderNotifier.mm PROPERTIES COMPILE_OPTIONS "-fobjc-arc")
else()
  set(HCB_REMINDER_SOURCES
    src/app/NativeReminderNotifier.cpp
    src/app/NativeReminderNotifier.h
  )
endif()

set(HCB_NATIVE_SOURCES
  src/app/ApplicationLifecycle.h
  src/app/AppController.cpp
  src/app/AppController.h
  src/app/AppPaths.cpp
  src/app/AppPaths.h
  src/app/AppServices.cpp
  src/app/AppServices.h
  ${HCB_DEEP_LINK_SOURCES}
  ${HCB_LINUX_CREDENTIAL_SOURCES}
  src/app/LinuxPathsAdapter.cpp
  src/app/LinuxPathsAdapter.h
  ${HCB_MACOS_CREDENTIAL_SOURCES}
  src/app/MacOSPathsAdapter.cpp
  src/app/MacOSPathsAdapter.h
  ${HCB_NOTIFICATION_SOURCES}
  ${HCB_REMINDER_SOURCES}
  ${HCB_SYSTEM_TRAY_SOURCES}
  ${HCB_WINDOWS_CREDENTIAL_SOURCES}
  src/app/WindowsPathsAdapter.cpp
  src/app/WindowsPathsAdapter.h
  src/core/AccountStatusService.cpp
  src/core/AccountStatusService.h
  src/core/AgendaModel.cpp
  src/core/AgendaModel.h
  src/core/AppError.h
  src/core/CalendarLayoutEngine.cpp
  src/core/CalendarLayoutEngine.h
  src/core/NativeCalendarNavigationBenchmark.cpp
  src/core/NativeCalendarNavigationBenchmark.h
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
  src/core/CalendarReadService.cpp
  src/core/CalendarReadService.h
  src/core/CalendarSourceModel.cpp
  src/core/CalendarSourceModel.h
  src/core/CalendarEventBulkMutationService.cpp
  src/core/CalendarEventBulkMutationService.h
  src/core/CalendarMutationService.cpp
  src/core/CalendarMutationService.h
  src/core/Cancellation.h
  src/core/Clock.h
  src/core/CommandRegistryModel.cpp
  src/core/CommandRegistryModel.h
  src/core/CredentialHealthDiagnostics.cpp
  src/core/CredentialHealthDiagnostics.h
  src/core/DiagnosticsJsonExporter.cpp
  src/core/DiagnosticsJsonExporter.h
  src/core/DiagnosticsSnapshot.cpp
  src/core/DiagnosticsSnapshot.h
  src/core/FilePath.h
  src/core/FeatureCapabilityRegistry.h
  src/core/GoogleApiError.cpp
  src/core/GoogleApiError.h
  src/core/GoogleHttpClient.cpp
  src/core/GoogleHttpClient.h
  src/core/ImportService.cpp
  src/core/ImportService.h
  src/core/ImportMutationService.cpp
  src/core/ImportMutationService.h
  src/core/GoogleMirrorStore.cpp
  src/core/GoogleMirrorStore.h
  src/core/GoogleTaskMirrorSyncService.cpp
  src/core/GoogleTaskMirrorSyncService.h
  src/core/GoogleCalendarMirrorSyncService.cpp
  src/core/GoogleCalendarMirrorSyncService.h
  src/core/GoogleSyncRecoveryService.cpp
  src/core/GoogleSyncRecoveryService.h
  src/core/GoogleSyncConflictResolver.cpp
  src/core/GoogleSyncConflictResolver.h
  src/core/GoogleCalendarListPullClient.cpp
  src/core/GoogleCalendarListPullClient.h
  src/core/GoogleCalendarManagementClient.cpp
  src/core/GoogleCalendarManagementClient.h
  src/core/GoogleCalendarFreeBusyClient.cpp
  src/core/GoogleCalendarFreeBusyClient.h
  src/core/GoogleDriveFilePickerClient.cpp
  src/core/GoogleDriveFilePickerClient.h
  src/core/GoogleCalendarEventPullClient.cpp
  src/core/GoogleCalendarEventPullClient.h
  src/core/GoogleCalendarInstanceCacheService.cpp
  src/core/GoogleCalendarInstanceCacheService.h
  src/core/GoogleCalendarEventMutationPushService.cpp
  src/core/GoogleCalendarEventMutationPushService.h
  src/core/GoogleTaskListPullClient.cpp
  src/core/GoogleTaskListPullClient.h
  src/core/GoogleTaskMutationPushService.cpp
  src/core/GoogleTaskMutationPushService.h
  src/core/GoogleTaskPullClient.cpp
  src/core/GoogleTaskPullClient.h
  src/core/NativeProcessMemory.cpp
  src/core/NativeProcessMemory.h
  src/core/NativeTaskScrollBenchmark.cpp
  src/core/NativeTaskScrollBenchmark.h
  src/core/NativeLocalSearchBenchmark.cpp
  src/core/NativeLocalSearchBenchmark.h
  src/core/NativeQuickCaptureBenchmark.cpp
  src/core/NativeQuickCaptureBenchmark.h
  src/core/LocalSearchService.cpp
  src/core/LocalSearchService.h
  src/core/LocalSearchQuery.cpp
  src/core/LocalSearchQuery.h
  src/core/MonthGridModel.cpp
  src/core/MonthGridModel.h
  src/core/ScheduledTaskDateIndex.cpp
  src/core/ScheduledTaskDateIndex.h
  src/core/ModelDiffPolicy.h
  src/core/MutationTelemetryStore.cpp
  src/core/MutationTelemetryStore.h
  src/core/NotesModel.cpp
  src/core/NotesModel.h
  src/core/OAuthBrowserAuthorizationLauncher.cpp
  src/core/OAuthBrowserAuthorizationLauncher.h
  src/core/OAuthClientConfigurationStore.cpp
  src/core/OAuthClientConfigurationStore.h
  src/core/OAuthCredentialStore.h
  src/core/OAuthDisconnectService.cpp
  src/core/OAuthDisconnectService.h
  src/core/OAuthLoopbackCallbackListener.cpp
  src/core/OAuthLoopbackCallbackListener.h
  src/core/OAuthTokenExchangeClient.cpp
  src/core/OAuthTokenExchangeClient.h
  src/core/OAuthTokenRefreshClient.cpp
  src/core/OAuthTokenRefreshClient.h
  src/core/OAuthTokenRevocationClient.cpp
  src/core/OAuthTokenRevocationClient.h
  src/core/OAuthTokenRevoker.h
  src/core/OptimisticMutationCoordinator.cpp
  src/core/OptimisticMutationCoordinator.h
  src/core/NativeCapabilityReport.cpp
  src/core/NativeCapabilityReport.h
  src/core/PkceAuthorization.cpp
  src/core/PkceAuthorization.h
  src/core/QuickCaptureParser.cpp
  src/core/QuickCaptureParser.h
  src/core/RecurrenceExpansionWorker.cpp
  src/core/RecurrenceExpansionWorker.h
  src/core/RecurrenceResultCache.cpp
  src/core/RecurrenceResultCache.h
  src/core/ReminderService.cpp
  src/core/ReminderService.h
  src/core/SearchResultsModel.cpp
  src/core/SearchResultsModel.h
  src/core/SavedSearchStore.cpp
  src/core/SavedSearchStore.h
  src/core/SettingsRegistry.h
  src/core/SettingsService.cpp
  src/core/SettingsService.h
  src/core/SecretRedactor.cpp
  src/core/SecretRedactor.h
  src/core/StructuredLogger.cpp
  src/core/StructuredLogger.h
  src/core/SyncBackoffPolicy.cpp
  src/core/SyncBackoffPolicy.h
  src/core/SyncCheckpointStore.cpp
  src/core/SyncCheckpointStore.h
  src/core/SyncConflictStore.cpp
  src/core/SyncConflictStore.h
  src/core/SyncThreeWayMerge.cpp
  src/core/SyncThreeWayMerge.h
  src/core/SyncScheduler.cpp
  src/core/SyncScheduler.h
  src/core/TaskListReadService.cpp
  src/core/TaskListReadService.h
  src/core/TaskListModel.cpp
  src/core/TaskListModel.h
  src/core/TaskListMutationService.cpp
  src/core/TaskListMutationService.h
  src/core/TaskBulkMutationService.cpp
  src/core/TaskBulkMutationService.h
  src/core/TaskModel.cpp
  src/core/TaskModel.h
  src/core/TaskMutationService.cpp
  src/core/TaskMutationService.h
  src/core/TaskRecurrenceMarker.cpp
  src/core/TaskRecurrenceMarker.h
  src/core/TaskReadService.cpp
  src/core/TaskReadService.h
  src/core/TimelineModel.cpp
  src/core/TimelineModel.h
  src/core/TimelineViewportModel.cpp
  src/core/TimelineViewportModel.h
  src/core/UndoRecoveryPolicy.cpp
  src/core/UndoRecoveryPolicy.h
  src/core/StartupTimingTracker.cpp
  src/core/StartupTimingTracker.h
  src/core/UiTransitionTimingTracker.cpp
  src/core/UiTransitionTimingTracker.h
  src/core/UnifiedLocalSearchRanker.cpp
  src/core/UnifiedLocalSearchRanker.h
  src/data/SqliteConnection.cpp
  src/data/SqliteConnection.h
  src/data/LocalSchema.cpp
  src/data/LocalSchema.h
  src/data/SqliteMigrationRunner.cpp
  src/data/SqliteMigrationRunner.h
  src/data/SqliteQueryTimingTracker.cpp
  src/data/SqliteQueryTimingTracker.h
  src/data/SqliteReadConnectionPool.cpp
  src/data/SqliteReadConnectionPool.h
  src/data/SqliteStatementCache.cpp
  src/data/SqliteStatementCache.h
  src/data/SqliteTransaction.cpp
  src/data/SqliteTransaction.h
  src/data/SqliteWriterQueue.cpp
  src/data/SqliteWriterQueue.h
)

qt_add_executable(hcb_native
  MACOSX_BUNDLE
  WIN32
  src/main.cpp
  ${HCB_NATIVE_SOURCES}
)
qt_add_resources(hcb_native "hcb_native_system_tray_resources"
  PREFIX "/hcb"
  FILES "${HCB_SYSTEM_TRAY_ICON}"
)
if(APPLE)
  set_target_properties(hcb_native PROPERTIES
    MACOSX_BUNDLE_INFO_PLIST "${CMAKE_CURRENT_SOURCE_DIR}/resources/Info.plist.in"
  )
endif()

set_source_files_properties(qml/Theme.qml PROPERTIES QT_QML_SINGLETON_TYPE TRUE)

qt_add_qml_module(hcb_native
  URI HCB
  VERSION 1.0
  QML_FILES
    qml/AccessibleButton.qml
    qml/AccessibleNavigationButton.qml
    qml/AgendaView.qml
    qml/CalendarBulkControls.qml
    qml/CalendarEventButton.qml
    qml/CalendarQuickCreateDialog.qml
    qml/CalendarSourceControls.qml
  qml/CalendarTaskButton.qml
  qml/UnscheduledTaskTray.qml
    qml/BulkTextReplaceDialog.qml
    qml/DayTimelineView.qml
    qml/DateEditor.qml
    qml/DateTimeEditor.qml
    qml/EventBulkDeleteDialog.qml
    qml/EventBulkEditDialog.qml
    qml/EventBulkMoveDialog.qml
    qml/EventCreateDialog.qml
    qml/EventColorPicker.qml
    qml/EmojiAutocomplete.qml
    qml/EmojiTextArea.qml
    qml/EmojiTextField.qml
    qml/EventDeleteDialog.qml
    qml/EventDetailDialog.qml
    qml/EventEditDialog.qml
    qml/EventMutationScopeDialog.qml
    qml/ExternalLinkText.qml
    qml/GoogleOnboardingView.qml
    qml/HcbDialog.qml
    qml/InvitationInbox.qml
    qml/Main.qml
    qml/MonthGridView.qml
    qml/NavigationSidebar.qml
    qml/NoteEditorDialog.qml
    qml/NotesListView.qml
    qml/QuickCaptureDialog.qml
    qml/ReminderEditor.qml
    qml/SearchPopup.qml
    qml/StatusEventPropertiesEditor.qml
    qml/TaskCreateDialog.qml
    qml/TaskDeleteDialog.qml
    qml/TaskDetailDialog.qml
    qml/TaskBulkDeleteDialog.qml
    qml/TaskBulkEditDialog.qml
    qml/TaskBulkMoveDialog.qml
    qml/TaskEditDialog.qml
    qml/TaskRecurrenceActionDialog.qml
    qml/TimeZonePicker.qml
    qml/TaskListView.qml
    qml/TaskListControls.qml
    qml/TaskListDeleteDialog.qml
    qml/TaskListEditorDialog.qml
    qml/TaskMoveDialog.qml
    qml/Theme.qml
    qml/WeekTimelineView.qml
  RESOURCES
    qml/EmojiCatalog.js
    qml/EmojiUnicode16.js
)

target_link_libraries(hcb_native PRIVATE
  Qt6::Core
  Qt6::Gui
  Qt6::Network
  Qt6::Qml
  Qt6::Quick
  Qt6::QuickControls2
  Qt6::Widgets
  hcb::sqlite
)
if(APPLE)
  target_link_libraries(hcb_native PRIVATE ${HCB_SECURITY_FRAMEWORK} ${HCB_USER_NOTIFICATIONS_FRAMEWORK})
endif()
if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
  target_link_libraries(hcb_native PRIVATE Qt6::DBus)
endif()

target_include_directories(hcb_native PRIVATE src)
hcb_enable_warnings(hcb_native)
hcb_enable_sanitizers(hcb_native)
if(WIN32)
  target_link_libraries(hcb_native PRIVATE Advapi32 Psapi)
endif()

if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
  qt_add_executable(hcb_reminderd
    src/reminder/ReminderDaemonMain.cpp
    src/app/AppPaths.cpp
    src/app/AppPaths.h
    src/app/LinuxPathsAdapter.cpp
    src/app/LinuxPathsAdapter.h
    src/app/NativeReminderNotifier.cpp
    src/app/NativeReminderNotifier.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/ReminderService.cpp
    src/core/ReminderService.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
  )
  target_link_libraries(hcb_reminderd PRIVATE hcb::sqlite Qt6::Core Qt6::DBus)
  target_include_directories(hcb_reminderd PRIVATE src)
  hcb_enable_warnings(hcb_reminderd)
  hcb_enable_sanitizers(hcb_reminderd)
endif()

qt_add_executable(hcb_launch_benchmark
  benchmarks/NativeLaunchBenchmarkMain.cpp
  src/core/NativeLaunchBenchmark.cpp
  src/core/NativeLaunchBenchmark.h
)
target_link_libraries(hcb_launch_benchmark PRIVATE Qt6::Core)
target_include_directories(hcb_launch_benchmark PRIVATE src)
hcb_enable_warnings(hcb_launch_benchmark)
hcb_enable_sanitizers(hcb_launch_benchmark)

qt_add_executable(hcb_idle_rss_benchmark
  benchmarks/NativeIdleRssBenchmarkMain.cpp
  src/core/NativeIdleRssBenchmark.cpp
  src/core/NativeIdleRssBenchmark.h
)
target_link_libraries(hcb_idle_rss_benchmark PRIVATE Qt6::Core)
target_include_directories(hcb_idle_rss_benchmark PRIVATE src)
hcb_enable_warnings(hcb_idle_rss_benchmark)
hcb_enable_sanitizers(hcb_idle_rss_benchmark)

qt_add_executable(hcb_command_palette_benchmark
  benchmarks/NativeCommandPaletteBenchmarkMain.cpp
  src/core/NativeCommandPaletteBenchmark.cpp
  src/core/NativeCommandPaletteBenchmark.h
)
target_link_libraries(hcb_command_palette_benchmark PRIVATE Qt6::Core)
target_include_directories(hcb_command_palette_benchmark PRIVATE src)
hcb_enable_warnings(hcb_command_palette_benchmark)
hcb_enable_sanitizers(hcb_command_palette_benchmark)

qt_add_executable(hcb_small_performance_fixture
  benchmarks/NativeSmallPerformanceFixtureMain.cpp
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
)
target_link_libraries(hcb_small_performance_fixture PRIVATE Qt6::Core)
target_include_directories(hcb_small_performance_fixture PRIVATE src)
hcb_enable_warnings(hcb_small_performance_fixture)
hcb_enable_sanitizers(hcb_small_performance_fixture)

qt_add_executable(hcb_medium_performance_fixture
  benchmarks/NativeMediumPerformanceFixtureMain.cpp
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
)
target_link_libraries(hcb_medium_performance_fixture PRIVATE Qt6::Core)
target_include_directories(hcb_medium_performance_fixture PRIVATE src)
hcb_enable_warnings(hcb_medium_performance_fixture)
hcb_enable_sanitizers(hcb_medium_performance_fixture)

qt_add_executable(hcb_event15k_performance_fixture
  benchmarks/Native15kEventPerformanceFixtureMain.cpp
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
)
target_link_libraries(hcb_event15k_performance_fixture PRIVATE Qt6::Core)
target_include_directories(hcb_event15k_performance_fixture PRIVATE src)
hcb_enable_warnings(hcb_event15k_performance_fixture)
hcb_enable_sanitizers(hcb_event15k_performance_fixture)

qt_add_executable(hcb_wrapper_scale_performance_fixture
  benchmarks/NativeWrapperScalePerformanceFixtureMain.cpp
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
)
target_link_libraries(hcb_wrapper_scale_performance_fixture PRIVATE Qt6::Core)
target_include_directories(hcb_wrapper_scale_performance_fixture PRIVATE src)
hcb_enable_warnings(hcb_wrapper_scale_performance_fixture)
hcb_enable_sanitizers(hcb_wrapper_scale_performance_fixture)

qt_add_executable(hcb_planner_benchmark
  benchmarks/NativePlannerBenchmarkMain.cpp
  src/core/CalendarLayoutEngine.cpp
  src/core/CalendarLayoutEngine.h
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
  src/core/NativePlannerBenchmark.cpp
  src/core/NativePlannerBenchmark.h
)
target_link_libraries(hcb_planner_benchmark PRIVATE Qt6::Core)
target_include_directories(hcb_planner_benchmark PRIVATE src)
hcb_enable_warnings(hcb_planner_benchmark)
hcb_enable_sanitizers(hcb_planner_benchmark)

qt_add_executable(hcb_quick_capture_benchmark
  benchmarks/NativeQuickCaptureBenchmarkMain.cpp
  src/core/AppError.h
  src/core/Clock.h
  src/core/FilePath.h
  src/core/NativeQuickCaptureBenchmark.cpp
  src/core/NativeQuickCaptureBenchmark.h
  src/core/TaskMutationService.cpp
  src/core/TaskMutationService.h
  src/core/TaskRecurrenceMarker.cpp
  src/core/TaskRecurrenceMarker.h
  src/data/LocalSchema.cpp
  src/data/LocalSchema.h
  src/data/SqliteConnection.cpp
  src/data/SqliteConnection.h
  src/data/SqliteMigrationRunner.cpp
  src/data/SqliteMigrationRunner.h
  src/data/SqliteQueryTimingTracker.cpp
  src/data/SqliteQueryTimingTracker.h
  src/data/SqliteTransaction.cpp
  src/data/SqliteTransaction.h
  src/data/SqliteWriterQueue.cpp
  src/data/SqliteWriterQueue.h
)
target_link_libraries(hcb_quick_capture_benchmark PRIVATE hcb::sqlite Qt6::Core)
target_include_directories(hcb_quick_capture_benchmark PRIVATE src)
hcb_enable_warnings(hcb_quick_capture_benchmark)
hcb_enable_sanitizers(hcb_quick_capture_benchmark)

qt_add_executable(hcb_local_search_benchmark
  benchmarks/NativeLocalSearchBenchmarkMain.cpp
  src/core/AppError.h
  src/core/FilePath.h
  src/core/LocalSearchService.cpp
  src/core/LocalSearchService.h
  src/core/LocalSearchQuery.cpp
  src/core/LocalSearchQuery.h
  src/core/NativeLocalSearchBenchmark.cpp
  src/core/NativeLocalSearchBenchmark.h
  src/core/TaskRecurrenceMarker.cpp
  src/core/TaskRecurrenceMarker.h
  src/core/UnifiedLocalSearchRanker.cpp
  src/core/UnifiedLocalSearchRanker.h
  src/data/LocalSchema.cpp
  src/data/LocalSchema.h
  src/data/SqliteConnection.cpp
  src/data/SqliteConnection.h
  src/data/SqliteMigrationRunner.cpp
  src/data/SqliteMigrationRunner.h
  src/data/SqliteQueryTimingTracker.cpp
  src/data/SqliteQueryTimingTracker.h
  src/data/SqliteReadConnectionPool.cpp
  src/data/SqliteReadConnectionPool.h
  src/data/SqliteTransaction.cpp
  src/data/SqliteTransaction.h
)
target_link_libraries(hcb_local_search_benchmark PRIVATE hcb::sqlite Qt6::Core)
target_include_directories(hcb_local_search_benchmark PRIVATE src)
hcb_enable_warnings(hcb_local_search_benchmark)
hcb_enable_sanitizers(hcb_local_search_benchmark)

qt_add_executable(hcb_task_scroll_benchmark
  benchmarks/NativeTaskScrollBenchmarkMain.cpp
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
  src/core/NativeTaskScrollBenchmark.cpp
  src/core/NativeTaskScrollBenchmark.h
  src/core/TaskModel.cpp
  src/core/TaskModel.h
  src/core/TaskMutationService.h
)
target_link_libraries(hcb_task_scroll_benchmark PRIVATE Qt6::Core Qt6::Gui Qt6::Qml Qt6::Quick)
target_include_directories(hcb_task_scroll_benchmark PRIVATE src)
hcb_enable_warnings(hcb_task_scroll_benchmark)
hcb_enable_sanitizers(hcb_task_scroll_benchmark)

qt_add_executable(hcb_calendar_navigation_benchmark
  benchmarks/NativeCalendarNavigationBenchmarkMain.cpp
  src/core/AppError.h
  src/core/FilePath.h
  src/core/MonthGridModel.cpp
  src/core/MonthGridModel.h
  src/core/NativeCalendarNavigationBenchmark.cpp
  src/core/NativeCalendarNavigationBenchmark.h
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
  src/core/CalendarReadService.h
  src/data/SqliteWriterQueue.h
)
target_link_libraries(hcb_calendar_navigation_benchmark PRIVATE Qt6::Core Qt6::Gui Qt6::Qml Qt6::Quick)
target_include_directories(hcb_calendar_navigation_benchmark PRIVATE src)
hcb_enable_warnings(hcb_calendar_navigation_benchmark)
hcb_enable_sanitizers(hcb_calendar_navigation_benchmark)

qt_add_executable(hcb_sync_apply_benchmark
  benchmarks/NativeSyncApplyBenchmarkMain.cpp
  src/core/AppError.h
  src/core/Clock.h
  src/core/FilePath.h
  src/core/GoogleCalendarEventPullClient.h
  src/core/GoogleCalendarListPullClient.h
  src/core/GoogleMirrorStore.cpp
  src/core/GoogleMirrorStore.h
  src/core/GoogleTaskListPullClient.h
  src/core/GoogleTaskPullClient.h
  src/core/NativePerformanceFixture.cpp
  src/core/NativePerformanceFixture.h
  src/core/NativeSyncApplyBenchmark.cpp
  src/core/NativeSyncApplyBenchmark.h
  src/core/TaskRecurrenceMarker.cpp
  src/core/TaskRecurrenceMarker.h
  src/data/LocalSchema.cpp
  src/data/LocalSchema.h
  src/data/SqliteConnection.cpp
  src/data/SqliteConnection.h
  src/data/SqliteMigrationRunner.cpp
  src/data/SqliteMigrationRunner.h
  src/data/SqliteQueryTimingTracker.cpp
  src/data/SqliteQueryTimingTracker.h
  src/data/SqliteStatementCache.cpp
  src/data/SqliteStatementCache.h
  src/data/SqliteTransaction.cpp
  src/data/SqliteTransaction.h
  src/data/SqliteWriterQueue.cpp
  src/data/SqliteWriterQueue.h
)
target_link_libraries(hcb_sync_apply_benchmark PRIVATE hcb::sqlite Qt6::Core)
target_include_directories(hcb_sync_apply_benchmark PRIVATE src)
hcb_enable_warnings(hcb_sync_apply_benchmark)
hcb_enable_sanitizers(hcb_sync_apply_benchmark)

qt_add_executable(hcb_performance_report
  benchmarks/NativePerformanceReportMain.cpp
  src/core/NativeIdleRssBenchmark.cpp
  src/core/NativeIdleRssBenchmark.h
  src/core/NativeLaunchBenchmark.cpp
  src/core/NativeLaunchBenchmark.h
  src/core/NativePerformanceReport.cpp
  src/core/NativePerformanceReport.h
)
target_link_libraries(hcb_performance_report PRIVATE Qt6::Core)
target_include_directories(hcb_performance_report PRIVATE src)
hcb_enable_warnings(hcb_performance_report)
hcb_enable_sanitizers(hcb_performance_report)

set_target_properties(hcb_native PROPERTIES
  OUTPUT_NAME "Hot Cross Buns"
  WIN32_EXECUTABLE TRUE
  MACOSX_BUNDLE TRUE
)

if(BUILD_TESTING)
  add_test(NAME hcb_native_dependency_manifest_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      "-DHCB_MANIFEST=${HCB_NATIVE_DEPENDENCY_MANIFEST_PATH}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/DependencyManifestTest.cmake"
  )
  add_test(NAME hcb_native_dependency_manifest_rejects_invalid_hash
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      "-DHCB_MANIFEST=${CMAKE_CURRENT_SOURCE_DIR}/tests/fixtures/invalid-dependency-manifest.json"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/DependencyManifestInvalidHashTest.cmake"
  )
  add_test(NAME hcb_macos_universal_package_config_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/MacosUniversalPackageConfigTest.cmake"
  )
  add_test(NAME hcb_linux_x64_package_config_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/LinuxX64PackageConfigTest.cmake"
  )
  add_test(NAME hcb_fedora_rpm_config_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/FedoraRpmConfigTest.cmake"
  )
  add_test(NAME hcb_windows_x64_package_config_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/WindowsX64PackageConfigTest.cmake"
  )
  add_test(NAME hcb_package_install_smoke_config_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/PackageInstallSmokeConfigTest.cmake"
  )
  add_test(NAME hcb_native_release_acceptance_config_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/NativeReleaseAcceptanceConfigTest.cmake"
  )
  add_test(NAME hcb_github_actions_workflow_context_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/GitHubActionsWorkflowContextTest.cmake"
  )
  add_test(NAME hcb_electron_cutover_removal_tests
    COMMAND ${CMAKE_COMMAND}
      "-DHCB_SOURCE_DIR=${PROJECT_SOURCE_DIR}"
      -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/ElectronCutoverRemovalTest.cmake"
  )

  qt_add_executable(hcb_sqlite_integration_tests tests/SqliteIntegrationTest.cpp)
  target_link_libraries(hcb_sqlite_integration_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  hcb_enable_warnings(hcb_sqlite_integration_tests)
  hcb_enable_sanitizers(hcb_sqlite_integration_tests)
  add_test(NAME hcb_sqlite_integration_tests COMMAND hcb_sqlite_integration_tests)

  qt_add_executable(hcb_sqlite_connection_tests
    tests/SqliteConnectionTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
  )
  target_link_libraries(hcb_sqlite_connection_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sqlite_connection_tests PRIVATE src)
  hcb_enable_warnings(hcb_sqlite_connection_tests)
  hcb_enable_sanitizers(hcb_sqlite_connection_tests)
  add_test(NAME hcb_sqlite_connection_tests COMMAND hcb_sqlite_connection_tests)

  qt_add_executable(hcb_sqlite_writer_queue_tests
    tests/SqliteWriterQueueTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_sqlite_writer_queue_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sqlite_writer_queue_tests PRIVATE src)
  hcb_enable_warnings(hcb_sqlite_writer_queue_tests)
  hcb_enable_sanitizers(hcb_sqlite_writer_queue_tests)
  add_test(NAME hcb_sqlite_writer_queue_tests COMMAND hcb_sqlite_writer_queue_tests)

  qt_add_executable(hcb_sqlite_read_connection_pool_tests
    tests/SqliteReadConnectionPoolTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteReadConnectionPool.cpp
    src/data/SqliteReadConnectionPool.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
  )
  target_link_libraries(hcb_sqlite_read_connection_pool_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sqlite_read_connection_pool_tests PRIVATE src)
  hcb_enable_warnings(hcb_sqlite_read_connection_pool_tests)
  hcb_enable_sanitizers(hcb_sqlite_read_connection_pool_tests)
  add_test(NAME hcb_sqlite_read_connection_pool_tests COMMAND hcb_sqlite_read_connection_pool_tests)

  qt_add_executable(hcb_sqlite_transaction_tests
    tests/SqliteTransactionTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
  )
  target_link_libraries(hcb_sqlite_transaction_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sqlite_transaction_tests PRIVATE src)
  hcb_enable_warnings(hcb_sqlite_transaction_tests)
  hcb_enable_sanitizers(hcb_sqlite_transaction_tests)
  add_test(NAME hcb_sqlite_transaction_tests COMMAND hcb_sqlite_transaction_tests)

  qt_add_executable(hcb_sqlite_statement_cache_tests
    tests/SqliteStatementCacheTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteStatementCache.cpp
    src/data/SqliteStatementCache.h
  )
  target_link_libraries(hcb_sqlite_statement_cache_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sqlite_statement_cache_tests PRIVATE src)
  hcb_enable_warnings(hcb_sqlite_statement_cache_tests)
  hcb_enable_sanitizers(hcb_sqlite_statement_cache_tests)
  add_test(NAME hcb_sqlite_statement_cache_tests COMMAND hcb_sqlite_statement_cache_tests)

  qt_add_executable(hcb_sqlite_migration_runner_tests
    tests/SqliteMigrationRunnerTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
  )
  target_link_libraries(hcb_sqlite_migration_runner_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sqlite_migration_runner_tests PRIVATE src)
  hcb_enable_warnings(hcb_sqlite_migration_runner_tests)
  hcb_enable_sanitizers(hcb_sqlite_migration_runner_tests)
  add_test(NAME hcb_sqlite_migration_runner_tests COMMAND hcb_sqlite_migration_runner_tests)

  qt_add_executable(hcb_sqlite_query_timing_tracker_tests
    tests/SqliteQueryTimingTrackerTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    tests/support/TemporarySqliteDatabase.h
  )
  target_link_libraries(hcb_sqlite_query_timing_tracker_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sqlite_query_timing_tracker_tests PRIVATE src)
  hcb_enable_warnings(hcb_sqlite_query_timing_tracker_tests)
  hcb_enable_sanitizers(hcb_sqlite_query_timing_tracker_tests)
  add_test(NAME hcb_sqlite_query_timing_tracker_tests COMMAND hcb_sqlite_query_timing_tracker_tests)

  qt_add_executable(hcb_temporary_sqlite_database_tests
    tests/TemporarySqliteDatabaseTest.cpp
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/FilePath.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
  )
  target_link_libraries(hcb_temporary_sqlite_database_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_temporary_sqlite_database_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_temporary_sqlite_database_tests)
  hcb_enable_sanitizers(hcb_temporary_sqlite_database_tests)
  add_test(NAME hcb_temporary_sqlite_database_tests COMMAND hcb_temporary_sqlite_database_tests)

  qt_add_executable(hcb_local_schema_tests
    tests/LocalSchemaTest.cpp
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/FilePath.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
  )
  target_link_libraries(hcb_local_schema_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_local_schema_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_local_schema_tests)
  hcb_enable_sanitizers(hcb_local_schema_tests)
  add_test(NAME hcb_local_schema_tests COMMAND hcb_local_schema_tests)

  qt_add_executable(hcb_core_tests
    tests/AppPathsTest.cpp
    src/app/AppPaths.cpp
    src/app/AppPaths.h
    src/app/LinuxPathsAdapter.cpp
    src/app/LinuxPathsAdapter.h
    src/app/MacOSPathsAdapter.cpp
    src/app/MacOSPathsAdapter.h
    src/app/WindowsPathsAdapter.cpp
    src/app/WindowsPathsAdapter.h
    src/core/AppError.h
    src/core/FilePath.h
  )
  target_link_libraries(hcb_core_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_core_tests PRIVATE src)
  hcb_enable_warnings(hcb_core_tests)
  hcb_enable_sanitizers(hcb_core_tests)
  add_test(NAME hcb_core_tests COMMAND hcb_core_tests)

  qt_add_executable(hcb_file_path_tests
    tests/FilePathTest.cpp
    src/core/FilePath.h
  )
  target_link_libraries(hcb_file_path_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_file_path_tests PRIVATE src)
  hcb_enable_warnings(hcb_file_path_tests)
  hcb_enable_sanitizers(hcb_file_path_tests)
  add_test(NAME hcb_file_path_tests COMMAND hcb_file_path_tests)

  qt_add_executable(hcb_test_bootstrap_tests
    tests/TestApplicationBootstrapTest.cpp
    tests/support/TestApplicationBootstrap.h
    src/app/AppPaths.cpp
    src/app/AppPaths.h
    src/app/AppServices.cpp
    src/app/AppServices.h
    src/app/LinuxPathsAdapter.cpp
    src/app/LinuxPathsAdapter.h
    src/app/MacOSPathsAdapter.cpp
    src/app/MacOSPathsAdapter.h
    src/app/WindowsPathsAdapter.cpp
    src/app/WindowsPathsAdapter.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/SettingsRegistry.h
  )
  target_link_libraries(hcb_test_bootstrap_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_test_bootstrap_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_test_bootstrap_tests)
  hcb_enable_sanitizers(hcb_test_bootstrap_tests)
  add_test(NAME hcb_test_bootstrap_tests COMMAND hcb_test_bootstrap_tests)

  qt_add_executable(hcb_cancellation_tests
    tests/CancellationTest.cpp
    src/core/Cancellation.h
  )
  target_link_libraries(hcb_cancellation_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_cancellation_tests PRIVATE src)
  hcb_enable_warnings(hcb_cancellation_tests)
  hcb_enable_sanitizers(hcb_cancellation_tests)
  add_test(NAME hcb_cancellation_tests COMMAND hcb_cancellation_tests)

  qt_add_executable(hcb_clock_tests
    tests/ClockTest.cpp
    src/core/Clock.h
  )
  target_link_libraries(hcb_clock_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_clock_tests PRIVATE src)
  hcb_enable_warnings(hcb_clock_tests)
  hcb_enable_sanitizers(hcb_clock_tests)
  add_test(NAME hcb_clock_tests COMMAND hcb_clock_tests)

  qt_add_executable(hcb_application_lifecycle_tests
    tests/ApplicationLifecycleTest.cpp
    src/app/ApplicationLifecycle.h
  )
  target_link_libraries(hcb_application_lifecycle_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_application_lifecycle_tests PRIVATE src)
  hcb_enable_warnings(hcb_application_lifecycle_tests)
  hcb_enable_sanitizers(hcb_application_lifecycle_tests)
  add_test(NAME hcb_application_lifecycle_tests COMMAND hcb_application_lifecycle_tests)

  qt_add_executable(hcb_feature_capability_registry_tests
    tests/FeatureCapabilityRegistryTest.cpp
    src/core/FeatureCapabilityRegistry.h
  )
  target_link_libraries(hcb_feature_capability_registry_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_feature_capability_registry_tests PRIVATE src)
  hcb_enable_warnings(hcb_feature_capability_registry_tests)
  hcb_enable_sanitizers(hcb_feature_capability_registry_tests)
  add_test(NAME hcb_feature_capability_registry_tests COMMAND hcb_feature_capability_registry_tests)

  qt_add_executable(hcb_native_capability_report_tests
    tests/NativeCapabilityReportTest.cpp
    src/core/NativeCapabilityReport.cpp
    src/core/NativeCapabilityReport.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_native_capability_report_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_native_capability_report_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_capability_report_tests)
  hcb_enable_sanitizers(hcb_native_capability_report_tests)
  add_test(NAME hcb_native_capability_report_tests COMMAND hcb_native_capability_report_tests)

  qt_add_executable(hcb_command_registry_model_tests
    tests/CommandRegistryModelTest.cpp
    src/core/CommandRegistryModel.cpp
    src/core/CommandRegistryModel.h
  )
  target_link_libraries(hcb_command_registry_model_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_command_registry_model_tests PRIVATE src)
  hcb_enable_warnings(hcb_command_registry_model_tests)
  hcb_enable_sanitizers(hcb_command_registry_model_tests)
  add_test(NAME hcb_command_registry_model_tests COMMAND hcb_command_registry_model_tests)

  qt_add_executable(hcb_quick_capture_parser_tests
    tests/QuickCaptureParserTest.cpp
    src/core/QuickCaptureParser.cpp
    src/core/QuickCaptureParser.h
  )
  target_link_libraries(hcb_quick_capture_parser_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_quick_capture_parser_tests PRIVATE src)
  hcb_enable_warnings(hcb_quick_capture_parser_tests)
  hcb_enable_sanitizers(hcb_quick_capture_parser_tests)
  add_test(NAME hcb_quick_capture_parser_tests COMMAND hcb_quick_capture_parser_tests)

  qt_add_executable(hcb_import_service_tests
    tests/ImportServiceTest.cpp
    src/core/ImportService.cpp
    src/core/ImportService.h
  )
  target_link_libraries(hcb_import_service_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_import_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_import_service_tests)
  hcb_enable_sanitizers(hcb_import_service_tests)
  add_test(NAME hcb_import_service_tests COMMAND hcb_import_service_tests)

  qt_add_executable(hcb_import_mutation_service_tests
    tests/ImportMutationServiceTest.cpp
    src/core/AppError.h
    src/core/CalendarMutationService.cpp
    src/core/CalendarMutationService.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/ImportMutationService.cpp
    src/core/ImportMutationService.h
    src/core/RecurrenceExpansionWorker.cpp
    src/core/RecurrenceExpansionWorker.h
    src/core/TaskMutationService.cpp
    src/core/TaskMutationService.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_import_mutation_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_import_mutation_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_import_mutation_service_tests)
  hcb_enable_sanitizers(hcb_import_mutation_service_tests)
  add_test(NAME hcb_import_mutation_service_tests COMMAND hcb_import_mutation_service_tests)

  qt_add_executable(hcb_settings_registry_tests
    tests/SettingsRegistryTest.cpp
    src/core/SettingsRegistry.h
  )
  target_link_libraries(hcb_settings_registry_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_settings_registry_tests PRIVATE src)
  hcb_enable_warnings(hcb_settings_registry_tests)
  hcb_enable_sanitizers(hcb_settings_registry_tests)
  add_test(NAME hcb_settings_registry_tests COMMAND hcb_settings_registry_tests)

  qt_add_executable(hcb_account_status_service_tests
    tests/AccountStatusServiceTest.cpp
    src/core/AccountStatusService.cpp
    src/core/AccountStatusService.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_account_status_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_account_status_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_account_status_service_tests)
  hcb_enable_sanitizers(hcb_account_status_service_tests)
  add_test(NAME hcb_account_status_service_tests COMMAND hcb_account_status_service_tests)

  qt_add_executable(hcb_calendar_read_service_tests
    tests/CalendarReadServiceTest.cpp
    src/core/AppError.h
    src/core/CalendarReadService.cpp
    src/core/CalendarReadService.h
    src/core/FilePath.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_calendar_read_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_calendar_read_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_calendar_read_service_tests)
  hcb_enable_sanitizers(hcb_calendar_read_service_tests)
  add_test(NAME hcb_calendar_read_service_tests COMMAND hcb_calendar_read_service_tests)

  qt_add_executable(hcb_calendar_source_model_tests
    tests/CalendarSourceModelTest.cpp
    src/core/AppError.h
    src/core/CalendarReadService.h
    src/core/CalendarSourceModel.cpp
    src/core/CalendarSourceModel.h
    src/core/FilePath.h
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_calendar_source_model_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_calendar_source_model_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_calendar_source_model_tests)
  hcb_enable_sanitizers(hcb_calendar_source_model_tests)
  add_test(NAME hcb_calendar_source_model_tests COMMAND hcb_calendar_source_model_tests)

  qt_add_executable(hcb_agenda_model_tests
    tests/AgendaModelTest.cpp
    src/core/AgendaModel.cpp
    src/core/AgendaModel.h
    src/core/AppError.h
    src/core/CalendarReadService.h
    src/core/FilePath.h
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_agenda_model_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_agenda_model_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_agenda_model_tests)
  hcb_enable_sanitizers(hcb_agenda_model_tests)
  add_test(NAME hcb_agenda_model_tests COMMAND hcb_agenda_model_tests)

  qt_add_executable(hcb_month_grid_model_tests
    tests/MonthGridModelTest.cpp
    src/core/CalendarLayoutEngine.cpp
    src/core/CalendarLayoutEngine.h
    src/core/MonthGridModel.cpp
    src/core/MonthGridModel.h
    src/core/AppError.h
    src/core/CalendarReadService.h
    src/core/FilePath.h
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_month_grid_model_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_month_grid_model_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_month_grid_model_tests)
  hcb_enable_sanitizers(hcb_month_grid_model_tests)
  add_test(NAME hcb_month_grid_model_tests COMMAND hcb_month_grid_model_tests)

  qt_add_executable(hcb_scheduled_task_date_index_tests
    tests/ScheduledTaskDateIndexTest.cpp
    src/core/ScheduledTaskDateIndex.cpp
    src/core/ScheduledTaskDateIndex.h
  )
  target_link_libraries(hcb_scheduled_task_date_index_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_scheduled_task_date_index_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_scheduled_task_date_index_tests)
  hcb_enable_sanitizers(hcb_scheduled_task_date_index_tests)
  add_test(NAME hcb_scheduled_task_date_index_tests COMMAND hcb_scheduled_task_date_index_tests)

  qt_add_executable(hcb_timeline_model_tests
    tests/TimelineModelTest.cpp
    src/core/AppError.h
    src/core/CalendarLayoutEngine.cpp
    src/core/CalendarLayoutEngine.h
    src/core/CalendarReadService.h
    src/core/FilePath.h
    src/core/TimelineModel.cpp
    src/core/TimelineModel.h
    src/core/TimelineViewportModel.cpp
    src/core/TimelineViewportModel.h
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_timeline_model_tests PRIVATE Qt6::Core Qt6::Qml Qt6::Test)
  target_include_directories(hcb_timeline_model_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_timeline_model_tests)
  hcb_enable_sanitizers(hcb_timeline_model_tests)
  add_test(NAME hcb_timeline_model_tests COMMAND hcb_timeline_model_tests)

  qt_add_executable(hcb_calendar_mutation_service_tests
    tests/CalendarMutationServiceTest.cpp
    src/core/AppError.h
    src/core/CalendarEventBulkMutationService.cpp
    src/core/CalendarEventBulkMutationService.h
    src/core/CalendarMutationService.cpp
    src/core/CalendarMutationService.h
    src/core/RecurrenceExpansionWorker.cpp
    src/core/RecurrenceExpansionWorker.h
    src/core/Clock.h
    src/core/FilePath.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_calendar_mutation_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_calendar_mutation_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_calendar_mutation_service_tests)
  hcb_enable_sanitizers(hcb_calendar_mutation_service_tests)
  add_test(NAME hcb_calendar_mutation_service_tests COMMAND hcb_calendar_mutation_service_tests)

  qt_add_executable(hcb_reminder_service_tests
    tests/ReminderServiceTest.cpp
    tests/support/TemporarySqliteDatabase.h
    src/app/NativeReminderNotifier.cpp
    src/app/NativeReminderNotifier.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/ReminderService.cpp
    src/core/ReminderService.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
  )
  target_link_libraries(hcb_reminder_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
    target_link_libraries(hcb_reminder_service_tests PRIVATE Qt6::DBus)
  endif()
  target_include_directories(hcb_reminder_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_reminder_service_tests)
  hcb_enable_sanitizers(hcb_reminder_service_tests)
  add_test(NAME hcb_reminder_service_tests COMMAND hcb_reminder_service_tests)

  qt_add_executable(hcb_notes_model_tests
    tests/NotesModelTest.cpp
    src/core/NotesModel.cpp
    src/core/NotesModel.h
    src/core/TaskModel.cpp
    src/core/TaskModel.h
    src/core/TaskMutationService.h
  )
  target_link_libraries(hcb_notes_model_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_notes_model_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_notes_model_tests)
  hcb_enable_sanitizers(hcb_notes_model_tests)
  add_test(NAME hcb_notes_model_tests COMMAND hcb_notes_model_tests)

  qt_add_executable(hcb_task_recurrence_marker_tests
    tests/TaskRecurrenceMarkerTest.cpp
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
  )
  target_link_libraries(hcb_task_recurrence_marker_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_task_recurrence_marker_tests PRIVATE src)
  hcb_enable_warnings(hcb_task_recurrence_marker_tests)
  hcb_enable_sanitizers(hcb_task_recurrence_marker_tests)
  add_test(NAME hcb_task_recurrence_marker_tests COMMAND hcb_task_recurrence_marker_tests)

  qt_add_executable(hcb_optimistic_mutation_coordinator_tests
    tests/OptimisticMutationCoordinatorTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/OptimisticMutationCoordinator.cpp
    src/core/OptimisticMutationCoordinator.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_optimistic_mutation_coordinator_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_optimistic_mutation_coordinator_tests PRIVATE src)
  hcb_enable_warnings(hcb_optimistic_mutation_coordinator_tests)
  hcb_enable_sanitizers(hcb_optimistic_mutation_coordinator_tests)
  add_test(NAME hcb_optimistic_mutation_coordinator_tests COMMAND hcb_optimistic_mutation_coordinator_tests)

  qt_add_executable(hcb_undo_recovery_policy_tests
    tests/UndoRecoveryPolicyTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/UndoRecoveryPolicy.cpp
    src/core/UndoRecoveryPolicy.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_undo_recovery_policy_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_undo_recovery_policy_tests PRIVATE src)
  hcb_enable_warnings(hcb_undo_recovery_policy_tests)
  hcb_enable_sanitizers(hcb_undo_recovery_policy_tests)
  add_test(NAME hcb_undo_recovery_policy_tests COMMAND hcb_undo_recovery_policy_tests)

  qt_add_executable(hcb_domain_service_contract_tests
    tests/DomainServiceContractTest.cpp
    src/core/AppError.h
    src/core/CalendarMutationService.cpp
    src/core/CalendarMutationService.h
    src/core/RecurrenceExpansionWorker.cpp
    src/core/RecurrenceExpansionWorker.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/OptimisticMutationCoordinator.cpp
    src/core/OptimisticMutationCoordinator.h
    src/core/TaskMutationService.cpp
    src/core/TaskMutationService.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/core/UndoRecoveryPolicy.cpp
    src/core/UndoRecoveryPolicy.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_domain_service_contract_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_domain_service_contract_tests PRIVATE src)
  hcb_enable_warnings(hcb_domain_service_contract_tests)
  hcb_enable_sanitizers(hcb_domain_service_contract_tests)
  add_test(NAME hcb_domain_service_contract_tests COMMAND hcb_domain_service_contract_tests)

  qt_add_executable(hcb_task_list_read_service_tests
    tests/TaskListReadServiceTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/core/TaskListReadService.cpp
    src/core/TaskListReadService.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_task_list_read_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_task_list_read_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_task_list_read_service_tests)
  hcb_enable_sanitizers(hcb_task_list_read_service_tests)
  add_test(NAME hcb_task_list_read_service_tests COMMAND hcb_task_list_read_service_tests)

  qt_add_executable(hcb_task_read_service_tests
    tests/TaskReadServiceTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/core/TaskModel.cpp
    src/core/TaskModel.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/core/TaskReadService.cpp
    src/core/TaskReadService.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_task_read_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_task_read_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_task_read_service_tests)
  hcb_enable_sanitizers(hcb_task_read_service_tests)
  add_test(NAME hcb_task_read_service_tests COMMAND hcb_task_read_service_tests)

  qt_add_executable(hcb_google_mirror_store_tests
    tests/GoogleMirrorStoreTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/GoogleCalendarEventPullClient.h
    src/core/GoogleCalendarListPullClient.h
    src/core/GoogleMirrorStore.cpp
    src/core/GoogleMirrorStore.h
    src/core/GoogleTaskListPullClient.h
    src/core/GoogleTaskPullClient.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteStatementCache.cpp
    src/data/SqliteStatementCache.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_google_mirror_store_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_google_mirror_store_tests PRIVATE src)
  hcb_enable_warnings(hcb_google_mirror_store_tests)
  hcb_enable_sanitizers(hcb_google_mirror_store_tests)
  add_test(NAME hcb_google_mirror_store_tests COMMAND hcb_google_mirror_store_tests)

  qt_add_executable(hcb_task_mutation_service_tests
    tests/TaskMutationServiceTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/TaskMutationService.cpp
    src/core/TaskMutationService.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_task_mutation_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_task_mutation_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_task_mutation_service_tests)
  hcb_enable_sanitizers(hcb_task_mutation_service_tests)
  add_test(NAME hcb_task_mutation_service_tests COMMAND hcb_task_mutation_service_tests)

  qt_add_executable(hcb_task_bulk_mutation_service_tests
    tests/TaskBulkMutationServiceTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/TaskBulkMutationService.cpp
    src/core/TaskBulkMutationService.h
    src/core/TaskMutationService.cpp
    src/core/TaskMutationService.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_task_bulk_mutation_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_task_bulk_mutation_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_task_bulk_mutation_service_tests)
  hcb_enable_sanitizers(hcb_task_bulk_mutation_service_tests)
  add_test(NAME hcb_task_bulk_mutation_service_tests COMMAND hcb_task_bulk_mutation_service_tests)

  qt_add_executable(hcb_task_list_mutation_service_tests
    tests/TaskListMutationServiceTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/TaskListMutationService.cpp
    src/core/TaskListMutationService.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_task_list_mutation_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_task_list_mutation_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_task_list_mutation_service_tests)
  hcb_enable_sanitizers(hcb_task_list_mutation_service_tests)
  add_test(NAME hcb_task_list_mutation_service_tests COMMAND hcb_task_list_mutation_service_tests)

  qt_add_executable(hcb_settings_service_tests
    tests/SettingsServiceTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/SettingsService.cpp
    src/core/SettingsService.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_settings_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_settings_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_settings_service_tests)
  hcb_enable_sanitizers(hcb_settings_service_tests)
  add_test(NAME hcb_settings_service_tests COMMAND hcb_settings_service_tests)

  qt_add_executable(hcb_saved_search_store_tests
    tests/SavedSearchStoreTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/SavedSearchStore.cpp
    src/core/SavedSearchStore.h
    src/core/SettingsService.cpp
    src/core/SettingsService.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_saved_search_store_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_saved_search_store_tests PRIVATE src)
  hcb_enable_warnings(hcb_saved_search_store_tests)
  hcb_enable_sanitizers(hcb_saved_search_store_tests)
  add_test(NAME hcb_saved_search_store_tests COMMAND hcb_saved_search_store_tests)

  qt_add_executable(hcb_sync_checkpoint_store_tests
    tests/SyncCheckpointStoreTest.cpp
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/SyncCheckpointStore.cpp
    src/core/SyncCheckpointStore.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_sync_checkpoint_store_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sync_checkpoint_store_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_sync_checkpoint_store_tests)
  hcb_enable_sanitizers(hcb_sync_checkpoint_store_tests)
  add_test(NAME hcb_sync_checkpoint_store_tests COMMAND hcb_sync_checkpoint_store_tests)

  qt_add_executable(hcb_google_mirror_sync_service_tests
    tests/GoogleMirrorSyncServiceTest.cpp
    tests/support/MockNetworkAccessManager.h
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/CalendarReadService.cpp
    src/core/CalendarReadService.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleCalendarEventPullClient.cpp
    src/core/GoogleCalendarEventPullClient.h
    src/core/GoogleCalendarListPullClient.cpp
    src/core/GoogleCalendarListPullClient.h
    src/core/GoogleCalendarMirrorSyncService.cpp
    src/core/GoogleCalendarMirrorSyncService.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/GoogleMirrorStore.cpp
    src/core/GoogleMirrorStore.h
    src/core/GoogleSyncRecoveryService.cpp
    src/core/GoogleSyncRecoveryService.h
    src/core/GoogleTaskListPullClient.cpp
    src/core/GoogleTaskListPullClient.h
    src/core/GoogleTaskMirrorSyncService.cpp
    src/core/GoogleTaskMirrorSyncService.h
    src/core/GoogleTaskPullClient.cpp
    src/core/GoogleTaskPullClient.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/SyncCheckpointStore.cpp
    src/core/SyncCheckpointStore.h
    src/core/SyncBackoffPolicy.cpp
    src/core/SyncBackoffPolicy.h
    src/core/TaskMutationService.cpp
    src/core/TaskMutationService.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteStatementCache.cpp
    src/data/SqliteStatementCache.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_google_mirror_sync_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_mirror_sync_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_mirror_sync_service_tests)
  hcb_enable_sanitizers(hcb_google_mirror_sync_service_tests)
  add_test(NAME hcb_google_mirror_sync_service_tests COMMAND hcb_google_mirror_sync_service_tests)

  qt_add_executable(hcb_sync_conflict_store_tests
    tests/SyncConflictStoreTest.cpp
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/SyncConflictStore.cpp
    src/core/SyncConflictStore.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_sync_conflict_store_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_sync_conflict_store_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_sync_conflict_store_tests)
  hcb_enable_sanitizers(hcb_sync_conflict_store_tests)
  add_test(NAME hcb_sync_conflict_store_tests COMMAND hcb_sync_conflict_store_tests)

  qt_add_executable(hcb_sync_three_way_merge_tests
    tests/SyncThreeWayMergeTest.cpp
    src/core/SyncConflictStore.h
    src/core/SyncThreeWayMerge.cpp
    src/core/SyncThreeWayMerge.h
  )
  target_link_libraries(hcb_sync_three_way_merge_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_sync_three_way_merge_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_sync_three_way_merge_tests)
  hcb_enable_sanitizers(hcb_sync_three_way_merge_tests)
  add_test(NAME hcb_sync_three_way_merge_tests COMMAND hcb_sync_three_way_merge_tests)

  qt_add_executable(hcb_google_sync_conflict_resolver_tests
    tests/GoogleSyncConflictResolverTest.cpp
    tests/support/MockNetworkAccessManager.h
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/GoogleSyncConflictResolver.cpp
    src/core/GoogleSyncConflictResolver.h
    src/core/OptimisticMutationCoordinator.cpp
    src/core/OptimisticMutationCoordinator.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/SyncConflictPolicy.h
    src/core/SyncConflictStore.cpp
    src/core/SyncConflictStore.h
    src/core/SyncThreeWayMerge.cpp
    src/core/SyncThreeWayMerge.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_google_sync_conflict_resolver_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_sync_conflict_resolver_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_sync_conflict_resolver_tests)
  hcb_enable_sanitizers(hcb_google_sync_conflict_resolver_tests)
  add_test(NAME hcb_google_sync_conflict_resolver_tests COMMAND hcb_google_sync_conflict_resolver_tests)

  qt_add_executable(hcb_sync_scheduler_tests
    tests/SyncSchedulerTest.cpp
    src/core/AppError.h
    src/core/SyncScheduler.cpp
    src/core/SyncScheduler.h
  )
  target_link_libraries(hcb_sync_scheduler_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_sync_scheduler_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_sync_scheduler_tests)
  hcb_enable_sanitizers(hcb_sync_scheduler_tests)
  add_test(NAME hcb_sync_scheduler_tests COMMAND hcb_sync_scheduler_tests)

  qt_add_executable(hcb_unified_local_search_ranker_tests
    tests/UnifiedLocalSearchRankerTest.cpp
    src/core/UnifiedLocalSearchRanker.cpp
    src/core/UnifiedLocalSearchRanker.h
  )
  target_link_libraries(hcb_unified_local_search_ranker_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_unified_local_search_ranker_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_unified_local_search_ranker_tests)
  hcb_enable_sanitizers(hcb_unified_local_search_ranker_tests)
  add_test(NAME hcb_unified_local_search_ranker_tests COMMAND hcb_unified_local_search_ranker_tests)

  qt_add_executable(hcb_recurrence_expansion_worker_tests
    tests/RecurrenceExpansionWorkerTest.cpp
    src/core/AppError.h
    src/core/Cancellation.h
    src/core/RecurrenceExpansionWorker.cpp
    src/core/RecurrenceExpansionWorker.h
  )
  target_link_libraries(hcb_recurrence_expansion_worker_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_recurrence_expansion_worker_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_recurrence_expansion_worker_tests)
  hcb_enable_sanitizers(hcb_recurrence_expansion_worker_tests)
  add_test(NAME hcb_recurrence_expansion_worker_tests COMMAND hcb_recurrence_expansion_worker_tests)

  qt_add_executable(hcb_recurrence_result_cache_tests
    tests/RecurrenceResultCacheTest.cpp
    src/core/AppError.h
    src/core/RecurrenceExpansionWorker.h
    src/core/RecurrenceResultCache.cpp
    src/core/RecurrenceResultCache.h
  )
  target_link_libraries(hcb_recurrence_result_cache_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_recurrence_result_cache_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_recurrence_result_cache_tests)
  hcb_enable_sanitizers(hcb_recurrence_result_cache_tests)
  add_test(NAME hcb_recurrence_result_cache_tests COMMAND hcb_recurrence_result_cache_tests)

  qt_add_executable(hcb_calendar_layout_engine_tests
    tests/CalendarLayoutEngineTest.cpp
    src/core/CalendarLayoutEngine.cpp
    src/core/CalendarLayoutEngine.h
  )
  target_link_libraries(hcb_calendar_layout_engine_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_calendar_layout_engine_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_calendar_layout_engine_tests)
  hcb_enable_sanitizers(hcb_calendar_layout_engine_tests)
  add_test(NAME hcb_calendar_layout_engine_tests COMMAND hcb_calendar_layout_engine_tests)

  qt_add_executable(hcb_native_planner_benchmark_tests
    tests/NativePlannerBenchmarkTest.cpp
    src/core/CalendarLayoutEngine.cpp
    src/core/CalendarLayoutEngine.h
    src/core/NativePerformanceFixture.cpp
    src/core/NativePerformanceFixture.h
    src/core/NativePlannerBenchmark.cpp
    src/core/NativePlannerBenchmark.h
  )
  target_link_libraries(hcb_native_planner_benchmark_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_native_planner_benchmark_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_native_planner_benchmark_tests)
  hcb_enable_sanitizers(hcb_native_planner_benchmark_tests)
  add_test(NAME hcb_native_planner_benchmark_tests COMMAND hcb_native_planner_benchmark_tests)

  qt_add_executable(hcb_native_quick_capture_benchmark_tests
    tests/NativeQuickCaptureBenchmarkTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/NativeQuickCaptureBenchmark.cpp
    src/core/NativeQuickCaptureBenchmark.h
    src/core/TaskMutationService.cpp
    src/core/TaskMutationService.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_native_quick_capture_benchmark_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_native_quick_capture_benchmark_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_quick_capture_benchmark_tests)
  hcb_enable_sanitizers(hcb_native_quick_capture_benchmark_tests)
  add_test(NAME hcb_native_quick_capture_benchmark_tests COMMAND hcb_native_quick_capture_benchmark_tests)

  qt_add_executable(hcb_native_local_search_benchmark_tests
    tests/NativeLocalSearchBenchmarkTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/core/LocalSearchService.cpp
    src/core/LocalSearchService.h
    src/core/LocalSearchQuery.cpp
    src/core/LocalSearchQuery.h
    src/core/NativeLocalSearchBenchmark.cpp
    src/core/NativeLocalSearchBenchmark.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/core/UnifiedLocalSearchRanker.cpp
    src/core/UnifiedLocalSearchRanker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteReadConnectionPool.cpp
    src/data/SqliteReadConnectionPool.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
  )
  target_link_libraries(hcb_native_local_search_benchmark_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_native_local_search_benchmark_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_local_search_benchmark_tests)
  hcb_enable_sanitizers(hcb_native_local_search_benchmark_tests)
  add_test(NAME hcb_native_local_search_benchmark_tests COMMAND hcb_native_local_search_benchmark_tests)

  qt_add_executable(hcb_native_task_scroll_benchmark_tests
    tests/NativeTaskScrollBenchmarkTest.cpp
    src/core/NativePerformanceFixture.cpp
    src/core/NativePerformanceFixture.h
    src/core/NativeTaskScrollBenchmark.cpp
    src/core/NativeTaskScrollBenchmark.h
    src/core/TaskModel.cpp
    src/core/TaskModel.h
    src/core/TaskMutationService.h
  )
  target_link_libraries(hcb_native_task_scroll_benchmark_tests PRIVATE Qt6::Core Qt6::Gui Qt6::Qml Qt6::Quick Qt6::Test)
  target_include_directories(hcb_native_task_scroll_benchmark_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_task_scroll_benchmark_tests)
  hcb_enable_sanitizers(hcb_native_task_scroll_benchmark_tests)
  add_test(NAME hcb_native_task_scroll_benchmark_tests
    COMMAND hcb_native_task_scroll_benchmark_tests -platform offscreen
  )

  qt_add_executable(hcb_native_calendar_navigation_benchmark_tests
    tests/NativeCalendarNavigationBenchmarkTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/core/MonthGridModel.cpp
    src/core/MonthGridModel.h
    src/core/NativeCalendarNavigationBenchmark.cpp
    src/core/NativeCalendarNavigationBenchmark.h
    src/core/NativePerformanceFixture.cpp
    src/core/NativePerformanceFixture.h
    src/core/CalendarReadService.h
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_native_calendar_navigation_benchmark_tests PRIVATE Qt6::Core Qt6::Gui Qt6::Qml Qt6::Quick Qt6::Test)
  target_include_directories(hcb_native_calendar_navigation_benchmark_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_calendar_navigation_benchmark_tests)
  hcb_enable_sanitizers(hcb_native_calendar_navigation_benchmark_tests)
  add_test(NAME hcb_native_calendar_navigation_benchmark_tests
    COMMAND hcb_native_calendar_navigation_benchmark_tests -platform offscreen
  )

  qt_add_executable(hcb_task_list_model_tests
    tests/TaskListModelTest.cpp
    src/core/AppError.h
    src/core/FilePath.h
    src/core/TaskListModel.cpp
    src/core/TaskListModel.h
    src/core/TaskListReadService.h
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_task_list_model_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_task_list_model_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_task_list_model_tests)
  hcb_enable_sanitizers(hcb_task_list_model_tests)
  add_test(NAME hcb_task_list_model_tests COMMAND hcb_task_list_model_tests)

  qt_add_executable(hcb_task_model_tests
    tests/TaskModelTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/TaskModel.cpp
    src/core/TaskModel.h
    src/core/TaskMutationService.h
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_task_model_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_task_model_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_task_model_tests)
  hcb_enable_sanitizers(hcb_task_model_tests)
  add_test(NAME hcb_task_model_tests COMMAND hcb_task_model_tests)

  qt_add_executable(hcb_task_tree_view_tests
    tests/TaskTreeViewTest.cpp
    src/core/TaskModel.cpp
    src/core/TaskModel.h
    src/core/TaskMutationService.h
  )
  target_compile_definitions(hcb_task_tree_view_tests PRIVATE
    HCB_NATIVE_QML_DIR="${CMAKE_CURRENT_SOURCE_DIR}/qml"
  )
  target_link_libraries(hcb_task_tree_view_tests PRIVATE Qt6::Core Qt6::Gui Qt6::Qml Qt6::Quick Qt6::Test)
  target_include_directories(hcb_task_tree_view_tests PRIVATE src)
  hcb_enable_warnings(hcb_task_tree_view_tests)
  hcb_enable_sanitizers(hcb_task_tree_view_tests)
  add_test(NAME hcb_task_tree_view_tests COMMAND hcb_task_tree_view_tests -platform offscreen)

  qt_add_executable(hcb_local_search_service_tests
    tests/LocalSearchServiceTest.cpp
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/FilePath.h
    src/core/LocalSearchService.cpp
    src/core/LocalSearchService.h
    src/core/LocalSearchQuery.cpp
    src/core/LocalSearchQuery.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/core/UnifiedLocalSearchRanker.cpp
    src/core/UnifiedLocalSearchRanker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteReadConnectionPool.cpp
    src/data/SqliteReadConnectionPool.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
  )
  target_link_libraries(hcb_local_search_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_local_search_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_local_search_service_tests)
  hcb_enable_sanitizers(hcb_local_search_service_tests)
  add_test(NAME hcb_local_search_service_tests COMMAND hcb_local_search_service_tests)

  qt_add_executable(hcb_search_results_model_tests
    tests/SearchResultsModelTest.cpp
    src/core/SearchResultsModel.cpp
    src/core/SearchResultsModel.h
    src/core/UnifiedLocalSearchRanker.h
  )
  target_link_libraries(hcb_search_results_model_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_search_results_model_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_search_results_model_tests)
  hcb_enable_sanitizers(hcb_search_results_model_tests)
  add_test(NAME hcb_search_results_model_tests COMMAND hcb_search_results_model_tests)

  qt_add_executable(hcb_model_diff_policy_tests
    tests/ModelDiffPolicyTest.cpp
    src/core/ModelDiffPolicy.h
  )
  target_link_libraries(hcb_model_diff_policy_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_model_diff_policy_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_model_diff_policy_tests)
  hcb_enable_sanitizers(hcb_model_diff_policy_tests)
  add_test(NAME hcb_model_diff_policy_tests COMMAND hcb_model_diff_policy_tests)

  qt_add_executable(hcb_model_virtualization_tests
    tests/ModelVirtualizationTest.cpp
    src/core/ModelDiffPolicy.h
    src/core/SearchResultsModel.cpp
    src/core/SearchResultsModel.h
    src/core/UnifiedLocalSearchRanker.h
  )
  target_link_libraries(hcb_model_virtualization_tests PRIVATE Qt6::Core Qt6::Qml Qt6::Quick Qt6::Test)
  target_include_directories(hcb_model_virtualization_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_model_virtualization_tests)
  hcb_enable_sanitizers(hcb_model_virtualization_tests)
  add_test(NAME hcb_model_virtualization_tests COMMAND hcb_model_virtualization_tests -platform offscreen)

  qt_add_executable(hcb_google_sync_recovery_service_tests
    tests/GoogleSyncRecoveryServiceTest.cpp
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleSyncRecoveryService.cpp
    src/core/GoogleSyncRecoveryService.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/SyncCheckpointStore.cpp
    src/core/SyncCheckpointStore.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_google_sync_recovery_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_google_sync_recovery_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_sync_recovery_service_tests)
  hcb_enable_sanitizers(hcb_google_sync_recovery_service_tests)
  add_test(NAME hcb_google_sync_recovery_service_tests COMMAND hcb_google_sync_recovery_service_tests)

  qt_add_executable(hcb_google_api_error_tests
    tests/GoogleApiErrorTest.cpp
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_google_api_error_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_google_api_error_tests PRIVATE src)
  hcb_enable_warnings(hcb_google_api_error_tests)
  hcb_enable_sanitizers(hcb_google_api_error_tests)
  add_test(NAME hcb_google_api_error_tests COMMAND hcb_google_api_error_tests)

  qt_add_executable(hcb_google_http_client_tests
    tests/GoogleHttpClientTest.cpp
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_google_http_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_http_client_tests PRIVATE src)
  hcb_enable_warnings(hcb_google_http_client_tests)
  hcb_enable_sanitizers(hcb_google_http_client_tests)
  add_test(NAME hcb_google_http_client_tests COMMAND hcb_google_http_client_tests)

  qt_add_executable(hcb_google_calendar_list_pull_client_tests
    tests/GoogleCalendarListPullClientTest.cpp
    tests/support/MockNetworkAccessManager.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleCalendarListPullClient.cpp
    src/core/GoogleCalendarListPullClient.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_google_calendar_list_pull_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_calendar_list_pull_client_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_calendar_list_pull_client_tests)
  hcb_enable_sanitizers(hcb_google_calendar_list_pull_client_tests)
  add_test(NAME hcb_google_calendar_list_pull_client_tests COMMAND hcb_google_calendar_list_pull_client_tests)

  qt_add_executable(hcb_google_calendar_event_pull_client_tests
    tests/GoogleCalendarEventPullClientTest.cpp
    tests/support/MockNetworkAccessManager.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleCalendarEventPullClient.cpp
    src/core/GoogleCalendarEventPullClient.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_google_calendar_event_pull_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_calendar_event_pull_client_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_calendar_event_pull_client_tests)
  hcb_enable_sanitizers(hcb_google_calendar_event_pull_client_tests)
  add_test(NAME hcb_google_calendar_event_pull_client_tests COMMAND hcb_google_calendar_event_pull_client_tests)

  qt_add_executable(hcb_google_calendar_event_mutation_push_service_tests
    tests/GoogleCalendarEventMutationPushServiceTest.cpp
    tests/support/MockNetworkAccessManager.h
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/CalendarMutationService.cpp
    src/core/CalendarMutationService.h
    src/core/RecurrenceExpansionWorker.cpp
    src/core/RecurrenceExpansionWorker.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleCalendarEventMutationPushService.cpp
    src/core/GoogleCalendarEventMutationPushService.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/MutationTelemetryStore.cpp
    src/core/MutationTelemetryStore.h
    src/core/GoogleSyncConflictResolver.cpp
    src/core/GoogleSyncConflictResolver.h
    src/core/OptimisticMutationCoordinator.cpp
    src/core/OptimisticMutationCoordinator.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/SyncBackoffPolicy.cpp
    src/core/SyncBackoffPolicy.h
    src/core/SyncConflictPolicy.h
    src/core/SyncConflictStore.cpp
    src/core/SyncConflictStore.h
    src/core/SyncThreeWayMerge.cpp
    src/core/SyncThreeWayMerge.h
    src/core/TaskMutationService.cpp
    src/core/TaskMutationService.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_google_calendar_event_mutation_push_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_calendar_event_mutation_push_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_calendar_event_mutation_push_service_tests)
  hcb_enable_sanitizers(hcb_google_calendar_event_mutation_push_service_tests)
  add_test(NAME hcb_google_calendar_event_mutation_push_service_tests COMMAND hcb_google_calendar_event_mutation_push_service_tests)

  qt_add_executable(hcb_google_calendar_management_client_tests
    tests/GoogleCalendarManagementClientTest.cpp
    tests/support/MockNetworkAccessManager.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleCalendarManagementClient.cpp
    src/core/GoogleCalendarManagementClient.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_google_calendar_management_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_calendar_management_client_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_calendar_management_client_tests)
  hcb_enable_sanitizers(hcb_google_calendar_management_client_tests)
  add_test(NAME hcb_google_calendar_management_client_tests COMMAND hcb_google_calendar_management_client_tests)

  qt_add_executable(hcb_google_calendar_availability_client_tests
    tests/GoogleCalendarAvailabilityClientTest.cpp
    tests/support/MockNetworkAccessManager.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleCalendarFreeBusyClient.cpp
    src/core/GoogleCalendarFreeBusyClient.h
    src/core/GoogleDriveFilePickerClient.cpp
    src/core/GoogleDriveFilePickerClient.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_google_calendar_availability_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_calendar_availability_client_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_calendar_availability_client_tests)
  hcb_enable_sanitizers(hcb_google_calendar_availability_client_tests)
  add_test(NAME hcb_google_calendar_availability_client_tests COMMAND hcb_google_calendar_availability_client_tests)

  qt_add_executable(hcb_google_task_list_pull_client_tests
    tests/GoogleTaskListPullClientTest.cpp
    tests/support/MockNetworkAccessManager.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/GoogleTaskListPullClient.cpp
    src/core/GoogleTaskListPullClient.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_google_task_list_pull_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_task_list_pull_client_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_task_list_pull_client_tests)
  hcb_enable_sanitizers(hcb_google_task_list_pull_client_tests)
  add_test(NAME hcb_google_task_list_pull_client_tests COMMAND hcb_google_task_list_pull_client_tests)

  qt_add_executable(hcb_google_task_pull_client_tests
    tests/GoogleTaskPullClientTest.cpp
    tests/support/MockNetworkAccessManager.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/GoogleTaskPullClient.cpp
    src/core/GoogleTaskPullClient.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_google_task_pull_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_task_pull_client_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_task_pull_client_tests)
  hcb_enable_sanitizers(hcb_google_task_pull_client_tests)
  add_test(NAME hcb_google_task_pull_client_tests COMMAND hcb_google_task_pull_client_tests)

  qt_add_executable(hcb_google_task_mutation_push_service_tests
    tests/GoogleTaskMutationPushServiceTest.cpp
    tests/support/MockNetworkAccessManager.h
    tests/support/TemporarySqliteDatabase.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/GoogleSyncConflictResolver.cpp
    src/core/GoogleSyncConflictResolver.h
    src/core/GoogleTaskMutationPushService.cpp
    src/core/GoogleTaskMutationPushService.h
    src/core/MutationTelemetryStore.cpp
    src/core/MutationTelemetryStore.h
    src/core/OptimisticMutationCoordinator.cpp
    src/core/OptimisticMutationCoordinator.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/SyncBackoffPolicy.cpp
    src/core/SyncBackoffPolicy.h
    src/core/SyncConflictPolicy.h
    src/core/SyncConflictStore.cpp
    src/core/SyncConflictStore.h
    src/core/SyncThreeWayMerge.cpp
    src/core/SyncThreeWayMerge.h
    src/core/TaskMutationService.cpp
    src/core/TaskMutationService.h
    src/core/TaskRecurrenceMarker.cpp
    src/core/TaskRecurrenceMarker.h
    src/core/TaskListMutationService.cpp
    src/core/TaskListMutationService.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_google_task_mutation_push_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_google_task_mutation_push_service_tests PRIVATE src tests)
  hcb_enable_warnings(hcb_google_task_mutation_push_service_tests)
  hcb_enable_sanitizers(hcb_google_task_mutation_push_service_tests)
  add_test(NAME hcb_google_task_mutation_push_service_tests COMMAND hcb_google_task_mutation_push_service_tests)

  qt_add_executable(hcb_mocked_oauth_transport_tests
    tests/MockedOAuthTransportTest.cpp
    src/core/AppError.h
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/GoogleHttpClient.cpp
    src/core/GoogleHttpClient.h
    src/core/OAuthTokenExchangeClient.cpp
    src/core/OAuthTokenExchangeClient.h
    src/core/OAuthTokenRefreshClient.cpp
    src/core/OAuthTokenRefreshClient.h
    src/core/OAuthTokenRevocationClient.cpp
    src/core/OAuthTokenRevocationClient.h
    src/core/OAuthTokenRevoker.h
    src/core/PkceAuthorization.cpp
    src/core/PkceAuthorization.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_mocked_oauth_transport_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_mocked_oauth_transport_tests PRIVATE src)
  hcb_enable_warnings(hcb_mocked_oauth_transport_tests)
  hcb_enable_sanitizers(hcb_mocked_oauth_transport_tests)
  add_test(NAME hcb_mocked_oauth_transport_tests COMMAND hcb_mocked_oauth_transport_tests)

  qt_add_executable(hcb_sync_backoff_policy_tests
    tests/SyncBackoffPolicyTest.cpp
    src/core/GoogleApiError.cpp
    src/core/GoogleApiError.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/SyncBackoffPolicy.cpp
    src/core/SyncBackoffPolicy.h
  )
  target_link_libraries(hcb_sync_backoff_policy_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_sync_backoff_policy_tests PRIVATE src)
  hcb_enable_warnings(hcb_sync_backoff_policy_tests)
  hcb_enable_sanitizers(hcb_sync_backoff_policy_tests)
  add_test(NAME hcb_sync_backoff_policy_tests COMMAND hcb_sync_backoff_policy_tests)

  qt_add_executable(hcb_pkce_authorization_tests
    tests/PkceAuthorizationTest.cpp
    src/core/Clock.h
    src/core/PkceAuthorization.cpp
    src/core/PkceAuthorization.h
  )
  target_link_libraries(hcb_pkce_authorization_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_pkce_authorization_tests PRIVATE src)
  hcb_enable_warnings(hcb_pkce_authorization_tests)
  hcb_enable_sanitizers(hcb_pkce_authorization_tests)
  add_test(NAME hcb_pkce_authorization_tests COMMAND hcb_pkce_authorization_tests)

  qt_add_executable(hcb_oauth_client_configuration_store_tests
    tests/OAuthClientConfigurationStoreTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/OAuthClientConfigurationStore.cpp
    src/core/OAuthClientConfigurationStore.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_oauth_client_configuration_store_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_oauth_client_configuration_store_tests PRIVATE src)
  hcb_enable_warnings(hcb_oauth_client_configuration_store_tests)
  hcb_enable_sanitizers(hcb_oauth_client_configuration_store_tests)
  add_test(NAME hcb_oauth_client_configuration_store_tests COMMAND hcb_oauth_client_configuration_store_tests)

  qt_add_executable(hcb_oauth_loopback_callback_listener_tests
    tests/OAuthLoopbackCallbackListenerTest.cpp
    src/core/AppError.h
    src/core/OAuthLoopbackCallbackListener.cpp
    src/core/OAuthLoopbackCallbackListener.h
  )
  target_link_libraries(hcb_oauth_loopback_callback_listener_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_oauth_loopback_callback_listener_tests PRIVATE src)
  hcb_enable_warnings(hcb_oauth_loopback_callback_listener_tests)
  hcb_enable_sanitizers(hcb_oauth_loopback_callback_listener_tests)
  add_test(NAME hcb_oauth_loopback_callback_listener_tests COMMAND hcb_oauth_loopback_callback_listener_tests)

  qt_add_executable(hcb_oauth_browser_authorization_launcher_tests
    tests/OAuthBrowserAuthorizationLauncherTest.cpp
    src/core/AppError.h
    src/core/OAuthBrowserAuthorizationLauncher.cpp
    src/core/OAuthBrowserAuthorizationLauncher.h
  )
  target_link_libraries(hcb_oauth_browser_authorization_launcher_tests PRIVATE Qt6::Core Qt6::Gui Qt6::Test)
  target_include_directories(hcb_oauth_browser_authorization_launcher_tests PRIVATE src)
  hcb_enable_warnings(hcb_oauth_browser_authorization_launcher_tests)
  hcb_enable_sanitizers(hcb_oauth_browser_authorization_launcher_tests)
  add_test(NAME hcb_oauth_browser_authorization_launcher_tests COMMAND hcb_oauth_browser_authorization_launcher_tests)

  qt_add_executable(hcb_oauth_token_exchange_client_tests
    tests/OAuthTokenExchangeClientTest.cpp
    src/core/AppError.h
    src/core/PkceAuthorization.cpp
    src/core/PkceAuthorization.h
    src/core/OAuthTokenExchangeClient.cpp
    src/core/OAuthTokenExchangeClient.h
  )
  target_link_libraries(hcb_oauth_token_exchange_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_oauth_token_exchange_client_tests PRIVATE src)
  hcb_enable_warnings(hcb_oauth_token_exchange_client_tests)
  hcb_enable_sanitizers(hcb_oauth_token_exchange_client_tests)
  add_test(NAME hcb_oauth_token_exchange_client_tests COMMAND hcb_oauth_token_exchange_client_tests)

  qt_add_executable(hcb_oauth_token_refresh_client_tests
    tests/OAuthTokenRefreshClientTest.cpp
    src/core/AppError.h
    src/core/PkceAuthorization.cpp
    src/core/PkceAuthorization.h
    src/core/OAuthTokenExchangeClient.cpp
    src/core/OAuthTokenExchangeClient.h
    src/core/OAuthTokenRefreshClient.cpp
    src/core/OAuthTokenRefreshClient.h
  )
  target_link_libraries(hcb_oauth_token_refresh_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_oauth_token_refresh_client_tests PRIVATE src)
  hcb_enable_warnings(hcb_oauth_token_refresh_client_tests)
  hcb_enable_sanitizers(hcb_oauth_token_refresh_client_tests)
  add_test(NAME hcb_oauth_token_refresh_client_tests COMMAND hcb_oauth_token_refresh_client_tests)

  qt_add_executable(hcb_oauth_disconnect_service_tests
    tests/OAuthDisconnectServiceTest.cpp
    src/core/AccountStatusService.cpp
    src/core/AccountStatusService.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/OAuthCredentialStore.h
    src/core/OAuthDisconnectService.cpp
    src/core/OAuthDisconnectService.h
    src/core/OAuthTokenRevoker.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_oauth_disconnect_service_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_oauth_disconnect_service_tests PRIVATE src)
  hcb_enable_warnings(hcb_oauth_disconnect_service_tests)
  hcb_enable_sanitizers(hcb_oauth_disconnect_service_tests)
  add_test(NAME hcb_oauth_disconnect_service_tests COMMAND hcb_oauth_disconnect_service_tests)

  qt_add_executable(hcb_credential_health_diagnostics_tests
    tests/CredentialHealthDiagnosticsTest.cpp
    src/core/AccountStatusService.cpp
    src/core/AccountStatusService.h
    src/core/AppError.h
    src/core/Clock.h
    src/core/CredentialHealthDiagnostics.cpp
    src/core/CredentialHealthDiagnostics.h
    src/core/FilePath.h
    src/core/OAuthCredentialStore.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_credential_health_diagnostics_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_credential_health_diagnostics_tests PRIVATE src)
  hcb_enable_warnings(hcb_credential_health_diagnostics_tests)
  hcb_enable_sanitizers(hcb_credential_health_diagnostics_tests)
  add_test(NAME hcb_credential_health_diagnostics_tests COMMAND hcb_credential_health_diagnostics_tests)

  qt_add_executable(hcb_macos_credential_adapter_tests
    tests/MacOSCredentialAdapterTest.cpp
    src/core/AppError.h
    src/core/OAuthCredentialStore.h
    ${HCB_MACOS_CREDENTIAL_SOURCES}
  )
  target_link_libraries(hcb_macos_credential_adapter_tests PRIVATE Qt6::Core Qt6::Test)
  if(APPLE)
    target_link_libraries(hcb_macos_credential_adapter_tests PRIVATE ${HCB_SECURITY_FRAMEWORK})
  endif()
  target_include_directories(hcb_macos_credential_adapter_tests PRIVATE src)
  hcb_enable_warnings(hcb_macos_credential_adapter_tests)
  hcb_enable_sanitizers(hcb_macos_credential_adapter_tests)
  add_test(NAME hcb_macos_credential_adapter_tests COMMAND hcb_macos_credential_adapter_tests)

  qt_add_executable(hcb_windows_credential_adapter_tests
    tests/WindowsCredentialAdapterTest.cpp
    src/core/AppError.h
    src/core/OAuthCredentialStore.h
    ${HCB_WINDOWS_CREDENTIAL_SOURCES}
  )
  target_link_libraries(hcb_windows_credential_adapter_tests PRIVATE Qt6::Core Qt6::Test)
  if(WIN32)
    target_link_libraries(hcb_windows_credential_adapter_tests PRIVATE Advapi32)
  endif()
  target_include_directories(hcb_windows_credential_adapter_tests PRIVATE src)
  hcb_enable_warnings(hcb_windows_credential_adapter_tests)
  hcb_enable_sanitizers(hcb_windows_credential_adapter_tests)
  add_test(NAME hcb_windows_credential_adapter_tests COMMAND hcb_windows_credential_adapter_tests)

  qt_add_executable(hcb_linux_credential_adapter_tests
    tests/LinuxCredentialAdapterTest.cpp
    src/core/AppError.h
    src/core/OAuthCredentialStore.h
    ${HCB_LINUX_CREDENTIAL_SOURCES}
  )
  target_link_libraries(hcb_linux_credential_adapter_tests PRIVATE Qt6::Core Qt6::Test)
  if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
    target_link_libraries(hcb_linux_credential_adapter_tests PRIVATE Qt6::DBus)
  endif()
  target_include_directories(hcb_linux_credential_adapter_tests PRIVATE src)
  hcb_enable_warnings(hcb_linux_credential_adapter_tests)
  hcb_enable_sanitizers(hcb_linux_credential_adapter_tests)
  add_test(NAME hcb_linux_credential_adapter_tests COMMAND hcb_linux_credential_adapter_tests)

  qt_add_executable(hcb_system_tray_adapter_tests
    tests/SystemTrayAdapterTest.cpp
    ${HCB_SYSTEM_TRAY_SOURCES}
  )
  qt_add_resources(hcb_system_tray_adapter_tests "hcb_system_tray_adapter_test_resources"
    PREFIX "/hcb"
    FILES "${HCB_SYSTEM_TRAY_ICON}"
  )
  target_link_libraries(hcb_system_tray_adapter_tests PRIVATE Qt6::Core Qt6::Gui Qt6::Test Qt6::Widgets)
  target_include_directories(hcb_system_tray_adapter_tests PRIVATE src)
  hcb_enable_warnings(hcb_system_tray_adapter_tests)
  hcb_enable_sanitizers(hcb_system_tray_adapter_tests)
  add_test(NAME hcb_system_tray_adapter_tests COMMAND hcb_system_tray_adapter_tests)

  qt_add_executable(hcb_notification_adapter_tests
    tests/NotificationAdapterTest.cpp
    ${HCB_NOTIFICATION_SOURCES}
  )
  target_link_libraries(hcb_notification_adapter_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_notification_adapter_tests PRIVATE src)
  hcb_enable_warnings(hcb_notification_adapter_tests)
  hcb_enable_sanitizers(hcb_notification_adapter_tests)
  add_test(NAME hcb_notification_adapter_tests COMMAND hcb_notification_adapter_tests)

  qt_add_executable(hcb_deep_link_adapter_tests
    tests/DeepLinkAdapterTest.cpp
    ${HCB_DEEP_LINK_SOURCES}
  )
  target_link_libraries(hcb_deep_link_adapter_tests PRIVATE Qt6::Core Qt6::Gui Qt6::Test)
  target_include_directories(hcb_deep_link_adapter_tests PRIVATE src)
  hcb_enable_warnings(hcb_deep_link_adapter_tests)
  hcb_enable_sanitizers(hcb_deep_link_adapter_tests)
  add_test(NAME hcb_deep_link_adapter_tests COMMAND hcb_deep_link_adapter_tests)

  qt_add_executable(hcb_oauth_token_revocation_client_tests
    tests/OAuthTokenRevocationClientTest.cpp
    src/core/AppError.h
    src/core/OAuthTokenRevocationClient.cpp
    src/core/OAuthTokenRevocationClient.h
    src/core/OAuthTokenRevoker.h
  )
  target_link_libraries(hcb_oauth_token_revocation_client_tests PRIVATE Qt6::Core Qt6::Network Qt6::Test)
  target_include_directories(hcb_oauth_token_revocation_client_tests PRIVATE src)
  hcb_enable_warnings(hcb_oauth_token_revocation_client_tests)
  hcb_enable_sanitizers(hcb_oauth_token_revocation_client_tests)
  add_test(NAME hcb_oauth_token_revocation_client_tests COMMAND hcb_oauth_token_revocation_client_tests)

  qt_add_executable(hcb_secret_redactor_tests
    tests/SecretRedactorTest.cpp
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
  )
  target_link_libraries(hcb_secret_redactor_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_secret_redactor_tests PRIVATE src)
  hcb_enable_warnings(hcb_secret_redactor_tests)
  hcb_enable_sanitizers(hcb_secret_redactor_tests)
  add_test(NAME hcb_secret_redactor_tests COMMAND hcb_secret_redactor_tests)

  qt_add_executable(hcb_structured_logger_tests
    tests/StructuredLoggerTest.cpp
    src/core/Clock.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/StructuredLogger.cpp
    src/core/StructuredLogger.h
  )
  target_link_libraries(hcb_structured_logger_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_structured_logger_tests PRIVATE src)
  hcb_enable_warnings(hcb_structured_logger_tests)
  hcb_enable_sanitizers(hcb_structured_logger_tests)
  add_test(NAME hcb_structured_logger_tests COMMAND hcb_structured_logger_tests)

  qt_add_executable(hcb_mutation_telemetry_store_tests
    tests/MutationTelemetryStoreTest.cpp
    src/core/AppError.h
    src/core/Clock.h
    src/core/FilePath.h
    src/core/MutationTelemetryStore.cpp
    src/core/MutationTelemetryStore.h
    src/data/LocalSchema.cpp
    src/data/LocalSchema.h
    src/data/SqliteConnection.cpp
    src/data/SqliteConnection.h
    src/data/SqliteQueryTimingTracker.cpp
    src/data/SqliteQueryTimingTracker.h
    src/data/SqliteMigrationRunner.cpp
    src/data/SqliteMigrationRunner.h
    src/data/SqliteTransaction.cpp
    src/data/SqliteTransaction.h
    src/data/SqliteWriterQueue.cpp
    src/data/SqliteWriterQueue.h
  )
  target_link_libraries(hcb_mutation_telemetry_store_tests PRIVATE hcb::sqlite Qt6::Core Qt6::Test)
  target_include_directories(hcb_mutation_telemetry_store_tests PRIVATE src)
  hcb_enable_warnings(hcb_mutation_telemetry_store_tests)
  hcb_enable_sanitizers(hcb_mutation_telemetry_store_tests)
  add_test(NAME hcb_mutation_telemetry_store_tests COMMAND hcb_mutation_telemetry_store_tests)

  qt_add_executable(hcb_startup_timing_tracker_tests
    tests/StartupTimingTrackerTest.cpp
    src/core/Clock.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/StructuredLogger.cpp
    src/core/StructuredLogger.h
    src/core/StartupTimingTracker.cpp
    src/core/StartupTimingTracker.h
  )
  target_link_libraries(hcb_startup_timing_tracker_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_startup_timing_tracker_tests PRIVATE src)
  hcb_enable_warnings(hcb_startup_timing_tracker_tests)
  hcb_enable_sanitizers(hcb_startup_timing_tracker_tests)
  add_test(NAME hcb_startup_timing_tracker_tests COMMAND hcb_startup_timing_tracker_tests)

  qt_add_executable(hcb_ui_transition_timing_tracker_tests
    tests/UiTransitionTimingTrackerTest.cpp
    src/core/Clock.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/StructuredLogger.cpp
    src/core/StructuredLogger.h
    src/core/UiTransitionTimingTracker.cpp
    src/core/UiTransitionTimingTracker.h
  )
  target_link_libraries(hcb_ui_transition_timing_tracker_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_ui_transition_timing_tracker_tests PRIVATE src)
  hcb_enable_warnings(hcb_ui_transition_timing_tracker_tests)
  hcb_enable_sanitizers(hcb_ui_transition_timing_tracker_tests)
  add_test(NAME hcb_ui_transition_timing_tracker_tests COMMAND hcb_ui_transition_timing_tracker_tests)

  qt_add_executable(hcb_diagnostics_snapshot_tests
    tests/DiagnosticsSnapshotTest.cpp
    src/core/Clock.h
    src/core/DiagnosticsSnapshot.cpp
    src/core/DiagnosticsSnapshot.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/StartupTimingTracker.cpp
    src/core/StartupTimingTracker.h
    src/core/StructuredLogger.cpp
    src/core/StructuredLogger.h
    src/core/UiTransitionTimingTracker.cpp
    src/core/UiTransitionTimingTracker.h
  )
  target_link_libraries(hcb_diagnostics_snapshot_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_diagnostics_snapshot_tests PRIVATE src)
  hcb_enable_warnings(hcb_diagnostics_snapshot_tests)
  hcb_enable_sanitizers(hcb_diagnostics_snapshot_tests)
  add_test(NAME hcb_diagnostics_snapshot_tests COMMAND hcb_diagnostics_snapshot_tests)

  qt_add_executable(hcb_diagnostics_json_exporter_tests
    tests/DiagnosticsJsonExporterTest.cpp
    src/core/Clock.h
    src/core/DiagnosticsJsonExporter.cpp
    src/core/DiagnosticsJsonExporter.h
    src/core/DiagnosticsSnapshot.h
    src/core/SecretRedactor.cpp
    src/core/SecretRedactor.h
    src/core/StartupTimingTracker.cpp
    src/core/StartupTimingTracker.h
    src/core/StructuredLogger.cpp
    src/core/StructuredLogger.h
    src/core/UiTransitionTimingTracker.cpp
    src/core/UiTransitionTimingTracker.h
  )
  target_link_libraries(hcb_diagnostics_json_exporter_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_diagnostics_json_exporter_tests PRIVATE src)
  hcb_enable_warnings(hcb_diagnostics_json_exporter_tests)
  hcb_enable_sanitizers(hcb_diagnostics_json_exporter_tests)
  add_test(NAME hcb_diagnostics_json_exporter_tests COMMAND hcb_diagnostics_json_exporter_tests)

  qt_add_executable(hcb_native_launch_benchmark_tests
    tests/NativeLaunchBenchmarkTest.cpp
    src/core/NativeLaunchBenchmark.cpp
    src/core/NativeLaunchBenchmark.h
  )
  target_link_libraries(hcb_native_launch_benchmark_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_native_launch_benchmark_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_launch_benchmark_tests)
  hcb_enable_sanitizers(hcb_native_launch_benchmark_tests)
  add_test(NAME hcb_native_launch_benchmark_tests COMMAND hcb_native_launch_benchmark_tests)
  add_test(NAME hcb_native_launch_benchmark
    COMMAND hcb_launch_benchmark
      --executable "$<TARGET_FILE:hcb_native>"
      --iterations 1
      --timeout-ms 15000
      -- -platform offscreen
  )

  qt_add_executable(hcb_native_idle_rss_benchmark_tests
    tests/NativeIdleRssBenchmarkTest.cpp
    src/core/NativeIdleRssBenchmark.cpp
    src/core/NativeIdleRssBenchmark.h
    src/core/NativeProcessMemory.cpp
    src/core/NativeProcessMemory.h
  )
  target_link_libraries(hcb_native_idle_rss_benchmark_tests PRIVATE Qt6::Core Qt6::Test)
  if(WIN32)
    target_link_libraries(hcb_native_idle_rss_benchmark_tests PRIVATE Psapi)
  endif()
  target_include_directories(hcb_native_idle_rss_benchmark_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_idle_rss_benchmark_tests)
  hcb_enable_sanitizers(hcb_native_idle_rss_benchmark_tests)
  add_test(NAME hcb_native_idle_rss_benchmark_tests COMMAND hcb_native_idle_rss_benchmark_tests)
  add_test(NAME hcb_native_idle_rss_benchmark
    COMMAND hcb_idle_rss_benchmark
      --executable "$<TARGET_FILE:hcb_native>"
      --idle-duration-ms 250
      --timeout-ms 15000
      -- -platform offscreen
  )

  qt_add_executable(hcb_native_command_palette_benchmark_tests
    tests/NativeCommandPaletteBenchmarkTest.cpp
    src/core/NativeCommandPaletteBenchmark.cpp
    src/core/NativeCommandPaletteBenchmark.h
  )
  target_link_libraries(hcb_native_command_palette_benchmark_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_native_command_palette_benchmark_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_command_palette_benchmark_tests)
  hcb_enable_sanitizers(hcb_native_command_palette_benchmark_tests)
  add_test(NAME hcb_native_command_palette_benchmark_tests
    COMMAND hcb_native_command_palette_benchmark_tests
  )
  add_test(NAME hcb_native_command_palette_benchmark
    COMMAND hcb_command_palette_benchmark
      --executable "$<TARGET_FILE:hcb_native>"
      --iterations 1
      --timeout-ms 15000
      -- -platform offscreen
  )

  qt_add_executable(hcb_native_performance_fixture_tests
    tests/NativePerformanceFixtureTest.cpp
    src/core/NativePerformanceFixture.cpp
    src/core/NativePerformanceFixture.h
  )
  target_link_libraries(hcb_native_performance_fixture_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_native_performance_fixture_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_performance_fixture_tests)
  hcb_enable_sanitizers(hcb_native_performance_fixture_tests)
  add_test(NAME hcb_native_performance_fixture_tests COMMAND hcb_native_performance_fixture_tests)

  qt_add_executable(hcb_native_performance_report_tests
    tests/NativePerformanceReportTest.cpp
    src/core/NativeLaunchBenchmark.h
    src/core/NativePerformanceReport.cpp
    src/core/NativePerformanceReport.h
  )
  target_link_libraries(hcb_native_performance_report_tests PRIVATE Qt6::Core Qt6::Test)
  target_include_directories(hcb_native_performance_report_tests PRIVATE src)
  hcb_enable_warnings(hcb_native_performance_report_tests)
  hcb_enable_sanitizers(hcb_native_performance_report_tests)
  add_test(NAME hcb_native_performance_report_tests COMMAND hcb_native_performance_report_tests)
  add_test(NAME hcb_native_performance_report
    COMMAND hcb_performance_report
      --executable "$<TARGET_FILE:hcb_native>"
      --launch-iterations 1
      --idle-duration-ms 250
      --timeout-ms 15000
      -- -platform offscreen
  )

  qt_add_executable(hcb_qml_tests tests/QmlTestMain.cpp)
  target_compile_definitions(hcb_qml_tests PRIVATE
    QUICK_TEST_SOURCE_DIR="${CMAKE_CURRENT_SOURCE_DIR}/tests/qml"
  )
  target_link_libraries(hcb_qml_tests PRIVATE Qt6::Qml Qt6::QuickControls2 Qt6::QuickTest)
  target_include_directories(hcb_qml_tests PRIVATE src)
  hcb_enable_warnings(hcb_qml_tests)
  hcb_enable_sanitizers(hcb_qml_tests)
  add_test(NAME hcb_qml_tests COMMAND hcb_qml_tests -platform offscreen)
  set_tests_properties(hcb_qml_tests PROPERTIES
    ENVIRONMENT "QT_LOGGING_RULES=qt.qpa.fonts.warning=false"
  )

  qt_add_executable(hcb_native_qml_shell_smoke tests/NativeQmlShellSmokeTest.cpp)
  add_dependencies(hcb_native_qml_shell_smoke hcb_native)
  target_link_libraries(hcb_native_qml_shell_smoke PRIVATE Qt6::Core Qt6::Test)
  hcb_enable_warnings(hcb_native_qml_shell_smoke)
  hcb_enable_sanitizers(hcb_native_qml_shell_smoke)
  add_test(NAME hcb_native_qml_shell_smoke
    COMMAND hcb_native_qml_shell_smoke "$<TARGET_FILE:hcb_native>"
  )
endif()

set(hcb_static_analysis_dependencies
  hcb_native
  hcb_launch_benchmark
  hcb_idle_rss_benchmark
  hcb_command_palette_benchmark
  hcb_small_performance_fixture
  hcb_medium_performance_fixture
  hcb_event15k_performance_fixture
  hcb_planner_benchmark
  hcb_quick_capture_benchmark
  hcb_local_search_benchmark
  hcb_task_scroll_benchmark
  hcb_calendar_navigation_benchmark
  hcb_performance_report
)
if(BUILD_TESTING)
  list(APPEND hcb_static_analysis_dependencies
    hcb_sqlite_integration_tests
    hcb_sqlite_connection_tests
    hcb_sqlite_writer_queue_tests
    hcb_sqlite_read_connection_pool_tests
    hcb_sqlite_transaction_tests
    hcb_sqlite_statement_cache_tests
    hcb_sqlite_migration_runner_tests
    hcb_sqlite_query_timing_tracker_tests
    hcb_temporary_sqlite_database_tests
    hcb_local_schema_tests
    hcb_core_tests
    hcb_file_path_tests
    hcb_test_bootstrap_tests
    hcb_cancellation_tests
    hcb_clock_tests
    hcb_application_lifecycle_tests
    hcb_feature_capability_registry_tests
    hcb_native_capability_report_tests
    hcb_command_registry_model_tests
    hcb_settings_registry_tests
    hcb_account_status_service_tests
    hcb_calendar_read_service_tests
    hcb_calendar_source_model_tests
    hcb_agenda_model_tests
    hcb_month_grid_model_tests
    hcb_scheduled_task_date_index_tests
    hcb_timeline_model_tests
    hcb_calendar_mutation_service_tests
    hcb_notes_model_tests
    hcb_task_recurrence_marker_tests
    hcb_optimistic_mutation_coordinator_tests
    hcb_undo_recovery_policy_tests
    hcb_domain_service_contract_tests
    hcb_task_bulk_mutation_service_tests
    hcb_task_list_read_service_tests
    hcb_task_read_service_tests
    hcb_google_mirror_store_tests
    hcb_settings_service_tests
    hcb_saved_search_store_tests
    hcb_sync_checkpoint_store_tests
    hcb_google_mirror_sync_service_tests
    hcb_sync_conflict_store_tests
    hcb_sync_three_way_merge_tests
    hcb_sync_scheduler_tests
    hcb_unified_local_search_ranker_tests
    hcb_recurrence_expansion_worker_tests
    hcb_recurrence_result_cache_tests
    hcb_calendar_layout_engine_tests
    hcb_native_planner_benchmark_tests
    hcb_native_quick_capture_benchmark_tests
    hcb_native_local_search_benchmark_tests
    hcb_native_task_scroll_benchmark_tests
    hcb_native_calendar_navigation_benchmark_tests
    hcb_task_list_model_tests
    hcb_task_model_tests
    hcb_task_tree_view_tests
    hcb_local_search_service_tests
    hcb_search_results_model_tests
    hcb_model_diff_policy_tests
    hcb_model_virtualization_tests
    hcb_google_sync_recovery_service_tests
    hcb_google_api_error_tests
    hcb_google_http_client_tests
    hcb_google_calendar_list_pull_client_tests
    hcb_google_calendar_event_pull_client_tests
    hcb_google_calendar_event_mutation_push_service_tests
    hcb_google_calendar_availability_client_tests
    hcb_google_task_list_pull_client_tests
    hcb_google_task_pull_client_tests
    hcb_google_task_mutation_push_service_tests
    hcb_mocked_oauth_transport_tests
    hcb_sync_backoff_policy_tests
    hcb_pkce_authorization_tests
    hcb_oauth_client_configuration_store_tests
    hcb_oauth_loopback_callback_listener_tests
    hcb_oauth_browser_authorization_launcher_tests
    hcb_oauth_token_exchange_client_tests
    hcb_oauth_token_refresh_client_tests
    hcb_oauth_disconnect_service_tests
    hcb_credential_health_diagnostics_tests
    hcb_oauth_token_revocation_client_tests
    hcb_secret_redactor_tests
    hcb_structured_logger_tests
    hcb_startup_timing_tracker_tests
    hcb_ui_transition_timing_tracker_tests
    hcb_diagnostics_snapshot_tests
    hcb_diagnostics_json_exporter_tests
    hcb_native_launch_benchmark_tests
    hcb_native_idle_rss_benchmark_tests
    hcb_native_command_palette_benchmark_tests
    hcb_native_performance_fixture_tests
    hcb_native_performance_report_tests
    hcb_qml_tests
    hcb_native_qml_shell_smoke
  )
endif()
hcb_add_static_analysis_target(${hcb_static_analysis_dependencies})
hcb_add_format_check_target()
if(BUILD_TESTING)
  hcb_add_bootstrap_check(
    hcb_native
    hcb_launch_benchmark
    hcb_idle_rss_benchmark
    hcb_command_palette_benchmark
    hcb_small_performance_fixture
    hcb_medium_performance_fixture
    hcb_event15k_performance_fixture
    hcb_performance_report
    hcb_sqlite_integration_tests
    hcb_sqlite_connection_tests
    hcb_sqlite_writer_queue_tests
    hcb_sqlite_read_connection_pool_tests
    hcb_sqlite_transaction_tests
    hcb_sqlite_statement_cache_tests
    hcb_sqlite_migration_runner_tests
    hcb_sqlite_query_timing_tracker_tests
    hcb_temporary_sqlite_database_tests
    hcb_local_schema_tests
    hcb_core_tests
    hcb_file_path_tests
    hcb_test_bootstrap_tests
    hcb_cancellation_tests
    hcb_clock_tests
    hcb_application_lifecycle_tests
    hcb_feature_capability_registry_tests
    hcb_native_capability_report_tests
    hcb_command_registry_model_tests
    hcb_settings_registry_tests
    hcb_account_status_service_tests
    hcb_calendar_read_service_tests
    hcb_calendar_source_model_tests
    hcb_agenda_model_tests
    hcb_month_grid_model_tests
    hcb_scheduled_task_date_index_tests
    hcb_timeline_model_tests
    hcb_calendar_mutation_service_tests
    hcb_notes_model_tests
    hcb_task_recurrence_marker_tests
    hcb_optimistic_mutation_coordinator_tests
    hcb_undo_recovery_policy_tests
    hcb_domain_service_contract_tests
    hcb_task_bulk_mutation_service_tests
    hcb_task_list_read_service_tests
    hcb_task_read_service_tests
    hcb_google_mirror_store_tests
    hcb_settings_service_tests
    hcb_saved_search_store_tests
    hcb_sync_checkpoint_store_tests
    hcb_google_mirror_sync_service_tests
    hcb_sync_conflict_store_tests
    hcb_sync_three_way_merge_tests
    hcb_sync_scheduler_tests
    hcb_unified_local_search_ranker_tests
    hcb_recurrence_expansion_worker_tests
    hcb_recurrence_result_cache_tests
    hcb_calendar_layout_engine_tests
    hcb_native_planner_benchmark_tests
    hcb_native_quick_capture_benchmark_tests
    hcb_native_local_search_benchmark_tests
    hcb_native_task_scroll_benchmark_tests
    hcb_native_calendar_navigation_benchmark_tests
    hcb_task_list_model_tests
    hcb_task_model_tests
    hcb_task_tree_view_tests
    hcb_local_search_service_tests
    hcb_search_results_model_tests
    hcb_model_diff_policy_tests
    hcb_model_virtualization_tests
    hcb_google_sync_recovery_service_tests
    hcb_google_api_error_tests
    hcb_google_http_client_tests
    hcb_google_calendar_list_pull_client_tests
    hcb_google_calendar_event_pull_client_tests
    hcb_google_calendar_event_mutation_push_service_tests
    hcb_google_calendar_availability_client_tests
    hcb_google_task_list_pull_client_tests
    hcb_google_task_pull_client_tests
    hcb_google_task_mutation_push_service_tests
    hcb_mocked_oauth_transport_tests
    hcb_sync_backoff_policy_tests
    hcb_pkce_authorization_tests
    hcb_oauth_client_configuration_store_tests
    hcb_oauth_loopback_callback_listener_tests
    hcb_oauth_browser_authorization_launcher_tests
    hcb_oauth_token_exchange_client_tests
    hcb_oauth_token_refresh_client_tests
    hcb_oauth_disconnect_service_tests
    hcb_credential_health_diagnostics_tests
    hcb_oauth_token_revocation_client_tests
    hcb_secret_redactor_tests
    hcb_structured_logger_tests
    hcb_startup_timing_tracker_tests
    hcb_ui_transition_timing_tracker_tests
    hcb_diagnostics_snapshot_tests
    hcb_diagnostics_json_exporter_tests
    hcb_native_launch_benchmark_tests
    hcb_native_idle_rss_benchmark_tests
    hcb_native_command_palette_benchmark_tests
    hcb_native_performance_fixture_tests
    hcb_native_performance_report_tests
    hcb_qml_tests
    hcb_native_qml_shell_smoke
  )
endif()
