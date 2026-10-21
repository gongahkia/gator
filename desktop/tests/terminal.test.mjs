import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, rm, access } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { execFileSync } from "node:child_process";
import { terminalScript } from "../electron/terminal.mjs";

test("terminal handoff preserves literal arguments and records success or an early failure", async () => {
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
