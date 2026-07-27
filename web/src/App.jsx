import { useCallback, useDeferredValue, useEffect, useMemo, useState } from "react";
import "./gnome.css";
import gnomeAvatar from "../../asset/reference/cut.jpeg";

const NAV = [["runs", "Runs"], ["apps", "Apps"], ["skills", "Skills"], ["channels", "Channels"], ["actions", "Agent actions"], ["operations", "Operations"], ["capacity", "Capacity"], ["health", "Health"]];
const STAGES = [["planner", "Plan"], ["builder", "Build"], ["verifier", "Verify"], ["deployer", "Deploy"]];
const ONBOARDING_KEY = "norbot.onboarding.v1";

const jsonHeaders = { "Content-Type": "application/json" };
const asArray = (value) => Array.isArray(value) ? value : [];
const lines = (value) => String(value || "").split("\n").map((item) => item.trim()).filter(Boolean);
const plural = (count, word) => `${count} ${word}${count === 1 ? "" : "s"}`;
const stageName = (stage) => STAGES.find(([id]) => id === stage)?.[1] || stage;
const statusName = (status) => String(status || "unknown").replaceAll("_", " ");
const featureLines = (items) => asArray(items).map((item) => [item.name, item.description, item.role].join(" | ")).join("\n");

function newArchitecture(profile = "full-stack") {
  return {
    app_name: "Generated app", app_type: profile, stack: [], integrations: [],
    core_features: [{ id: "core-request", name: "Requested application", description: "Deliver the approved request.", role: "app_logic", selected: true }],
    optional_features: [],
    workflow: {
      nodes: [{ id: "input-request", label: "User request", kind: "input" }, { id: "logic-build", label: "Build app", kind: "logic" }, { id: "output-deployment", label: "Deployed app", kind: "output" }],
      edges: [{ id: "input-request-to-logic-build", source: "input-request", target: "logic-build" }, { id: "logic-build-to-output-deployment", source: "logic-build", target: "output-deployment" }]
    }
  };
}

function encodeBase64URL(bytes) {
  return btoa(String.fromCharCode(...bytes)).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
}

function randomValue() {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return encodeBase64URL(bytes);
}

async function challengeFor(verifier) {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return encodeBase64URL(new Uint8Array(digest));
}

function useAuth() {
  const [config, setConfig] = useState(null);
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const complete = useCallback(async (value) => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    if (!code || !value?.enabled) return;
    const pending = JSON.parse(sessionStorage.getItem("norbot.pkce") || "null");
    if (!pending || pending.state !== params.get("state")) throw new Error("OIDC login state did not match");
    const discovery = await fetch(`${value.issuer.replace(/\/$/, "")}/.well-known/openid-configuration`).then((response) => response.json());
    const body = new URLSearchParams({ grant_type: "authorization_code", client_id: value.client_id, code, redirect_uri: pending.redirectURI, code_verifier: pending.verifier });
    const response = await fetch(discovery.token_endpoint, { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body });
    const result = await response.json();
    if (!response.ok || !result.access_token) throw new Error(result.error_description || "OIDC token exchange failed");
    sessionStorage.removeItem("norbot.pkce");
    window.history.replaceState({}, "", window.location.pathname);
    setToken(result.access_token);
  }, []);
  useEffect(() => {
    fetch("/api/auth/config").then(async (response) => {
      if (!response.ok) throw new Error("Could not load authentication configuration");
      return response.json();
    }).then(async (value) => {
      setConfig(value);
      await complete(value);
    }).catch((cause) => setError(cause.message));
  }, [complete]);
  const login = useCallback(async () => {
    try {
      const discovery = await fetch(`${config.issuer.replace(/\/$/, "")}/.well-known/openid-configuration`).then((response) => response.json());
      const state = randomValue();
      const verifier = randomValue();
      const redirectURI = `${window.location.origin}/`;
      sessionStorage.setItem("norbot.pkce", JSON.stringify({ state, verifier, redirectURI }));
      const query = new URLSearchParams({ response_type: "code", client_id: config.client_id, redirect_uri: redirectURI, scope: (config.scopes?.length ? config.scopes : ["openid", "profile", "email"]).join(" "), state, code_challenge: await challengeFor(verifier), code_challenge_method: "S256" });
      window.location.assign(`${discovery.authorization_endpoint}?${query}`);
    } catch (cause) { setError(cause.message); }
  }, [config]);
  const logout = useCallback(() => setToken(""), []);
  const clearError = useCallback(() => setError(""), []);
  return { config, token, error, login, logout, clearError };
}

function useAPI(token, logout) {
  return useCallback(async (path, options = {}) => {
    const { allowError, ...request } = options; const headers = { ...jsonHeaders, ...(request.headers || {}) };
    if (token) headers.Authorization = `Bearer ${token}`;
    const response = await fetch(path, { ...request, headers });
    if (response.status === 401) { logout(); throw new Error("Your session expired. Sign in again."); }
    const text = await response.text();
    const value = text ? JSON.parse(text) : null;
    if (!response.ok && !allowError) throw new Error(value?.error || `Request failed (${response.status})`);
    return allowError ? { status: response.status, value } : value;
  }, [token, logout]);
}

function AuthGate({ auth, children }) {
  const errors = auth.error ? [{ id: "auth", message: auth.error, tone: "error" }] : [];
  if (!auth.config) return <><main className="center"><p>Loading Norbot…</p></main><GnomeToasts toasts={errors} onboarding={false} onDismiss={auth.clearError}/></>;
  if (auth.config.enabled && !auth.token) return <><main className="center"><section className="login"><p className="eyebrow">Norbot operator console</p><h1>Sign in to continue</h1><p>Use your operator identity to review and approve app-building work.</p><button onClick={auth.login}>Sign in with OIDC</button></section></main><GnomeToasts toasts={errors} onboarding={false} onDismiss={auth.clearError}/></>;
  return children;
}

