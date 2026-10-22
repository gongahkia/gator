import { memo, useEffect, useRef } from "react";
import { useReducedMotion } from "./motion";

const columns = 93;
const rows = 23;
const glyphs = " .:~";
const distances = Array.from({ length: rows }, (_, row) => Array.from({ length: columns }, (_, column) => {
  const x = (column - (columns - 1) / 2) / 3.5;
  const y = row - (rows - 1) / 2;
  return Math.hypot(x, y) + Math.sin(x * .55 + y * .25) * .4;
}));

function waveFrame(seconds: number) {
  const swell = 1 + Math.sin(seconds * .55) * .06;
  return distances.map((line) => line.map((distance) => {
    const crest = (Math.cos(distance / swell * 1.12 - seconds * .75) + 1) / 2;
    return glyphs[Math.min(glyphs.length - 1, Math.floor(crest ** 6 * glyphs.length))];
  }).join("")).join("\n");
}

const stillWave = waveFrame(0);

// Keep the decorative frame loop outside React's render cycle and the chat state.
export const AsciiWaves = memo(function AsciiWaves() {
  const surface = useRef<HTMLPreElement>(null);
  const reduced = useReducedMotion();

  useEffect(() => {
    const element = surface.current;
    if (!element) return;
    element.textContent = stillWave;
    if (reduced) return;

    // The browser animation is also our clock: pausing or slowing it in the
    // animation inspector keeps the glyphs and the soft pulse together.
    const pulse = element.animate([{ opacity: .32 }, { opacity: .52 }, { opacity: .32 }], {
      duration: 6800, iterations: Infinity, easing: "ease-in-out",
    });
    let frame = 0;
    let previous = -Infinity;
    const draw = (now: number) => {
      if (now - previous >= 1000 / 12) {
        element.textContent = waveFrame(typeof pulse.currentTime === "number" ? pulse.currentTime / 1000 : 0);
        previous = now;
      }
      frame = requestAnimationFrame(draw);
    };
    const visibility = () => {
      cancelAnimationFrame(frame);
      if (document.hidden) pulse.pause();
      else { pulse.play(); frame = requestAnimationFrame(draw); }
    };
    visibility();
    document.addEventListener("visibilitychange", visibility);
    return () => {
      cancelAnimationFrame(frame);
      pulse.cancel();
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [reduced]);

  return <div className="ascii-waves" aria-hidden="true"><pre ref={surface}>{stillWave}</pre></div>;
});
