import { useCallback, useEffect, useMemo, useState } from "react";

const NAV = [["create", "Create"], ["runs", "Runs"], ["apps", "Apps"], ["skills", "Skills"], ["channels", "Channels"], ["actions", "Agent actions"], ["capacity", "Capacity"], ["health", "Health"]];
const STAGES = [["planner", "Plan"], ["builder", "Build"], ["verifier", "Verify"], ["deployer", "Deploy"]];

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
  return { config, token, error, login, logout };
}

function useAPI(token, logout) {
  return useCallback(async (path, options = {}) => {
    const headers = { ...jsonHeaders, ...(options.headers || {}) };
    if (token) headers.Authorization = `Bearer ${token}`;
    const response = await fetch(path, { ...options, headers });
    if (response.status === 401) { logout(); throw new Error("Your session expired. Sign in again."); }
    const text = await response.text();
    const value = text ? JSON.parse(text) : null;
    if (!response.ok) throw new Error(value?.error || `Request failed (${response.status})`);
    return value;
  }, [token, logout]);
}

function AuthGate({ auth, children }) {
  if (!auth.config) return <main className="center"><p>Loading Norbot…</p></main>;
  if (auth.config.enabled && !auth.token) return <main className="center"><section className="login"><p className="eyebrow">Norbot operator console</p><h1>Sign in to continue</h1><p>Use your operator identity to review and approve app-building work.</p>{auth.error && <p className="error">{auth.error}</p>}<button onClick={auth.login}>Sign in with OIDC</button></section></main>;
  return children;
}

function App() {
  const auth = useAuth();
  const api = useAPI(auth.token, auth.logout);
  const [page, setPage] = useState("create");
  const [runs, setRuns] = useState([]);
  const [apps, setApps] = useState([]);
  const [selectedID, setSelectedID] = useState("");
  const [details, setDetails] = useState({ events: [], revisions: [], usage: [], skills: [], deployment: null });
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const selected = useMemo(() => runs.find((run) => run.id === selectedID) || runs[0], [runs, selectedID]);
  const loadRuns = useCallback(async () => {
    try {
      const value = await api("/api/runs");
      setRuns(value);
      setSelectedID((current) => current || value[0]?.id || "");
    } catch (cause) { setError(cause.message); }
  }, [api]);
  const loadApps = useCallback(async () => {
    try { setApps(await api("/api/apps")); } catch (cause) { setError(cause.message); }
  }, [api]);
  const loadDetails = useCallback(async (run) => {
    if (!run?.id) return;
    const runID = run.id;
    try {
      const requests = [
        api(`/api/runs/${runID}/revisions`), api(`/api/runs/${runID}/usage`), api(`/api/runs/${runID}/skills`),
      ];
      const [revisions, usage, skills] = await Promise.all(requests);
      const deployment = run.stage === "deployer" || run.status === "completed" ? await api(`/api/runs/${runID}/deployment`).catch(() => null) : null;
      setDetails((current) => ({ ...current, revisions, usage, skills, deployment }));
    } catch (cause) { setError(cause.message); }
  }, [api]);
  useEffect(() => { if (!auth.config || auth.config.enabled && !auth.token) return; loadRuns(); loadApps(); }, [auth.config, auth.token, loadRuns, loadApps]);
  useEffect(() => { loadDetails(selected); }, [selected, loadDetails]);
  const setEvents = useCallback((events) => setDetails((current) => ({ ...current, events })), []);
  useRunEvents(selected?.id, auth.token, setEvents, setError);
  const act = async (action) => {
    setBusy(true); setError("");
    try { const value = await action(); setNotice("Saved."); await loadRuns(); await loadApps(); if (selected) await loadDetails(selected); return value; }
    catch (cause) { setError(cause.message); return null; } finally { setBusy(false); }
  };
  const createRun = (input) => act(async () => {
    const run = await api("/api/runs", { method: "POST", body: JSON.stringify(input) });
    setSelectedID(run.id); setPage("runs"); return run;
  });
  const approve = (input) => act(() => api(`/api/runs/${selected.id}/approval`, { method: "POST", body: JSON.stringify(input) }));
  const updateArchitecture = (architecture) => act(() => api(`/api/runs/${selected.id}/architecture`, { method: "PUT", body: JSON.stringify(architecture) }));
  const start = (runID) => act(() => api(`/api/runs/${runID}/deployment/start`, { method: "POST" }));
  const stop = (runID) => act(() => api(`/api/runs/${runID}/deployment/stop`, { method: "POST" }));
  const destroy = (runID) => act(() => api(`/api/runs/${runID}/deployment`, { method: "DELETE" }));
  const changeRun = (runID, change) => act(async () => { const run = await api(`/api/runs/${runID}/change-runs`, { method: "POST", body: JSON.stringify({ change }) }); setSelectedID(run.id); setPage("runs"); return run; });
  return <AuthGate auth={auth}><div className="app-shell"><aside><div className="brand">Norbot<span>operator console</span></div><nav>{NAV.map(([id, label]) => <button key={id} className={page === id ? "active" : ""} onClick={() => setPage(id)}>{label}</button>)}</nav><div className="aside-footer">{auth.config?.enabled && <button className="quiet" onClick={auth.logout}>Sign out</button>}<p>{plural(runs.length, "run")}</p></div></aside><main className="main"><header><div><p className="eyebrow">{NAV.find(([id]) => id === page)?.[1]}</p><h1>{page === "create" ? "Build an application" : page === "runs" ? "Review workflow" : page}</h1></div><button className="quiet" onClick={() => { loadRuns(); loadApps(); }}>Refresh</button></header>{notice && <p className="notice">{notice}</p>}{error && <p className="error">{error}</p>}{page === "create" && <CreatePage busy={busy} onCreate={createRun} />}{page === "runs" && <RunsPage runs={runs} selected={selected} details={details} busy={busy} onSelect={(id) => setSelectedID(id)} onApprove={approve} onUpdateArchitecture={updateArchitecture} onChange={changeRun} onCancel={() => act(() => api(`/api/runs/${selected.id}/cancel`, { method: "POST" }))} />}{page === "apps" && <AppsPage apps={apps} busy={busy} onStart={start} onStop={stop} onDelete={destroy} onChange={changeRun} />}{page === "skills" && <SkillsPage api={api} act={act} />}{page === "channels" && <ChannelsPage api={api} act={act} runs={runs} />}{page === "actions" && <ActionsPage api={api} act={act} />}{page === "capacity" && <CapacityPage api={api} act={act} />}{page === "health" && <HealthPage api={api} />}</main></div></AuthGate>;
}

