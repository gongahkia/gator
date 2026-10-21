import { Trash2, X } from "lucide-react";
import type { DraftRequest, DraftSnapshot, SavedDraft } from "./bridge";
import { useEffect, useState } from "react";
import { useModalFocus } from "./useModalFocus";
import type { PresenceRef } from "./motion";

export function DraftManager({ open, presenceRef, saved, issue, manage, restore, close }: { open: boolean; presenceRef: PresenceRef; saved: DraftSnapshot; issue: string; manage(action: DraftRequest): Promise<DraftSnapshot>; restore(draft: SavedDraft): void; close(): void }) {
  const focus = useModalFocus(open);
  const [busy, setBusy] = useState(false);
  const [enabled, setEnabled] = useState(saved.enabled);
  useEffect(() => setEnabled(saved.enabled), [saved.enabled]);
  const act = async (request: DraftRequest) => { setBusy(true); try { await manage(request); } catch { setEnabled(saved.enabled); } finally { setBusy(false); } };
  return <div ref={presenceRef} className="modal-backdrop" data-open={open} inert={!open} aria-hidden={!open} onClick={close}><section ref={focus} className="draft-manager" role="dialog" aria-modal="true" aria-labelledby="drafts-title" onClick={(event) => event.stopPropagation()} onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); } }}>
    <header><h2 id="drafts-title">Saved drafts</h2><button className="icon-button" aria-label="Close saved drafts" onClick={close}><X size={18} /></button></header>
    <label className="draft-preference"><input type="checkbox" checked={enabled} disabled={busy} onChange={(event) => { setEnabled(event.target.checked); void act({ action: "configure", enabled: event.target.checked }); }} /><span>Keep unsent messages on this device</span></label>
    <p className="muted">Composer text is saved locally for up to seven days. Turning this off removes saved copies. Drafts are not encrypted or synced; credential forms are never included.</p>
    {issue && <p className="composer-error" role="alert">{issue}</p>}
    <div className="saved-draft-list">{saved.drafts.length ? saved.drafts.map((draft) => <article key={draft.context}><div><strong>{draft.title}</strong><small title={draft.source}>{draft.source}</small><p>{draft.text}</p></div><div className="saved-draft-actions"><button className="small-button" disabled={busy} onClick={() => { restore(draft); close(); }}>Add to current message</button><button className="icon-button" disabled={busy} aria-label={`Delete draft for ${draft.title}`} onClick={() => void act({ action: "remove", context: draft.context })}><Trash2 size={16} /></button></div></article>) : <p className="empty-hint">No saved drafts.</p>}</div>
    <footer><p className="muted">Adding a draft keeps your current text. Nothing is sent.</p><button className="button" disabled={busy || !saved.drafts.length && !issue} onClick={() => void act({ action: "clear" })}>Clear saved drafts</button></footer>
  </section></div>;
}
