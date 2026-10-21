import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { ArrowLeft, BookOpen, Check, Circle, ChevronDown, ChevronLeft, ChevronRight, Inbox, ListChecks, MessageSquare, Plus, RefreshCw, Search, Target } from "lucide-react";
import type { BackendPanel, PanelAction } from "../../ui/src/protocol";
import type { DesktopFrame } from "./bridge";
import { PanelField } from "./PanelField";
import { PanelBlocks, PanelFacts } from "./PanelBlocks";
import { shortDate, workspaceName } from "./presentation";

const actionLabel = (value: string) => value ? value[0].toLocaleUpperCase() + value.slice(1) : value;
const shortcutLabel = (key: string, shortcut: string) => key === "ctrl+r" ? `${window.gator?.platform === "darwin" ? "⌘" : "Ctrl+"}Enter` : shortcut;

function shortcutKeys(shortcut: string) {
  return shortcut.toLowerCase().replaceAll(" ", "").split("/").map((key) => ({ "←": "left", "→": "right", "↑": "up", "↓": "down", "f1/esc": "f1" })[key] ?? key).filter((key) => /^(enter|esc|up|down|left|right|tab|shift\+tab|space|ctrl\+[ru]|f1|[a-z0-9])$/.test(key));
}

function emptyState(panel: BackendPanel) {
  if (panel.rows?.length) return;
  if (panel.query?.trim()) return { icon: Search, title: "No results", detail: "Try a different search." };
  if (panel.title === "Saved knowledge") return { icon: BookOpen, title: "No saved knowledge yet.", detail: "Notes and useful context you save will appear here." };
  if (panel.kind === "picker" && panel.title === "Conversations") return { icon: MessageSquare, title: "No conversations yet", detail: "Start a conversation to see it here." };
  if (panel.kind === "picker" && panel.title === "Ongoing goals") return { icon: Target, title: "No ongoing goals", detail: "Set up a follow-up check to create an ongoing goal, or add one with gator goal add in Terminal." };
  if (panel.kind === "followup" && !panel.subtitle) return { icon: ListChecks, title: "No follow-up checks yet", detail: "Choose a project and what to watch. You’ll review the sources, timing, and permissions before saving." };
  if (panel.kind === "learnings" && !panel.blocks?.length) return { icon: BookOpen, title: "No saved guidance yet", detail: "Add a preference or procedure for Gator to follow in future work." };
  if (panel.kind === "inbox" && (!panel.notice || panel.notice === "No recent results.")) return { icon: Inbox, title: "You’re all caught up", detail: panel.notice || "Results from your follow-up checks will appear here." };
  if (panel.kind === "history" && !panel.blocks?.length) return { icon: MessageSquare, title: "No task runs in this view", detail: "Try another filter or start a conversation." };
}

