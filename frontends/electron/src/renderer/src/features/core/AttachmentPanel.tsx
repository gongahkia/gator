import { useEffect, useState } from "react";
import type { AttachmentEntityKind, AttachmentSummary } from "@shared/ipc/contracts";
import { Download, ExternalLink, Paperclip, Trash2, Upload } from "lucide-react";
import { Badge, Button } from "../../components/primitives";

export function AttachmentPanel({
  editable = false,
  entityId,
  entityKind
}: {
  editable?: boolean;
  entityId: string;
  entityKind: AttachmentEntityKind;
}): JSX.Element {
  const [items, setItems] = useState<AttachmentSummary[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    void refresh();
  }, [entityId, entityKind]);

  async function refresh(): Promise<void> {
    setLoaded(false);
    const result = await window.hcb?.settings.listAttachments({ entityKind, entityId });
    if (result?.ok) {
      setItems(result.data.items);
    }
    setLoaded(true);
  }

  async function add(file: File): Promise<void> {
    setMessage("Adding attachment.");
    const result = await window.hcb?.settings.addAttachment({
      entityKind,
      entityId,
      fileName: file.name,
      mimeType: file.type,
      dataBase64: await fileToBase64(file)
    });

    if (!result?.ok) {
      setMessage(result?.error.message ?? "Attachment add failed.");
      return;
    }

    setItems(result.data.items);
    setMessage(result.data.queued ? "Attachment added and queued for sync." : "Attachment added locally.");
  }

  async function open(item: AttachmentSummary): Promise<void> {
    const result = await window.hcb?.settings.openAttachment({
      pointer: item.pointer,
      displayName: item.displayName
    });
    setMessage(result?.ok ? result.data.message : result?.error.message ?? "Attachment open failed.");
  }

  async function download(item: AttachmentSummary): Promise<void> {
    const result = await window.hcb?.settings.downloadAttachment({
      pointer: item.pointer,
      displayName: item.displayName
    });
    setMessage(result?.ok ? result.data.message : result?.error.message ?? "Attachment download failed.");
  }

  async function remove(item: AttachmentSummary): Promise<void> {
    const result = await window.hcb?.settings.removeAttachment({
      pointer: item.pointer,
      displayName: item.displayName
    });
    if (!result?.ok) {
      setMessage(result?.error.message ?? "Attachment remove failed.");
      return;
    }
    setItems(result.data.items);
    setMessage(result.data.queued ? "Attachment removed and queued for sync." : "Attachment removed locally.");
  }

  if (!editable && (!loaded || items.length === 0)) {
    return <></>;
  }

  return (
    <section className="grid gap-2">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2 text-[var(--text-xs)] font-semibold uppercase text-text-muted">
          <Paperclip aria-hidden="true" size={14} />
          Attachments
          <Badge>{items.length}</Badge>
        </div>
        {editable ? (
          <label className="inline-flex h-8 cursor-pointer items-center gap-2 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-sm)] font-semibold text-text-primary">
            <Upload aria-hidden="true" size={14} />
            Add
            <input
              className="sr-only"
              onChange={(event) => {
                const file = event.currentTarget.files?.[0];
                event.currentTarget.value = "";
                if (file) {
                  void add(file);
                }
              }}
              type="file"
            />
          </label>
        ) : null}
      </div>
      {items.length > 0 ? (
        <div className="grid gap-2">
          {items.map((item) => (
            <div
              className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-2 sm:grid-cols-[minmax(0,1fr)_auto]"
              key={item.id}
            >
              <div className="min-w-0">
                <div className="truncate text-[var(--text-sm)] font-medium text-text-primary">{item.displayName}</div>
                <div className="text-[var(--text-xs)] text-text-muted">
                  {item.exists ? `${item.kind}${item.sizeBytes === null ? "" : `, ${item.sizeBytes} bytes`}` : "missing"}
                </div>
              </div>
              <div className="flex flex-wrap gap-1">
                <Button disabled={!item.exists} onClick={() => void open(item)} size="sm" variant="secondary">
                  <ExternalLink aria-hidden="true" size={14} />
                  Open
                </Button>
                <Button disabled={!item.exists} onClick={() => void download(item)} size="sm" variant="secondary">
                  <Download aria-hidden="true" size={14} />
                  Download
                </Button>
                <Button onClick={() => void remove(item)} size="sm" variant="secondary">
                  <Trash2 aria-hidden="true" size={14} />
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : null}
      {message ? <div className="text-[var(--text-sm)] text-text-muted">{message}</div> : null}
    </section>
  );
}

async function fileToBase64(file: File): Promise<string> {
  const buffer = await file.arrayBuffer();
  let binary = "";
  const bytes = new Uint8Array(buffer);
  for (let index = 0; index < bytes.length; index += 1) {
    binary += String.fromCharCode(bytes[index] ?? 0);
  }
  return btoa(binary);
}
