import { useEffect, useRef, useState } from "react";
import { History, MoreHorizontal, Pencil, X } from "lucide-react";
import type { BackendMessage } from "./protocol";
import { useModalFocus } from "./useModalFocus";

const withinLimit = (text: string) => new TextEncoder().encode(text).length <= 128 * 1024;

export function MessageTools({ context, messages, draft, disabled, editorDisabled, stage, restoreEditor, editExternally, onBusyChange }: {
  context: string; messages: BackendMessage[]; draft: string; disabled: boolean; editorDisabled: boolean;
  stage(text: string, append: boolean): void; restoreEditor(before: string, text: string): void;
  editExternally(): Promise<{ text: string; error: string }>; onBusyChange(busy: boolean): void;
}) {
  const [mode, setMode] = useState<"menu" | "history" | "replace" | "recovery">();
  const [query, setQuery] = useState("");
  const [review, setReview] = useState("");
  const [issue, setIssue] = useState("");
  const [busy, setBusy] = useState(false);
  const root = useRef<HTMLDivElement>(null), alive = useRef(true);
  const focus = useModalFocus(!!mode);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useEffect(() => {
    if (!mode) return;
    const outside = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node) && mode !== "recovery" && !busy) setMode(undefined); };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, [mode, busy]);
  const reuse = (text: string, append: boolean) => {
    const next = append && draft ? `${draft}\n\n${text}` : text;
    if (!withinLimit(next)) { setIssue("The combined message exceeds 128 KiB. Shorten it before adding it to your draft."); return; }
    stage(text, append); setMode(undefined); setIssue("");
  };
  const editor = async () => {
    if (busy || disabled || editorDisabled) return;
    const before = draft;
    setBusy(true); onBusyChange(true); setIssue("");
    try {
      const result = await editExternally();
      if (!alive.current) return;
      if (!withinLimit(result.text)) { setReview(result.text); setMode("recovery"); setIssue("The editor returned more than 128 KiB. Your original draft is kept. Shorten the returned text, then add it explicitly."); }
      else {
        try { restoreEditor(before, result.text); setMode(result.error ? "menu" : undefined); setIssue(result.error); }
        catch { setReview(result.text); setMode("recovery"); setIssue("The kept draft changed and cannot fit the editor result. Shorten the returned text before adding it."); }
      }
    } catch (error) { if (alive.current) { setMode("menu"); setIssue(error instanceof Error ? error.message : "The editor could not return its result. Your draft is kept."); } }
    finally { onBusyChange(false); if (alive.current) setBusy(false); }
  };
  const items = messages.filter(message => message.role === "You" && message.text.trim()).reverse().filter(message => message.text.toLocaleLowerCase().includes(query.toLocaleLowerCase()));
  return <div className="message-tools" ref={root} data-context={context}>
    <button className="icon-button" aria-label="Message tools" title="Message tools" aria-expanded={!!mode} disabled={disabled || busy} onClick={() => { setIssue(""); setMode(mode ? undefined : "menu"); }}><MoreHorizontal size={18} /></button>
    {mode && <section className="message-tools-panel" ref={focus} role="dialog" aria-label="Message tools" onKeyDown={event => { if (event.key === "Escape") { event.stopPropagation(); if (!busy && mode !== "recovery") setMode(undefined); } }}>
      <header><strong>{mode === "history" ? "Reuse a message" : mode === "recovery" ? "Recover editor text" : mode === "replace" ? "Review replacement" : "Message tools"}</strong><button className="icon-button" aria-label="Close message tools" disabled={busy || mode === "recovery"} onClick={() => setMode(undefined)}><X size={16} /></button></header>
      {mode === "menu" ? <>
        <button className="message-tool-action" disabled={busy || !items.length} onClick={() => { setQuery(""); setMode("history"); }}><History size={17} /><span>Reuse a message<small>Stage previous text from this chat without sending</small></span></button>
        <button className="message-tool-action" disabled={busy || editorDisabled} onClick={() => void editor()}><Pencil size={17} /><span>{busy ? "Waiting for the editor…" : "Edit draft in external editor"}<small>{editorDisabled ? "Available for idle chats on macOS and Linux" : "Uses the host’s configured editor in system Terminal"}</small></span></button>
      </> : mode === "history" ? <>
        <input aria-label="Search previous messages" placeholder="Search this chat’s messages…" value={query} onChange={event => setQuery(event.target.value)} />
        <div className="message-history-list">{items.slice(0, 50).map((message, index) => <article key={`${message.run_id ?? ""}:${index}`}><pre>{message.text}</pre><div><button className="small-button" onClick={() => reuse(message.text, true)}>Append to draft</button><button className="small-button" onClick={() => { setReview(message.text); setMode("replace"); }}>Review replacement</button></div></article>)}{!items.length && <p>No matching messages.</p>}{items.length > 50 && <p>Showing 50 matches. Search to narrow the list.</p>}</div>
      </> : <>
        <p>{mode === "recovery" ? "Your original draft and attachments are still kept." : "This replaces the message text. Attached files and the selected update reply stay in this chat."}</p>
        <textarea aria-label="Reviewed message text" value={review} onChange={event => setReview(event.target.value)} rows={6} />
        <div className="message-tools-review"><button className="small-button" onClick={() => { setMode(mode === "recovery" ? undefined : "history"); setReview(""); setIssue(""); }}>{mode === "recovery" ? "Discard returned text" : "Back"}</button><button className="button" disabled={!withinLimit(review)} onClick={() => reuse(review, mode === "recovery")}>{mode === "recovery" ? "Add to kept draft" : "Replace draft text"}</button></div>
      </>}
      {issue && <p className="composer-error" role="alert">{issue}</p>}
      <p className="work-details-caption">Nothing is sent. Files from previous messages are not reattached.</p>
    </section>}
  </div>;
}
