import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { createInterface } from "node:readline";
import { Box, Text, render, useApp, useInput, TerminalSizeContext, type Instance } from "@claude-code-kit/ink-renderer";
import { Byline, Divider, KeyboardShortcutHint, Markdown, MessageList, Spinner, StatusLine, WelcomeScreen, type Message, type VirtualListHandle } from "@claude-code-kit/ui";
import React, { useContext, useEffect, useRef, useState } from "react";
import { runDoctor } from "./doctor.js";
import { ResultBody, type BackendResult } from "./result.js";
import { searchMessages } from "./search.js";
import { panelGeometry, wrapTerminalText } from "./layout.js";
import { cursorAtGraphemeColumn, graphemeAt, graphemeColumn, nextGrapheme, previousGrapheme } from "./text.js";
import { GatorPaletteContext, paletteForTheme, useGatorPalette } from "./theme.js";

type BackendCommand = { name: string; description: string; advanced: boolean; needs_input: boolean };
type BackendBundle = {
  Path: string;
  Status: string;
  Artifacts: { Path: string; MediaType: string; Valid: boolean }[] | null;
  Candidates: { ID: string; Status: string; ChangedPaths: string[] }[] | null;
};
type BackendMessage = { role: string; text: string; bundle?: BackendBundle; result?: BackendResult };
type BackendPanel = {
  kind: string;
  title: string;
  subtitle?: string;
  query?: string;
  rows?: { label: string; detail?: string; state?: string; tone?: "success" | "warning" | "error" | "muted" }[];
  blocks?: { title?: string; lines: string[] }[];
  selected?: number;
  notice?: string;
  footer?: string;
  hints?: { shortcut: string; action: string }[];
};
type ExternalCommand = { kind: string; program: string; args: string[]; dir?: string; env?: string[] };
type BackendState = {
  mode: "chat" | "panel";
  panel?: BackendPanel;
  messages: BackendMessage[];
  commands: BackendCommand[];
  actions: { Label: string; Command: string }[];
  title: string;
  source: string;
  provider?: string;
  model?: string;
  theme: string;
  status?: string;
  status_line: string[];
  status_line_enabled: boolean;
  input?: string;
  running: boolean;
  cancelling: boolean;
  elapsed?: string;
  loading_frame: number;
  queued: number;
  home: boolean;
  first_run: boolean;
  supervisor_needs_start: boolean;
  external?: ExternalCommand;
};
type Frame = { version: 1; type: "submit" | "key" | "resize" | "exit" | "external_done"; text?: string; key?: string; error?: string; width?: number; height?: number };

class GatorBridge {
  private child: ChildProcessWithoutNullStreams;
  private current?: BackendState;
  private listeners = new Set<(state: BackendState) => void>();
  private exits = new Set<(code: number | null, detail: string) => void>();
  private errorText = "";
  private closed = false;

  constructor(binary: string, conversationID: string, goalID: string) {
    this.child = spawn(binary, ["ui-backend", conversationID, goalID], {
      env: { ...process.env, GATOR_UI_BACKEND: "1" },
      stdio: ["pipe", "pipe", "pipe"],
    });
    const lines = createInterface({ input: this.child.stdout });
    lines.on("line", (line) => {
      try {
        const frame = JSON.parse(line) as { version: number; type: string; state?: BackendState };
        if (frame.version !== 1 || frame.type !== "state" || !frame.state) throw new Error("invalid Gator UI frame");
        this.current = frame.state;
        for (const listener of this.listeners) listener(frame.state);
      } catch (error) {
        this.errorText = `Gator UI protocol error: ${String(error)}`;
        this.stop();
      }
    });
    this.child.stderr.on("data", (chunk: Buffer) => {
      this.errorText = (this.errorText + chunk.toString()).slice(-4000);
    });
    this.child.on("error", (error) => { this.errorText = error.message; });
    this.child.on("exit", (code) => {
      this.closed = true;
      for (const listener of this.exits) listener(code, this.errorText.trim());
    });
  }

