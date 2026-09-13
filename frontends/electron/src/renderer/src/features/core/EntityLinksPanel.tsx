import { useEffect, useState } from "react";
import { Link2, RotateCcw, Search } from "lucide-react";
import type { NoteEntityKind, NoteEntityLink, NoteEntityLinksResponse } from "@shared/ipc/contracts";
import { Badge, Button, cx } from "../../components/primitives";

export function EntityLinksPanel({
  entityId,
  entityKind
}: {
  entityId: string;
  entityKind: NoteEntityKind;
}): JSX.Element | null {
  const [links, setLinks] = useState<NoteEntityLinksResponse | null>(null);

  useEffect(() => {
    let cancelled = false;

    void window.hcb?.notes.entityLinks({ entityKind, entityId }).then((result) => {
      if (!cancelled && result?.ok) {
        setLinks(result.data);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [entityId, entityKind]);

  if (!links || (links.outgoing.length === 0 && links.backlinks.length === 0 && links.broken.length === 0)) {
    return null;
  }

  return (
    <div className="grid gap-3 rounded-hcbMd border border-border bg-bg-tertiary p-3">
      <LinkGroup icon={Link2} links={links.outgoing} title="Outgoing links" />
      <LinkGroup icon={RotateCcw} links={links.backlinks} title="Backlinks" />
      <LinkGroup icon={Search} links={links.broken} title="Broken links" warning />
    </div>
  );
}

function LinkGroup({
  icon: Icon,
  links,
  title,
  warning = false
}: {
  icon: typeof Link2;
  links: NoteEntityLink[];
  title: string;
  warning?: boolean;
}): JSX.Element | null {
  if (links.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-2">
      <div className={cx(
        "flex items-center gap-1 text-[var(--text-xs)] font-semibold uppercase",
        warning ? "text-warning" : "text-text-muted"
      )}>
        <Icon aria-hidden="true" size={13} />
        {title}
      </div>
      <div className="flex flex-wrap gap-2">
        {links.map((link) => (
          <LinkChip key={`${link.sourceKind}-${link.sourceId}-${link.targetKind}-${link.raw}`} link={link} />
        ))}
      </div>
    </div>
  );
}

function LinkChip({ link }: { link: NoteEntityLink }): JSX.Element {
  const canOpen = Boolean(link.targetId) &&
    (link.targetKind === "note" || link.targetKind === "task" || link.targetKind === "event");
  const label = link.targetId && !link.broken
    ? `${link.targetKind}: ${link.targetLabel}`
    : `Broken ${link.targetKind}: ${link.targetLabel}`;

  return (
    <Button
      disabled={!canOpen}
      onClick={() => {
        if (canOpen && link.targetId) {
          window.dispatchEvent(new CustomEvent("hcb:open-entity", {
            detail: { id: link.targetId, kind: link.targetKind }
          }));
        }
      }}
      size="sm"
      variant={link.broken ? "secondary" : "ghost"}
    >
      <span className={cx("truncate", link.broken && "text-warning")}>{label}</span>
      {link.linkType === "transclusion" ? <Badge tone="info">embed</Badge> : null}
    </Button>
  );
}
