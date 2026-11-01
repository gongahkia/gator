import { useState } from "react";
import { X } from "lucide-react";
import { useModalFocus } from "./useModalFocus";

export function CommandArguments({ name, description, run, close }: { name: string; description?: string; run(text: string): Promise<void>; close(): void }) {
  const focus = useModalFocus();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [issue, setIssue] = useState("");
  return <div className="modal-backdrop" data-open="true"><section className="draft-manager" ref={focus} role="dialog" aria-modal="true" aria-label={`Arguments for /${name}`} onKeyDown={event => { if (event.key === "Escape") { event.stopPropagation(); if (!busy) close(); } }}>
    <header><h2>/{name}</h2><button className="icon-button" aria-label="Cancel command" disabled={busy} onClick={close}><X size={18} /></button></header>
    <p>{description}</p><p className="muted">Your message draft and attachments stay in the conversation. Gator still validates and reviews this command.</p>
    <form onSubmit={event => { event.preventDefault(); if (busy) return; setBusy(true); setIssue(""); void run(`/${name}${text ? ` ${text}` : ""}`).then(close).catch(error => setIssue(error instanceof Error ? error.message : "The command was not confirmed. Inspect its current state before trying again.")).finally(() => setBusy(false)); }}>
      <label>Arguments<input className="command-arguments-input" aria-label={`Arguments for /${name}`} value={text} maxLength={32768} disabled={busy} onChange={event => setText(event.target.value)} autoComplete="off" /></label>
      {issue && <p className="composer-error" role="alert">{issue}</p>}<footer><button className="button" type="button" disabled={busy} onClick={close}>Cancel</button><button className="button primary" disabled={busy} type="submit">{busy ? "Running command…" : "Continue"}</button></footer>
    </form>
  </section></div>;
}
