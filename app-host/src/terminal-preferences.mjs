import { randomUUID } from "node:crypto";
import { FrontendPreferences, frontendDefaults, validFrontendPreferences } from "./preferences.mjs";

const complete = value => validFrontendPreferences(value) && Object.keys(value).length === 4 && value.workDetailsSections && Object.keys(value.workDetailsSections).length === 4;

// The old file/Chromium data is only an import source. Active reads and writes
// use the same Go settings transaction as conversational application controls.
export class TerminalPreferences {
  #pending = Promise.resolve();
  constructor(application) { this.application = application; }
  read() { return this.#queue(); }
  initializeDefaults() { return this.#queue(undefined, true); }
  configure(patch) {
    if (!validFrontendPreferences(patch)) return Promise.reject(new Error("Invalid terminal preferences."));
    return this.#queue(patch);
  }
  flush() { return this.#pending; }
  #queue(patch, defaults = false) { const operation = this.#pending.then(() => this.#update(patch, defaults)); this.#pending = operation.catch(() => {}); return operation; }
  #state(owner) {
    const state = owner.state?.terminal_preferences;
    if (!state || typeof state.initialized !== "boolean" || !/^[a-f0-9]{64}$/.test(state.id) || !complete(state.values)) throw new Error("This Gator runtime cannot load terminal preferences. Rebuild or reconnect before changing them.");
    if (state.issue) throw new Error(state.issue);
    return state;
  }
  async #apply(owner, action) {
    const request_id = randomUUID();
    await owner.apply({ type: "terminal_preferences", terminal_preferences: { id: this.#state(owner).id, request_id, ...action } });
    const result = owner.state?.terminal_preferences_result;
    if (result?.request_id !== request_id) throw new Error("Terminal preference changes were not confirmed. Inspect them before retrying.");
    if (result.error) throw new Error(result.error);
    if (!complete(result.values)) throw new Error("The saved terminal preferences could not be read.");
    return result.values;
  }
  async #update(patch, defaults) {
    this.application.mutable();
    const owner = this.application.backend;
    await owner.refreshPreferences();
    if (!this.#state(owner).initialized) {
      let legacy;
      try { legacy = defaults ? frontendDefaults() : await new FrontendPreferences(this.application.userData).read(); }
      catch (cause) { throw new Error("Existing terminal settings could not be imported. Retry or choose default display settings in /terminal-settings. The original settings file is kept.", { cause }); }
      await this.#apply(owner, { action: "migrate", legacy });
    }
    if (!patch || !Object.keys(patch).length) return this.#state(owner).values;
    return this.#apply(owner, { action: "configure", patch });
  }
}
