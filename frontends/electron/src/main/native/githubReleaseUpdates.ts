import type { NativeOperationResult } from "./types";

export interface GitHubReleaseAssetPreference {
  label: string;
  matches: (asset: GitHubReleaseAsset) => boolean;
}

export const macReleaseAssetPreferences: GitHubReleaseAssetPreference[] = [
  {
    label: "macOS DMG",
    matches: (asset) => assetMatchesExtension(asset, "dmg")
  },
  {
    label: "macOS ZIP",
    matches: (asset) => assetMatchesExtension(asset, "zip")
  }
];

export const linuxReleaseAssetPreferences: GitHubReleaseAssetPreference[] = [
  {
    label: "Linux x64 AppImage",
    matches: (asset) =>
      assetMatchesExtension(asset, "AppImage") &&
      assetMatchesPattern(asset, /linux-(?:x64|x86_64)\.AppImage(?:$|[?#])/i)
  },
  {
    label: "Linux AppImage",
    matches: (asset) => assetMatchesExtension(asset, "AppImage")
  }
];

export const windowsReleaseAssetPreferences: GitHubReleaseAssetPreference[] = [
  {
    label: "Windows x64 installer",
    matches: (asset) =>
      assetMatchesExtension(asset, "exe") &&
      assetMatchesPattern(asset, /windows-(?:x64|x86_64)\.exe(?:$|[?#])/i)
  },
  {
    label: "Windows installer",
    matches: (asset) =>
      assetMatchesExtension(asset, "exe") &&
      assetMatchesPattern(asset, /windows(?:[-.][A-Za-z0-9_]+)*\.exe(?:$|[?#])/i)
  },
  {
    label: "Windows executable",
    matches: (asset) => assetMatchesExtension(asset, "exe")
  },
  {
    label: "Windows MSI",
    matches: (asset) => assetMatchesExtension(asset, "msi")
  },
  {
    label: "Windows ZIP",
    matches: (asset) => assetMatchesExtension(asset, "zip")
  }
];

export async function checkGitHubReleaseForUpdates(input: {
  appVersion: string;
  assetPreferences: GitHubReleaseAssetPreference[];
  userAgentVersion: string;
}): Promise<NativeOperationResult> {
  const checkedAt = new Date().toISOString();

  try {
    const response = await fetch("https://api.github.com/repos/gongahkia/hot-cross-buns/releases/latest", {
      headers: {
        Accept: "application/vnd.github+json",
        "User-Agent": `HotCrossBuns/${input.userAgentVersion}`
      },
      signal: AbortSignal.timeout(15_000)
    });

    if (!response.ok) {
      throw new Error(`GitHub Releases returned HTTP ${response.status}.`);
    }

    const release = parseGitHubRelease(await response.json());
    const latestVersion = normalizedVersionString(release.tagName);
    const currentVersion = normalizedVersionString(input.appVersion);
    const updateAvailable = compareReleaseVersions(latestVersion, currentVersion) > 0;
    const selectedAsset = selectReleaseAsset(release.assets, input.assetPreferences);

    return {
      checkedAt,
      ...(selectedAsset ? { downloadUrl: selectedAsset.browserDownloadUrl } : {}),
      latestVersion,
      ok: true,
      releaseName: release.name || release.tagName,
      releaseUrl: release.htmlUrl,
      state: "ready",
      updateAvailable,
      message: releaseCheckMessage({
        assetPreferences: input.assetPreferences,
        latestVersion,
        selectedAsset,
        updateAvailable
      })
    };
  } catch (error) {
    return {
      checkedAt,
      ok: false,
      state: "error",
      message: error instanceof Error ? error.message : "GitHub release check failed."
    };
  }
}

interface GitHubReleaseAsset {
  browserDownloadUrl: string;
  name: string;
}

interface GitHubRelease {
  assets: GitHubReleaseAsset[];
  htmlUrl: string;
  name: string;
  tagName: string;
}

function parseGitHubRelease(value: unknown): GitHubRelease {
  const release = value as {
    assets?: Array<{ browser_download_url?: unknown; name?: unknown }>;
    html_url?: unknown;
    name?: unknown;
    tag_name?: unknown;
  };

  if (typeof release.tag_name !== "string" || typeof release.html_url !== "string") {
    throw new Error("GitHub release metadata was incomplete.");
  }

  return {
    assets: Array.isArray(release.assets)
      ? release.assets
        .filter((asset) => typeof asset.name === "string" && typeof asset.browser_download_url === "string")
        .map((asset) => ({
          browserDownloadUrl: asset.browser_download_url as string,
          name: asset.name as string
        }))
      : [],
    htmlUrl: release.html_url,
    name: typeof release.name === "string" ? release.name : release.tag_name,
    tagName: release.tag_name
  };
}

function selectReleaseAsset(
  assets: GitHubReleaseAsset[],
  preferences: GitHubReleaseAssetPreference[]
): GitHubReleaseAsset | undefined {
  for (const preference of preferences) {
    const asset = assets.find((candidate) => preference.matches(candidate));

    if (asset) {
      return asset;
    }
  }

  return undefined;
}

function releaseCheckMessage(input: {
  assetPreferences: GitHubReleaseAssetPreference[];
  latestVersion: string;
  selectedAsset: GitHubReleaseAsset | undefined;
  updateAvailable: boolean;
}): string {
  if (!input.updateAvailable) {
    return "Hot Cross Buns is up to date.";
  }

  if (input.selectedAsset) {
    return `Hot Cross Buns ${input.latestVersion} is available from GitHub Releases.`;
  }

  const preferredAssets = input.assetPreferences.map((preference) => preference.label).join(" or ");

  return `Hot Cross Buns ${input.latestVersion} is available from GitHub Releases, but no ${preferredAssets || "matching"} asset was found.`;
}

function assetMatchesExtension(asset: GitHubReleaseAsset, extension: string): boolean {
  const pattern = new RegExp(`\\.${escapeRegExp(extension)}(?:$|[?#])`, "i");

  return pattern.test(asset.name) || pattern.test(asset.browserDownloadUrl);
}

function assetMatchesPattern(asset: GitHubReleaseAsset, pattern: RegExp): boolean {
  return pattern.test(asset.name) || pattern.test(asset.browserDownloadUrl);
}

function normalizedVersionString(value: string): string {
  return value.trim().replace(/^v/i, "") || "0";
}

function compareReleaseVersions(left: string, right: string): number {
  const leftParts = versionParts(left);
  const rightParts = versionParts(right);
  const length = Math.max(leftParts.length, rightParts.length);

  for (let index = 0; index < length; index += 1) {
    const diff = (leftParts[index] ?? 0) - (rightParts[index] ?? 0);

    if (diff !== 0) {
      return diff;
    }
  }

  return 0;
}

function versionParts(value: string): number[] {
  return normalizedVersionString(value)
    .split(/[^0-9]+/)
    .filter(Boolean)
    .map((part) => Number.parseInt(part, 10))
    .filter((part) => Number.isFinite(part));
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
