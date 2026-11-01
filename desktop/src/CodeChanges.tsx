import { useEffect, useState } from "react";
import { Check, CircleAlert, FileCode2 } from "lucide-react";
import type { CodePreview } from "@gator/application-host/contracts/codePreview";
import type { BackendBundle } from "./protocol";
import { CopyButton } from "./CopyButton";

export function CodeChanges({ context, bundle, candidate }: { context: string; bundle: BackendBundle; candidate: string }) {
  const [result, setResult] = useState<{ key: string; value?: CodePreview; issue?: string }>();
  const key = JSON.stringify([context, bundle.RunID, candidate, bundle.ManifestSHA256]);
  useEffect(() => {
    let active = true;
    if (!bundle.RunID) { setResult({ key, issue: "This older result has no exact run binding for a patch preview." }); return; }
    void window.gator!.codePreview({ context, run: bundle.RunID, candidate }).then(value => { if (active) setResult({ key, value }); }).catch(error => { if (active) setResult({ key, issue: error instanceof Error ? error.message : "The retained patch is unavailable." }); });
    return () => { active = false; };
  }, [key]);
  // Key the rendered value as well as the request: a selection change must
  // never display the preceding target's patch for even a single render.
  const current = result?.key === key ? result : undefined, view = current?.value;
 const lines = view?.patch.split("\n") ?? [];
  return <section className="code-changes" aria-label="Code changes">
    <p className="work-details-caption">Staged proposal · your source files are unchanged. Passing checks does not establish independent review or permission to apply.</p>
    {current?.issue ? <p role="alert">{current.issue}</p> : !view ? <p role="status">Opening the retained patch…</p> : <>
      <h4>{view.changedPaths.length} changed {view.changedPaths.length === 1 ? "file" : "files"}</h4>
      <ul className="code-changed-files">{view.changedPaths.map(path => <li key={path}><FileCode2 size={14} /><code>{path}</code></li>)}</ul>
      <h4>Recorded verification</h4>
      {view.verification.length ? <ul className="code-verification">{view.verification.map((check, i) => <li key={i} data-passed={check.passed}>{check.passed ? <Check size={14} /> : <CircleAlert size={14} />}<span>{check.passed ? "Passed" : "Failed"}</span><code>{check.argv.map(value => JSON.stringify(value)).join(" ")}</code></li>)}</ul> : <p className="work-details-caption">No functional verification commands were recorded.</p>}
      <div className="output-preview-toolbar"><span>Unified Git diff · retained bytes</span><CopyButton label="Copy displayed diff" text={view.patch} /></div>
      {view.truncated && <p role="status" className="work-details-notice">Partial diff · this preview is limited to 128 KiB / 2,000 lines. The retained patch remains unchanged.</p>}
      <pre className="code-diff" aria-label="Retained Git diff"><code>{lines.map((line, i) => <span key={i} className={line.startsWith("+++") || line.startsWith("---") || line.startsWith("diff ") ? "diff-header" : line.startsWith("+") ? "diff-added" : line.startsWith("-") ? "diff-removed" : line.startsWith("@@") ? "diff-hunk" : undefined}><span className="diff-line-number" aria-hidden="true">{i + 1}</span>{line}{i < lines.length - 1 ? "\n" : ""}</span>)}</code></pre>
    </>}
  </section>;
}
