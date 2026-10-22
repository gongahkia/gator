import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const { releaseConfig } = createRequire(import.meta.url)("../scripts/release-config.cjs");

const common = { VERSION: "v1.2.3", DESKTOP_ARCH: "arm64" };
test("signed release configuration fails before packaging when setup is missing", () => {
  assert.throws(() => releaseConfig({ ...common, GOOS: "darwin" }), /CSC_LINK/);
  assert.throws(() => releaseConfig({ ...common, GOOS: "windows" }), /GATOR_WINDOWS_PUBLISHER/);
  assert.throws(() => releaseConfig({ ...common, GOOS: "linux", VERSION: "0.0.0-dev" }), /stable/);
  assert.throws(() => releaseConfig({ ...common, GOOS: "other" }), /supported/);
});
test("macOS releases require the real identity and notarization and include the Go runtime", () => {
  const config = releaseConfig({ ...common, GOOS: "darwin", CSC_LINK: "fixture", CSC_KEY_PASSWORD: "fixture", GATOR_MAC_IDENTITY: "Developer ID Application: Fixture", APPLE_ID: "fixture", APPLE_APP_SPECIFIC_PASSWORD: "fixture", APPLE_TEAM_ID: "fixture" });
  assert.equal(config.forceCodeSigning, true); assert.equal(config.mac.notarize, true); assert.equal(config.mac.hardenedRuntime, true);
  assert.deepEqual(config.mac.binaries, ["Contents/Resources/runtime/gator-runtime"]);
  assert.equal(config.mac.identity, "Developer ID Application: Fixture");
  assert.equal(config.publish[0].channel, "stable-arm64"); assert.equal(config.publish[0].releaseType, "draft");
});
test("Windows releases bind publisher verification and sign the private runtime", async () => {
  const config = releaseConfig({ ...common, GOOS: "windows", WIN_CSC_LINK: "fixture", WIN_CSC_KEY_PASSWORD: "fixture", GATOR_WINDOWS_PUBLISHER: "Fixture Publisher" });
  assert.equal(config.win.verifyUpdateCodeSignature, true); assert.equal(config.win.signtoolOptions.publisherName, "Fixture Publisher");
  assert.equal(config.forceCodeSigning, true); assert.ok(config.win.target.includes("nsis"));
  const signed = [];
  await config.afterPack({ electronPlatformName: "win32", appOutDir: "output", packager: { signIf: async (path) => signed.push(path.replaceAll("\\", "/")) } });
  assert.deepEqual(signed, ["output/resources/runtime/gator-runtime.exe"]);
});
test("architectures get distinct update feeds and Linux keeps manual update behavior", () => {
  for (const arch of ["arm64", "x64"]) {
    const config = releaseConfig({ ...common, GOOS: "linux", DESKTOP_ARCH: arch });
    assert.equal(config.publish[0].channel, `stable-${arch}`);
    assert.equal(config.generateUpdatesFilesForAllChannels, false);
    assert.equal(config.extraMetadata.gatorDistribution.signed, false);
  }
});

test("Windows cloud signing uses a separate CI identity and requires its full configuration", () => {
  const env = { ...common, GOOS: "windows", GATOR_WINDOWS_PUBLISHER: "Fixture Publisher", GATOR_WINDOWS_SIGNING: "azure" };
  assert.throws(() => releaseConfig(env), /AZURE_TENANT_ID/);
  const config = releaseConfig({ ...env, AZURE_TENANT_ID: "fixture", AZURE_CLIENT_ID: "fixture", AZURE_CLIENT_SECRET: "fixture", GATOR_SIGNING_ENDPOINT: "https://fixture.example", GATOR_SIGNING_ACCOUNT: "fixture", GATOR_SIGNING_PROFILE: "fixture" });
  assert.equal(config.win.signtoolOptions, undefined); assert.equal(config.win.azureSignOptions.publisherName, "Fixture Publisher");
});