function useRunEvents(runID, token, setEvents, setError) {
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
      } catch (cause) { if (!controller.signal.aborted) setError(cause.message); }
    };
    connect(); return () => controller.abort();
  }, [runID, token, setEvents, setError]);
}

function CreatePage({ busy, onCreate }) {
  const [prompt, setPrompt] = useState(""); const [profile, setProfile] = useState("full-stack"); const [target, setTarget] = useState("docker");
  return <section className="stack wide"><section className="card hero"><p>Describe the app, then review each explicit approval gate before anything is deployed.</p><textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder="Describe the application you want Norbot to build…" rows="7"/><div className="grid three"><Field label="Profile"><select value={profile} onChange={(event) => setProfile(event.target.value)}><option value="frontend-only">Frontend</option><option value="full-stack">Full stack</option><option value="agentic">Agentic</option></select></Field><Field label="Target"><select value={target} onChange={(event) => setTarget(event.target.value)}><option value="docker">Docker</option><option value="kubernetes">Kubernetes</option></select></Field><Field label="Approval model"><p>Plan · Code · Test/Fix · Deploy</p></Field></div><button disabled={busy || !prompt.trim()} onClick={() => onCreate({ prompt, profile, deployment_target: target })}>{busy ? "Creating…" : "Create plan"}</button></section><section className="card"><h2>What happens next</h2><ol><li>Norbot generates an editable architecture.</li><li>You approve the plan, code proposal, verification, and deployment separately.</li><li>Use linked change runs to evolve an application after approval.</li></ol></section></section>;
}

function RunsPage({ runs, selected, details, busy, onSelect, onApprove, onUpdateArchitecture, onChange, onCancel }) {
  if (!runs.length) return <Empty text="No runs yet. Create an application to start the approval workflow."/>;
  return <div className="run-layout"><section className="run-list">{runs.map((run) => <button key={run.id} className={`run-row ${selected?.id === run.id ? "selected" : ""}`} onClick={() => onSelect(run.id)}><strong>{run.architecture?.app_name || run.prompt}</strong><span>{stageName(run.stage)} · {statusName(run.status)}</span><small>{run.id.slice(0, 12)} · {run.profile}</small></button>)}</section><RunDetail run={selected} details={details} busy={busy} onApprove={onApprove} onUpdateArchitecture={onUpdateArchitecture} onChange={onChange} onCancel={onCancel}/></div>;
}

