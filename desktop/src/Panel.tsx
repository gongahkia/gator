import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { ArrowLeft, BookOpen, ChevronDown, ChevronLeft, ChevronRight, Inbox, ListChecks, MessageSquare, Plus, RefreshCw, Search, Target } from "lucide-react";
import type { BackendPanel, PanelAction } from "../../ui/src/protocol";
import type { DesktopFrame } from "./bridge";

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
  const [busy, setBusy] = useState(false);
  const selected = panel.selected ?? 0;
  const selectedRow = useRef<HTMLButtonElement>(null);
  const formIdentity = `${panel.kind}:${panel.title}:${panel.rows?.map((row) => row.label).join("|")}`;
  useEffect(() => { setFields({}); }, [formIdentity]);
  useEffect(() => { setQuery(panel.query ?? ""); }, [panel.query]);
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
    // Native form values are submitted together. Go still validates, reviews,
    // and commits the action; secrets never return in its display projection.
    if (key === "enter" && hasInputs && row === undefined) row = (panel.rows?.length ?? 1) - 1;
    void action({ action: "key", row, fields: key === "esc" ? undefined : fields }, key);
  };
  const hints = (panel.hints ?? []).flatMap((hint) => shortcutKeys(hint.shortcut).map((key, i, keys) => ({ key, label: keys.length > 1 ? `${hint.action} (${key})` : hint.action, shortcut: hint.shortcut, id: `${hint.shortcut}-${i}` }))).filter((hint) => !["up", "down", "tab", "shift+tab", "ctrl+u"].includes(hint.key));

  const empty = emptyState(panel);
  const EmptyIcon = empty?.icon;
  const rowActivation = new Set(["select", "continue", "expand", "details", "choose", "open", "set effort"]);
  const visibleHints = hints.filter((hint) => {
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
  const keyboardFooter = !hasInputs && panel.kind !== "confirmation" && /^(Esc\b|↑|↓|Type to filter|Tab\b)/i.test(panel.footer ?? "");
  const supportingBlock = (block: NonNullable<BackendPanel["blocks"]>[number]) => !hasInputs && panel.kind !== "confirmation" && (
    ["Storage", "Retrieval", "Chat processing", "How to read this"].includes(block.title ?? "") ||
    panel.title === "Saved knowledge" && ["Work context", "Observed source references"].includes(block.title ?? "")
  );
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
      if (event.defaultPrevented || event.repeat || busy || event.metaKey || event.altKey || !(event.target instanceof HTMLElement)) return;
      if (event.target.closest('input, textarea, [contenteditable], [role="dialog"], [role="alertdialog"]')) return;
      const key = event.ctrlKey ? `ctrl+${event.key.toLowerCase()}` : event.key.toLowerCase();
      if (["escape", "enter", " "].includes(key)) return; // Native buttons and App own these.
      if (hints.some((hint) => hint.key === key)) { event.preventDefault(); keyAction(key); }
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  });

  return <section className="workflow" data-kind={panel.kind} aria-label={title}>
    <div className="workflow-heading"><button className="icon-button" aria-label="Back to conversation" disabled={busy} onClick={() => keyAction("esc")}><ArrowLeft size={18} /></button><div><h1>{title}</h1>{subtitle && <p>{subtitle}</p>}</div>{panel.kind === "confirmation" && <span className="badge" data-tone="warning">Review</span>}</div>
    {navigation}
    {!!toolbar.length && <div className="panel-toolbar" role="toolbar" aria-label={`${title} actions`}>{toolbar.map((hint) => hint === refresh ? <button className="icon-button" aria-label={hint.label} title={`${hint.label} (${hint.shortcut})`} key={hint.id} disabled={busy} onClick={() => keyAction(hint.key)}><RefreshCw size={17} /></button> : <button className={`button ${creates(hint) ? "primary" : ""}`} key={hint.id} disabled={busy} onClick={() => keyAction(hint.key)}>{creates(hint) && <Plus size={16} />}{hint.label}<kbd>{hint.shortcut}</kbd></button>)}</div>}
    {panel.kind === "picker" && <form className="panel-search" onSubmit={(event) => { event.preventDefault(); void action({ action: "query", text: query }); }}><Search size={16} /><input aria-label={`Search ${panel.title.toLowerCase()}`} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search…" autoFocus /><button className="small-button" type="submit">Search</button>{query && <button className="small-button" type="button" onClick={() => { setQuery(""); void action({ action: "query", text: "" }); }}>Clear</button>}</form>}
    {panel.notice && empty?.detail !== panel.notice && !(empty && /^No (?:learnings|retained Work|recent results)/.test(panel.notice)) && <p className="panel-notice" role="status">{panel.notice}</p>}
    <div className="workflow-content">
      {!!panel.rows?.length && <div className={hasInputs ? "form-fields" : "panel-rows"}>
        {panel.rows.map((row, index) => refresh && !row.input && row.label.toLowerCase() === "refresh" ? null : row.input ? <label className="field" key={`${index}-${row.label}`}><span>{row.label}</span>{row.input.multiline ? <textarea aria-label={row.label} value={fields[index] ?? row.input.value ?? ""} maxLength={row.input.limit} placeholder={row.input.placeholder} onChange={(event) => setFields({ ...fields, [index]: event.target.value })} rows={3} /> : <input aria-label={row.label} type={row.input.secret ? "password" : "text"} autoComplete="off" value={fields[index] ?? row.input.value ?? ""} maxLength={row.input.limit} placeholder={row.input.placeholder} onChange={(event) => setFields({ ...fields, [index]: event.target.value })} />}{row.input.secret && <small>Leave empty to keep the stored credential.</small>}</label> : <div className="panel-row-wrap" key={`${index}-${row.label}`}>
          <button ref={index === selected ? selectedRow : undefined} className="panel-row" data-row={index} aria-current={index === selected ? "true" : undefined} onKeyDown={(event) => rowKeys(event, index)} onClick={() => panel.kind === "status-line" ? keyAction("space", index) : void action({ action: hasInputs ? "select" : "activate", row: index, fields })}><div><strong>{row.label.split(" · ←")[0]}</strong>{row.state && <span className="badge" data-tone={row.tone}>{row.state}</span>}</div>{row.detail && <p>{row.detail}</p>}{!hasInputs && <ChevronRight className="panel-row-chevron" size={16} />}</button>
          {hasInputs && <div className="row-adjust"><button className="icon-button" aria-label={`Previous ${row.label}`} onClick={() => keyAction("left", index)}><ChevronLeft size={16} /></button><button className="small-button" onClick={() => keyAction("space", index)}>Change</button><button className="icon-button" aria-label={`Next ${row.label}`} onClick={() => keyAction("right", index)}><ChevronRight size={16} /></button></div>}
        </div>)}
      </div>}
      {empty && EmptyIcon && <div className="panel-empty"><EmptyIcon size={28} /><h2>{empty.title}</h2><p>{empty.detail}</p></div>}
      {panel.blocks?.filter((block) => !(empty && panel.kind === "followup" && block.title === "Start with one useful loop")).map((block, i) => supportingBlock(block) ? <details className="panel-block panel-disclosure" key={block.title || i}><summary>{block.title}<ChevronDown size={16} aria-hidden="true" /></summary><div className="disclosure-content">{(block.lines ?? []).map((line, j) => <p className="literal" key={j}>{line || " "}</p>)}</div></details> : <section className="panel-block" key={block.title || i}>{block.title && <h2>{block.title}</h2>}{(block.lines ?? []).map((line, j) => <p className="literal" key={j}>{line || " "}</p>)}</section>)}
      {!empty && !panel.rows?.length && !panel.blocks?.length && !panel.notice && <p className="empty-hint">Nothing here yet.</p>}
    </div>
    {(!!primary.length || panel.footer) && <footer className="workflow-footer"><div className="flex flex-wrap gap-2">{primary.map((hint) => <button className={`button ${hint.key !== "esc" ? "primary" : ""}`} key={hint.id} disabled={busy} onClick={() => keyAction(hint.key)}>{hint.label}<kbd>{hint.shortcut}</kbd></button>)}</div>{keyboardFooter ? <details className="keyboard-shortcuts"><summary>Keyboard shortcuts<ChevronDown size={14} aria-hidden="true" /></summary><p>{panel.footer}</p></details> : panel.footer && <p>{panel.footer}</p>}</footer>}
  </section>;
}
