import { useState } from "react";
import { ArrowLeft, ChevronDown, Pause, Play, RefreshCw, SlidersHorizontal } from "lucide-react";
import type { BackendPanel, Usage } from "../../ui/src/protocol";
import type { DesktopFrame } from "./bridge";

const number = new Intl.NumberFormat();
const categoryLabels: Record<string, string> = { chat: "Conversations", work: "Tasks", background: "Background jobs", routing: "Routing", retrieval: "Retrieval", code: "Code" };

function UsageMeter({ title, used, limit, period }: { title: string; used: number; limit: number; period: string }) {
  const remaining = Math.max(0, limit - used);
  const percent = limit > 0 ? Math.min(100, Math.round(remaining / limit * 100)) : undefined;
  return <section className="usage-meter" aria-label={title}>
    <h2>{title}</h2>
    <div className="usage-meter-value">{percent === undefined ? <strong>Uncapped</strong> : <><strong>{percent}%</strong><span>remaining</span></>}</div>
    {percent !== undefined ? <meter aria-label={`${title} remaining`} min={0} max={limit} value={remaining} /> : <div className="uncapped-track" aria-hidden="true" />}
    <div className="usage-meter-caption"><span>{number.format(used)} used{limit > 0 && ` / ${number.format(limit)}`}</span><span>{period}</span></div>
  </section>;
}

export function UsagePanel({ panel, usage, send }: { panel: BackendPanel; usage: Usage; send(frame: DesktopFrame): Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const action = async (key: string) => {
    if (!panel.id || busy) return;
    setBusy(true);
    try { await send({ type: "panel", key, panel: { id: panel.id, action: "key" } }); }
    catch { /* The application presents the transport error. */ }
    finally { setBusy(false); }
  };
  const exhausted = (usage.daily_request_limit > 0 && usage.requests >= usage.daily_request_limit) || (usage.daily_token_limit > 0 && usage.tokens >= usage.daily_token_limit);
  const atCapacity = usage.concurrent_limit > 0 && usage.active_requests >= usage.concurrent_limit;
  const status = usage.paused ? "Paused" : exhausted ? "Daily limit reached" : atCapacity ? "At capacity" : "Active";
  const reset = new Date(usage.reset_at);
  const largestCategory = Math.max(1, ...usage.categories.map((category) => category.requests));

  return <section className="usage-page" aria-label="Usage & limits">
    <div className="usage-page-inner">
      <header className="usage-heading"><button className="icon-button" aria-label="Back to conversation" disabled={busy} onClick={() => void action("esc")}><ArrowLeft size={19} /></button><h1>Usage & limits</h1><button className="icon-button usage-refresh" aria-label="Refresh usage" disabled={busy} onClick={() => void action("r")}><RefreshCw size={17} /></button></header>
      <div className="usage-section-title"><div><h2>Daily usage</h2><p>All Gator activity on this computer</p></div><span className="usage-status" data-tone={usage.paused || exhausted || atCapacity ? "warning" : "success"}><span className="status-dot" data-tone={usage.paused || exhausted || atCapacity ? "warning" : "success"} />{status}</span></div>
      {panel.notice && <p className="panel-notice" role="status">{panel.notice}</p>}
      <div className="usage-meters">
        <UsageMeter title="Provider requests" used={usage.requests} limit={usage.daily_request_limit} period="Today" />
        <UsageMeter title="Tokens" used={usage.tokens} limit={usage.daily_token_limit} period="Today" />
      </div>
      <p className="usage-reset">Resets <time dateTime={usage.reset_at}>{reset.toLocaleDateString(undefined, { month: "short", day: "numeric", timeZone: "UTC" })}, 00:00 UTC</time></p>
      <section className="usage-settings" aria-label="Usage controls">
        <div className="usage-setting-row"><div><h3>Shared limits</h3><p>Daily requests, tokens, and concurrent requests</p></div><button className="button" disabled={busy} onClick={() => void action("e")}><SlidersHorizontal size={15} />Edit limits</button></div>
        <div className="usage-setting-row"><div><h3>Provider requests</h3><p>{usage.paused ? "New requests are paused across Gator." : "New requests are allowed within your limits."}</p></div><button className="button" disabled={busy} onClick={() => void action("p")}>{usage.paused ? <Play size={15} /> : <Pause size={15} />}{usage.paused ? "Resume requests" : "Pause requests"}</button></div>
        <div className="usage-setting-row"><div><h3>Concurrent requests</h3><p>Requests running at the same time</p></div><span className="usage-setting-value">{number.format(usage.active_requests)} active<span>{usage.concurrent_limit > 0 ? `of ${number.format(usage.concurrent_limit)}` : "Uncapped"}</span></span></div>
      </section>
      <section className="usage-breakdown" aria-labelledby="usage-breakdown-title"><h2 id="usage-breakdown-title">Requests by activity</h2><div className="usage-categories">{usage.categories.map((category) => <div className="usage-category" key={category.name}><span>{categoryLabels[category.name] ?? category.name}</span><div className="category-track" aria-hidden="true"><span style={{ width: `${category.requests / largestCategory * 100}%` }} /></div><strong>{number.format(category.requests)}</strong></div>)}</div></section>
      <details className="usage-accounting"><summary>Usage details<ChevronDown size={16} /></summary><dl><div><dt>Failed attempts</dt><dd>{number.format(usage.failed_requests)}</dd></div><div><dt>Estimated or unreported usage</dt><dd>{number.format(usage.estimated_requests)} requests</dd></div><div><dt>Accounting day</dt><dd>{usage.day} (UTC)</dd></div>{usage.known_cost_usd !== undefined && <div><dt>Provider-reported cost</dt><dd>${usage.known_cost_usd.toFixed(4)}</dd></div>}</dl><p>Daily limits are shared by conversations, tasks, background jobs, routing, retrieval, and Code. Retries count as requests. Existing per-run limits still apply.</p><p>Tokens include reservations until provider usage arrives. Missing usage keeps its reservation, and provider totals can exceed estimates. These controls are not a billing ceiling; cost is incomplete when usage or pricing is missing.</p><p>Pausing stops new provider requests, routing, and embeddings. Current requests may finish, and source-only checks can continue. Saved schedules stay in place.</p></details>
    </div>
  </section>;
}
