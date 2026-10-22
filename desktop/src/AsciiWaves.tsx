import { memo, useEffect, useRef } from "react";
import { useReducedMotion } from "./motion";

const glyphs = " ·:;+*";
const rowHeight = 16;
const font = "12px ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";
const fract = (n: number) => n - Math.floor(n);
const smooth = (n: number) => n * n * (3 - 2 * n);
const mix = (a: number, b: number, n: number) => a + (b - a) * n;
const hash = (x: number, y: number, z: number) => fract(Math.sin(x * 127.1 + y * 311.7 + z * 74.7) * 43758.5453);

// The Tide field explored in InvestmentGini: crossing swells gently warped by
// deterministic noise. Keep only the ambient field, with Gator's own palette.
function noise(x: number, y: number, z: number) {
  const ix = Math.floor(x), iy = Math.floor(y), iz = Math.floor(z);
  const fx = smooth(fract(x)), fy = smooth(fract(y)), fz = smooth(fract(z));
  const plane = (dz: number) => mix(mix(hash(ix, iy, iz + dz), hash(ix + 1, iy, iz + dz), fx), mix(hash(ix, iy + 1, iz + dz), hash(ix + 1, iy + 1, iz + dz), fx), fy);
  return mix(plane(0), plane(1), fz);
}

export const AsciiWaves = memo(function AsciiWaves() {
  const surface = useRef<HTMLCanvasElement>(null);
  const reduced = useReducedMotion();

  useEffect(() => {
    const canvas = surface.current;
    const context = canvas?.getContext("2d");
    if (!canvas || !context) return;
    const atlas = document.createElement("canvas");
    const ink = atlas.getContext("2d");
    if (!ink) return;
    let width = 0, height = 0, cellWidth = 0, cellPixels = 0, rowPixels = 0;
    let frame = 0, previous = -Infinity;
    // A native animation provides a pauseable clock and supports inspection at
    // reduced playback speed. Glyph updates stay outside React's render cycle.
    const clock = reduced ? undefined : canvas.animate([{ opacity: .8 }, { opacity: 1 }, { opacity: .8 }], { duration: 9000, iterations: Infinity, easing: "ease-in-out" });
    const draw = () => {
      const seconds = 18 + (typeof clock?.currentTime === "number" ? clock.currentTime / 1000 : 0);
      context.clearRect(0, 0, canvas.width, canvas.height);
      context.globalCompositeOperation = "source-over";
      for (let row = 0; row < Math.ceil(height / rowHeight); row++) {
        for (let col = 0; col < Math.ceil(width / cellWidth); col++) {
          const u = col * cellWidth / rowHeight;
          const warp = noise(u * .025, row * .05, seconds * .04) * 2.4;
          const swell = (Math.sin(u * .08 + row * .1 + seconds * .35 + warp) + Math.sin(u * .05 - row * .14 + seconds * .25 - warp * .6)) / 2;
          const index = Math.round(Math.max(0, Math.min(1, (swell - .05) / .95)) * (glyphs.length - 1));
          if (!index) continue;
          context.globalAlpha = .18 + .3 * index / (glyphs.length - 1);
          context.drawImage(atlas, index * cellPixels, 0, cellPixels, rowPixels, col * cellPixels, row * rowPixels, cellPixels, rowPixels);
        }
      }
      context.globalAlpha = 1;
      context.globalCompositeOperation = "source-in";
      context.fillStyle = getComputedStyle(canvas).color;
      context.fillRect(0, 0, canvas.width, canvas.height);
      context.globalCompositeOperation = "source-over";
    };
    const layout = () => {
      ({ width, height } = canvas.getBoundingClientRect());
      const ratio = Math.min(devicePixelRatio || 1, 2);
      context.font = font;
      cellPixels = Math.ceil(context.measureText("M").width * ratio);
      cellWidth = cellPixels / ratio;
      rowPixels = Math.round(rowHeight * ratio);
      canvas.width = Math.ceil(width / cellWidth) * cellPixels;
      canvas.height = Math.ceil(height / rowHeight) * rowPixels;
      atlas.width = glyphs.length * cellPixels;
      atlas.height = rowPixels;
      ink.scale(ratio, ratio);
      ink.font = font; ink.fillStyle = "white"; ink.textBaseline = "middle";
      [...glyphs].forEach((glyph, index) => ink.fillText(glyph, index * cellWidth, rowHeight / 2));
      if (width && height) draw();
    };
    const tick = (now: number) => {
      if (now - previous >= 1000 / 20) { draw(); previous = now; }
      frame = requestAnimationFrame(tick);
    };
    const visibility = () => {
      cancelAnimationFrame(frame);
      if (document.hidden) clock?.pause();
      else if (clock) { clock.play(); frame = requestAnimationFrame(tick); }
    };
    const resize = new ResizeObserver(layout);
    resize.observe(canvas);
    layout(); visibility();
    document.addEventListener("visibilitychange", visibility);
    return () => { cancelAnimationFrame(frame); clock?.cancel(); resize.disconnect(); document.removeEventListener("visibilitychange", visibility); };
  }, [reduced]);

  return <canvas ref={surface} className="ascii-waves" aria-hidden="true" />;
});
