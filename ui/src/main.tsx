import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { createInterface } from "node:readline";
import { Ansi, Box, Text, render, useApp, useInput, TerminalSizeContext, type Instance } from "@claude-code-kit/ink-renderer";
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
type ExternalCommand = { kind: string; program: string; args: string[]; dir?: string; env?: string[] };
type BackendState = {
  mode: "chat" | "legacy";
  legacy_view?: string;
  messages: BackendMessage[];
  commands: BackendCommand[];
  actions: { Label: string; Command: string }[];
  title: string;
  source: string;
  provider?: string;
  model?: string;
  status?: string;
  input?: string;
  running: boolean;
  cancelling: boolean;
  elapsed?: string;
  loading_frame: number;
  queued: number;
  home: boolean;
  first_run: boolean;
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
    if (key.return) { if (value.trim()) onSubmit(value); setCursor(0); setHistoryIndex(-1); return; }
    if (key.leftArrow) { setCursor((position) => Math.max(0, position - 1)); return; }
    if (key.rightArrow) { setCursor((position) => Math.min(value.length, position + 1)); return; }
    if (key.home || (key.ctrl && input === "a")) { setCursor(0); return; }
    if (key.end || (key.ctrl && input === "e")) { setCursor(value.length); return; }
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
    <Box>
      <Text color={green}>❯ </Text>
      {value ? (
        <Text>{value.slice(0, cursor)}<Text inverse>{value[cursor] ?? " "}</Text>{value.slice(cursor + 1)}</Text>
      ) : (
        <Text><Text inverse> </Text><Text dimColor>Ask Gator to work on something…</Text></Text>
      )}
    </Box>
  );
}

function ChatKeys({
  state, draft, matches, selected, setSelected, choose, dismiss, search, scroll, exit, bridge,
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
  bridge: GatorBridge;
}): null {
  useInput((input, key, event) => {
    if (key.ctrl && input === "d") { event.stopImmediatePropagation(); exit(); return; }
    if (key.ctrl && input === "c" && !state.running) { event.stopImmediatePropagation(); exit(); return; }
    if ((key.ctrl && input === "c") || (key.escape && state.running)) {
      event.stopImmediatePropagation(); bridge.send({ type: "key", key: "esc" }); return;
    }
    if (key.ctrl && input === "f") { event.stopImmediatePropagation(); search(); return; }
    const shortcuts: Record<string, string> = { p: "ctrl+p", x: "ctrl+x", b: "ctrl+b", j: "ctrl+j", i: "ctrl+i", g: "ctrl+g" };
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
      event.stopImmediatePropagation(); choose(matches[Math.min(selected, matches.length - 1)]);
    }
  }, { captureCtrlC: true });
  return null;
}

function LegacyKeys({ bridge }: { bridge: GatorBridge }): null {
  useInput((input, key, event) => {
    event.stopImmediatePropagation();
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
    else if (key.tab) name = "tab";
    else if (key.ctrl) name = `ctrl+${input}`;
    else if (input === " ") name = "space";
    else if (!key.meta) name = input;
    if (name) bridge.send({ type: "key", key: name });
  }, { captureCtrlC: true });
  return null;
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

  if (!state) return <Box padding={1}><Text color={green}>{failure || "Starting Gator…"}</Text></Box>;
  if (state.mode === "legacy") {
    return <Box flexDirection="column"><LegacyKeys bridge={bridge} /><Ansi>{state.legacy_view ?? ""}</Ansi></Box>;
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
    if (command.needs_input) {
      setInput(`/${command.name} `);
      setPrefillVersion((version) => version + 1);
      setDismissed(true);
    } else {
      submit(`/${command.name}`);
    }
  };
  const close = () => { bridge.stop(); exit(); };
  const welcome = (
    <Box flexDirection="column">
      <WelcomeScreen appName="Gator" subtitle="Your ongoing work, ready when you are" color={green} logo={<Text color={green}>🐊</Text>} />
      <Box marginTop={1} marginLeft={1} flexDirection="column">
        <Text color={mutedGreen} bold>{state.first_run ? "Get started" : "Next up"}</Text>
        {state.first_run ? (
          <><Text>  1  /model  Choose a cloud or local model</Text><Text>  2  Ask: Summarize this folder into a daily brief with next actions.</Text></>
        ) : state.actions.slice(0, 4).map((action, index) => (
          <Text key={index}>  {action.Label} · {action.Command}</Text>
        ))}
        <Text dimColor>  Type / for Gator commands.</Text>
      </Box>
    </Box>
  );
  const messages: Message[] = state.messages.map((item, index) => ({
    id: `${index}-${item.role}-${item.text.length}`,
    role: item.role === "You" ? "user" : item.role === "Gator" ? "assistant" : "system",
    content: item.text,
  }));
  const viewportHeight = Math.max(1, (terminal?.rows ?? 24) - 6 - matches.length - (state.running ? 2 : 0) - (searchOpen ? 1 : 0));
  const statusSegments = [
    ...(state.status ? [{ content: state.status, color: mutedGreen }] : []),
    { content: state.provider && state.model ? `${state.provider}/${state.model}` : "Choose model: /model", color: green },
    { content: state.source ? state.source.split("/").filter(Boolean).at(-1) || state.source : "Gator" },
    ...(state.queued ? [{ content: `${state.queued} queued` }] : []),
  ];
  return (
    <Box flexDirection="column" flexGrow={1} paddingX={1} paddingTop={1}>
      <Box flexDirection="column" flexGrow={1}>
        {state.home && state.messages.length === 0 && <Box marginBottom={1}>{welcome}</Box>}
        <MessageList ref={list} viewportHeight={viewportHeight} messages={messages} renderMessage={(item) => <MessageBody item={state.messages[Number(item.id.split("-", 1)[0])]} />} />
        {state.running && <Box marginTop={1}><Text color={green}>{state.cancelling ? "◌ Stopping… retaining evidence" : `${["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"][state.loading_frame % 10]} Working… ${state.elapsed ?? ""} · Esc to interrupt`}</Text></Box>}
      </Box>
      {!searchOpen && <ChatKeys state={state} draft={input} matches={matches} selected={selected} setSelected={setSelected} choose={choose} dismiss={() => setDismissed(true)} search={() => setSearchOpen(true)} scroll={(rows) => list.current?.scrollBy(rows)} exit={close} bridge={bridge} />}
      {searchOpen && <GreenSearch messages={state.messages} close={() => setSearchOpen(false)} navigate={(index) => list.current?.scrollTo(index)} />}
      <Divider />
      <GreenPrompt value={input} onChange={(value) => { setInput(value); setSelected(0); setDismissed(false); }} onSubmit={submit} disabled={searchOpen || !!failure} history={history} prefillVersion={prefillVersion} />
      {matches.length > 0 && <Box marginLeft={2} flexDirection="column">{matches.map((command, index) => <Box key={command.name}><Text color={index === selected ? green : undefined} bold={index === selected}>{index === selected ? "❯" : " "} /{command.name}</Text><Text dimColor>  {command.description}</Text></Box>)}</Box>}
      {failure && <Text color="red">{failure}</Text>}
      <Divider />
      <StatusLine segments={statusSegments} />
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
