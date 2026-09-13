import { AlertTriangle, FolderSearch, Loader2, WifiOff } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Button } from "./primitives";

interface StateBlockProps {
  icon: LucideIcon;
  title: string;
  description: string;
  role?: "alert" | "status";
  actionLabel?: string;
}

function StateBlock({
  actionLabel,
  description,
  icon: Icon,
  role,
  title
}: StateBlockProps): JSX.Element {
  return (
    <div
      className="grid min-h-40 place-items-center px-5 py-6 text-center"
      role={role}
      aria-live={role === "status" ? "polite" : undefined}
    >
      <div className="max-w-sm">
        <div className="mx-auto flex size-10 items-center justify-center rounded-hcbMd border border-border bg-surface-0 text-accent">
          <Icon aria-hidden="true" className={Icon === Loader2 ? "animate-spin" : undefined} size={20} />
        </div>
        <h3 className="mt-3 text-[var(--text-md)] font-semibold text-text-primary">{title}</h3>
        <p className="mt-1 text-[var(--text-sm)] text-text-muted">{description}</p>
        {actionLabel ? (
          <Button className="mt-4" size="sm" variant="secondary">
            {actionLabel}
          </Button>
        ) : null}
      </div>
    </div>
  );
}

export function LoadingState({
  description = "Reading planner data.",
  title = "Loading"
}: {
  title?: string;
  description?: string;
} = {}): JSX.Element {
  return (
    <StateBlock
      description={description}
      icon={Loader2}
      role="status"
      title={title}
    />
  );
}

export function EmptyState({
  description,
  title
}: {
  title: string;
  description: string;
}): JSX.Element {
  return <StateBlock description={description} icon={FolderSearch} title={title} />;
}

export function OfflineState({
  description = "Google sync is disconnected.",
  title = "Offline"
}: {
  title?: string;
  description?: string;
} = {}): JSX.Element {
  return (
    <StateBlock
      actionLabel="Retry later"
      description={description}
      icon={WifiOff}
      title={title}
    />
  );
}

export function ErrorState({
  actionLabel = "Retry",
  description = "The request did not complete. The app remains usable with cached state where available.",
  title = "Something went wrong"
}: {
  title?: string;
  description?: string;
  actionLabel?: string;
} = {}): JSX.Element {
  return (
    <StateBlock
      actionLabel={actionLabel}
      description={description}
      icon={AlertTriangle}
      role="alert"
      title={title}
    />
  );
}
