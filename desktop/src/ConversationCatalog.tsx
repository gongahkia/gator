import { Archive, ArchiveRestore, ArrowLeft, ChevronLeft, ChevronRight, MessageSquare, MoreHorizontal, Pencil, Pin, PinOff, RefreshCw, Search, X } from "lucide-react";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { createPortal } from "react-dom";
import type { BackendPanel, PanelAction } from "./protocol";
import type { DesktopFrame } from "./bridge";
import { datedChats, Highlight } from "./chatPresentation";
import { shortDate, workspaceName } from "./presentation";

export function ConversationCatalog({ panel, send }: { panel: BackendPanel; send(frame: DesktopFrame): Promise<void> }) {
  const [query, setQuery] = useState(panel.query || "");
  const [sending, setSending] = useState(false);
  const [menu, setMenu] = useState<number | null>(null);
  const [editing, setEditing] = useState<{ row: number; identity?: string; text: string } | null>(null);
  const [savingTitle, setSavingTitle] = useState<string | null>(null);
  const [menuAt, setMenuAt] = useState({ left: 0, top: 0 });
  const menuElement = useRef<HTMLDivElement>(null);
  const root = useRef<HTMLElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const current = useRef(panel);
  current.current = panel;
  const prefix = useId();
  const loading = !!panel.catalog?.loading;
  const busy = loading || sending;
  const queryPending = query !== (panel.query || "");
  const rows = (panel.rows || []).map((row, index) => ({ row, index }));
  const pinned = rows.filter(({ row }) => row.pinned);
  const regular = rows.filter(({ row }) => !row.pinned);
  const positionMenu = (rect: DOMRect) => ({ left: Math.max(8, Math.min(innerWidth - 192, rect.right - 184)), top: rect.bottom + 124 > innerHeight ? Math.max(8, rect.top - 124) : rect.bottom + 4 });

  async function act(action: PanelAction["action"], options: { row?: number; text?: string } = {}) {
    const view = current.current;
    if (!view.id || sending) return;
    setSending(true); setMenu(null);
    try { await send({ type: "panel", ...(action === "key" ? { key: "esc" } : {}), panel: { id: view.id, action, ...options } }); }
    catch { /* The app presents transport/stale-view feedback. */ }
    finally { setSending(false); }
  }
  useEffect(() => {
    if (!queryPending) return;
    const timer = window.setTimeout(() => { void act("query", { text: query }); }, 260);
    return () => window.clearTimeout(timer);
  }, [query, panel.query, panel.id, sending]);
  useEffect(() => { setMenu(null); }, [panel.id]);
  useEffect(() => {
    if (savingTitle && !loading && !sending && panel.id !== savingTitle) {
      if (panel.notice_tone !== "error") setEditing(null);
      setSavingTitle(null);
    }
  }, [panel.id, loading, sending]);
  useEffect(() => {
    if (menu === null) return;
  	 const focus = requestAnimationFrame(() => menuElement.current?.querySelector("button")?.focus({ preventScroll: true }));
    const outside = (event: PointerEvent) => { if (!(event.target as HTMLElement).closest("[data-chat-actions]") && !menuElement.current?.contains(event.target as Node)) setMenu(null); };
	 const scroller = root.current?.querySelector(".workflow-content");
	 const reposition = () => {
	   const button = root.current?.querySelector<HTMLButtonElement>(`[data-manage-row="${menu}"]`);
	   if (!button) { setMenu(null); return; }
	   const rect = button.getBoundingClientRect(), viewport = scroller?.getBoundingClientRect();
	   if (viewport && (rect.bottom < viewport.top || rect.top > viewport.bottom)) setMenu(null);
	   else setMenuAt(positionMenu(rect));
	 };
    const escape = (event: globalThis.KeyboardEvent) => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); setMenu(null); root.current?.querySelector<HTMLButtonElement>(`[data-manage-row="${menu}"]`)?.focus(); } };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", escape, true);
    window.addEventListener("resize", reposition);
    scroller?.addEventListener("scroll", reposition);
    return () => { cancelAnimationFrame(focus); document.removeEventListener("pointerdown", outside); document.removeEventListener("keydown", escape, true); window.removeEventListener("resize", reposition); scroller?.removeEventListener("scroll", reposition); };
  }, [menu]);

  function resetLocal() { setEditing(null); setMenu(null); }
  function keys(event: KeyboardEvent, index?: number) {
    if (event.nativeEvent.isComposing) return;
    if (event.key === "Escape" && (editing || menu !== null)) { event.preventDefault(); event.stopPropagation(); resetLocal(); return; }
    if (event.key === "Escape" && query) { event.preventDefault(); event.stopPropagation(); setQuery(""); search.current?.focus(); return; }
    if (event.key === "Enter" && index === undefined && !busy && !queryPending) { event.preventDefault(); root.current?.querySelector<HTMLButtonElement>("[data-chat-row]")?.click(); return; }
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key) || busy || queryPending) return;
    if (index === undefined && ["Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const buttons = [...(root.current?.querySelectorAll<HTMLButtonElement>("[data-chat-row]") || [])];
    const at = buttons.findIndex(button => button.dataset.chatRow === String(index));
    const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : index === undefined ? event.key === "ArrowDown" ? 0 : buttons.length - 1 : Math.max(0, Math.min(buttons.length - 1, at + (event.key === "ArrowDown" ? 1 : -1)));
    buttons[next]?.focus();
  }
  function item({ row, index }: typeof rows[number]) {
    return <div className="catalog-chat" key={row.identity || index}>
      {editing?.row === index ? <form className="catalog-rename" onSubmit={event => { event.preventDefault(); if (editing.identity !== row.identity) return; setSavingTitle(panel.id || null); void act("rename", { row: index, text: editing.text }); }}>
        <label htmlFor={`${prefix}-title`}>Conversation title</label><input id={`${prefix}-title`} autoFocus value={editing.text} maxLength={256} disabled={busy} onChange={event => setEditing({ ...editing, text: event.target.value })} onKeyDown={event => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); setEditing(null); } }} />
        {editing.identity !== row.identity && <p role="status">This list changed. Cancel and choose the conversation again.</p>}
        <div><button className="small-button" type="submit" disabled={busy || editing.identity !== row.identity || !editing.text.trim() || new TextEncoder().encode(editing.text.trim()).length > 256}>Save title</button><button className="text-button" type="button" disabled={busy} onClick={() => setEditing(null)}>Cancel</button></div>
      </form> : <button className="panel-row chat-search-row" data-chat-row={index} aria-label={`Open conversation: ${row.label}`} disabled={busy || queryPending} onKeyDown={event => keys(event, index)} onClick={() => void act("activate", { row: index })}>
        <span className="chat-search-copy"><strong><Highlight text={row.label || "Untitled conversation"} query={query} /></strong>{row.archived && <small className="catalog-archived">Archived</small>}<span className="chat-search-source" title={row.source}><Highlight text={row.source ? query && row.source.toLocaleLowerCase().includes(query.toLocaleLowerCase()) ? row.source : workspaceName(row.source, rows.map(({ row }) => row.source || "")) : "Workspace"} query={query} /></span>{row.detail && <span className="catalog-snippet"><Highlight text={row.detail} query={query} /></span>}</span>
        <span className="chat-search-meta">{row.updated_at && <time dateTime={row.updated_at} title={new Date(row.updated_at).toLocaleString()}>{shortDate(row.updated_at)}</time>}<ChevronRight size={16} aria-hidden="true" /></span>
      </button>}
      {panel.catalog?.can_organize && !editing && <div className="catalog-row-actions" data-chat-actions>
        <button className="icon-button" data-manage-row={index} disabled={busy || queryPending} aria-label={`Manage conversation: ${row.label}`} aria-expanded={menu === index} aria-controls={menu === index ? `${prefix}-actions-${index}` : undefined} onClick={event => { setMenuAt(positionMenu(event.currentTarget.getBoundingClientRect())); setMenu(menu === index ? null : index); }}><MoreHorizontal size={17} /></button>
        {menu === index && createPortal(<div ref={menuElement} data-chat-actions className="catalog-row-menu" id={`${prefix}-actions-${index}`} style={menuAt} onBlur={event => { if (event.relatedTarget && !event.currentTarget.contains(event.relatedTarget)) setMenu(null); }}><button onClick={() => { setMenu(null); setEditing({ row: index, identity: row.identity, text: row.label }); }}><Pencil size={14} />Rename</button><button onClick={() => void act(row.pinned ? "unpin" : "pin", { row: index })}>{row.pinned ? <PinOff size={14} /> : <Pin size={14} />}{row.pinned ? "Unpin" : "Pin"}</button><button onClick={() => void act(row.archived ? "restore" : "archive", { row: index })}>{row.archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}{row.archived ? "Restore to active chats" : "Archive"}</button></div>, document.body)}
      </div>}
    </div>;
  }
  return <section ref={root} className="workflow chat-browser conversation-catalog" aria-label="Conversations" aria-busy={busy || queryPending} onPointerDown={event => { if (!(event.target as HTMLElement).closest("[data-chat-actions]")) setMenu(null); }}>
    <div className="workflow-heading"><button className="icon-button" aria-label="Back to conversation" onClick={() => void act("key")}><ArrowLeft size={18} /></button><div><h1>Conversations</h1><p>Pick up where you left off.</p></div></div>
    <div className="panel-search" role="search"><Search size={16} /><input ref={search} type="search" aria-label="Search conversations" aria-describedby={`${prefix}-scope`} autoFocus placeholder="Search titles, folders or messages…" maxLength={256} value={query} onChange={event => { resetLocal(); setQuery(event.target.value); }} onKeyDown={event => keys(event)} />{query && <button className="icon-button" aria-label="Clear conversation search" onClick={() => { setQuery(""); search.current?.focus(); }}><X size={16} /></button>}</div>
    <div className="catalog-toolbar"><div className="catalog-scopes" role="group" aria-label="Conversation scope">{panel.filter?.options.map(option => <button key={option.value} aria-pressed={panel.filter?.value === option.value} disabled={busy || queryPending} onClick={() => { resetLocal(); void act("filter", { text: option.value }); }}>{option.label}</button>)}</div><button className="icon-button" aria-label="Refresh conversations" disabled={busy || queryPending} onClick={() => void act("refresh")}><RefreshCw size={15} /></button></div>
    <p className="chat-search-scope" id={`${prefix}-scope`}>Titles, folders and retained messages · up to 50 chats per page</p>
    <div className="workflow-content"><p className="chat-search-results" role="status" aria-live="polite">{busy || queryPending ? "Searching conversations…" : query.trim() ? `${rows.length} ${rows.length === 1 ? "match" : "matches"} on this page${panel.catalog?.has_next ? " · more on the next page" : ""}` : ""}</p>
      {!!pinned.length && <section className="chat-date-group" aria-label="Pinned conversations"><h2><Pin size={12} /> Pinned</h2>{pinned.map(item)}</section>}
      {datedChats(regular, ({ row }) => row.updated_at).map(([label, items]) => <section key={label} className="chat-date-group" aria-label={label}><h2>{label}</h2>{items.map(item)}</section>)}
      {!rows.length && !busy && !queryPending && <div className="panel-empty"><MessageSquare size={24} /><h2>{panel.notice_tone === "error" ? "Conversation list unavailable" : query.trim() ? "No matching conversations" : panel.filter?.value === "archived" ? "No archived chats" : "No conversations yet"}</h2><p>{query.trim() ? "Try another phrase or include archived chats." : panel.filter?.value === "archived" ? "Archived chats stay available here." : "Start a conversation to see it here."}</p></div>}
      {panel.notice && <p className="work-details-notice" role={panel.notice_tone === "error" ? "alert" : "status"}>{panel.notice}</p>}
    </div>
    <div className="catalog-pagination"><button className="small-button" disabled={busy || queryPending || !panel.catalog?.has_previous} onClick={() => { resetLocal(); void act("previous"); }}><ChevronLeft size={14} />Previous</button><span>Page {panel.catalog?.page || 1}</span><button className="small-button" disabled={busy || queryPending || !panel.catalog?.has_next} onClick={() => { resetLocal(); void act("next"); }}>Next<ChevronRight size={14} /></button></div>
  </section>;
}
