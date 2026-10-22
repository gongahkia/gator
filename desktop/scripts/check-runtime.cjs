// Packaging must never ship a runtime left over from another target build.
const { readFile } = require("node:fs/promises");
const { join } = require("node:path");

module.exports = async function checkRuntime(context) {
  const platform = context.electronPlatformName;
  const name = platform === "win32" ? "gator-runtime.exe" : "gator-runtime";
  const bytes = await readFile(join(context.packager.projectDir, "runtime", name));
  const architecture = { 1: "amd64", 3: "arm64" }[context.arch];
  if (!architecture) throw new Error("Gator packages only x64 and ARM64 runtimes.");
  let actual;
  if (platform === "win32" && bytes.toString("ascii", 0, 2) === "MZ") {
    const pe = bytes.readUInt32LE(0x3c);
    if (bytes.toString("ascii", pe, pe + 4) === "PE\0\0") actual = { 0x8664: "amd64", 0xaa64: "arm64" }[bytes.readUInt16LE(pe + 4)];
  } else if (platform === "linux" && bytes.toString("ascii", 1, 4) === "ELF") {
    actual = { 62: "amd64", 183: "arm64" }[bytes.readUInt16LE(18)];
  } else if (platform === "darwin" && bytes.readUInt32LE(0) === 0xfeedfacf) {
    actual = { 0x1000007: "amd64", 0x100000c: "arm64" }[bytes.readUInt32LE(4)];
  }
  if (actual !== architecture) throw new Error(`The bundled runtime does not match ${platform}/${architecture}. Rebuild it with matching GOOS and GOARCH.`);
};
