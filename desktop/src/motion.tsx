import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";

export function useReducedMotion() {
  const [reduced, setReduced] = useState(() => matchMedia("(prefers-reduced-motion: reduce)").matches);
  useEffect(() => {
    const media = matchMedia("(prefers-reduced-motion: reduce)");
    const change = () => setReduced(media.matches);
    media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);
  return reduced;
}

// Retain only the closing visual. Callers make it inert and release focus as
// soon as open becomes false, so exits never keep a dismissed control active.
export type PresenceRef = (element: HTMLElement | null) => void;

export function Presence({ open, children }: { open: boolean; children(open: boolean, ref: PresenceRef): ReactNode }) {
  const reduced = useReducedMotion();
  const [retained, setRetained] = useState(open);
  const root = useRef<HTMLElement | null>(null);
  const ref = useCallback<PresenceRef>((element) => { root.current = element; }, []);
  useLayoutEffect(() => {
    if (open) { setRetained(true); return; }
    if (!retained) return;
    const exits = reduced ? [] : root.current?.getAnimations() ?? [];
    if (!exits.length) { setRetained(false); return; }
    let live = true;
    // Follow the browser's actual transitions, including reversals and slowed
    // playback in the animation inspector, rather than guessing with a timer.
    void Promise.allSettled(exits.map((animation) => animation.finished)).then(() => { if (live) setRetained(false); });
    return () => { live = false; };
  }, [open, retained, reduced]);
  return open || (retained && !reduced) ? children(open, ref) : null;
}

// Animate navigation, never changing panel identities, form fields, scroll
// containers, or React keys. Provider deltas and snapshot refreshes do not replay it.
export function useViewMotion(key: string | undefined, ref: RefObject<HTMLElement | null>) {
  const reduced = useReducedMotion();
  const previous = useRef<string | undefined>(undefined);
  useLayoutEffect(() => {
    const old = previous.current;
    previous.current = key;
    if (reduced || old === undefined || old === key || !ref.current) return;
    const animation = ref.current.animate([
      { opacity: 0, transform: "translateY(6px)" },
      { opacity: 1, transform: "translateY(0)" },
    ], { duration: 180, easing: "cubic-bezier(0.2, 0, 0, 1)" });
    return () => animation.cancel();
  }, [key, ref, reduced]);
}

export function IconSwap({ active, on, off, size = 18 }: { active: boolean; on: ReactNode; off: ReactNode; size?: number }) {
  return <span className="icon-swap" data-active={active} style={{ width: size, height: size }} aria-hidden="true"><span className="icon-swap-on">{on}</span><span className="icon-swap-off">{off}</span></span>;
}
