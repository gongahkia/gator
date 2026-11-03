import { ActivityDetails } from "./ActivityDetails";
import { Check, ChevronDown, CircleAlert, CircleDashed, Clock3, LoaderCircle, Wrench } from "lucide-react";
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
  const labels: Record<string, string> = { code_open: "Opened an isolated coding workspace", code_read_file: "Read code", code_list_files: "Listed code files", code_search_files: "Searched code", code_apply_patch: "Edited staged code", code_run_command: "Ran a coding command", code_git_status: "Checked changed files", code_git_diff: "Inspected the patch", code_stage: "Staged and checked a patch", code_terminal_start: "Started a workspace terminal", code_terminal_write: "Sent approved terminal input", code_terminal_read: "Read terminal output", complete_code: "Implemented and reviewed code", integrate_code: "Checked a frozen code candidate", read_file: "Read a file", list_files: "Listed files", search_files: "Searched files", write_artifact: "Prepared a file", web_search: "Searched the web", web_fetch: "Read a web page", delegate_agents: "Consulted specialists", knowledge_read: "Read saved knowledge", knowledge_search: "Searched saved knowledge", memory_remember: "Saved a memory", memory_forget: "Forgot a memory", chat_search: "Searched earlier chats", chat_read: "Read an earlier chat" };
  Object.assign(labels, { create_agent: "Created a specialist", agent_profiles: "Managed saved specialists", list_agents: "Checked available specialists", start_agent: "Started a specialist", inspect_agent: "Checked specialist progress", await_agent: "Waited for a specialist", steer_agent: "Updated a specialist’s assignment", answer_agent: "Answered a specialist", cancel_agent: "Stopped a specialist" });
  return labels[name] || name.replace(/[_.-]+/g, " ").replace(/^./, (letter) => letter.toUpperCase()) || "Tool activity";
}

function Indicator({ state }: { state: ActivityEntry["state"] }) {
  return state === "waiting" ? <Clock3 size={14} /> : state === "running" ? <LoaderCircle size={14} className="spin" /> : state === "failed" ? <CircleAlert size={14} /> : state === "stopped" ? <CircleDashed size={14} /> : <Check size={14} />;
}

function Disclosure({ label, icon, children, className = "", tone, follow = false, initiallyOpen = false }: { label: ReactNode; icon?: ReactNode; children: ReactNode; className?: string; tone?: string; follow?: boolean; initiallyOpen?: boolean }) {
  const [open, setOpen] = useState(initiallyOpen);
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
  const active = entries.some((entry) => entry.state === "running" || entry.state === "waiting");
  const state = entries.some(entry=>entry.state==="waiting") ? "waiting" : active ? "running" : failed ? "failed" : entries.some((entry) => entry.state === "stopped") ? "stopped" : "completed";
  const label = entries.length === 1 ? toolLabel(entries[0].tool) : `${active ? "Using" : "Used"} ${entries.length} tools`;
  return <div className="activity-tool-group"><Disclosure follow={active} className="activity-tools" icon={<Indicator state={state} />} tone={failed ? "warning" : undefined} label={<><span className={active ? "activity-shimmer" : undefined}>{label}</span>{failed > 0 && <small>{failed} failed</small>}</>}>
    <ol className="activity-tool-list">{entries.map((entry) => <li key={entry.id} data-tone={entry.state === "failed" ? "warning" : undefined}><Wrench size={13} /><span>{toolLabel(entry.tool)}</span><span className="activity-tool-state"><Indicator state={entry.state} />{{ running: "Running", waiting: "Waiting for approval", completed: "Finished", failed: "Failed", stopped: "Unfinished" }[entry.state]}</span>{!!entry.details?.length && <ActivityDetails details={entry.details} />}{entry.partial && <small className="activity-note">Partial details · some observations were shortened</small>}</li>)}</ol>
  </Disclosure>{entries.some(entry=>entry.details?.length) && <ActivityDetails compact details={entries.flatMap(entry=>entry.details??[])}/>}</div>;
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
  return <Disclosure initiallyOpen={trace.entries.some(entry => entry.tool?.startsWith("code_") || entry.tool === "complete_code")} className="activity-history" icon={<Indicator state={trace.state === "completed" ? "completed" : trace.state === "failed" ? "failed" : "stopped"} />} tone={trace.state === "failed" ? "warning" : undefined} label={summary}><Entries trace={trace} /></Disclosure>;
}

export function LiveActivity({ trace, status, elapsed, attention }: { trace?: BackendActivity; status: string; elapsed?: string; attention: boolean }) {
  return <section className="live-activity" aria-label="Gator activity">
    {trace && <Entries trace={trace} />}
    <div className="run-progress" role="status"><span className="gator-working-mark" data-attention={attention} aria-hidden="true">🐊</span><span className={attention ? undefined : "activity-shimmer"}>{status}</span><span className="elapsed">{elapsed}</span></div>
  </section>;
}
