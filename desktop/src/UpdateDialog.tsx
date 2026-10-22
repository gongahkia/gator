import { Download, ExternalLink, LoaderCircle, RotateCw, X } from "lucide-react";
import { useState } from "react";
import type { UpdateRequest, UpdateState } from "./bridge";
import { useModalFocus } from "./useModalFocus";
import type { PresenceRef } from "./motion";

export function UpdateDialog({ open, presenceRef, state, blocked, close }: { open: boolean; presenceRef: PresenceRef; state: UpdateState; blocked: string; close(): void }) {
  const focus = useModalFocus(open);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const phase = state.phase;
  const pending = busy || ["checking", "downloading", "installing"].includes(phase);
  const act = async (action: UpdateRequest["action"]) => {
    setBusy(true); setError("");
    try { await window.gator!.updates({ action }); }
    catch (error) { setError(error instanceof Error ? error.message : "The update could not complete."); }
    finally { setBusy(false); }
  };
  const label = { manual: "Manual updates", idle: "Ready when you are", checking: "Checking for updates…", current: "You’re up to date", available: "A new version is available", downloading: "Downloading your update…", ready: "Your update is ready", installing: "Preparing to restart…", error: "Couldn’t complete the update" }[phase];
  return <div ref={presenceRef} className="modal-backdrop" data-open={open} inert={!open} aria-hidden={!open} onClick={() => { if (phase !== "installing") close(); }}>
    <section ref={focus} className="draft-manager update-dialog" role="dialog" aria-modal="true" aria-labelledby="update-title" onClick={(event) => event.stopPropagation()} onKeyDown={(event) => { event.stopPropagation(); if (event.key === "Escape" && phase !== "installing") { event.preventDefault(); close(); } }}>
      <header><h2 id="update-title">About &amp; updates</h2><button className="icon-button" aria-label="Close updates" disabled={phase === "installing"} onClick={close}><X size={18} /></button></header>
      <div className="update-identity"><span aria-hidden="true">🐊</span><div><strong>Gator</strong><p>Version {state.installed}</p></div></div>
      <div className="update-status" role="status" aria-live="polite"><strong>{pending && <LoaderCircle className="spin" size={15} />}{label}</strong>{state.version && <span>Version {state.version}</span>}</div>
      {phase === "downloading" && <div className="update-progress"><progress aria-label="Update download" value={state.percent ?? 0} max={100} /><span>{state.percent ?? 0}%</span></div>}
      {state.reason && <p className="muted">{state.reason}</p>}
      {phase !== "manual" && <p className="muted">Checks and downloads start when you ask. A downloaded update waits for your review before restarting.</p>}
      {phase === "ready" && <p className="muted">Saved conversations and retained drafts stay on this device. Work and background checks do not automatically resume.</p>}
      {phase === "ready" && blocked && <p className="update-blocked" role="status">{blocked}</p>}
      {error && <p className="composer-error" role="alert">{error}</p>}
      <footer><button className="small-button" onClick={() => void act("releases")} disabled={busy || phase === "installing"}>Release notes<ExternalLink size={13} /></button>
        {phase !== "manual" && <button className="button primary" disabled={pending || phase === "ready" && !!blocked} onClick={() => void act(phase === "ready" ? "install" : phase === "available" ? "download" : "check")}>
          {phase === "ready" ? <RotateCw size={15} /> : phase === "available" ? <Download size={15} /> : null}{phase === "ready" ? "Restart and install…" : phase === "available" ? "Download update" : phase === "checking" ? "Checking…" : phase === "downloading" ? "Downloading…" : phase === "installing" ? "Preparing…" : "Check for updates"}
        </button>}
      </footer>
    </section>
  </div>;
}