function App() {
  const auth = useAuth();
  const api = useAPI(auth.token, auth.logout);
  const [page, setPage] = useState("runs");
  const [runs, setRuns] = useState([]);
  const [apps, setApps] = useState([]);
	const [runsCursor, setRunsCursor] = useState("");
	const [appsCursor, setAppsCursor] = useState("");
	const [runsFilter, setRunsFilter] = useState({ status: "", search: "" });
	const [appsFilter, setAppsFilter] = useState({ status: "", search: "" });
	const deferredRunsFilter = useDeferredValue(runsFilter);
	const deferredAppsFilter = useDeferredValue(appsFilter);
  const [selectedID, setSelectedID] = useState("");
  const [details, setDetails] = useState({ events: [], revisions: [], usage: [], skills: [], deployment: null, swarm: null, agentPolicy: null, policyHistory: [], trace: [], agentTurns: [], sandboxes: [] });
  const [toasts, setToasts] = useState([]);
  const [busy, setBusy] = useState(false);
  const [showOnboarding, setShowOnboarding] = useState(() => localStorage.getItem(ONBOARDING_KEY) !== "seen");
  const notify = useCallback((message, tone = "notice") => {
    const text = String(message || "").trim();
    if (!text) return;
    setToasts((current) => [...current.slice(-3), { id: `${Date.now()}-${Math.random()}`, message: text, tone }]);
  }, []);
  const dismissToast = useCallback((id) => setToasts((current) => current.filter((toast) => toast.id !== id)), []);
  const selected = useMemo(() => runs.find((run) => run.id === selectedID) || null, [runs, selectedID]);
  const loadRuns = useCallback(async (cursor = "", append = false) => {
    try {
      const query = new URLSearchParams({ limit: "50" });
      if (cursor) query.set("cursor", cursor); if (deferredRunsFilter.status) query.set("status", deferredRunsFilter.status); if (deferredRunsFilter.search) query.set("search", deferredRunsFilter.search);
      const value = await api(`/api/runs?${query}`);
      setRuns((current) => append ? [...current, ...asArray(value.items)] : asArray(value.items));
      setRunsCursor(value.next_cursor || "");
    } catch (cause) { notify(cause.message, "error"); }
  }, [api, notify, deferredRunsFilter]);
  const loadApps = useCallback(async (cursor = "", append = false) => {
    try { const query = new URLSearchParams({ limit: "50" }); if (cursor) query.set("cursor", cursor); if (deferredAppsFilter.status) query.set("status", deferredAppsFilter.status); if (deferredAppsFilter.search) query.set("search", deferredAppsFilter.search); const value = await api(`/api/apps?${query}`); setApps((current) => append ? [...current, ...asArray(value.items)] : asArray(value.items)); setAppsCursor(value.next_cursor || ""); } catch (cause) { notify(cause.message, "error"); }
  }, [api, notify, deferredAppsFilter]);
  const loadDetails = useCallback(async (run) => {
    if (!run?.id) return;
    const runID = run.id;
    try {
      const requests = [
        api(`/api/runs/${runID}/revisions`), api(`/api/runs/${runID}/usage`), api(`/api/runs/${runID}/skills`), api(`/api/runs/${runID}/planning-swarm`), api(`/api/runs/${runID}/agent-policy`), api(`/api/runs/${runID}/agent-policy-history`), api(`/api/runs/${runID}/trace?limit=100`), api(`/api/runs/${runID}/agent-turns?limit=50`), api(`/api/runs/${runID}/sandboxes`),
      ];
      const [revisions, usage, skills, swarm, agentPolicy, policyHistory, trace, agentTurns, sandboxes] = await Promise.all(requests);
      const deployment = run.stage === "deployer" || run.status === "completed" ? await api(`/api/runs/${runID}/deployment`).catch(() => null) : null;
      setDetails((current) => ({ ...current, revisions, usage, skills, deployment, swarm, agentPolicy, policyHistory, trace: asArray(trace.items), agentTurns: asArray(agentTurns.items), sandboxes }));
    } catch (cause) { notify(cause.message, "error"); }
  }, [api, notify]);
  useEffect(() => { if (!auth.config || auth.config.enabled && !auth.token) return; loadRuns(); loadApps(); }, [auth.config, auth.token, loadRuns, loadApps]);
  useEffect(() => { loadDetails(selected); }, [selected, loadDetails]);
  const setEvents = useCallback((events) => setDetails((current) => ({ ...current, events })), []);
  useRunEvents(selected?.id, auth.token, setEvents, notify);
  const act = async (action, success) => {
    setBusy(true);
    try { const value = await action(); await loadRuns(); await loadApps(); if (success) notify(success); return value; }
    catch (cause) { notify(cause.message, "error"); return null; } finally { setBusy(false); }
  };
  const createRun = (input) => act(async () => {
    const run = await api("/api/runs", { method: "POST", body: JSON.stringify(input) });
    setSelectedID(""); setPage("runs"); return run;
  }, "Plan created.");
  const approve = (runID, input) => act(() => api(`/api/runs/${runID}/approval`, { method: "POST", body: JSON.stringify(input) }), input.action === "retry" ? "Stage retry queued." : input.action === "revise" ? "Revision requested." : input.action === "fix" ? "Bounded fix approved." : "Approval recorded.");
  const updateArchitecture = (architecture) => act(() => api(`/api/runs/${selected.id}/architecture`, { method: "PUT", body: JSON.stringify(architecture) }), "Architecture saved.");
  const selectSwarmCandidate = (runID, taskID) => act(async () => { const value = await api(`/api/runs/${runID}/planning-swarm/select`, { method: "POST", body: JSON.stringify({ task_id: taskID }) }); await loadDetails({ id: runID }); return value; }, "Planning candidate selected for approval.");
  const start = (runID) => act(() => api(`/api/runs/${runID}/deployment/start`, { method: "POST" }), "Application started.");
  const stop = (runID) => act(() => api(`/api/runs/${runID}/deployment/stop`, { method: "POST" }), "Application stopped.");
  const destroy = (runID) => act(() => api(`/api/runs/${runID}/deployment`, { method: "DELETE" }), "Deployment deleted.");
  const removeRun = (runID) => act(() => api(`/api/runs/${runID}`, { method: "DELETE" }), "Run removed.");
  const changeRun = (runID, change, architectureAffecting = false) => act(async () => { const run = await api(`/api/runs/${runID}/change-runs`, { method: "POST", body: JSON.stringify({ change, architecture_affecting: architectureAffecting }) }); setSelectedID(""); setPage("runs"); return run; }, architectureAffecting ? "Architecture change run created." : "Change run created from approved snapshot.");
  const restrictAgentPolicy = (runID, policy) => act(async () => { const value = await api(`/api/runs/${runID}/agent-policy`, { method: "PUT", body: JSON.stringify(policy) }); await loadDetails({ id: runID, stage: selected?.stage, status: selected?.status }); return value; }, "Agent capabilities restricted.");
  const dismissOnboarding = () => { localStorage.setItem(ONBOARDING_KEY, "seen"); setShowOnboarding(false); };
  return <AuthGate auth={auth}><div className="app-shell"><aside><div className="brand">Norbot<span>operator console</span></div><nav>{NAV.map(([id, label]) => <button key={id} className={page === id ? "active" : ""} onClick={() => setPage(id)}>{label}</button>)}</nav><div className="aside-footer">{auth.config?.enabled && <button className="quiet" onClick={auth.logout}>Sign out</button>}<p>{plural(runs.length, "loaded run")}</p></div></aside><main className="main"><header><div><p className="eyebrow">{page === "create" ? "New run" : NAV.find(([id]) => id === page)?.[1]}</p><h1>{page === "create" ? "Build an application" : page === "runs" ? "Runs" : page}</h1></div><button className="quiet" onClick={() => { loadRuns(); loadApps(); }}>Refresh</button></header>{page === "create" && <CreatePage api={api} busy={busy} notify={notify} onCreate={createRun} onBack={() => setPage("runs")} />}{page === "runs" && <RunsPage api={api} runs={runs} filter={runsFilter} onFilter={setRunsFilter} selected={selected} details={details} busy={busy} onCreate={() => setPage("create")} onLoadMore={runsCursor ? () => loadRuns(runsCursor, true) : null} onSelect={(id) => setSelectedID((current) => current === id ? "" : id)} onExpand={setSelectedID} onApprove={approve} onUpdateArchitecture={updateArchitecture} onSelectSwarmCandidate={selectSwarmCandidate} onChange={changeRun} onSaveAgentPolicy={restrictAgentPolicy} onCancel={(runID) => act(() => api(`/api/runs/${runID}/cancel`, { method: "POST" }), "Run cancelled.")} onRemove={removeRun} />}{page === "apps" && <AppsPage apps={apps} filter={appsFilter} onFilter={setAppsFilter} busy={busy} onLoadMore={appsCursor ? () => loadApps(appsCursor, true) : null} onStart={start} onStop={stop} onDelete={destroy} onChange={changeRun} onManage={(runID) => api(`/api/runs/${runID}`).then((run) => { setRuns((current) => current.some((item) => item.id === run.id) ? current : [run, ...current]); setSelectedID(run.id); setPage("runs"); }).catch((cause) => notify(cause.message, "error"))} />}{page === "skills" && <SkillsPage api={api} act={act} notify={notify} />}{page === "channels" && <ChannelsPage api={api} act={act} runs={runs} notify={notify} />}{page === "actions" && <ActionsPage api={api} act={act} notify={notify} />}{page === "operations" && <OperationsPage api={api} act={act} notify={notify} />}{page === "capacity" && <CapacityPage api={api} act={act} notify={notify} />}{page === "health" && <HealthPage api={api} notify={notify} />}</main><GnomeToasts toasts={toasts} onboarding={showOnboarding} onDismiss={dismissToast} onDismissOnboarding={dismissOnboarding}/></div></AuthGate>;
}