  subscribe(listener: (state: BackendState) => void): () => void {
    this.listeners.add(listener);
    if (this.current) listener(this.current);
    return () => { this.listeners.delete(listener); };
  }
  snapshot(): BackendState | undefined { return this.current; }
  onExit(listener: (code: number | null, detail: string) => void): () => void {
    this.exits.add(listener);
    return () => { this.exits.delete(listener); };
  }
  send(frame: Omit<Frame, "version">): void {
    if (!this.closed && this.child.stdin.writable) this.child.stdin.write(`${JSON.stringify({ version: 1, ...frame })}\n`);
  }
  resize(): void {
    this.send({ type: "resize", width: process.stdout.columns || 80, height: process.stdout.rows || 24 });
  }
  stop(): void {
    if (this.closed) return;
    this.send({ type: "exit" });
    this.child.stdin.end();
    const timeout = setTimeout(() => this.child.kill(), 2000);
    timeout.unref();
  }
}

function bundleResult(bundle: BackendBundle): BackendResult {
  return {
    kind: "list", title: "Verified deliverables", tone: bundle.Status === "verified" ? "success" : "warning",
    summary: bundle.Status,
    items: [
      ...(bundle.Artifacts ?? []).map((item) => ({ label: item.Path, detail: item.MediaType, state: item.Valid ? "verified" : "needs review", tone: item.Valid ? "success" as const : "warning" as const })),
      ...(bundle.Candidates ?? []).map((item) => ({ label: `Change ${item.ID}`, detail: item.ChangedPaths.join(", "), state: item.Status })),
    ],
    footer: bundle.Path,
  };
}

function MessageBody({ item }: { item: BackendMessage }): React.ReactNode {
  const palette = useGatorPalette();
  const isUser = item.role === "You";
  const isGator = item.role === "Gator" || item.role === "Doctor";
  const label = isUser ? "› You" : isGator ? "🐊 Gator" : item.role === "Deliverables" ? "◈ Deliverables" : `◌ ${item.role}`;
  return (
    <Box flexDirection="column">
      <Text color={isGator ? palette.accent : isUser ? undefined : palette.secondary} bold={!isUser} dimColor={isUser}>{label}</Text>
      <Box marginLeft={2} flexDirection="column">
        {item.result ? <ResultBody result={item.result} /> : item.bundle ? <ResultBody result={bundleResult(item.bundle)} /> : isGator ? <Markdown>{item.text}</Markdown> : item.text.split("\n").map((line, index) => <Text key={index} dimColor={!isUser}>{line || " "}</Text>)}
      </Box>
    </Box>
  );
}

