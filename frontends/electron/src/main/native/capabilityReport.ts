import { tmpdir } from "node:os";
import { join } from "node:path";
import type {
  NativeCapabilityDescriptor,
  NativeCapabilityDiagnostic,
  NativeCapabilityFlags,
  NativeCapabilityKey,
  NativeCapabilityReport,
  NativeCapabilitiesResponse,
  NativeFeatureState
} from "@shared/ipc/contracts";
import { redactDiagnosticText } from "@shared/redaction";
import type { NativeAppPaths, NativePackageFormat } from "./types";

type NativePlatform = NativeCapabilitiesResponse["platform"];

interface BuildCapabilityReportInput {
  platform: NativePlatform;
  adapterId: string;
  appPaths: NativeAppPaths;
  packageFormat?: NativePackageFormat;
  flags: NativeCapabilityFlags;
  capabilityOverrides?: Partial<Record<NativeCapabilityKey, Partial<NativeCapabilityDescriptor>>>;
  diagnostics?: NativeCapabilityDiagnostic[];
}

const capabilityMetadata: Array<{
  key: NativeCapabilityKey;
  label: string;
  flag: keyof NativeCapabilityFlags;
}> = [
  { key: "appPaths", label: "App paths", flag: "supportsAppPaths" },
  { key: "credentialStorage", label: "Credential storage", flag: "supportsCredentialStorage" },
  { key: "tray", label: "Tray icon", flag: "supportsTray" },
  { key: "appMenu", label: "App menu", flag: "supportsAppMenu" },
  { key: "globalShortcuts", label: "Global shortcuts", flag: "supportsGlobalShortcut" },
  { key: "notifications", label: "Notifications", flag: "supportsNotifications" },
  { key: "customProtocol", label: "Protocol registration", flag: "supportsProtocolRegistration" },
  { key: "autostart", label: "Open at login", flag: "supportsAutostart" },
  { key: "updater", label: "Updater", flag: "supportsInPlaceAutoUpdate" },
  { key: "installerMetadata", label: "Installer metadata", flag: "supportsInstallerMetadata" },
  { key: "externalOpen", label: "External open", flag: "supportsExternalUrlOpen" },
  { key: "diagnostics", label: "Diagnostics", flag: "supportsDiagnosticsCollection" },
  { key: "oauthLoopback", label: "OAuth loopback", flag: "supportsOAuthLoopback" },
  { key: "mcpLoopback", label: "MCP loopback", flag: "supportsMcpLoopback" },
  { key: "packaging", label: "Packaging", flag: "supportsInstallerMetadata" }
];

export function nativePlatform(): NativePlatform {
  if (process.platform === "darwin" || process.platform === "linux" || process.platform === "win32") {
    return process.platform;
  }

  return "unknown";
}

export function defaultNativeAppPaths(baseDirectory = join(tmpdir(), "hot-cross-buns")): NativeAppPaths {
  return {
    configDirectory: join(baseDirectory, "config"),
    dataDirectory: join(baseDirectory, "data"),
    cacheDirectory: join(baseDirectory, "cache"),
    logsDirectory: join(baseDirectory, "logs"),
    diagnosticsDirectory: join(baseDirectory, "diagnostics"),
    tempDirectory: join(tmpdir(), "hot-cross-buns")
  };
}

export function buildNativeCapabilityReport(
  input: BuildCapabilityReportInput
): NativeCapabilityReport {
  return {
    platform: input.platform,
    adapterId: input.adapterId,
    packageFormat: input.packageFormat ?? "development",
    flags: input.flags,
    paths: [
      pathCapability("config", "adapter", input.appPaths.configDirectory),
      pathCapability("data", "adapter", input.appPaths.dataDirectory),
      pathCapability("cache", "adapter", input.appPaths.cacheDirectory),
      pathCapability("logs", "adapter", input.appPaths.logsDirectory),
      pathCapability("diagnostics", "adapter", input.appPaths.diagnosticsDirectory),
      pathCapability("temp", "adapter", input.appPaths.tempDirectory)
    ],
    capabilities: capabilityMetadata.map(({ key, label, flag }) => {
      const supported = Boolean(input.flags[flag]);
      const override = input.capabilityOverrides?.[key] ?? {};

      return {
        key,
        label,
        supported,
        state: supportedState(supported),
        ...(supported
          ? { message: `${label} is available through the ${input.adapterId} adapter.` }
          : { message: `${label} is not available through the ${input.adapterId} adapter.` }),
        ...override
      };
    }),
    diagnostics: input.diagnostics ?? []
  };
}

export function capabilityDiagnostic(
  key: NativeCapabilityKey,
  severity: NativeCapabilityDiagnostic["severity"],
  message: string
): NativeCapabilityDiagnostic {
  return {
    key,
    severity,
    message: redactDiagnosticText(message).slice(0, 500)
  };
}

function pathCapability(
  role: NativeCapabilityReport["paths"][number]["role"],
  source: string,
  rawPath: string
): NativeCapabilityReport["paths"][number] {
  return {
    role,
    available: rawPath.trim().length > 0,
    source,
    redactedPath: redactDiagnosticText(rawPath).slice(0, 1_000)
  };
}

function supportedState(supported: boolean): NativeFeatureState {
  return supported ? "ready" : "unsupported";
}