function useRunEvents(runID, token, setEvents, notify) {
  useEffect(() => {
    if (!runID) return undefined;
    const controller = new AbortController();
    const connect = async () => {
      try {
        const response = await fetch(`/api/runs/${runID}/events`, { headers: token ? { Authorization: `Bearer ${token}` } : {}, signal: controller.signal });
        if (!response.ok || !response.body) throw new Error("Could not stream run events");
        const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ""; const received = [];
        while (!controller.signal.aborted) {
          const { value, done } = await reader.read(); if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const blocks = buffer.split("\n\n"); buffer = blocks.pop() || "";
          blocks.forEach((block) => { const raw = block.split("\n").find((line) => line.startsWith("data: "))?.slice(6); if (raw) { try { received.push(JSON.parse(raw)); } catch {} } });
          if (received.length) setEvents([...received]);
        }
      } catch (cause) { if (!controller.signal.aborted) notify(cause.message, "warning"); }
    };
    connect(); return () => controller.abort();
  }, [runID, token, setEvents, notify]);
}

function CreatePage({ api, busy, notify, onCreate, onBack }) {
  const [prompt, setPrompt] = useState(""); const [profile, setProfile] = useState("full-stack"); const [target, setTarget] = useState("docker"); const [imports, setImports] = useState([]); const [skillDigests, setSkillDigests] = useState([]); const [providers, setProviders] = useState([]); const [selectedProviders, setSelectedProviders] = useState({});
  const loadOptions = useCallback(async () => {
    const [skillImports, providerOptions] = await Promise.all([api("/api/skills/imports"), api("/api/providers")]);
    setImports(skillImports); setProviders(providerOptions);
    setSelectedProviders((current) => {
      const next = { ...current };
      for (const [stage] of STAGES.slice(0, 3)) {
        const options = asArray(providerOptions).filter((item) => asArray(item.stages).includes(stage));
        if (!options.some((item) => item.id === next[stage])) next[stage] = options[0]?.id || "";
      }
      return next;
    });
  }, [api]);
  useEffect(() => { loadOptions().catch((cause) => notify(cause.message, "error")); }, [loadOptions, notify]);
  const activeSkills = useMemo(() => [...new Map(imports.filter((item) => item.state === "active").map((item) => [item.digest, item])).values()], [imports]);
  const toggleSkill = (digest) => setSkillDigests((current) => current.includes(digest) ? current.filter((item) => item !== digest) : [...current, digest]);
  const optionsFor = (stage) => providers.filter((item) => asArray(item.stages).includes(stage));
  const setProvider = (stage, value) => setSelectedProviders((current) => ({ ...current, [stage]: value }));
  const selected = Object.fromEntries(Object.entries(selectedProviders).filter(([, value]) => value));
  return <section className="stack wide"><section className="card hero"><div className="title-row"><div><h2>New run</h2><p>Describe the app, select a configured provider per stage, then review each explicit approval gate before anything is deployed.</p></div><button className="quiet" onClick={onBack}>− Cancel</button></div><textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder="Describe the application you want Norbot to build…" rows="7"/><div className="grid three"><Field label="Profile"><select value={profile} onChange={(event) => setProfile(event.target.value)}><option value="frontend-only">Frontend</option><option value="full-stack">Full stack</option><option value="agentic">Agentic</option></select></Field><Field label="Target"><select value={target} onChange={(event) => setTarget(event.target.value)}><option value="docker">Docker</option><option value="kubernetes">Kubernetes</option></select></Field><Field label="Approval model"><p>Plan · Code · Test/Fix · Deploy</p></Field></div><section className="skill-picker"><div className="title-row"><div><h3>Stage providers</h3><p>Only adapters enabled in <code>config.json</code> appear. Credentials stay in environment variables.</p></div><button className="quiet" onClick={() => loadOptions().catch((cause) => notify(cause.message, "error"))}>Refresh options</button></div><div className="grid three">{STAGES.slice(0, 3).map(([stage, label]) => <Field key={stage} label={label}><select value={selectedProviders[stage] || ""} onChange={(event) => setProvider(stage, event.target.value)} disabled={!optionsFor(stage).length}>{optionsFor(stage).map((item) => <option key={item.id} value={item.id}>{item.id} · {item.model || item.kind}</option>)}</select></Field>)}</div></section><section className="skill-picker"><div className="title-row"><div><h3>Approved skills</h3><p>Selected skills are copied read-only into this run’s isolated workspace.</p></div><button className="quiet" onClick={() => loadOptions().catch((cause) => notify(cause.message, "error"))}>Refresh skills</button></div>{activeSkills.length ? <div className="skill-options">{activeSkills.map((item) => <label key={item.digest}><input type="checkbox" checked={skillDigests.includes(item.digest)} onChange={() => toggleSkill(item.digest)}/><span><strong>{item.findings?.name || item.bundle_path || item.digest.slice(7, 19)}</strong><small>{item.mode} · {item.bundle_path || "."}</small></span></label>)}</div> : <p>No active skills. Import and activate skills first.</p>}</section><button disabled={busy || !prompt.trim()} onClick={() => onCreate({ prompt, profile, providers: selected, deployment_target: target, skill_digests: skillDigests })}>{busy ? "Creating…" : "Create plan"}</button></section></section>;
}

function GnomeToasts({ toasts, onboarding, onDismiss, onDismissOnboarding }) { return <div className="gnome-toasts" aria-live="polite">{onboarding && <GnomeToast title="What happens next" tone="instruction" persistent onDismiss={onDismissOnboarding}><ol><li>Norbot generates an editable architecture.</li><li>You approve the plan, code proposal, verification, and deployment separately.</li><li>Use linked change runs to evolve an application after approval.</li></ol><button autoFocus onClick={onDismissOnboarding}>Continue</button></GnomeToast>}{toasts.map((toast) => <GnomeToast key={toast.id} title={toast.tone === "error" ? "Gnome alert" : toast.tone === "warning" ? "Gnome warning" : "Gnome note"} tone={toast.tone} message={toast.message} onDismiss={() => onDismiss(toast.id)}/>)}</div>; }

function GnomeToast({ title, tone, message, persistent, children, onDismiss }) { useEffect(() => { if (persistent) return undefined; const timer = window.setTimeout(onDismiss, 7000); return () => window.clearTimeout(timer); }, [persistent, onDismiss]); return <article className={`gnome-toast ${tone}`}><img className="gnome" src={gnomeAvatar} alt=""/><div><p className="eyebrow">{tone === "instruction" ? "Welcome to Norbot" : title}</p><h2>{title}</h2>{message && <p>{message}</p>}{children}</div><button className="toast-dismiss" aria-label={`Dismiss ${title}`} onClick={onDismiss}>×</button></article>; }