function GreenPrompt({
  value, onChange, onSubmit, history, disabled, prefillVersion,
}: {
  value: string;
  onChange: (value: string) => void;
  onSubmit: (value: string) => void;
  history: string[];
  disabled: boolean;
  prefillVersion: number;
}): React.ReactNode {
  const palette = useGatorPalette();
  const [cursor, setCursor] = useState(value.length);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const valueRef = useRef(value);
  const cursorRef = useRef(cursor);
  const historyIndexRef = useRef(historyIndex);
  valueRef.current = value;
  cursorRef.current = cursor;
  const moveCursor = (position: number) => { cursorRef.current = position; setCursor(position); };
  const moveHistory = (position: number) => { historyIndexRef.current = position; setHistoryIndex(position); };
  useEffect(() => { valueRef.current = value; moveCursor(value.length); }, [prefillVersion]);
  const replace = (next: string, position: number) => {
    valueRef.current = next;
    onChange(next);
    moveCursor(position);
    moveHistory(-1);
  };
  useInput((input, key) => {
    if (disabled) return;
    const current = valueRef.current;
    const position = cursorRef.current;
    const historyPosition = historyIndexRef.current;
    if ((key.return && key.shift) || (key.ctrl && input === "j")) { replace(current.slice(0, position) + "\n" + current.slice(position), position + 1); return; }
    if (key.return) { if (current.trim()) onSubmit(current); valueRef.current = ""; moveCursor(0); moveHistory(-1); return; }
    if (key.leftArrow) { moveCursor(previousGrapheme(current, position)); return; }
    if (key.rightArrow) { moveCursor(nextGrapheme(current, position)); return; }
    if (key.home || (key.ctrl && input === "a")) { moveCursor(0); return; }
    if (key.end || (key.ctrl && input === "e")) { moveCursor(current.length); return; }
    const beforeCursor = current.slice(0, position);
    const currentLine = beforeCursor.split("\n").length - 1;
    const lines = current.split("\n");
    const column = graphemeColumn(lines[currentLine], beforeCursor.length - beforeCursor.lastIndexOf("\n") - 1);
    if (key.upArrow && currentLine > 0) {
      const start = lines.slice(0, currentLine - 1).reduce((count, line) => count + line.length + 1, 0);
      moveCursor(start + cursorAtGraphemeColumn(lines[currentLine - 1], column)); return;
    }
    if (key.downArrow && currentLine + 1 < lines.length) {
      const start = lines.slice(0, currentLine + 1).reduce((count, line) => count + line.length + 1, 0);
      moveCursor(start + cursorAtGraphemeColumn(lines[currentLine + 1], column)); return;
    }
    if (key.upArrow && historyPosition + 1 < history.length) {
      const next = historyPosition + 1; moveHistory(next); valueRef.current = history[next]; onChange(history[next]); moveCursor(history[next].length); return;
    }
    if (key.downArrow && historyPosition >= 0) {
      const next = historyPosition - 1; moveHistory(next); valueRef.current = next < 0 ? "" : history[next]; onChange(valueRef.current); moveCursor(valueRef.current.length); return;
    }
    if (key.backspace) { if (position > 0) { const previous = previousGrapheme(current, position); replace(current.slice(0, previous) + current.slice(position), previous); } return; }
    if (key.delete) { if (position < current.length) replace(current.slice(0, position) + current.slice(nextGrapheme(current, position)), position); return; }
    if (key.ctrl && input === "u") { replace(current.slice(position), 0); return; }
    if (key.ctrl && input === "w") {
      const before = current.slice(0, position);
      const keep = before.replace(/\s*\S+\s*$/, "");
      replace(keep + current.slice(position), keep.length);
      return;
    }
    if (key.ctrl || key.meta || key.tab || key.escape) return;
    if (input) replace(current.slice(0, position) + input + current.slice(position), position + input.length);
  }, { isActive: !disabled });
  return (
    <Box flexDirection="column">
      {value ? value.split("\n").map((line, index, lines) => {
        const start = lines.slice(0, index).reduce((count, item) => count + item.length + 1, 0);
        const localCursor = cursor - start;
        return <Box key={index}>
          <Text color={palette.accent}>{index === 0 ? "❯ " : "· "}</Text>
          {localCursor >= 0 && localCursor <= line.length
            ? <Text>{line.slice(0, localCursor)}<Text inverse>{graphemeAt(line, localCursor) || " "}</Text>{line.slice(nextGrapheme(line, localCursor))}</Text>
            : <Text>{line || " "}</Text>}
        </Box>;
      }) : <Box><Text color={palette.accent}>❯ </Text><Text><Text inverse> </Text><Text dimColor>Ask Gator to work on something…</Text></Text></Box>}
    </Box>
  );
}

function ChatKeys({
  state, draftRef, selectedRef, dismissedRef, setSelected, choose, dismiss, search, scroll, scrollToEnd, exit, clearDraft, bridge,
}: {
  state: BackendState;
  draftRef: React.RefObject<string>;
  selectedRef: React.RefObject<number>;
  dismissedRef: React.RefObject<boolean>;
  setSelected: React.Dispatch<React.SetStateAction<number>>;
  choose: (command: BackendCommand) => void;
  dismiss: () => void;
  search: () => void;
  scroll: (rows: number) => void;
  scrollToEnd: () => void;
  exit: () => void;
  clearDraft: () => void;
  bridge: GatorBridge;
}): null {
  useInput((input, key, event) => {
    const draft = draftRef.current;
    const matches = !dismissedRef.current && draft.startsWith("/") && !draft.includes(" ")
      ? state.commands.filter((command) => (draft === "/" ? !command.advanced : `/${command.name}`.startsWith(draft))).slice(0, 8)
      : [];
    if (key.ctrl && input === "d") { event.stopImmediatePropagation(); exit(); return; }
    if (key.ctrl && input === "c" && !state.running) { event.stopImmediatePropagation(); clearDraft(); return; }
    if ((key.ctrl && input === "c") || (key.escape && state.running)) {
      event.stopImmediatePropagation(); bridge.send({ type: "key", key: "esc" }); return;
    }
    if (key.ctrl && input === "f") { event.stopImmediatePropagation(); search(); return; }
    if (key.ctrl && key.end) { event.stopImmediatePropagation(); scrollToEnd(); return; }
    const shortcuts: Record<string, string> = { p: "ctrl+p", x: "ctrl+x", b: "ctrl+b", i: "ctrl+i", g: "ctrl+g" };
    if (key.ctrl && shortcuts[input]) {
      event.stopImmediatePropagation(); bridge.send({ type: "key", key: shortcuts[input], text: input === "g" ? draft : undefined }); return;
    }
    if (key.pageUp || key.pageDown || key.wheelUp || key.wheelDown) {
      event.stopImmediatePropagation(); scroll(key.pageUp ? -12 : key.pageDown ? 12 : key.wheelUp ? -3 : 3); return;
    }
    if (matches.length === 0) {
      if (key.escape) { event.stopImmediatePropagation(); bridge.send({ type: "key", key: "esc" }); }
      return;
    }
    if (key.escape) { event.stopImmediatePropagation(); dismiss(); }
    else if (key.upArrow || key.downArrow) {
      event.stopImmediatePropagation();
      const next = (selectedRef.current + (key.upArrow ? -1 : 1) + matches.length) % matches.length;
      selectedRef.current = next;
      setSelected(next);
    } else if (key.return || key.tab) {
      event.stopImmediatePropagation();
      const exact = matches.find((command) => `/${command.name}` === draft.trim());
      choose(exact ?? matches[Math.min(selectedRef.current, matches.length - 1)]);
    }
  }, { captureCtrlC: true });
  return null;
}

