import React, { useContext } from "react";
import { Box, Text, TerminalSizeContext } from "@claude-code-kit/ink-renderer";
import { Markdown } from "@claude-code-kit/ui";

export type ResultTone = "success" | "warning" | "error" | "muted" | "";
export type ResultField = { label: string; value: string; tone?: ResultTone };
export type ResultItem = { label: string; detail?: string; state?: string; tone?: ResultTone };
export type ResultSection = { title: string; summary?: string; fields?: ResultField[]; items?: ResultItem[]; lines?: string[]; markdown?: string };
export type BackendResult = {
  kind: "notice" | "details" | "list" | "report" | "preview" | "approval";
  title: string;
  tone?: ResultTone;
  summary?: string;
  fields?: ResultField[];
  items?: ResultItem[];
  sections?: ResultSection[];
  lines?: string[];
  markdown?: string;
  preview?: unknown;
  footer?: string;
};

const colors: Record<Exclude<ResultTone, "">, string> = {
  success: "#4BAA72", warning: "#D6AD69", error: "#DF7777", muted: "#86BE9A",
};
const green = "#4BAA72";
const muted = "#86BE9A";

function toneColor(tone?: ResultTone): string | undefined { return tone ? colors[tone] : undefined; }

function JsonPreview({ value, depth = 0 }: { value: unknown; depth?: number }): React.ReactNode {
  if (value === null || typeof value !== "object") {
    return <Text color={typeof value === "string" ? undefined : muted}>{JSON.stringify(value) ?? String(value)}</Text>;
  }
  if (depth >= 12) return <Text dimColor>…</Text>;
  const entries = Array.isArray(value) ? value.map((item, index) => [String(index + 1), item] as const) : Object.entries(value);
  if (entries.length === 0) return <Text dimColor>{Array.isArray(value) ? "[]" : "{}"}</Text>;
  return <Box flexDirection="column">{entries.map(([key, item]) => <Box key={key} flexDirection="column" marginLeft={depth ? 2 : 0}>
    <Box><Text color={muted}>{key}</Text><Text dimColor>: </Text>{item === null || typeof item !== "object" ? <JsonPreview value={item} depth={depth + 1} /> : null}</Box>
    {item !== null && typeof item === "object" ? <Box marginLeft={2}><JsonPreview value={item} depth={depth + 1} /></Box> : null}
  </Box>)}</Box>;
}

function ResultContent({ fields, items, lines, markdown }: Pick<BackendResult, "fields" | "items" | "lines" | "markdown">): React.ReactNode {
  return <Box flexDirection="column">
    {fields?.map((field, index) => <Box key={`${index}-${field.label}`} flexDirection="column" marginBottom={index + 1 === fields.length ? 0 : 0}>
      <Text color={muted}>{field.label}</Text>
      <Box marginLeft={2}><Text color={toneColor(field.tone)}>{field.value}</Text></Box>
    </Box>)}
    {items?.map((item, index) => <Box key={`${index}-${item.label}`} flexDirection="column" marginTop={index ? 1 : 0}>
      <Box><Text color={toneColor(item.tone) ?? green}>• </Text><Text bold>{item.label}</Text>{item.state ? <Text color={toneColor(item.tone) ?? muted}>  ·  {item.state}</Text> : null}</Box>
      {item.detail ? <Box marginLeft={2}><Text dimColor>{item.detail}</Text></Box> : null}
    </Box>)}
    {lines?.map((line, index) => <Text key={index}>{line || " "}</Text>)}
    {markdown ? <Markdown>{markdown}</Markdown> : null}
  </Box>;
}

export function ResultBody({ result }: { result: BackendResult }): React.ReactNode {
  const terminal = useContext(TerminalSizeContext);
  const width = Math.max(16, Math.min(76, (terminal?.columns ?? 80) - 7));
  const marker = result.kind === "approval" ? "◇" : result.tone === "error" ? "!" : result.tone === "success" ? "✓" : "◌";
  return <Box flexDirection="column" width={width}>
    <Box><Text color={toneColor(result.tone) ?? green}>{marker} </Text><Text color={toneColor(result.tone) ?? green} bold>{result.title}</Text></Box>
    {result.summary ? <Box marginLeft={2}><Text>{result.summary}</Text></Box> : null}
    {(result.fields?.length || result.items?.length || result.lines?.length || result.markdown) ? <Box marginLeft={2} marginTop={1} flexDirection="column"><ResultContent fields={result.fields} items={result.items} lines={result.lines} markdown={result.markdown} /></Box> : null}
    {result.sections?.map((section, index) => <Box key={`${index}-${section.title}`} marginLeft={2} marginTop={1} flexDirection="column">
      <Text color={muted} bold>{section.title}</Text>
      {section.summary ? <Text dimColor>{section.summary}</Text> : null}
      <Box marginLeft={2} flexDirection="column"><ResultContent fields={section.fields} items={section.items} lines={section.lines} markdown={section.markdown} /></Box>
    </Box>)}
    {result.kind === "approval" && result.preview !== undefined ? <Box marginLeft={2} marginTop={1} flexDirection="column">
      <Text color={muted} bold>What Gator will do</Text>
      <Box marginLeft={2}><JsonPreview value={result.preview} /></Box>
    </Box> : null}
    {result.footer ? <Box marginLeft={2} marginTop={1}><Text color={result.kind === "approval" && result.tone === "warning" ? colors.warning : muted}>{result.footer}</Text></Box> : null}
  </Box>;
}
