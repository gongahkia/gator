#pragma once

#include <QByteArray>
#include <QString>

#include <array>
#include <cstdint>
#include <vector>

namespace hcb {

enum class NativePlatform : std::uint8_t {
  MacOS,
  Linux,
  Windows,
  Unknown,
};

enum class NativePackageFormat : std::uint8_t {
  Development,
  Dmg,
  Zip,
  AppImage,
  Deb,
  Rpm,
  Nsis,
  Portable,
  Unknown,
};

enum class NativeFeatureState : std::uint8_t {
  Ready,
  Pending,
  Disabled,
  Unsupported,
  Error,
};

enum class NativeCapabilityKey : std::uint8_t {
  AppPaths,
  CredentialStorage,
  Tray,
  AppMenu,
  GlobalShortcuts,
  Notifications,
  CustomProtocol,
  Autostart,
  Updater,
  InstallerMetadata,
  ExternalOpen,
  Diagnostics,
  OAuthLoopback,
  McpLoopback,
  Packaging,
  Count,
};

enum class NativePathRole : std::uint8_t {
  Config,
  Data,
  Cache,
  Logs,
  Diagnostics,
  Temp,
};

enum class NativeCapabilityDiagnosticSeverity : std::uint8_t {
  Info,
  Warning,
  Blocker,
};

struct NativeCapabilityFlags final {
  bool supportsAppPaths{false};
  bool supportsTray{false};
  bool supportsAppMenu{false};
  bool supportsGlobalShortcut{false};
  bool supportsNotifications{false};
  bool supportsNotificationPermissionQuery{false};
  bool supportsProtocolRegistration{false};
  bool supportsProtocolRegistrationCheck{false};
  bool supportsAutostart{false};
  bool supportsInPlaceAutoUpdate{false};
  bool supportsInstallerMetadata{false};
  bool supportsExternalUrlOpen{false};
  bool supportsDiagnosticsCollection{false};
  bool supportsCredentialStorage{false};
  bool supportsOAuthLoopback{false};
  bool supportsMcpLoopback{false};
  bool requiresSignedBuildForNotifications{false};
  bool hasWaylandSession{false};
  bool hasPortalShortcutSupport{false};
};

struct NativePathCapabilityInput final {
  NativePathRole role;
  QString source;
  QString path;
};

struct NativeCapabilityDiagnostic final {
  NativeCapabilityKey key;
  NativeCapabilityDiagnosticSeverity severity;
  QString message;
};

struct NativeCapabilityReportInput final {
  NativePlatform platform;
  QString adapterId;
  NativePackageFormat packageFormat{NativePackageFormat::Development};
  NativeCapabilityFlags flags;
  std::vector<NativePathCapabilityInput> paths;
  std::vector<NativeCapabilityDiagnostic> diagnostics;
  QString homeDirectory;
};

struct NativePathCapability final {
  NativePathRole role;
  bool available;
  QString source;
  QString redactedPath;
};

struct NativeCapabilityDescriptor final {
  NativeCapabilityKey key;
  QString label;
  bool supported;
  NativeFeatureState state;
  QString message;
};

struct NativeCapabilityReport final {
  NativePlatform platform;
  QString adapterId;
  NativePackageFormat packageFormat;
  NativeCapabilityFlags flags;
  std::vector<NativePathCapability> paths;
  std::vector<NativeCapabilityDescriptor> capabilities;
  std::vector<NativeCapabilityDiagnostic> diagnostics;
};

class NativeCapabilityReportBuilder final {
public:
  [[nodiscard]] static NativeCapabilityReport build(NativeCapabilityReportInput input);
  [[nodiscard]] static QByteArray toJson(const NativeCapabilityReport& report);
};

} // namespace hcb