function PanelKeys({ bridge, scroll, exit }: { bridge: GatorBridge; scroll: (amount: number) => void; exit: () => void }): null {
  useInput((input, key, event) => {
    event.stopImmediatePropagation();
    if (key.ctrl && input === "d") { exit(); return; }
    if (key.pageUp || key.wheelUp) { scroll(-8); return; }
    if (key.pageDown || key.wheelDown) { scroll(8); return; }
    let name = "";
    if (key.escape) name = "esc";
    else if (key.return) name = "enter";
    else if (key.upArrow) name = "up";
    else if (key.downArrow) name = "down";
    else if (key.leftArrow) name = "left";
    else if (key.rightArrow) name = "right";
    else if (key.backspace) name = "backspace";
    else if (key.delete) name = "delete";
    else if (key.pageUp) name = "pgup";
    else if (key.pageDown) name = "pgdown";
    else if (key.tab && key.shift) name = "shift+tab";
    else if (key.tab) name = "tab";
    else if (input === "f1") name = "f1";
    else if (key.ctrl) name = `ctrl+${input}`;
    else if (input === " ") name = "space";
    else if (!key.meta) name = input;
    if (name) bridge.send({ type: "key", key: name });
  }, { captureCtrlC: true });
  return null;
}

function PanelScreen({ panel, status, draft, statusLine, bridge, exit }: { panel: BackendPanel; status?: string; draft?: string; statusLine?: string[]; bridge: GatorBridge; exit: () => void }): React.ReactNode {
  const terminal = useContext(TerminalSizeContext);
  const palette = useGatorPalette();
  const [offset, setOffset] = useState(0);
  useEffect(() => { setOffset(0); }, [panel.kind, panel.title, panel.subtitle]);
  const rows = panel.rows ?? [];
  const focused = panel.kind === "confirmation" || panel.kind === "model-form" || panel.kind === "learning-form";
  const geometry = panelGeometry(terminal?.columns ?? 80, terminal?.rows ?? 24, focused, panel.kind === "picker", !!panel.subtitle);
  const blocks = (panel.blocks ?? []).flatMap((block) => [
    ...(block.title ? [{ text: block.title, heading: true }] : []),
    ...(block.lines.length ? block.lines.flatMap((line) => wrapTerminalText(line, geometry.contentWidth).map((text) => ({ text, heading: false }))) : [{ text: "None", heading: false }]),
    { text: "", heading: false },
  ]);
  const rowHeight = rows.some((row) => row.detail) ? 3 : 2;
  const noticeRows = panel.notice || status ? 1 : 0;
  const bodyRows = Math.max(1, geometry.bodyRows - noticeRows);
  const maxRows = Math.max(1, Math.floor((blocks.length > 0 ? Math.ceil(bodyRows / 2) : bodyRows) / rowHeight));
  const selected = Math.min(Math.max(0, panel.selected ?? 0), Math.max(0, rows.length - 1));
  const rowStart = Math.max(0, Math.min(selected - Math.floor(maxRows / 2), rows.length - maxRows));
  const maxLines = Math.max(1, bodyRows - (rows.length > 0 ? Math.min(rows.length, maxRows) * rowHeight : 0));
  const blockOffset = Math.min(offset, Math.max(0, blocks.length - maxLines));
  return (
    <Box flexDirection="column" flexGrow={1} paddingX={geometry.paddingX} paddingTop={geometry.compact ? 0 : 1}>
      <PanelKeys bridge={bridge} exit={exit} scroll={(amount) => setOffset((current) => Math.max(0, Math.min(blocks.length - maxLines, current + amount)))} />
      <Box flexDirection="column" flexGrow={focused ? 0 : 1} width={geometry.panelWidth} alignSelf={focused ? "center" : undefined} borderStyle={focused && !geometry.compact ? "round" : undefined} borderColor={focused && !geometry.compact ? palette.accent : undefined} paddingX={focused && !geometry.compact ? 1 : 0} marginTop={focused && !geometry.compact ? 1 : 0}>
        <Text color={palette.accent} bold wrap="truncate-end">🐊 Gator  <Text color={palette.secondary}>{panel.title}</Text></Text>
        {geometry.showSubtitle && <Box marginTop={1}><Text wrap="truncate-end">{panel.subtitle}</Text></Box>}
        {panel.kind === "picker" && (geometry.compact
          ? <Text wrap="truncate-end"><Text color={palette.secondary}>Search </Text>{panel.query || "type to filter"}<Text inverse> </Text></Text>
          : <Box marginTop={1} width={geometry.contentWidth} borderStyle="round" borderColor={palette.secondary} paddingX={1}>
              <Text wrap="truncate-end">{panel.query || "Type to filter"}<Text inverse> </Text></Text>
            </Box>)}
        <Box marginTop={geometry.compact ? 0 : 1}><Divider width={geometry.contentWidth} color={palette.accent} /></Box>
        <Box flexDirection="column" flexGrow={focused ? 0 : 1} marginTop={1}>
        {rows.slice(rowStart, rowStart + maxRows).map((row, index) => {
          const active = panel.kind !== "inbox" && panel.kind !== "jobs" && rowStart + index === selected;
          return <Box key={`${rowStart + index}-${row.label}`} flexDirection="column" marginBottom={geometry.compact ? 0 : 1}>
            <Text wrap="truncate-end" color={active ? palette.accent : undefined} bold={active}>{active ? "❯ " : "  "}{row.label}{row.state ? <Text color={row.tone === "warning" ? palette.warning : row.tone === "error" ? palette.error : row.tone === "success" ? palette.accent : palette.secondary}>  ·  {row.state}</Text> : null}</Text>
            {row.detail && <Box marginLeft={2}><Text wrap="truncate-end" dimColor={!active}>{row.detail}{active && (panel.kind === "model-form" || panel.kind === "learning-form") && <Text inverse> </Text>}</Text></Box>}
          </Box>;
        })}
        {blocks.slice(blockOffset, blockOffset + maxLines).map((line, index) => <Box key={`${blockOffset + index}-${line.text}`} marginBottom={line.heading ? 0 : undefined}>
          <Text wrap="truncate-end" color={line.heading ? palette.secondary : undefined} bold={line.heading}>{line.text || " "}</Text>
        </Box>)}
        {blocks.length > maxLines && <Text dimColor wrap="truncate-end">{`${blockOffset + 1}–${Math.min(blockOffset + maxLines, blocks.length)} of ${blocks.length} · PgUp/PgDn`}</Text>}
        {rows.length === 0 && blocks.length === 0 && !panel.notice && <Text dimColor>No items to show.</Text>}
        {panel.notice && <Box marginTop={geometry.compact ? 0 : 1}><Text wrap="truncate-end" color={palette.secondary}>{panel.notice}</Text></Box>}
        {status && status !== panel.notice && <Box marginTop={geometry.compact ? 0 : 1}><Text wrap="truncate-end" dimColor>{status}</Text></Box>}
        </Box>
        <Divider width={geometry.contentWidth} color={palette.accent} />
        <Text wrap="truncate-end" dimColor>{geometry.compact ? "Esc back" : panel.hints?.length
          ? <Byline>{panel.hints.map((hint) => <KeyboardShortcutHint key={`${hint.shortcut}-${hint.action}`} shortcut={hint.shortcut} action={hint.action} />)}</Byline>
          : panel.footer || "Esc back"}</Text>
      </Box>
      {focused && <Box flexGrow={1} />}
      {geometry.showComposerAnchor && <><Box marginTop={1}><Divider width={geometry.panelWidth} color={palette.accent} /></Box>
      <Text wrap="truncate-end" dimColor>❯ {draft || "Ask Gator to work on something…"}</Text></>}
      {!geometry.compact && statusLine && statusLine.length > 0 && <StatusLine segments={statusLine.map((content) => ({ content, color: palette.secondary }))} />}
    </Box>
  );
}

