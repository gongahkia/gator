import { useEffect, useRef } from "react";

// Read the existing microphone stream locally; no second capture or audio output.
export function DictationWaveform({ stream }: { stream?: MediaStream }) {
  const root = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (!stream || !root.current || !window.AudioContext) return;
    const bars = [...root.current.children] as HTMLElement[];
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
    let audio: AudioContext | undefined, frame = 0, last = 0;
    const levels = Array<number>(bars.length).fill(0);
    try {
      audio = new AudioContext();
      const source = audio.createMediaStreamSource(stream), analyser = audio.createAnalyser();
      analyser.fftSize = 512; source.connect(analyser);
      const samples = new Uint8Array(analyser.fftSize);
      const draw = (now: number) => {
        if (!document.hidden && now - last >= 60) {
          last = now; analyser.getByteTimeDomainData(samples);
          const energy = Math.sqrt(samples.reduce((sum, value) => sum + ((value - 128) / 128) ** 2, 0) / samples.length);
          levels.shift(); levels.push(Math.min(1, energy * 6));
          bars.forEach((bar, index) => { bar.style.transform = `scaleY(${0.15 + levels[index] * 0.85})`; });
        }
        frame = requestAnimationFrame(draw);
      };
      const motion = () => {
        cancelAnimationFrame(frame);
        if (reduced.matches) bars.forEach(bar => { bar.style.transform = "scaleY(.15)"; });
        else frame = requestAnimationFrame(draw);
      };
      reduced.addEventListener("change", motion); motion();
      void audio.resume().catch(() => {});
      return () => { cancelAnimationFrame(frame); reduced.removeEventListener("change", motion); source.disconnect(); analyser.disconnect(); void audio?.close().catch(() => {}); };
    } catch { void audio?.close().catch(() => {}); }
  }, [stream]);
  return <span ref={root} className="dictation-waveform" aria-hidden="true">{Array.from({ length: 9 }, (_, index) => <span key={index} />)}</span>;
}
