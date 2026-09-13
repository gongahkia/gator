#include "core/NativeCapabilityReport.h"

#include "core/SecretRedactor.h"

#include <QDir>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>

#include <algorithm>
#include <array>
#include <utility>

namespace hcb {
namespace {

constexpr std::size_t kMaximumPaths = 12;
constexpr std::size_t kMaximumDiagnostics = 40;

struct CapabilityMetadata final {
  NativeCapabilityKey key;
  QStringView label;
  bool NativeCapabilityFlags::* flag;
};

constexpr std::array<CapabilityMetadata, static_cast<std::size_t>(NativeCapabilityKey::Count)>
    kCapabilityMetadata{{
        {NativeCapabilityKey::AppPaths, u"App paths", &NativeCapabilityFlags::supportsAppPaths},
        {NativeCapabilityKey::CredentialStorage,
         u"Credential storage",
         &NativeCapabilityFlags::supportsCredentialStorage},
        {NativeCapabilityKey::Tray, u"Tray icon", &NativeCapabilityFlags::supportsTray},
        {NativeCapabilityKey::AppMenu, u"App menu", &NativeCapabilityFlags::supportsAppMenu},
        {NativeCapabilityKey::GlobalShortcuts,
         u"Global shortcuts",
         &NativeCapabilityFlags::supportsGlobalShortcut},
        {NativeCapabilityKey::Notifications,
         u"Notifications",
         &NativeCapabilityFlags::supportsNotifications},
        {NativeCapabilityKey::CustomProtocol,
         u"Protocol registration",
         &NativeCapabilityFlags::supportsProtocolRegistration},
        {NativeCapabilityKey::Autostart,
         u"Open at login",
         &NativeCapabilityFlags::supportsAutostart},
        {NativeCapabilityKey::Updater,
         u"Updater",
         &NativeCapabilityFlags::supportsInPlaceAutoUpdate},
        {NativeCapabilityKey::InstallerMetadata,
         u"Installer metadata",
         &NativeCapabilityFlags::supportsInstallerMetadata},
        {NativeCapabilityKey::ExternalOpen,
         u"External open",
         &NativeCapabilityFlags::supportsExternalUrlOpen},
        {NativeCapabilityKey::Diagnostics,
         u"Diagnostics",
         &NativeCapabilityFlags::supportsDiagnosticsCollection},
        {NativeCapabilityKey::OAuthLoopback,
         u"OAuth loopback",
         &NativeCapabilityFlags::supportsOAuthLoopback},
        {NativeCapabilityKey::McpLoopback,
         u"MCP loopback",
         &NativeCapabilityFlags::supportsMcpLoopback},
        {NativeCapabilityKey::Packaging,
         u"Packaging",
         &NativeCapabilityFlags::supportsInstallerMetadata},
    }};

QString safeText(QStringView value, qsizetype maximumLength, QStringView fallback) {
  const QString redacted = SecretRedactor::redactText(value, maximumLength);
  return redacted.isEmpty() ? fallback.toString() : redacted;
}

QString normalizedPath(QStringView path) {
  return QDir::fromNativeSeparators(path.toString()).trimmed();
}

QString redactPath(QStringView path, QStringView homeDirectory) {
  const QString normalized = normalizedPath(path);
  if (normalized.isEmpty()) {
    return {};
  }
  const QString home = normalizedPath(homeDirectory);
  if (!home.isEmpty() && (normalized == home || normalized.startsWith(home + QLatin1Char('/')))) {
    return QStringLiteral("<home>") + normalized.mid(home.size());
  }
  return QStringLiteral("<path>");
}

QString platformName(NativePlatform platform) {
  switch (platform) {
  case NativePlatform::MacOS:
    return QStringLiteral("darwin");
  case NativePlatform::Linux:
    return QStringLiteral("linux");
  case NativePlatform::Windows:
    return QStringLiteral("win32");
  case NativePlatform::Unknown:
    return QStringLiteral("unknown");
  }
  return QStringLiteral("unknown");
}

QString packageFormatName(NativePackageFormat packageFormat) {
  switch (packageFormat) {
  case NativePackageFormat::Development:
    return QStringLiteral("development");
  case NativePackageFormat::Dmg:
    return QStringLiteral("dmg");
  case NativePackageFormat::Zip:
    return QStringLiteral("zip");
  case NativePackageFormat::AppImage:
    return QStringLiteral("appimage");
  case NativePackageFormat::Deb:
    return QStringLiteral("deb");
  case NativePackageFormat::Rpm:
    return QStringLiteral("rpm");
  case NativePackageFormat::Nsis:
    return QStringLiteral("nsis");
  case NativePackageFormat::Portable:
    return QStringLiteral("portable");
  case NativePackageFormat::Unknown:
    return QStringLiteral("unknown");
  }
  return QStringLiteral("unknown");
}

QString featureStateName(NativeFeatureState state) {
  switch (state) {
  case NativeFeatureState::Ready:
    return QStringLiteral("ready");
  case NativeFeatureState::Pending:
    return QStringLiteral("pending");
  case NativeFeatureState::Disabled:
    return QStringLiteral("disabled");
  case NativeFeatureState::Unsupported:
    return QStringLiteral("unsupported");
  case NativeFeatureState::Error:
    return QStringLiteral("error");
  }
  return QStringLiteral("error");
}

QString capabilityKeyName(NativeCapabilityKey key) {
  switch (key) {
  case NativeCapabilityKey::AppPaths:
    return QStringLiteral("appPaths");
  case NativeCapabilityKey::CredentialStorage:
    return QStringLiteral("credentialStorage");
  case NativeCapabilityKey::Tray:
    return QStringLiteral("tray");
  case NativeCapabilityKey::AppMenu:
    return QStringLiteral("appMenu");
  case NativeCapabilityKey::GlobalShortcuts:
    return QStringLiteral("globalShortcuts");
  case NativeCapabilityKey::Notifications:
    return QStringLiteral("notifications");
  case NativeCapabilityKey::CustomProtocol:
    return QStringLiteral("customProtocol");
  case NativeCapabilityKey::Autostart:
    return QStringLiteral("autostart");
  case NativeCapabilityKey::Updater:
    return QStringLiteral("updater");
  case NativeCapabilityKey::InstallerMetadata:
    return QStringLiteral("installerMetadata");
  case NativeCapabilityKey::ExternalOpen:
    return QStringLiteral("externalOpen");
  case NativeCapabilityKey::Diagnostics:
    return QStringLiteral("diagnostics");
  case NativeCapabilityKey::OAuthLoopback:
    return QStringLiteral("oauthLoopback");
  case NativeCapabilityKey::McpLoopback:
    return QStringLiteral("mcpLoopback");
  case NativeCapabilityKey::Packaging:
    return QStringLiteral("packaging");
  case NativeCapabilityKey::Count:
    break;
  }
  return QStringLiteral("unknown");
}

QString pathRoleName(NativePathRole role) {
  switch (role) {
  case NativePathRole::Config:
    return QStringLiteral("config");
  case NativePathRole::Data:
    return QStringLiteral("data");
  case NativePathRole::Cache:
    return QStringLiteral("cache");
  case NativePathRole::Logs:
    return QStringLiteral("logs");
  case NativePathRole::Diagnostics:
    return QStringLiteral("diagnostics");
  case NativePathRole::Temp:
    return QStringLiteral("temp");
  }
  return QStringLiteral("unknown");
}

QString diagnosticSeverityName(NativeCapabilityDiagnosticSeverity severity) {
  switch (severity) {
  case NativeCapabilityDiagnosticSeverity::Info:
    return QStringLiteral("info");
  case NativeCapabilityDiagnosticSeverity::Warning:
    return QStringLiteral("warning");
  case NativeCapabilityDiagnosticSeverity::Blocker:
    return QStringLiteral("blocker");
  }
  return QStringLiteral("blocker");
}

QString redactDiagnostic(QString message, QStringView homeDirectory) {
  const QString home = normalizedPath(homeDirectory);
  if (!home.isEmpty()) {
    message.replace(home, QStringLiteral("<home>"), Qt::CaseSensitive);
  }
  return safeText(message, 500, QStringLiteral("native capability diagnostic"));
}

QJsonObject flagsObject(const NativeCapabilityFlags& flags) {
  return {{QStringLiteral("supportsAppPaths"), flags.supportsAppPaths},
          {QStringLiteral("supportsTray"), flags.supportsTray},
          {QStringLiteral("supportsAppMenu"), flags.supportsAppMenu},
          {QStringLiteral("supportsGlobalShortcut"), flags.supportsGlobalShortcut},
          {QStringLiteral("supportsNotifications"), flags.supportsNotifications},
          {QStringLiteral("supportsNotificationPermissionQuery"),
           flags.supportsNotificationPermissionQuery},
          {QStringLiteral("supportsProtocolRegistration"), flags.supportsProtocolRegistration},
          {QStringLiteral("supportsProtocolRegistrationCheck"),
           flags.supportsProtocolRegistrationCheck},
          {QStringLiteral("supportsAutostart"), flags.supportsAutostart},
          {QStringLiteral("supportsInPlaceAutoUpdate"), flags.supportsInPlaceAutoUpdate},
          {QStringLiteral("supportsInstallerMetadata"), flags.supportsInstallerMetadata},
          {QStringLiteral("supportsExternalUrlOpen"), flags.supportsExternalUrlOpen},
          {QStringLiteral("supportsDiagnosticsCollection"), flags.supportsDiagnosticsCollection},
          {QStringLiteral("supportsCredentialStorage"), flags.supportsCredentialStorage},
          {QStringLiteral("supportsOAuthLoopback"), flags.supportsOAuthLoopback},
          {QStringLiteral("supportsMcpLoopback"), flags.supportsMcpLoopback},
          {QStringLiteral("requiresSignedBuildForNotifications"),
           flags.requiresSignedBuildForNotifications},
          {QStringLiteral("hasWaylandSession"), flags.hasWaylandSession},
          {QStringLiteral("hasPortalShortcutSupport"), flags.hasPortalShortcutSupport}};
}

} // namespace

NativeCapabilityReport NativeCapabilityReportBuilder::build(NativeCapabilityReportInput input) {
  NativeCapabilityReport report{input.platform,
                                safeText(input.adapterId, 80, QStringLiteral("unknown")),
                                input.packageFormat,
                                input.flags,
                                {},
                                {},
                                {}};
  const std::size_t pathCount = std::min(input.paths.size(), kMaximumPaths);
  report.paths.reserve(pathCount);
  for (std::size_t index = 0; index < pathCount; ++index) {
    const NativePathCapabilityInput& path = input.paths[index];
    report.paths.push_back(
        NativePathCapability{path.role,
                             !path.path.trimmed().isEmpty(),
                             safeText(path.source, 120, QStringLiteral("adapter")),
                             redactPath(path.path, input.homeDirectory)});
  }

  report.capabilities.reserve(kCapabilityMetadata.size());
  for (const CapabilityMetadata& metadata : kCapabilityMetadata) {
    const bool supported = input.flags.*(metadata.flag);
    const QString label = metadata.label.toString();
    report.capabilities.push_back(NativeCapabilityDescriptor{
        metadata.key,
        label,
        supported,
        supported ? NativeFeatureState::Ready : NativeFeatureState::Unsupported,
        QStringLiteral("%1 is %2 through the %3 adapter.")
            .arg(label,
                 supported ? QStringLiteral("available") : QStringLiteral("not available"),
                 report.adapterId)});
  }

  const std::size_t diagnosticCount = std::min(input.diagnostics.size(), kMaximumDiagnostics);
  report.diagnostics.reserve(diagnosticCount);
  for (std::size_t index = 0; index < diagnosticCount; ++index) {
    NativeCapabilityDiagnostic diagnostic = std::move(input.diagnostics[index]);
    diagnostic.message = redactDiagnostic(std::move(diagnostic.message), input.homeDirectory);
    report.diagnostics.push_back(std::move(diagnostic));
  }
  return report;
}

QByteArray NativeCapabilityReportBuilder::toJson(const NativeCapabilityReport& report) {
  QJsonArray paths;
  for (const NativePathCapability& path : report.paths) {
    QJsonObject object{
        {QStringLiteral("role"), pathRoleName(path.role)},
        {QStringLiteral("available"), path.available},
        {QStringLiteral("source"), safeText(path.source, 120, QStringLiteral("adapter"))}};
    if (!path.redactedPath.isEmpty()) {
      object.insert(QStringLiteral("redactedPath"), path.redactedPath);
    }
    paths.append(std::move(object));
  }

  QJsonArray capabilities;
  for (const NativeCapabilityDescriptor& capability : report.capabilities) {
    capabilities.append(QJsonObject{
        {QStringLiteral("key"), capabilityKeyName(capability.key)},
        {QStringLiteral("label"), safeText(capability.label, 80, QStringLiteral("Capability"))},
        {QStringLiteral("supported"), capability.supported},
        {QStringLiteral("state"), featureStateName(capability.state)},
        {QStringLiteral("message"),
         safeText(capability.message, 500, QStringLiteral("Unavailable"))}});
  }

  QJsonArray diagnostics;
  const std::size_t diagnosticCount = std::min(report.diagnostics.size(), kMaximumDiagnostics);
  for (std::size_t index = 0; index < diagnosticCount; ++index) {
    const NativeCapabilityDiagnostic& diagnostic = report.diagnostics[index];
    diagnostics.append(QJsonObject{
        {QStringLiteral("key"), capabilityKeyName(diagnostic.key)},
        {QStringLiteral("severity"), diagnosticSeverityName(diagnostic.severity)},
        {QStringLiteral("message"),
         safeText(diagnostic.message, 500, QStringLiteral("native capability diagnostic"))}});
  }

  return QJsonDocument(
             QJsonObject{{QStringLiteral("platform"), platformName(report.platform)},
                         {QStringLiteral("adapterId"),
                          safeText(report.adapterId, 80, QStringLiteral("unknown"))},
                         {QStringLiteral("packageFormat"), packageFormatName(report.packageFormat)},
                         {QStringLiteral("flags"), flagsObject(report.flags)},
                         {QStringLiteral("paths"), paths},
                         {QStringLiteral("capabilities"), capabilities},
                         {QStringLiteral("diagnostics"), diagnostics}})
      .toJson(QJsonDocument::Compact);
}

} // namespace hcb
