import { Paperclip, X } from "lucide-react";
import { useContext, useEffect, useRef, useState, type ClipboardEvent, type DragEvent } from "react";
import type { UploadedFile } from "./bridge";
import { FileCard, FileChatContext } from "./FileSurface";

export function useAttachmentPicker(context: string | undefined, files: UploadedFile[], change: (files: UploadedFile[]) => void, disabled: boolean) {
  const [busy, setBusy] = useState(false);
  const [issue, setIssue] = useState("");
  const [dragging, setDragging] = useState(false);
  const pending = useRef(false);
  const owner = useRef({ context, files, change, disabled });
  owner.current = { context, files, change, disabled };
  useEffect(() => { setIssue(""); setDragging(false); }, [context]);
  const add = async (dropped?: File[], pasted = false) => {
    const original = owner.current;
    if (!original.context || original.disabled || pending.current) return;
    pending.current = true; setBusy(true); setIssue("");
    try {
      if (dropped && (original.files.length + dropped.length > 4 || dropped.some(file => file.size > 4 * 1024 * 1024) || [...original.files,...dropped].reduce((sum,file)=>sum+file.size,0) > 8 * 1024 * 1024)) throw new Error("Attach up to four files, no larger than 4 MiB each and 8 MiB total.");
      const added = await (pasted && dropped ? window.gator!.pasteImages(await Promise.all(dropped.map(async file => ({ type: file.type, data: new Uint8Array(await file.arrayBuffer()) }))), original.context) : dropped ? window.gator!.dropFiles(dropped, original.context) : window.gator!.chooseFiles(original.context));
      if (!added.length) return;
      if (owner.current.context !== original.context) throw new Error("The conversation changed. Attach these files again in the intended conversation.");
      owner.current.change([...owner.current.files, ...added]);
    } catch (error) { setIssue(error instanceof Error ? error.message : "Couldn’t attach these files. Try again."); }
    finally { pending.current = false; setBusy(false); }
  };
  const fileDrag = (event: DragEvent) => event.dataTransfer.types.includes("Files");
  const paste = (event: ClipboardEvent<HTMLTextAreaElement>, text: (value: string) => void) => {
    const images = [...event.clipboardData.items].filter(item => item.kind === "file" && item.type.startsWith("image/")).map(item => item.getAsFile()).filter((file): file is File => !!file);
    if (!images.length) return;
    event.preventDefault();
    if (owner.current.disabled || pending.current) return;
    const plain = event.clipboardData.getData("text/plain");
    if (plain) { event.currentTarget.setRangeText(plain, event.currentTarget.selectionStart, event.currentTarget.selectionEnd, "end"); text(event.currentTarget.value); }
    void add(images, true);
  };
  return { busy, issue, choose: () => add(), paste, dragging, drop: {
    onDragOver: (event: DragEvent) => { if (fileDrag(event)) { event.preventDefault(); event.dataTransfer.dropEffect = disabled || busy ? "none" : "copy"; setDragging(!disabled && !busy); } },
    onDragLeave: (event: DragEvent<HTMLElement>) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); },
    onDrop: (event: DragEvent) => { if (!fileDrag(event)) return; event.preventDefault(); setDragging(false); void add([...event.dataTransfer.files]); },
  } };
}

export function AttachButton({ busy, disabled, choose }: { busy: boolean; disabled: boolean; choose(): void }) {
  return <button className="icon-button attach-button" aria-label="Attach files" title="Attach documents, text or images · up to 4 files, 4 MiB each, 8 MiB total" disabled={disabled || busy} aria-busy={busy} onClick={choose}><Paperclip size={17} /></button>;
}

export function PendingAttachments({ files, disabled, remove }: { files: UploadedFile[]; disabled: boolean; remove(id: string): void }) {
  const chat = useContext(FileChatContext);
  if (!files.length) return null;
  return <section className="pending-attachments" aria-label="Attachments for your next message"><ul>{files.map(file => <li key={chat.attachmentKey?.(file.id) ?? file.id}><FileCard reference={{ kind: "upload", id: file.id }} name={file.name} caption={file.size < 1024 ? `${file.size} B` : `${Math.ceil(file.size / 1024)} KB`} /><button className="icon-button attachment-remove" aria-label={`Remove attachment ${file.name}`} disabled={disabled || chat.dirtyKeys?.has(JSON.stringify({ kind: "upload", id: file.id }))} onClick={() => remove(file.id)}><X size={14} /></button></li>)}</ul></section>;
}

export function InputFiles({ names, ids }: { names?: string[]; ids?: string[] }) {
  if (!names?.length) return null;
  return <ul className="message-inputs" aria-label="Attached files">{names.map((name, index) => { const sameNameIndex = names.slice(0,index).filter(value => value === name).length; const id = ids?.filter(id => id.split("/").at(-1) === name)[sameNameIndex] ?? ids?.[index]; return <li key={index}>{id ? <FileCard reference={{ kind: "upload", id }} name={name} /> : <><Paperclip size={14} /><span title={name}>{name}</span></>}</li>; })}</ul>;
}
