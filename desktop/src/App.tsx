import { memo, useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { ArrowDown, ArrowUp, BookOpen, Check, ChevronDown, ChevronRight, Clock3, Command, Folder, Inbox, LayoutDashboard, ListChecks, ListPlus, LoaderCircle, MessageSquare, PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen, Plus, Search, Settings2, ShieldCheck, Square, Target, X } from "lucide-react";
import type { BackendMessage, BackendState } from "../../ui/src/protocol";
import type { DesktopFrame, DesktopSnapshot } from "./bridge";
import { Panel } from "./Panel";
import { Markdown, Result, bundleResult } from "./Result";
import { useModalFocus } from "./useModalFocus";
import { StarterDashboard } from "./StarterDashboard";
import { UsagePanel } from "./UsagePanel";
import { ConversationsPanel } from "./ConversationsPanel";
import { IconSwap, Presence, useViewMotion, type PresenceRef } from "./motion";
import { MessageQueue, useQueueAction } from "./MessageQueue";
import { SettingsNavigation, settingsRoutes } from "./SettingsNavigation";
import { CopyButton } from "./CopyButton";
import { useDrafts } from "./useDrafts";
import { DraftManager } from "./DraftManager";
import { useConversationSearch } from "./useConversationSearch";

const destinations = [
  { command: "/overview", label: "Overview", icon: LayoutDashboard, group: "work" },
  { command: "/goals", label: "Ongoing goals", icon: Target, group: "work" },
  { command: "/conversations", label: "Conversations", icon: MessageSquare, group: "work" },
  { command: "/inbox", label: "Inbox", icon: Inbox, group: "work" },
  { command: "/followup", label: "Follow-up checks", icon: ListChecks, group: "work" },
  { command: "/knowledge", label: "Saved knowledge", icon: BookOpen, group: "library" },
] as const;

const overviewCopy: Record<string, { label: string; empty: string; icon: typeof Check }> = {
  changed: { label: "What changed", empty: "You're up to date", icon: Clock3 },
  needs: { label: "Needs you", empty: "Nothing waiting on you", icon: Inbox },
  next: { label: "Up next", empty: "Room for your next step", icon: ArrowUp },
  blocked: { label: "Blocked", empty: "Nothing blocked", icon: ShieldCheck },
};

function Logo({ small = false }: { small?: boolean }) {
  return <span className={`gator-logo ${small ? "small" : ""}`} aria-hidden="true">🐊</span>;
}

function useBackend() {
  const [snapshot, setSnapshot] = useState<DesktopSnapshot>({});
  const [error, setError] = useState("");
  const [reconnecting, setReconnecting] = useState(false);
  const [reconnectError, setReconnectError] = useState("");
  useEffect(() => {
    const api = window.gator;
    if (!api) { setError("Open Gator as a desktop app with npm start from the desktop directory."); return; }
    let live = true;
    let received = false;
    const unsubscribe = api.subscribe((next) => { received = true; if (live) { setSnapshot(next); if (next.connection === "connected" || next.connection === "disconnected") setReconnecting(false); } });
    api.snapshot().then((next) => { if (live && !received) setSnapshot(next); }).catch((err) => { if (live) setError(String(err)); });
    return () => { live = false; unsubscribe(); };
  }, []);
  const send = useCallback(async (frame: DesktopFrame) => {
    try { await window.gator?.send(frame); setError(""); }
    catch (err) { setError(err instanceof Error ? err.message : String(err)); throw err; }
  }, []);
  const reconnect = async () => {
    if (reconnecting) return;
    setReconnecting(true); setReconnectError(""); setError("");
    try { await window.gator!.restart(); }
    catch (error) { setReconnectError(error instanceof Error ? error.message : "Could not reconnect. Try again."); setReconnecting(false); }
  };
  return { ...snapshot, error: error || reconnectError || snapshot.error, reconnecting, reconnect, send, clearError: () => { setError(""); setReconnectError(""); } };
}

const Message = memo(function Message({ message, pending, onApproval }: { message: BackendMessage; pending?: number; onApproval(approved: boolean): void }) {
  const user = message.role === "You";
  const result = message.result ?? (message.bundle ? bundleResult(message.bundle) : undefined);
  const approval = result?.kind === "approval" && result.fields?.some((field) => field.label === "Request" && field.value === String(pending)) ? pending : undefined;
  return <article className={`message ${user ? "user-message" : message.role === "Gator" ? "assistant-message" : "result-message"}`} aria-label={user ? "Your message" : `${message.role} message`}>
    <div className="message-content"><div className="message-byline">{user ? "You" : message.role === "Gator" ? "Gator" : message.role}</div>
      {result ? <Result result={result} approval={approval} onApproval={onApproval} /> : message.role === "Gator" ? <Markdown text={message.text} /> : <div className="literal prose-text">{message.text}</div>}
      {!result && message.role === "Gator" && message.text && <div className="message-actions"><CopyButton text={message.text} label="Copy response" compact /></div>}
    </div>
  </article>;
}, (before, after) => before.pending === after.pending && before.message.role === after.message.role && before.message.text === after.message.text && JSON.stringify(before.message.result ?? before.message.bundle) === JSON.stringify(after.message.result ?? after.message.bundle) && before.onApproval === after.onApproval);

function OverviewCards({ state, navigate }: { state: BackendState; navigate(command: string): void }) {
  return <section className="overview-grid" aria-label="Everyday overview">{["changed", "needs", "next", "blocked"].map((id) => {
    const section = state.overview?.sections.find((item) => item.id === id);
    const item = overviewCopy[id];
    const Icon = item.icon;
    return <button className="overview-card" key={id} onClick={() => navigate("/overview")}>
      <div className="overview-card-label"><Icon size={16} /><span>{item.label}</span>{section && <span className="overview-count" data-attention={(id === "needs" || id === "blocked") && section.total > 0}>{section.total}</span>}</div>
      <p>{state.overview_issue ? "Open overview to retry" : !section ? "Reading your overview…" : section.summary || item.empty}</p>
      <ChevronRight className="card-chevron" size={14} />
    </button>;
  })}</section>;
}

function WorkspacePanel({ state, open, presenceRef, tab, setTab, navigate }: { state: BackendState; open: boolean; presenceRef: PresenceRef; tab: string; setTab(value: string): void; navigate(command: string): void }) {
  const content = useRef<HTMLDivElement>(null);
  useViewMotion(tab, content);
  const bundles = state.messages.filter((message) => message.bundle).map((message) => message.bundle!);
  const latest = bundles.at(-1);
  return <aside ref={presenceRef} className="workspace-panel" aria-label="Work details" data-open={open} inert={!open} aria-hidden={!open}>
    <div className="detail-tabs" role="tablist" aria-label="Work details">{["Deliverables", "Checks"].map((label, index) => <button key={label} id={`work-details-tab-${label}`} role="tab" aria-controls="work-details-content" aria-selected={tab === label} tabIndex={tab === label ? 0 : -1} onClick={() => setTab(label)} onKeyDown={(event) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const next = event.key === "Home" ? 0 : event.key === "End" ? 1 : (index + (event.key === "ArrowRight" ? 1 : -1) + 2) % 2;
      setTab(["Deliverables", "Checks"][next]);
      event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
    }}>{label}</button>)}</div>
    <div className="workspace-content" id="work-details-content" ref={content} role="tabpanel" aria-labelledby={`work-details-tab-${tab}`} tabIndex={0}>
      {tab === "Deliverables" ? latest ? <Result result={bundleResult(latest)} /> : <Empty icon={<Folder size={24} />} title="Your work lands here" text="Documents, verified outputs, and proposed changes appear after a task produces them." /> : <>
        <div className="flex items-center justify-between mb-5"><span className="eyebrow">Recurring checks</span><button className="small-button" disabled={state.running} onClick={() => navigate("/followup")}>Manage<ChevronRight size={13} /></button></div>
        {state.checks?.length ? state.checks.map((check, i) => <div className="check-item" key={i}><div className="flex items-center gap-2"><span className="status-dot" data-tone={check.tone} /><strong>{check.title}</strong></div><span className="badge" data-tone={check.tone}>{check.state}</span><p>{check.health}</p></div>) : <Empty icon={<ListChecks size={24} />} title="Keep an eye on things" text="Set up a recurring check for an ongoing goal. Results arrive in your inbox." />}
        {state.supervisor_needs_start && <p className="panel-notice">Checks are configured, but the local supervisor needs to start. Open Recovery to review.</p>}
      </>}
    </div>
  </aside>;
}

