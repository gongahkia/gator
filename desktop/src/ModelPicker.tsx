import { ArrowLeft, Check, ChevronDown, Cpu, LoaderCircle, RotateCcw, Search, Settings2 } from "lucide-react";
import { createPortal } from "react-dom";
import { useEffect, useId, useLayoutEffect, useRef, useState, type CSSProperties, type KeyboardEvent } from "react";
import type { BackendState, ComposerAction } from "./protocol";
import type { DesktopFrame } from "./bridge";
import { Presence, useViewMotion, type PresenceRef } from "./motion";

const efforts = ["low", "standard", "high"] as const;
const labels = ["Low", "Standard", "High"];
const providerLabel = (id: string) => ({ openai: "OpenAI", anthropic: "Anthropic", gemini: "Google Gemini", "gator-local": "Local models", xai: "xAI", openrouter: "OpenRouter" }[id] ?? id.replaceAll("-", " "));
type Props = { state: BackendState; disabled: boolean; send(frame: DesktopFrame): Promise<void>; manage(): void; onBusyChange(busy: boolean): void };

export function ModelPicker({ state, disabled, send, manage, onBusyChange }: Props) {
  const [open, setOpen] = useState(false);
  const [view, setView] = useState("effort");
  const [query, setQuery] = useState("");
  const [activeModel, setActiveModel] = useState("");
  const [pending, setPending] = useState<{ id: string; model: boolean }>();
  const [issue, setIssue] = useState("");
  const [position, setPosition] = useState(1);
  const [placement, setPlacement] = useState<CSSProperties>({ visibility: "hidden" });
  const root = useRef<HTMLDivElement>(null), trigger = useRef<HTMLButtonElement>(null), popup = useRef<HTMLDivElement>(null), body = useRef<HTMLDivElement>(null);
  const dragging = useRef(false), inFlight = useRef(false);
  const owner = useRef({ state, disabled, send }); owner.current = { state, disabled, send };
  const id = useId();
  const settings = state.composer;
  const choices = settings?.models ?? [];
  const current = choices.find(choice => choice.provider === state.provider && choice.model === state.model);
  const modelLabel = current?.label || state.model || "Choose model";
  const effort = Math.max(0, efforts.indexOf(settings?.effort ?? "standard"));
  const locked = disabled || !!pending || !!settings?.disabled;
  const filtered = choices.filter(choice => `${choice.label} ${choice.model} ${providerLabel(choice.provider)}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
  const groups = [...new Set(filtered.map(choice => choice.provider))];
  const activeChoice = filtered.find(choice => !choice.unavailable && `${choice.provider}:${choice.model}` === activeModel) ?? filtered.find(choice => !choice.unavailable && choice.provider === state.provider && choice.model === state.model) ?? filtered.find(choice => !choice.unavailable);
  useViewMotion(view, body);
  useEffect(() => { if (!dragging.current && !pending) setPosition(effort); }, [effort, open, pending]);
  useEffect(() => { onBusyChange(!!pending); }, [pending, onBusyChange]);
  useEffect(() => () => onBusyChange(false), [onBusyChange]);
  useEffect(() => { setOpen(false); setIssue(""); }, [state.source, state.conversation_id]);
  useEffect(() => { if (disabled) setOpen(false); }, [disabled]);
  useEffect(() => {
    if (!pending) return;
    if (state.composer_result?.request_id === pending.id) {
      const error = state.composer_result.error;
      setIssue(error ?? ""); setPending(undefined); inFlight.current = false;
      if (!error && pending.model) setView("effort");
      return;
    }
    const timer = setTimeout(() => { inFlight.current = false; setPending(undefined); setIssue("Gator has not confirmed this change. Check the current selection before trying again."); }, 8000);
    return () => clearTimeout(timer);
  }, [pending, state.composer_result]);
  const close = (restore: boolean) => { setOpen(false); if (restore) trigger.current?.focus({ preventScroll: true }); };
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const rect = trigger.current?.getBoundingClientRect(); if (!rect) return;
      const width = Math.min(304, innerWidth - 24), above = rect.top - 16, below = innerHeight - rect.bottom - 16;
      const upwards = above >= 240 || above >= below;
      setPlacement({ width, left: Math.max(12, Math.min(rect.right - width, innerWidth - width - 12)), maxHeight: Math.min(420, Math.max(120, upwards ? above : below)), ...(upwards ? { bottom: innerHeight - rect.top + 8 } : { top: rect.bottom + 8 }) });
    };
    place();
    window.addEventListener("resize", place); window.addEventListener("scroll", place, true);
    const resize = new ResizeObserver(place); if (trigger.current) resize.observe(trigger.current);
    return () => { resize.disconnect(); window.removeEventListener("resize", place); window.removeEventListener("scroll", place, true); };
  }, [open]);
  useLayoutEffect(() => {
    if (!open) return;
    const focus = view === "models" ? (popup.current?.querySelector<HTMLElement>('input[type="search"], [role="option"][aria-selected="true"]:not(:disabled)') ?? popup.current?.querySelector<HTMLElement>('[role="option"]:not(:disabled)') ?? popup.current?.querySelector<HTMLElement>('.picker-heading button')) : popup.current?.querySelector<HTMLElement>(".picker-model-select");
    focus?.focus({ preventScroll: true });
  }, [open, view]);
  useEffect(() => {
    if (!open) return;
    const inside = (target: EventTarget | null) => target instanceof Node && (root.current?.contains(target) || popup.current?.contains(target));
    const outside = (event: PointerEvent | FocusEvent) => { if (!inside(event.target)) close(false); };
    const escape = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault(); event.stopPropagation();
      if (view === "models") { setView("effort"); setQuery(""); } else close(true);
    };
    window.addEventListener("keydown", escape, true); window.addEventListener("pointerdown", outside); window.addEventListener("focusin", outside);
    return () => { window.removeEventListener("keydown", escape, true); window.removeEventListener("pointerdown", outside); window.removeEventListener("focusin", outside); };
  }, [open, view]);
  const change = async (action: { action: "model"; provider: string; model: string } | { action: "effort"; effort: typeof efforts[number] }) => {
    const latest = owner.current;
    if (!latest.state.composer || latest.disabled || latest.state.composer.disabled || inFlight.current) return;
    inFlight.current = true; const request = crypto.randomUUID();
    setIssue(""); setPending({ id: request, model: action.action === "model" });
    try { await latest.send({ type: "composer", composer: { id: latest.state.composer.id, request_id: request, ...action } as ComposerAction }); }
    catch (error) { inFlight.current = false; setPending(undefined); setIssue(error instanceof Error ? error.message : "Could not save this selection."); }
  };
  const chooseEffort = (value: number) => { if (value !== effort) void change({ action: "effort", effort: efforts[value] }); };
  const listKeys = (event: KeyboardEvent) => {
    const keys = ["ArrowDown", "ArrowUp", "Home", "End"];
    if (!keys.includes(event.key)) return;
    event.preventDefault(); event.stopPropagation();
    const options = [...(popup.current?.querySelectorAll<HTMLButtonElement>('[role="option"]:not(:disabled)') ?? [])];
    const current = options.indexOf(document.activeElement as HTMLButtonElement);
    const next = event.key === "Home" ? 0 : event.key === "End" ? options.length - 1 : (current + (event.key === "ArrowDown" ? 1 : -1) + options.length) % options.length;
    options[next]?.focus();
  };
  const panel = (shown: boolean, ref: PresenceRef) => <div ref={element => { popup.current = element; ref(element); }} id={id} role="dialog" aria-label="Model and work effort" className="model-picker-popover" style={placement} data-open={shown} inert={!shown} aria-hidden={!shown}>
    <div ref={body} className="model-picker-body" aria-busy={!!pending}>
      {view === "models" ? <>
        <header className="picker-heading"><button className="icon-button" aria-label="Back to work effort" onClick={() => { setView("effort"); setQuery(""); }}><ArrowLeft size={17} /></button><h2>Choose model</h2>{pending && <LoaderCircle size={15} className="spin" />}</header>
        {choices.length > 7 && <div className="picker-search"><Search size={15} /><input type="search" aria-label="Find a model" placeholder="Find a model…" value={query} onChange={event => setQuery(event.target.value)} onKeyDown={listKeys} /></div>}
        <div className="picker-models" role="listbox" aria-label="Models" onKeyDown={listKeys}>
          {groups.map(provider => <div role="group" aria-label={providerLabel(provider)} key={provider}><h3>{providerLabel(provider)}</h3>{filtered.filter(choice => choice.provider === provider).map(choice => {
            const selected = choice.provider === state.provider && choice.model === state.model;
            return <button key={choice.model} role="option" aria-selected={selected} tabIndex={choice === activeChoice ? 0 : -1} onFocus={() => setActiveModel(`${choice.provider}:${choice.model}`)} title={choice.unavailable || choice.model} disabled={locked || !!choice.unavailable} onClick={() => { if (selected) setView("effort"); else void change({ action: "model", provider: choice.provider, model: choice.model }); }}><Cpu size={17} /><span><strong>{choice.label}</strong>{choice.unavailable && <small>{choice.unavailable}</small>}</span>{selected && <Check size={16} />}</button>;
          })}</div>)}
          {!filtered.length && <p className="picker-empty">{choices.length ? "No matching models" : "Add a cloud or local model to get started."}</p>}
        </div>
      </> : <div className="picker-effort">
        <span className="picker-eyebrow">Work effort</span>
        <strong className="picker-effort-label">{labels[position]}</strong>
        <button className="icon-button picker-reset" aria-label="Reset work effort to Standard" title="Reset work effort to Standard" disabled={locked || effort === 1 || !settings} onClick={() => { setPosition(1); void change({ action: "effort", effort: "standard" }); }}><RotateCcw size={16} /></button>
        <button className="picker-model-select" disabled={!!pending} onClick={() => setView("models")}><span>{modelLabel}</span><ChevronDown size={16} /></button>
        <div className="effort-slider" style={{ "--effort-position": position / 2 } as CSSProperties}>
          <div className="effort-track" aria-hidden="true"><div className="effort-fill" /></div>
          <div className="effort-stops" aria-hidden="true">{labels.map((label, index) => <span key={label} data-filled={index <= position} />)}</div>
          <input type="range" aria-label="Work effort" aria-valuetext={labels[position]} min={0} max={2} step={1} value={position} disabled={locked || !settings} onPointerDown={() => { dragging.current = true; }} onPointerUp={event => { dragging.current = false; chooseEffort(Number(event.currentTarget.value)); }} onPointerCancel={() => { dragging.current = false; setPosition(effort); }} onBlur={event => { if (dragging.current) { dragging.current = false; chooseEffort(Number(event.currentTarget.value)); } }} onChange={event => { const value = Number(event.target.value); setPosition(value); if (!dragging.current) chooseEffort(value); }} />
        </div>
        <p className="picker-effort-help">How much work Gator can do on a request.</p>
      </div>}
      {(issue || settings?.issue || settings?.disabled) && <p className="picker-issue" role={issue ? "alert" : "status"}>{issue || settings?.issue || settings?.disabled}</p>}
      <button className="picker-manage" disabled={!!pending || !!settings?.disabled || disabled} onClick={() => { close(false); manage(); }}><Settings2 size={15} />Manage models</button>
    </div>
  </div>;
  return <div ref={root} className="model-picker">
    <button ref={trigger} className="model-picker-trigger" aria-label={`Model and work effort: ${modelLabel}, ${labels[effort]}`} title={`${modelLabel} · ${labels[effort]} work effort`} aria-haspopup="dialog" aria-expanded={open} aria-controls={open ? id : undefined} disabled={disabled} onClick={() => { setView("effort"); setQuery(""); setOpen(!open); }}><Cpu size={16} /><span className="picker-trigger-model">{modelLabel}</span><span className="picker-trigger-effort">· {labels[effort]}</span><ChevronDown size={14} className="picker-trigger-chevron" data-open={open} /></button>
    {createPortal(<Presence open={open}>{panel}</Presence>, root.current?.closest(".app-shell") ?? document.body)}
  </div>;
}
