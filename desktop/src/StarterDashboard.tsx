import { useCallback, useEffect, useId, useRef, useState } from "react";
import { IconSwap, useReducedMotion } from "./motion";
import { BookOpen, ChevronLeft, ChevronRight, Clock3, Inbox, ListChecks, Pause, Play, Target } from "lucide-react";

const cards = [
  { icon: Target, title: "Continue an ongoing goal", text: "Your context, decisions, and next steps.", command: "/goals" },
  { icon: ListChecks, title: "Set up a follow-up", text: "Let Gator watch for a meaningful change.", command: "/followup" },
  { icon: BookOpen, title: "Find what you’ve saved", text: "Notes, preferences, and useful context.", command: "/knowledge" },
  { icon: Inbox, title: "Review your inbox", text: "Updates and actions waiting for you.", command: "/inbox" },
  { icon: Clock3, title: "Revisit recent work", text: "Saved chats and the tasks within them.", command: "/conversations" },
];

export function StarterDashboard({ navigate }: { navigate(command: string): void }) {
  const track = useRef<HTMLDivElement>(null);
  const id = useId();
  const [paused, setPaused] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const reducedMotion = useReducedMotion();
  const move = useCallback((direction: number) => {
    const element = track.current;
    if (!element) return;
    const first = element.children[0] as HTMLElement;
    const second = element.children[1] as HTMLElement;
    const step = second.offsetLeft - first.offsetLeft;
    const end = element.scrollWidth - element.clientWidth;
    const next = direction > 0 && element.scrollLeft >= end - 2 ? 0 : direction < 0 && element.scrollLeft <= 2 ? end : Math.max(0, Math.min(end, element.scrollLeft + direction * step));
    element.scrollTo({ left: next, behavior: reducedMotion ? "instant" : "smooth" });
  }, [reducedMotion]);
  useEffect(() => {
    if (paused || hovered || focused || reducedMotion) return;
    const timer = window.setInterval(() => { if (!document.hidden) move(1); }, 6000);
    return () => clearInterval(timer);
  }, [paused, hovered, focused, reducedMotion, move]);

  return <section className="starter-dashboard" aria-label="Workspace dashboard" aria-roledescription="carousel"
    onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)}
    onFocusCapture={() => setFocused(true)} onBlurCapture={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) setFocused(false); }}>
    <div ref={track} id={id} className="starter-track" onPointerDown={() => setPaused(true)} onWheel={() => setPaused(true)}>
      {cards.map(({ icon: Icon, title, text, command }, index) => <div className="starter-slide" key={command} role="group" aria-roledescription="slide" aria-label={`${index + 1} of ${cards.length}`}><button className="starter" onClick={() => navigate(command)}><Icon size={20} /><strong>{title}</strong><p>{text}</p><ChevronRight size={16} /></button></div>)}
    </div>
    <div className="dashboard-controls">
      <button className="icon-button" aria-label="Previous dashboard cards" aria-controls={id} onClick={() => { setPaused(true); move(-1); }}><ChevronLeft size={16} /></button>
      <button className="icon-button" aria-label={paused || reducedMotion ? "Resume dashboard scrolling" : "Pause dashboard scrolling"} aria-controls={id} disabled={reducedMotion} title={reducedMotion ? "Automatic scrolling is off for reduced motion" : undefined} onClick={() => setPaused(!paused)}><IconSwap active={paused || reducedMotion} size={14} on={<Play size={14} />} off={<Pause size={14} />} /></button>
      <button className="icon-button" aria-label="Next dashboard cards" aria-controls={id} onClick={() => { setPaused(true); move(1); }}><ChevronRight size={16} /></button>
    </div>
  </section>;
}
