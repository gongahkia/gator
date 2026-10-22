import { ChevronDown, ChevronRight, Folder, Search, X } from "lucide-react";
import { useId, useMemo, useState } from "react";
import type { BackendState } from "./protocol";
import type { ChatActivity } from "./bridge";
import { datedChats, Highlight } from "./chatPresentation";

export function RecentConversations({ state, activity: activityRows, disabled, open, browse }: { state: BackendState; activity?: ChatActivity[]; disabled: boolean; open(id: string): void; browse(): void }) {
  const [grouped, setGrouped] = useState(() => localStorage.getItem("gator:group-topics-by-folder") === "true");
  const [all, setAll] = useState(false);
  const [closed, setClosed] = useState(new Set<string>());
  const [expanded, setExpanded] = useState(new Set<string>());
  const [query, setQuery] = useState("");
  const prefix = useId();
  const term = query.trim().toLocaleLowerCase();
  const groups = useMemo(() => {
    const grouped = new Map<string, NonNullable<BackendState["recent_conversations"]>>();
    for (const item of state.recent_conversations ?? []) { const items = grouped.get(item.source) ?? []; items.push(item); grouped.set(item.source, items); }
    const names = [...grouped.keys()].map((source) => source.split(/[\\/]/).filter(Boolean).at(-1));
    return [...grouped].map(([source, items]) => {
      const parts = source.split(/[\\/]/).filter(Boolean), name = parts.at(-1) || source || "Workspace";
      return { source, items, name: names.filter((item) => item === name).length > 1 ? parts.slice(-2).join("/") : name };
    });
  }, [state.recent_conversations]);
  if (!groups.length && !state.recent_issue) return null;
  const matchesTopic = (item: NonNullable<BackendState["recent_conversations"]>[number]) => !term || `${item.title}\n${item.source}`.toLocaleLowerCase().includes(term);
  const matches = groups.map((group) => ({ ...group, visible: group.items.filter(matchesTopic) })).filter((group) => group.visible.length);
  const total = matches.reduce((count, group) => count + group.visible.length, 0);
  const toggle = (source: string, current: Set<string>, set: (next: Set<string>) => void) => { const next = new Set(current); if (next.has(source)) next.delete(source); else next.add(source); set(next); };
  const row = (item: NonNullable<BackendState["recent_conversations"]>[number]) => {
    const activity = activityRows?.find(row => row.id === item.id)?.status ?? (item.id === state.conversation_id && state.running ? state.cancelling ? "Stopping" : state.pending_approval ? "Needs review" : "Working" : undefined);
    return <button key={item.id} className="recent-chat" aria-label={`Open conversation: ${item.title}`} aria-describedby={activity ? `${prefix}-${item.id}-activity` : undefined} aria-current={state.conversation_id === item.id && !state.panel ? "page" : undefined} title={`${item.title}\n${item.source}\n${new Date(item.updated_at).toLocaleString()}`} disabled={disabled} onKeyDown={(event) => {
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const buttons = [...(event.currentTarget.closest(".recent-work")?.querySelectorAll<HTMLButtonElement>(".recent-chat:not(:disabled)") ?? [])].filter((button) => button.getClientRects().length);
      const position = buttons.indexOf(event.currentTarget);
      const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : Math.max(0, Math.min(buttons.length - 1, position + (event.key === "ArrowDown" ? 1 : -1)));
      buttons[next]?.focus();
    }} onClick={() => open(item.id)}><span><Highlight text={item.title || "Untitled conversation"} query={query} />{term && item.source.toLocaleLowerCase().includes(term) && <small className="recent-chat-source"><Highlight text={item.source} query={query} /></small>}</span>{activity && <small id={`${prefix}-${item.id}-activity`} className="recent-chat-activity" data-attention={activity === "Needs review" || activity === "Needs attention"}>{activity}</small>}</button>;
  };
  const flat = (state.recent_conversations ?? []).filter((item) => matchesTopic(item));
  const shown = all || term ? flat : flat.slice(0, 8);
  const active = flat.find((item) => item.id === state.conversation_id);
  if (!all && !term && active && !shown.includes(active)) shown[shown.length - 1] = active;
  return <section className="recent-work" aria-label="Recent conversations"><div className="recent-heading"><div className="nav-group-label">Recent chats</div><button className="icon-button" aria-label="Group chats by folder" aria-pressed={grouped} title={grouped ? "Show chats without folder groups" : "Group chats by folder"} onClick={() => { const next = !grouped; setGrouped(next); try { localStorage.setItem("gator:group-topics-by-folder", String(next)); } catch {} }}><Folder size={14} /></button></div>
    {state.recent_issue && <p className="recent-issue" role="status">{state.recent_issue}</p>}
    {!!groups.length && <div className="recent-filter"><Search size={14} aria-hidden="true" /><input type="search" aria-label="Filter recent conversations" placeholder="Filter recent chats" maxLength={128} value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (!event.nativeEvent.isComposing && ["ArrowDown", "ArrowUp", "Enter"].includes(event.key)) {
      const buttons = [...(event.currentTarget.closest(".recent-work")?.querySelectorAll<HTMLButtonElement>(".recent-chat:not(:disabled)") ?? [])].filter((button) => button.getClientRects().length);
      if (buttons.length) { event.preventDefault(); if (event.key === "Enter") buttons[0].click(); else (event.key === "ArrowDown" ? buttons[0] : buttons.at(-1))?.focus(); }
    }
    if (event.key === "Escape" && query) { event.preventDefault(); event.stopPropagation(); setQuery(""); } }} />{query && <button className="icon-button" aria-label="Clear recent filter" onClick={(event) => { setQuery(""); event.currentTarget.parentElement?.querySelector("input")?.focus(); }}><X size={13} /></button>}</div>}
    {term && <p className="recent-filter-status" role="status">{total ? `${total} recent ${total === 1 ? "conversation" : "conversations"}` : "No matching recent conversations"}</p>}
    {!grouped ? <div className="recent-flat">{datedChats(shown, (item) => item.updated_at).map(([label, items]) => <div className="recent-date-group" key={label}><h3>{label}</h3>{items.map(row)}</div>)}{!term && flat.length > 8 && <button className="recent-more" onClick={() => setAll(!all)}>{all ? "Show fewer" : "Show more"}<ChevronDown size={12} /></button>}</div> : matches.map(({ source, name, items, visible }, index) => {
      const expandedGroup = !!term || expanded.has(source);
      const shown = expandedGroup ? visible : visible.slice(0, 4);
      const active = visible.find((item) => item.id === state.conversation_id);
      if (!expandedGroup && active && !shown.includes(active)) shown[shown.length - 1] = active;
      return <div className="recent-group" key={source}><button className="recent-workspace" aria-expanded={!!term || !closed.has(source)} aria-controls={`${prefix}-recent-${index}`} title={source} onClick={() => { if (term) setQuery(""); toggle(source, closed, setClosed); }}><ChevronRight size={13} /><Folder size={14} /><span>{name}</span></button><div id={`${prefix}-recent-${index}`} className="recent-group-list" hidden={!term && closed.has(source)}>{shown.map(row)}{!term && items.length > 4 && <button className="recent-more" onClick={() => toggle(source, expanded, setExpanded)}>{expanded.has(source) ? "Show fewer" : items.length > shown.length ? `Show ${items.length - shown.length} more` : "Show fewer"}<ChevronDown size={12} /></button>}</div></div>;
    })}
    <button className="recent-browse" disabled={disabled} onClick={browse}>All conversations<ChevronRight size={13} /></button>
  </section>;
}
