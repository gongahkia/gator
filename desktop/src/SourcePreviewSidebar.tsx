import { useEffect, useRef, useState } from "react";
import { FileText, LoaderCircle, X } from "lucide-react";
import type { SourcePreview } from "@gator/application-host/contracts/sourcePreview";
import type { SourceSelection } from "./sourceReferences";
import type { PresenceRef } from "./motion";
import { useModalFocus } from "./useModalFocus";

export function SourcePreviewSidebar({ selection, open, presenceRef, close }: { selection: SourceSelection; open: boolean; presenceRef: PresenceRef; close(): void }) {
  const [loaded, setLoaded] = useState<{key:string; value?:SourcePreview; issue?:string}>();
  const selectionKey=JSON.stringify(selection);
  const current=loaded?.key===selectionKey?loaded:undefined;
  const result=current?.value, issue=current?.issue;
  const [narrow, setNarrow] = useState(() => matchMedia("(max-width: 960px)").matches);
  const root = useModalFocus(open && narrow), cited = useRef<HTMLDivElement>(null);
  useEffect(() => { const media = matchMedia("(max-width: 960px)"); const change = () => setNarrow(media.matches); media.addEventListener("change", change); return () => media.removeEventListener("change", change); }, []);
  useEffect(() => {
    setLoaded(undefined); if (!open) return;
    let active = true;
    const { request: _, ...request } = selection;
    void window.gator!.sourcePreview(request).then(value => { if (active) setLoaded({key:selectionKey,value}); }).catch(error => { if (active) setLoaded({key:selectionKey,issue:error instanceof Error ? error.message : "The retained source could not be opened."}); });
    return () => { active = false; };
  }, [selection, open]);
  useEffect(() => { cited.current?.scrollIntoView({ block: "center" }); }, [result]);
  return <>
    {narrow && open && <div className="work-details-scrim" aria-hidden="true" onClick={close} />}
    <aside className="workspace-panel source-preview-panel" id="work-details-panel" ref={element => { root.current = element; presenceRef(element); }} aria-label="Source preview" role={narrow ? "dialog" : undefined} aria-modal={narrow && open ? true : undefined} data-open={open} inert={!open} aria-hidden={!open} onKeyDown={event => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); } }}>
      <header className="work-details-heading"><h2><FileText size={17} />Source preview</h2><button className="icon-button" aria-label="Close source preview" onClick={close}><X size={16} /></button></header>
      <div className="workspace-content"><h3 className="source-preview-path">{result?.label || selection.path}</h3>
        <p className="work-details-caption">Source snapshot retained for this answer. The original may have changed.</p>
        {!result && !issue && <p role="status"><LoaderCircle size={16} className="spin" />Opening retained source…</p>}
        {issue && <p role="alert" className="composer-error">{issue}</p>}
        {result && <>
          <div className="source-read-summary">{result.cited && <p>Cited: lines {result.cited.start}–{result.cited.end}</p>}{!!result.reads.length && <p>Read by Gator: {result.reads.map(range => `${range.start}–${range.end}${range.endColumn !== undefined ? ` (last line through character ${range.endColumn})` : range.truncated ? " (bounded read)" : ""}`).join(", ")}</p>}</div>
          {result.captured_at && <p className="work-details-caption">Captured {new Date(result.captured_at).toLocaleString()}</p>}
          {result.original_url && <button className="small-button" onClick={()=>void window.gator?.openLink(result.original_url!).catch(()=>{})}>Open original page</button>}
          {result.notice && <p role="status" className="work-details-notice">{result.notice}</p>}
          <div className="source-line-legend"><span>Cited lines</span><span>Read by Gator</span></div>
          <div className="source-lines" aria-label="Retained file contents">{result.text.split("\n").map((text, index) => {
            const line = result.firstLine + index, selected = !!result.cited && line >= result.cited.start && line <= result.cited.end;
            const intervals = result.reads.filter(range=>line>=range.start&&line<=range.end).map(range=>({start:range.start===line?range.startColumn??0:0,end:range.end===line?range.endColumn??text.length:text.length})).sort((a,b)=>a.start-b.start);
            const merged: {start:number;end:number}[]=[];
            for(const interval of intervals){const prior=merged.at(-1);if(prior&&interval.start<=prior.end)prior.end=Math.max(prior.end,interval.end);else merged.push({...interval});}
            const full=merged.length===1&&merged[0].start===0&&merged[0].end>=text.length;
            const fragments: import("react").ReactNode[]=[];let offset=0;
            for(const interval of merged){if(interval.start>offset)fragments.push(text.slice(offset,interval.start));fragments.push(<span key={interval.start} className="source-read-fragment">{text.slice(interval.start,interval.end)}</span>);offset=interval.end;}
            if(offset<text.length)fragments.push(text.slice(offset));
            return <div key={line} ref={line === (result.cited?.start ?? result.reads[0]?.start) ? cited : undefined} data-cited={selected} data-read={full} data-partial-read={!full&&merged.length>0}><span className="source-line-number" aria-hidden="true">{line}</span><code>{fragments.length?fragments:text||" "}</code></div>;
          })}</div>
          {result.truncated && <p className="work-details-caption">Bounded preview around the selected lines · {result.totalLines} lines in the retained file.</p>}
          <details className="output-location"><summary>Evidence identity</summary><p>{result.run}</p><p>{result.sha256}</p></details>
        </>}
      </div>
    </aside>
  </>;
}
