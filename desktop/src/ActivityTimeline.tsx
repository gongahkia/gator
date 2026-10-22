import { Check, ChevronDown, CircleAlert, CircleDashed, LoaderCircle, Wrench } from "lucide-react";
import { useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import type { ActivityEntry, BackendActivity } from "./protocol";
import { Markdown } from "./Result";

function duration(trace: BackendActivity) {
  const milliseconds = Date.parse(trace.finished_at || "") - Date.parse(trace.started_at);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return "";
  const seconds = Math.floor(milliseconds / 1000);
  if (seconds < 1) return "<1s";
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

function toolLabel(name = "") {
  const labels: Record<string, string> = { read_file: "Read a file", list_files: "Listed files", search_files: "Searched files", write_artifact: "Prepared a file", web_search: "Searched the web", web_fetch: "Read a web page", delegate_agents: "Consulted specialists", knowledge_read: "Read saved knowledge" };
  return labels[name] || name.replace(/[_.-]+/g, " ").replace(/^./, (letter) => letter.toUpperCase()) || "Tool activity";
}

function Indicator({ state }: { state: ActivityEntry["state"] }) {
  return state === "running" ? <LoaderCircle size={14} className="spin" /> : state === "failed" ? <CircleAlert size={14} /> : state === "stopped" ? <CircleDashed size={14} /> : <Check size={14} />;
}

function Disclosure({ label, icon, children, className = "", tone, follow = false }: { label: ReactNode; icon?: ReactNode; children: ReactNode; className?: string; tone?: string; follow?: boolean }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const scroll = useRef<HTMLDivElement>(null);
  const latest = useRef(true);
  useLayoutEffect(() => { if (open && follow && latest.current && scroll.current) scroll.current.scrollTop = scroll.current.scrollHeight; }, [children, open, follow]);
  return <div className={`activity-disclosure ${className}`} data-open={open} data-tone={tone}>
    <button className="activity-toggle" aria-expanded={open} aria-controls={id} onClick={() => setOpen(!open)}>{icon}<span>{label}</span><ChevronDown size={14} className="activity-chevron" /></button>
    <div className="activity-collapse" inert={!open} aria-hidden={!open}><div><div ref={scroll} id={id} className="activity-disclosure-content" onScroll={(event) => { const el = event.currentTarget; latest.current = el.scrollHeight - el.clientHeight - el.scrollTop < 24; }}>{children}</div></div></div>
  </div>;
}

function Tools({ entries }: { entries: ActivityEntry[] }) {
  const failed = entries.filter((entry) => entry.state === "failed").length;
  const active = entries.some((entry) => entry.state === "running");
  const state = active ? "running" : failed ? "failed" : entries.some((entry) => entry.state === "stopped") ? "stopped" : "completed";
  const label = entries.length === 1 ? toolLabel(entries[0].tool) : `${active ? "Using" : "Used"} ${entries.length} tools`;
  return <Disclosure follow={active} className="activity-tools" icon={<Indicator state={state} />} tone={failed ? "warning" : undefined} label={<><span className={active ? "activity-shimmer" : undefined}>{label}</span>{failed > 0 && <small>{failed} failed</small>}</>}>
    <ol className="activity-tool-list">{entries.map((entry) => <li key={entry.id} data-tone={entry.state === "failed" ? "warning" : undefined}><Wrench size={13} /><span>{toolLabel(entry.tool)}</span><span className="activity-tool-state"><Indicator state={entry.state} />{{ running: "Running", completed: "Finished", failed: "Failed", stopped: "Unfinished" }[entry.state]}</span></li>)}</ol>
  </Disclosure>;
}

function Entries({ trace }: { trace: BackendActivity }) {
  const groups: ActivityEntry[][] = [];
  for (const entry of trace.entries) {
    if (entry.kind === "text" && !entry.text?.trim()) continue;
    const previous = groups.at(-1);
    if (entry.kind === "tool" && previous?.[0].kind === "tool") previous.push(entry);
    else groups.push([entry]);
  }
  return <div className="activity-entries">{groups.map((entries) => {
    const entry = entries[0];
    if (entry.kind === "tool") return <Tools key={entry.id} entries={entries} />;
    if (entry.kind === "status") return <p key={entry.id} className="activity-note"><Check size={14} />{entry.text}</p>;
    const content = <div className="activity-narration"><Markdown text={entry.text || ""} /></div>;
    return <div key={entry.id} className="activity-text">{content}</div>;
  })}{trace.truncated && <p className="activity-note">Some activity was shortened to keep this conversation responsive.</p>}</div>;
}

export function ActivityHistory({ trace }: { trace: BackendActivity }) {
  const elapsed = duration(trace);
  const failed = trace.entries.filter((entry) => entry.state === "failed").length;
  const label = trace.state === "stopped" ? "Stopped" : trace.state === "failed" ? "Work ended" : "Worked";
  const summary = <>{label}{elapsed ? `${trace.state === "completed" ? " for" : " after"} ${elapsed}` : ""}{failed > 0 && <small>{failed} failed {failed === 1 ? "tool" : "tools"}</small>}</>;
  if (!trace.entries.some((entry) => entry.kind !== "text" || entry.text?.trim()) && !trace.truncated) {
    if (trace.state === "completed") return null;
    return <p className="activity-summary"><Indicator state={trace.state} /><span>{summary}</span></p>;
  }
  return <Disclosure className="activity-history" icon={<Indicator state={trace.state === "completed" ? "completed" : trace.state === "failed" ? "failed" : "stopped"} />} tone={trace.state === "failed" ? "warning" : undefined} label={summary}><Entries trace={trace} /></Disclosure>;
}

export function LiveActivity({ trace, status, elapsed, attention }: { trace?: BackendActivity; status: string; elapsed?: string; attention: boolean }) {
  return <section className="live-activity" aria-label="Gator activity">
    {trace && <Entries trace={trace} />}
    <div className="run-progress" role="status"><span className="gator-working-mark" data-attention={attention} aria-hidden="true">🐊</span><span className={attention ? undefined : "activity-shimmer"}>{status}</span><span className="elapsed">{elapsed}</span></div>
  </section>;
}
