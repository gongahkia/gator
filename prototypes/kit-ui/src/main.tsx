import { Box, Text, render, useApp, useInput, TerminalSizeContext } from "@claude-code-kit/ink-renderer";
import { Divider, MessageList, PromptInput, StatusLine, WelcomeScreen, type Message, type SearchMatch, type VirtualListHandle } from "@claude-code-kit/ui";
import React, { useCallback, useContext, useMemo, useRef, useState } from "react";

const green = "#4BAA72";
const mutedGreen = "#86BE9A";
const sampleGoal = "Prepare the October launch brief";

type PreviewCommand = {
  name: string;
  description: string;
  onExecute: (args: string) => void;
};

function message(id: string, role: Message["role"], content: string): Message {
  return { id, role, content };
}

function briefScene(): Message[] {
  return [
    message("brief-user", "user", "Prepare my daily brief from the launch notes and latest updates."),
    message(
      "brief-gator",
      "assistant",
      [
        "Daily brief · preview data",
        "",
        "Launch: the draft deck is ready for review; two source claims still need confirmation.",
        "Follow-up: the partner reply is due today. An email draft is ready in your inbox.",
        "Next action: review the deck claims, then approve or edit the email draft.",
        "",
        "Sources shown in the real product would link to the files or pages used.",
      ].join("\n"),
    ),
  ];
}

function PreviewMessage({ item }: { item: Message }): React.ReactNode {
  const isUser = item.role === "user";
  const label = isUser ? "› You" : item.role === "assistant" ? "● Gator" : "◌ Preview";
  const color = isUser ? undefined : item.role === "assistant" ? green : mutedGreen;
  const content = typeof item.content === "string" ? item.content : "";

  return (
    <Box flexDirection="column">
      <Text bold={!isUser} color={color} dimColor={isUser}>
        {label}
      </Text>
      {content.split("\n").map((line, index) => (
        <Box key={`${item.id}-${index}`} marginLeft={2}>
          <Text dimColor={item.role === "system"}>{line || " "}</Text>
        </Box>
      ))}
    </Box>
  );
}

// The kit's REPL does not pass PromptInput's prefixColor through, and its
// suggestion highlight is fixed to cyan. Compose its public parts so the
// preview can use Gator's accent without patching node_modules.
function CommandKeys({
  matches,
  selected,
  setSelected,
  onChoose,
  onDismiss,
  onExit,
  onSearch,
  onScroll,
  searchOpen,
}: {
  matches: PreviewCommand[];
  selected: number;
  setSelected: React.Dispatch<React.SetStateAction<number>>;
  onChoose: (name: string) => void;
  onDismiss: () => void;
  onExit: () => void;
  onSearch: () => void;
  onScroll: (rows: number) => void;
  searchOpen: boolean;
}): null {
  useInput((input, key, event) => {
    if (searchOpen) return;
    if (key.ctrl && input === "d") {
      event.stopImmediatePropagation();
      onExit();
      return;
    }
    if (key.ctrl && input === "f") {
      event.stopImmediatePropagation();
      onSearch();
      return;
    }
    if (key.pageUp || key.pageDown || key.wheelUp || key.wheelDown) {
      event.stopImmediatePropagation();
      onScroll(key.pageUp ? -12 : key.pageDown ? 12 : key.wheelUp ? -3 : 3);
      return;
    }
    if (matches.length === 0) return;
    if (key.escape) {
      event.stopImmediatePropagation();
      onDismiss();
    } else if (key.upArrow || key.downArrow) {
      event.stopImmediatePropagation();
      setSelected((index) => (index + (key.upArrow ? -1 : 1) + matches.length) % matches.length);
    } else if (key.return || key.tab) {
      event.stopImmediatePropagation();
      onChoose(matches[Math.min(selected, matches.length - 1)].name);
    }
  });
  return null;
}

