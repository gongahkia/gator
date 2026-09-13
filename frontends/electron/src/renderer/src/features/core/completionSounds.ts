import type { SettingsSnapshot } from "@shared/ipc/contracts";

type CompletionSoundId = SettingsSnapshot["taskCompletionSoundId"];

interface CompletionTone {
  duration: number;
  frequency: number;
  gain?: number;
  offset?: number;
  type?: OscillatorType;
}

const completionSoundPatterns: Record<CompletionSoundId, CompletionTone[]> = {
  arcade: [
    { frequency: 660, duration: 0.08, gain: 0.055, type: "square" },
    { frequency: 880, duration: 0.12, gain: 0.045, offset: 0.08, type: "square" }
  ],
  chime: [
    { frequency: 660, duration: 0.16, gain: 0.065 },
    { frequency: 990, duration: 0.22, gain: 0.035, offset: 0.08 }
  ],
  click: [{ frequency: 320, duration: 0.05, gain: 0.055, type: "square" }],
  coin: [
    { frequency: 988, duration: 0.07, gain: 0.055, type: "square" },
    { frequency: 1318, duration: 0.11, gain: 0.05, offset: 0.07, type: "square" }
  ],
  ding: [{ frequency: 1046, duration: 0.22, gain: 0.07 }],
  glass: [
    { frequency: 840, duration: 0.13, gain: 0.06 },
    { frequency: 1240, duration: 0.18, gain: 0.04, offset: 0.05 }
  ],
  pluck: [
    { frequency: 520, duration: 0.08, gain: 0.07, type: "triangle" },
    { frequency: 780, duration: 0.11, gain: 0.04, offset: 0.06, type: "triangle" }
  ],
  pop: [{ frequency: 420, duration: 0.16, gain: 0.075, type: "sine" }],
  pulse: [
    { frequency: 360, duration: 0.07, gain: 0.055 },
    { frequency: 360, duration: 0.09, gain: 0.04, offset: 0.13 }
  ],
  rise: [
    { frequency: 440, duration: 0.07, gain: 0.05 },
    { frequency: 554, duration: 0.07, gain: 0.05, offset: 0.07 },
    { frequency: 659, duration: 0.12, gain: 0.05, offset: 0.14 }
  ],
  softBell: [
    { frequency: 740, duration: 0.28, gain: 0.05 },
    { frequency: 1110, duration: 0.24, gain: 0.025, offset: 0.02 }
  ],
  sparkle: [
    { frequency: 1046, duration: 0.05, gain: 0.04 },
    { frequency: 1318, duration: 0.07, gain: 0.04, offset: 0.05 },
    { frequency: 1568, duration: 0.1, gain: 0.035, offset: 0.12 }
  ],
  success: [
    { frequency: 523, duration: 0.07, gain: 0.05 },
    { frequency: 659, duration: 0.07, gain: 0.05, offset: 0.07 },
    { frequency: 784, duration: 0.16, gain: 0.055, offset: 0.14 }
  ],
  tick: [{ frequency: 1200, duration: 0.035, gain: 0.035, type: "square" }],
  wood: [
    { frequency: 260, duration: 0.05, gain: 0.065, type: "triangle" },
    { frequency: 190, duration: 0.07, gain: 0.045, offset: 0.04, type: "triangle" }
  ]
};

export function playCompletionSound(soundId: CompletionSoundId): void {
  if (typeof window === "undefined") {
    return;
  }

  const audioWindow = window as Window & {
    AudioContext?: typeof AudioContext;
    webkitAudioContext?: typeof AudioContext;
  };
  const AudioContextConstructor = audioWindow.AudioContext ?? audioWindow.webkitAudioContext;

  if (!AudioContextConstructor) {
    return;
  }

  const context = new AudioContextConstructor();
  const resume = context.resume?.();

  if (resume) {
    void resume.catch(() => undefined);
  }

  const pattern = completionSoundPatterns[soundId];
  const startedAt = context.currentTime;
  const totalDuration = pattern.reduce(
    (duration, tone) => Math.max(duration, (tone.offset ?? 0) + tone.duration),
    0
  );

  for (const tone of pattern) {
    const oscillator = context.createOscillator();
    const gain = context.createGain();
    const startAt = startedAt + (tone.offset ?? 0);
    const peakAt = startAt + Math.min(0.012, tone.duration / 3);
    const stopAt = startAt + tone.duration;

    oscillator.type = tone.type ?? "sine";
    oscillator.frequency.setValueAtTime(tone.frequency, startAt);
    gain.gain.setValueAtTime(0.0001, startAt);
    gain.gain.exponentialRampToValueAtTime(tone.gain ?? 0.06, peakAt);
    gain.gain.exponentialRampToValueAtTime(0.0001, stopAt);
    oscillator.connect(gain);
    gain.connect(context.destination);
    oscillator.start(startAt);
    oscillator.stop(stopAt + 0.02);
  }

  window.setTimeout(() => {
    void context.close().catch(() => undefined);
  }, Math.ceil((totalDuration + 0.08) * 1_000));
}
