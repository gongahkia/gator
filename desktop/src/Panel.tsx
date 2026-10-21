import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ChevronLeft, ChevronRight, Search } from "lucide-react";
import type { BackendPanel, PanelAction } from "../../ui/src/protocol";
import type { DesktopFrame } from "./bridge";

function shortcutKeys(shortcut: string) {
  return shortcut.toLowerCase().replaceAll(" ", "").split("/").map((key) => ({ "←": "left", "→": "right", "↑": "up", "↓": "down", "f1/esc": "f1" })[key] ?? key).filter((key) => /^(enter|esc|up|down|left|right|tab|shift\+tab|space|ctrl\+[ru]|f1|[a-z0-9])$/.test(key));
}

export function Panel({ panel, send }: { panel: BackendPanel; send(frame: DesktopFrame): Promise<void> }) {
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

  return <section className="workflow" aria-label={panel.title}>
    <div className="workflow-heading"><button className="icon-button" aria-label="Back to conversation" onClick={() => keyAction("esc")}><ArrowLeft size={18} /></button><div><h1>{panel.title}</h1>{panel.subtitle && <p>{panel.subtitle}</p>}</div>{panel.kind === "confirmation" && <span className="badge" data-tone="warning">Review</span>}</div>
    {panel.kind === "picker" && <form className="panel-search" onSubmit={(event) => { event.preventDefault(); void action({ action: "query", text: query }); }}><Search size={16} /><input aria-label={`Search ${panel.title.toLowerCase()}`} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Filter…" autoFocus /><button className="small-button" type="submit">Search</button>{query && <button className="small-button" type="button" onClick={() => { setQuery(""); void action({ action: "query", text: "" }); }}>Clear</button>}</form>}
    {panel.notice && <p className="panel-notice" role="status">{panel.notice}</p>}
    <div className="workflow-content">
      {!!panel.rows?.length && <div className={hasInputs ? "form-fields" : "panel-rows"}>
        {panel.rows.map((row, index) => row.input ? <label className="field" key={`${index}-${row.label}`}><span>{row.label}</span>{row.input.multiline ? <textarea aria-label={row.label} value={fields[index] ?? row.input.value ?? ""} maxLength={row.input.limit} placeholder={row.input.placeholder} onChange={(event) => setFields({ ...fields, [index]: event.target.value })} rows={3} /> : <input aria-label={row.label} type={row.input.secret ? "password" : "text"} autoComplete="off" value={fields[index] ?? row.input.value ?? ""} maxLength={row.input.limit} placeholder={row.input.placeholder} onChange={(event) => setFields({ ...fields, [index]: event.target.value })} />}{row.input.secret && <small>Leave empty to keep the stored credential.</small>}</label> : <div className="panel-row-wrap" key={`${index}-${row.label}`}>
          <button ref={index === selected ? selectedRow : undefined} className="panel-row" aria-current={index === selected ? "true" : undefined} onClick={() => panel.kind === "status-line" ? keyAction("space", index) : void action({ action: hasInputs ? "select" : "activate", row: index, fields })}><div><strong>{row.label.split(" · ←")[0]}</strong>{row.state && <span className="badge" data-tone={row.tone}>{row.state}</span>}</div>{row.detail && <p>{row.detail}</p>}</button>
          {hasInputs && <div className="row-adjust"><button className="icon-button" aria-label={`Previous ${row.label}`} onClick={() => keyAction("left", index)}><ChevronLeft size={16} /></button><button className="small-button" onClick={() => keyAction("space", index)}>Change</button><button className="icon-button" aria-label={`Next ${row.label}`} onClick={() => keyAction("right", index)}><ChevronRight size={16} /></button></div>}
        </div>)}
      </div>}
      {panel.kind === "context" && panel.title === "Saved knowledge" && !panel.rows?.length && <p className="empty-hint">No saved knowledge yet.</p>}
      {panel.blocks?.map((block, i) => <section className="panel-block" key={i}>{block.title && <h2>{block.title}</h2>}{(block.lines ?? []).map((line, j) => <p className="literal" key={j}>{line || " "}</p>)}</section>)}
      {!panel.rows?.length && !panel.blocks?.length && !panel.notice && <p className="empty-hint">Nothing to review here yet.</p>}
    </div>
    <footer className="workflow-footer"><div className="flex flex-wrap gap-2">{hints.map((hint) => <button className={`button ${hint.key === "enter" || hint.key === "ctrl+r" ? "primary" : ""}`} key={hint.id} disabled={busy} onClick={() => keyAction(hint.key)}>{hint.label}<kbd>{hint.shortcut}</kbd></button>)}</div>{panel.footer && <p>{panel.footer}</p>}</footer>
  </section>;
}
