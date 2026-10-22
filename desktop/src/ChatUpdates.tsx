import { useEffect, useRef, useState } from "react";
import { Check, CornerUpLeft, RotateCcw } from "lucide-react";
import type { BackendMessage, ChatUpdate } from "./protocol";
import type { DesktopFrame } from "./bridge";

type TimelineItem = { kind: "message"; message: BackendMessage; index: number } | { kind: "update"; update: ChatUpdate };
const time = (value?: string) => Math.max(0, Date.parse(value ?? "") || 0);

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

export function ChatUpdateCard({ update, conversation, send, reply, disabled }: { update: ChatUpdate; conversation: string; send(frame: DesktopFrame): Promise<void>; reply(update: ChatUpdate): void; disabled: boolean }) {
  const element = useRef<HTMLElement>(null);
  const [busy, setBusy] = useState(false);
  const [issue, setIssue] = useState("");
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
  const summary = update.summary.length > 240 ? `${update.summary.slice(0, 240).trimEnd()}…` : update.summary;
  return <article ref={element} className="chat-update" data-update-id={update.id} data-resolved={update.resolved} aria-label={`Follow-up: ${update.title}`}>
    <header><span className="chat-update-state">{update.resolved ? <><Check size={14} />Done</> : !update.read ? "New update" : "Follow-up"}</span><time dateTime={update.created_at}>{new Date(update.created_at).toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}</time></header>
    <h3>{update.title}</h3>
    {!update.resolved && <p className="literal">{summary}</p>}
    <details><summary>Details</summary><div className="chat-update-details"><p className="literal">{update.summary}</p>{update.next_step && <p className="literal">Next step: {update.next_step}</p>}{!!update.evidence?.length && <ul aria-label="Supporting sources">{update.evidence.map((reference, index) => <li key={index}>{reference}</li>)}</ul>}<dl><dt>Check result</dt><dd>{update.status}</dd>{update.job_id && <><dt>Responsibility</dt><dd>{update.job_id}</dd></>}<dt>Update</dt><dd>{update.id}</dd></dl></div></details>
    <footer><button className="small-button" disabled={disabled || busy} onClick={() => reply(update)}><CornerUpLeft size={15} />Reply</button>{!update.read && <button className="small-button" disabled={disabled || busy} onClick={() => void act("read")}>Mark read</button>}<button className="small-button" disabled={disabled || busy} onClick={() => void act(update.resolved ? "reopen" : "resolve")}>{update.resolved ? <RotateCcw size={15} /> : <Check size={15} />}{update.resolved ? "Reopen" : "Done"}</button></footer>
    {issue && <p className="composer-error" role="alert">{issue}</p>}
  </article>;
}
