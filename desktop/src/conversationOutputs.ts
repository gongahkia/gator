import type { BackendBundle, BackendState } from "./protocol";

export type ConversationOutput = { id: string; bundle: BackendBundle; group: number; file?: NonNullable<BackendBundle["Artifacts"]>[number]; candidate?: NonNullable<BackendBundle["Candidates"]>[number] };

export function bundleOutputs(bundle: BackendBundle, group = 1): ConversationOutput[] {
  return [
    ...(bundle.Artifacts ?? []).map((file) => ({ id: JSON.stringify([bundle.Path, "file", file.Path]), bundle, group, file })),
    ...(bundle.Candidates ?? []).map((candidate) => ({ id: JSON.stringify([bundle.Path, "change", candidate.ID]), bundle, group, candidate })),
  ];
}

export function conversationBundles(state: BackendState) {
  const values = new Map<string, BackendBundle>();
  for (const message of state.messages) if (message.bundle?.Path) values.set(message.bundle.Path, message.bundle);
  if (state.work_details?.latest_bundle?.Path) values.set(state.work_details.latest_bundle.Path, state.work_details.latest_bundle);
  return [...values.values()];
}

export function outputState(output: ConversationOutput) {
  if (output.candidate) return `Proposal · ${output.candidate.Status || "unreported checks"}`;
  return output.file?.Valid ? "Checks passed" : "Needs review";
}
