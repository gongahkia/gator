import { readFile, readdir } from "node:fs/promises";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import { join, basename } from "node:path";

const require = createRequire(import.meta.url);
const { load } = require("js-yaml");
const os = process.env.GOOS, arch = process.env.DESKTOP_ARCH, version = process.env.VERSION?.replace(/^v/, "");
if (!/^(darwin|windows|linux)$/.test(os || "") || !/^(arm64|x64)$/.test(arch || "") || !/^\d+\.\d+\.\d+$/.test(version || "")) throw new Error("Invalid release verification target.");
const run = (program, args) => {
  const result = spawnSync(program, args, { encoding: "utf8", windowsHide: true });
  if (result.error || result.status !== 0) throw new Error(`${program} verification failed: ${result.error?.message || result.stderr || result.stdout}`);
  return result.stdout + result.stderr;
};
const root = "release";
if (os === "darwin") {
  const bundle = join(root, arch === "arm64" ? "mac-arm64" : "mac", "Gator.app");
  for (const path of [bundle, join(bundle, "Contents/Resources/runtime/gator-runtime")]) {
    run("codesign", ["--verify", "--deep", "--strict", path]);
    const identity = run("codesign", ["--display", "--verbose=4", path]);
    if (!identity.includes("Authority=Developer ID Application:") || !identity.includes(`TeamIdentifier=${process.env.APPLE_TEAM_ID}\n`)) throw new Error("The signed application does not match the configured Apple team.");
  }
  run("xcrun", ["stapler", "validate", bundle]);
  run("spctl", ["--assess", "--type", "execute", bundle]);
}
if (os === "windows") {
  const unpacked = join(root, arch === "arm64" ? "win-arm64-unpacked" : "win-unpacked");
  const installers = (await readdir(root)).filter((file) => file.startsWith(`Gator-${version}-`) && file.includes(`-${arch}.`) && file.endsWith(".exe"));
  if (installers.length !== 1) throw new Error("Expected exactly one signed NSIS installer for this target.");
  const files = [join(unpacked, "Gator.exe"), join(unpacked, "resources/runtime/gator-runtime.exe"), ...installers.map((file) => join(root, file))];
  // Payload is JSON in an environment variable, never PowerShell source.
  const script = "$ErrorActionPreference = 'Stop'; $paths = ConvertFrom-Json $env:GATOR_VERIFY_FILES; foreach ($path in $paths) { $s = Get-AuthenticodeSignature -LiteralPath $path; if ($s.Status -ne 'Valid' -or $s.SignerCertificate.GetNameInfo([System.Security.Cryptography.X509Certificates.X509NameType]::SimpleName, $false) -ne $env:GATOR_WINDOWS_PUBLISHER) { throw 'Signature or publisher verification failed' } }";
  process.env.GATOR_VERIFY_FILES = JSON.stringify(files);
  run(join(process.env.SystemRoot, "System32/WindowsPowerShell/v1.0/powershell.exe"), ["-NoProfile", "-NonInteractive", "-Command", script]);
}

const suffix = os === "darwin" ? "-mac" : os === "linux" ? `-linux${arch === "arm64" ? "-arm64" : ""}` : "";
const metadata = load(await readFile(join(root, `stable-${arch}${suffix}.yml`), "utf8"));
if (metadata.version !== version || !metadata.files?.length) throw new Error("Missing matching update metadata.");
for (const file of metadata.files) {
  if (typeof file.url !== "string" || basename(file.url) !== file.url || !file.url.startsWith(`Gator-${version}-`) || !file.url.includes(`-${arch}.`)) throw new Error("Update metadata references the wrong release/architecture.");
  const bytes = await readFile(join(root, file.url));
  if (createHash("sha512").update(bytes).digest("base64") !== file.sha512) throw new Error("Update metadata checksum does not match its artifact.");
}
console.log(`Verified ${os}/${arch} release signatures where applicable, update target and artifact hashes.`);