function RunsPage({ api, runs, filter, onFilter, selected, details, busy, onCreate, onLoadMore, onSelect, onExpand, onApprove, onUpdateArchitecture, onSelectSwarmCandidate, onChange, onSaveAgentPolicy, onCancel, onRemove }) {
  const live = runs.filter((run) => !["failed", "interrupted", "abandoned", "completed"].includes(run.status)); const finished = runs.filter((run) => run.status === "completed"); const failed = runs.filter((run) => ["failed", "interrupted", "abandoned"].includes(run.status));
  const group = (label, items) => items.length ? <section className="run-list"><p className="eyebrow">{label} · {plural(items.length, "run")}</p>{items.map((run) => <article className={`run-entry ${selected?.id === run.id ? "selected" : ""}`} key={run.id}><div className={`run-row ${selected?.id === run.id ? "selected" : ""}`}><button className="run-summary" onClick={() => onSelect(run.id)}><strong>{run.architecture?.app_name || run.prompt}</strong><span>{stageName(run.stage)} · {statusName(run.status)}</span><small>{selected?.id === run.id ? "− Hide details" : "+ Details"}</small></button><RunCompactActions run={run} busy={busy} onOpen={() => onExpand(run.id)} onRetry={() => onApprove(run.id, { action: "retry" })} onCancel={() => onCancel(run.id)} onRemove={() => onRemove(run.id)}/></div>{selected?.id === run.id && <div className="run-expanded"><RunDetail api={api} run={run} details={details} busy={busy} onApprove={(input) => onApprove(run.id, input)} onUpdateArchitecture={onUpdateArchitecture} onSelectSwarmCandidate={(taskID) => onSelectSwarmCandidate(run.id, taskID)} onChange={onChange} onSaveAgentPolicy={onSaveAgentPolicy} onCancel={() => onCancel(run.id)} onRemove={() => onRemove(run.id)}/></div>}</article>)}</section> : null;
  return <section className="stack"><div className="title-row"><div><h2>All runs</h2><p>Each run shows its current state. Expand one for its workflow and history.</p></div><button onClick={onCreate}>+ New run</button></div><ListFilters value={filter} statuses={["queued", "running", "awaiting_approval", "completed", "failed", "interrupted", "abandoned"]} onApply={onFilter}/>{runs.length ? <section className="run-directory">{group("Live", live)}{group("Finished", finished)}{group("Failed / stopped", failed)}{onLoadMore && <button className="quiet" onClick={onLoadMore}>Load more runs</button>}</section> : <Empty text="No runs match this filter."/>}</section>;
}

function RunCompactActions({ run, busy, onOpen, onRetry, onCancel, onRemove }) {
  const remove = () => { if (window.confirm(`Remove ${run.architecture?.app_name || "this run"} and all run history?`)) onRemove(); };
  if (run.status === "failed" || run.status === "interrupted") return <div className="compact-actions"><button disabled={busy} onClick={onRetry}>Retry {stageName(run.stage)}</button><button className="quiet" onClick={onOpen}>Review details</button><button className="danger" disabled={busy} onClick={remove}>− Remove</button></div>;
  if (["queued", "running"].includes(run.status)) return <div className="compact-actions"><button className="danger" disabled={busy} onClick={onCancel}>− Cancel</button><button className="quiet" onClick={onOpen}>View progress</button></div>;
  if (run.status === "awaiting_approval") return <div className="compact-actions"><button className="quiet" onClick={onOpen}>Review {stageName(run.stage)}</button></div>;
  if (["abandoned", "completed"].includes(run.status)) return <div className="compact-actions"><button className="quiet" onClick={onOpen}>View details</button><button className="danger" disabled={busy} onClick={remove}>− Remove</button></div>;
  return <div className="compact-actions"><button className="quiet" onClick={onOpen}>View details</button></div>;
}

function RunDetail({ api, run, details, busy, onApprove, onUpdateArchitecture, onSelectSwarmCandidate, onChange, onSaveAgentPolicy, onCancel, onRemove }) {
  if (!run) return null;
  const revision = details.revisions?.[0]; const report = revision?.report || {}; const failed = report.status === "fail";
  const [feedback, setFeedback] = useState(""); const [change, setChange] = useState(""); const [architectureAffecting, setArchitectureAffecting] = useState(false);
  const cancel = () => { if (window.confirm(`Cancel ${run.architecture?.app_name || "this run"}?`)) onCancel(); };
  const remove = () => { if (window.confirm(`Remove ${run.architecture?.app_name || "this run"} and all run history?`)) onRemove(); };
  const action = () => {
    if (run.status === "failed" || run.status === "interrupted") return <button disabled={busy} onClick={() => onApprove({ action: "retry" })}>Retry {stageName(run.stage)}</button>;
    if (run.status !== "awaiting_approval") return null;
    if (run.stage === "planner") return <div className="actions"><button disabled={busy} onClick={() => onApprove({ action: "approve" })}>Approve architecture</button><input value={feedback} onChange={(event) => setFeedback(event.target.value)} placeholder="Tell the planner what to revise"/><button className="quiet" disabled={busy || !feedback.trim()} onClick={() => onApprove({ action: "revise", feedback })}>Request revision</button></div>;
    if (run.stage === "builder") return <button disabled={busy} onClick={() => onApprove({ action: "approve", revision_id: revision?.id })}>Approve code and run checks</button>;
    if (run.stage === "verifier") return failed ? <div className="actions"><input value={feedback} onChange={(event) => setFeedback(event.target.value)} placeholder="Optional fix instructions"/><button disabled={busy} onClick={() => onApprove({ action: "fix", feedback })}>Approve bounded fix</button></div> : <button disabled={busy} onClick={() => onApprove({ action: "approve", revision_id: revision?.id })}>Approve checks</button>;
    return <button disabled={busy} onClick={() => onApprove({ action: "approve" })}>Approve deployment</button>;
  };
  return <section className="stack"><section className="card"><div className="title-row"><div><p className="eyebrow">{statusName(run.status)}</p><h2>{run.architecture?.app_name || "Application run"}</h2><p>{run.prompt}</p></div>{run.status.match(/failed|interrupted|abandoned|completed/) ? <button className="danger" disabled={busy} onClick={remove}>− Remove</button> : <button className="danger" disabled={busy} onClick={cancel}>− Cancel</button>}</div><Stepper run={run}/>{action()}</section>{run.stage === "planner" && run.status === "awaiting_approval" ? <><PlanningSwarmPanel swarm={details.swarm} busy={busy} onSelect={onSelectSwarmCandidate}/><ArchitectureEditor architecture={run.architecture || newArchitecture(run.profile)} busy={busy} onSave={onUpdateArchitecture}/></> : <ReviewPanel revision={revision} usage={details.usage} skills={details.skills} deployment={details.deployment}/>}<TracePanel api={api} run={run} trace={details.trace} turns={details.agentTurns} sandboxes={details.sandboxes} policyHistory={details.policyHistory}/><AgentControlsPanel run={run} policy={details.agentPolicy} busy={busy} onSave={onSaveAgentPolicy}/>{run.status === "completed" && <section className="card"><h2>Change this application</h2><p>Ordinary changes start from this approved snapshot and skip planning. Select architecture change only when the system design must change.</p><div className="actions"><input value={change} onChange={(event) => setChange(event.target.value)} placeholder="Describe the requested change"/><label><input type="checkbox" checked={architectureAffecting} onChange={(event) => setArchitectureAffecting(event.target.checked)}/> Architecture change — require planner</label><button className="quiet" disabled={busy || !change.trim()} onClick={() => onChange(run.id, change, architectureAffecting)}>Create change run</button></div></section>}<EventLog events={details.events}/></section>;
}

function PlanningSwarmPanel({ swarm, busy, onSelect }) { if (!swarm?.tasks?.length) return null; return <section className="card"><h2>Planning candidates</h2><p>{swarm.state === "completed" ? "Candidates are ranked automatically. You can select another valid candidate before approving the architecture." : `Swarm state: ${statusName(swarm.state)}.`}</p><div className="swarm-candidates">{swarm.tasks.map((task) => <article key={task.id} className={task.id === swarm.selected_task_id ? "selected" : ""}><div><p className="eyebrow">{task.role} · {statusName(task.state)}{task.score ? ` · ${task.score}/100` : ""}</p><h3>{task.architecture?.app_name || "No valid architecture"}</h3><p>{task.rationale || task.error || "Candidate pending."}</p>{task.rank_reason && <small>{task.rank_reason}</small>}</div>{task.state === "completed" && task.id !== swarm.selected_task_id && <button className="quiet" disabled={busy} onClick={() => onSelect(task.id)}>Use candidate</button>}</article>)}</div>{swarm.ranker_error && <small>Ranker fallback: {swarm.ranker_error}</small>}</section>; }

