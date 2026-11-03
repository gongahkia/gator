import { useCallback, useEffect, useRef, useState } from "react";
import { GitBranch, MessageSquare, Square } from "lucide-react";
import type { BackendState, SpecialistAction, SpecialistTask } from "./protocol";
import type { DesktopFrame } from "./bridge";

const statusText: Record<string, string> = {
  queued: "Queued", running: "Working", completed: "Finished", cancelled: "Stopped",
  failed: "Needs review", awaiting_input: "Waiting for input", answering: "Receiving answer",
};

const taskKey = (task: SpecialistTask) => `${task.id}:${task.token}`;
type GuidanceDraft = { task: SpecialistTask; text: string };

export function SpecialistTasks({ state, send, disabled, connectionIssue, onEditingChange, stage }: {
  state: BackendState; send(frame: DesktopFrame): Promise<void>; disabled: boolean; connectionIssue?: string;
  onEditingChange(id: string, editing: boolean): void;
  stage(text: string): void;
}) {
  const [busyTask, setBusyTask] = useState<string>();
  const [drafts, setDrafts] = useState<Record<string, GuidanceDraft>>({});
  const [draftIssue, setDraftIssue] = useState("");
  const retainDraft = useCallback((task: SpecialistTask, text: string) => setDrafts(previous => {
    const next = { ...previous }, key = taskKey(task);
    if (text) next[key] = { task, text }; else delete next[key];
    return next;
  }), []);
  const hasDrafts = Object.keys(drafts).length > 0;
  useEffect(() => {
    onEditingChange("specialist-guidance-drafts", hasDrafts);
    return () => onEditingChange("specialist-guidance-drafts", false);
  }, [hasDrafts, onEditingChange]);
  const lock = useRef<string | undefined>(undefined);
  // Only one control awaits acknowledgement at a time, so React batching of
  // consecutive snapshots cannot lose another card's single result envelope.
  const lease = useCallback((id: string, acquire: boolean) => {
    if (acquire) {
      if (lock.current) return false;
      lock.current = id; setBusyTask(id);
    } else if (lock.current === id) { lock.current = undefined; setBusyTask(undefined); }
    return true;
  }, []);
  const tasks = state.running ? state.specialists ?? [] : [];
  const available = new Set(tasks.filter(task => task.can_guide).map(taskKey));
  const retained = Object.entries(drafts).filter(([key]) => !available.has(key));
  if (!tasks.length && !retained.length) return null;
  return <section className="specialist-tasks" aria-label="Specialists">
    <div className="specialist-heading"><GitBranch size={16} /><span>Specialists</span>{!!tasks.length && <small>{tasks.length}</small>}</div>
    {tasks.map(task => <SpecialistCard key={taskKey(task)} task={task} draft={drafts[taskKey(task)]?.text ?? ""} retainDraft={retainDraft} result={state.specialist_result} send={send} disabled={disabled || !!busyTask && busyTask !== task.id} connectionIssue={connectionIssue} lease={lease} onEditingChange={onEditingChange} />)}
    {retained.map(([key, draft]) => <article key={key} className="specialist-card" aria-label={`Retained guidance for ${draft.task.name.replaceAll("_", " ")}`}>
      <strong>Keep this guidance</strong><p className="specialist-description">{draft.task.name.replaceAll("_", " ")} is no longer available for guidance. Review this text before using it as a follow-up.</p>
      <p className="specialist-draft">{draft.text}</p><div className="specialist-actions"><button className="small-button" disabled={disabled} onClick={() => {
        try { stage(draft.text); retainDraft(draft.task, ""); setDraftIssue(""); }
        catch (error) { setDraftIssue(error instanceof Error ? error.message : "Copy this guidance before leaving."); }
      }}>Move to composer</button><button className="small-button" disabled={disabled} onClick={() => retainDraft(draft.task, "")}>Discard</button></div>
    </article>)}
    {draftIssue && <p className="specialist-issue" role="alert">{draftIssue}</p>}
  </section>;
}