function GreenSearch({ messages, close, navigate }: { messages: BackendMessage[]; close: () => void; navigate: (index: number) => void }): React.ReactNode {
  const palette = useGatorPalette();
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const [matches, setMatches] = useState<ReturnType<typeof searchMessages>>([]);
  const [selected, setSelected] = useState(0);
  const queryRef = useRef(query);
  const cursorRef = useRef(cursor);
  const matchesRef = useRef(matches);
  const selectedRef = useRef(selected);
  queryRef.current = query;
  cursorRef.current = cursor;
  matchesRef.current = matches;
  selectedRef.current = selected;
  const change = (value: string, position: number) => {
    const found = searchMessages(messages, value);
    queryRef.current = value; cursorRef.current = position; matchesRef.current = found; selectedRef.current = 0;
    setQuery(value); setCursor(position); setMatches(found); setSelected(0);
    if (found.length) navigate(found[0].index);
  };
  useInput((input, key, event) => {
    event.stopImmediatePropagation();
    const current = queryRef.current;
    const position = cursorRef.current;
    const found = matchesRef.current;
    const selectedMatch = selectedRef.current;
    if (key.escape) close();
    else if (key.leftArrow) { cursorRef.current = previousGrapheme(current, position); setCursor(cursorRef.current); }
    else if (key.rightArrow) { cursorRef.current = nextGrapheme(current, position); setCursor(cursorRef.current); }
    else if (key.backspace && position > 0) { const previous = previousGrapheme(current, position); change(current.slice(0, previous) + current.slice(position), previous); }
    else if (key.delete && position < current.length) change(current.slice(0, position) + current.slice(nextGrapheme(current, position)), position);
    else if ((key.return || (key.ctrl && input === "n")) && found.length) {
      const next = (selectedMatch + 1) % found.length; selectedRef.current = next; setSelected(next); navigate(found[next].index);
    } else if (key.ctrl && input === "p" && found.length) {
      const previous = (selectedMatch - 1 + found.length) % found.length; selectedRef.current = previous; setSelected(previous); navigate(found[previous].index);
    } else if (!key.ctrl && !key.meta && input) change(current.slice(0, position) + input + current.slice(position), position + input.length);
  });
  return <Box paddingX={1}><Text color={palette.accent}>Search: </Text><Text>{query.slice(0, cursor)}<Text inverse>{graphemeAt(query, cursor) || " "}</Text>{query.slice(nextGrapheme(query, cursor))}</Text><Text dimColor>{`  ${matches.length ? `${selected + 1}/${matches.length} matches` : "no matches"}`}</Text></Box>;
}

