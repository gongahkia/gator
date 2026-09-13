import { afterEach, describe, expect, it, vi } from "vitest";
import {
  checkGitHubReleaseForUpdates,
  linuxReleaseAssetPreferences,
  macReleaseAssetPreferences,
  windowsReleaseAssetPreferences
} from "./githubReleaseUpdates";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("GitHub release update checks", () => {
  it("selects Linux AppImage assets for Linux update checks", async () => {
    const fetchMock = stubLatestRelease({
      tag_name: "v5.1.0",
      html_url: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v5.1.0",
      name: "5.1.0",
      assets: [
        asset("Hot-Cross-Buns-5.1.0.dmg", "https://example.test/Hot-Cross-Buns-5.1.0.dmg"),
        asset(
          "Hot-Cross-Buns-5.1.0-linux-arm64.AppImage",
          "https://example.test/Hot-Cross-Buns-5.1.0-linux-arm64.AppImage"
        ),
        asset(
          "Hot-Cross-Buns-5.1.0-linux-x86_64.AppImage",
          "https://example.test/Hot-Cross-Buns-5.1.0-linux-x86_64.AppImage"
        )
      ]
    });

    await expect(checkGitHubReleaseForUpdates({
      appVersion: "5.0.0",
      assetPreferences: linuxReleaseAssetPreferences,
      userAgentVersion: "5.0.0"
    })).resolves.toMatchObject({
      downloadUrl: "https://example.test/Hot-Cross-Buns-5.1.0-linux-x86_64.AppImage",
      latestVersion: "5.1.0",
      ok: true,
      updateAvailable: true
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.github.com/repos/gongahkia/hot-cross-buns/releases/latest",
      expect.objectContaining({
        headers: expect.objectContaining({
          "User-Agent": "HotCrossBuns/5.0.0"
        })
      })
    );
  });

  it("prefers Windows x64 NSIS installers for Windows update checks", async () => {
    stubLatestRelease({
      tag_name: "v5.1.0",
      html_url: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v5.1.0",
      name: "5.1.0",
      assets: [
        asset(
          "Hot-Cross-Buns-5.1.0-windows-arm64.exe",
          "https://example.test/Hot-Cross-Buns-5.1.0-windows-arm64.exe"
        ),
        asset("Hot-Cross-Buns-windows.exe", "https://example.test/Hot-Cross-Buns-windows.exe"),
        asset(
          "Hot-Cross-Buns-5.1.0-windows-x64.exe",
          "https://example.test/Hot-Cross-Buns-5.1.0-windows-x64.exe"
        )
      ]
    });

    await expect(checkGitHubReleaseForUpdates({
      appVersion: "5.0.0",
      assetPreferences: windowsReleaseAssetPreferences,
      userAgentVersion: "5.0.0"
    })).resolves.toMatchObject({
      downloadUrl: "https://example.test/Hot-Cross-Buns-5.1.0-windows-x64.exe",
      latestVersion: "5.1.0",
      ok: true,
      updateAvailable: true
    });
  });

  it("falls back to stable Windows EXE aliases when no x64 installer exists", async () => {
    stubLatestRelease({
      tag_name: "v5.1.0",
      html_url: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v5.1.0",
      name: "5.1.0",
      assets: [
        asset("Hot-Cross-Buns-5.1.0.msi", "https://example.test/Hot-Cross-Buns-5.1.0.msi"),
        asset("Hot-Cross-Buns-windows.exe", "https://example.test/Hot-Cross-Buns-windows.exe")
      ]
    });

    await expect(checkGitHubReleaseForUpdates({
      appVersion: "5.0.0",
      assetPreferences: windowsReleaseAssetPreferences,
      userAgentVersion: "5.0.0"
    })).resolves.toMatchObject({
      downloadUrl: "https://example.test/Hot-Cross-Buns-windows.exe",
      updateAvailable: true
    });
  });

  it("prefers macOS DMG assets before ZIP assets", async () => {
    stubLatestRelease({
      tag_name: "v5.1.0",
      html_url: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v5.1.0",
      name: "5.1.0",
      assets: [
        asset("Hot-Cross-Buns-5.1.0-mac.zip", "https://example.test/Hot-Cross-Buns-5.1.0-mac.zip"),
        asset("Hot-Cross-Buns-5.1.0.dmg", "https://example.test/Hot-Cross-Buns-5.1.0.dmg")
      ]
    });

    await expect(checkGitHubReleaseForUpdates({
      appVersion: "5.0.0",
      assetPreferences: macReleaseAssetPreferences,
      userAgentVersion: "5.0.0"
    })).resolves.toMatchObject({
      downloadUrl: "https://example.test/Hot-Cross-Buns-5.1.0.dmg",
      updateAvailable: true
    });
  });

  it("falls back to macOS ZIP assets when no DMG asset exists", async () => {
    stubLatestRelease({
      tag_name: "v5.1.0",
      html_url: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v5.1.0",
      name: "5.1.0",
      assets: [
        asset("Hot-Cross-Buns-5.1.0-mac.zip", "https://example.test/Hot-Cross-Buns-5.1.0-mac.zip")
      ]
    });

    await expect(checkGitHubReleaseForUpdates({
      appVersion: "5.0.0",
      assetPreferences: macReleaseAssetPreferences,
      userAgentVersion: "5.0.0"
    })).resolves.toMatchObject({
      downloadUrl: "https://example.test/Hot-Cross-Buns-5.1.0-mac.zip",
      updateAvailable: true
    });
  });

  it("keeps the previous release-tag version parsing semantics", async () => {
    stubLatestRelease({
      tag_name: "release-v5.1.0",
      html_url: "https://github.com/gongahkia/hot-cross-buns/releases/tag/release-v5.1.0",
      name: "5.1.0",
      assets: [
        asset(
          "Hot-Cross-Buns-5.1.0-linux-x86_64.AppImage",
          "https://example.test/Hot-Cross-Buns-5.1.0-linux-x86_64.AppImage"
        )
      ]
    });

    await expect(checkGitHubReleaseForUpdates({
      appVersion: "5.0.0",
      assetPreferences: linuxReleaseAssetPreferences,
      userAgentVersion: "5.0.0"
    })).resolves.toMatchObject({
      latestVersion: "release-v5.1.0",
      updateAvailable: true
    });
  });

  it("keeps manual release discovery working when no preferred asset exists", async () => {
    stubLatestRelease({
      tag_name: "v5.1.0",
      html_url: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v5.1.0",
      name: "5.1.0",
      assets: [
        asset("Hot-Cross-Buns-5.1.0.dmg", "https://example.test/Hot-Cross-Buns-5.1.0.dmg")
      ]
    });

    const result = await checkGitHubReleaseForUpdates({
      appVersion: "5.0.0",
      assetPreferences: linuxReleaseAssetPreferences,
      userAgentVersion: "5.0.0"
    });

    expect(result).toMatchObject({
      message: expect.stringContaining("no Linux x64 AppImage or Linux AppImage asset was found"),
      releaseUrl: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v5.1.0",
      updateAvailable: true
    });
    expect(result.downloadUrl).toBeUndefined();
  });

  it("reports up to date without requiring a preferred asset", async () => {
    stubLatestRelease({
      tag_name: "v5.0.0",
      html_url: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v5.0.0",
      name: "5.0.0",
      assets: []
    });

    await expect(checkGitHubReleaseForUpdates({
      appVersion: "5.0.0",
      assetPreferences: linuxReleaseAssetPreferences,
      userAgentVersion: "5.0.0"
    })).resolves.toMatchObject({
      message: "Hot Cross Buns is up to date.",
      updateAvailable: false
    });
  });
});

function stubLatestRelease(release: unknown) {
  const fetchMock = vi.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => release
  } as Response));

  vi.stubGlobal("fetch", fetchMock);

  return fetchMock;
}

function asset(name: string, browserDownloadUrl: string) {
  return {
    browser_download_url: browserDownloadUrl,
    name
  };
}
