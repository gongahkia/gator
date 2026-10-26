import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, rm, access } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { execFileSync } from "node:child_process";
import { terminalScript, windowsTerminalScript } from "../src/terminal.mjs";
import { powershellPath, windowsArguments, runtimeName, stateDirectory } from "../src/platform.mjs";

test("terminal handoff preserves literal arguments and records success or an early failure", { skip: process.platform === "win32" }, async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator terminal 'test-"));
  const script = join(directory, "action.command");
  const result = join(directory, "result file");
  const marker = join(directory, "must-not-exist");
  const literal = `a quote ' and $(touch ${marker})`;
  try {
    await writeFile(script, terminalScript({ program: "/usr/bin/printf", args: ["%s", literal], dir: directory }, result));
    const output = execFileSync("/bin/sh", [script], { encoding: "utf8" });
    assert.ok(output.startsWith(literal));
    assert.equal(await readFile(result, "utf8"), "0");
    await assert.rejects(access(marker));
    await writeFile(script, terminalScript({ program: "/usr/bin/true", args: [], dir: join(directory, "missing") }, result));
    assert.throws(() => execFileSync("/bin/sh", [script], { stdio: "ignore" }));
    assert.equal(await readFile(result, "utf8"), "1");
  } finally { await rm(directory, { recursive: true, force: true }); }
});

test("runtime naming and Windows native argument quoting are platform-aware", () => {
  assert.equal(runtimeName("win32"), "gator-runtime.exe");
  assert.equal(runtimeName("windows"), "gator-runtime.exe");
  assert.equal(runtimeName("darwin"), "gator-runtime");
  assert.equal(windowsArguments(["", 'a"b', "C:\\folder\\", "a b", "$(literal)"]), '"" "a\\"b" "C:\\folder\\\\" "a b" "$(literal)"');
  assert.equal(stateDirectory({ LOCALAPPDATA: join(tmpdir(), "local") }, "win32"), join(tmpdir(), "local", "Gator", "state"));
  assert.equal(stateDirectory({ GATOR_STATE_DIR: join(tmpdir(), "override"), LOCALAPPDATA: "ignored" }, "win32"), join(tmpdir(), "override"));
});

test("Windows handoff executes native argv literally and retains exit status", { skip: process.platform !== "win32" }, async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator ' Windows 🐊 "));
  const result = join(directory, "result");
  const recorded = join(directory, "argv.json");
  const marker = join(directory, "must-not-exist");
  const literal = ["", "C:\\a path\\", 'embedded "quotes"', "'single'", "🐊", "line\nbreak", `$(New-Item '${marker}')`, "%PATH% & | > ^ `"];
  const code = "require('node:fs').writeFileSync(process.argv[1], JSON.stringify({args: process.argv.slice(2), config: process.env.GATOR_CONFIG_DIR, internal: process.env.GATOR_INTERNAL_COMMANDS, ignored: process.env.GATOR_UNTRUSTED_HANDOFF_ENV}));process.exit(7)";
  const run = (job) => execFileSync(powershellPath(), ["-NoProfile", "-NonInteractive", "-EncodedCommand", Buffer.from(windowsTerminalScript(job, result), "utf16le").toString("base64")], { windowsHide: true, stdio: "ignore" });
  try {
    assert.throws(() => run({ program: process.execPath, args: ["-e", code, recorded, ...literal], dir: directory, env: [`GATOR_CONFIG_DIR=${directory}`, "GATOR_UNTRUSTED_HANDOFF_ENV=bad"] }));
    assert.equal(await readFile(result, "utf8"), "7");
    const data = JSON.parse(await readFile(recorded, "utf8"));
    assert.deepEqual(data.args, literal);
    assert.equal(data.config, directory);
    assert.equal(data.internal, "1");
    assert.equal(data.ignored, undefined);
    await assert.rejects(access(marker));
    assert.throws(() => run({ program: process.execPath, args: ["--version"], dir: join(directory, "missing") }));
    assert.equal(await readFile(result, "utf8"), "1");
  } finally { await rm(directory, { recursive: true, force: true }); }
});
