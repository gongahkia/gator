import type { BackendBundle, BackendResult } from "./protocol.js";
export type { BackendBundle } from "./protocol.js";

export function bundleResult(bundle: BackendBundle): BackendResult {
  return {
    kind: "list", title: "Verified deliverables", tone: bundle.Status === "verified" ? "success" : "warning",
    summary: bundle.Status,
    items: [
      ...(bundle.Artifacts ?? []).map((item) => ({ label: item.Path, detail: item.MediaType, state: item.Valid ? "verified" : "needs review", tone: item.Valid ? "success" as const : "warning" as const })),
      ...(bundle.Candidates ?? []).map((item) => ({ label: `Change ${item.ID}`, detail: item.ChangedPaths.join(", "), state: item.Status })),
    ],
    footer: bundle.Path,
  };
}