function App({ bridge }: { bridge: GatorBridge }): React.ReactNode {
  const { exit } = useApp();
  // Keep the terminal renderer active until the backend's first state arrives.
  useInput(() => {});
  const [state, setState] = useState<BackendState | undefined>(bridge.snapshot());
  const [failure, setFailure] = useState("");
  const [input, setInput] = useState("");
  const [prefillVersion, setPrefillVersion] = useState(0);
  const [history, setHistory] = useState<string[]>([]);
  const [selected, setSelected] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const terminal = useContext(TerminalSizeContext);
  const palette = paletteForTheme(state?.theme);
  const list = useRef<VirtualListHandle>(null);
  const lastBackendInput = useRef("");
  const inputRef = useRef(input);
  const selectedRef = useRef(selected);
  const dismissedRef = useRef(dismissed);

  useEffect(() => {
    const unsubscribe = bridge.subscribe(setState);
    const unexit = bridge.onExit((code, detail) => {
      if (code === 0) exit();
      else setFailure(detail || `Gator backend exited (${code ?? "unknown"}).`);
    });
    const resize = () => bridge.resize();
    process.stdout.on("resize", resize);
    resize();
    return () => { unsubscribe(); unexit(); process.stdout.off("resize", resize); };
  }, [bridge, exit]);

  useEffect(() => {
    if (state?.mode === "chat" && state.input && state.input !== lastBackendInput.current) {
      inputRef.current = state.input;
      setInput(state.input);
      setPrefillVersion((version) => version + 1);
    }
    lastBackendInput.current = state?.input ?? "";
  }, [state?.mode, state?.input]);

  const close = () => { bridge.stop(); exit(); };
  if (!state) return <Box padding={1}><Spinner verb={failure || "Starting Gator"} color={palette.accent} /></Box>;
  if (state.mode === "panel" && state.panel) {
    return <GatorPaletteContext.Provider value={palette}><PanelScreen panel={state.panel} status={state.status} draft={input} statusLine={state.status_line_enabled ? state.status_line : undefined} bridge={bridge} exit={close} /></GatorPaletteContext.Provider>;
  }
  const matches = !searchOpen && !dismissed && input.startsWith("/") && !input.includes(" ")
    ? state.commands.filter((command) => (input === "/" ? !command.advanced : `/${command.name}`.startsWith(input))).slice(0, 8)
    : [];
  const submit = (value: string) => {
    const text = value.trim();
    if (!text) return;
    bridge.send({ type: "submit", text });
    inputRef.current = ""; dismissedRef.current = false;
    setInput(""); setDismissed(false); setHistory((current) => [text, ...current]);
  };
  const choose = (command: BackendCommand) => {
    if (command.needs_input && inputRef.current.trim() !== `/${command.name}`) {
      inputRef.current = `/${command.name} `; dismissedRef.current = true;
      setInput(`/${command.name} `);
      setPrefillVersion((version) => version + 1);
      setDismissed(true);
    } else {
      submit(`/${command.name}`);
    }
  };
  const context = [state.title, state.provider ? `${state.provider}${state.model ? `/${state.model}` : ""}` : "Choose model: /model", state.source.split("/").filter(Boolean).at(-1) || state.source].filter(Boolean).join(" · ");
  const welcome = (
    <Box flexDirection="column">
      <WelcomeScreen appName="Gator" subtitle="Your ongoing work, ready when you are" color={palette.accent} logo={<Text color={palette.accent}>🐊</Text>} />
      <Text dimColor>{context}</Text>
      <Box marginTop={1} marginLeft={1} flexDirection="column">
        <Text color={palette.secondary} bold>{state.first_run ? "Get started" : "Next up"}</Text>
        {state.first_run ? (
          <><Text>  1  /model  Choose a cloud or local model</Text><Text>  2  Ask: Summarize this folder into a daily brief with next actions.</Text></>
        ) : state.actions.slice(0, 4).map((action, index) => (
          <Text key={index}>  {action.Label} · {action.Command}</Text>
        ))}
        <Text dimColor>  Type / for Gator commands.</Text>
        {state.supervisor_needs_start && <Text color={palette.secondary}>  Recurring checks need a running supervisor · gator job supervisor</Text>}
      </Box>
    </Box>
  );
  const messages: Message[] = state.messages.map((item, index) => ({
    id: `${index}-${item.role}-${item.text.length}-${item.result?.tone ?? ""}-${item.result?.footer ?? ""}`,
    role: item.role === "You" ? "user" : item.role === "Gator" ? "assistant" : "system",
    content: item.text,
  }));
  const viewportHeight = Math.max(1, (terminal?.rows ?? 24) - 6 - matches.length - (state.running ? 2 : 0) - (searchOpen ? 1 : 0) - input.split("\n").length + 1);
  const statusSegments = (state.status_line ?? []).map((content) => ({ content, color: palette.secondary }));
  return (
    <GatorPaletteContext.Provider value={palette}>
    <Box flexDirection="column" flexGrow={1} paddingX={1} paddingTop={1}>
      {!state.home && <Box marginBottom={1}><Text color={palette.accent} bold>🐊 Gator</Text><Text color={palette.secondary}>  {context}</Text></Box>}
      <Box flexDirection="column" flexGrow={1}>
        {state.home && state.messages.length === 0 && <Box marginBottom={1}>{welcome}</Box>}
        {state.messages.length > 0 && <MessageList ref={list} viewportHeight={viewportHeight} messages={messages} renderMessage={(item) => <MessageBody item={state.messages[Number(item.id.split("-", 1)[0])]} />} />}
        {state.running && <Box marginTop={1}>{state.cancelling ? <Text color={palette.warning}>◌ Stopping… retaining evidence</Text> : <Spinner verb="Working" label={`${state.elapsed ?? ""} · Esc to interrupt`} color={palette.accent} showElapsed={false} />}</Box>}
      </Box>
      {state.status && <Text color={palette.secondary}>{state.status}</Text>}
      {!searchOpen && <ChatKeys state={state} draftRef={inputRef} selectedRef={selectedRef} dismissedRef={dismissedRef} setSelected={setSelected} choose={choose} dismiss={() => { dismissedRef.current = true; setDismissed(true); }} search={() => setSearchOpen(true)} scroll={(rows) => list.current?.scrollBy(rows)} scrollToEnd={() => list.current?.scrollToEnd()} exit={close} clearDraft={() => { inputRef.current = ""; setInput(""); setPrefillVersion((version) => version + 1); }} bridge={bridge} />}
      {searchOpen && <GreenSearch messages={state.messages} close={() => setSearchOpen(false)} navigate={(index) => list.current?.scrollTo(index)} />}
      <Divider color={palette.secondary} />
      <GreenPrompt value={input} onChange={(value) => { inputRef.current = value; selectedRef.current = 0; dismissedRef.current = false; setInput(value); setSelected(0); setDismissed(false); }} onSubmit={submit} disabled={searchOpen || !!failure} history={history} prefillVersion={prefillVersion} />
      {matches.length > 0 && <Box marginLeft={2} flexDirection="column">{matches.map((command, index) => <Box key={command.name}><Text color={index === selected ? palette.accent : undefined} bold={index === selected}>{index === selected ? "❯" : " "} /{command.name}</Text><Text dimColor>  {command.description}</Text></Box>)}</Box>}
      {failure && <Text color={palette.error}>{failure}</Text>}
      <Divider color={palette.secondary} />
      {state.status_line_enabled && statusSegments.length > 0 && <StatusLine segments={statusSegments} />}
    </Box>
    </GatorPaletteContext.Provider>
  );
}