export function Panel({ panel, send, title = panel.title, subtitle = panel.subtitle, navigation }: { panel: BackendPanel; send(frame: DesktopFrame): Promise<void>; title?: string; subtitle?: string; navigation?: ReactNode }) {
  const [fields, setFields] = useState<Record<number, string>>({});
  const [query, setQuery] = useState(panel.query ?? "");
  const [filter, setFilter] = useState("");
  const [busy, setBusy] = useState(false);
  const selected = panel.selected ?? 0;
  const modelCatalog = panel.title === "Cloud models" || panel.title === "Local models";
  const knowledgeLibrary = panel.title === "Saved knowledge" && panel.kind === "context";
  const localFilter = modelCatalog || knowledgeLibrary;
  const rows = (panel.rows ?? []).map((row, index) => ({ row, index }));
  const displayedRows = localFilter ? rows.filter(({ row }) => `${row.label} ${row.detail ?? ""} ${row.state ?? ""}`.toLocaleLowerCase().includes(filter.trim().toLocaleLowerCase())) : rows;
  const displayedSelected = displayedRows.some(({ index }) => index === selected) ? selected : displayedRows[0]?.index;
  const selectedRow = useRef<HTMLButtonElement>(null);
  const formIdentity = `${panel.kind}:${panel.title}:${panel.rows?.map((row) => row.label).join("|")}`;
  useEffect(() => { setFields({}); }, [formIdentity]);
  useEffect(() => { setQuery(panel.query ?? ""); }, [panel.query]);
  useEffect(() => { setFilter(""); }, [panel.kind, panel.title]);
  useEffect(() => { selectedRow.current?.scrollIntoView({ block: "nearest" }); }, [selected, formIdentity]);
  const hasInputs = panel.rows?.some((row) => row.input);
  const action = async (value: Omit<PanelAction, "id">, key?: string) => {
    if (!panel.id || busy) return;
    setBusy(true);
    try { await send({ type: "panel", key, panel: { id: panel.id, ...value } }); }
    catch { /* The application presents the transport error. */ }
    finally { setBusy(false); }
  };
  const keyAction = (key: string, row?: number) => {
    if (modelCatalog && !displayedRows.length && !["esc", "r", "n", "g", "s", "i", "f1"].includes(key)) return;
    // Native form values are submitted together. Go still validates, reviews,
    // and commits the action; secrets never return in its display projection.
    if (key === "enter" && hasInputs && row === undefined) row = (panel.rows?.length ?? 1) - 1;
    if (modelCatalog && row === undefined && key !== "esc") row = displayedSelected;
    void action({ action: "key", row, fields: key === "esc" ? undefined : fields }, key);
  };
  const hints = (panel.hints ?? []).flatMap((hint) => shortcutKeys(hint.shortcut).map((key, i, keys) => ({ key, label: actionLabel(keys.length > 1 ? `${hint.action} (${key})` : hint.action), group: hint.group, tone: hint.tone, shortcut: hint.shortcut, id: `${hint.shortcut}-${i}` }))).filter((hint) => !["up", "down", "tab", "shift+tab", "ctrl+u"].includes(hint.key));

  const empty = localFilter && filter.trim() && !displayedRows.length ? { icon: Search, title: knowledgeLibrary ? "No matching loaded items" : "No matching models", detail: knowledgeLibrary ? "Try another phrase, or search beyond this list using the commands below." : "Try another provider or model name." } : emptyState(panel);
  const EmptyIcon = empty?.icon;
  const rowActivation = new Set(["select", "continue", "expand", "details", "choose", "open", "set effort", "use"]);
  const visibleHints = hints.filter((hint) => {
    if (panel.filter && hint.key === "f" || panel.kind === "effort" && hint.key === "enter") return false;
    if (empty && hint.key === "enter" && rowActivation.has(hint.label.toLowerCase())) return false;
    if (empty && panel.kind === "followup" && hint.key === "s") return false;
    if (hint.key === "esc") return hasInputs || panel.kind === "confirmation";
    if (["enter", "ctrl+r"].includes(hint.key) && (hasInputs || panel.kind === "confirmation")) return true;
    if (hint.key === "enter" && !hasInputs && panel.kind !== "confirmation" && rowActivation.has(hint.label.toLowerCase()) && (panel.rows?.length || empty)) return false;
    // The row already activates this exact Go action. Keep refresh in the
    // toolbar and omit its duplicate row instead.
    return hint.key === "r" || !panel.rows?.some((row) => !row.input && row.label.toLowerCase() === hint.label.toLowerCase());
  });
  const primary = visibleHints.filter((hint) => ["enter", "ctrl+r", "esc"].includes(hint.key));
  const toolbar = visibleHints.filter((hint) => !primary.includes(hint));
  const refresh = toolbar.find((hint) => hint.key === "r" && hint.label.toLowerCase() === "refresh");
  const creates = (hint: typeof hints[number]) => hint.key === "n" && /^(new|add)\b/i.test(hint.label);
  const administration = toolbar.filter((hint) => hint.group === "more" || modelCatalog && (panel.title === "Cloud models" ? ["n", "g", "e", "d", "x"] : ["e", "x"]).includes(hint.key));
  const toolbarButton = (hint: typeof hints[number]) => hint === refresh ? <button className="icon-button" aria-label={hint.label} title={`${hint.label} (${hint.shortcut})`} key={hint.id} disabled={busy} onClick={() => keyAction(hint.key)}><RefreshCw size={17} /></button> : <button className={`button ${creates(hint) && !administration.includes(hint) ? "primary" : ""}`} key={hint.id} disabled={busy || modelCatalog && !displayedRows.length && !["n", "g", "r", "s", "i"].includes(hint.key)} onClick={() => keyAction(hint.key)}>{creates(hint) && <Plus size={16} />}{hint.label}<kbd>{shortcutLabel(hint.key, hint.shortcut)}</kbd></button>;
  const keyboardFooter = /^(Esc\b|↑|↓|Type to filter|Tab\b|Enter(?:\/| or | confirm| save)|V show|F1\b)/i.test(panel.footer ?? "");
  const rowKeys = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (!["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
    if (busy) { event.preventDefault(); return; }
    const buttons = [...event.currentTarget.closest(".panel-rows, .form-fields")!.querySelectorAll<HTMLButtonElement>(".panel-row")];
    const current = buttons.indexOf(event.currentTarget);
    const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : Math.max(0, Math.min(buttons.length - 1, current + (event.key === "ArrowDown" ? 1 : -1)));
    event.preventDefault();
    const target = buttons[next];
    if (target) { target.focus(); void action({ action: "select", row: Number(target.dataset.row ?? index), fields }); }
  };
  useEffect(() => {
    const shortcut = (event: globalThis.KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat || busy || event.altKey || !(event.target instanceof HTMLElement)) return;
      if (event.target.closest('[role="dialog"], [role="alertdialog"]')) return;
      if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && hints.some((hint) => hint.key === "ctrl+r")) { event.preventDefault(); keyAction("ctrl+r"); return; }
      if (event.metaKey) return;
      if (event.target.closest('input, textarea, select, [contenteditable]') && !(event.ctrlKey && event.key.toLowerCase() === "r")) return;
      const key = event.ctrlKey ? `ctrl+${event.key.toLowerCase()}` : event.key.toLowerCase();
      if (["escape", "enter", " "].includes(key)) return; // Native buttons and App own these.
      if (hints.some((hint) => hint.key === key)) { event.preventDefault(); keyAction(key); }
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  });

  return <section className="workflow" data-kind={panel.kind} aria-label={title}>
    <div className="workflow-heading"><button className="icon-button" aria-label={panel.back_label || "Back to conversation"} disabled={busy} onClick={() => keyAction("esc")}><ArrowLeft size={18} /></button><div><h1>{title}</h1>{subtitle && <p>{subtitle}</p>}</div>{panel.kind === "confirmation" && <span className="badge" data-tone="warning">Review</span>}</div>
    {navigation}
    {panel.filter && <label className="collection-filter"><span>Status</span><select aria-label={panel.kind === "history" ? "Filter task runs" : "Filter saved guidance"} value={panel.filter.value} disabled={busy} onChange={(event) => void action({ action: "filter", text: event.target.value })}>{panel.filter.options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>}
    {!!toolbar.length && <div className="panel-toolbar" role="toolbar" aria-label={`${title} actions`}>{toolbar.filter((hint) => !administration.includes(hint)).map(toolbarButton)}{!!administration.length && <details className="panel-more"><summary>{modelCatalog ? "Manage providers" : "More actions"}<ChevronDown size={15} /></summary><div role="group" aria-label={modelCatalog ? "Provider administration" : "Additional actions"}>{administration.map(toolbarButton)}</div></details>}</div>}
    {localFilter && <div className="panel-filter"><div className="panel-search" role="search"><Search size={16} /><input type="search" aria-label={knowledgeLibrary ? "Filter loaded knowledge" : "Filter models"} value={filter} maxLength={256} onChange={(event) => setFilter(event.target.value)} placeholder={knowledgeLibrary ? "Filter loaded titles, text, or scope…" : "Filter by provider or model…"} autoFocus />{filter && <button className="small-button" onClick={() => setFilter("")}>Clear</button>}</div><p className="collection-count" role="status">{displayedRows.length} of {rows.length} {knowledgeLibrary ? "loaded items · up to 100 available in this view" : "models"}</p>{knowledgeLibrary && <details className="collection-search-help"><summary>Search beyond these items<ChevronDown size={14} /></summary><p>In a conversation, use <code>/memory search your query</code> for saved facts or <code>/note search your query</code> for notes. This filter only checks the items loaded above.</p></details>}</div>}
    {panel.kind === "picker" && <form className="panel-search" onSubmit={(event) => { event.preventDefault(); void action({ action: "query", text: query }); }}><Search size={16} /><input aria-label={`Search ${panel.title.toLowerCase()}`} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search…" autoFocus /><button className="small-button" type="submit">Search</button>{query && <button className="small-button" type="button" onClick={() => { setQuery(""); void action({ action: "query", text: "" }); }}>Clear</button>}</form>}
    {panel.notice && empty?.detail !== panel.notice && !(empty && /^No (?:learnings|retained Work|recent results)/.test(panel.notice)) && <p className="panel-notice" role="status">{panel.notice}</p>}
    <div className="workflow-content">
      {!!panel.summary?.length && <PanelFacts fields={panel.summary} summary />}
      {!!panel.rows?.length && <div className={hasInputs ? "form-fields" : "panel-rows"}>
        {displayedRows.map(({ row, index }) => (refresh && !row.input && row.label.toLowerCase() === "refresh" || panel.kind === "confirmation" && !row.input && primary.some((hint) => hint.key === "enter" && hint.label.toLowerCase() === row.label.toLowerCase())) ? null : row.input ? <PanelField key={`${index}-${row.label}`} row={row} panelID={panel.id} group={row.group !== panel.rows?.[index - 1]?.group ? row.group : undefined} value={fields[index] ?? row.input.value ?? ""} disabled={busy} change={(value) => setFields((current) => ({ ...current, [index]: value }))} /> : <div className="panel-row-wrap" key={`${index}-${row.label}`}>
          <button ref={index === displayedSelected ? selectedRow : undefined} className="panel-row" title={row.label} data-row={index} aria-current={index === displayedSelected ? "true" : undefined} aria-pressed={panel.kind === "effort" ? row.state === "current" : undefined} onKeyDown={(event) => rowKeys(event, index)} onClick={() => panel.kind === "status-line" ? keyAction("space", index) : void action({ action: hasInputs ? "select" : "activate", row: index, fields })}><div><strong>{row.label.split(" · ←")[0]}</strong>{row.state && <span className="badge" data-tone={row.tone}>{row.state_label || row.state}</span>}</div>{row.metadata || row.source || row.updated_at ? <div className="panel-row-meta">{row.source && <span title={row.source}>{workspaceName(row.source, rows.map(({ row }) => row.source || ""))}</span>}{row.metadata?.map((field, position) => <span key={position} title={field.label} data-tone={field.tone}>{field.value}</span>)}{row.updated_at && <time dateTime={row.updated_at} title={new Date(row.updated_at).toLocaleString()}>{shortDate(row.updated_at)}</time>}</div> : row.detail && <p className={knowledgeLibrary ? "knowledge-preview" : undefined}>{row.detail}</p>}{!hasInputs && row.affordance !== "action" && (panel.kind === "effort" ? row.state === "current" ? <Check className="panel-row-chevron accent" size={18} /> : <Circle className="panel-row-chevron" size={16} /> : <ChevronRight className="panel-row-chevron" size={16} />)}</button>
          {hasInputs && <div className="row-adjust"><button className="icon-button" aria-label={`Previous ${row.label}`} onClick={() => keyAction("left", index)}><ChevronLeft size={16} /></button><button className="small-button" onClick={() => keyAction("space", index)}>Change</button><button className="icon-button" aria-label={`Next ${row.label}`} onClick={() => keyAction("right", index)}><ChevronRight size={16} /></button></div>}
        </div>)}
      </div>}
      {empty && EmptyIcon && <div className="panel-empty"><EmptyIcon size={28} /><h2>{empty.title}</h2><p>{empty.detail}</p></div>}
      <PanelBlocks panel={panel} hasInputs={hasInputs} empty={!!empty} />
      {!empty && !panel.rows?.length && !panel.blocks?.length && !panel.notice && <p className="empty-hint">Nothing here yet.</p>}
    </div>
    {(!!primary.length || panel.footer) && <footer className="workflow-footer"><div className="flex flex-wrap gap-2">{primary.map((hint) => <button className={`button ${hint.tone === "error" ? "danger" : hint.key !== "esc" ? "primary" : ""}`} key={hint.id} disabled={busy} onClick={() => keyAction(hint.key)}>{hint.label}<kbd>{shortcutLabel(hint.key, hint.shortcut)}</kbd></button>)}</div>{keyboardFooter ? <details className="keyboard-shortcuts"><summary>Keyboard shortcuts<ChevronDown size={14} aria-hidden="true" /></summary><p>{panel.footer}</p></details> : panel.footer && <p>{panel.footer}</p>}</footer>}
  </section>;
}
