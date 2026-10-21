import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";

async function copyText(text: string) {
  try { await navigator.clipboard.writeText(text); return; } catch { /* Sandboxed hosts may deny the modern clipboard permission. */ }
  const previous = document.activeElement as HTMLElement | null;
  const selection = document.getSelection();
  const ranges = selection ? Array.from({ length: selection.rangeCount }, (_, index) => selection.getRangeAt(index).cloneRange()) : [];
  const input = document.createElement("textarea");
  input.className = "clipboard-buffer";
  input.value = text;
  input.readOnly = true;
  input.setAttribute("aria-hidden", "true");
  document.body.append(input);
  try {
    input.focus({ preventScroll: true }); input.select();
    if (!document.execCommand("copy")) throw new Error("Copy unavailable");
  } finally {
    input.remove();
    previous?.focus({ preventScroll: true });
    if (selection) { selection.removeAllRanges(); ranges.forEach((range) => selection.addRange(range)); }
  }
}

export function CopyButton({ text, label, compact = false }: { text: string; label: string; compact?: boolean }) {
  const [status, setStatus] = useState<"idle" | "copying" | "copied" | "error">("idle");
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  return <span className="copy-control"><button className={`small-button copy-button ${compact ? "compact" : ""}`} aria-label={label} title={status === "copied" ? "Copied" : label} disabled={status === "copying"} onClick={async () => {
    clearTimeout(timer.current); setStatus("copying");
    try { await copyText(text); setStatus("copied"); timer.current = setTimeout(() => setStatus("idle"), 1800); }
    catch { setStatus("error"); }
  }}>{status === "copied" ? <Check size={15} /> : <Copy size={15} />}{!compact && <span>{status === "copied" ? "Copied" : "Copy"}</span>}</button>
    <span className="visually-hidden" role="status">{status === "copied" ? `${label} complete` : ""}</span>
    {status === "error" && <span className="copy-error" role="alert">Couldn’t copy. Select the text to copy it.</span>}
  </span>;
}
