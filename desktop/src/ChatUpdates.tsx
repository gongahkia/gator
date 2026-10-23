import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Check, CircleAlert, CirclePause, CornerUpLeft, RotateCcw } from "lucide-react";
import type { BackendMessage, BackendState, ChatUpdate } from "./protocol";
import type { DesktopFrame } from "./bridge";
import "./chat-updates.css";

type TimelineItem = { kind: "message"; message: BackendMessage; index: number } | { kind: "update"; update: ChatUpdate };
const time = (value?: string) => Math.max(0, Date.parse(value ?? "") || 0);
type FeedbackRating = NonNullable<ChatUpdate["rating"]> | "clear";
type FeedbackAction = (update: ChatUpdate, rating: FeedbackRating) => Promise<void>;
const feedbackLabels = { useful: "Useful", noise: "Unnecessary", wrong: "Incorrect" } as const;

// All cards share one pending operation: bridge results contain one exact host
// acknowledgement, and a transport send alone is not evidence of a saved rating.
export function useChatUpdateFeedback(state: BackendState | undefined, send: (frame: DesktopFrame) => Promise<void>, connectionError?: string) {
  const pending = useRef<{ id: string; conversation: string; timeout: ReturnType<typeof setTimeout>; resolve(): void; reject(error: Error): void } | null>(null);
  const [busy, setBusy] = useState(false);
  const finish = useCallback((id: string, error?: string) => {
    const current = pending.current;
    if (!current || current.id !== id) return;
    pending.current = null; clearTimeout(current.timeout); setBusy(false);
    if (error) current.reject(new Error(error)); else current.resolve();
  }, []);
  useEffect(() => {
    const current = pending.current, result = state?.chat_update_result;
    if (!current) return;
    if (current.conversation !== state?.conversation_id) finish(current.id, "Return to the original chat to check whether your feedback was saved.");
    else if (result?.request_id === current.id) finish(current.id, result.error);
  }, [state?.conversation_id, state?.chat_update_result, finish]);
  useEffect(() => {
    if (connectionError && pending.current) finish(pending.current.id, "Couldn’t confirm your feedback. Reconnect and check its saved selection before trying again.");
  }, [connectionError, finish]);
  useEffect(() => () => {
    const current = pending.current; pending.current = null;
    if (current) { clearTimeout(current.timeout); current.reject(new Error("Conversation closed. Check its saved feedback when you return.")); }
  }, []);
  const act: FeedbackAction = useCallback((update, rating) => new Promise<void>((resolve, reject) => {
    const conversation = state?.conversation_id;
    if (pending.current) { reject(new Error("Wait for the previous feedback change.")); return; }
    if (!conversation || !update.can_rate || update.status !== "completed") { reject(new Error("Only a completed update can receive this feedback.")); return; }
    const id = crypto.randomUUID();
    const timeout = setTimeout(() => finish(id, "Couldn’t confirm your feedback. Check its saved selection before trying again."), 15_000);
    pending.current = { id, conversation, timeout, resolve, reject }; setBusy(true);
    void send({ type: "update", update: { conversation_id: conversation, id: update.id, action: "rate", version: update.version, rating, rating_version: update.rating_version, request_id: id } }).catch(error => finish(id, error instanceof Error ? error.message : String(error)));
  }), [state?.conversation_id, send, finish]);
  return { act, busy };
}

// Retain message/approval indexes while interleaving durable updates. An
// explicit reply always follows its card, even if a machine clock changed.
export function chatTimeline(messages: BackendMessage[], updates: ChatUpdate[]): TimelineItem[] {
  const remaining = [...updates].sort((a, b) => time(a.created_at) - time(b.created_at) || a.id.localeCompare(b.id));
  const result: TimelineItem[] = [];
  let at = 0;
  messages.forEach((message, index) => {
    at = Math.max(at, time(message.created_at), time(message.activity?.finished_at || message.activity?.started_at));
    while (remaining.length && time(remaining[0].created_at) <= at) result.push({ kind: "update", update: remaining.shift()! });
    const replied = remaining.findIndex(update => update.id === message.reply_to_update_id);
    if (replied >= 0) result.push({ kind: "update", update: remaining.splice(replied, 1)[0] });
    result.push({ kind: "message", message, index });
  });
  result.push(...remaining.map(update => ({ kind: "update" as const, update })));
  return result;
}

