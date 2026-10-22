import { ChevronRight, FileCode2, FileText } from "lucide-react";
import { outputState, type ConversationOutput } from "./conversationOutputs";

export function OutputCards({ outputs, preview }: { outputs: ConversationOutput[]; preview(id: string): void }) {
  return <div className="output-cards">{outputs.map((output) => <button key={output.id} className="output-card" onClick={() => preview(output.id)}>
    {output.candidate ? <FileCode2 size={18} /> : <FileText size={18} />}<span><strong>{output.file?.Path || `Change ${output.candidate?.ID}`}</strong><small data-tone={output.bundle.Status === "failed" || (output.file && !output.file.Valid) ? "warning" : undefined}>{outputState(output)}{output.bundle.Status === "failed" ? " · failed run" : ""}</small></span><ChevronRight size={14} />
  </button>)}</div>;
}