function Empty({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return <div className="empty-state"><div>{icon}</div><h3>{title}</h3><p>{text}</p></div>;
}

export function App() {
  const { state, error, connection, sessionID, terminalBusy, fullScreen, reconnecting, reconnect, send, clearError } = useBackend();
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("gator:sidebar-collapsed") === "true");
  const [details, setDetails] = useState(false);
  const [tab, setTab] = useState("Deliverables");
  const drafts = useDrafts(state);
  const draft = drafts.text, setDraft = drafts.setText;
  const [showDrafts, setShowDrafts] = useState(false);
  const originalSession = useRef<string | undefined>(undefined);
  const [sessionChanged, setSessionChanged] = useState(false);
  const [search, setSearch] = useState<string | null>(null);
  const [navigation, setNavigation] = useState<{ command: string; draft?: string } | null>(null);
  const [showCommands, setShowCommands] = useState(false);
  const [sending, setSending] = useState(false);
  const [approvalBusy, setApprovalBusy] = useState(false);
  const [draftError, setDraftError] = useState("");
  const [queueEditing, setQueueEditing] = useState(false);
  const queueAction = useQueueAction(state, send, error);
  const composer = useRef<HTMLTextAreaElement>(null);
  const transcript = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const [atLatest, setAtLatest] = useState(true);
  const [newActivity, setNewActivity] = useState(false);
  const previousTranscript = useRef({ identity: "", content: "" });
  const backendInput = useRef<string | undefined>(undefined);
  const view = useRef<HTMLDivElement>(null);
  const panel = state?.panel;
  const conversationIdentity = state?.conversation_id || state?.source || "";
  const conversationSearch = useConversationSearch(transcript, search, conversationIdentity, !panel);
  const settingsPage = settingsRoutes.some((route) => route.command === state?.destination);
  const modifier = window.gator?.platform === "darwin" ? "⌘" : "Ctrl+";
  const viewKey = state ? `${state.destination || (state.home && !state.messages.length ? "home" : "chat")}:${panel?.kind ?? ""}:${panel?.title ?? ""}` : undefined;
  useViewMotion(viewKey, view);
  const hasUnsavedView = queueEditing || panel?.rows?.some((row) => row.input) || panel?.kind === "confirmation";

  useEffect(() => { localStorage.setItem("gator:sidebar-collapsed", String(collapsed)); }, [collapsed]);
  useEffect(() => { if (sessionID) { if (originalSession.current && originalSession.current !== sessionID) setSessionChanged(true); originalSession.current = sessionID; } }, [sessionID]);
  useEffect(() => {
    if (state && (state.input ?? "") !== backendInput.current) {
      backendInput.current = state.input ?? "";
      if (state.input) { setDraft(state.input); composer.current?.focus(); }
    }
  }, [state?.input]);
  useLayoutEffect(() => {
    if (!state || panel) return;
    const content = JSON.stringify(state.messages);
    if (previousTranscript.current.identity !== conversationIdentity) { atBottom.current = true; setAtLatest(true); setNewActivity(false); }
    else if (previousTranscript.current.content !== content && !atBottom.current) setNewActivity(true);
    previousTranscript.current = { identity: conversationIdentity, content };
    if (atBottom.current && search === null) transcript.current?.scrollTo({ top: transcript.current.scrollHeight });
  }, [state?.messages, state?.running, panel, conversationIdentity, search]);
  useEffect(() => { if (composer.current) { composer.current.style.height = "auto"; composer.current.style.height = `${Math.min(180, composer.current.scrollHeight)}px`; } }, [draft]);

  const attempt = useCallback((frame: DesktopFrame) => { void send(frame).catch(() => {}); }, [send]);
  const navigate = (command: string, prefill?: string) => {
    if (hasUnsavedView) { setNavigation({ command, draft: prefill }); return; }
    setSearch(null); setShowCommands(false);
    if (settingsRoutes.some((route) => route.command === command)) setDetails(false);
    attempt({ type: "navigate", text: command });
    if (prefill !== undefined) setDraft(prefill);
  };
  useEffect(() => {
    const keyboard = (event: globalThis.KeyboardEvent) => {
      if (event.defaultPrevented || showDrafts) return;
      if (navigation && event.key !== "Escape") return;
      if ((event.metaKey || event.ctrlKey) && ["k", "f", "n", ","].includes(event.key.toLowerCase())) {
        event.preventDefault();
        if (event.key.toLowerCase() === "f") setSearch((current) => current === null ? "" : null);
        else if (event.key.toLowerCase() === "k") setShowCommands((current) => !current);
        else if (event.key === ",") { if (!state?.running) navigate("/model"); }
        else navigate("/new");
      }
      if (event.key === "Escape") {
        if (navigation) setNavigation(null);
        else if (showCommands) setShowCommands(false);
        else if (search !== null) setSearch(null);
        else if (panel?.id) attempt({ type: "panel", key: "esc", panel: { id: panel.id, action: "key" } });
        else if (state?.running) attempt({ type: "key", key: "esc" });
        composer.current?.focus();
      }
    };
    window.addEventListener("keydown", keyboard);
    return () => window.removeEventListener("keydown", keyboard);
  });

  const approve = useCallback(async (approved: boolean) => {
    if (!state?.pending_approval || approvalBusy) return;
    setApprovalBusy(true);
    try { await send({ type: "approval", interaction_id: state.pending_approval.id, approved }); }
    catch {} finally { setApprovalBusy(false); }
  }, [state?.pending_approval?.id, approvalBusy, send]);

  if (!state) return <main className="boot-screen"><Logo /><h1>{reconnecting ? "Reconnecting to Gator…" : error ? "Gator could not connect" : "Getting your workspace ready"}</h1><p>{error || "Opening your local Gator profile. No messages will be submitted."}</p>{(!error || reconnecting) && <LoaderCircle className="spin accent" size={20} />}{error && window.gator && <button className="button" disabled={reconnecting} onClick={() => void reconnect()}>{reconnecting ? "Reconnecting…" : "Reconnect"}</button>}</main>;

  const queueFull = state.running && state.queued >= 16 && !draft.trimStart().startsWith("/");
  const submit = async (guide = false) => {
    if (!draft.trim() || !drafts.ready || sending || queueAction.busy || panel || error || state.external || (guide ? !state.guidance_id : queueFull)) return;
    setSending(true);
    setDraftError("");
    try {
      if (guide) await queueAction.act({ id: state.guidance_id!, action: "guide", text: draft });
      else await send({ type: "submit", text: draft });
      setDraft(""); setShowCommands(false); atBottom.current = true;
    } catch (err) { setDraftError(err instanceof Error ? err.message : String(err)); }
    finally { setSending(false); composer.current?.focus(); }
  };
  const composerKeys = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && (event.metaKey || event.ctrlKey) && !event.nativeEvent.isComposing && state.running) { event.preventDefault(); void submit(true); return; }
    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); void submit(); }
  };
  const home = state.home && !state.messages.length && !panel;
  const conversationBrowser = state.destination === "/conversations" || state.destination === "/history";
  const activeDestination = state.destination === "/history" ? "/conversations" : state.destination;
  const messages = state.messages;
  const jumpToLatest = () => { setSearch(null); atBottom.current = true; setAtLatest(true); setNewActivity(false); transcript.current?.scrollTo({ top: transcript.current.scrollHeight }); composer.current?.focus({ preventScroll: true }); };
  const slash = draft.startsWith("/") && !draft.includes(" ") ? state.commands.filter((command) => command.name.startsWith(draft.slice(1))).slice(0, 7) : [];
  const sourceName = state.source.split("/").at(-1) || "Workspace";
  const headerTitle = home ? "New conversation" : panel ? conversationBrowser ? "Conversations" : panel.title : state.title;

  return <div className={`app-shell ${collapsed ? "sidebar-collapsed" : ""} ${details ? "details-open" : ""}`} data-theme={state.theme} data-platform={window.gator?.platform} data-fullscreen={fullScreen}>
    <div className="titlebar" aria-hidden="true"><span>Gator</span></div>
    <aside className="sidebar" aria-label="Main navigation">
      <div className="sidebar-brand"><button className="brand-button" onClick={() => navigate("/new")} aria-label="Gator home"><Logo /><span>Gator</span></button><button className="icon-button collapse-button" onClick={() => setCollapsed(!collapsed)} aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}><IconSwap active={collapsed} size={17} on={<PanelLeftOpen size={17} />} off={<PanelLeftClose size={17} />} /></button></div>
      {settingsPage ? <SettingsNavigation destination={state.destination} collapsed={collapsed} navigate={navigate} /> : <>
      <button className="nav-button new-chat" aria-label="New conversation" onClick={() => navigate("/new")} disabled={state.running}><Plus size={18} /><span>New conversation</span><kbd>{modifier}N</kbd></button>
      <button className="nav-button command-trigger" aria-label="Search commands" onClick={() => setShowCommands(true)} title="Search commands"><Search size={17} /><span>Search commands</span><kbd>{modifier}K</kbd></button>
      <div className="sidebar-scroll">{["work", "library"].map((group) => <div className="nav-group" key={group}><div className="nav-group-label">{group === "work" ? "Your workspace" : "Library"}</div><nav aria-label={group}>{destinations.filter((destination) => destination.group === group).map(({ command, label, icon: Icon }) => <button className="nav-button" key={command} aria-label={label} aria-current={activeDestination === command ? "page" : undefined} onClick={() => navigate(command)} disabled={state.running} title={collapsed ? label : undefined}><Icon size={18} /><span>{label}</span></button>)}</nav></div>)}
      </div>
      <div className="sidebar-bottom"><button className="nav-button" aria-label="Settings" title={collapsed ? "Settings" : `Settings (${modifier},)`} onClick={() => navigate("/model")} disabled={state.running}><Settings2 size={17} /><span>Settings</span><kbd>{modifier},</kbd></button></div>
      </>}
    </aside>
    <main className="main-area">
      <header className="main-header"><div className="breadcrumbs"><span>{settingsPage ? "Settings" : sourceName}</span><ChevronRight size={13} /><strong title={headerTitle}>{headerTitle}</strong></div><div className="header-actions">{!settingsPage && <><button className="header-workspace" aria-label="Choose workspace folder" onClick={() => void window.gator?.chooseFolder().catch(() => {})} disabled={state.running || !!panel} title={state.source}><Folder size={15} /><span>{sourceName}</span><ChevronDown size={13} /></button><button className="icon-button" aria-label={details ? "Hide work details" : "Show work details"} aria-expanded={details} onClick={() => setDetails(!details)}><IconSwap active={details} on={<PanelRightClose size={18} />} off={<PanelRightOpen size={18} />} /></button></>}</div></header>
      {error && <div className="connection-error" role="alert"><div><strong>{connection === "disconnected" ? "Connection lost" : "Action could not complete"}</strong><p>{connection === "disconnected" ? "This view may be out of date. Reconnecting opens a new local session and does not resume or resubmit work." : error}</p>{connection === "disconnected" && <details><summary>Technical details</summary><p>{error}</p></details>}</div>{connection === "disconnected" ? <button className="button" disabled={reconnecting} aria-busy={reconnecting} onClick={() => void reconnect()}>{reconnecting ? "Reconnecting…" : "Reconnect"}</button> : <button className="icon-button" aria-label="Dismiss action error" onClick={clearError}><X size={16} /></button>}</div>}
      {sessionChanged && !error && <div className="session-notice" role="status"><span>A new local session is ready. Previous work was not resumed. Saved conversations and drafts remain available.</span><button className="icon-button" aria-label="Dismiss session notice" onClick={() => setSessionChanged(false)}><X size={16} /></button></div>}
      {state.notice && <div className="view-status" role="status">{state.notice}</div>}
      {panel && state.pending_approval && <div className="approval-reminder panel-approval"><ShieldCheck size={16} /><span>A current action needs your review.</span><button className="small-button" onClick={() => navigate("/chat")}>Review action<ChevronRight size={13} /></button></div>}
      {state.external && <div className="terminal-banner" role="status"><div><strong>Continue this step in Terminal</strong><p>Gator needs an interactive terminal for this {state.external.kind === "connector" ? "connection" : "action"}. Return here when it finishes.</p></div><button className="button" disabled={terminalBusy} onClick={() => void window.gator?.continueInTerminal().catch(() => {})}>{terminalBusy ? "Waiting for Terminal…" : "Open Terminal"}</button></div>}
      <div className="view-content" data-home={home} data-settings={settingsPage} ref={view}>
      {panel ? conversationBrowser ? <ConversationsPanel panel={panel} send={send} navigate={navigate} /> : panel.kind === "usage" && panel.usage ? <UsagePanel panel={panel} usage={panel.usage} send={send} /> : <Panel panel={panel} send={send} /> : <>
        {search !== null && <div className="transcript-search" role="search" aria-label="Find in conversation"><Search size={16} /><input autoFocus aria-label="Search conversation" maxLength={256} value={search} onChange={(event) => setSearch(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.nativeEvent.isComposing) { event.preventDefault(); conversationSearch.move(event.shiftKey ? -1 : 1); } }} placeholder="Find in this conversation…" /><span className="search-count" role="status">{!search ? "Find text" : !conversationSearch.count ? "No matches" : `${conversationSearch.active + 1} of ${conversationSearch.count}${conversationSearch.capped ? "+" : ""}`}</span><button className="icon-button" aria-label="Previous match" title="Previous match (Shift+Enter)" disabled={!conversationSearch.count} onClick={() => conversationSearch.move(-1)}><ArrowUp size={16} /></button><button className="icon-button" aria-label="Next match" title="Next match (Enter)" disabled={!conversationSearch.count} onClick={() => conversationSearch.move(1)}><ArrowDown size={16} /></button><button className="icon-button" aria-label="Close search" onClick={() => { setSearch(null); composer.current?.focus(); }}><X size={16} /></button></div>}
        <div className={`transcript-stage ${home ? "home-stage" : ""}`}><div className={`conversation-scroll ${home ? "home-scroll" : ""}`} ref={transcript} onScroll={(event) => { const el = event.currentTarget; const near = el.scrollHeight - el.scrollTop - el.clientHeight < 100; atBottom.current = near; setAtLatest(near); if (near) setNewActivity(false); }}>
          {home ? <div className="home-content"><h1>What’s on your mind?</h1>
            {state.first_run && <div className="setup-callout"><div><Settings2 size={18} /><div><strong>Make Gator yours</strong><p>Choose a cloud or local model to start your first conversation.</p></div></div><button className="button" onClick={() => navigate("/model")}>Choose a model<ChevronRight size={15} /></button></div>}
            </div> : <div className="transcript-content" aria-label="Conversation">{messages.map((message, index) => <Message key={`${conversationIdentity}:${index}`} message={message} pending={approvalBusy ? undefined : state.pending_approval?.id} onApproval={approve} />)}{state.running && <div className="run-progress" role="status"><LoaderCircle size={16} className="spin" /><span>{state.cancelling ? "Stopping…" : state.pending_approval ? "Waiting for your approval" : state.status || "Gator is working…"}</span><span className="elapsed">{state.elapsed}</span></div>}</div>}
        </div>{!home && !atLatest && <button className="jump-latest" aria-label={newActivity ? "New activity. Jump to latest" : "Jump to latest"} onClick={jumpToLatest}><ArrowDown size={16} />{newActivity && <span>New activity</span>}</button>}</div>
        <div className="composer-area">{state.pending_approval && <div className="approval-reminder"><ShieldCheck size={16} /><span>An action is waiting for your review.</span><button className="small-button" onClick={jumpToLatest}>View request<ArrowDown size={13} /></button></div>}
          {!!slash.length && <div className="slash-suggestions" aria-label="Command suggestions">{slash.map((command) => <button key={command.name} onClick={() => { setDraft(`/${command.name}${command.needs_input ? " " : ""}`); composer.current?.focus(); }}><span>/{command.name}</span><span>{command.description}</span></button>)}</div>}
          <MessageQueue items={state.queue ?? []} running={state.running} busy={queueAction.busy || sending || !!error || !!state.external} act={queueAction.act} onEditingChange={setQueueEditing} onUseDraft={(text) => { setDraft((current) => current ? `${current}\n\n${text}` : text); composer.current?.focus(); }} />
          {(drafts.recovered || drafts.issue) && <p className="draft-recovery" role="status">{drafts.issue || "Draft recovered. Review it before sending."}</p>}
          <div className="composer"><textarea ref={composer} value={draft} onChange={(event) => { setDraft(event.target.value); setDraftError(""); }} onKeyDown={composerKeys} placeholder={state.running ? "Add a follow-up…" : "Ask Gator, or / for commands…"} aria-label="Message Gator" disabled={sending || !!state.external} rows={2} /><div className="composer-toolbar"><button className="composer-model" disabled={state.running || !!error} onClick={() => navigate("/model")}><span className="status-dot" /><span>{state.model || "Choose a model"}</span><ChevronDown size={12} /></button><button className="icon-button draft-trigger" aria-label="Saved drafts" title={drafts.saving ? "Saving draft…" : "Saved drafts and local retention"} onClick={() => { setShowDrafts(true); void drafts.manage({ action: "list" }).catch(() => {}); }}><Clock3 size={16} /></button><div className="composer-actions">
            {state.running && <button className="small-button guide-button" title={`Guide the current task (${window.gator?.platform === "darwin" ? "⌘" : "Ctrl"}+Enter)`} disabled={!draft.trim() || sending || queueAction.busy || !state.guidance_id || !!error || !!state.external} onClick={() => void submit(true)}>Guide now</button>}
            {state.running && <button className="send-button stop" aria-label="Stop current work" disabled={state.cancelling} onClick={() => attempt({ type: "key", key: "esc" })}><Square size={13} fill="currentColor" /></button>}
            <button className="send-button" aria-label={state.running ? "Queue message" : "Send message"} title={queueFull ? "Queue is full (16 messages)" : state.running ? "Queue for after this task (Enter)" : "Send message"} aria-busy={sending} disabled={!draft.trim() || !drafts.ready || sending || queueAction.busy || queueFull || !!error || !!state.external} onClick={() => void submit()}><IconSwap active={sending} on={<LoaderCircle size={18} className="spin" />} off={state.running ? <ListPlus size={18} /> : <ArrowUp size={18} />} /></button>
          </div></div></div>
          {draftError && <p className="composer-error" role="alert">{draftError}</p>}
          {queueFull && <p className="composer-error" role="status">Queue full. Edit or remove a message to make room.</p>}
        </div>
        {home && <div className="home-dashboard"><OverviewCards state={state} navigate={navigate} /><StarterDashboard navigate={navigate} /></div>}
      </>}
      </div>
    </main>
    <Presence open={details}>{(open, presenceRef) => <WorkspacePanel state={state} open={open} presenceRef={presenceRef} tab={tab} setTab={setTab} navigate={navigate} />}</Presence>
    {panel && <div className="panel-draft-anchor"><MessageSquare size={14} /><span>{draft ? "Your message draft is kept while you’re here." : "Return to the conversation to write a message."}</span><button className="small-button" onClick={() => attempt({ type: "panel", key: "esc", panel: { id: panel.id!, action: "key" } })}>Back<kbd>Esc</kbd></button></div>}
    <Presence open={showCommands}>{(open, presenceRef) => <CommandMenu open={open} presenceRef={presenceRef} commands={state.commands} close={() => setShowCommands(false)} select={(name, needsInput) => { setShowCommands(false); if (needsInput) navigate("/chat", `/${name} `); else navigate(`/${name}`); }} />}</Presence>
    <Presence open={showDrafts}>{(open, presenceRef) => <DraftManager open={open} presenceRef={presenceRef} saved={drafts.saved} issue={drafts.issue} manage={drafts.manage} close={() => setShowDrafts(false)} restore={(retained) => { setDraft((current) => !current || retained.context === drafts.context ? retained.text : `${current}\n\n${retained.text}`); composer.current?.focus(); }} />}</Presence>
    <Presence open={!!navigation}>{(open, presenceRef) => <DiscardDialog open={open} presenceRef={presenceRef} cancel={() => setNavigation(null)} confirm={() => { if (!navigation) return; attempt({ type: "navigate", text: navigation.command }); if (navigation.draft !== undefined) setDraft(navigation.draft); setNavigation(null); }} />}</Presence>
  </div>;
}

