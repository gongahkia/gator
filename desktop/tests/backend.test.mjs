import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { once } from "node:events";
import { Backend } from "../electron/backend.mjs";

test("host streams Go state and closes its child on exit", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-bridge-test-"));
  const executable = join(directory, "fixture");
  await writeFile(executable, `#!/usr/bin/env node\nconst readline = require('node:readline');\nconst send = (text) => process.stdout.write(JSON.stringify({ version: 1, type: 'state', state: { title: text, messages: [] } }) + '\\n');\nsend('🐊 connected');\nreadline.createInterface({ input: process.stdin }).on('line', line => { const frame = JSON.parse(line); if (frame.type === 'exit') process.exit(0); send(frame.text); });\n`, { mode: 0o700 });
  const backend = new Backend(executable, directory);
  try {
    const [initial] = await once(backend, "state");
    assert.equal(initial.title, "🐊 connected");
    const next = once(backend, "state");
    backend.send({ type: "submit", text: "next turn" });
    assert.equal((await next)[0].title, "next turn");
    assert.throws(() => backend.send({ type: "external_done" }));
    await backend.stop();
    assert.equal(backend.closed, true);
    assert.throws(() => backend.send({ type: "submit", text: "too late" }));
  } finally { await backend.stop(); await rm(directory, { recursive: true, force: true }); }
});