function RunDetail({ run, details, busy, onApprove, onUpdateArchitecture, onChange, onCancel }) {
  if (!run) return null;
  const revision = details.revisions?.[0]; const report = revision?.report || {}; const failed = report.status === "fail";
  const [feedback, setFeedback] = useState(""); const [change, setChange] = useState("");
  const action = () => {
    if (run.status === "failed" || run.status === "interrupted") return <button disabled={busy} onClick={() => onApprove({ action: "retry" })}>Retry {stageName(run.stage)}</button>;
    if (run.status !== "awaiting_approval") return null;
    if (run.stage === "planner") return <div className="actions"><button disabled={busy} onClick={() => onApprove({ action: "approve" })}>Approve architecture</button><input value={feedback} onChange={(event) => setFeedback(event.target.value)} placeholder="Tell the planner what to revise"/><button className="quiet" disabled={busy || !feedback.trim()} onClick={() => onApprove({ action: "revise", feedback })}>Request revision</button></div>;
    if (run.stage === "builder") return <button disabled={busy} onClick={() => onApprove({ action: "approve", revision_id: revision?.id })}>Approve code and run checks</button>;
    if (run.stage === "verifier") return failed ? <div className="actions"><input value={feedback} onChange={(event) => setFeedback(event.target.value)} placeholder="Optional fix instructions"/><button disabled={busy} onClick={() => onApprove({ action: "fix", feedback })}>Approve bounded fix</button></div> : <button disabled={busy} onClick={() => onApprove({ action: "approve", revision_id: revision?.id })}>Approve checks</button>;
    return <button disabled={busy} onClick={() => onApprove({ action: "approve" })}>Approve deployment</button>;
  };
  return <section className="stack"><section className="card"><div className="title-row"><div><p className="eyebrow">{statusName(run.status)}</p><h2>{run.architecture?.app_name || "Application run"}</h2><p>{run.prompt}</p></div>{!run.status.match(/completed|abandoned/) && <button className="danger" disabled={busy} onClick={onCancel}>Cancel</button>}</div><Stepper run={run}/>{run.failure_reason && <p className="error">{run.failure_reason}</p>}{action()}</section>{run.stage === "planner" && run.status === "awaiting_approval" ? <ArchitectureEditor architecture={run.architecture || newArchitecture(run.profile)} busy={busy} onSave={onUpdateArchitecture}/> : <ReviewPanel revision={revision} usage={details.usage} skills={details.skills} deployment={details.deployment}/>}<section className="card"><h2>Change this application</h2><div className="actions"><input value={change} onChange={(event) => setChange(event.target.value)} placeholder="Describe the requested change"/><button className="quiet" disabled={busy || !change.trim()} onClick={() => onChange(run.id, change)}>Create change run</button></div></section><EventLog events={details.events}/></section>;
}

function Stepper({ run }) { const index = STAGES.findIndex(([id]) => id === run.stage); return <div className="stepper">{STAGES.map(([id, label], position) => <div key={id} className={position < index ? "done" : position === index ? "current" : ""}><span>{position + 1}</span>{label}<small>{position === index ? statusName(run.status) : position < index ? "complete" : "upcoming"}</small></div>)}</div>; }

