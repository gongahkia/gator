import { useEffect, useRef, useState } from "react";
import { FileText, LoaderCircle, X } from "lucide-react";
import type { SourcePreview } from "@gator/application-host/contracts/sourcePreview";
import type { SourceSelection } from "./sourceReferences";
import type { PresenceRef } from "./motion";
import { useModalFocus } from "./useModalFocus";

export function SourcePreviewSidebar({ selection, open, presenceRef, close }: { selection: SourceSelection; open: boolean; presenceRef: PresenceRef; close(): void }) {
  const [result, setResult] = useState<SourcePreview>(), [issue, setIssue] = useState("");
  const [narrow, setNarrow] = useState(() => matchMedia("(max-width: 960px)").matches);
  const root = useModalFocus(open && narrow), cited = useRef<HTMLDivElement>(null);
  useEffect(() => { const media = matchMedia("(max-width: 960px)"); const change = () => setNarrow(media.matches); media.addEventListener("change", change); return () => media.removeEventListener("change", change); }, []);
  useEffect(() => {
    setResult(undefined); setIssue(""); if (!open) return;
    let active = true;
    const { request: _, ...request } = selection;
    void window.gator!.sourcePreview(request).then(value => { if (active) setResult(value); }).catch(error => { if (active) setIssue(error instanceof Error ? error.message : "The retained source could not be opened."); });
    return () => { active = false; };
  }, [selection, open]);
  useEffect(() => { cited.current?.scrollIntoView({ block: "center" }); }, [result]);
  return <>
    {narrow && open && <div className="work-details-scrim" aria-hidden="true" onClick={close} />}
    <aside className="workspace-panel source-preview-panel" id="work-details-panel" ref={element => { root.current = element; presenceRef(element); }} aria-label="Source preview" role={narrow ? "dialog" : undefined} aria-modal={narrow && open ? true : undefined} data-open={open} inert={!open} aria-hidden={!open} onKeyDown={event => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); } }}>
      <header className="work-details-heading"><h2><FileText size={17} />Source preview</h2><button className="icon-button" aria-label="Close source preview" onClick={close}><X size={16} /></button></header>
      <div className="workspace-content"><h3 className="source-preview-path">{selection.path}</h3>
        <p className="work-details-caption">Original source retained for this answer. Your current file may have changed.</p>
        {!result && !issue && <p role="status"><LoaderCircle size={16} className="spin" />Opening retained source…</p>}
        {issue && <p role="alert" className="composer-error">{issue}</p>}
        {result && <>
          <div className="source-read-summary">{result.cited && <p>Cited: lines {result.cited.start}–{result.cited.end}</p>}{!!result.reads.length && <p>Read by Gator: {result.reads.map(range => `${range.start}–${range.end}${range.truncated ? " (bounded read)" : ""}`).join(", ")}</p>}</div>
          {result.notice && <p role="status" className="work-details-notice">{result.notice}</p>}
          <div className="source-line-legend"><span>Cited lines</span><span>Read by Gator</span></div>
          <div className="source-lines" aria-label="Retained file contents">{result.text.split("\n").map((text, index) => {
            const line = result.firstLine + index, selected = !!result.cited && line >= result.cited.start && line <= result.cited.end;
            const read = result.reads.some(range => line >= range.start && line <= range.end);
            return <div key={line} ref={line === (result.cited?.start ?? result.reads[0]?.start) ? cited : undefined} data-cited={selected} data-read={read}><span className="source-line-number" aria-hidden="true">{line}</span><code>{text || " "}</code></div>;
          })}</div>
          {result.truncated && <p className="work-details-caption">Bounded preview around the selected lines · {result.totalLines} lines in the retained file.</p>}
          <details className="output-location"><summary>Evidence identity</summary><p>{result.run}</p><p>{result.sha256}</p></details>
        </>}
      </div>
    </aside>
  </>;
}