function CommandMenu({ commands, open, presenceRef, close, select }: { commands: BackendState["commands"]; open: boolean; presenceRef: PresenceRef; close(): void; select(name: string, needsInput: boolean): void }) {
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);
  const focus = useModalFocus(open);
  const shown = commands.filter((command) => `${command.name} ${command.description}`.toLowerCase().includes(query.toLowerCase()));
  return <div ref={presenceRef} className="modal-backdrop" data-open={open} inert={!open} aria-hidden={!open} onClick={close}><section ref={focus} className="command-menu" role="dialog" aria-modal="true" aria-label="Search commands" onClick={(event) => event.stopPropagation()}><div className="command-search"><Search size={18} /><input autoFocus placeholder="Find a command…" aria-label="Find a command" value={query} onChange={(event) => { setQuery(event.target.value); setSelected(0); }} onKeyDown={(event) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); const next = Math.max(0, Math.min(shown.length - 1, selected + (event.key === "ArrowDown" ? 1 : -1))); setSelected(next); focus.current?.querySelectorAll(".command-list button")[next]?.scrollIntoView({ block: "nearest" }); }
    if (event.key === "Enter" && shown[selected]) { event.preventDefault(); select(shown[selected].name, shown[selected].needs_input); }
  }} /><button className="icon-button" aria-label="Close commands" onClick={close}><X size={16} /></button></div><div className="command-list">{shown.map((command, index) => <button key={command.name} data-selected={index === selected} onClick={() => select(command.name, command.needs_input)}><Command size={16} /><div><strong>/{command.name}</strong><p>{command.description}</p></div>{command.advanced && <span className="badge">Advanced</span>}<ChevronRight size={15} /></button>)}{!shown.length && <p className="empty-hint">No commands match “{query}”.</p>}</div><footer><span>↑↓ choose · Enter run · Tab move focus</span><kbd>Esc</kbd></footer></section></div>;
}

function DiscardDialog({ open, presenceRef, cancel, confirm }: { open: boolean; presenceRef: PresenceRef; cancel(): void; confirm(): void }) {
  const focus = useModalFocus(open);
  return <div ref={presenceRef} className="modal-backdrop" data-open={open} inert={!open} aria-hidden={!open}><section ref={focus} role="alertdialog" aria-modal="true" aria-labelledby="discard-title" className="confirm-dialog"><h2 id="discard-title">Leave this edit?</h2><p>Your unsaved changes will be discarded.</p><div className="flex justify-end gap-2"><button className="button" onClick={cancel}>Keep editing</button><button className="button primary" onClick={confirm}>Leave view</button></div></section></div>;
}
