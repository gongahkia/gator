import { useCallback, useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { ArrowDown, ArrowUp, BookOpen, Check, ChevronDown, ChevronRight, CircleHelp, Clock3, Command, Database, Folder, Inbox, LayoutDashboard, ListChecks, LoaderCircle, MessageSquare, PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen, Plus, Search, Settings2, ShieldCheck, Square, Target, Wallet, X } from "lucide-react";
import type { BackendMessage, BackendState } from "../../ui/src/protocol";
import type { DesktopFrame, DesktopSnapshot } from "./bridge";
import { Panel } from "./Panel";
import { Markdown, Result, bundleResult } from "./Result";
import { useModalFocus } from "./useModalFocus";

const destinations = [
  { command: "/overview", label: "Overview", icon: LayoutDashboard, group: "work" },
  { command: "/goals", label: "Ongoing goals", icon: Target, group: "work" },
  { command: "/conversations", label: "Conversations", icon: MessageSquare, group: "work" },
  { command: "/inbox", label: "Inbox", icon: Inbox, group: "work" },
  { command: "/followup", label: "Follow-up checks", icon: ListChecks, group: "work" },
  { command: "/knowledge", label: "Saved knowledge", icon: BookOpen, group: "library" },
  { command: "/history", label: "Work history", icon: Clock3, group: "library" },
  { command: "/usage", label: "Usage & limits", icon: Wallet, group: "settings" },
  { command: "/data", label: "Data & backups", icon: Database, group: "settings" },
  { command: "/recovery", label: "Recovery", icon: ShieldCheck, group: "settings" },
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
  useEffect(() => {
    const api = window.gator;
    if (!api) { setError("Open Gator as a desktop app with npm start from the desktop directory."); return; }
    let live = true;
    let received = false;
    const unsubscribe = api.subscribe((next) => { received = true; if (live) setSnapshot(next); });
    api.snapshot().then((next) => { if (live && !received) setSnapshot(next); }).catch((err) => { if (live) setError(String(err)); });
    return () => { live = false; unsubscribe(); };
  }, []);
  const send = useCallback(async (frame: DesktopFrame) => {
    try { await window.gator?.send(frame); setError(""); }
    catch (err) { setError(err instanceof Error ? err.message : String(err)); throw err; }
  }, []);
  return { ...snapshot, error: error || snapshot.error, send, clearError: () => setError("") };
}

function Message({ message, pending, onApproval }: { message: BackendMessage; pending?: number; onApproval(approved: boolean): void }) {
  const user = message.role === "You";
  const result = message.result ?? (message.bundle ? bundleResult(message.bundle) : undefined);
  const approval = result?.kind === "approval" && result.fields?.some((field) => field.label === "Request" && field.value === String(pending)) ? pending : undefined;
  return <article className={`message ${user ? "user-message" : ""}`}>
    <div className="message-avatar">{user ? <span>Y</span> : <Logo small />}</div>
    <div className="message-content"><div className="message-byline">{user ? "You" : message.role === "Gator" ? "Gator" : message.role}</div>
      {result ? <Result result={result} approval={approval} onApproval={onApproval} /> : message.role === "Gator" ? <Markdown text={message.text} /> : <div className="literal prose-text">{message.text}</div>}
    </div>
  </article>;
}

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

function WorkspacePanel({ state, tab, setTab, navigate }: { state: BackendState; tab: string; setTab(value: string): void; navigate(command: string): void }) {
  const bundles = state.messages.filter((message) => message.bundle).map((message) => message.bundle!);
  const latest = bundles.at(-1);
  return <aside className="workspace-panel" aria-label="Work details">
    <div className="detail-tabs" role="tablist" aria-label="Work details">{["Context", "Deliverables", "Checks"].map((label) => <button key={label} role="tab" aria-selected={tab === label} onClick={() => setTab(label)}>{label}</button>)}</div>
    <div className="workspace-content" role="tabpanel" aria-label={tab}>
      {tab === "Context" ? <>
        <div className="detail-section"><span className="eyebrow">Selected workspace</span><div className="source-card"><Folder size={18} /><div><strong>{state.source.split("/").at(-1) || "Workspace"}</strong><p>{state.source}</p></div></div><p className="detail-caption">Gator captures this folder for your next turn. Work starts with read-only access.</p></div>
        <div className="detail-section"><span className="eyebrow">Conversation</span><dl className="context-fields"><dt>Goal</dt><dd>{state.goal_id ? state.title : "One-off conversation"}</dd><dt>Model</dt><dd>{state.model || "Choose a model"}</dd><dt>Provider</dt><dd>{state.provider || "Not configured"}</dd></dl></div>
        {state.status && <details className="session-details"><summary>Session details</summary><p className="literal">{state.status}</p></details>}
        <div className="detail-section"><span className="eyebrow">Context & sources</span><p className="detail-caption">Inspect the saved context and source references supplied to a run, or browse knowledge for this workspace.</p><button className="button w-full" disabled={state.running} onClick={() => navigate("/context")}>Inspect context<ChevronRight size={15} /></button></div>
        <div className="detail-section"><ShieldCheck size={18} className="accent mb-2" /><h3>You stay in control</h3><p className="detail-caption">External changes need your approval. Gator shows the exact action before you decide.</p></div>
      </> : tab === "Deliverables" ? latest ? <Result result={bundleResult(latest)} /> : <Empty icon={<Folder size={24} />} title="Your work lands here" text="Documents, verified outputs, and proposed changes appear after a task produces them." /> : <>
        <div className="flex items-center justify-between mb-5"><span className="eyebrow">Recurring checks</span><button className="small-button" disabled={state.running} onClick={() => navigate("/followup")}>Manage<ChevronRight size={13} /></button></div>
        {state.checks?.length ? state.checks.map((check, i) => <div className="check-item" key={i}><div className="flex items-center gap-2"><span className="status-dot" data-tone={check.tone} /><strong>{check.title}</strong></div><span className="badge" data-tone={check.tone}>{check.state}</span><p>{check.health}</p></div>) : <Empty icon={<ListChecks size={24} />} title="Keep an eye on things" text="Set up a recurring check for an ongoing goal. Results arrive in your inbox." />}
        {state.supervisor_needs_start && <p className="panel-notice">Checks are configured, but the local supervisor needs to start. Open Recovery to review.</p>}
      </>}
    </div>
    <div className="workspace-footer"><span className="status-dot" />Same Gator profile as your terminal</div>
  </aside>;
}

function Empty({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return <div className="empty-state"><div>{icon}</div><h3>{title}</h3><p>{text}</p></div>;
}

export function App() {
  const { state, error, terminalBusy, fullScreen, send, clearError } = useBackend();
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("gator:sidebar-collapsed") === "true");
  const [details, setDetails] = useState(false);
  const [tab, setTab] = useState("Context");
  const [draft, setDraft] = useState("");
  const [search, setSearch] = useState<string | null>(null);
  const [navigation, setNavigation] = useState<{ command: string; draft?: string } | null>(null);
  const [showCommands, setShowCommands] = useState(false);
  const [sending, setSending] = useState(false);
  const [approvalBusy, setApprovalBusy] = useState(false);
  const [messageTiming, setMessageTiming] = useState("next");
  const composer = useRef<HTMLTextAreaElement>(null);
  const transcript = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const backendInput = useRef<string | undefined>(undefined);
  const panel = state?.panel;
  const hasUnsavedView = panel?.rows?.some((row) => row.input) || panel?.kind === "confirmation";

  useEffect(() => { localStorage.setItem("gator:sidebar-collapsed", String(collapsed)); }, [collapsed]);
  useEffect(() => {
    if (state && (state.input ?? "") !== backendInput.current) {
      backendInput.current = state.input ?? "";
      if (state.input) { setDraft(state.input); composer.current?.focus(); }
    }
  }, [state?.input]);
  useEffect(() => { if (atBottom.current && search === null) transcript.current?.scrollTo({ top: transcript.current.scrollHeight }); }, [state?.messages, state?.running, search]);
  useEffect(() => { if (composer.current) { composer.current.style.height = "auto"; composer.current.style.height = `${Math.min(180, composer.current.scrollHeight)}px`; } }, [draft]);
  useEffect(() => { if (!state?.running) setMessageTiming("next"); }, [state?.running]);

  const attempt = useCallback((frame: DesktopFrame) => { void send(frame).catch(() => {}); }, [send]);
  const navigate = (command: string, prefill?: string) => {
    if (hasUnsavedView) { setNavigation({ command, draft: prefill }); return; }
    setSearch(null); setShowCommands(false);
    attempt({ type: "navigate", text: command });
    if (prefill !== undefined) setDraft(prefill);
  };
  useEffect(() => {
    const keyboard = (event: globalThis.KeyboardEvent) => {
      if (navigation && event.key !== "Escape") return;
      if ((event.metaKey || event.ctrlKey) && ["k", "f", "n"].includes(event.key.toLowerCase())) {
        event.preventDefault();
        if (event.key.toLowerCase() === "f") setSearch((current) => current === null ? "" : null);
        else if (event.key.toLowerCase() === "k") setShowCommands((current) => !current);
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

  if (!state) return <main className="boot-screen"><Logo /><h1>{error ? "Gator could not connect" : "Getting your workspace ready"}</h1><p>{error || "Opening your local Gator profile…"}</p>{!error && <LoaderCircle className="spin accent" size={20} />}{error && window.gator && <button className="button" onClick={() => { clearError(); void window.gator!.restart().catch(() => {}); }}>Reconnect</button>}</main>;

  const submit = async () => {
    if (!draft.trim() || sending || panel || error) return;
    const text = state.running && messageTiming === "guide" && !draft.startsWith("/") ? `/steer ${draft}` : draft;
    setSending(true);
    try { await send({ type: "submit", text }); setDraft(""); setShowCommands(false); atBottom.current = true; }
    catch {} finally { setSending(false); }
  };
  const approve = async (approved: boolean) => {
    if (!state.pending_approval || approvalBusy) return;
    setApprovalBusy(true);
    try { await send({ type: "approval", interaction_id: state.pending_approval.id, approved }); }
    catch {} finally { setApprovalBusy(false); }
  };
  const composerKeys = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); void submit(); }
  };
  const home = state.home && !state.messages.length && !panel;
  const messages = state.messages.filter((message) => !search || `${message.text} ${JSON.stringify(message.result ?? message.bundle ?? "")}`.toLowerCase().includes(search.toLowerCase()));
  const slash = draft.startsWith("/") && !draft.includes(" ") ? state.commands.filter((command) => command.name.startsWith(draft.slice(1))).slice(0, 7) : [];
  const sourceName = state.source.split("/").at(-1) || "Workspace";

  return <div className={`app-shell ${collapsed ? "sidebar-collapsed" : ""} ${details ? "details-open" : ""}`} data-theme={state.theme} data-platform={window.gator?.platform} data-fullscreen={fullScreen}>
    <div className="titlebar" aria-hidden="true"><span>Gator</span></div>
    <aside className="sidebar" aria-label="Main navigation">
      <div className="sidebar-brand"><button className="brand-button" onClick={() => navigate("/new")} aria-label="Gator home"><Logo /><span>Gator</span></button><button className="icon-button collapse-button" onClick={() => setCollapsed(!collapsed)} aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}>{collapsed ? <PanelLeftOpen size={17} /> : <PanelLeftClose size={17} />}</button></div>
      <button className="nav-button new-chat" onClick={() => navigate("/new")} disabled={state.running}><Plus size={18} /><span>New conversation</span><kbd>⌘N</kbd></button>
      <button className="nav-button command-trigger" onClick={() => setShowCommands(true)} title="Search commands"><Search size={17} /><span>Search commands</span><kbd>⌘K</kbd></button>
      <div className="sidebar-scroll">{["work", "library"].map((group) => <div className="nav-group" key={group}><div className="nav-group-label">{group === "work" ? "Your workspace" : "Library"}</div><nav aria-label={group}>{destinations.filter((destination) => destination.group === group).map(({ command, label, icon: Icon }) => <button className="nav-button" key={command} aria-current={state.destination === command ? "page" : undefined} onClick={() => navigate(command)} disabled={state.running} title={collapsed ? label : undefined}><Icon size={18} /><span>{label}</span></button>)}</nav></div>)}
        <div className="sidebar-note"><div className="flex items-center gap-2"><span className="status-dot" /><span>Your personal agent</span></div><p>Ongoing goals.<br />A little less on your mind.</p></div>
      </div>
      <div className="sidebar-bottom">{destinations.filter((destination) => destination.group === "settings").map(({ command, label, icon: Icon }) => <button className="nav-button" key={command} aria-current={state.destination === command ? "page" : undefined} onClick={() => navigate(command)} disabled={state.running} title={collapsed ? label : undefined}><Icon size={17} /><span>{label}</span></button>)}<button className="nav-button" aria-current={state.destination === "/model" ? "page" : undefined} onClick={() => navigate("/model")} disabled={state.running} title="Model settings"><Settings2 size={17} /><span>Model settings</span></button><div className="profile-tile"><span className="profile-avatar">Y</span><div><strong>Personal workspace</strong><span><span className="status-dot" />{error ? "Disconnected" : "Local Gator profile"}</span></div></div></div>
    </aside>
    <main className="main-area">
      <header className="main-header"><div className="breadcrumbs"><span>{home ? "Your workspace" : panel ? panel.title : "Conversation"}</span><ChevronRight size={13} /><strong>{home ? "Home" : state.title}</strong></div><div className="header-actions"><button className="header-workspace" aria-label="Choose workspace folder" onClick={() => void window.gator?.chooseFolder().catch(() => {})} disabled={state.running || !!panel} title={state.source}><Folder size={15} /><span>{sourceName}</span><ChevronDown size={13} /></button><button className="icon-button" aria-label={details ? "Hide work details" : "Show work details"} aria-expanded={details} onClick={() => setDetails(!details)}>{details ? <PanelRightClose size={18} /> : <PanelRightOpen size={18} />}</button></div></header>
      {error && <div className="connection-error" role="alert"><span>{error}</span><button className="small-button" onClick={() => void window.gator?.restart().catch(() => {})}>Reconnect</button></div>}
      {state.notice && <div className="view-status" role="status">{state.notice}</div>}
      {panel && state.pending_approval && <div className="approval-reminder panel-approval"><ShieldCheck size={16} /><span>A current action needs your review.</span><button className="small-button" onClick={() => navigate("/chat")}>Review action<ChevronRight size={13} /></button></div>}
      {state.external && <div className="terminal-banner" role="status"><div><strong>Continue this step in Terminal</strong><p>Gator needs an interactive terminal for this {state.external.kind === "connector" ? "connection" : "action"}. Return here when it finishes.</p></div><button className="button" disabled={terminalBusy} onClick={() => void window.gator?.continueInTerminal().catch(() => {})}>{terminalBusy ? "Waiting for Terminal…" : "Open Terminal"}</button></div>}
      {panel ? <Panel panel={panel} send={send} /> : <>
        {search !== null && <div className="transcript-search"><Search size={16} /><input autoFocus aria-label="Search conversation" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Find in this conversation…" /><span>{messages.length} messages</span><button className="icon-button" aria-label="Close search" onClick={() => setSearch(null)}><X size={16} /></button></div>}
        <div className={`conversation-scroll ${home ? "home-scroll" : ""}`} ref={transcript} onScroll={(event) => { const el = event.currentTarget; atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 100; }}>
          {home ? <div className="home-content"><div className="home-eyebrow"><span className="status-dot" />A little more continuity</div><h1>What’s on your mind?</h1><p className="home-intro">Pick up where you left off, or make room for something new.</p><OverviewCards state={state} navigate={navigate} />
            {state.first_run && <div className="setup-callout"><div><Settings2 size={18} /><div><strong>Make Gator yours</strong><p>Choose a cloud or local model to start your first conversation.</p></div></div><button className="button" onClick={() => navigate("/model")}>Choose a model<ChevronRight size={15} /></button></div>}
            <div className="home-section-heading"><h2>A good place to start</h2><span>Built around your ongoing work</span></div><div className="starter-grid">{[
              { icon: Target, title: "Continue an ongoing goal", text: "Your context, decisions, and next steps.", command: "/goals" },
              { icon: ListChecks, title: "Set up a follow-up", text: "Let Gator watch for a meaningful change.", command: "/followup" },
              { icon: BookOpen, title: "Find what you’ve saved", text: "Notes, preferences, and useful context.", command: "/knowledge" },
            ].map(({ icon: Icon, title, text, command }) => <button className="starter" key={title} onClick={() => navigate(command)}><Icon size={19} /><strong>{title}</strong><p>{text}</p><ChevronRight size={15} /></button>)}</div><p className="overview-freshness">{state.overview ? `Updated ${new Date(state.overview.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : "Reading saved activity"}<span>·</span>Refreshes quietly every 15 seconds</p></div> : <div className="transcript-content" aria-label="Conversation">{messages.map((message, index) => <Message key={index} message={message} pending={approvalBusy ? undefined : state.pending_approval?.id} onApproval={(approved) => void approve(approved)} />)}{search && !messages.length && <Empty icon={<Search size={24} />} title="No matching messages" text="Try another word or close search to return to the conversation." />}{state.running && <div className="run-progress" role="status"><LoaderCircle size={16} className="spin" /><span>{state.cancelling ? "Stopping…" : state.pending_approval ? "Waiting for your approval" : state.status || "Gator is working…"}</span><span className="elapsed">{state.elapsed}</span></div>}</div>}
        </div>
        <div className="composer-area">{state.pending_approval && <div className="approval-reminder"><ShieldCheck size={16} /><span>An action is waiting for your review.</span><button className="small-button" onClick={() => { setSearch(null); transcript.current?.scrollTo({ top: transcript.current.scrollHeight }); }}>View request<ArrowDown size={13} /></button></div>}
          {!!slash.length && <div className="slash-suggestions" aria-label="Command suggestions">{slash.map((command) => <button key={command.name} onClick={() => { setDraft(`/${command.name}${command.needs_input ? " " : ""}`); composer.current?.focus(); }}><span>/{command.name}</span><span>{command.description}</span></button>)}</div>}
          <div className="composer"><textarea ref={composer} value={draft} onChange={(event) => setDraft(event.target.value)} onKeyDown={composerKeys} placeholder={state.running ? messageTiming === "guide" ? "Add guidance for the current task…" : "Queue your next request…" : "Ask Gator, or / for commands…"} aria-label="Message Gator" disabled={sending || !!error || !!state.external} rows={2} /><div className="composer-toolbar"><button className="composer-model" disabled={state.running} onClick={() => navigate("/model")}><span className="status-dot" /><span>{state.model || "Choose a model"}</span><ChevronDown size={12} /></button><div className="flex items-center gap-2">{state.running && <select className="message-timing" aria-label="When to use this message" value={messageTiming} onChange={(event) => setMessageTiming(event.target.value)}><option value="next">Next request</option><option value="guide">Guide current task</option></select>}<span className="composer-shortcut">{state.running ? `${state.queued} queued` : "↵ send"}</span>{state.running && <button className="send-button stop" aria-label="Stop current work" disabled={state.cancelling} onClick={() => attempt({ type: "key", key: "esc" })}><Square size={13} fill="currentColor" /></button>}<button className="send-button" aria-label="Send message" disabled={!draft.trim() || sending || !!error || !!state.external} onClick={() => void submit()}><ArrowUp size={18} /></button></div></div></div>
          <div className="composer-footnote"><span>{state.running ? messageTiming === "guide" ? "Guidance applies to the current task. Esc stops work." : "This request will run after the current task. Esc stops work." : home ? "Your work stays connected. You choose what happens next." : "Your next message continues this conversation. Shift+Enter for a new line."}</span><button aria-label="Open help" onClick={() => navigate("/help")}><CircleHelp size={13} /></button></div>
        </div>
      </>}
    </main>
    {details && <WorkspacePanel state={state} tab={tab} setTab={setTab} navigate={navigate} />}
    {panel && <div className="panel-draft-anchor"><MessageSquare size={14} /><span>{draft ? "Your message draft is kept while you’re here." : "Return to the conversation to write a message."}</span><button className="small-button" onClick={() => attempt({ type: "panel", key: "esc", panel: { id: panel.id!, action: "key" } })}>Back<kbd>Esc</kbd></button></div>}
    {showCommands && <CommandMenu commands={state.commands} close={() => setShowCommands(false)} select={(name, needsInput) => { setShowCommands(false); if (needsInput) navigate("/chat", `/${name} `); else navigate(`/${name}`); }} />}
    {navigation && <DiscardDialog cancel={() => setNavigation(null)} confirm={() => { attempt({ type: "navigate", text: navigation.command }); if (navigation.draft !== undefined) setDraft(navigation.draft); setNavigation(null); }} />}
  </div>;
}

function CommandMenu({ commands, close, select }: { commands: BackendState["commands"]; close(): void; select(name: string, needsInput: boolean): void }) {
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);
  const focus = useModalFocus();
  const shown = commands.filter((command) => `${command.name} ${command.description}`.toLowerCase().includes(query.toLowerCase()));
  return <div className="modal-backdrop" onClick={close}><section ref={focus} className="command-menu" role="dialog" aria-modal="true" aria-label="Search commands" onClick={(event) => event.stopPropagation()}><div className="command-search"><Search size={18} /><input autoFocus placeholder="Find a command…" aria-label="Find a command" value={query} onChange={(event) => { setQuery(event.target.value); setSelected(0); }} onKeyDown={(event) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); const next = Math.max(0, Math.min(shown.length - 1, selected + (event.key === "ArrowDown" ? 1 : -1))); setSelected(next); focus.current?.querySelectorAll(".command-list button")[next]?.scrollIntoView({ block: "nearest" }); }
    if (event.key === "Enter" && shown[selected]) { event.preventDefault(); select(shown[selected].name, shown[selected].needs_input); }
  }} /><button className="icon-button" aria-label="Close commands" onClick={close}><X size={16} /></button></div><div className="command-list">{shown.map((command, index) => <button key={command.name} data-selected={index === selected} onClick={() => select(command.name, command.needs_input)}><Command size={16} /><div><strong>/{command.name}</strong><p>{command.description}</p></div>{command.advanced && <span className="badge">Advanced</span>}<ChevronRight size={15} /></button>)}{!shown.length && <p className="empty-hint">No commands match “{query}”.</p>}</div><footer><span>↑↓ choose · Enter run · Tab move focus</span><kbd>Esc</kbd></footer></section></div>;
}

function DiscardDialog({ cancel, confirm }: { cancel(): void; confirm(): void }) {
  const focus = useModalFocus();
  return <div className="modal-backdrop"><section ref={focus} role="alertdialog" aria-modal="true" aria-labelledby="discard-title" className="confirm-dialog"><h2 id="discard-title">Leave this review?</h2><p>Your unsaved form changes will be discarded.</p><div className="flex justify-end gap-2"><button className="button" onClick={cancel}>Keep editing</button><button className="button primary" onClick={confirm}>Leave view</button></div></section></div>;
}
