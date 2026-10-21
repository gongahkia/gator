import { ChevronDown } from "lucide-react";
import type { BackendPanel, ResultField } from "../../ui/src/protocol";

export function PanelFacts({ fields, summary = false }: { fields: ResultField[]; summary?: boolean }) {
  return <dl className={summary ? "panel-summary" : "panel-facts"}>{fields.map((field, index) => <div key={`${field.label}-${index}`}><dt>{field.label}</dt><dd data-tone={field.tone}>{field.value}</dd></div>)}</dl>;
}

export function PanelBlocks({ panel, hasInputs, empty }: { panel: BackendPanel; hasInputs?: boolean; empty?: boolean }) {
  return <>{panel.blocks?.filter((block) => !(empty && panel.kind === "followup" && block.title === "Start with one useful loop")).map((block, index) => {
    const content = <>{block.fields?.length ? <PanelFacts fields={block.fields} /> : null}{(block.lines ?? []).filter((line) => line.trim()).map((line, lineIndex) => <p className="literal" key={lineIndex}>{line}</p>)}</>;
    const supporting = panel.kind !== "confirmation" && (block.collapsible || !hasInputs && (
      ["Storage", "Retrieval", "Chat processing", "How to read this"].includes(block.title ?? "") ||
      panel.title === "Saved knowledge" && ["Work context", "Observed source references"].includes(block.title ?? "")
    ));
    return supporting ? <details className="panel-block panel-disclosure" key={block.title || index}><summary>{block.title || "Details"}<ChevronDown size={16} aria-hidden="true" /></summary><div className="disclosure-content">{content}</div></details>
      : <section className={`panel-block ${block.role === "content" ? "panel-content" : ""}`} key={block.title || index}>{block.title && <h2>{block.title}</h2>}{content}</section>;
  })}</>;
}
