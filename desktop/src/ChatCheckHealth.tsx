import { ChevronDown, CircleAlert, ListChecks } from "lucide-react";
import type { BackendState } from "./protocol";
import "./chat-check-health.css";

const needsAttention = (state: string) => ["offline", "couldn't check", "daily limit", "shared limit", "settings changed; waiting for check"].includes(state);
const when = (value: string) => new Date(value).toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });

export function ChatCheckHealth({ state, disabled, manage }: { state: BackendState; disabled: boolean; manage(): void }) {
  const checks = state.chat_checks ?? [];
  if (!checks.length && !state.chat_checks_issue) return null;
  const attention = checks.filter(check => needsAttention(check.state)).length;
  return <details className="chat-check-health">
    <summary>{attention || state.chat_checks_issue ? <CircleAlert size={15} aria-hidden="true" /> : <ListChecks size={15} aria-hidden="true" />}<span>{state.chat_checks_issue ? "Check health unavailable" : attention ? `${attention} ${attention === 1 ? "check needs" : "checks need"} attention` : `${checks.length} configured ${checks.length === 1 ? "check" : "checks"}`}</span><ChevronDown className="chat-check-health-arrow" size={14} aria-hidden="true" /></summary>
    <div className="chat-check-health-content">
      {state.chat_checks_issue && <p role="status">{state.chat_checks_issue}</p>}
      <ul>{checks.map(check => <li key={check.id}>
        <div><strong>{check.title}</strong><span data-attention={needsAttention(check.state) || undefined}>{check.state}</span></div>
        <p>{check.last_success ? `Last successful check for current settings: ${when(check.last_success)}.` : "No successful check recorded for current settings."}{check.next_due ? ` Next scheduled: ${when(check.next_due)}.` : " No check is currently scheduled."}</p>
        {check.unsuccessful > 0 && <p>{check.unsuccessful} consecutive unsuccessful {check.unsuccessful === 1 ? "check" : "checks"}.{check.issue && ` ${check.issue}`}</p>}
        {needsAttention(check.state) && <p>{check.next_action.replaceAll("/usage", "Usage & limits").replaceAll("/inbox", "Updates").replace(/\.$/, "")}. Checks depend on this computer being on.</p>}
        {check.gap_from && check.gap_to && <p>Observed supervisor gap: {when(check.gap_from)}–{when(check.gap_to)}. This does not establish what changed while checks were unavailable.</p>}
      </li>)}</ul>
      <button className="small-button" disabled={disabled} onClick={manage}>Manage checks</button>
    </div>
  </details>;
}
