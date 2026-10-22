import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowLeft, BookOpen, ChevronDown, ChevronRight, FileCode2, Files, FileText, Folder, Plug, X } from "lucide-react";
import type { BackendState } from "./protocol";
import type { PresenceRef } from "./motion";
import { Markdown } from "./Result";
import { CopyButton } from "./CopyButton";
import { useModalFocus } from "./useModalFocus";
import { bundleOutputs, conversationBundles, outputState } from "./conversationOutputs";

type SectionID = "outputs" | "context" | "connections" | "workspace";
const defaults: Record<SectionID, boolean> = { outputs: true, context: false, connections: false, workspace: false };
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

function bytes(value: number) {
  return value < 1024 ? `${value} B` : value < 1024 * 1024 ? `${(value / 1024).toFixed(1)} KB` : `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function timestamp(value?: string) {
  if (!value || !Number.isFinite(Date.parse(value))) return undefined;
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value));
}

export function WorkDetails({ state, identity, requestedOutput, open, presenceRef, close, navigate }: { state: BackendState; identity: string; requestedOutput?: { conversation: string; output: string; request: number }; open: boolean; presenceRef: PresenceRef; close(): void; navigate(command: string): void }) {
  const [sections, setSections] = useState(readSections);
  const [narrow, setNarrow] = useState(() => matchMedia("(max-width: 960px)").matches);
  const [selection, setSelection] = useState<{ conversation: string; output: string }>();
  const focus = useModalFocus(open && narrow);
  const root = useRef<HTMLElement | null>(null);
  const previewHeading = useRef<HTMLHeadingElement>(null);
  const returnToOutput = useRef<string | undefined>(undefined);
  const content = useRef<HTMLDivElement>(null);
  const listPosition = useRef(0);
  const previousIdentity = useRef(identity);
  const details = state.work_details;
  const bundles = useMemo(() => conversationBundles(state), [state.messages, details?.latest_bundle]);
  const latest = details?.latest_bundle ?? bundles.at(-1);
  const outputs = useMemo(() => bundles.flatMap((bundle, index) => bundleOutputs(bundle, index + 1)).reverse(), [bundles]);
  const selected = selection?.conversation === identity ? outputs.find((output) => output.id === selection.output) : undefined;

  useEffect(() => {
    if (!open) return;
    if (!requestedOutput) setSelection(undefined);
    else if (requestedOutput.conversation === identity) setSelection({ conversation: identity, output: requestedOutput.output });
  }, [requestedOutput?.request, identity, open]);

  useEffect(() => {
    const media = matchMedia("(max-width: 960px)");
    const change = () => setNarrow(media.matches);
    media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);
  useLayoutEffect(() => {
    if (previousIdentity.current !== identity) {
      previousIdentity.current = identity;
      if (content.current) content.current.scrollTop = 0;
    }
    if (selected) {
      if (content.current) content.current.scrollTop = 0;
      previewHeading.current?.focus({ preventScroll: true });
    }
    else if (returnToOutput.current) {
      if (content.current) content.current.scrollTop = listPosition.current;
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
    <aside ref={(element) => { root.current = element; focus.current = element; presenceRef(element); }} id="work-details-panel" className="workspace-panel" aria-label="Conversation information" role={narrow ? "dialog" : undefined} aria-modal={narrow && open ? true : undefined} data-open={open} inert={!open} aria-hidden={!open} onKeyDown={(event) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); if (selected) back(); else close(); }
      if (narrow && (event.metaKey || event.ctrlKey) && ["n", "k", "f", ","].includes(event.key.toLowerCase())) event.stopPropagation();
    }}>
      <header className="work-details-heading"><h2>Conversation information</h2><button className="icon-button" aria-label="Close conversation information" onClick={close}><X size={16} /></button></header>
      <div ref={content} className="workspace-content">
        {selected ? <section className="output-preview" aria-label="Output preview">
          <button className="small-button output-back" onClick={back}><ArrowLeft size={15} />All outputs</button>
          <h3 ref={previewHeading} tabIndex={-1}>{title}</h3>
          <div className="output-preview-meta"><span data-tone={selected.file && !selected.file.Valid ? "warning" : undefined}>{outputState(selected)}</span><span>{selected.file?.MediaType || "Proposed file changes"}</span>{selected.file?.Bytes !== undefined && <span>{bytes(selected.file.Bytes)}</span>}</div>
          {selected.bundle.Status === "failed" && <p className="work-details-notice" data-tone="warning">Retained from a failed run.</p>}
          {timestamp(selected.bundle.FinishedAt) && <p className="work-details-caption">Retained {timestamp(selected.bundle.FinishedAt)}</p>}
          {selected.file ? <>
            {selected.file.Preview ? <><div className="output-preview-toolbar"><span>Retained preview</span><CopyButton text={selected.file.Preview} label="Copy preview" /></div>{selected.file.PreviewKind === "markdown" ? <Markdown text={selected.file.Preview} /> : <pre className="output-source">{selected.file.Preview}</pre>}{selected.file.Truncated && <p className="work-details-caption">Showing a shortened preview.</p>}</> : <p className="work-details-empty">No text preview is available for this file.</p>}
            <p className="work-details-caption">File checks do not confirm external delivery.</p>
          </> : <><p className="work-details-caption">Proposed changes; this view does not apply them.</p><ul className="work-details-list">{(selected.candidate?.ChangedPaths ?? []).map((path) => <li key={path}>{path}</li>)}</ul></>}
          <details className="output-location"><summary>Retained location<ChevronDown size={14} /></summary><p>{selected.bundle.Path}</p>{selected.candidate?.PatchPath && <p>{selected.candidate.PatchPath}</p>}</details>
        </section> : <>
          <Section id="outputs" title="Outputs" icon={<Files size={16} />} summary={String(outputs.length)} open={sections.outputs} toggle={() => toggle("outputs")}>
            {outputs.length ? <div className="work-output-list">{outputs.map((output) => <button key={output.id} data-output-id={output.id} className="work-output" onClick={() => { listPosition.current = content.current?.scrollTop ?? 0; setSelection({ conversation: identity, output: output.id }); }}>
              {output.candidate ? <FileCode2 size={17} /> : <FileText size={17} />}<span><strong>{output.file?.Path || `Change ${output.candidate?.ID}`}</strong><small data-tone={output.bundle.Status === "failed" || (output.file && !output.file.Valid) ? "warning" : undefined}>{outputState(output)}{output.bundle.Status === "failed" ? " · failed run" : ""}{bundles.length > 1 && <> · <span className="keep-together">Result {output.group}</span></>}</small></span><ChevronRight size={14} />
            </button>)}</div> : <p className="work-details-empty">Files and proposed changes appear here when a task produces them.</p>}
          </Section>
          <Section id="context" title="Sources & memory" icon={<BookOpen size={16} />} open={sections.context} toggle={() => toggle("context")}>
            {!details?.attachments.length && !latest?.Sources?.length && <p className="work-details-empty">No selected files or retained source references in this conversation yet.</p>}
            {!!details?.attachments.length && <><h3 className="work-details-label">Selected for new requests</h3><ul className="work-details-list">{details.attachments.map((path) => <li key={path}>{path}</li>)}</ul></>}
            {!!latest?.Sources?.length && <><h3 className="work-details-label">Sources retained in latest result</h3><ul className="work-details-list">{latest.Sources.map((source, index) => <li key={index}>{source}</li>)}</ul><p className="work-details-caption">Recorded references are not a list of everything currently in the model’s context.</p></>}
            <div className="work-details-links"><button className="small-button" disabled={blocked} onClick={() => go(latest ? "/context" : "/knowledge")}>Inspect saved context<ChevronRight size={13} /></button><button className="small-button" disabled={blocked} onClick={() => go("/usage")}>Usage &amp; limits<ChevronRight size={13} /></button></div>
          </Section>
          <Section id="connections" title="Connections" icon={<Plug size={16} />} summary={selectedIDs.length ? `${selectedIDs.length} selected` : "None selected"} open={sections.connections} toggle={() => toggle("connections")}>
            <h3 className="work-details-label">Selected for new requests</h3>{selectedIDs.length ? <ul className="work-details-list">{selectedIDs.map((id) => <li key={id}>{id}</li>)}</ul> : <p className="work-details-empty">No connections selected.</p>}
            <p className="work-details-caption">Selection does not verify sign-in or approve an action. Queued requests keep their original selections.</p>
            <h3 className="work-details-label">Retained connection sources</h3>{used.length ? <ul className="work-connections">{used.map((connection) => <li key={connection.ID}><strong>{connection.ID}</strong><span>{connection.Operations.join(", ")}</span></li>)}</ul> : <p className="work-details-empty">No retained connection sources{latest ? " in this result" : " yet"}.</p>}{!!used.length && <p className="work-details-caption">Records can include sources carried over from earlier turns.</p>}
          </Section>
          <Section id="workspace" title="This conversation" icon={<Folder size={16} />} summary={state.source.split(/[\\/]/).filter(Boolean).at(-1) || "Local"} open={sections.workspace} toggle={() => toggle("workspace")}>
            <dl className="work-details-fields"><div><dt>Runs on</dt><dd>This computer</dd></div><div><dt>Folder</dt><dd>{state.source || "Not selected"}</dd></div>{state.model && <div><dt>Model</dt><dd>{state.model}</dd></div>}{state.provider && <div><dt>Provider</dt><dd>{state.provider}</dd></div>}{state.goal_id && <div><dt>Ongoing goal</dt><dd>{state.goal_id}</dd></div>}<div><dt>New requests</dt><dd>{{ auto: "Automatic · chosen per request", inspect: "Inspect", draft: "Draft", act: "Act · approval required" }[details?.mode || ""] || "Not reported"}</dd></div>{details?.snapshot_id && <div><dt>Snapshot</dt><dd>{details.snapshot_id}</dd></div>}</dl>
            <div className="work-details-links"><button className="small-button" disabled={blocked} onClick={() => go("/model")}>Model settings<ChevronRight size={13} /></button><button className="small-button" disabled={blocked} onClick={() => void window.gator?.chooseFolder().catch(() => {})}>Choose folder<ChevronRight size={13} /></button></div>
            <p className="work-details-caption">Choosing a different folder starts a separate topic with its own source scope.</p>
          </Section>

        </>}
      </div>
    </aside>
  </>;
}
