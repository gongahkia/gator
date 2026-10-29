import assert from "node:assert/strict";
import { mkdtemp, mkdir, rm, lstat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { acquireUpdateGuard } from "../../app-host/src/update-guard.mjs";
const artifact = resolve(process.argv[2]);
const directory = await mkdtemp(join(tmpdir(), "gator-install-check-"));
const prefix = join(directory, "prefix"), workspace = join(directory, "workspace");
await mkdir(workspace);
const env = { ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_TERMINAL_USER_DATA: join(directory, "app") };
function run(command, args, success = true) {
  const result = spawnSync(command, args, { env, cwd: workspace, encoding: "utf8" });
  if (success) assert.equal(result.status, 0, result.stderr || result.error?.message);
  else assert.notEqual(result.status, 0);
  return result;
}
try {
  run(process.execPath, [resolve("ui/scripts/install.mjs"), artifact, prefix]);
  const launcher = join(prefix, "lib/gator-terminal/bin/gator");
  assert.match(run("bun", [launcher, "--version"]).stdout, /gator terminal/);
  run("bun", [launcher, "note", "add", "--title", "Preserved install fixture", "--body", "Private data survives removal."]);
  const publicLauncher = join(prefix, "bin", process.platform === "win32" ? "gator.cmd" : "gator");
  await rm(publicLauncher);
  run("bun", [join(prefix, "lib/gator-terminal/ui/scripts/install.mjs"), "--recover", prefix]);
  assert.ok(await lstat(publicLauncher));
  assert.match(run("bun", [launcher, "note", "list"]).stdout, /Preserved install fixture/);
  const binary = join(prefix, "lib/gator-terminal/runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime");
  const guard = await acquireUpdateGuard(binary, workspace, env);
  const uninstall = resolve("ui/scripts/uninstall.mjs");
  try { assert.match(run("bun", [uninstall, prefix], false).stderr, /other work is using this profile/); }
  finally { await guard.release(); }
  assert.ok(await lstat(launcher));
  run("bun", [uninstall, prefix]);
  await assert.rejects(lstat(launcher), { code: "ENOENT" });
  assert.match(run("bun", [join(artifact, "bin/gator"), "note", "list"]).stdout, /Preserved install fixture/);
  console.log("PASS: independent install/help/version, launcher recovery, scripted commands, canonical active-profile uninstall guard, owned uninstall and retained user data.");
} finally { await rm(directory, { recursive: true, force: true }); }
