import React, { useContext } from "react";
import { Box, Text, TerminalSizeContext } from "@claude-code-kit/ink-renderer";
import { Markdown } from "@claude-code-kit/ui";
import { useGatorPalette, type GatorPalette } from "./theme.js";

import type { BackendResult, ResultTone } from "./protocol.js";
export type { BackendResult, ResultField, ResultItem, ResultSection, ResultTone } from "./protocol.js";

function toneColor(tone: ResultTone | undefined, palette: GatorPalette): string | undefined {
  return tone === "success" ? palette.accent : tone === "warning" ? palette.warning : tone === "error" ? palette.error : tone === "muted" ? palette.secondary : undefined;
}

function JsonPreview({ value, depth = 0 }: { value: unknown; depth?: number }): React.ReactNode {
  const palette = useGatorPalette();
  if (value === null || typeof value !== "object") {
    return <Text color={typeof value === "string" ? undefined : palette.secondary}>{JSON.stringify(value) ?? String(value)}</Text>;
  }
  if (depth >= 12) return <Text>{JSON.stringify(value, null, 2)}</Text>;
  const entries = Array.isArray(value) ? value.map((item, index) => [String(index + 1), item] as const) : Object.entries(value);
  if (entries.length === 0) return <Text dimColor>{Array.isArray(value) ? "[]" : "{}"}</Text>;
  return <Box flexDirection="column">{entries.map(([key, item]) => <Box key={key} flexDirection="column" marginLeft={depth ? 2 : 0}>
    <Box><Text color={palette.secondary}>{key}</Text><Text dimColor>: </Text>{item === null || typeof item !== "object" ? <JsonPreview value={item} depth={depth + 1} /> : null}</Box>
    {item !== null && typeof item === "object" ? <Box marginLeft={2}><JsonPreview value={item} depth={depth + 1} /></Box> : null}
  </Box>)}</Box>;
}

function ResultContent({ fields, items, lines, markdown }: Pick<BackendResult, "fields" | "items" | "lines" | "markdown">): React.ReactNode {
  const palette = useGatorPalette();
  const hasFields = (fields?.length ?? 0) > 0;
  const hasItems = (items?.length ?? 0) > 0;
  return <Box flexDirection="column">
    {fields?.map((field, index) => <Text key={`${index}-${field.label}`}><Text color={palette.secondary}>{field.label}: </Text><Text color={toneColor(field.tone, palette)}>{field.value}</Text></Text>)}
    {items?.map((item, index) => <Box key={`${index}-${item.label}`} flexDirection="column" marginTop={index || hasFields ? 1 : 0}>
      <Box><Text color={toneColor(item.tone, palette) ?? palette.accent}>• </Text><Text bold>{item.label}</Text>{item.state ? <Text color={toneColor(item.tone, palette) ?? palette.secondary}>  ·  {item.state}</Text> : null}</Box>
      {item.detail ? <Box marginLeft={2}><Text dimColor>{item.detail}</Text></Box> : null}
    </Box>)}
    {lines?.map((line, index) => <Box key={index} marginTop={index === 0 && (hasFields || hasItems) ? 1 : 0}><Text>{line || " "}</Text></Box>)}
    {markdown ? <Box marginTop={hasFields || hasItems ? 1 : 0}><Markdown key={palette.accent}>{markdown}</Markdown></Box> : null}
  </Box>;
}

export function ResultBody({ result }: { result: BackendResult }): React.ReactNode {
  const terminal = useContext(TerminalSizeContext);
  const palette = useGatorPalette();
  const width = Math.max(1, Math.min(76, (terminal?.columns ?? 80) - 5));
  const marker = result.kind === "approval" ? "◇" : result.tone === "error" ? "!" : result.tone === "success" ? "✓" : "◌";
  return <Box flexDirection="column" width={width}>
    <Box><Text color={toneColor(result.tone, palette) ?? palette.accent}>{marker} </Text><Text color={toneColor(result.tone, palette) ?? palette.accent} bold>{result.title}</Text></Box>
    {result.summary ? <Box marginLeft={2}><Text>{result.summary}</Text></Box> : null}
    {(result.fields?.length || result.items?.length || result.lines?.length || result.markdown) ? <Box marginLeft={2} marginTop={1} flexDirection="column"><ResultContent fields={result.fields} items={result.items} lines={result.lines} markdown={result.markdown} /></Box> : null}
    {result.sections?.map((section, index) => <Box key={`${index}-${section.title}`} marginLeft={2} marginTop={1} flexDirection="column">
      <Text color={palette.secondary} bold>{section.title}</Text>
      {section.summary ? <Text dimColor>{section.summary}</Text> : null}
      <Box marginLeft={2} flexDirection="column"><ResultContent fields={section.fields} items={section.items} lines={section.lines} markdown={section.markdown} /></Box>
    </Box>)}
    {result.kind === "approval" && result.preview !== undefined ? <Box marginLeft={2} marginTop={1} flexDirection="column">
      <Text color={palette.secondary} bold>What Gator will do</Text>
      <Box marginLeft={2}><JsonPreview value={result.preview} /></Box>
    </Box> : null}
    {result.footer ? <Box marginLeft={2} marginTop={1}><Text color={result.kind === "approval" && result.tone === "warning" ? palette.warning : palette.secondary}>{result.footer}</Text></Box> : null}
  </Box>;
}