function ArchitectureEditor({ architecture, busy, onSave }) {
  const [draft, setDraft] = useState(architecture); const [core, setCore] = useState(featureLines(architecture.core_features)); const [optional, setOptional] = useState(featureLines(architecture.optional_features));
  useEffect(() => { setDraft(architecture); setCore(featureLines(architecture.core_features)); setOptional(featureLines(architecture.optional_features)); }, [architecture]);
  const updateNode = (index, key, value) => setDraft((current) => ({ ...current, workflow: { ...current.workflow, nodes: current.workflow.nodes.map((node, nodeIndex) => nodeIndex === index ? { ...node, [key]: value } : node) } }));
  const updateEdge = (index, key, value) => setDraft((current) => ({ ...current, workflow: { ...current.workflow, edges: current.workflow.edges.map((edge, edgeIndex) => edgeIndex === index ? { ...edge, [key]: value, id: key === "source" ? `${value}-to-${edge.target}` : key === "target" ? `${edge.source}-to-${value}` : edge.id } : edge) } }));
  const parseFeatures = (value, selected) => lines(value).map((line, index) => { const [name, description = "", role = "app_logic"] = line.split("|").map((item) => item.trim()); return { id: `${selected ? "core" : "optional"}-${index + 1}-${name.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`, name, description, role, selected }; });
  const save = () => onSave({ ...draft, stack: lines(draft.stack.join("\n")), integrations: lines(draft.integrations.join("\n")), core_features: parseFeatures(core, true), optional_features: parseFeatures(optional, false) });
  return <section className="card architecture"><div className="title-row"><div><p className="eyebrow">Planner approval</p><h2>Architecture contract</h2><p>Edit the exact scope Norbot may build, test, and deploy.</p></div><button disabled={busy} onClick={save}>Save architecture</button></div><div className="grid two"><Field label="Application name"><input value={draft.app_name} onChange={(event) => setDraft({ ...draft, app_name: event.target.value })}/></Field><Field label="Application type"><input value={draft.app_type} onChange={(event) => setDraft({ ...draft, app_type: event.target.value })}/></Field><Field label="Stack (one per line)"><textarea value={draft.stack.join("\n")} onChange={(event) => setDraft({ ...draft, stack: lines(event.target.value) })}/></Field><Field label="Integrations (one per line)"><textarea value={draft.integrations.join("\n")} onChange={(event) => setDraft({ ...draft, integrations: lines(event.target.value) })}/></Field></div><div className="grid two"><Field label="Selected core features — name | description | role"><textarea value={core} onChange={(event) => setCore(event.target.value)} rows="6"/></Field><Field label="Optional features — name | description | role"><textarea value={optional} onChange={(event) => setOptional(event.target.value)} rows="6"/></Field></div><h3>Workflow canvas</h3><div className="canvas">{draft.workflow.nodes.map((node, index) => <article key={node.id} className="node"><input value={node.label} aria-label="Node label" onChange={(event) => updateNode(index, "label", event.target.value)}/><select value={node.kind} onChange={(event) => updateNode(index, "kind", event.target.value)}><option value="input">Input</option><option value="logic">App logic</option><option value="tool">Tool</option><option value="output">Output</option></select><small>{node.id}</small><button className="icon danger" disabled={draft.workflow.nodes.length <= 2 || node.kind === "input" || node.kind === "output"} onClick={() => setDraft((current) => ({ ...current, workflow: { nodes: current.workflow.nodes.filter((_, nodeIndex) => nodeIndex !== index), edges: current.workflow.edges.filter((edge) => edge.source !== node.id && edge.target !== node.id) } }))}>Remove</button></article>)}<button className="add-node" onClick={() => setDraft((current) => { const id = `step-${current.workflow.nodes.length + 1}`; return { ...current, workflow: { ...current.workflow, nodes: [...current.workflow.nodes.slice(0, -1), { id, label: "New step", kind: "logic" }, current.workflow.nodes.at(-1)] } }; })}>+ Add workflow step</button></div><h3>Connections</h3><div className="edge-list">{draft.workflow.edges.map((edge, index) => <div key={`${edge.id}-${index}`}><select value={edge.source} onChange={(event) => updateEdge(index, "source", event.target.value)}>{draft.workflow.nodes.map((node) => <option key={node.id} value={node.id}>{node.label}</option>)}</select><span>→</span><select value={edge.target} onChange={(event) => updateEdge(index, "target", event.target.value)}>{draft.workflow.nodes.map((node) => <option key={node.id} value={node.id}>{node.label}</option>)}</select><button className="icon danger" onClick={() => setDraft((current) => ({ ...current, workflow: { ...current.workflow, edges: current.workflow.edges.filter((_, edgeIndex) => edgeIndex !== index) } }))}>Remove</button></div>)}</div><button className="quiet" onClick={() => setDraft((current) => ({ ...current, workflow: { ...current.workflow, edges: [...current.workflow.edges, { id: `${current.workflow.nodes[0]?.id}-to-${current.workflow.nodes.at(-1)?.id}`, source: current.workflow.nodes[0]?.id, target: current.workflow.nodes.at(-1)?.id }] } }))}>Add connection</button></section>;
}

