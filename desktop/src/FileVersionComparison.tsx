import { useEffect, useState } from "react";
import { Download } from "lucide-react";
import type { FileComparison, FileReference, FileView } from "./fileProtocol";
import "./file-comparison.css";

export function FileVersionComparison({ view, reference, context, busy, exportVersion }: {
  view: FileView; reference: FileReference; context: string; busy: boolean; exportVersion(version: string): void;
}) {
  const selected = view.versions.findIndex(version => version.id === view.revision);
  const [from, setFrom] = useState(view.versions[Math.max(0, selected - 1)].id);
  const [to, setTo] = useState(selected > 0 ? view.revision : view.current);
  const [result, setResult] = useState<FileComparison>(), [issue, setIssue] = useState("");
  const referenceKey = JSON.stringify(reference);
  useEffect(() => {
    let active = true; setResult(undefined); setIssue("");
    void window.gator!.files<FileComparison>({ action: "compare", context, ref: JSON.parse(referenceKey), base: from, version: to }).then(value => { if (active) setResult(value); }).catch(error => { if (active) setIssue(error instanceof Error ? error.message : "The saved versions could not be compared."); });
    return () => { active = false; };
  }, [from, to, context, referenceKey]);
  const options = view.versions.map((version, i) => <option key={version.id} value={version.id}>Version {i + 1} · {version.author}</option>);
  return <section className="file-comparison" aria-label={`Compare versions of ${view.name}`}>
    <div className="file-comparison-choices">
      <div><label>From version<select autoFocus aria-label="Compare from version" value={from} disabled={busy} onChange={event => setFrom(event.target.value)}>{options}</select></label><button className="icon-button" title="Export before version" disabled={busy} aria-label="Export before version" onClick={() => exportVersion(from)}><Download size={14} /></button></div>
      <div><label>To version<select aria-label="Compare to version" value={to} disabled={busy} onChange={event => setTo(event.target.value)}>{options}</select></label><button className="icon-button" title="Export after version" disabled={busy} aria-label="Export after version" onClick={() => exportVersion(to)}><Download size={14} /></button></div>
    </div>
    <div className="file-comparison-body">
      {issue ? <p role="alert">{issue}</p> : !result ? <p role="status">Comparing saved versions…</p> : <>
        <p className="file-comparison-status" role="status">{result.identical ? "The file bytes are identical." : result.truncated ? "Partial comparison" : result.changes.length ? `${result.changes.length} changed ${result.changes.length === 1 ? "section" : "sections"}` : "No supported content differences"}</p>
        {result.notices.map((notice, i) => <p className="file-comparison-notice" key={i}>{notice}</p>)}
        {result.changes.map((change, i) => <section className="file-change" key={i} aria-label={change.location}>
          <h4>{change.location}</h4>
          <div data-change="before"><strong>Before · Version {result.from.number}</strong><pre>{change.before || (change.beforeLines ? "(Empty line)" : "(No text)")}</pre></div>
          <div data-change="after"><strong>After · Version {result.to.number}</strong><pre>{change.after || (change.afterLines ? "(Empty line)" : "(No text)")}</pre></div>
        </section>)}
      </>}
    </div>
  </section>;
}
