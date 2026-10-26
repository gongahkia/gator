import { test } from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { Updates, updateBlockReason } from "../src/updates.mjs";

class FixtureUpdater extends EventEmitter {
  calls = [];
  available = true;
  async checkForUpdates() { this.calls.push("check"); return { isUpdateAvailable: this.available, updateInfo: { version: "1.1.0" } }; }
  async downloadUpdate() { this.calls.push("download"); this.emit("download-progress", { percent: 58.8 }); return ["verified-installer"]; }
  quitAndInstall(...args) { this.calls.push(["install", ...args]); }
}
function setup(options = {}) {
  const updater = new FixtureUpdater();
  const updates = new Updates({ updater, supported: true, version: "1.0.0", arch: "arm64", blocked: () => "", confirm: async () => true, prepare: async () => async () => {}, ...options });
  return { updater, updates };
}
async function ready(updates) { await updates.request({ action: "check" }); await updates.request({ action: "download" }); }

test("updates require explicit actions and cannot change feed, channel or install on ordinary quit", async () => {
  const { updates, updater } = setup();
  assert.deepEqual(updater.calls, []);
  assert.equal(updater.autoDownload, false); assert.equal(updater.autoInstallOnAppQuit, false);
  assert.equal(updater.allowPrerelease, false); assert.equal(updater.allowDowngrade, false);
  assert.equal(updater.disableWebInstaller, true); assert.equal(updater.channel, "stable-arm64");
  for (const action of [{ action: "install" }, { action: "download" }, { action: "check", url: "https://other.example" }, { action: "arbitrary" }]) await assert.rejects(updates.request(action));
  assert.deepEqual(updater.calls, []);
});

test("manual builds neither configure nor contact an updater", async () => {
  const { updates, updater } = setup({ supported: false, reason: "Manual build" });
  assert.equal(updates.snapshot().phase, "manual");
  await assert.rejects(updates.request({ action: "check" }), /Manual build/);
  assert.deepEqual(updater.calls, []); assert.equal(updater.autoDownload, undefined);
});

test("availability, actual download progress and ready state stay separate", async () => {
  const { updates, updater } = setup();
  const phases = []; updates.on("change", (state) => phases.push([state.phase, state.percent]));
  await updates.request({ action: "check" });
  assert.equal(updates.snapshot().phase, "available"); assert.deepEqual(updater.calls, ["check"]);
  await updates.request({ action: "download" });
  assert.ok(phases.some(([phase, percent]) => phase === "downloading" && percent === 58));
  assert.equal(updates.snapshot().phase, "ready"); assert.equal(updates.snapshot().version, "1.1.0");
  assert.deepEqual(updater.calls, ["check", "download"]);
  const current = setup(); current.updater.available = false;
  await current.updates.request({ action: "check" }); assert.equal(current.updates.snapshot().phase, "current");
});

test("restart checks current work both before and after the user's review", async () => {
  let blocked = "", confirmed = 0, prepared = 0;
  const { updates, updater } = setup({ blocked: () => blocked, confirm: async () => { confirmed++; blocked = "Work just started"; return true; }, prepare: async () => { prepared++; } });
  await ready(updates); blocked = "Existing work";
  await updates.request({ action: "install" }); assert.equal(confirmed, 0);
  blocked = ""; await updates.request({ action: "install" });
  assert.equal(confirmed, 1); assert.equal(prepared, 0); assert.equal(updates.snapshot().phase, "ready");
  assert.deepEqual(updater.calls, ["check", "download"]);
});

test("declining restart and failed preparation never invoke the installer", async () => {
  let prepare = 0;
  const declined = setup({ confirm: async () => false, prepare: async () => { prepare++; } });
  await ready(declined.updates); await declined.updates.request({ action: "install" }); assert.equal(prepare, 0);
  const failed = setup({ prepare: async () => { throw new Error("A supervisor is running"); } });
  await ready(failed.updates); await failed.updates.request({ action: "install" });
  assert.equal(failed.updates.snapshot().phase, "ready"); assert.match(failed.updates.snapshot().reason, /Recovery/);
  assert.deepEqual(failed.updater.calls, ["check", "download"]);
});

test("an installer error event restores the session without resubmitting work", async () => {
  let restored = 0;
  const { updates, updater } = setup({ prepare: async () => async () => { restored++; } });
  updater.quitAndInstall = () => updater.emit("error", new Error("spawn failed"));
  await ready(updates); await updates.request({ action: "install" });
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(restored, 1); assert.equal(updates.snapshot().phase, "ready");
  assert.match(updates.snapshot().reason, /without resubmitting/);
});

test("only a reviewed prepared install invokes quitAndInstall", async () => {
  let prepared = 0;
  const { updates, updater } = setup({ prepare: async () => { prepared++; return async () => {}; } });
  await ready(updates); await updates.request({ action: "install" });
  assert.equal(prepared, 1); assert.deepEqual(updater.calls.at(-1), ["install", false, true]);
});

test("download failure never becomes an installable update", async () => {
  const { updates, updater } = setup();
  updater.downloadUpdate = async () => { throw new Error("network failed"); };
  await ready(updates); assert.equal(updates.snapshot().phase, "error");
  await assert.rejects(updates.request({ action: "install" }));
});

test("host restart blockers cover live work, queue, approvals, external steps and editable forms", () => {
  const idle = { connection: "connected", state: {} };
  assert.equal(updateBlockReason(idle), "");
  for (const state of [{ receipt_busy: true }, { running: true }, { cancelling: true }, { pending_approval: { id: 1 } }, { queued: 1 }, { queue: [{}] }, { external: {} }, { panel: { kind: "confirmation" } }, { panel: { rows: [{ input: {} }] } }]) assert.ok(updateBlockReason({ ...idle, state }));
  assert.ok(updateBlockReason({ ...idle, terminalBusy: true }));
  assert.ok(updateBlockReason({ ...idle, connection: "disconnected" }));
});
