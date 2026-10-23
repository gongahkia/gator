import { EventEmitter } from "node:events";

// A process owns a chat's execution. Changing the visible chat never moves its
// operation, approval token, queue or event stream into a different process.
export class Sessions extends EventEmitter {
  current;
  switching = false;
  issue = "";
  workers = new Set();
  #spawn;
  #seen = new WeakMap();
  #previous = new WeakMap();
  #unread = new Map();
  #transition = Promise.resolve();
  #stopping = false;
  #retiring = new WeakSet();
  constructor(spawn) { super(); this.#spawn = spawn; }
  get busy() { return [...this.workers].some(worker => !worker.closed && (worker.state?.running || (worker.state?.receipt_busy || worker.state?.catalog_busy) || worker.state?.pending_connection || worker.state?.pending_approval || worker.state?.external)); }
  get pending() { return [...this.workers].some(worker => !worker.closed && worker.state?.queued); }
  settled() { return this.#transition; }
  activity() {
    const rows = new Map(this.#unread);
    for (const update of this.current?.state?.update_conversations ?? []) {
      rows.set(update.id, { id: update.id, title: update.title, status: update.count === 1 ? "New update" : `${update.count} updates`, running: false });
    }
    for (const worker of this.workers) {
      if (this.#retiring.has(worker)) continue;
      const state = worker.state;
      if (!state?.conversation_id) continue;
      const status = worker.closed ? "Disconnected" : state.pending_approval ? "Needs review" : state.running ? state.cancelling ? "Stopping" : "Working" : state.queued ? "Paused" : rows.get(state.conversation_id)?.status;
      if (status) rows.set(state.conversation_id, { id: state.conversation_id, title: state.title, status, running: !!state.running });
    }
    return [...rows.values()];
  }
  #changed(worker) {
    const before = this.#previous.get(worker), state = worker.state;
    if (before?.running && !state?.running && state?.conversation_id && worker !== this.current) {
      const completion = [...(state.messages ?? [])].reverse().find(message => message.activity)?.activity?.state;
      this.#unread.set(state.conversation_id, { id: state.conversation_id, title: state.title, status: completion === "failed" ? "Needs attention" : completion === "stopped" ? "Stopped" : "Finished", running: false });
    }
    if (this.#unread.size > 64) this.#unread.delete(this.#unread.keys().next().value);
    this.#previous.set(worker, state);
    const request = state?.session_request;
    if (!this.#stopping && worker === this.current && request && this.#seen.get(worker) !== request.sequence) {
      this.#seen.set(worker, request.sequence);
      void this.open(request).catch(error => { this.issue = error.message; this.emit("change"); });
    }
    this.emit("change");
  }
  async #create(request) {
    if (this.workers.size >= 4) {
      const idle = [...this.workers].find(worker => worker !== this.current && (worker.closed || !worker.state?.running && !worker.state?.queued && !worker.state?.external && !worker.state?.pending_connection && !worker.state?.pending_approval && !worker.state?.receipt_busy && !worker.state?.catalog_busy));
      if (!idle) throw new Error("Four chats are open. Finish a task or review a paused queue before opening another.");
      this.#retiring.add(idle); await idle.stop(); this.workers.delete(idle);
    }
    const worker = this.#spawn(request);
    this.workers.add(worker);
    worker.on("state", () => this.#changed(worker));
    worker.on("closed", () => this.#changed(worker));
    try { await worker.ready(); return worker; }
    catch (error) { await worker.stop(); this.workers.delete(worker); throw error; }
  }
  open(request = {}) {
    const operation = this.#transition.then(async () => {
      if (this.#stopping) throw new Error("Gator is closing.");
      this.switching = true; this.issue = ""; this.emit("change");
      const previous = this.current;
      let next;
      try {
        if (previous && !previous.closed) await previous.focus(false);
        next = [...this.workers].find(worker => !worker.closed && request.conversation_id && (request.goal ? worker.state?.goal_id === request.conversation_id : worker.state?.conversation_id === request.conversation_id));
        if (request.view === "/connections") next ??= [...this.workers].find(worker => !worker.closed && worker.state?.pending_connection);
        next ??= await this.#create(request);
        await next.focus(true);
        this.current = next;
        this.#unread.delete(next.state?.conversation_id);
        if (request.view) next.send({ type: "navigate", text: request.view });
        else if (next.state?.panel) next.send({ type: "navigate", text: "/chat" });
      } catch (error) {
        if (previous && !previous.closed) await previous.focus(true).catch(() => {});
        this.current = previous; this.issue = error.message;
        throw error;
      } finally { this.switching = false; this.emit("change"); }
    });
    this.#transition = operation.catch(() => {});
    return operation;
  }
  assertSend(frame) {
    if (this.switching || this.#stopping) throw new Error("Wait for this conversation to open.");
    if (!this.current || this.current.closed) throw new Error("Reconnect to this chat before sending.");
    const profileAction = /^\/(data|recovery)(\s|$)/.test(frame.text ?? "") || ["/data", "/recovery"].includes(this.current.state?.destination);
    if (profileAction && (this.current.state?.receipt_busy || this.current.state?.catalog_busy)) throw new Error("Wait for the current local operation before changing retained data or recovery settings.");
    const otherBusy = [...this.workers].some(worker => worker !== this.current && !worker.closed && (worker.state?.running || worker.state?.queued || (worker.state?.receipt_busy || worker.state?.catalog_busy) || worker.state?.pending_connection || worker.state?.pending_approval || worker.state?.external));
    if (otherBusy && (/^\/(data|recovery)(\s|$)/.test(frame.text ?? "") || ["/data", "/recovery"].includes(this.current.state?.destination))) throw new Error("Finish work in the other chats before changing retained data or recovery settings.");
    const starting = frame.type === "upload_submit" || frame.type === "queue" && frame.queue?.action === "run" || frame.type === "submit" && !frame.text?.startsWith("/");
    if (starting && !this.current.state?.running && [...this.workers].filter(worker => !worker.closed && worker.state?.running).length >= 3) throw new Error("Three chats are working. Wait for one to finish, or stop it before sending this message.");
  }
  async resumeConnection(frame) {
    if (frame.type !== "navigate" || frame.text !== "/connections") return false;
    const owner = [...this.workers].find(worker => !worker.closed && worker.state?.pending_connection);
    if (!owner || owner === this.current) return false;
    await this.open({ view: "/connections" }); return true;
  }
  async prepareProfileAction(frame) {
    if (!/^\/(data|recovery)(\s|$)/.test(frame.text ?? "") && !["/data", "/recovery"].includes(this.current?.state?.destination)) return;
    this.assertSend(frame);
    // An intentional profile reset must not leave a cached private transcript
    // in another idle process. Running/queued sessions were rejected above.
    for (const worker of this.workers) if (worker !== this.current) {
      this.#retiring.add(worker); await worker.stop(); this.workers.delete(worker);
    }
    this.#unread.clear(); this.emit("change");
  }
  async restart() {
    await this.#transition;
    const current = this.current, id = current?.state?.conversation_id;
    if (current && !current.closed && (current.state?.running || (current.state?.receipt_busy || current.state?.catalog_busy) || current.state?.external || current.state?.pending_connection)) throw new Error("Finish this chat's current operation before reconnecting.");
    if (current) { this.#retiring.add(current); await current.stop(); this.workers.delete(current); }
    return this.open(id ? { conversation_id: id } : {});
  }
  async prepareUpdate() {
    await this.#transition;
    if (this.busy || this.pending) throw new Error("Finish work and queued messages in all chats before restarting.");
    for (const worker of this.workers) if (!worker.closed) {
      this.#retiring.add(worker);
      try { await worker.prepareUpdate(); } catch (error) { this.#retiring.delete(worker); throw error; }
    }
  }
  async stop() {
    this.#stopping = true;
    await this.#transition;
    await Promise.all([...this.workers].map(worker => worker.stop()));
  }
}
