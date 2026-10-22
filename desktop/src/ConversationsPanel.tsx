import type { BackendPanel } from "./protocol";
import type { DesktopFrame } from "./bridge";
import { Panel } from "./Panel";

export function ConversationsPanel({ panel, send }: { panel: BackendPanel; send(frame: DesktopFrame): Promise<void> }) {
  const runs = panel.kind === "history";
  const detail = runs && !!panel.blocks?.length;
  return <Panel panel={panel} send={send} title={detail ? panel.subtitle || "Task run" : runs ? "Task runs" : "Conversations"}
    subtitle={detail ? "Task run details" : runs ? `Individual task runs · ${panel.subtitle || "all"}` : "Saved chats you can reopen and continue."} />;
}
