import { Box, Text, render, useApp } from "@claude-code-kit/ink-renderer";
import { REPL, WelcomeScreen, type Message } from "@claude-code-kit/ui";
import React, { useCallback, useMemo, useState } from "react";

const green = "#4BAA72";
const mutedGreen = "#86BE9A";
const sampleGoal = "Prepare the October launch brief";

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
      <REPL
        messages={messages}
        onSubmit={onSubmit}
        onExit={exit}
        commands={commands}
        welcome={welcome}
        renderMessage={(item) => <PreviewMessage item={item} />}
        prefix="❯"
        placeholder="Ask Gator to work on something…"
        statusSegments={[
          { content: "UI preview", color: green },
          { content: "no model calls" },
          { content: "Gator · local" },
        ]}
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