function ReviewPanel({ revision, usage, skills, deployment }) { return <section className="grid two"><section className="card"><h2>Review artifact</h2>{revision ? <><p><strong>{revision.kind}</strong> · attempt {revision.attempt} · {revision.state}</p><pre>{JSON.stringify(revision.report || {}, null, 2)}</pre><details><summary>{plural(Object.keys(revision.files || {}).length, "file changed")}</summary><pre>{Object.entries(revision.files || {}).map(([path, body]) => `// ${path}\n${body}`).join("\n\n")}</pre></details></> : <p>Norbot is working. The next review artifact will appear here.</p>}</section><section className="card"><h2>Execution context</h2><p>{plural(asArray(usage).length, "usage record")}</p><pre>{JSON.stringify({ usage, selected_skills: skills, deployment }, null, 2)}</pre></section></section>; }

function EventLog({ events }) { return <section className="card"><h2>Live events</h2>{events?.length ? <div className="events">{events.map((event) => <article key={event.id}><strong>{statusName(event.type)}</strong><span>{new Date(event.created_at).toLocaleString()}</span><p>{event.message}</p>{Object.keys(event.metadata || {}).length > 0 && <details><summary>Details</summary><pre>{JSON.stringify(event.metadata, null, 2)}</pre></details>}</article>)}</div> : <p>No events yet.</p>}</section>; }

function AppsPage({ apps, busy, onStart, onStop, onDelete, onChange }) { const [change, setChange] = useState({}); if (!apps.length) return <Empty text="No deployed applications yet."/>; return <section className="cards">{apps.map((app) => <article className="card" key={app.run_id}><div className="title-row"><div><p className="eyebrow">{app.profile}</p><h2>{app.deployment.project_name || app.run_id}</h2><p>{statusName(app.deployment.status)} · {statusName(app.run_status)}</p></div><span className="badge">{app.deployment.status}</span></div>{app.deployment.public_url && <a href={app.deployment.public_url} target="_blank" rel="noreferrer">Open application ↗</a>}<div className="actions"><button disabled={busy} onClick={() => onStart(app.run_id)}>Start</button><button className="quiet" disabled={busy} onClick={() => onStop(app.run_id)}>Stop</button><button className="danger" disabled={busy} onClick={() => onDelete(app.run_id)}>Delete</button></div><div className="actions"><input value={change[app.run_id] || ""} onChange={(event) => setChange({ ...change, [app.run_id]: event.target.value })} placeholder="Describe a change"/><button className="quiet" disabled={busy || !change[app.run_id]?.trim()} onClick={() => onChange(app.run_id, change[app.run_id])}>Change run</button></div></article>)}</section>; }

function SkillsPage({ api, act }) { const [imports, setImports] = useState([]); const [form, setForm] = useState({ source_type: "git", source_uri: "", source_ref: "", credential_env: "" }); const load = useCallback(() => api("/api/skills/imports").then(setImports), [api]); useEffect(() => { load().catch(() => {}); }, [load]); return <section className="stack"><section className="card"><h2>Import a skill bundle</h2><div className="grid two"><Field label="Source"><select value={form.source_type} onChange={(event) => setForm({ ...form, source_type: event.target.value })}><option value="git">HTTPS Git</option><option value="oci">OCI</option></select></Field><Field label="Source URI"><input value={form.source_uri} onChange={(event) => setForm({ ...form, source_uri: event.target.value })} placeholder="https://…"/></Field><Field label="Reference"><input value={form.source_ref} onChange={(event) => setForm({ ...form, source_ref: event.target.value })}/></Field><Field label="Credential env reference"><input value={form.credential_env} onChange={(event) => setForm({ ...form, credential_env: event.target.value })}/></Field></div><button disabled={!form.source_uri} onClick={() => act(async () => { await api("/api/skills/imports", { method: "POST", body: JSON.stringify(form) }); await load(); })}>Import and scan</button></section><section className="card"><h2>Imports</h2><Table rows={imports} columns={["id", "source_type", "source_uri", "state", "digest"]} action={(item) => item.state === "scanned" ? <button onClick={() => act(async () => { await api(`/api/skills/imports/${item.id}/activate`, { method: "POST" }); await load(); })}>Activate</button> : null}/></section></section>; }

