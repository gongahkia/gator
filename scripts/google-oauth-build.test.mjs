import { test } from "node:test";
import assert from "node:assert/strict";
import { googleOAuthFlags } from "./google-oauth-build.mjs";

test("distribution OAuth is optional, paired and cannot inject linker flags", () => {
  assert.deepEqual(googleOAuthFlags({}), []);
  const env = { GATOR_GOOGLE_DESKTOP_CLIENT_ID: "123-fixture.apps.googleusercontent.com", GATOR_GOOGLE_DESKTOP_CLIENT_SECRET: "fixture-only" };
  assert.equal(googleOAuthFlags(env).length, 2);
  for (const patch of [{ GATOR_GOOGLE_DESKTOP_CLIENT_SECRET: "" }, { GATOR_GOOGLE_DESKTOP_CLIENT_ID: "" }, { GATOR_GOOGLE_DESKTOP_CLIENT_SECRET: "x -X main.version=bad" }, { GATOR_GOOGLE_DESKTOP_CLIENT_ID: "https://example.com" }]) {
    assert.throws(() => googleOAuthFlags({ ...env, ...patch }), /Configure both/);
  }
});
