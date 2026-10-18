import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { createInterface } from "node:readline";
import { Box, Text, render, useApp, useInput, TerminalSizeContext, type Instance } from "@claude-code-kit/ink-renderer";
import { Divider, MessageList, StatusLine, WelcomeScreen, type Message, type VirtualListHandle } from "@claude-code-kit/ui";
import React, { useContext, useEffect, useRef, useState } from "react";

const green = "#4BAA72";
const mutedGreen = "#86BE9A";

type BackendCommand = { name: string; description: string; advanced: boolean; needs_input: boolean };
type BackendBundle = {
  Path: string;
  Status: string;
  Artifacts: { Path: string; MediaType: string; Valid: boolean }[] | null;
  Candidates: { ID: string; Status: string; ChangedPaths: string[] }[] | null;
};
type BackendMessage = { role: string; text: string; bundle?: BackendBundle };
type BackendPanel = {
  kind: string;
  title: string;
  subtitle?: string;
  query?: string;
  rows?: { label: string; detail?: string; state?: string }[];
  blocks?: { title?: string; lines: string[] }[];
  selected?: number;
  notice?: string;
  footer?: string;
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

function formatBundle(bundle: BackendBundle): string {
  const lines = [`Verified deliverables · ${bundle.Status}`];
  for (const item of bundle.Artifacts ?? []) lines.push(`${item.Valid ? "✓" : "!"} ${item.Path} · ${item.MediaType}`);
  for (const item of bundle.Candidates ?? []) lines.push(`${item.Status} change ${item.ID}: ${item.ChangedPaths.join(", ")}`);
  if (lines.length === 1) lines.push(bundle.Path);
  return lines.join("\n");
}

function MessageBody({ item }: { item: BackendMessage }): React.ReactNode {
  const isUser = item.role === "You";
  const isGator = item.role === "Gator";
  const label = isUser ? "› You" : isGator ? "🐊 Gator" : item.role === "Deliverables" ? "◈ Deliverables" : `◌ ${item.role}`;
  const content = item.bundle ? formatBundle(item.bundle) : item.text;
  return (
    <Box flexDirection="column">
      <Text color={isGator ? green : isUser ? undefined : mutedGreen} bold={!isUser} dimColor={isUser}>{label}</Text>
      {content.split("\n").map((line, index) => (
        <Box marginLeft={2} key={index}><Text dimColor={!isGator && !isUser}>{line || " "}</Text></Box>
      ))}
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
  const [cursor, setCursor] = useState(value.length);
  const [historyIndex, setHistoryIndex] = useState(-1);
  useEffect(() => { setCursor(value.length); }, [prefillVersion]);
  const replace = (next: string, position: number) => {
    onChange(next);
    setCursor(position);
    setHistoryIndex(-1);
  };
  useInput((input, key) => {
    if (disabled) return;
    if ((key.return && key.shift) || (key.ctrl && input === "j")) { replace(value.slice(0, cursor) + "\n" + value.slice(cursor), cursor + 1); return; }
    if (key.return) { if (value.trim()) onSubmit(value); setCursor(0); setHistoryIndex(-1); return; }
    if (key.leftArrow) { setCursor((position) => Math.max(0, position - 1)); return; }
    if (key.rightArrow) { setCursor((position) => Math.min(value.length, position + 1)); return; }
    if (key.home || (key.ctrl && input === "a")) { setCursor(0); return; }
    if (key.end || (key.ctrl && input === "e")) { setCursor(value.length); return; }
    const beforeCursor = value.slice(0, cursor);
    const currentLine = beforeCursor.split("\n").length - 1;
    const lines = value.split("\n");
    const column = beforeCursor.length - beforeCursor.lastIndexOf("\n") - 1;
    if (key.upArrow && currentLine > 0) {
      const start = lines.slice(0, currentLine - 1).reduce((count, line) => count + line.length + 1, 0);
      setCursor(start + Math.min(column, lines[currentLine - 1].length)); return;
    }
    if (key.downArrow && currentLine + 1 < lines.length) {
      const start = lines.slice(0, currentLine + 1).reduce((count, line) => count + line.length + 1, 0);
      setCursor(start + Math.min(column, lines[currentLine + 1].length)); return;
    }
    if (key.upArrow && historyIndex + 1 < history.length) {
      const next = historyIndex + 1; setHistoryIndex(next); onChange(history[next]); setCursor(history[next].length); return;
    }
    if (key.downArrow && historyIndex >= 0) {
      const next = historyIndex - 1; setHistoryIndex(next); onChange(next < 0 ? "" : history[next]); setCursor(next < 0 ? 0 : history[next].length); return;
    }
    if (key.backspace) { if (cursor > 0) replace(value.slice(0, cursor - 1) + value.slice(cursor), cursor - 1); return; }
    if (key.delete) { if (cursor < value.length) replace(value.slice(0, cursor) + value.slice(cursor + 1), cursor); return; }
    if (key.ctrl && input === "u") { replace(value.slice(cursor), 0); return; }
    if (key.ctrl && input === "w") {
      const before = value.slice(0, cursor);
      const keep = before.replace(/\s*\S+\s*$/, "");
      replace(keep + value.slice(cursor), keep.length);
      return;
    }
    if (key.ctrl || key.meta || key.tab || key.escape) return;
    if (input) replace(value.slice(0, cursor) + input + value.slice(cursor), cursor + input.length);
  }, { isActive: !disabled });
  return (
    <Box flexDirection="column">
      {value ? value.split("\n").map((line, index, lines) => {
        const start = lines.slice(0, index).reduce((count, item) => count + item.length + 1, 0);
        const localCursor = cursor - start;
        return <Box key={index}>
          <Text color={green}>{index === 0 ? "❯ " : "· "}</Text>
          {localCursor >= 0 && localCursor <= line.length
            ? <Text>{line.slice(0, localCursor)}<Text inverse>{line[localCursor] ?? " "}</Text>{line.slice(localCursor + 1)}</Text>
            : <Text>{line || " "}</Text>}
        </Box>;
      }) : <Box><Text color={green}>❯ </Text><Text><Text inverse> </Text><Text dimColor>Ask Gator to work on something…</Text></Text></Box>}
    </Box>
  );
}

function ChatKeys({
  state, draft, matches, selected, setSelected, choose, dismiss, search, scroll, exit, clearDraft, bridge,
}: {
  state: BackendState;
  draft: string;
  matches: BackendCommand[];
  selected: number;
  setSelected: React.Dispatch<React.SetStateAction<number>>;
  choose: (command: BackendCommand) => void;
  dismiss: () => void;
  search: () => void;
  scroll: (rows: number) => void;
  exit: () => void;
  clearDraft: () => void;
  bridge: GatorBridge;
}): null {
  useInput((input, key, event) => {
    if (key.ctrl && input === "d") { event.stopImmediatePropagation(); exit(); return; }
    if (key.ctrl && input === "c" && !state.running) { event.stopImmediatePropagation(); clearDraft(); return; }
    if ((key.ctrl && input === "c") || (key.escape && state.running)) {
      event.stopImmediatePropagation(); bridge.send({ type: "key", key: "esc" }); return;
    }
    if (key.ctrl && input === "f") { event.stopImmediatePropagation(); search(); return; }
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
      setSelected((index) => (index + (key.upArrow ? -1 : 1) + matches.length) % matches.length);
    } else if (key.return || key.tab) {
      event.stopImmediatePropagation();
      const exact = matches.find((command) => `/${command.name}` === draft.trim());
      choose(exact ?? matches[Math.min(selected, matches.length - 1)]);
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
  const [offset, setOffset] = useState(0);
  useEffect(() => { setOffset(0); }, [panel.kind, panel.title, panel.subtitle]);
  const rows = panel.rows ?? [];
  const blocks = (panel.blocks ?? []).flatMap((block) => [
    ...(block.title ? [{ text: block.title, heading: true }] : []),
    ...(block.lines.length ? block.lines.map((line) => ({ text: line, heading: false })) : [{ text: "None", heading: false }]),
    { text: "", heading: false },
  ]);
  const focused = panel.kind === "confirmation" || panel.kind === "model-form" || panel.kind === "learning-form";
  const available = Math.max(4, (terminal?.rows ?? 24) - (focused ? 14 : 11));
  const rowHeight = rows.some((row) => row.detail) ? 3 : 2;
  const maxRows = Math.max(2, Math.floor((blocks.length > 0 ? available / 2 : available) / rowHeight));
  const selected = Math.min(Math.max(0, panel.selected ?? 0), Math.max(0, rows.length - 1));
  const rowStart = Math.max(0, Math.min(selected - Math.floor(maxRows / 2), rows.length - maxRows));
  const maxLines = Math.max(2, available - (rows.length > 0 ? Math.min(rows.length, maxRows) * rowHeight : 0));
  const blockOffset = Math.min(offset, Math.max(0, blocks.length - maxLines));
  const panelWidth = Math.max(20, Math.min(78, (terminal?.columns ?? 80) - 4));
  const contentWidth = focused ? panelWidth - 4 : Math.max(1, (terminal?.columns ?? 80) - 4);
  return (
    <Box flexDirection="column" flexGrow={1} paddingX={2} paddingTop={1}>
      <PanelKeys bridge={bridge} exit={exit} scroll={(amount) => setOffset((current) => Math.max(0, Math.min(blocks.length - maxLines, current + amount)))} />
      <Box flexDirection="column" flexGrow={focused ? 0 : 1} width={focused ? panelWidth : undefined} alignSelf={focused ? "center" : undefined} borderStyle={focused ? "round" : undefined} borderColor={focused ? green : undefined} paddingX={focused ? 1 : 0} marginTop={focused ? 1 : 0}>
        <Text color={green} bold>🐊 Gator  <Text color={mutedGreen}>{panel.title}</Text></Text>
        {panel.subtitle && <Box marginTop={1}><Text>{panel.subtitle}</Text></Box>}
        {panel.kind === "picker" && <Box marginTop={1}><Text color={mutedGreen}>Search  </Text><Text>{panel.query || "type to filter"}<Text inverse> </Text></Text></Box>}
        <Box marginTop={1}><Divider width={contentWidth} /></Box>
        <Box flexDirection="column" flexGrow={focused ? 0 : 1} marginTop={1}>
        {rows.slice(rowStart, rowStart + maxRows).map((row, index) => {
          const active = panel.kind !== "inbox" && panel.kind !== "jobs" && rowStart + index === selected;
          return <Box key={`${rowStart + index}-${row.label}`} flexDirection="column" marginBottom={1}>
            <Text color={active ? green : undefined} bold={active}>{active ? "❯ " : "  "}{row.label}{row.state ? <Text color={mutedGreen}>  ·  {row.state}</Text> : null}</Text>
            {row.detail && <Box marginLeft={2}><Text dimColor={!active}>{row.detail}{active && (panel.kind === "model-form" || panel.kind === "learning-form") && <Text inverse> </Text>}</Text></Box>}
          </Box>;
        })}
        {blocks.slice(blockOffset, blockOffset + maxLines).map((line, index) => <Box key={`${blockOffset + index}-${line.text}`} marginBottom={line.heading ? 0 : undefined}>
          <Text color={line.heading ? mutedGreen : undefined} bold={line.heading}>{line.text || " "}</Text>
        </Box>)}
        {blocks.length > maxLines && <Text dimColor>{`  ${blockOffset + 1}–${Math.min(blockOffset + maxLines, blocks.length)} of ${blocks.length} lines · PgUp/PgDn scroll`}</Text>}
        {rows.length === 0 && blocks.length === 0 && !panel.notice && <Text dimColor>No items to show.</Text>}
        {panel.notice && <Box marginTop={1}><Text color={mutedGreen}>{panel.notice}</Text></Box>}
        {status && status !== panel.notice && <Box marginTop={1}><Text dimColor>{status}</Text></Box>}
        </Box>
        <Divider width={contentWidth} />
        <Text dimColor>{panel.footer || "Esc back"}</Text>
      </Box>
      {focused && <Box flexGrow={1} />}
      <Box marginTop={1}><Divider width={Math.max(1, (terminal?.columns ?? 80) - 4)} /></Box>
      <Text dimColor>❯ {draft || "Ask Gator to work on something…"}</Text>
      {statusLine && statusLine.length > 0 && <StatusLine segments={statusLine.map((content) => ({ content, color: mutedGreen }))} />}
    </Box>
  );
}

function GreenSearch({ messages, close, navigate }: { messages: BackendMessage[]; close: () => void; navigate: (index: number) => void }): React.ReactNode {
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<number[]>([]);
  const [selected, setSelected] = useState(0);
  const change = (value: string) => {
    const found = messages.flatMap((item, index) => (item.text.toLowerCase().includes(value.toLowerCase()) && value ? [index] : []));
    setQuery(value); setMatches(found); setSelected(0);
    if (found.length) navigate(found[0]);
  };
  useInput((input, key, event) => {
    event.stopImmediatePropagation();
    if (key.escape) close();
    else if (key.backspace) change(query.slice(0, -1));
    else if ((key.return || (key.ctrl && input === "n")) && matches.length) {
      const next = (selected + 1) % matches.length; setSelected(next); navigate(matches[next]);
    } else if (!key.ctrl && !key.meta && input) change(query + input);
  });
  return <Box paddingX={1}><Text color={green}>Search: </Text><Text>{query}<Text inverse> </Text></Text><Text dimColor>{`  ${matches.length ? `${selected + 1}/${matches.length} matches` : "no matches"}`}</Text></Box>;
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
  const list = useRef<VirtualListHandle>(null);
  const lastBackendInput = useRef("");

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
      setInput(state.input);
      setPrefillVersion((version) => version + 1);
    }
    lastBackendInput.current = state?.input ?? "";
  }, [state?.mode, state?.input]);

  const close = () => { bridge.stop(); exit(); };
  if (!state) return <Box padding={1}><Text color={green}>{failure || "Starting Gator…"}</Text></Box>;
  if (state.mode === "panel" && state.panel) {
    return <PanelScreen panel={state.panel} status={state.status} draft={input} statusLine={state.status_line_enabled ? state.status_line : undefined} bridge={bridge} exit={close} />;
  }
  const matches = !searchOpen && !dismissed && input.startsWith("/") && !input.includes(" ")
    ? state.commands.filter((command) => (input === "/" ? !command.advanced : `/${command.name}`.startsWith(input))).slice(0, 8)
    : [];
  const submit = (value: string) => {
    const text = value.trim();
    if (!text) return;
    bridge.send({ type: "submit", text });
    setInput(""); setDismissed(false); setHistory((current) => [text, ...current]);
  };
  const choose = (command: BackendCommand) => {
    if (command.needs_input && input.trim() !== `/${command.name}`) {
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
      <WelcomeScreen appName="Gator" subtitle="Your ongoing work, ready when you are" color={green} logo={<Text color={green}>🐊</Text>} />
      <Text dimColor>{context}</Text>
      <Box marginTop={1} marginLeft={1} flexDirection="column">
        <Text color={mutedGreen} bold>{state.first_run ? "Get started" : "Next up"}</Text>
        {state.first_run ? (
          <><Text>  1  /model  Choose a cloud or local model</Text><Text>  2  Ask: Summarize this folder into a daily brief with next actions.</Text></>
        ) : state.actions.slice(0, 4).map((action, index) => (
          <Text key={index}>  {action.Label} · {action.Command}</Text>
        ))}
        <Text dimColor>  Type / for Gator commands.</Text>
        {state.supervisor_needs_start && <Text color={mutedGreen}>  Recurring checks need a running supervisor · gator job supervisor</Text>}
      </Box>
    </Box>
  );
  const messages: Message[] = state.messages.map((item, index) => ({
    id: `${index}-${item.role}-${item.text.length}`,
    role: item.role === "You" ? "user" : item.role === "Gator" ? "assistant" : "system",
    content: item.text,
  }));
  const viewportHeight = Math.max(1, (terminal?.rows ?? 24) - 6 - matches.length - (state.running ? 2 : 0) - (searchOpen ? 1 : 0) - input.split("\n").length + 1);
  const statusSegments = (state.status_line ?? []).map((content) => ({ content, color: mutedGreen }));
  return (
    <Box flexDirection="column" flexGrow={1} paddingX={1} paddingTop={1}>
      {!state.home && <Box marginBottom={1}><Text color={green} bold>🐊 Gator</Text><Text color={mutedGreen}>  {context}</Text></Box>}
      <Box flexDirection="column" flexGrow={1}>
        {state.home && state.messages.length === 0 && <Box marginBottom={1}>{welcome}</Box>}
        {state.messages.length > 0 && <MessageList ref={list} viewportHeight={viewportHeight} messages={messages} renderMessage={(item) => <MessageBody item={state.messages[Number(item.id.split("-", 1)[0])]} />} />}
        {state.running && <Box marginTop={1}><Text color={green}>{state.cancelling ? "◌ Stopping… retaining evidence" : `${["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"][state.loading_frame % 10]} Working… ${state.elapsed ?? ""} · Esc to interrupt`}</Text></Box>}
      </Box>
      {state.status && <Text color={mutedGreen}>{state.status}</Text>}
      {!searchOpen && <ChatKeys state={state} draft={input} matches={matches} selected={selected} setSelected={setSelected} choose={choose} dismiss={() => setDismissed(true)} search={() => setSearchOpen(true)} scroll={(rows) => list.current?.scrollBy(rows)} exit={close} clearDraft={() => { setInput(""); setPrefillVersion((version) => version + 1); }} bridge={bridge} />}
      {searchOpen && <GreenSearch messages={state.messages} close={() => setSearchOpen(false)} navigate={(index) => list.current?.scrollTo(index)} />}
      <Divider />
      <GreenPrompt value={input} onChange={(value) => { setInput(value); setSelected(0); setDismissed(false); }} onSubmit={submit} disabled={searchOpen || !!failure} history={history} prefillVersion={prefillVersion} />
      {matches.length > 0 && <Box marginLeft={2} flexDirection="column">{matches.map((command, index) => <Box key={command.name}><Text color={index === selected ? green : undefined} bold={index === selected}>{index === selected ? "❯" : " "} /{command.name}</Text><Text dimColor>  {command.description}</Text></Box>)}</Box>}
      {failure && <Text color="red">{failure}</Text>}
      <Divider />
      {state.status_line_enabled && statusSegments.length > 0 && <StatusLine segments={statusSegments} />}
    </Box>
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
  const [binary, conversationID = "", goalID = ""] = process.argv.slice(2);
  if (!binary) throw new Error("Gator UI requires its backend executable");
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
