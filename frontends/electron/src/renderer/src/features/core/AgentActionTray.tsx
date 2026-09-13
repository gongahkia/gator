import { Check, X } from "lucide-react";
import { useEffect, useState } from "react";
import type { AgentActionSummary } from "@shared/ipc/contracts";
import { Button, IconButton } from "../../components/primitives";

export function AgentActionTray({ enabled }: { enabled: boolean }): JSX.Element | null {
  const [items, setItems] = useState<AgentActionSummary[]>([]);

  async function refresh(): Promise<void> {
    if (!enabled) {
      setItems([]);
      return;
    }
    const result = await window.hcb?.agent.listActions({ statuses: ["pending"], limit: 20 });
    if (result?.ok) {
      setItems(result.data.items);
    }
  }

  async function apply(id: string): Promise<void> {
    await window.hcb?.agent.applyAction({ id });
    await refresh();
  }

  async function reject(id: string): Promise<void> {
    await window.hcb?.agent.rejectAction({ id });
    await refresh();
  }

  useEffect(() => {
    void refresh();
    if (!enabled) {
      return undefined;
    }
    const interval = window.setInterval(() => void refresh(), 5_000);
    return () => window.clearInterval(interval);
  }, [enabled]);

  if (!enabled || items.length === 0) {
    return null;
  }

  return (
    <aside className="fixed bottom-3 right-3 z-50 grid max-h-80 w-[min(28rem,calc(100vw-1.5rem))] gap-2 overflow-auto rounded-hcbLg border border-border bg-bg-primary p-3 shadow-xl">
      <div className="flex items-center gap-2">
        <h2 className="text-[var(--text-sm)] font-semibold text-text-primary">Pending agent actions</h2>
        <Button onClick={() => void refresh()} size="sm" variant="ghost">Refresh</Button>
      </div>
      {items.map((item) => (
        <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-hcbMd border border-border bg-bg-secondary px-2 py-2" key={item.id}>
          <div className="min-w-0">
            <div className="truncate text-[var(--text-sm)] font-medium text-text-primary">{item.toolName}</div>
            <div className="truncate text-[var(--text-xs)] text-text-muted">{item.summary}</div>
          </div>
          <div className="flex items-center gap-1">
            <IconButton icon={Check} label="Approve agent action" onClick={() => void apply(item.id)} variant="ghost" />
            <IconButton icon={X} label="Reject agent action" onClick={() => void reject(item.id)} variant="ghost" />
          </div>
        </div>
      ))}
    </aside>
  );
}
