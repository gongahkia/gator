import { ChevronDown, ChevronRight, Folder, Search, X } from "lucide-react";
import { useId, useMemo, useState } from "react";
import type { BackendState } from "./protocol";

export function RecentConversations({ state, disabled, open, browse }: { state: BackendState; disabled: boolean; open(id: string): void; browse(): void }) {
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
  const matches = groups.map((group) => ({ ...group, visible: group.items.filter((item) => !term || `${item.title}\n${group.name}`.toLocaleLowerCase().includes(term)) })).filter((group) => group.visible.length);
  const total = matches.reduce((count, group) => count + group.visible.length, 0);
  const toggle = (source: string, current: Set<string>, set: (next: Set<string>) => void) => { const next = new Set(current); if (next.has(source)) next.delete(source); else next.add(source); set(next); };
  return <section className="recent-work" aria-label="Recent work"><div className="nav-group-label">Recent work</div>
    {state.recent_issue && <p className="recent-issue" role="status">{state.recent_issue}</p>}
    {!!groups.length && <div className="recent-filter"><Search size={14} aria-hidden="true" /><input type="search" aria-label="Filter recent conversations" placeholder="Filter recent chats" maxLength={128} value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (event.key === "Escape" && query) { event.preventDefault(); event.stopPropagation(); setQuery(""); } }} />{query && <button className="icon-button" aria-label="Clear recent filter" onClick={(event) => { setQuery(""); event.currentTarget.parentElement?.querySelector("input")?.focus(); }}><X size={13} /></button>}</div>}
    {term && <p className="recent-filter-status" role="status">{total ? `${total} recent ${total === 1 ? "conversation" : "conversations"}` : "No matching recent conversations"}</p>}
    {matches.map(({ source, name, items, visible }, index) => {
      const expandedGroup = !!term || expanded.has(source);
      const shown = expandedGroup ? visible : visible.slice(0, 4);
      const active = visible.find((item) => item.id === state.conversation_id);
      if (!expandedGroup && active && !shown.includes(active)) shown[shown.length - 1] = active;
      return <div className="recent-group" key={source}><button className="recent-workspace" aria-expanded={!!term || !closed.has(source)} aria-controls={`${prefix}-recent-${index}`} title={source} onClick={() => { if (term) setQuery(""); toggle(source, closed, setClosed); }}><ChevronRight size={13} /><Folder size={14} /><span>{name}</span></button><div id={`${prefix}-recent-${index}`} className="recent-group-list" hidden={!term && closed.has(source)}>{shown.map((item) => {
        const activity = item.id === state.conversation_id && state.running ? state.cancelling ? "Stopping" : state.pending_approval ? "Needs review" : "Working" : undefined;
        return <button key={item.id} className="recent-chat" aria-label={`Open conversation: ${item.title}`} aria-describedby={activity ? `${prefix}-${item.id}-activity` : undefined} aria-current={state.conversation_id === item.id && !state.panel ? "page" : undefined} title={`${item.title}\n${item.source}`} disabled={disabled} onClick={() => open(item.id)}><span>{item.title || "Untitled conversation"}</span>{activity && <small id={`${prefix}-${item.id}-activity`} className="recent-chat-activity" data-attention={!!state.pending_approval}>{activity}</small>}</button>;
      })}{!term && items.length > 4 && <button className="recent-more" onClick={() => toggle(source, expanded, setExpanded)}>{expanded.has(source) ? "Show fewer" : items.length > shown.length ? `Show ${items.length - shown.length} more` : "Show fewer"}<ChevronDown size={12} /></button>}</div></div>;
    })}
    <button className="recent-browse" disabled={disabled} onClick={browse}>All conversations<ChevronRight size={13} /></button>
  </section>;
}
