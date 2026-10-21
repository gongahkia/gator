import { ChevronDown, ChevronRight, Folder } from "lucide-react";
import { useMemo, useState } from "react";
import type { BackendState } from "../../ui/src/protocol";

export function RecentConversations({ state, disabled, open, browse }: { state: BackendState; disabled: boolean; open(id: string): void; browse(): void }) {
  const [closed, setClosed] = useState(new Set<string>());
  const [expanded, setExpanded] = useState(new Set<string>());
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
  const toggle = (source: string, current: Set<string>, set: (next: Set<string>) => void) => { const next = new Set(current); if (next.has(source)) next.delete(source); else next.add(source); set(next); };
  return <section className="recent-work" aria-label="Recent work"><div className="nav-group-label">Recent work</div>
    {state.recent_issue && <p className="recent-issue" role="status">{state.recent_issue}</p>}
    {groups.map(({ source, name, items }, index) => <div className="recent-group" key={source}><button className="recent-workspace" aria-expanded={!closed.has(source)} aria-controls={`recent-group-${index}`} title={source} onClick={() => toggle(source, closed, setClosed)}><ChevronRight size={13} /><Folder size={14} /><span>{name}</span></button><div id={`recent-group-${index}`} className="recent-group-list" hidden={closed.has(source)}>{(expanded.has(source) ? items : items.slice(0, 4)).map((item) => <button key={item.id} className="recent-chat" aria-label={`Open conversation: ${item.title}`} aria-current={state.conversation_id === item.id && !state.panel ? "page" : undefined} title={`${item.title}\n${item.source}`} disabled={disabled} onClick={() => open(item.id)}><span>{item.title || "Untitled conversation"}</span></button>)}{items.length > 4 && <button className="recent-more" onClick={() => toggle(source, expanded, setExpanded)}>{expanded.has(source) ? "Show fewer" : `Show ${items.length - 4} more`}<ChevronDown size={12} /></button>}</div></div>)}
    <button className="recent-browse" disabled={disabled} onClick={browse}>All conversations<ChevronRight size={13} /></button>
  </section>;
}
