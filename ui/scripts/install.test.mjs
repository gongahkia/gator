import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, readFile, readdir, lstat, rm, symlink } from "node:fs/promises";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execute = promisify(execFile), script = resolve("ui/scripts/install.mjs");
async function fixture() {
  const directory = await mkdtemp(join(tmpdir(), "gator-install-atomic-")), artifact = join(directory, "artifact"), prefix = join(directory, "prefix");
  for (const path of ["bin", "runtime", "ui/dist", "app-host"]) await mkdir(join(artifact, path), { recursive: true });
  await writeFile(join(artifact, "gator-distribution.json"), JSON.stringify({ product: "gator-terminal", version: 1, platform: process.platform, arch: process.arch }));
  for (const path of ["bin/gator", `runtime/${process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"}`, "ui/dist/index.js", "ui/package.json", "app-host/package.json"]) await writeFile(join(artifact, path), "fixture contents\n");
  return { directory, artifact, prefix, root: join(prefix, "lib/gator-terminal"), launcher: join(prefix, "bin", process.platform === "win32" ? "gator.cmd" : "gator") };
}
const run = (...args) => execute(process.execPath, [script, ...args], { timeout: 15000 });

test("concurrent installers publish one complete package and preserve existing installations", async () => {
  const f = await fixture();
  try {
    const outcomes = await Promise.allSettled([run(f.artifact, f.prefix), run(f.artifact, f.prefix)]);
    assert.equal(outcomes.filter(outcome => outcome.status === "fulfilled").length, 1);
    assert.equal(await readFile(join(f.root, "ui/dist/index.js"), "utf8"), "fixture contents\n");
    const manifest = await readFile(join(f.root, "installation.json"), "utf8");
    await assert.rejects(run(f.artifact, f.prefix));
    assert.equal(await readFile(join(f.root, "installation.json"), "utf8"), manifest);
    assert.deepEqual((await readdir(join(f.prefix, "lib"))).filter(name => name.startsWith(".gator-terminal-install-")), []);
    await run("--recover", f.prefix);
  } finally { await rm(f.directory, { recursive: true, force: true }); }
});

test("an interrupted launcher setup can recover without overwriting a conflict", async () => {
  const f = await fixture();
  try {
    await run(f.artifact, f.prefix);
    // Reproduce the persisted state after package publication but before the
    // launcher was created. An unrelated object must still block recovery.
    await rm(f.launcher); await rm(join(f.prefix, "bin"), { recursive: true });
    await writeFile(join(f.prefix, "bin"), "unrelated file");
    await assert.rejects(run("--recover", f.prefix));
    assert.equal(await readFile(join(f.prefix, "bin"), "utf8"), "unrelated file");
    assert.equal(await readFile(join(f.root, "ui/dist/index.js"), "utf8"), "fixture contents\n");
    await rm(join(f.prefix, "bin")); await run("--recover", f.prefix);
    assert.ok(await lstat(f.launcher));
    await rm(f.launcher); await writeFile(f.launcher, "another product");
    await assert.rejects(run("--recover", f.prefix), /belongs to something else/);
    assert.equal(await readFile(f.launcher, "utf8"), "another product");
    await rm(f.launcher);
    const path = join(f.root, "installation.json"), manifest = JSON.parse(await readFile(path, "utf8"));
    await writeFile(path, JSON.stringify({ ...manifest, prefix: "wrong-owner" }));
    await assert.rejects(run("--recover", f.prefix), /ownership/);
    await assert.rejects(lstat(f.launcher), { code: "ENOENT" });
  } finally { await rm(f.directory, { recursive: true, force: true }); }
});

test("recovery refuses an incomplete package and installation rejects recursive destinations", async () => {
  const f = await fixture();
  try {
    const nested = join(f.artifact, "new-prefix");
    await assert.rejects(run(f.artifact, nested), /outside the source artifact/);
    await assert.rejects(lstat(nested), { code: "ENOENT" });
    await run(f.artifact, f.prefix); await rm(f.launcher); await rm(join(f.root, "ui/dist/index.js"));
    await assert.rejects(run("--recover", f.prefix));
    await assert.rejects(lstat(f.launcher), { code: "ENOENT" });
  } finally { await rm(f.directory, { recursive: true, force: true }); }
});

test("copy failure cleans staging without publishing a partial installation", async t => {
  const f = await fixture();
  try {
    try { await symlink(join(f.directory, "missing"), join(f.artifact, "broken-copy")); }
    catch (error) { if (process.platform === "win32") return t.skip(`symlinks unavailable: ${error.code}`); throw error; }
    await assert.rejects(run(f.artifact, f.prefix));
    await assert.rejects(lstat(f.root), { code: "ENOENT" });
    await assert.rejects(lstat(f.launcher), { code: "ENOENT" });
    assert.deepEqual(await readdir(join(f.prefix, "lib")), []);
    await rm(join(f.artifact, "broken-copy"));
    await run(f.artifact, f.prefix);
  } finally { await rm(f.directory, { recursive: true, force: true }); }
});
