import { useCallback, useEffect, useId, useRef, useState, type DragEvent, type KeyboardEvent } from "react";
import { CornerDownLeft, GripVertical, ListOrdered, Pencil, X } from "lucide-react";
import type { BackendState, QueueAction, QueuedMessage } from "./protocol";
import type { DesktopFrame } from "./bridge";

export type QueueChange = Omit<QueueAction, "request_id">;
type QueueDrag = { id: string; signature: string; keyboard: boolean; position: number; target?: string; placement?: "before" | "after" };

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
  const [drag, setDrag] = useState<QueueDrag | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const instructions = useId();
  const editor = useRef<HTMLTextAreaElement>(null);
  const region = useRef<HTMLElement>(null);
  const latestItems = useRef(items);
  latestItems.current = items;
  const signature = items.map((item) => item.id).join(":");
  useEffect(() => {
    if (drag && (drag.signature !== signature || busy || editing)) {
      setDrag(null); setAnnouncement("The queue changed. Choose a message to reorder again.");
    }
  }, [signature, busy, !!editing, drag?.signature]);
  useEffect(() => {
    if (!drag) return;
    const cancel = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault(); event.stopPropagation(); setDrag(null); setAnnouncement("Reordering cancelled.");
    };
    window.addEventListener("keydown", cancel, true);
    return () => window.removeEventListener("keydown", cancel, true);
  }, [!!drag]);
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
  const focusHandle = (id: string) => requestAnimationFrame(() => {
    const row = [...(region.current?.querySelectorAll<HTMLElement>("[data-queue-id]") ?? [])].find((element) => element.dataset.queueId === id);
    const handle = row?.querySelector<HTMLButtonElement>(".queue-drag-handle");
    (handle ?? region.current)?.focus({ preventScroll: true });
  });
  const reorder = async (id: string, target: string, placement: "before" | "after") => {
    setDrag(null); setError("");
    const sourceIndex = items.findIndex((item) => item.id === id), targetIndex = items.findIndex((item) => item.id === target);
    if (sourceIndex >= 0 && targetIndex >= 0 && (id === target || sourceIndex === targetIndex + (placement === "after" ? 1 : -1))) { setAnnouncement("Queue order unchanged."); focusHandle(id); return; }
    try {
      await act({ id, action: "move", target_id: target, placement });
      const position = latestItems.current.findIndex((item) => item.id === id);
      setAnnouncement(position < 0 ? "Queue updated. The message has left the queue." : `Message moved to position ${position + 1} of ${latestItems.current.length}.`);
    } catch (err) { setError(err instanceof Error ? err.message : String(err)); }
    focusHandle(id);
  };
  const pickUp = (item: QueuedMessage, index: number, keyboard: boolean) => {
    setError(""); setDrag({ id: item.id, signature, keyboard, position: index });
    setAnnouncement(`Picked up message ${index + 1} of ${items.length}. Use the up and down arrow keys to choose a position, Enter to drop, or Escape to cancel.`);
  };
  const keyboardDrag = (event: KeyboardEvent<HTMLButtonElement>, item: QueuedMessage, index: number) => {
    if (busy || editing) return;
    if (event.key === "Escape" && drag?.keyboard) { event.preventDefault(); event.stopPropagation(); setDrag(null); setAnnouncement("Reordering cancelled."); return; }
    if (event.key === " " || event.key === "Enter") {
      event.preventDefault();
      if (drag?.keyboard && drag.id === item.id) {
        if (drag.target && drag.placement) void reorder(item.id, drag.target, drag.placement);
        else { setDrag(null); setAnnouncement("Queue order unchanged."); }
      } else pickUp(item, index, true);
      return;
    }
    if (!drag?.keyboard || drag.id !== item.id || !["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
    event.preventDefault(); event.stopPropagation();
    const position = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : Math.max(0, Math.min(items.length - 1, drag.position + (event.key === "ArrowUp" ? -1 : 1)));
    const others = items.filter((value) => value.id !== item.id);
    const target = others[Math.min(position, others.length - 1)];
    setDrag({ ...drag, position, target: target.id, placement: position === others.length ? "after" : "before" });
    setAnnouncement(`Position ${position + 1} of ${items.length}. Press Enter to move this message here.`);
  };
  const dropPosition = (event: DragEvent<HTMLLIElement>) => event.clientY < event.currentTarget.getBoundingClientRect().top + event.currentTarget.offsetHeight / 2 ? "before" as const : "after" as const;
  if (!items.length && !editing && !error) return null;
  const stale = editing && !items.some((item) => item.id === editing.id);
  return <section className="message-queue" aria-label="Queued messages" tabIndex={-1} ref={region} onBlur={(event) => { if (drag?.keyboard && !event.currentTarget.contains(event.relatedTarget as Node | null)) { setDrag(null); setAnnouncement("Reordering cancelled."); } }}>
    <div className="queue-heading"><ListOrdered size={14} /><span role="status">{items.length ? `${items.length} queued` : editing ? "Unsent edits" : "Queue changed"}</span>{!running && items.length > 0 && <span className="queue-paused">Paused</span>}</div>
    <p id={instructions} className="sr-only">Drag to reorder. With the keyboard, press Space or Enter to pick up a message, use arrow keys to choose a position, then Enter to drop. Escape cancels.</p>
    <p className="sr-only" role="status" aria-live="polite">{announcement}</p>
    <ol className="queue-list" onDragOver={(event) => {
      if (!drag || busy || editing) return;
      event.preventDefault();
      const rect = event.currentTarget.getBoundingClientRect();
      if (event.clientY < rect.top + 28) event.currentTarget.scrollTop -= 18;
      else if (event.clientY > rect.bottom - 28) event.currentTarget.scrollTop += 18;
    }}>{items.map((item, index) => <li className="queued-message" key={item.id} data-queue-id={item.id} data-editing={editing?.id === item.id} data-dragging={drag?.id === item.id} data-drop-edge={drag?.target === item.id && drag.id !== item.id ? drag.placement : undefined} onDragOver={(event) => {
      if (!drag || drag.keyboard || busy || editing) return;
      event.preventDefault(); event.dataTransfer.dropEffect = "move";
      const placement = dropPosition(event);
      setDrag((current) => current && (current.target !== item.id || current.placement !== placement) ? { ...current, target: item.id, placement } : current);
    }} onDrop={(event) => {
      if (!drag || drag.keyboard || busy || editing) return;
      event.preventDefault(); event.stopPropagation();
      void reorder(drag.id, item.id, dropPosition(event));
    }}>
      {items.length > 1 && <button className="icon-button queue-drag-handle" draggable={!busy && !editing} aria-label={`Reorder queued message ${index + 1}`} aria-describedby={instructions} aria-pressed={drag?.id === item.id} title="Drag to reorder" disabled={busy || !!editing} onClick={() => {
        if (drag?.keyboard && drag.id === item.id && drag.target && drag.placement) void reorder(item.id, drag.target, drag.placement);
        else if (drag?.keyboard && drag.id === item.id) { setDrag(null); setAnnouncement("Queue order unchanged."); }
        else pickUp(item, index, true);
      }} onKeyDown={(event) => keyboardDrag(event, item, index)} onDragStart={(event) => {
        if (busy || editing) { event.preventDefault(); return; }
        pickUp(item, index, false);
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("application/x-gator-queued-message", item.id);
        event.dataTransfer.setDragImage(event.currentTarget.closest("li")!, 20, 20);
      }} onDragEnd={() => setDrag(null)}><GripVertical size={16} /></button>}
      <span className="queue-position" aria-hidden="true">{index + 1}</span><p className="queue-preview">{item.text}</p>
      <div className="queue-actions">
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
    {error && <div className="queue-error" role="alert"><span>{error}</span><button className="icon-button" aria-label="Dismiss queue error" onClick={() => setError("")}><X size={14} /></button></div>}
  </section>;
}
