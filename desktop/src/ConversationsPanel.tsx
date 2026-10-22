import { ArrowLeft, ChevronRight, MessageSquare, Search, X } from "lucide-react";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import type { BackendPanel } from "./protocol";
import type { DesktopFrame } from "./bridge";
import { Panel } from "./Panel";
import { datedChats, Highlight } from "./chatPresentation";
import { shortDate, workspaceName } from "./presentation";

export function ConversationsPanel({ panel, send, issue }: { panel: BackendPanel; send(frame: DesktopFrame): Promise<void>; issue?: string }) {
  const runs = panel.kind === "history";
  const detail = runs && !!panel.blocks?.length;
  return runs ? <Panel panel={panel} send={send} title={detail ? panel.subtitle || "Task run" : "Task runs"} subtitle={detail ? "Task run details" : `Individual task runs · ${panel.subtitle || "all"}`} /> : <ChatBrowser panel={panel} send={send} issue={issue} />;
}

function ChatBrowser({ panel, send, issue }: { panel: BackendPanel; send(frame: DesktopFrame): Promise<void>; issue?: string }) {
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(panel.selected ?? 0);
  const [busy, setBusy] = useState(false);
  const search = useRef<HTMLInputElement>(null);
  const root = useRef<HTMLElement>(null);
  const id = useId();
  const term = query.trim().toLocaleLowerCase();
  const rows = (panel.rows ?? []).map((row, index) => ({ row, index })).filter(({ row }) => !term || `${row.label}\n${row.source || row.detail || ""}`.toLocaleLowerCase().includes(term));
  const active = rows.some(({ index }) => index === selected) ? selected : rows[0]?.index;
  useEffect(() => { setQuery(""); setSelected(panel.selected ?? 0); }, [panel.id]);
  const action = async (index?: number) => {
    if (busy || !panel.id) return;
    setBusy(true);
    try { await send({ type: "panel", ...(index === undefined ? { key: "esc" } : {}), panel: { id: panel.id, action: index === undefined ? "key" : "activate", ...(index === undefined ? {} : { row: index }) } }); }
    catch { /* The app owns transport and stale-view feedback. */ }
    finally { setBusy(false); }
  };
  const focusRow = (index?: number) => { if (index !== undefined) { setSelected(index); root.current?.querySelector<HTMLButtonElement>(`[data-row="${index}"]`)?.focus(); } };
  const keys = (event: KeyboardEvent, index?: number) => {
    if (event.nativeEvent.isComposing) return;
    if (event.key === "Escape" && query) { event.preventDefault(); event.stopPropagation(); setQuery(""); search.current?.focus(); return; }
    if (event.key === "Enter" && index === undefined && active !== undefined) { event.preventDefault(); void action(active); return; }
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key) || busy) return;
    if (index === undefined && ["Home", "End"].includes(event.key)) return; // Keep native text editing.
    event.preventDefault();
    const position = rows.findIndex((item) => item.index === index);
    const next = event.key === "Home" ? 0 : event.key === "End" ? rows.length - 1 : index === undefined ? event.key === "ArrowDown" ? 0 : rows.length - 1 : Math.max(0, Math.min(rows.length - 1, position + (event.key === "ArrowDown" ? 1 : -1)));
    focusRow(rows[next]?.index);
  };
  return <section ref={root} className="workflow chat-browser" aria-label="Conversations" aria-busy={busy}>
    <div className="workflow-heading"><button className="icon-button" aria-label="Back to conversation" disabled={busy} onClick={() => void action()}><ArrowLeft size={18} /></button><div><h1>Conversations</h1><p>Find a topic and pick up where you left off.</p></div></div>
    {issue && <div className="chat-list-issue" role="status"><p>The conversation list couldn’t refresh.{!!panel.rows?.length && " Showing the last loaded conversations."}</p><button className="small-button" disabled={busy} onClick={async () => { setBusy(true); try { await send({ type: "navigate", text: "/conversations" }); } catch {} finally { setBusy(false); } }}>Try again</button></div>}
    <div className="panel-search" role="search"><Search size={16} /><input ref={search} type="search" aria-label="Search conversations" aria-describedby={`${id}-scope`} autoFocus placeholder="Search titles or folders…" maxLength={256} value={query} onChange={(event) => { setQuery(event.target.value); setSelected(-1); }} onKeyDown={(event) => keys(event)} />{query && <button className="icon-button" aria-label="Clear conversation search" onClick={() => { setQuery(""); search.current?.focus(); }}><X size={16} /></button>}</div>
    <p className="chat-search-scope" id={`${id}-scope`}>Titles and folders · up to 50 recent saved conversations</p>
    <div className="workflow-content"><div className="chat-search-results" aria-live="polite" role="status">{term ? `${rows.length} ${rows.length === 1 ? "match" : "matches"}` : ""}</div>
      {rows.length ? datedChats(rows, ({ row }) => row.updated_at).map(([label, items]) => <section key={label} className="chat-date-group" aria-label={label}><h2>{label}</h2>{items.map(({ row, index }) => <button key={index} className="panel-row chat-search-row" data-row={index} aria-current={active === index ? "true" : undefined} disabled={busy} onFocus={() => setSelected(index)} onKeyDown={(event) => keys(event, index)} onClick={() => void action(index)}>
        <span className="chat-search-copy"><strong><Highlight text={row.label || "Untitled conversation"} query={query} /></strong><span className="chat-search-source" title={row.source || row.detail}><Highlight text={row.source ? term && row.source.toLocaleLowerCase().includes(term) ? row.source : workspaceName(row.source, panel.rows?.map((row) => row.source || "")) : row.detail || ""} query={query} /></span></span><span className="chat-search-meta">{row.updated_at && <time dateTime={row.updated_at} title={new Date(row.updated_at).toLocaleString()}>{shortDate(row.updated_at)}</time>}<ChevronRight size={16} aria-hidden="true" /></span>
      </button>)}</section>) : <div className="panel-empty"><MessageSquare size={24} /><h2>{term ? "No matching conversations" : issue ? "Conversation list unavailable" : "No conversations yet"}</h2><p>{term ? "Try another title or folder." : issue ? "Try refreshing the list." : "Start a conversation to see it here."}</p></div>}
      {panel.notice && <p className="work-details-notice" role="status">{panel.notice}</p>}
    </div>
  </section>;
}
