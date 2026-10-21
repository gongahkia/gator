import { useState } from "react";
import { ArrowDown, ArrowLeft, ArrowUp, RotateCcw } from "lucide-react";
import type { BackendPanel } from "../../ui/src/protocol";
import type { DesktopFrame } from "./bridge";

export function StatusLinePanel({ panel, send }: { panel: BackendPanel; send(frame: DesktopFrame): Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const action = async (key: string, row?: number) => {
    if (!panel.id || busy) return;
    setBusy(true);
    try { await send({ type: "panel", key, panel: { id: panel.id, action: "key", row } }); }
    catch { /* The application presents transport errors. */ }
    finally { setBusy(false); }
  };
  const rows = (panel.rows ?? []).map((row, index) => ({ row, index })).sort((a, b) => (a.row.position || 1000 + a.index) - (b.row.position || 1000 + b.index));
  const enabled = rows.filter(({ row }) => row.checked).length;
  return <section className="workflow" aria-label="Status line settings">
    <header className="workflow-heading"><button className="icon-button" aria-label="Cancel status line changes" disabled={busy} onClick={() => void action("esc")}><ArrowLeft size={18} /></button><div><h1>Terminal status line</h1><p>Choose the information shown below messages in Gator’s terminal interface.</p></div></header>
    <div className="panel-toolbar"><label className="status-line-toggle"><input type="checkbox" role="switch" checked={!!panel.toggle?.checked} disabled={busy} onChange={() => void action("v")} /><span>Show in Terminal</span></label><button className="small-button" disabled={busy} onClick={() => void action("r")}><RotateCcw size={15} />Restore defaults</button></div>
    {panel.notice && <p className="panel-notice" role="status">{panel.notice}</p>}
    <div className="workflow-content"><p className="collection-count">{enabled} items selected · changes apply when you save</p><div className="status-line-options">{rows.map(({ row, index }) => <div className="status-line-option" key={index}>
      <label><input type="checkbox" aria-label={row.label} checked={!!row.checked} disabled={busy} onChange={() => void action("space", index)} /><span><strong>{row.label}</strong><small>{row.detail}</small></span></label>
      <div className="status-line-order"><span>{row.position || "—"}</span><button className="icon-button" aria-label={`Move ${row.label} earlier`} disabled={busy || !row.checked || row.position === 1} onClick={() => void action("left", index)}><ArrowUp size={16} /></button><button className="icon-button" aria-label={`Move ${row.label} later`} disabled={busy || !row.checked || row.position === enabled} onClick={() => void action("right", index)}><ArrowDown size={16} /></button></div>
    </div>)}</div>{panel.blocks?.map((block, index) => <section className="status-line-preview" key={index}><h2>{block.title}</h2><p>{block.lines?.join(" · ")}</p></section>)}</div>
    <footer className="workflow-footer"><div className="flex gap-2"><button className="button primary" disabled={busy} onClick={() => void action("enter")}>Save changes</button><button className="button" disabled={busy} onClick={() => void action("esc")}>Cancel</button></div></footer>
  </section>;
}