function Stepper({ run }) { const index = STAGES.findIndex(([id]) => id === run.stage); return <div className="stepper">{STAGES.map(([id, label], position) => <div key={id} className={position < index ? "done" : position === index ? "current" : ""}><span>{position + 1}</span>{label}</div>)}</div>; }

function ArchitectureEditor({ architecture, busy, onSave }) {
  const [draft, setDraft] = useState(architecture); const [core, setCore] = useState(featureLines(architecture.core_features)); const [optional, setOptional] = useState(featureLines(architecture.optional_features));
  useEffect(() => { setDraft(architecture); setCore(featureLines(architecture.core_features)); setOptional(featureLines(architecture.optional_features)); }, [architecture]);
  const updateNode = (index, key, value) => setDraft((current) => ({ ...current, workflow: { ...current.workflow, nodes: current.workflow.nodes.map((node, nodeIndex) => nodeIndex === index ? { ...node, [key]: value } : node) } }));
  const updateEdge = (index, key, value) => setDraft((current) => ({ ...current, workflow: { ...current.workflow, edges: current.workflow.edges.map((edge, edgeIndex) => edgeIndex === index ? { ...edge, [key]: value, id: key === "source" ? `${value}-to-${edge.target}` : key === "target" ? `${edge.source}-to-${value}` : edge.id } : edge) } }));
  const parseFeatures = (value, selected) => lines(value).map((line, index) => { const [name, description = "", role = "app_logic"] = line.split("|").map((item) => item.trim()); return { id: `${selected ? "core" : "optional"}-${index + 1}-${name.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`, name, description, role, selected }; });
  const save = () => onSave({ ...draft, stack: lines(draft.stack.join("\n")), integrations: lines(draft.integrations.join("\n")), core_features: parseFeatures(core, true), optional_features: parseFeatures(optional, false) });
  return <section className="card architecture"><div className="title-row"><div><p className="eyebrow">Planner approval</p><h2>Architecture contract</h2><p>Edit the exact scope Norbot may build, test, and deploy.</p></div><button disabled={busy} onClick={save}>Save architecture</button></div><div className="grid two"><Field label="Application name"><input value={draft.app_name} onChange={(event) => setDraft({ ...draft, app_name: event.target.value })}/></Field><Field label="Application type"><input value={draft.app_type} onChange={(event) => setDraft({ ...draft, app_type: event.target.value })}/></Field><Field label="Stack (one per line)"><textarea value={draft.stack.join("\n")} onChange={(event) => setDraft({ ...draft, stack: lines(event.target.value) })}/></Field><Field label="Integrations (one per line)"><textarea value={draft.integrations.join("\n")} onChange={(event) => setDraft({ ...draft, integrations: lines(event.target.value) })}/></Field></div><div className="grid two"><Field label="Selected core features — name | description | role"><textarea value={core} onChange={(event) => setCore(event.target.value)} rows="6"/></Field><Field label="Optional features — name | description | role"><textarea value={optional} onChange={(event) => setOptional(event.target.value)} rows="6"/></Field></div><h3>Workflow canvas</h3><div className="canvas">{draft.workflow.nodes.map((node, index) => <article key={node.id} className="node"><input value={node.label} aria-label="Node label" onChange={(event) => updateNode(index, "label", event.target.value)}/><select value={node.kind} onChange={(event) => updateNode(index, "kind", event.target.value)}><option value="input">Input</option><option value="logic">App logic</option><option value="tool">Tool</option><option value="output">Output</option></select><small>{node.id}</small><button className="icon danger" disabled={draft.workflow.nodes.length <= 2 || node.kind === "input" || node.kind === "output"} onClick={() => setDraft((current) => ({ ...current, workflow: { nodes: current.workflow.nodes.filter((_, nodeIndex) => nodeIndex !== index), edges: current.workflow.edges.filter((edge) => edge.source !== node.id && edge.target !== node.id) } }))}>Remove</button></article>)}<button className="add-node" onClick={() => setDraft((current) => { const id = `step-${current.workflow.nodes.length + 1}`; return { ...current, workflow: { ...current.workflow, nodes: [...current.workflow.nodes.slice(0, -1), { id, label: "New step", kind: "logic" }, current.workflow.nodes.at(-1)] } }; })}>+ Add workflow step</button></div><h3>Connections</h3><div className="edge-list">{draft.workflow.edges.map((edge, index) => <div key={`${edge.id}-${index}`}><select value={edge.source} onChange={(event) => updateEdge(index, "source", event.target.value)}>{draft.workflow.nodes.map((node) => <option key={node.id} value={node.id}>{node.label}</option>)}</select><span>→</span><select value={edge.target} onChange={(event) => updateEdge(index, "target", event.target.value)}>{draft.workflow.nodes.map((node) => <option key={node.id} value={node.id}>{node.label}</option>)}</select><button className="icon danger" onClick={() => setDraft((current) => ({ ...current, workflow: { ...current.workflow, edges: current.workflow.edges.filter((_, edgeIndex) => edgeIndex !== index) } }))}>Remove</button></div>)}</div><button className="quiet" onClick={() => setDraft((current) => ({ ...current, workflow: { ...current.workflow, edges: [...current.workflow.edges, { id: `${current.workflow.nodes[0]?.id}-to-${current.workflow.nodes.at(-1)?.id}`, source: current.workflow.nodes[0]?.id, target: current.workflow.nodes.at(-1)?.id }] } }))}>Add connection</button></section>;
}

function AgentControlsPanel({ run, policy, busy, onSave }) {
  const [draft, setDraft] = useState(policy);
  useEffect(() => setDraft(policy), [policy?.digest, policy?.version]);
  if (!draft) return <section className="card"><h2>Agent controls</h2><p>Loading capability policy…</p></section>;
  const updateStage = (stage, key, value) => setDraft((current) => ({ ...current, stages: { ...current.stages, [stage]: { ...current.stages[stage], [key]: value } } }));
  const updateTool = (tool, key, value) => setDraft((current) => ({ ...current, tools: { ...current.tools, [tool]: { ...current.tools[tool], [key]: value } } }));
  const updateList = (tool, key, value) => updateTool(tool, key, value.split(",").map((item) => item.trim()).filter(Boolean));
  return <section className="card agent-controls"><div className="title-row"><div><h2>Agent controls</h2><p>Capabilities are enforced server-side. Save only removes permissions for this run; it cannot restore them. Pending actions are checked again before execution.</p></div><button disabled={busy} onClick={() => onSave(run.id, draft)}>Save restrictions</button></div><h3>Internal workflow agents</h3><div className="agent-control-grid">{STAGES.map(([stage, label]) => { const value = draft.stages?.[stage] || {}; return <fieldset key={stage}><legend>{label} · {run.providers?.[stage] || "runtime"}</legend><label><input type="checkbox" checked={!!value.enabled} onChange={(event) => updateStage(stage, "enabled", event.target.checked)}/> Enabled</label><label><input type="checkbox" checked={!!value.allow_model} onChange={(event) => updateStage(stage, "allow_model", event.target.checked)}/> Model</label><label><input type="checkbox" checked={!!value.allow_network} onChange={(event) => updateStage(stage, "allow_network", event.target.checked)}/> Network</label><label><input type="checkbox" checked={!!value.allow_cli} onChange={(event) => updateStage(stage, "allow_cli", event.target.checked)}/> Local runtime</label></fieldset>; })}</div>{run.profile === "agentic" ? <><h3>Agentic app functions</h3><p><code>http_get</code> is an allowlisted HTTPS fetch. Generic web search is not configured as a Norbot tool.</p><div className="stack">{Object.entries(draft.tools || {}).sort(([a], [b]) => a.localeCompare(b)).map(([name, tool]) => <fieldset className="agent-tool" key={name}><legend>{name}</legend><div className="agent-tool-flags"><label><input type="checkbox" checked={!!tool.enabled} onChange={(event) => updateTool(name, "enabled", event.target.checked)}/> Enabled</label><label><input type="checkbox" checked={!!tool.approval_required} disabled={!!tool.approval_required} onChange={(event) => updateTool(name, "approval_required", event.target.checked)}/> Approval required</label><label>Max calls<input type="number" min="1" max={tool.max_calls || 1} value={tool.max_calls || 1} onChange={(event) => updateTool(name, "max_calls", Number(event.target.value || 1))}/></label></div><div className="grid two"><Field label="Roles"><input value={(tool.roles || []).join(", ")} onChange={(event) => updateList(name, "roles", event.target.value)}/></Field><Field label="HTTPS hosts"><input value={(tool.allowed_hosts || []).join(", ")} onChange={(event) => updateList(name, "allowed_hosts", event.target.value)}/></Field><Field label="Commands"><input value={(tool.allowed_commands || []).join(", ")} onChange={(event) => updateList(name, "allowed_commands", event.target.value)}/></Field><Field label="Path prefixes"><input value={(tool.allowed_path_prefixes || []).join(", ")} onChange={(event) => updateList(name, "allowed_path_prefixes", event.target.value)}/></Field></div></fieldset>)}</div></> : <p>This app is not agentic. Its internal planning, building, verification, and deployment agents remain controlled above.</p>}<p className="eyebrow">Policy v{draft.version} · {draft.digest}</p></section>;
}

