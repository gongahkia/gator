import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowLeft, ChartNoAxesCombined, ChevronDown, ChevronRight, FileCode2, Files, FileText, Folder, ListChecks, Plug, X } from "lucide-react";
import type { BackendBundle, BackendState } from "../../ui/src/protocol";
import type { PresenceRef } from "./motion";
import { Markdown } from "./Result";
import { CopyButton } from "./CopyButton";
import { useModalFocus } from "./useModalFocus";

type SectionID = "outputs" | "context" | "connections" | "workspace" | "checks";
type Output = { id: string; bundle: BackendBundle; group: number; file?: NonNullable<BackendBundle["Artifacts"]>[number]; candidate?: NonNullable<BackendBundle["Candidates"]>[number] };
const defaults: Record<SectionID, boolean> = { outputs: true, context: false, connections: false, workspace: false, checks: false };
const preferenceKey = "gator:work-details-sections";

function readSections() {
  try {
    const value = JSON.parse(localStorage.getItem(preferenceKey) || "null");
    return Object.fromEntries(Object.entries(defaults).map(([key, fallback]) => [key, typeof value?.[key] === "boolean" ? value[key] : fallback])) as typeof defaults;
  } catch { return defaults; }
}

function Section({ id, title, icon, summary, open, toggle, children }: { id: SectionID; title: string; icon: ReactNode; summary?: string; open: boolean; toggle(): void; children: ReactNode }) {
  return <section className="work-section" aria-label={title}>
    <button className="work-section-toggle" aria-expanded={open} aria-controls={`work-section-${id}`} onClick={toggle}>{icon}<span>{title}</span><small>{summary}</small><ChevronDown size={14} /></button>
    <div id={`work-section-${id}`} className="work-section-body" hidden={!open}>{children}</div>
  </section>;
}

function outputState(output: Output) {
  if (output.candidate) return `Proposal · ${output.candidate.Status || "unreported checks"}`;
  return output.file?.Valid ? "Checks passed" : "Needs review";
}