function runExternal(command: ExternalCommand): Promise<string> {
  return new Promise((resolve) => {
    // The child shares the terminal process group. Let Ctrl+C interrupt that
    // child without killing the renderer that will resume afterward.
    const onInterrupt = () => {};
    process.on("SIGINT", onInterrupt);
    const environment = command.env?.length
      ? Object.fromEntries(command.env.map((entry) => {
        const separator = entry.indexOf("=");
        return [entry.slice(0, separator), entry.slice(separator + 1)];
      }))
      : process.env;
    const child = spawn(command.program, command.args ?? [], {
      cwd: command.dir || undefined,
      env: environment,
      stdio: "inherit",
    });
    let finished = false;
    const done = (error: string) => {
      if (finished) return;
      finished = true;
      process.off("SIGINT", onInterrupt);
      resolve(error);
    };
    child.on("error", (error) => done(error.message));
    child.on("exit", (code, signal) => done(code === 0 ? "" : signal ? `terminated by ${signal}` : `exited with status ${code}`));
  });
}

async function main(): Promise<void> {
  const [binary, mode, ...arguments_] = process.argv.slice(2);
  if (!binary) throw new Error("Gator UI requires its backend executable");
  if (mode === "doctor") return runDoctor(binary, arguments_);
  if (mode !== "work") throw new Error("unknown Gator UI mode");
  const [conversationID = "", goalID = ""] = arguments_;
  const bridge = new GatorBridge(binary, conversationID, goalID);
  let app: Instance | undefined;
  let handoff: ExternalCommand | undefined;
  let externalActive = false;
  const unsubscribe = bridge.subscribe((state) => {
    if (!state.external) { externalActive = false; return; }
    if (!externalActive) {
      externalActive = true;
      handoff = state.external;
      app?.unmount();
    }
  });
  try {
    while (true) {
      app = await render(<App bridge={bridge} />);
      if (handoff) app.unmount();
      await app.waitUntilExit();
      app = undefined;
      if (!handoff) break;
      const command = handoff;
      handoff = undefined;
      bridge.send({ type: "external_done", error: await runExternal(command) });
    }
  } finally {
    unsubscribe();
    bridge.stop();
  }
}

void main().catch((error) => { process.stderr.write(`gator UI: ${String(error)}\n`); process.exitCode = 1; });
