import type { NativePlatformAdapter, NativePlatformCapabilities } from "./types";

type NativePlatform = NativePlatformCapabilities["platform"];
type NativeAdapterKind = "electron-mac" | "electron-linux-preview" | "electron-windows-preview" | "noop";

export function nativePlatformFromNodePlatform(platform: NodeJS.Platform | string): NativePlatform {
  if (platform === "darwin" || platform === "linux" || platform === "win32") {
    return platform;
  }

  return "unknown";
}

export function nativeAdapterKindForPlatform(platform: NodeJS.Platform | string): NativeAdapterKind {
  switch (nativePlatformFromNodePlatform(platform)) {
    case "darwin":
      return "electron-mac";
    case "linux":
      return "electron-linux-preview";
    case "win32":
      return "electron-windows-preview";
    case "unknown":
      return "noop";
  }
}

export async function createNativeAdapter(
  platform: NodeJS.Platform | string = process.platform
): Promise<NativePlatformAdapter> {
  const normalizedPlatform = nativePlatformFromNodePlatform(platform);

  switch (nativeAdapterKindForPlatform(normalizedPlatform)) {
    case "electron-mac": {
      const { createElectronMacNativeAdapter } = await import("./electronMacAdapter");
      return createElectronMacNativeAdapter();
    }
    case "electron-linux-preview": {
      const { createElectronLinuxNativeAdapter } = await import("./electronLinuxAdapter");
      return createElectronLinuxNativeAdapter();
    }
    case "electron-windows-preview": {
      const { createElectronWindowsNativeAdapter } = await import("./electronWindowsAdapter");
      return createElectronWindowsNativeAdapter();
    }
    case "noop": {
      const { createNoopNativeAdapter } = await import("./noopAdapter");
      return createNoopNativeAdapter(normalizedPlatform);
    }
  }
}
