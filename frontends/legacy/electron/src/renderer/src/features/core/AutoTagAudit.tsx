import { previewAutoTagRules, type AutoTagInput } from "@shared/ipc/autoTags";
import type { AutoTagRule } from "@shared/ipc/contracts";
import { Badge } from "../../components/primitives";

interface AutoTagAuditProps {
  input: AutoTagInput;
  rules: readonly AutoTagRule[];
}

export function AutoTagAudit({ input, rules }: AutoTagAuditProps): JSX.Element | null {
  if (rules.length === 0 || input.hcbKind === "birthday") {
    return null;
  }

  const preview = previewAutoTagRules(rules, input);
  const visibleTraces = preview.traces.filter(
    (trace) =>
      trace.status === "matched" ||
      trace.status === "invalid" ||
      trace.status === "disabled" ||
      trace.status === "no-output" ||
      trace.issues.length > 0
  );

  if (visibleTraces.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3">
      <div className="text-[var(--text-xs)] font-semibold uppercase text-text-muted">Auto-tag audit</div>
      <div className="grid gap-2">
        {visibleTraces.map((trace) => (
          <div className="grid gap-1 text-[var(--text-sm)] text-text-secondary" key={trace.ruleId}>
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={trace.status === "matched" ? "success" : "warning"}>{trace.status}</Badge>
              <span className="font-medium text-text-primary">{trace.ruleName}</span>
              <span className="text-text-muted">#{trace.order}</span>
            </div>
            {trace.status === "matched" ? (
              <div className="flex flex-wrap gap-1.5">
                {trace.matchedField ? <Badge tone="neutral">{trace.matchedField}</Badge> : null}
                {trace.tagsAdded.map((tag) => <Badge key={tag} tone="accent">{tag}</Badge>)}
                {trace.strippedField ? <Badge tone="warning">stripped {trace.strippedField}</Badge> : null}
                {trace.eventColorStatus !== "not-configured" ? (
                  <Badge tone={trace.eventColorStatus === "applied" ? "success" : "neutral"}>
                    color {trace.eventColorStatus}
                  </Badge>
                ) : null}
              </div>
            ) : null}
            {trace.issues.map((issue) => (
              <div className="text-[var(--text-xs)] text-warning" key={`${trace.ruleId}-${issue.field}-${issue.message}`}>
                {issue.field}: {issue.message}
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}
