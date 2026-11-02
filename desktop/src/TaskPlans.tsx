import { useCallback, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { ClipboardList, LoaderCircle, Plus, X } from "lucide-react";
import type { BackendState, PlanContent, TaskPlanAction, TaskPlanReview, TaskPlanSummary } from "./protocol";
import type { DesktopFrame } from "./bridge";
import { useModalFocus } from "./useModalFocus";

type Change = Omit<TaskPlanAction, "request_id">;
const effects = [
  ["read", "Read files and research"], ["draft", "Stage deliverables"],
  ["code", "Edit isolated code candidates"], ["checks", "Run coding checks"],
  ["connected_write", "Propose connected-service changes"], ["settings", "Change Gator settings or saved memory"],
  ["browser", "Use reviewed browser interactions"],
] as const;

// Transport acknowledgement is not plan acceptance. Match the owner result,
// never retry a decision automatically, and forget pending reads on navigation.
function usePlanAction(state: BackendState, send: (frame: DesktopFrame) => Promise<void>, connectionIssue?: string) {
  const [busy, setBusy] = useState(false);
  const pending = useRef<{ id: string; timer: ReturnType<typeof setTimeout>; resolve(value?: TaskPlanReview): void; reject(error: Error): void } | undefined>(undefined);
  const finish = useCallback((issue?: string, review?: TaskPlanReview) => {
    const request = pending.current;
    if (!request) return;
    clearTimeout(request.timer); pending.current = undefined; setBusy(false);
    if (issue) request.reject(new Error(issue)); else request.resolve(review);
  }, []);
  useEffect(() => {
    if (pending.current && state.task_plan_result?.request_id === pending.current.id) finish(state.task_plan_result.error, state.task_plan_result.review);
  }, [state.task_plan_result, finish]);
  useEffect(() => { if (connectionIssue) finish("Plan status is unconfirmed. Reconnect and inspect the retained task before making another decision."); }, [connectionIssue, finish]);
  useEffect(() => () => {
    const request = pending.current; pending.current = undefined;
    if (request) { clearTimeout(request.timer); request.reject(new Error("The conversation changed. No decision was resent.")); }
  }, []);
  const act = useCallback((change: Change) => new Promise<TaskPlanReview | undefined>((resolve, reject) => {
    if (pending.current) { reject(new Error("Wait for the current plan request.")); return; }
    const id = crypto.randomUUID();
    const timer = setTimeout(() => finish("Plan status is unconfirmed. Close this review and inspect the retained task. Do not assume work was started or resend automatically."), 15000);
    pending.current = { id, timer, resolve, reject }; setBusy(true);
    void send({ type: "task_plan", task_plan: { ...change, request_id: id } }).catch(() => finish("Plan status is unconfirmed. Inspect the retained task after reconnecting; no automatic retry was sent."));
  }), [send, finish]);
  return { act, busy };
}

function planStatus(plan: TaskPlanSummary) {
  if (plan.state === "discarded") return "Dismissed";
  if (plan.state === "interrupted") return "Interrupted · review required";
  if (plan.state === "failed" || plan.state === "stopped") return "Work ended · results may be partial";
  if (plan.state === "completed") return "Work ended · inspect results";
  if (plan.phase === "preparing") return "Preparing proposal · implementation paused";
  if (plan.phase === "review") return "Waiting for your review";
  if (plan.phase === "approved") return "Reviewed · not yet dispatched";
  return "Work admitted · observe activity below";
}

export function TaskPlans({ state, send, disabled, connectionIssue, onBusyChange, onReviewChange, stage, inspectWork }: {
  state: BackendState; send(frame: DesktopFrame): Promise<void>; disabled: boolean; connectionIssue?: string;
  onBusyChange(busy: boolean): void; onReviewChange(open: boolean): void; stage(text: string): void; inspectWork(run: string): void;
}) {
  const { act, busy } = usePlanAction(state, send, connectionIssue);
  const [selected, setSelected] = useState<string>();
  const [review, setReview] = useState<TaskPlanReview>();
  const [issue, setIssue] = useState("");
  const sequence = useRef(0), mounted = useRef(true);
  const returnTo = useRef<HTMLElement | null>(null);
  const current = state.task_plans?.find(plan => plan.id === selected);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; sequence.current++; }; }, []);
  useEffect(() => { onBusyChange(busy); return () => onBusyChange(false); }, [busy, onBusyChange]);
  useEffect(() => { onReviewChange(!!selected); return () => onReviewChange(false); }, [selected, onReviewChange]);
  const restoreFocus = () => requestAnimationFrame(() => { if (returnTo.current?.isConnected) returnTo.current.focus({ preventScroll: true }); });
  const close = () => { if (!busy) { sequence.current++; setSelected(undefined); setReview(undefined); setIssue(""); restoreFocus(); } };
  const load = async (plan: TaskPlanSummary) => {
    const ticket = ++sequence.current;
    returnTo.current = document.activeElement as HTMLElement | null;
    setSelected(plan.id); setReview(undefined); setIssue("");
    try {
      const value = await act({ conversation_id: state.conversation_id!, task_id: plan.id, revision: plan.revision, digest: plan.digest, action: "inspect" });
      if (!mounted.current || ticket !== sequence.current) return;
      if (!value || value.id !== plan.id || value.revision !== plan.revision || value.digest !== plan.digest) throw new Error("This plan changed. Close this review and inspect the current card.");
      setReview(value);
    } catch (error) { if (mounted.current && ticket === sequence.current) setIssue(error instanceof Error ? error.message : "The retained plan is unavailable."); }
  };
  const change = async (action: Change["action"], content?: PlanContent) => {
    if (!review || !current || disabled || busy) return;
    const ticket = sequence.current;
    setIssue("");
    try {
      await act({ conversation_id: state.conversation_id!, task_id: review.id, revision: review.revision, digest: review.digest, action, ...(content ? { content } : {}) });
      if (!mounted.current || ticket !== sequence.current) return;
      setReview(undefined); setSelected(undefined);
      restoreFocus();
    } catch (error) { if (mounted.current && ticket === sequence.current) setIssue(error instanceof Error ? error.message : "Could not confirm the plan decision."); }
  };
  if (!state.task_plans?.length) return null;
  return <section className="task-plans" aria-label="Reviewed task plans">
    {state.task_plans.map(plan => <details className="task-plan-card" key={plan.id} open={plan.state === "queued" || plan.state === "running" || plan.state === "interrupted"}>
      <summary><ClipboardList size={17}/><span><strong>{plan.outcome || plan.task}</strong><small>{planStatus(plan)} · revision {plan.revision}</small></span></summary>
      <div className="task-plan-body"><p className="task-plan-task">{plan.task}</p>
        {!!plan.steps?.length && <ol aria-label="Proposed steps">{plan.steps.map(step => <li key={step.id}>{step.text}</li>)}</ol>}
        <p className="task-plan-note">Proposed steps are intentions, not verified completion. {plan.work_id ? "Actual results and tool activity belong to the linked Work run." : "Preparing or reviewing does not start implementation."}</p>
        {plan.work_id && <><small className="task-plan-run">Work run: {plan.work_id}</small>{state.messages.some(message => message.role === "Gator" && message.run_id === plan.work_id) && <button className="small-button" onClick={() => inspectWork(plan.work_id!)}>Inspect run activity &amp; results</button>}</>}
        <button className="small-button" disabled={disabled || busy || plan.phase === "preparing" && plan.state === "queued"} onClick={() => void load(plan)}>{plan.phase === "review" && plan.state === "queued" ? "Review plan" : "Inspect retained plan"}</button>
      </div>
    </details>)}
    {selected && createPortal(<PlanReview key={`${selected}:${review?.revision ?? "loading"}`} review={review} current={current} issue={issue} busy={busy} disabled={disabled || state.running} close={close} change={change} stage={text => { sequence.current++; setSelected(undefined); setReview(undefined); setIssue(""); stage(text); }}/>, document.body)}
  </section>;
}

