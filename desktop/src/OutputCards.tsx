import { ChevronRight, FileCode2 } from "lucide-react";
import { outputState, type ConversationOutput } from "./conversationOutputs";
import { FileCard } from "./FileSurface";

export function OutputCards({ outputs, preview }: { outputs: ConversationOutput[]; preview(id: string): void }) {
  return <div className="output-cards">{outputs.map((output) => output.file ? <div key={output.id} data-chat-output={output.id}><FileCard reference={{ kind: "output", bundle: output.bundle.Path, path: output.file.Path }} name={output.file.Path} caption={`${outputState(output)}${output.bundle.Status === "failed" ? " · failed run" : ""}`} fallback={output.file.Preview} /></div> : <button key={output.id} className="output-card" data-chat-output={output.id} onClick={() => preview(output.id)}>
    <FileCode2 size={18} /><span><strong>{`Change ${output.candidate?.ID}`}</strong><small data-tone={output.bundle.Status === "failed" ? "warning" : undefined}>{outputState(output)}{output.bundle.Status === "failed" ? " · failed run" : ""}</small></span><ChevronRight size={14} />
  </button>)}</div>;
}