function bytes(value: number) {
  return value < 1024 ? `${value} B` : value < 1024 * 1024 ? `${(value / 1024).toFixed(1)} KB` : `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function timestamp(value?: string) {
  if (!value || !Number.isFinite(Date.parse(value))) return undefined;
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value));
}

export function WorkDetails({ state, open, presenceRef, close, navigate }: { state: BackendState; open: boolean; presenceRef: PresenceRef; close(): void; navigate(command: string): void }) {
  const [sections, setSections] = useState(readSections);
  const [narrow, setNarrow] = useState(() => matchMedia("(max-width: 960px)").matches);
  const [selection, setSelection] = useState<{ conversation: string; output: string }>();
  const identity = state.conversation_id || state.source;
  const focus = useModalFocus(open && narrow);
  const root = useRef<HTMLElement | null>(null);
  const previewHeading = useRef<HTMLHeadingElement>(null);
  const returnToOutput = useRef<string | undefined>(undefined);
  const details = state.work_details;
  const bundles = useMemo(() => {
    const values = new Map<string, BackendBundle>();
    for (const message of state.messages) if (message.bundle?.Path) values.set(message.bundle.Path, message.bundle);
    if (details?.latest_bundle?.Path) values.set(details.latest_bundle.Path, details.latest_bundle);
    return [...values.values()];
  }, [state.messages, details?.latest_bundle]);
  const latest = details?.latest_bundle ?? bundles.at(-1);
  const outputs = useMemo(() => bundles.flatMap((bundle, index) => [
    ...(bundle.Artifacts ?? []).map((file) => ({ id: JSON.stringify([bundle.Path, "file", file.Path]), bundle, group: index + 1, file })),
    ...(bundle.Candidates ?? []).map((candidate) => ({ id: JSON.stringify([bundle.Path, "change", candidate.ID]), bundle, group: index + 1, candidate })),
  ] as Output[]).reverse(), [bundles]);
  const selected = selection?.conversation === identity ? outputs.find((output) => output.id === selection.output) : undefined;

  useEffect(() => {
    const media = matchMedia("(max-width: 960px)");
    const change = () => setNarrow(media.matches);
    media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);
  useLayoutEffect(() => {
    if (selected) previewHeading.current?.focus({ preventScroll: true });
    else if (returnToOutput.current) {
      const button = [...(root.current?.querySelectorAll<HTMLButtonElement>("[data-output-id]") ?? [])].find((element) => element.dataset.outputId === returnToOutput.current);
      button?.focus({ preventScroll: true }); returnToOutput.current = undefined;
    }
  }, [selected?.id, identity]);
  const toggle = (id: SectionID) => {
    const next = { ...sections, [id]: !sections[id] }; setSections(next);
    try { localStorage.setItem(preferenceKey, JSON.stringify(next)); } catch { /* Disclosure still works when preferences cannot be saved. */ }
  };
  const back = () => { returnToOutput.current = selected?.id; setSelection(undefined); };
  const selectedIDs = details?.connector_ids ?? [];
  const used = latest?.Connections ?? [];
  const title = selected?.file?.Path ?? (selected?.candidate ? `Change ${selected.candidate.ID}` : "");
  const blocked = state.running || !!state.external || state.mode === "panel";
  const go = (command: string) => { close(); navigate(command); };

  return <>
    {narrow && open && <div className="work-details-scrim" aria-hidden="true" onClick={close} />}
    <aside ref={(element) => { root.current = element; focus.current = element; presenceRef(element); }} id="work-details-panel" className="workspace-panel" aria-label="Work details" role={narrow ? "dialog" : undefined} aria-modal={narrow && open ? true : undefined} data-open={open} inert={!open} aria-hidden={!open} onKeyDown={(event) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); if (selected) back(); else close(); }
      if (narrow && (event.metaKey || event.ctrlKey) && ["n", "k", "f", ","].includes(event.key.toLowerCase())) event.stopPropagation();
    }}>
      <header className="work-details-heading"><h2>Work details</h2><button className="icon-button" aria-label="Close work details" onClick={close}><X size={16} /></button></header>
      <div className="workspace-content">
        {selected ? <section className="output-preview" aria-label="Output preview">
          <button className="small-button output-back" onClick={back}><ArrowLeft size={15} />All outputs</button>
          <h3 ref={previewHeading} tabIndex={-1}>{title}</h3>
          <div className="output-preview-meta"><span data-tone={selected.file && !selected.file.Valid ? "warning" : undefined}>{outputState(selected)}</span><span>{selected.file?.MediaType || "Proposed file changes"}</span>{selected.file?.Bytes !== undefined && <span>{bytes(selected.file.Bytes)}</span>}</div>
          {selected.bundle.Status === "failed" && <p className="work-details-notice" data-tone="warning">Retained from a failed run.</p>}
          {timestamp(selected.bundle.FinishedAt) && <p className="work-details-caption">Retained {timestamp(selected.bundle.FinishedAt)}</p>}
          {selected.file ? <>
            {selected.file.Preview ? <><div className="output-preview-toolbar"><span>Retained preview</span><CopyButton text={selected.file.Preview} label="Copy preview" /></div>{selected.file.PreviewKind === "markdown" ? <Markdown text={selected.file.Preview} /> : <pre className="output-source">{selected.file.Preview}</pre>}{selected.file.Truncated && <p className="work-details-caption">Showing a shortened preview.</p>}</> : <p className="work-details-empty">No text preview is available for this file.</p>}
            <p className="work-details-caption">File checks do not confirm external delivery.</p>
          </> : <><p className="work-details-caption">Proposed changes; this view does not apply them.</p><ul className="work-details-list">{selected.candidate?.ChangedPaths.map((path) => <li key={path}>{path}</li>)}</ul></>}
          <details className="output-location"><summary>Retained location<ChevronDown size={14} /></summary><p>{selected.bundle.Path}</p>{selected.candidate?.PatchPath && <p>{selected.candidate.PatchPath}</p>}</details>
        </section> : <>
          <Section id="outputs" title="Outputs" icon={<Files size={16} />} summary={String(outputs.length)} open={sections.outputs} toggle={() => toggle("outputs")}>
            {outputs.length ? <div className="work-output-list">{outputs.map((output) => <button key={output.id} data-output-id={output.id} className="work-output" onClick={() => setSelection({ conversation: identity, output: output.id })}>
              {output.candidate ? <FileCode2 size={17} /> : <FileText size={17} />}<span><strong>{output.file?.Path || `Change ${output.candidate?.ID}`}</strong><small data-tone={output.bundle.Status === "failed" || (output.file && !output.file.Valid) ? "warning" : undefined}>{outputState(output)}{output.bundle.Status === "failed" ? " · failed run" : ""}{bundles.length > 1 ? ` · Result ${output.group}` : ""}</small></span><ChevronRight size={14} />
            </button>)}</div> : <p className="work-details-empty">Files and proposed changes appear here when a task produces them.</p>}
          </Section>
          <Section id="context" title="Context" icon={<ChartNoAxesCombined size={16} />} summary="Not measured" open={sections.context} toggle={() => toggle("context")}>
            <p className="work-details-copy">Context capacity isn’t measured yet. Daily token usage is tracked separately.</p>
            {!!details?.attachments.length && <><h3 className="work-details-label">Selected for new requests</h3><ul className="work-details-list">{details.attachments.map((path) => <li key={path}>{path}</li>)}</ul></>}
            {!!latest?.Sources?.length && <><h3 className="work-details-label">Recorded sources · latest retained run</h3><ul className="work-details-list">{latest.Sources.map((source, index) => <li key={index}>{source}</li>)}</ul><p className="work-details-caption">Recorded references are not a list of everything currently in the model’s context.</p></>}
            <div className="work-details-links"><button className="small-button" disabled={blocked} onClick={() => go("/context")}>Inspect saved context<ChevronRight size={13} /></button><button className="small-button" disabled={blocked} onClick={() => go("/usage")}>Usage &amp; limits<ChevronRight size={13} /></button></div>
          </Section>
          <Section id="connections" title="Connections" icon={<Plug size={16} />} summary={selectedIDs.length ? `${selectedIDs.length} selected` : "None selected"} open={sections.connections} toggle={() => toggle("connections")}>
            <h3 className="work-details-label">Selected for new requests</h3>{selectedIDs.length ? <ul className="work-details-list">{selectedIDs.map((id) => <li key={id}>{id}</li>)}</ul> : <p className="work-details-empty">No connections selected.</p>}
            <p className="work-details-caption">Selection does not verify sign-in or approve an action. Queued requests keep their original selections.</p>
            <h3 className="work-details-label">Recorded use · latest retained run</h3>{used.length ? <ul className="work-connections">{used.map((connection) => <li key={connection.ID}><strong>{connection.ID}</strong><span>{connection.Operations.join(", ")}</span></li>)}</ul> : <p className="work-details-empty">No connection use recorded{latest ? " in this run" : " yet"}.</p>}
          </Section>
          <Section id="workspace" title="Workspace" icon={<Folder size={16} />} summary={state.source.split(/[\\/]/).filter(Boolean).at(-1) || "Local"} open={sections.workspace} toggle={() => toggle("workspace")}>
            <dl className="work-details-fields"><div><dt>Runs on</dt><dd>This computer</dd></div><div><dt>Folder</dt><dd>{state.source || "Not selected"}</dd></div>{state.model && <div><dt>Model</dt><dd>{state.model}</dd></div>}{state.provider && <div><dt>Provider</dt><dd>{state.provider}</dd></div>}{state.goal_id && <div><dt>Ongoing goal</dt><dd>{state.goal_id}</dd></div>}<div><dt>New requests</dt><dd>{{ auto: "Automatic · chosen per request", inspect: "Inspect", draft: "Draft", act: "Act · approval required" }[details?.mode || ""] || "Not reported"}</dd></div>{details?.snapshot_id && <div><dt>Snapshot</dt><dd>{details.snapshot_id}</dd></div>}</dl>
          </Section>
          {!!(state.checks?.length || state.goal_id || state.supervisor_needs_start) && <Section id="checks" title="Follow-up checks" icon={<ListChecks size={16} />} summary={String(state.checks?.length ?? 0)} open={sections.checks} toggle={() => toggle("checks")}>
            {state.checks?.length ? state.checks.map((check, index) => <div className="work-check" key={index}><strong>{check.title}</strong><span data-tone={check.tone}>{check.state}</span><p>{check.health}</p></div>) : <p className="work-details-empty">No recurring checks attached.</p>}{state.supervisor_needs_start && <p className="work-details-notice" data-tone="warning">The local supervisor needs to start before configured checks can run.</p>}<button className="small-button" disabled={blocked} onClick={() => go("/followup")}>Manage checks<ChevronRight size={13} /></button>
          </Section>}
        </>}
      </div>
    </aside>
  </>;
}