function SpecialistCard({ task, draft, retainDraft, result, send, disabled, connectionIssue, lease, onEditingChange }: {
  task: SpecialistTask; result: BackendState["specialist_result"]; send(frame: DesktopFrame): Promise<void>; disabled: boolean; connectionIssue?: string;
  lease(id: string, acquire: boolean): boolean;
  onEditingChange(id: string, editing: boolean): void;
  draft: string; retainDraft(task: SpecialistTask, text: string): void;
}) {
  const [editing, setEditing] = useState(false);
  const setDraft = (text: string) => retainDraft(task, text);
  const [issue, setIssue] = useState(""), [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false), [stopRequested, setStopRequested] = useState(false);
  const guideButton = useRef<HTMLButtonElement>(null);
  const guidanceForm = useRef<HTMLFormElement>(null);
  const pending = useRef<{ id: string; action: SpecialistAction["action"]; timer: ReturnType<typeof setTimeout> } | undefined>(undefined);
  const identity = `${task.id}:${task.token}`;
  useEffect(() => {
    onEditingChange(identity, editing);
    return () => onEditingChange(identity, false);
  }, [editing, identity, onEditingChange]);
  const finish = (error?: string) => {
    const request = pending.current;
    if (!request) return;
    clearTimeout(request.timer); pending.current = undefined; setBusy(false); lease(task.id, false);
    if (error) { setIssue(error); return; }
    setIssue("");
    if (request.action === "guide") {
      setDraft(""); setEditing(false); setNotice("Guidance queued for the next model step."); requestAnimationFrame(() => guideButton.current?.focus());
    } else { setStopRequested(true); setNotice("Stop requested."); }
  };
  useEffect(() => { if (result?.request_id === pending.current?.id) finish(result?.error); }, [result]);
  useEffect(() => { if (connectionIssue) finish("Action unconfirmed. Inspect the specialist after reconnecting before trying again."); }, [connectionIssue]);
  useEffect(() => { if (!task.can_guide) setEditing(false); }, [task.can_guide]);
  useEffect(() => {
    if (editing) {
      const frame = requestAnimationFrame(() => guidanceForm.current?.scrollIntoView({ block: "nearest", behavior: "instant" }));
      return () => cancelAnimationFrame(frame);
    }
  }, [editing]);
  useEffect(() => () => { if (pending.current) clearTimeout(pending.current.timer); pending.current = undefined; lease(task.id, false); }, [task.id, lease]);
  const act = (action: SpecialistAction["action"]) => {
    if (disabled || pending.current || !task.token || !lease(task.id, true)) return;
    const id = crypto.randomUUID(); setBusy(true); setIssue(""); setNotice("");
    const timer = setTimeout(() => finish("Action unconfirmed. Inspect current activity before trying again. Your guidance is still here."), 15000);
    pending.current = { id, action, timer };
    void send({ type: "specialist", specialist: { id: task.id, token: task.token, request_id: id, action, ...(action === "guide" ? { text: draft.trim() } : {}) } })
      .catch(() => { if (pending.current?.id === id) finish("Action unconfirmed. Inspect the specialist after reconnecting before trying again."); });
  };
  const name = task.name.replaceAll("_", " ");
  return <article className="specialist-card" aria-label={`${name} specialist`}>
    <div className="specialist-card-heading"><strong>{name}</strong><span className="specialist-status" data-state={task.status}>{statusText[task.status] || task.status.replaceAll("_", " ")}</span></div>
    {task.description && <p className="specialist-description">{task.description}</p>}
    {task.assignment && <details className="specialist-assignment"><summary>Assignment</summary><p>{task.assignment}</p></details>}
    {(task.can_guide || task.can_stop) && <div className="specialist-actions">
      {task.can_guide && <button ref={guideButton} className="small-button" disabled={disabled || busy || stopRequested} aria-expanded={editing} onClick={() => setEditing(!editing)}><MessageSquare size={14} />Guide</button>}
      {task.can_stop && <button className="small-button" disabled={disabled || busy || stopRequested} onClick={() => act("stop")}><Square size={13} />{stopRequested ? "Stopping…" : "Stop"}</button>}
    </div>}
    {editing && <form ref={guidanceForm} className="specialist-guidance" onSubmit={event => { event.preventDefault(); act("guide"); }}>
      <label>Guidance for {name}<textarea autoFocus rows={2} value={draft} onChange={event => setDraft(event.target.value)} disabled={busy} placeholder="Focus on the primary sources…" /></label>
      <p>Applied at the next model step if this specialist is still working.</p>
      <div><button className="small-button" type="submit" disabled={disabled || busy || !task.can_guide || stopRequested || !draft.trim() || new TextEncoder().encode(draft.trim()).length > 8192}>{busy ? "Sending…" : "Send guidance"}</button><button className="small-button" type="button" disabled={busy} onClick={() => { setEditing(false); guideButton.current?.focus(); }}>Cancel</button></div>
    </form>}
    {issue && <p className="specialist-issue" role="alert">{issue}</p>}
    {notice && (task.can_guide || task.can_stop) && <p className="specialist-notice" role="status">{notice}</p>}
  </article>;
}
