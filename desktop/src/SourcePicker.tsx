import { Check, ChevronDown, LoaderCircle, Plug, RefreshCw, Settings2, X } from "lucide-react";
import { createPortal } from "react-dom";
import { useEffect, useId, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import type { BackendState } from "./protocol";
import type { DesktopFrame } from "./bridge";

type Props = { state: BackendState; disabled: boolean; send(frame: DesktopFrame): Promise<void>; manage(): void; onBusyChange(busy: boolean): void };
export function SourcePicker({ state, disabled, send, manage, onBusyChange }: Props) {
  const [open, setOpen] = useState(false), [pending, setPending] = useState<string>(), [issue, setIssue] = useState("");
  const [needsRefresh, setNeedsRefresh] = useState(false);
  const [placement, setPlacement] = useState<CSSProperties>({ visibility: "hidden" });
  const root = useRef<HTMLDivElement>(null), trigger = useRef<HTMLButtonElement>(null), popup = useRef<HTMLDivElement>(null), flight = useRef(false);
  const id = useId(), sources = state.sources, selected = sources?.selected ?? [], choices = sources?.choices ?? [];
  const locked = disabled || !!pending || !!sources?.disabled;
  const close = (restore = false) => { setOpen(false); if (restore) trigger.current?.focus({ preventScroll: true }); };
  useEffect(() => { onBusyChange(!!pending || needsRefresh); }, [pending, needsRefresh, onBusyChange]);
  useEffect(() => () => onBusyChange(false), [onBusyChange]);
  useEffect(() => { close(); setIssue(""); }, [state.conversation_id, state.source]);
  useEffect(() => { if (disabled) close(); }, [disabled]);
  useEffect(() => {
    if (!pending) return;
    if (state.sources_result?.request_id === pending) { setIssue(state.sources_result.error || ""); setNeedsRefresh(false); setPending(undefined); flight.current = false; return; }
    const timer = setTimeout(() => { setIssue("Source selection is unconfirmed. Refresh before submitting work."); setNeedsRefresh(true); setPending(undefined); flight.current = false; }, 8000);
    return () => clearTimeout(timer);
  }, [pending, state.sources_result]);
  const action = async (kind: "refresh" | "select", ids?: string[]) => {
    if (!sources || disabled || flight.current) return;
    const request = crypto.randomUUID(); flight.current = true; setPending(request); setIssue("");
    try { await send({ type: "sources", sources: { id: sources.id, request_id: request, ...(kind === "select" ? { action: "select", ids: ids ?? [] } : { action: "refresh" }) } }); }
    catch (error) { flight.current = false; setPending(undefined); setIssue(error instanceof Error ? error.message : "Sources could not be changed."); }
  };
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const rect = trigger.current?.getBoundingClientRect(); if (!rect) return;
      const width = Math.min(360, innerWidth - 24), above = rect.top - 16, below = innerHeight - rect.bottom - 16, upwards = above >= 240 || above >= below;
      setPlacement({ width, left: Math.max(12, Math.min(rect.left, innerWidth - width - 12)), maxHeight: Math.min(480, Math.max(120, upwards ? above : below)), ...(upwards ? { bottom: innerHeight - rect.top + 8 } : { top: rect.bottom + 8 }) });
    };
    place(); popup.current?.querySelector<HTMLButtonElement>("button")?.focus({ preventScroll: true });
    window.addEventListener("resize", place); window.addEventListener("scroll", place, true);
    const inside = (target: EventTarget | null) => target instanceof Node && (root.current?.contains(target) || popup.current?.contains(target));
    const outside = (event: PointerEvent | FocusEvent) => { if (!inside(event.target)) close(); };
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(true); } };
    window.addEventListener("keydown", escape, true); window.addEventListener("pointerdown", outside); window.addEventListener("focusin", outside);
    return () => { window.removeEventListener("resize", place); window.removeEventListener("scroll", place, true); window.removeEventListener("keydown", escape, true); window.removeEventListener("pointerdown", outside); window.removeEventListener("focusin", outside); };
  }, [open]);
  return <div className="source-picker" ref={root}>
    <button ref={trigger} className="small-button source-picker-trigger" disabled={disabled || !!sources?.disabled || !sources} aria-label="Add sources" aria-expanded={open} aria-controls={id} onClick={() => { if (open) close(); else { setOpen(true); void action("refresh"); } }}><Plug size={16} /><span>Sources{selected.length ? ` · ${selected.length}` : ""}</span><ChevronDown size={13} /></button>
    {needsRefresh && <span className="composer-error" role="status">Refresh sources to confirm</span>}
    {!!selected.length && <div className="selected-source-chips" aria-label="Selected sources">{selected.slice(0, 4).map(value => <span key={value} title={choices.find(choice => choice.id === value)?.status || "Source unavailable; refresh connections"}>{choices.find(choice => choice.id === value)?.label || value}</span>)}{selected.length > 4 && <span>+{selected.length - 4}</span>}</div>}
    {open && createPortal(<section id={id} ref={popup} role="dialog" aria-label="Conversation sources" className="source-picker-popover" style={placement}>
      <header><h2>Add sources</h2><button className="icon-button" aria-label="Close sources" onClick={() => close(true)}><X size={16} /></button></header>
      <p>Choose sources for new requests. Up to 16 sources. Selection does not sign in or approve changes. Queued requests keep their original sources.</p>
      <div role="group" aria-label="Available sources" onKeyDown={event => {
        if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
        event.preventDefault(); const rows = [...(popup.current?.querySelectorAll<HTMLButtonElement>('[role="checkbox"]:not(:disabled)') ?? [])];
        const index = rows.indexOf(document.activeElement as HTMLButtonElement), next = event.key === "Home" ? 0 : event.key === "End" ? rows.length - 1 : (index + (event.key === "ArrowDown" ? 1 : -1) + rows.length) % rows.length;
        rows[next]?.focus();
      }}>{choices.map(choice => <button key={choice.id} className="source-picker-row" role="checkbox" aria-checked={selected.includes(choice.id)} aria-label={`${choice.label} source`} disabled={locked || (!!choice.unavailable || selected.length >= 16) && !selected.includes(choice.id)} onClick={() => void action("select", selected.includes(choice.id) ? selected.filter(value => value !== choice.id) : [...selected, choice.id])}><Plug size={17} /><span><strong>{choice.label}</strong><small>{choice.service} · {choice.status}</small><small>{choice.unavailable || choice.access}{choice.checked_at && ` · ${new Date(choice.checked_at).toLocaleTimeString()}`}</small></span>{selected.includes(choice.id) && <Check size={16} />}</button>)}</div>
      {!choices.length && <p>No sources configured. Connect an account in Settings.</p>}
      {(issue || sources?.issue) && <p role="alert" className="composer-error">{issue || sources?.issue}</p>}
      <footer><button className="small-button" disabled={locked} onClick={() => void action("refresh")}><RefreshCw size={14} />Refresh</button><button className="small-button" disabled={locked || !selected.length} onClick={() => void action("select", [])}>Clear sources</button><button className="small-button" disabled={!!pending} onClick={() => { close(); manage(); }}><Settings2 size={14} />Manage connections</button>{pending && <span role="status"><LoaderCircle size={14} className="spin" />Updating…</span>}</footer>
    </section>, document.body)}
  </div>;
}