function PlanReview({ review, current, issue, busy, disabled, close, change, stage }: {
  review?: TaskPlanReview; current?: TaskPlanSummary; issue: string; busy: boolean; disabled: boolean;
  close(): void; change(action: Change["action"], content?: PlanContent): Promise<void>; stage(text: string): void;
}) {
  const focus = useModalFocus(), title = useId();
  const [editing, setEditing] = useState(false), [draft, setDraft] = useState<PlanContent | undefined>(() => review?.content ? structuredClone(review.content) : undefined);
  const changed = !!review && (!current || review.id !== current.id || review.revision !== current.revision || review.digest !== current.digest || review.state !== current.state || review.phase !== current.phase);
  const blocked = disabled || busy || changed;
  useEffect(() => {
    const key = (event: KeyboardEvent) => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); } };
    window.addEventListener("keydown", key, true); return () => window.removeEventListener("keydown", key, true);
  }, [busy, close]);
  const editable = review?.state === "queued" && review.phase === "review";
  const display = editing ? draft : review?.content;
  const valid = !!draft && !!draft.outcome.trim() && new TextEncoder().encode(draft.outcome).length <= 4096 && draft.steps.length > 0 && draft.steps.length <= 24 && draft.steps.every(step => step.text.trim() && new TextEncoder().encode(step.text).length <= 1024) && draft.effects.length > 0;
  return <div className="modal-backdrop branch-backdrop"><section className="task-plan-dialog" role="dialog" aria-modal="true" aria-labelledby={title} ref={focus}>
    <header><h2 id={title}><ClipboardList size={20}/>Review task plan</h2><button className="icon-button" aria-label="Close plan review" disabled={busy} onClick={close}><X size={18}/></button></header>
    {!review && !issue && <p role="status"><LoaderCircle size={16} className="spin"/> Loading the exact retained proposal…</p>}
    {review && <>
      <p className="task-plan-note">This review belongs to this chat and task, revision {review.revision}. Approval starts only this reviewed intent. Later exact action, command and artifact reviews still apply.</p>
      <h3>Original request</h3><pre className="task-plan-original">{review.task}</pre>
      {!!review.input_names?.length && <><h3>Attached inputs</h3><ul>{review.input_names.map((name,index) => <li key={index}>{name}</li>)}</ul><p className="task-plan-note">Files were selected with this request. They have not been inspected merely to prepare this proposal.</p></>}
      {display ? <>
        <div className="task-plan-field"><label htmlFor={editing ? `${title}-outcome` : undefined}>Intended outcome</label>{editing ? <textarea id={`${title}-outcome`} value={display.outcome} maxLength={4096} onChange={event => setDraft({ ...display, outcome: event.target.value })}/> : <p>{display.outcome}</p>}</div>
        <h3>Proposed steps</h3><ol className="task-plan-steps" aria-label="Proposed steps">{display.steps.map((step, index) => <li key={step.id}>{editing ? <div><label className="sr-only" htmlFor={`${title}-${step.id}`}>Step {index + 1}</label><textarea id={`${title}-${step.id}`} value={step.text} maxLength={1024} onChange={event => setDraft({ ...display, steps: display.steps.map(value => value.id === step.id ? { ...value, text: event.target.value } : value) })}/><button className="icon-button" aria-label={`Remove step ${index + 1}`} disabled={display.steps.length === 1} onClick={() => setDraft({ ...display, steps: display.steps.filter(value => value.id !== step.id) })}><X size={15}/></button></div> : step.text}</li>)}</ol>
        {editing && <button className="small-button" disabled={display.steps.length >= 24} onClick={() => setDraft({ ...display, steps: [...display.steps, { id: crypto.randomUUID(), text: "" }] })}><Plus size={14}/>Add step</button>}
        <fieldset className="task-plan-effects"><legend>Foreseeable effects</legend>{effects.filter(([id]) => editing || display.effects.includes(id)).map(([id, label]) => <label key={id}>{editing && <input type="checkbox" checked={display.effects.includes(id)} onChange={event => setDraft({ ...display, effects: event.target.checked ? [...display.effects, id] : display.effects.filter(effect => effect !== id) })}/>}<span>{label}</span></label>)}</fieldset>
        <fieldset className="task-plan-effects"><legend>Connected sources</legend>{review.sources.length ? review.sources.filter(id => editing || display.sources.includes(id)).map(id => <label key={id}>{editing && <input type="checkbox" checked={display.sources.includes(id)} onChange={event => setDraft({ ...display, sources: event.target.checked ? [...display.sources, id] : display.sources.filter(source => source !== id) })}/>}<span>{id}</span></label>) : <p>No connected sources were selected for this request.</p>}{!editing && !display.sources.length && !!review.sources.length && <p>This plan uses none of the selected connections.</p>}</fieldset>
      </> : <p>No proposal was retained. No plan approval is available.</p>}
      <h3>Frozen execution limits</h3><p>Manager: {review.budget.max_steps > 0 ? `up to ${review.budget.max_steps} steps` : "configured default"}. Each coding assignment: {review.budget.code_max_steps > 0 ? `up to ${review.budget.code_max_steps} steps` : "configured default"}. Total cost and specialist count are not estimated here. Normal configured usage limits still apply.</p>
      <p className="task-plan-note">Code requires the read, code and checks categories together. Dynamic coding integration setup and desktop control are unsupported in plan-first mode. New steering requires a new reviewed task. Plans do not grant external terminal authority.</p>
      {review.state === "interrupted" && <p role="status">This task will not restart automatically. Check retained activity, receipts and any partial output before creating a new request. Previously dispatched actions may have unknown outcomes.</p>}
    </>}
    {changed && <p className="composer-error" role="alert">This task changed. Close the review and inspect its current card before making a decision.</p>}
    {issue && <p className="composer-error" role="alert">{issue}</p>}
    <footer>
      {review && (review.state === "queued" || review.state === "interrupted") && <button className="small-button" disabled={blocked} onClick={() => void change("cancel")}>{review.state === "interrupted" ? "Dismiss retained request" : "Cancel plan"}</button>}
      {review?.state === "interrupted" && <button className="small-button" disabled={blocked} onClick={() => stage(review.task)}>Use request in composer</button>}
      {editable && !editing && <button className="small-button" disabled={blocked} onClick={() => { setDraft(structuredClone(review!.content!)); setEditing(true); }}>Edit plan</button>}
      {editing && <><button className="small-button" disabled={busy} onClick={() => setEditing(false)}>Discard edits</button><button className="button" disabled={blocked || !valid} onClick={() => void change("edit", draft)}>Save revision for review</button></>}
      {editable && !editing && <button className="button" disabled={blocked} onClick={() => void change("approve")}>Approve plan &amp; start</button>}
      {review?.state === "queued" && review.phase === "approved" && <button className="button" disabled={blocked} onClick={() => void change("start")}>Start reviewed work</button>}
    </footer>
  </section></div>;
}
