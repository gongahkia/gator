import type { BackendPanel } from "../../ui/src/protocol";
import type { DesktopFrame } from "./bridge";
import { Panel } from "./Panel";

export function ConversationsPanel({ panel, send, navigate }: { panel: BackendPanel; send(frame: DesktopFrame): Promise<void>; navigate(command: string): void }) {
  const runs = panel.kind === "history";
  const detail = runs && !!panel.blocks?.length;
  return <Panel panel={panel} send={send} title="Conversations"
    subtitle={detail ? panel.subtitle : runs ? `Individual task runs · ${panel.subtitle || "all"}` : "Saved chats you can reopen and continue."}
    navigation={<nav className="conversation-views" aria-label="Conversation views">
      <button aria-current={!runs ? "page" : undefined} onClick={() => navigate("/conversations")}>Chats</button>
      <button aria-current={runs ? "page" : undefined} onClick={() => navigate("/history")}>Task runs</button>
    </nav>} />;
}