function ReviewPanel({ revision, usage, skills, deployment }) { return <section className="grid two"><section className="card"><h2>Review artifact</h2>{revision ? <><p><strong>{revision.kind}</strong> · attempt {revision.attempt} · {revision.state}</p><pre>{JSON.stringify(revision.report || {}, null, 2)}</pre><details><summary>{plural(Object.keys(revision.files || {}).length, "file changed")}</summary><pre>{Object.entries(revision.files || {}).map(([path, body]) => `// ${path}\n${body}`).join("\n\n")}</pre></details></> : <p>Norbot is working. The next review artifact will appear here.</p>}</section><section className="card"><h2>Execution context</h2><p>{plural(asArray(usage).length, "usage record")}</p><pre>{JSON.stringify({ usage, selected_skills: skills, deployment }, null, 2)}</pre></section></section>; }

function TracePanel({ api, run, trace, turns, sandboxes, policyHistory }) {
  const [query, setQuery] = useState(""); const [visible, setVisible] = useState(trace || []); const [raw, setRaw] = useState({}); const [loading, setLoading] = useState(false);
  useEffect(() => { setVisible(trace || []); setRaw({}); }, [run.id, trace]);
  const search = async (event) => { event.preventDefault(); setLoading(true); try { const params = new URLSearchParams({ limit: "100" }); if (query.trim()) params.set("q", query.trim()); const result = await api(`/api/runs/${run.id}/trace?${params}`); setVisible(asArray(result.items)); } finally { setLoading(false); } };
  const reveal = async (id) => { if (!window.confirm("Reveal locally retained raw forensic data?")) return; const value = await api(`/api/runs/${run.id}/trace/${id}/raw`); setRaw((current) => ({ ...current, [id]: value.raw })); };
  const exportTrace = async () => { const value = await api(`/api/runs/${run.id}/trace/export?format=json`); const blob = new Blob([JSON.stringify(value, null, 2)], { type: "application/json" }); const url = URL.createObjectURL(blob); const link = document.createElement("a"); link.href = url; link.download = `norbot-trace-${run.id}.json`; link.click(); URL.revokeObjectURL(url); };
  return <section className="card trace-panel"><div className="title-row"><div><p className="eyebrow">Operator traceback</p><h2>Unified trace</h2><p>Workflow, provider, revision, approval, policy, turn, action, and sandbox records share IDs here.</p></div><button className="quiet" onClick={exportTrace}>Export JSON</button></div><form className="actions" onSubmit={search}><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search redacted trace"/><button className="quiet" disabled={loading}>{loading ? "Searching…" : "Search"}</button></form><div className="trace-summary"><span>{plural(visible.length, "trace event")}</span><span>{plural(asArray(turns).length, "agent turn")}</span><span>{plural(asArray(sandboxes).length, "sandbox execution")}</span><span>{plural(asArray(policyHistory).length, "policy version")}</span></div>{visible.length ? <div className="trace-events">{visible.map((event) => <article key={event.id}><div><strong>{statusName(event.type)}</strong><small>{new Date(event.occurred_at).toLocaleString()} · {event.stage || "operator"}{event.provider_id ? ` · ${event.provider_id}` : ""}</small><p>{event.summary}</p></div><div className="trace-ids">{event.trace_id && <code title="OpenTelemetry trace ID">{event.trace_id}</code>}{event.turn_id && <code>turn:{event.turn_id.slice(0, 8)}</code>}{event.action_id && <code>action:{event.action_id.slice(0, 8)}</code>}</div><details><summary>Details</summary><pre>{JSON.stringify({ status: event.status, refs: event.entity_refs, payload: event.payload, trace_id: event.trace_id, span_id: event.span_id }, null, 2)}</pre>{event.raw_available && (raw[event.id] ? <pre>{JSON.stringify(raw[event.id], null, 2)}</pre> : <button className="danger" onClick={() => reveal(event.id)}>Reveal local raw data</button>)}</details></article>)}</div> : <p>No trace records match this query.</p>}<details className="trace-history"><summary>Agent turns, sandboxes, and policy history</summary><pre>{JSON.stringify({ turns, sandboxes, policy_history: policyHistory }, null, 2)}</pre></details></section>;
}

function EventLog({ events }) { return <section className="card"><h2>Live events</h2>{events?.length ? <div className="events">{events.map((event) => <article key={event.id}><strong>{statusName(event.type)}</strong><span>{new Date(event.created_at).toLocaleString()}</span><p>{event.message}</p>{Object.keys(event.metadata || {}).length > 0 && <details><summary>Details</summary><pre>{JSON.stringify(event.metadata, null, 2)}</pre></details>}</article>)}</div> : <p>No events yet.</p>}</section>; }

function AppsPage({ apps, filter, onFilter, busy, onLoadMore, onStart, onStop, onDelete, onChange, onManage }) { const [change, setChange] = useState({}); const [architectureChanges, setArchitectureChanges] = useState({}); return <section className="stack"><ListFilters value={filter} statuses={["running", "stopped", "deleted", "failed"]} onApply={onFilter}/>{apps.length ? <section className="cards">{apps.map((app) => <article className="card" key={app.app_id || app.run_id}><div className="title-row"><div><p className="eyebrow">{app.profile}</p><h2>{app.deployment.project_name || app.app_id || app.run_id}</h2><p>{statusName(app.deployment.status)} · {statusName(app.run_status)}</p></div><span className="badge">{app.deployment.status}</span></div>{app.deployment.public_url && <a href={app.deployment.public_url} target="_blank" rel="noreferrer">Open application ↗</a>}<div className="actions"><button disabled={busy} onClick={() => onStart(app.run_id)}>Start</button><button className="quiet" disabled={busy} onClick={() => onStop(app.run_id)}>Stop</button><button className="danger" disabled={busy} onClick={() => onDelete(app.run_id)}>Delete</button>{app.profile === "agentic" && <button className="quiet" onClick={() => onManage(app.run_id)}>Agent controls</button>}</div><div className="actions"><input value={change[app.run_id] || ""} onChange={(event) => setChange({ ...change, [app.run_id]: event.target.value })} placeholder="Describe a change"/><label><input type="checkbox" checked={Boolean(architectureChanges[app.run_id])} onChange={(event) => setArchitectureChanges({ ...architectureChanges, [app.run_id]: event.target.checked })}/> Architecture change</label><button className="quiet" disabled={busy || !change[app.run_id]?.trim()} onClick={() => onChange(app.run_id, change[app.run_id], Boolean(architectureChanges[app.run_id]))}>Change run</button></div></article>)}{onLoadMore && <button className="quiet" onClick={onLoadMore}>Load more applications</button>}</section> : <Empty text="No deployed applications match this filter."/>}</section>; }