function ChannelsPage({ api, act, runs }) { const [accounts, setAccounts] = useState([]); const [form, setForm] = useState({ run_id: runs[0]?.id || "", adapter: "telegram", name: "", secret_refs: "{}", settings: "{}" }); const load = useCallback(() => api("/api/channels/accounts").then(setAccounts), [api]); useEffect(() => { load().catch(() => {}); }, [load]); return <section className="stack"><section className="card"><h2>Connect a channel</h2><div className="grid two"><Field label="Run"><select value={form.run_id} onChange={(event) => setForm({ ...form, run_id: event.target.value })}>{runs.map((run) => <option value={run.id} key={run.id}>{run.architecture?.app_name || run.id}</option>)}</select></Field><Field label="Provider"><select value={form.adapter} onChange={(event) => setForm({ ...form, adapter: event.target.value })}>{["telegram", "slack", "discord", "whatsapp"].map((item) => <option key={item}>{item}</option>)}</select></Field><Field label="Account name"><input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })}/></Field><Field label="Secret env references (JSON)"><input value={form.secret_refs} onChange={(event) => setForm({ ...form, secret_refs: event.target.value })}/></Field><Field label="Settings (JSON)"><input value={form.settings} onChange={(event) => setForm({ ...form, settings: event.target.value })}/></Field></div><button disabled={!form.run_id || !form.name} onClick={() => act(async () => { await api("/api/channels/accounts", { method: "POST", body: JSON.stringify({ ...form, secret_refs: JSON.parse(form.secret_refs), settings: JSON.parse(form.settings) }) }); await load(); })}>Create account</button></section><section className="card"><h2>Channel accounts</h2><Table rows={accounts} columns={["id", "adapter", "name", "run_id", "enabled"]}/></section></section>; }

function ActionsPage({ api, act }) { const [actions, setActions] = useState([]); const load = useCallback(() => api("/api/agent/actions").then(setActions), [api]); useEffect(() => { load().catch(() => {}); }, [load]); return <section className="card"><h2>Pending agent actions</h2><Table rows={actions} columns={["id", "tool", "role", "run_id", "state"]} action={(item) => <div className="actions"><button onClick={() => act(async () => { await api(`/api/agent/actions/${item.id}/decision`, { method: "POST", body: JSON.stringify({ decision: "approved" }) }); await load(); })}>Approve</button><button className="danger" onClick={() => act(async () => { await api(`/api/agent/actions/${item.id}/decision`, { method: "POST", body: JSON.stringify({ decision: "rejected" }) }); await load(); })}>Reject</button></div>}/></section>; }

function CapacityPage({ api, act }) { const [capacity, setCapacity] = useState(null); const [workers, setWorkers] = useState(2); const load = useCallback(() => api("/api/capacity").then(setCapacity), [api]); useEffect(() => { load().catch(() => {}); }, [load]); return <section className="stack"><section className="card"><h2>Capacity</h2><pre>{JSON.stringify(capacity, null, 2)}</pre></section><section className="card"><h2>Record a worker recommendation</h2><div className="actions"><input type="number" min="1" value={workers} onChange={(event) => setWorkers(Number(event.target.value))}/><button onClick={() => act(async () => { await api("/api/capacity/recommendations", { method: "POST", body: JSON.stringify({ recommended_workers: workers, factors: { source: "operator-console" } }) }); await load(); })}>Save recommendation</button></div></section></section>; }

function HealthPage({ api }) { const [health, setHealth] = useState(null); const [detail, setDetail] = useState(null); const load = useCallback(async () => { setHealth(await api("/api/health")); setDetail(await api("/api/health/detail")); }, [api]); useEffect(() => { load().catch(() => {}); }, [load]); return <section className="stack"><section className="card"><div className="title-row"><div><h2>Service health</h2><p>{health?.status || "Loading…"}</p></div><button onClick={() => load()}>Refresh</button></div></section><section className="card"><pre>{JSON.stringify(detail, null, 2)}</pre></section></section>; }

function Field({ label, children }) { return <label className="field"><span>{label}</span>{children}</label>; }
function Empty({ text }) { return <section className="card empty"><p>{text}</p></section>; }
function Table({ rows, columns, action }) { return rows?.length ? <div className="table-wrap"><table><thead><tr>{columns.map((column) => <th key={column}>{column.replaceAll("_", " ")}</th>)}{action && <th>Actions</th>}</tr></thead><tbody>{rows.map((row) => <tr key={row.id || row.run_id}>{columns.map((column) => <td key={column}>{typeof row[column] === "object" ? JSON.stringify(row[column]) : String(row[column] ?? "")}</td>)}{action && <td>{action(row)}</td>}</tr>)}</tbody></table></div> : <p>No records.</p>; }

export default App;
