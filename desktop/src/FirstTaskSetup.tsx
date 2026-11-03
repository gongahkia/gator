import { ChevronRight, FileText, MessageSquare, Plug, Settings2, Target } from "lucide-react";
import { connections_title as connectionsTitle, connections_setup as connectionsSetup } from "../../internal/productguide/capabilities.json";

// Suggestions only fill the existing draft. Sending, account access and ongoing
// responsibilities keep their normal explicit setup/review paths.
export function FirstTaskSetup({ hasDraft, hasFiles, disabled, suggest, attach, models, connections }: {
  hasDraft: boolean; hasFiles: boolean; disabled: boolean;
  suggest(text: string): void; attach(): void; models(): void; connections(): void;
}) {
  return <section className="first-task-setup" aria-label="Get started with Gator">
    {!hasDraft && !hasFiles && <div className="first-task-ideas" role="group" aria-label="Choose a starting point">
      <button className="small-button" disabled={disabled} onClick={() => suggest("Help me think through a decision. Ask me what I’m deciding and what matters to me.")}><MessageSquare size={15} />Think through a decision</button>
      <button className="small-button" disabled={disabled} onClick={() => { suggest("Help me understand this file. Summarize the main points and anything I should pay attention to."); attach(); }}><FileText size={15} />Understand a file</button>
      <button className="small-button" disabled={disabled} onClick={() => suggest("I’d like you to keep an eye on something for me. Ask what to watch, when to check, and which changes are worth telling me about before we set it up.")}><Target size={15} />Follow something over time</button>
    </div>}
    <div className="first-task-model"><Settings2 size={18} aria-hidden="true" /><div><h2>Choose a model to get started</h2><p>A cloud model uses your provider’s API key and billing. A local model runs on this computer and needs a download.</p><button className="button" disabled={disabled} onClick={models}>Choose a model<ChevronRight size={15} /></button></div></div>
    <p className="first-task-privacy">Cloud models receive the message, files and saved context supplied to a request. Your draft and attached files stay here during setup. Send when you’re ready.</p>
    <details className="first-task-connections"><summary><Plug size={15} aria-hidden="true" />{connectionsTitle}</summary><p>{connectionsSetup}</p><button className="small-button" disabled={disabled} onClick={connections}>Connect an account<ChevronRight size={14} /></button></details>
  </section>;
}
