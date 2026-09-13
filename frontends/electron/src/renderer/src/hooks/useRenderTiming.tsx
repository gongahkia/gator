import { Profiler, useEffect, useRef } from "react";
import type { ProfilerOnRenderCallback, ReactNode } from "react";

const renderProfilingEnabled =
  import.meta.env.MODE === "performance" || import.meta.env.VITE_HCB_RENDER_PROFILING === "true";

type TimingMetadata = Record<string, string | number | boolean | null | undefined>;

function sanitizedMetadata(metadata: TimingMetadata | undefined): Record<string, string | number | boolean | null> {
  if (metadata === undefined) {
    return {};
  }

  return Object.fromEntries(
    Object.entries(metadata).filter((entry): entry is [string, string | number | boolean | null] => {
      const value = entry[1];

      return value === null || typeof value === "string" || typeof value === "number" || typeof value === "boolean";
    })
  );
}

export function reportRendererTiming(
  label: string,
  durationMs: number,
  metadata?: TimingMetadata
): void {
  if (!renderProfilingEnabled) {
    return;
  }

  console.debug("[hcb-render]", {
    durationMs: Number(durationMs.toFixed(2)),
    label,
    ...sanitizedMetadata(metadata)
  });
}

export function rendererTimingEnabled(): boolean {
  return renderProfilingEnabled;
}

export function rendererNow(): number | null {
  if (!renderProfilingEnabled || typeof performance === "undefined") {
    return null;
  }

  const now = performance.now();
  return Number.isFinite(now) ? now : null;
}

export function reportRendererTimingSince(
  label: string,
  startedAt: number | null,
  metadata?: TimingMetadata
): void {
  const now = rendererNow();

  if (startedAt === null || now === null) {
    return;
  }

  reportRendererTiming(label, now - startedAt, metadata);
}

export function useRenderTiming(label: string): void {
  const startedAt = useRef<number | null>(null);

  if (renderProfilingEnabled && typeof performance !== "undefined") {
    startedAt.current = performance.now();
  }

  useEffect(() => {
    if (!renderProfilingEnabled || startedAt.current === null || typeof performance === "undefined") {
      return;
    }

    reportRendererTiming(label, performance.now() - startedAt.current);
  });
}

const handleProfilerRender: ProfilerOnRenderCallback = (
  id,
  phase,
  actualDuration
): void => {
  reportRendererTiming(`${id}:${phase}`, actualDuration);
};

export function RenderTimingBoundary({
  children,
  id
}: {
  children: ReactNode;
  id: string;
}): JSX.Element {
  if (!renderProfilingEnabled) {
    return <>{children}</>;
  }

  return (
    <Profiler id={id} onRender={handleProfilerRender}>
      {children}
    </Profiler>
  );
}
