import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Children, isValidElement, type ReactNode } from "react";
import { Check, ChevronDown, FileText, Info, ShieldCheck, TriangleAlert } from "lucide-react";
import type { BackendBundle, BackendResult, ResultSection } from "../../ui/src/protocol";
import { CopyButton } from "./CopyButton";

function CodeBlock({ children }: { children?: ReactNode }) {
  const child = Children.toArray(children)[0];
  const code = isValidElement<{ className?: string; children?: string }>(child) ? child.props : undefined;
  const language = code?.className?.replace(/^language-/, "") || "text";
  return <div className="code-block"><div className="code-block-heading"><span>{language}</span>{typeof code?.children === "string" && <CopyButton label="Copy code" text={code.children} />}</div><pre>{children}</pre></div>;
}

export function Markdown({ text }: { text: string }) {
  return <div className="markdown"><ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml components={{
    pre: CodeBlock,
    a: ({ href, children }) => <a href={href} onClick={(event) => { event.preventDefault(); if (href) void window.gator?.openLink(href).catch(() => {}); }}>{children}</a>,
    // Remote images would contact a service just by reading a conversation.
    img: ({ alt }) => <span className="muted">[Image{alt ? `: ${alt}` : ""}]</span>,
  }}>{text}</ReactMarkdown></div>;
}

function Content({ fields, items, lines, markdown }: Pick<ResultSection, "fields" | "items" | "lines" | "markdown">) {
  return <>
    {!!fields?.length && <dl className="result-fields">{fields.map((field, i) => <div key={i}><dt>{field.label}</dt><dd data-tone={field.tone}>{field.value}</dd></div>)}</dl>}
    {!!items?.length && <ul className="result-items">{items.map((item, i) => <li key={i}><div><strong>{item.label}</strong>{item.state && <span className="badge" data-tone={item.tone}>{item.state}</span>}</div>{item.detail && <p>{item.detail}</p>}</li>)}</ul>}
    {lines?.map((line, i) => <p className="literal" key={i}>{line || " "}</p>)}
    {markdown && <Markdown text={markdown} />}
  </>;
}

export function Result({ result, approval, onApproval }: { result: BackendResult; approval?: number; onApproval?: (approved: boolean) => void }) {
  const Icon = result.kind === "approval" ? ShieldCheck : result.tone === "error" || result.tone === "warning" ? TriangleAlert : result.tone === "success" ? Check : result.kind === "notice" ? Info : FileText;
  return <section className="result" data-kind={result.kind} data-tone={result.tone} data-pending={approval !== undefined} aria-label={result.title}>
    <div className="result-heading"><h3><Icon size={18} />{result.title}</h3>{approval !== undefined && <span className="badge" data-tone="warning">Needs review</span>}</div>
    {result.summary && <p>{result.summary}</p>}
    <Content {...result} />
    {result.sections?.map((section, i) => <section className="result-section" key={i}><h4>{section.title}</h4>{section.summary && <p>{section.summary}</p>}<Content {...section} /></section>)}
    {result.preview !== undefined && <details className="result-action-details" open><summary>{result.kind === "approval" ? "Exact action details" : "Preview details"}<ChevronDown size={16} aria-hidden="true" /></summary><pre className="preview">{JSON.stringify(result.preview, null, 2)}</pre></details>}
    {result.footer && <p className="result-footer literal">{result.footer}</p>}
    {approval !== undefined && onApproval && <div className="flex flex-wrap gap-2 mt-4"><button className="button" onClick={() => onApproval(false)}>Deny</button><button className="button primary" onClick={() => onApproval(true)}>Approve this action</button></div>}
  </section>;
}

export function bundleResult(bundle: BackendBundle): BackendResult {
  return {
    kind: "list", title: "Deliverables", tone: bundle.Status === "verified" ? "success" : "warning", summary: bundle.Status,
    items: [
      ...(bundle.Artifacts ?? []).map((item) => ({ label: item.Path, detail: item.MediaType, state: item.Valid ? "verified" : "needs review", tone: item.Valid ? "success" as const : "warning" as const })),
      ...(bundle.Candidates ?? []).map((item) => ({ label: `Change ${item.ID}`, detail: item.ChangedPaths.join(", "), state: item.Status })),
    ], footer: bundle.Path,
  };
}