function GreenSearch({
  onClose,
  onSearch,
  onNavigate,
}: {
  onClose: () => void;
  onSearch: (query: string) => SearchMatch[];
  onNavigate: (match: SearchMatch) => void;
}): React.ReactNode {
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const [matches, setMatches] = useState<SearchMatch[]>([]);
  const [selected, setSelected] = useState(0);
  const updateQuery = (value: string, position: number) => {
    const found = onSearch(value);
    setQuery(value);
    setCursor(position);
    setMatches(found);
    setSelected(0);
    if (found.length > 0) onNavigate(found[0]);
  };
  const navigate = (direction: number) => {
    if (matches.length === 0) return;
    const next = (selected + direction + matches.length) % matches.length;
    setSelected(next);
    onNavigate(matches[next]);
  };
  useInput((input, key, event) => {
    event.stopImmediatePropagation();
    if (key.escape) onClose();
    else if (key.return || (key.ctrl && input === "n")) navigate(1);
    else if (key.ctrl && input === "p") navigate(-1);
    else if (key.backspace && cursor > 0) updateQuery(query.slice(0, cursor - 1) + query.slice(cursor), cursor - 1);
    else if (key.leftArrow) setCursor((position) => Math.max(0, position - 1));
    else if (key.rightArrow) setCursor((position) => Math.min(query.length, position + 1));
    else if (!key.ctrl && !key.meta && input.length > 0) updateQuery(query.slice(0, cursor) + input + query.slice(cursor), cursor + input.length);
  });
  return (
    <Box paddingX={1}>
      <Text color={green}>Search: </Text>
      <Text>
        {query.slice(0, cursor)}
        <Text inverse>{query[cursor] ?? " "}</Text>
        {query.slice(cursor + 1)}
      </Text>
      <Text dimColor>{`  ${matches.length === 0 ? "no matches" : `${selected + 1}/${matches.length} matches`}`}</Text>
    </Box>
  );
}

function GatorPreviewREPL({
  messages,
  commands,
  welcome,
  onSubmit,
  onExit,
}: {
  messages: Message[];
  commands: PreviewCommand[];
  welcome: React.ReactNode;
  onSubmit: (text: string) => void;
  onExit: () => void;
}): React.ReactNode {
  const [inputValue, setInputValue] = useState("");
  const [history, setHistory] = useState<string[]>([]);
  const [selected, setSelected] = useState(0);
  const [pickerDismissed, setPickerDismissed] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const historyRef = useRef<VirtualListHandle>(null);
  const terminal = useContext(TerminalSizeContext);
  const matches = !pickerDismissed && inputValue.startsWith("/") && !inputValue.includes(" ")
    ? commands.filter((command) => `/${command.name}`.startsWith(inputValue))
    : [];
  const viewportHeight = Math.max(1, (terminal?.rows ?? 24) - 6 - matches.length - (searchOpen ? 1 : 0));

  const changeInput = (value: string) => {
    setInputValue(value);
    setSelected(0);
    setPickerDismissed(false);
  };
  const submit = (value: string) => {
    const trimmed = value.trim();
    if (!trimmed) return;
    setInputValue("");
    setPickerDismissed(false);
    setHistory((current) => [trimmed, ...current]);
    if (trimmed.startsWith("/")) {
      const [name, ...args] = trimmed.slice(1).split(/\s+/);
      const command = commands.find((item) => item.name === name);
      if (command) {
        command.onExecute(args.join(" "));
        return;
      }
    }
    onSubmit(trimmed);
  };
  const search = (query: string): SearchMatch[] => {
    if (!query) return [];
    const found: SearchMatch[] = [];
    messages.forEach((item, index) => {
      const content = typeof item.content === "string" ? item.content : "";
      let offset = content.toLowerCase().indexOf(query.toLowerCase());
      while (offset >= 0) {
        found.push({ index, offset, length: query.length });
        offset = content.toLowerCase().indexOf(query.toLowerCase(), offset + 1);
      }
    });
    return found;
  };

  return (
    <Box flexDirection="column" flexGrow={1}>
      <Box flexDirection="column" flexGrow={1}>
        {messages.length === 0 && <Box marginBottom={1}>{welcome}</Box>}
        <MessageList ref={historyRef} viewportHeight={viewportHeight} messages={messages} renderMessage={(item) => <PreviewMessage item={item} />} />
      </Box>
      <CommandKeys
        matches={matches}
        selected={selected}
        setSelected={setSelected}
        onChoose={(name) => submit(`/${name}`)}
        onDismiss={() => setPickerDismissed(true)}
        onExit={onExit}
        onSearch={() => setSearchOpen(true)}
        onScroll={(rows) => historyRef.current?.scrollBy(rows)}
        searchOpen={searchOpen}
      />
      {searchOpen && <GreenSearch onClose={() => setSearchOpen(false)} onSearch={search} onNavigate={(match) => historyRef.current?.scrollTo(match.index)} />}
      <Divider />
      <PromptInput
        value={inputValue}
        onChange={changeInput}
        onSubmit={submit}
        placeholder="Ask Gator to work on something…"
        prefix="❯"
        prefixColor={green}
        disabled={searchOpen}
        history={history}
      />
      {matches.length > 0 && (
        <Box flexDirection="column" marginLeft={2}>
          {matches.map((command, index) => (
            <Box key={command.name}>
              <Text color={index === selected ? green : undefined} bold={index === selected}>
                {index === selected ? "❯" : " "} /{command.name}
              </Text>
              <Text dimColor>  {command.description}</Text>
            </Box>
          ))}
        </Box>
      )}
      <Divider />
      <StatusLine segments={[
        { content: "UI preview", color: green },
        { content: "no model calls" },
        { content: "Gator · local" },
      ]} />
    </Box>
  );
}

