import { useCallback, useEffect, useRef, useState } from "react";
import { Bookmark, Check, ChevronDown, Pencil, Undo2 } from "lucide-react";
import type { BackendState, ChatMemoryAction, ChatMemoryReceipt } from "./protocol";
import type { DesktopFrame } from "./bridge";

type Change = Omit<ChatMemoryAction, "request_id">;
export function useChatMemoryAction(state: BackendState | undefined, send: (frame: DesktopFrame) => Promise<void>, connectionError?: string) {
  const pending = useRef<{ id: string; resolve(): void; reject(error: Error): void } | null>(null);
  const [busy, setBusy] = useState(false);
  const finish = useCallback((error?: string) => {
    const p = pending.current; if (!p) return;
    pending.current = null; setBusy(false);
    if (error) p.reject(new Error(error)); else p.resolve();
  }, []);
  useEffect(() => { if (state?.chat_memory_result?.request_id === pending.current?.id) finish(state?.chat_memory_result?.error); }, [state?.chat_memory_result, finish]);
  useEffect(() => { if (connectionError) finish(connectionError); }, [connectionError, finish]);
  useEffect(() => () => { pending.current?.reject(new Error("Conversation closed.")); pending.current = null; }, []);
  const act = useCallback((change: Change) => new Promise<void>((resolve, reject) => {
    if (pending.current) { reject(new Error("Wait for the previous memory change.")); return; }
    const id = crypto.randomUUID(); pending.current = {id,resolve,reject}; setBusy(true);
    void send({type:"memory",memory:{...change,request_id:id}}).catch(error => finish(String(error)));
  }), [send, finish]);
  return {act, busy};
}

type ReceiptProps = {
  conversation: string;
  disabled: boolean;
  act(change: Change): Promise<void>;
  onEditingChange(id: string, editing: boolean): void;
};

export function ChatMemory({ receipts, ...props }: ReceiptProps & { receipts: ChatMemoryReceipt[] }) {
  // Keep only open editor drafts when a refresh removes or rebinds a memory.
  // Forgotten records must not linger as ordinary saved-memory receipts.
  const [drafts, setDrafts] = useState<Map<string, ChatMemoryReceipt>>(() => new Map());
  const retainDraft = useCallback((receipt: ChatMemoryReceipt, keep: boolean) => {
    setDrafts(current => {
      const next = new Map(current);
      if (keep) next.set(receipt.id, receipt); else next.delete(receipt.id);
      return next;
    });
  }, []);
  const ids = new Set(receipts.map(receipt => receipt.id));
  const shown = [...receipts, ...Array.from(drafts.values()).filter(receipt => !ids.has(receipt.id))];
  if (!shown.length) return null;
  return <div className="chat-memory-receipts">{shown.map(receipt =>
    <MemoryReceipt key={receipt.id} receipt={receipt} available={ids.has(receipt.id)} retainDraft={retainDraft} {...props} />
  )}</div>;
}

function MemoryReceipt({ receipt: r, available, retainDraft, conversation, disabled, act, onEditingChange }: ReceiptProps & {
  receipt: ChatMemoryReceipt;
  available: boolean;
  retainDraft(receipt: ChatMemoryReceipt, keep: boolean): void;
}) {
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(r.text);
  const [error, setError] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [saved, setSaved] = useState(false);
  const [reviewed, setReviewed] = useState(r.updated);
  const changed = editing && reviewed !== r.updated;
  const editorID = `${conversation}:${r.run_id}:${r.id}`;
  useEffect(() => {
    onEditingChange(editorID, editing);
    return () => onEditingChange(editorID, false);
  }, [editorID, editing, onEditingChange]);

  const finishEditing = () => {
    setEditing(false);
    retainDraft(r, false);
  };
  const change = async (action: Change["action"]) => {
    setError("");
    setSaved(false);
    try {
      await act({ conversation_id: conversation, id: r.id, updated: action === "undo" ? r.updated : reviewed, action, ...(action === "correct" ? { text } : {}) });
      finishEditing();
      setConfirm(false);
      setSaved(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };
  return <details className="chat-memory-receipt">
    <summary><Bookmark size={13} aria-hidden="true" />{!available ? "Unsaved memory edit" : r.corrected ? "Memory edited" : "Saved to memory"}<ChevronDown size={12} aria-hidden="true" /></summary>
    <div className="chat-memory-detail">
      {available && <p className="chat-memory-scope">{r.scope} · {r.corrected ? "Edited by you" : "From your message in this chat"}</p>}
      {editing ? <form onSubmit={event => { event.preventDefault(); if (available && !changed) void change("correct"); }}>
        {!available && <p role="status" className="chat-memory-scope">This memory was removed or moved to another chat. Your unsaved edit is kept here until you cancel.</p>}
        {changed && <p role="status" className="chat-memory-scope">This memory changed elsewhere. Your draft is kept. Copy any text you need before loading the saved version.</p>}
        <label className="visually-hidden" htmlFor={`memory-${editorID}`}>Edit saved memory</label>
        <textarea id={`memory-${editorID}`} autoFocus value={text} maxLength={4096} onChange={e => setText(e.target.value)} disabled={disabled} />
        <div className="chat-memory-actions">
          <button className="small-button" disabled={disabled || !available || changed || !text.trim() || new TextEncoder().encode(text).length > 4096} type="submit"><Check size={13} />Save memory</button>
          {changed && <button className="small-button" type="button" disabled={disabled} onClick={() => { setText(r.text); setReviewed(r.updated); setError(""); }}>Load saved version</button>}
          <button className="small-button" type="button" disabled={disabled} onClick={finishEditing}>Cancel</button>
        </div>
      </form> : <p className="literal">{r.text}</p>}
      {!editing && !confirm && <div className="chat-memory-actions">
        <button className="small-button" disabled={disabled} onClick={() => { setText(r.text); setReviewed(r.updated); retainDraft(r, true); setEditing(true); setSaved(false); }}><Pencil size={13} />Edit memory</button>
        <button className="small-button" disabled={disabled} onClick={() => r.can_undo ? void change("undo") : (setReviewed(r.updated), setConfirm(true))}><Undo2 size={13} />{r.can_undo ? "Undo memory save" : "Forget memory"}</button>
      </div>}
      {confirm && <div><p>Remove this saved memory? Messages in your chats are kept.</p><div className="chat-memory-actions">
        <button className="small-button" disabled={disabled} onClick={() => void change("forget")}>Forget saved memory</button>
        <button className="small-button" disabled={disabled} onClick={() => setConfirm(false)}>Cancel</button>
      </div></div>}
      {available && r.quote && r.quote !== r.text && <p className="chat-memory-scope">Source: “{r.quote}”</p>}
      <p className="chat-memory-scope">Only this saved fact changes. Earlier messages and context already supplied to running tasks are kept.</p>
      {error && <p role="alert" className="composer-error">{error}</p>}
      {saved && <p role="status" className="chat-memory-scope">Memory updated.</p>}
    </div>
  </details>;
}