function ListFilters({ value, statuses, onApply }) { const [draft, setDraft] = useState(value); useEffect(() => setDraft(value), [value]); return <form className="list-filters" onSubmit={(event) => { event.preventDefault(); onApply({ status: draft.status, search: draft.search.trim() }); }}><input value={draft.search} onChange={(event) => setDraft({ ...draft, search: event.target.value })} placeholder="Search"/><select value={draft.status} onChange={(event) => setDraft({ ...draft, status: event.target.value })}><option value="">All states</option>{statuses.map((status) => <option key={status} value={status}>{statusName(status)}</option>)}</select><button className="quiet">Apply</button><button type="button" className="quiet" onClick={() => { const empty = { status: "", search: "" }; setDraft(empty); onApply(empty); }}>Clear</button></form>; }

function SkillsPage({ api, act, notify }) { const [imports, setImports] = useState([]); const [form, setForm] = useState({ source_type: "git", source_uri: "", source_ref: "", credential_env: "", mode: "auto", follow_readme_links: false }); const load = useCallback(() => api("/api/skills/imports").then(setImports), [api]); useEffect(() => { load().catch((cause) => notify(cause.message, "error")); }, [load, notify]); const importBundle = async () => { const result = await act(async () => { const value = await api("/api/skills/imports", { method: "POST", body: JSON.stringify(form) }); await load(); return value; }, null); if (result) { const catalogue = result.catalogue; const detail = catalogue ? `; scanned ${plural(catalogue.repositories_scanned, "linked repository")}${catalogue.repositories_skipped ? `; deferred ${plural(catalogue.repositories_skipped, "link")}` : ""}` : ""; notify(`Quarantined ${plural(result.imports?.length || 0, "skill")}${detail}${result.rejected?.length ? `; skipped ${plural(result.rejected.length, "unsafe bundle")}` : ""}. Review provenance before activation.`, result.rejected?.length ? "warning" : "notice"); } }; return <section className="stack"><section className="card"><h2>Import skills from a repository</h2><p>Norbot recursively discovers each <code>SKILL.md</code>. Imports are quarantined as untrusted text. Tool access remains configured by Norbot, never by a skill.</p><div className="grid two"><Field label="Source"><select value={form.source_type} onChange={(event) => setForm({ ...form, source_type: event.target.value })}><option value="git">HTTPS Git</option><option value="oci">OCI</option></select></Field><Field label="Source URI"><input value={form.source_uri} onChange={(event) => setForm({ ...form, source_uri: event.target.value })} placeholder="https://github.com/org/repository.git"/></Field><Field label="Reference"><input value={form.source_ref} onChange={(event) => setForm({ ...form, source_ref: event.target.value })} placeholder="branch, tag, or immutable commit"/></Field><Field label="Import mode"><select value={form.mode} onChange={(event) => setForm({ ...form, mode: event.target.value })}><option value="auto">Auto — native where possible, otherwise adapt</option><option value="native">Native only — require skill.json</option><option value="adapted">Adapted only — derive safe metadata</option></select></Field><Field label="README catalogue"><span className="catalogue-toggle"><input type="checkbox" checked={form.follow_readme_links} onChange={(event) => setForm({ ...form, follow_readme_links: event.target.checked })}/><span>Follow up to 32 public GitHub repository links; credentials are never forwarded.</span></span></Field><Field label="Credential env reference"><input value={form.credential_env} onChange={(event) => setForm({ ...form, credential_env: event.target.value })} placeholder="Optional private-source token env"/></Field></div><button disabled={!form.source_uri} onClick={importBundle}>Import and scan</button></section><section className="card"><h2>Imported skills</h2><p>Activation records the reviewing operator. Only reviewed imports can be selected by a run.</p><Table rows={imports} columns={["id", "trust_level", "mode", "resolved_ref", "tree_digest", "bundle_path", "state"]} action={(item) => item.state === "scanned" ? <button onClick={() => act(async () => { await api(`/api/skills/imports/${item.id}/activate`, { method: "POST" }); await load(); }, "Skill reviewed and activated.")}>Review + activate</button> : null}/></section></section>; }

function ChannelsPage({ api, act, runs, notify }) {
  const [accounts, setAccounts] = useState([]); const [showForm, setShowForm] = useState(false); const [form, setForm] = useState({ run_id: runs[0]?.id || "", adapter: "telegram", name: "", secret_refs: "{}", settings: "{}" });
  const load = useCallback(() => api("/api/channels/accounts").then(setAccounts), [api]);
  useEffect(() => { load().catch((cause) => notify(cause.message, "error")); }, [load, notify]);
  useEffect(() => { setForm((current) => current.run_id || !runs[0]?.id ? current : { ...current, run_id: runs[0].id }); }, [runs]);
  const runName = (account) => runs.find((run) => run.id === account.run_id)?.architecture?.app_name || account.run_id.slice(0, 12);
  const disconnect = (account) => {
    if (!window.confirm(`Disconnect ${account.name}?`)) return;
    act(async () => { await api(`/api/channels/accounts/${account.id}`, { method: "DELETE" }); await load(); }, "Channel disconnected.");
  };
  return <section className="stack"><section className="card"><div className="title-row"><div><h2>Connected channels</h2><p>Each channel is attached to one application run.</p></div><button className="quiet" aria-expanded={showForm} onClick={() => setShowForm((current) => !current)}>{showForm ? "− Cancel" : "+ Connect channel"}</button></div>{accounts.length ? <div className="channel-list">{accounts.map((account) => <article className="channel-account" key={account.id}><div><p className="eyebrow">{account.adapter}</p><h3>{account.name}</h3><p>Attached to {runName(account)}</p></div><div className="channel-actions"><span className="badge">{account.enabled ? "connected" : "disabled"}</span><button className="danger" aria-label={`Disconnect ${account.name}`} onClick={() => disconnect(account)}>− Disconnect</button></div><small>Webhook: /api/channels/{account.id}/webhook</small></article>)}</div> : <p>No connected channels. Use + Connect channel to add one.</p>}</section>{showForm && <section className="card"><h2>Connect a channel</h2><div className="grid two"><Field label="Run"><select value={form.run_id} onChange={(event) => setForm({ ...form, run_id: event.target.value })}>{runs.map((run) => <option value={run.id} key={run.id}>{run.architecture?.app_name || run.id}</option>)}</select></Field><Field label="Provider"><select value={form.adapter} onChange={(event) => setForm({ ...form, adapter: event.target.value })}>{["telegram", "slack", "discord", "whatsapp"].map((item) => <option key={item}>{item}</option>)}</select></Field><Field label="Account name"><input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })}/></Field><Field label="Secret env references (JSON)"><input value={form.secret_refs} onChange={(event) => setForm({ ...form, secret_refs: event.target.value })}/></Field><Field label="Settings (JSON)"><input value={form.settings} onChange={(event) => setForm({ ...form, settings: event.target.value })}/></Field></div><button disabled={!form.run_id || !form.name} onClick={() => act(async () => { await api("/api/channels/accounts", { method: "POST", body: JSON.stringify({ ...form, secret_refs: JSON.parse(form.secret_refs), settings: JSON.parse(form.settings) }) }); setShowForm(false); await load(); }, "Channel connected.")}>Create account</button></section>}</section>;
}