function App({ initialScene }: { initialScene: "home" | "brief" }): React.ReactNode {
  const { exit } = useApp();
  const [messages, setMessages] = useState<Message[]>(() =>
    initialScene === "brief" ? briefScene() : [],
  );

  const append = useCallback((content: string) => {
    setMessages((current) => [
      ...current,
      message(`preview-${Date.now()}-${current.length}`, "system", content),
    ]);
  }, []);

  const commands = useMemo(
    () => [
      {
        name: "goals",
        description: "Resume an ongoing goal",
        onExecute: () => append(`Goals · preview data\n1. ${sampleGoal}\nNext: review two claims in the deck.`),
      },
      {
        name: "inbox",
        description: "Review follow-up results",
        onExecute: () => append("Inbox · preview data\n1. Email draft: reply to partner\n2. Review needed: launch deck claims"),
      },
      {
        name: "memory",
        description: "Manage saved knowledge",
        onExecute: () => append("Memory · preview data\nLaunch brief preference: concise, with source links and next actions."),
      },
      {
        name: "model",
        description: "Choose a cloud or local model",
        onExecute: () => append("Model selection is part of Gator's live CLI. This preview has no model connection."),
      },
      {
        name: "source",
        description: "Change the workspace",
        onExecute: () => append("Workspace selection is part of Gator's live CLI. This preview reads no files."),
      },
      {
        name: "help",
        description: "Show shortcuts and common commands",
        onExecute: () => append("Type / for Gator commands. Ctrl+F searches the conversation. Ctrl+D exits this preview."),
      },
      {
        name: "new",
        description: "Start a new conversation",
        onExecute: () => setMessages([]),
      },
      { name: "exit", description: "Leave Gator", onExecute: () => exit() },
    ],
    [append, exit],
  );

  const onSubmit = useCallback((text: string) => {
    if (text.startsWith("/")) {
      append(`Unknown preview command: ${text.split(" ", 1)[0]}. Type /help for the available Gator commands.`);
      return;
    }
    setMessages((current) => [
      ...current,
      message(`user-${Date.now()}`, "user", text),
      message(
        `notice-${Date.now()}`,
        "assistant",
        "This is a visual preview. The Gator agent is not connected here. Run the Go CLI for real work.",
      ),
    ]);
  }, [append]);

  const welcome = (
    <Box flexDirection="column">
      <WelcomeScreen
        appName="Gator"
        subtitle="Your ongoing work, ready when you are"
        color={green}
        logo={<Text color={green}>{"🐊"}</Text>}
      />
      <Box flexDirection="column" marginTop={1} marginLeft={1}>
        <Text bold color={mutedGreen}>Today · preview data</Text>
        <Text>{`  ● ${sampleGoal}`}</Text>
        <Text dimColor>    Deck claims need review · partner reply drafted</Text>
        <Text dimColor>  2 inbox results ready for your decision</Text>
      </Box>
      <Box marginTop={1} marginLeft={1}>
        <Text dimColor>Try a request, or type / to browse Gator commands.</Text>
      </Box>
    </Box>
  );

  return (
    <Box flexDirection="column" flexGrow={1} paddingX={1} paddingTop={1}>
      <GatorPreviewREPL
        messages={messages}
        onSubmit={onSubmit}
        onExit={exit}
        commands={commands}
        welcome={welcome}
      />
    </Box>
  );
}

const sceneArgument = process.argv.find((value) => value.startsWith("--scene="));
const scene = sceneArgument?.slice("--scene=".length) ?? "home";
if (scene !== "home" && scene !== "brief") {
  process.stderr.write("Use --scene=home or --scene=brief.\n");
  process.exit(2);
}

await render(<App initialScene={scene} />);