export function ChatUpdateCard({ update, conversation, send, reply, disabled, feedback, feedbackBusy }: { update: ChatUpdate; conversation: string; send(frame: DesktopFrame): Promise<void>; reply(update: ChatUpdate): void; disabled: boolean; feedback: FeedbackAction; feedbackBusy: boolean }) {
  const element = useRef<HTMLElement>(null);
  const [busy, setBusy] = useState(false);
  const [issue, setIssue] = useState("");
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const [feedbackSaving, setFeedbackSaving] = useState(false);
  const [feedbackIssue, setFeedbackIssue] = useState("");
  const [feedbackNotice, setFeedbackNotice] = useState("");
  const feedbackRegion = useId();
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => {
    if (!element.current || update.delivered || disabled) return;
    let visible = false, sent = false, active = true;
    const acknowledge = () => {
      if (!visible || document.hidden || sent || !active) return;
      sent = true;
      void send({ type: "update", update: { conversation_id: conversation, id: update.id, action: "delivered", version: update.version } }).catch(() => { if (active) setIssue("Couldn’t record delivery. Your update is still saved."); });
    };
    const observer = new IntersectionObserver(entries => { visible = entries.some(entry => entry.isIntersecting); acknowledge(); });
    observer.observe(element.current);
    document.addEventListener("visibilitychange", acknowledge);
    return () => { active = false; observer.disconnect(); document.removeEventListener("visibilitychange", acknowledge); };
  }, [conversation, update.id, update.delivered, disabled, send]);
  const act = async (action: "read" | "resolve" | "reopen") => {
    setBusy(true); setIssue("");
    try { await send({ type: "update", update: { conversation_id: conversation, id: update.id, action, version: update.version } }); }
    catch (error) { setIssue(error instanceof Error ? error.message : "Couldn’t save this change."); }
    finally { setBusy(false); }
  };
  const rate = async (rating: FeedbackRating) => {
    setFeedbackSaving(true); setFeedbackIssue(""); setFeedbackNotice("");
    try {
      await feedback(update, rating);
      if (mounted.current) setFeedbackNotice(rating === "clear" ? "Feedback cleared." : "Feedback saved.");
    } catch (error) {
      if (mounted.current) setFeedbackIssue(error instanceof Error ? error.message : "Couldn’t save your feedback.");
    } finally { if (mounted.current) setFeedbackSaving(false); }
  };
  const summary = update.summary.length > 240 ? `${update.summary.slice(0, 240).trimEnd()}…` : update.summary;
  const failed = update.status === "failed", interrupted = update.status === "interrupted";
  const checkIssue = failed || interrupted;
  return <article ref={element} className="chat-update" data-update-id={update.id} data-resolved={update.resolved} aria-label={`Follow-up: ${update.title}`}>
    <header><span className="chat-update-state" data-check-issue={checkIssue || undefined}>{checkIssue ? <>{failed ? <CircleAlert size={14} aria-hidden="true" /> : <CirclePause size={14} aria-hidden="true" />}{failed ? "Couldn’t check" : "Check interrupted"}{update.resolved && <span className="chat-update-handled">· Marked done</span>}</> : update.resolved ? <><Check size={14} aria-hidden="true" />Done</> : !update.read ? "New update" : "Follow-up"}</span><time dateTime={update.created_at}>{new Date(update.created_at).toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}</time></header>
    <h3>{update.title}</h3>
    {!update.resolved && <p className="literal">{summary}</p>}
    {checkIssue && <p className="chat-update-status-note">{failed ? "This check didn’t complete successfully." : "This check stopped before it finished."} Earlier results don’t confirm that nothing changed.</p>}
    <details><summary>Details</summary><div className="chat-update-details"><p className="literal">{update.summary}</p>{update.next_step && <p className="literal">Next step: {update.next_step}</p>}{!!update.evidence?.length && <ul aria-label="Supporting sources">{update.evidence.map((reference, index) => <li key={index}>{reference}</li>)}</ul>}<dl><dt>Check result</dt><dd>{update.status}</dd>{update.job_id && <><dt>Responsibility</dt><dd>{update.job_id}</dd></>}<dt>Update</dt><dd>{update.id}</dd></dl></div></details>
    <footer><button className="small-button" disabled={disabled || busy || feedbackBusy} onClick={() => reply(update)}><CornerUpLeft size={15} />Reply</button>{!update.read && <button className="small-button" disabled={disabled || busy || feedbackBusy} onClick={() => void act("read")}>Mark read</button>}<button className="small-button" disabled={disabled || busy || feedbackBusy} onClick={() => void act(update.resolved ? "reopen" : "resolve")}>{update.resolved ? <RotateCcw size={15} /> : <Check size={15} />}{update.resolved ? "Reopen" : "Done"}</button>{update.can_rate && !checkIssue && <button className="small-button" aria-expanded={feedbackOpen} aria-controls={feedbackRegion} onClick={() => setFeedbackOpen(open => !open)}>Feedback{update.rating ? ` · ${feedbackLabels[update.rating]}` : ""}</button>}</footer>
    {update.can_rate && !checkIssue && feedbackOpen && <section id={feedbackRegion} className="chat-update-feedback" aria-label="Update feedback"><p>How was this update? Feedback is saved for review; it doesn’t change your checks automatically.</p><div className="chat-update-feedback-choices" role="group" aria-label="Rate this update">{(Object.keys(feedbackLabels) as NonNullable<ChatUpdate["rating"]>[]).map(rating => <button className="small-button" key={rating} aria-pressed={update.rating === rating} disabled={disabled || busy || feedbackBusy} onClick={() => void rate(rating)}>{feedbackLabels[rating]}</button>)}{update.rating && <button className="small-button" disabled={disabled || busy || feedbackBusy} onClick={() => void rate("clear")}>Clear feedback</button>}</div>{feedbackSaving && <p role="status">Saving feedback…</p>}{feedbackNotice && <p role="status">{feedbackNotice}</p>}{feedbackIssue && <p className="composer-error" role="alert">{feedbackIssue}</p>}</section>}
    {issue && <p className="composer-error" role="alert">{issue}</p>}
  </article>;
}
