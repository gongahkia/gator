import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowDown, ArrowUp, CornerDownLeft, ListOrdered, Pencil, X } from "lucide-react";
import type { BackendState, QueueAction, QueuedMessage } from "../../ui/src/protocol";
import type { DesktopFrame } from "./bridge";

export type QueueChange = Omit<QueueAction, "request_id">;

// IPC delivery alone is not acceptance: a queued message may have started, or
// the active run may have ended, before Go receives the action.
export function useQueueAction(state: BackendState | undefined, send: (frame: DesktopFrame) => Promise<void>, connectionError?: string) {
  const [busy, setBusy] = useState(false);
  const pending = useRef<{ id: string; resolve(): void; reject(error: Error): void } | null>(null);
  const finish = useCallback((error?: string) => {
    const request = pending.current;
    if (!request) return;
    pending.current = null;
    setBusy(false);
    if (error) request.reject(new Error(error)); else request.resolve();
  }, []);
  useEffect(() => {
    if (state?.queue_result?.request_id === pending.current?.id) finish(state?.queue_result?.error);
  }, [state?.queue_result, finish]);
  useEffect(() => { if (connectionError) finish(connectionError); }, [connectionError, finish]);
  useEffect(() => () => { pending.current?.reject(new Error("Conversation closed.")); pending.current = null; }, []);
  const act = useCallback((change: QueueChange) => new Promise<void>((resolve, reject) => {
    if (pending.current) { reject(new Error("Wait for the previous queue change.")); return; }
    if (change.action === "edit" || change.action === "guide") {
      const limit = change.action === "guide" ? 64 : 128;
      if (!change.text?.trim() || new TextEncoder().encode(change.text).length > limit * 1024) {
        reject(new Error(`Enter a message of up to ${limit} KB.`)); return;
      }
    }
    const id = crypto.randomUUID();
    pending.current = { id, resolve, reject };
    setBusy(true);
    void send({ type: "queue", queue: { ...change, request_id: id } }).catch((error) => finish(String(error)));
  }), [send, finish]);
  return { act, busy };
}

export function MessageQueue({ items, running, busy, act, onUseDraft, onEditingChange }: {
  items: QueuedMessage[]; running: boolean; busy: boolean; act(change: QueueChange): Promise<void>; onUseDraft(text: string): void; onEditingChange(editing: boolean): void;
}) {
  const [editing, setEditing] = useState<{ id: string; text: string } | null>(null);
  const [error, setError] = useState("");
  const editor = useRef<HTMLTextAreaElement>(null);
  const region = useRef<HTMLElement>(null);
  useEffect(() => { onEditingChange(!!editing); return () => onEditingChange(false); }, [!!editing, onEditingChange]);
  useEffect(() => { if (editing) editor.current?.focus(); }, [editing?.id]);
  const change = async (action: QueueChange) => {
    setError("");
    try {
      await act(action);
      if (action.action === "edit") setEditing(null);
      // Restore keyboard focus after a row or editor disappears.
      requestAnimationFrame(() => { if (document.activeElement === document.body) region.current?.focus(); });
    } catch (err) { setError(err instanceof Error ? err.message : String(err)); }
  };
  if (!items.length && !editing) return null;
  const stale = editing && !items.some((item) => item.id === editing.id);
  return <section className="message-queue" aria-label="Queued messages" tabIndex={-1} ref={region}>
    <div className="queue-heading"><ListOrdered size={14} /><span role="status">{items.length ? `${items.length} queued` : "Unsent edits"}</span>{!running && items.length > 0 && <span className="queue-paused">Paused</span>}</div>
    <ol className="queue-list">{items.map((item, index) => <li className="queued-message" key={item.id} data-editing={editing?.id === item.id}>
      <span className="queue-position" aria-hidden="true">{index + 1}</span><p className="queue-preview">{item.text}</p>
      <div className="queue-actions">
        {items.length > 1 && <><button className="icon-button" aria-label={`Move queued message ${index + 1} up`} title="Move up" disabled={busy || index === 0} onClick={() => void change({ id: item.id, action: "earlier" })}><ArrowUp size={14} /></button><button className="icon-button" aria-label={`Move queued message ${index + 1} down`} title="Move down" disabled={busy || index === items.length - 1} onClick={() => void change({ id: item.id, action: "later" })}><ArrowDown size={14} /></button></>}
        {!running && <button className="icon-button" aria-label={`Send queued message ${index + 1} now`} title="Send now" disabled={busy} onClick={() => void change({ id: item.id, action: "run" })}><CornerDownLeft size={15} /></button>}
        <button className="icon-button" aria-label={`Edit queued message ${index + 1}`} title="Edit message" disabled={busy || !!editing} onClick={() => { setError(""); setEditing({ ...item }); }}><Pencil size={14} /></button>
        <button className="icon-button" aria-label={`Remove queued message ${index + 1}`} title="Remove message" disabled={busy} onClick={() => void change({ id: item.id, action: "remove" })}><X size={15} /></button>
      </div>
    </li>)}</ol>
    {editing && <div className="queue-editor"><textarea ref={editor} aria-label="Edit queued message" value={editing.text} maxLength={128 * 1024} disabled={busy} onChange={(event) => setEditing({ ...editing, text: event.target.value })} onKeyDown={(event) => {
      if (event.key === "Escape") { event.stopPropagation(); if (!busy) { setEditing(null); region.current?.focus(); } }
      if (event.key === "Enter" && (event.metaKey || event.ctrlKey) && !event.nativeEvent.isComposing && !stale && editing.text.trim()) { event.preventDefault(); void change({ id: editing.id, action: "edit", text: editing.text }); }
    }} rows={3} />
      {stale && <p className="queue-edit-notice">This message has left the queue. Your edits are kept here.</p>}
      <div className="queue-edit-actions"><button className="small-button" disabled={busy} onClick={() => { setEditing(null); setError(""); region.current?.focus(); }}>Cancel</button>{stale ? <button className="small-button" disabled={busy || !editing.text.trim()} onClick={() => { onUseDraft(editing.text); setEditing(null); setError(""); }}>Keep as new draft</button> : <button className="button" disabled={busy || !editing.text.trim()} onClick={() => void change({ id: editing.id, action: "edit", text: editing.text })}>{busy ? "Saving…" : "Save changes"}</button>}</div>
    </div>}
    {error && <p className="queue-error" role="alert">{error}</p>}
  </section>;
}