function ActionsPage({ api, act, notify }) { const [actions, setActions] = useState([]); const load = useCallback(() => api("/api/agent/actions").then(setActions), [api]); useEffect(() => { load().catch((cause) => notify(cause.message, "error")); }, [load, notify]); return <section className="card"><h2>Pending agent actions</h2><p>Each decision is bound to its stored tool parameters and expires automatically.</p><Table rows={actions} columns={["id", "tool", "role", "run_id", "digest", "expires_at"]} action={(item) => <div className="actions"><button onClick={() => act(async () => { await api(`/api/agent/actions/${item.id}/decision`, { method: "POST", body: JSON.stringify({ decision: "approved" }) }); await load(); }, "Agent action approved.")}>Approve</button><button className="danger" onClick={() => act(async () => { await api(`/api/agent/actions/${item.id}/decision`, { method: "POST", body: JSON.stringify({ decision: "rejected" }) }); await load(); }, "Agent action rejected.")}>Reject</button></div>}/></section>; }

function OperationsPage({ api, act, notify }) { const [dead, setDead] = useState([]); const [reasons, setReasons] = useState({}); const load = useCallback(() => api("/api/operations/outbox/dead").then(setDead), [api]); useEffect(() => { load().catch((cause) => notify(cause.message, "error")); }, [load, notify]); return <section className="card"><div className="title-row"><div><h2>Dead-letter outbox</h2><p>Replay only after identifying and recording the cause. Delivery receipts prevent duplicate internal sink processing.</p></div><button className="quiet" onClick={() => load().catch((cause) => notify(cause.message, "error"))}>Refresh</button></div><Table rows={dead} columns={["id", "event_type", "run_id", "attempts", "last_error", "dead_lettered_at"]} action={(item) => <div className="actions"><input value={reasons[item.id] || ""} onChange={(event) => setReasons({ ...reasons, [item.id]: event.target.value })} placeholder="Replay reason"/><button disabled={(reasons[item.id] || "").trim().length < 4} onClick={() => act(async () => { await api(`/api/operations/outbox/${item.id}/replay`, { method: "POST", body: JSON.stringify({ reason: reasons[item.id] }) }); await load(); }, "Dead-letter event replayed.")}>Replay</button></div>}/></section>; }

function CapacityPage({ api, act, notify }) {
  const [capacity, setCapacity] = useState(null); const [workers, setWorkers] = useState(2); const [showJSON, setShowJSON] = useState(false);
  const load = useCallback(() => api("/api/capacity").then(setCapacity), [api]); useEffect(() => { load().catch((cause) => notify(cause.message, "error")); }, [load, notify]);
  return <section className="stack"><section className="card"><div className="title-row"><div><p className="eyebrow">Runtime</p><h2>Capacity</h2></div><button className="quiet" onClick={() => setShowJSON((current) => !current)}>{showJSON ? "Hide JSON" : "Show JSON"}</button></div>{showJSON ? <pre>{JSON.stringify(capacity, null, 2)}</pre> : capacity ? <><div className="metrics"><Metric label="Target" value={statusName(capacity.target)}/><Metric label="CPU" value={`${capacity.cpus} cores`}/><Metric label="Memory" value={formatBytes(capacity.memory_bytes)}/><Metric label="Workers" value={`${capacity.configured_workers} configured`}/><Metric label="Recommended" value={`${capacity.recommended_workers} workers`}/><Metric label="Quota" value={`${capacity.quota_workers} workers`}/><Metric label="Docker" value={capacity.docker_available ? "available" : "unavailable"}/><Metric label="Kubernetes" value={capacity.kubernetes_available ? "available" : "unavailable"}/></div><p className="capacity-note">{capacity.recommendation}</p><h3>Provider limits</h3><div className="table-wrap"><table><thead><tr><th>Provider</th><th>Concurrent</th><th>Requests/min</th></tr></thead><tbody>{asArray(capacity.quota_factors).map((factor) => <tr key={factor.provider_id}><td>{factor.provider_id}</td><td>{factor.configured_limit}</td><td>{factor.configured_requests_per_minute}</td></tr>)}</tbody></table></div></> : <p>Loading capacity…</p>}</section><section className="card"><h2>Record a worker recommendation</h2><div className="actions"><input type="number" min="1" value={workers} onChange={(event) => setWorkers(Number(event.target.value))}/><button onClick={() => act(async () => { await api("/api/capacity/recommendations", { method: "POST", body: JSON.stringify({ recommended_workers: workers, factors: { source: "operator-console" } }) }); await load(); }, "Worker recommendation saved.")}>Save recommendation</button></div></section></section>;
}

function HealthPage({ api, notify }) {
  const [health, setHealth] = useState(null); const [detail, setDetail] = useState(null);
  const load = useCallback(async () => { const [service, report] = await Promise.all([api("/api/health"), api("/api/health/detail", { allowError: true })]); setHealth(service); setDetail(report.value); }, [api]);
  useEffect(() => { load().catch((cause) => notify(cause.message, "error")); }, [load, notify]);
  const backend = health?.status === "ok" ? { id: "backend", state: "healthy", message: "control-plane API responding" } : { id: "backend", state: "down", message: "control-plane API is unavailable" };
  const checks = [{ id: "frontend", state: "healthy", message: "operator console loaded" }, backend, ...asArray(detail?.checks)];
  return <section className="stack"><section className="card"><div className="title-row"><div><p className="eyebrow">Control plane</p><h2>Service health</h2><p>{detail ? `Overall state: ${detail.state}` : "Loading service checks…"}</p></div><button onClick={() => load().catch((cause) => notify(cause.message, "error"))}>Refresh</button></div></section><section className="card"><h2>Detected services</h2><div className="health-checks">{checks.map((check) => <HealthCheck check={check} key={check.id}/>)}</div></section></section>;
}

function Field({ label, children }) { return <label className="field"><span>{label}</span>{children}</label>; }
function Metric({ label, value }) { return <article className="metric"><span>{label}</span><strong>{value}</strong></article>; }
function HealthCheck({ check }) { return <article className="health-check"><div><p className="eyebrow">{healthLabel(check)}</p><h3>{check.message || "No detail returned"}</h3>{check.diagnostics?.credential_env && <small>env: {check.diagnostics.credential_env}</small>}</div><div className="health-state"><span className="badge">{check.state}</span>{Number.isFinite(check.latency_ms) && <small>{check.latency_ms} ms</small>}</div></article>; }
function healthLabel(check) { if (check.id === "frontend") return "Frontend"; if (check.id === "backend") return "Backend API"; if (check.id === "postgres") return "PostgreSQL"; if (check.id === "queue") return "Job queue"; if (check.id === "runtime") return check.diagnostics?.target === "docker" ? "Docker" : "Kubernetes"; if (check.id === "marketplace") return "Skill marketplace"; if (check.id === "channels") return "Channel gateway"; if (check.id.startsWith("provider:")) return `${statusName(check.id.slice(9))} API key & provider`; if (check.id.startsWith("deployment:")) return `Deployment ${check.id.slice(11, 23)}`; return statusName(check.id); }
function formatBytes(value) { return Number.isFinite(value) ? `${(value / 1073741824).toFixed(1)} GiB` : "unknown"; }
function Empty({ text }) { return <section className="card empty"><p>{text}</p></section>; }
function Table({ rows, columns, action }) { return rows?.length ? <div className="table-wrap"><table><thead><tr>{columns.map((column) => <th key={column}>{column.replaceAll("_", " ")}</th>)}{action && <th>Actions</th>}</tr></thead><tbody>{rows.map((row) => <tr key={row.id || row.run_id}>{columns.map((column) => <td key={column}>{typeof row[column] === "object" ? JSON.stringify(row[column]) : String(row[column] ?? "")}</td>)}{action && <td>{action(row)}</td>}</tr>)}</tbody></table></div> : <p>No records.</p>; }

export default App;
