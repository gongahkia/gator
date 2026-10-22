const base = require("../package.json").build;
const { join } = require("node:path");

function required(env, names) {
  const missing = names.filter((name) => !env[name]?.trim());
  if (missing.length) throw new Error(`Release setup is incomplete: ${missing.join(", ")}. See docs/DESKTOP-RELEASES.md.`);
}

function releaseConfig(env = process.env) {
  const os = env.GOOS;
  const arch = env.DESKTOP_ARCH;
  if (!["darwin", "windows", "linux"].includes(os) || !["arm64", "x64"].includes(arch)) throw new Error("Set GOOS and DESKTOP_ARCH to one supported release target.");
  if (!/^v?\d+\.\d+\.\d+$/.test(env.VERSION || "")) throw new Error("Signed releases require a stable vMAJOR.MINOR.PATCH tag; preview builds use the development configuration.");
  const config = {
    ...base,
    forceCodeSigning: os !== "linux",
    detectUpdateChannel: false,
    generateUpdatesFilesForAllChannels: false,
    extraMetadata: { gatorDistribution: { released: true, signed: os !== "linux", channel: `stable-${arch}` } },
    publish: [{ provider: "github", owner: "gongahkia", repo: "gator", channel: `stable-${arch}`, releaseType: "draft" }],
  };
  if (os === "darwin") {
    required(env, ["CSC_LINK", "CSC_KEY_PASSWORD", "GATOR_MAC_IDENTITY", "APPLE_ID", "APPLE_APP_SPECIFIC_PASSWORD", "APPLE_TEAM_ID"]);
    if (!env.GATOR_MAC_IDENTITY.startsWith("Developer ID Application:")) throw new Error("Use a Developer ID Application identity for direct macOS distribution.");
    config.mac = { ...base.mac, identity: env.GATOR_MAC_IDENTITY, hardenedRuntime: true, notarize: true,
      entitlements: "build/entitlements.mac.plist", entitlementsInherit: "build/entitlements.mac.plist",
      binaries: ["Contents/Resources/runtime/gator-runtime"],
    };
  }
  if (os === "windows") {
    required(env, ["GATOR_WINDOWS_PUBLISHER"]);
    config.win = { ...base.win, verifyUpdateCodeSignature: true };
    if (env.GATOR_WINDOWS_SIGNING === "azure") {
      required(env, ["AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "GATOR_SIGNING_ENDPOINT", "GATOR_SIGNING_ACCOUNT", "GATOR_SIGNING_PROFILE"]);
      config.win.azureSignOptions = { publisherName: env.GATOR_WINDOWS_PUBLISHER, endpoint: env.GATOR_SIGNING_ENDPOINT, codeSigningAccountName: env.GATOR_SIGNING_ACCOUNT, certificateProfileName: env.GATOR_SIGNING_PROFILE };
    } else {
      required(env, ["WIN_CSC_LINK", "WIN_CSC_KEY_PASSWORD"]);
      config.win.signtoolOptions = { publisherName: env.GATOR_WINDOWS_PUBLISHER, signingHashAlgorithms: ["sha256"] };
    }
    // electron-builder signs the main executable, installer and unpacked app
    // files, but extraResources/runtime is outside its default signing walk.
    config.afterPack = async (context) => {
      if (context.electronPlatformName !== "win32") return;
      await context.packager.signIf(join(context.appOutDir, "resources", "runtime", "gator-runtime.exe"));
    };
  }
  return config;
}
module.exports = { releaseConfig };
if (require.main === module) { releaseConfig(); process.stdout.write("Release target and required signing settings are configured.\n"); }
