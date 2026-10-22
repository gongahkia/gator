import { useEffect, useState } from "react";
import { BookOpen, ChevronRight, Clock3, History, Inbox, ListChecks, MessageSquare, Target, X } from "lucide-react";
import type { BackendState } from "./protocol";
import type { PresenceRef } from "./motion";
import { useModalFocus } from "./useModalFocus";
import { bundleOutputs, conversationBundles } from "./conversationOutputs";
import { OutputCards } from "./OutputCards";

export function LibraryDrawer({ kind, state, open, presenceRef, close, navigate, preview, drafts }: { kind: "following" | "saved"; state: BackendState; open: boolean; presenceRef: PresenceRef; close(): void; navigate(command: string): void; preview(id: string): void; drafts(): void }) {
  const [narrow, setNarrow] = useState(() => matchMedia("(max-width: 960px)").matches);
  const focus = useModalFocus(open && narrow);
  useEffect(() => {
    const media = matchMedia("(max-width: 960px)");
    const change = () => setNarrow(media.matches);
    media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);
  const title = kind === "following" ? "Following" : "Saved";
  const blocked = state.running || !!state.external;
  const link = (label: string, description: string, command: string, Icon: typeof BookOpen) => <button className="library-link" disabled={blocked} onClick={() => navigate(command)}><Icon size={17} /><span><strong>{label}</strong><small>{description}</small></span><ChevronRight size={14} /></button>;
  const outputs = conversationBundles(state).flatMap((bundle, index) => bundleOutputs(bundle, index + 1)).reverse();
  return <>
    {narrow && open && <div className="work-details-scrim" aria-hidden="true" onClick={close} />}
    <aside ref={(element) => { focus.current = element; presenceRef(element); }} id="library-panel" className="workspace-panel library-drawer" aria-label={title} role={narrow ? "dialog" : undefined} aria-modal={narrow && open ? true : undefined} data-open={open} inert={!open} aria-hidden={!open} onKeyDown={(event) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); }
      if (narrow && (event.metaKey || event.ctrlKey) && ["n", "k", "f", ","].includes(event.key.toLowerCase())) event.stopPropagation();
    }}>
      <header className="work-details-heading"><h2>{title}</h2><button className="icon-button" aria-label={`Close ${title.toLowerCase()}`} onClick={close}><X size={16} /></button></header>
      <div className="workspace-content">
        {kind === "following" ? <>
          <p className="library-intro">Your ongoing goals and configured checks.</p>
          <section className="library-section" aria-label="Ongoing goals"><h3>Ongoing goals</h3>
            {state.following_issue ? <p className="work-details-notice" role="status">{state.following_issue}</p> : state.following_goals?.length ? <ul className="following-list">{state.following_goals.map((goal) => <li key={goal.id}><strong>{goal.title}</strong><span className="badge">{goal.status === "paused" ? "Paused" : "Saved goal"}</span><p>{state.checks?.some((check) => check.goal_id === goal.id) ? "Has a configured check · timing and availability below" : "No recurring check attached"}</p></li>)}</ul> : <p className="work-details-empty">No ongoing goals yet. You can start with a conversation.</p>}
            {link("Open goals", "Review or change your ongoing goals", "/goals", Target)}
          </section>
          <section className="library-section" aria-label="Configured checks"><h3>Configured checks</h3>
            {state.checks_issue && <p className="work-details-notice" role="status">{state.checks_issue}{!!state.checks?.length && " Showing the last reported states."}</p>}
            {state.checks?.length ? state.checks.map((check, index) => <div className="work-check" key={index}><strong>{check.title}</strong><span data-tone={check.tone}>{check.state}</span><p>{check.health}</p></div>) : !state.checks_issue && <p className="work-details-empty">No recurring checks configured.</p>}
            {state.supervisor_needs_start && <p className="work-details-notice" data-tone="warning">Background checks need the local supervisor to start.</p>}
            {link("Manage checks", "Sources, timing and notification settings", "/followup", ListChecks)}
          </section>
          <section className="library-section" aria-label="Updates and activity">{link("Updates", "Results from your configured checks", "/inbox", Inbox)}{link("Activity & attention", "Changes, requests and blocked work", "/overview", History)}</section>
        </> : <>
          <p className="library-intro">Find saved knowledge, conversations and work.</p>
          <section className="library-section" aria-label="Files in this conversation"><h3>Files in this conversation</h3>
            {outputs.length ? <OutputCards outputs={outputs} preview={preview} /> : <p className="work-details-empty">Files appear here when this conversation produces them.</p>}
          </section>
          <section className="library-section" aria-label="Saved library">
            {link("What Gator remembers", "Saved notes, decisions and source references", "/knowledge", BookOpen)}
            {link("All conversations", "Find and continue a separate topic", "/conversations", MessageSquare)}
            {link("Task runs", "Retained results and individual run details", "/history", History)}
            <button className="library-link" onClick={drafts}><Clock3 size={17} /><span><strong>Unsent drafts</strong><small>Review local drafts and retention</small></span><ChevronRight size={14} /></button>
          </section>
        </>}
        {blocked && <p className="work-details-caption" role="status">Finish or stop the current work to open another screen.</p>}
      </div>
    </aside>
  </>;
}
