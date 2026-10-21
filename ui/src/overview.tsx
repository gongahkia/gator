import React from "react";
import { Box, Text } from "@claude-code-kit/ink-renderer";
import { useGatorPalette } from "./theme.js";

import type { Overview } from "./protocol.js";
export type { Overview } from "./protocol.js";

export const overviewLabels: Record<string, string> = {
  changed: "Changed",
  needs: "Needs you",
  next: "Next",
  blocked: "Blocked",
};

// Home retains all four questions at every terminal size. Detailed items and
// their host-owned actions remain in the existing picker behind /overview.
export function EverydayOverview({ view, issue, compact }: { view?: Overview; issue?: string; compact: boolean }): React.ReactNode {
  const palette = useGatorPalette();
  return (
    <Box flexDirection="column">
      {!compact && <Text color={palette.secondary} bold>Everyday overview · /overview</Text>}
      {issue ? <Text color={palette.warning} wrap="truncate-end">Overview unavailable · /overview to retry</Text> : !view ? (
        <Text dimColor wrap="truncate-end">Reading your overview…</Text>
      ) : view.sections.map((section) => (
        <Box key={section.id} flexDirection="column">
          <Text wrap="truncate-end">
            <Text bold color={section.id === "blocked" && section.total > 0 ? palette.warning : section.id === "needs" && section.total > 0 ? palette.accent : palette.secondary}>
              {compact ? overviewLabels[section.id] ?? section.title : section.title}
            </Text>
            {" · "}{section.summary}
          </Text>
        </Box>
      ))}
      <Text dimColor wrap="truncate-end">{compact ? "/overview · Ctrl+O" : "Ctrl+O open actions · refreshes every 15s · retained facts, no model call"}</Text>
    </Box>
  );
}
