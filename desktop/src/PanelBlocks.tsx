import { useState } from "react";
import { ChevronDown, ExternalLink } from "lucide-react";
import type { BackendPanel, ResultField } from "./protocol";
import { ShortcutReference } from "./DesktopHelp";

export function PanelFacts({ fields, summary = false }: { fields: ResultField[]; summary?: boolean }) {
  return <dl className={summary ? "panel-summary" : "panel-facts"}>{fields.map((field, index) => <div key={`${field.label}-${index}`}><dt>{field.label}</dt><dd data-tone={field.tone}>{field.value}</dd></div>)}</dl>;
}

export function PanelLinks({ links }: { links: NonNullable<BackendPanel["links"]> }) {
  const [issue, setIssue] = useState("");
  return <div className="panel-links"><div className="flex flex-wrap gap-2">{links.map(link => <button className="button primary" key={link.url} onClick={() => {
    setIssue("");
    if (!window.gator) { setIssue("Open Gator on your desktop to continue sign-in."); return; }
    void window.gator.openLink(link.url).catch(() => setIssue("Could not open your browser. Try again."));
  }}>{link.label}<ExternalLink size={16} aria-hidden="true" /></button>)}</div>{issue && <p role="alert" className="panel-notice">{issue}</p>}</div>;
}

export function PanelBlocks({ panel, hasInputs, empty }: { panel: BackendPanel; hasInputs?: boolean; empty?: boolean }) {
  return <>{panel.blocks?.filter((block) => !(empty && panel.kind === "followup" && block.title === "Start with one useful loop")).map((block, index) => {
    const lines = (block.lines ?? []).filter((line) => line.trim() && !(panel.kind === "confirmation" && /^(Enter\/y|n\/esc)\s/i.test(line)));
    const content = <>{block.fields?.length ? <PanelFacts fields={block.fields} /> : null}{panel.kind === "help" || panel.title === "Model help" ? <ShortcutReference lines={lines} /> : lines.map((line, lineIndex) => <p className="literal" key={lineIndex}>{/^https?:\/\/\S+$/.test(line) ? <a href={line} onClick={(event) => { event.preventDefault(); void window.gator?.openLink(line).catch(() => {}); }}>{line}</a> : line}</p>)}</>;
    const supporting = panel.kind !== "confirmation" && (block.collapsible || !hasInputs && (
      ["Storage", "Retrieval", "Chat processing", "How to read this"].includes(block.title ?? "") ||
      panel.title === "Saved knowledge" && ["Work context", "Observed source references"].includes(block.title ?? "")
    ));
    return supporting ? <details className="panel-block panel-disclosure" key={block.title || index}><summary>{block.title || "Details"}<ChevronDown size={16} aria-hidden="true" /></summary><div className="disclosure-content">{content}</div></details>
      : <section className={`panel-block ${block.role === "content" ? "panel-content" : ""}`} key={block.title || index}>{block.title && <h2>{block.title}</h2>}{content}</section>;
  })}</>;
}
