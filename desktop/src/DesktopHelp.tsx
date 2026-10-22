import { ArrowUpRight, ChevronDown, Keyboard } from "lucide-react";
import type { BackendCommand } from "./protocol";
import { CopyButton } from "./CopyButton";

const pages = [
  { name: "overview", label: "Overview", route: "/overview" },
  { name: "goals", label: "Ongoing goals", route: "/goals" },
  { name: "followup", label: "Follow-up checks", route: "/followup" },
  { name: "context", label: "Saved knowledge", route: "/knowledge" },
  { name: "model", label: "Models", route: "/model" },
  { name: "usage", label: "Usage & limits", route: "/usage" },
  { name: "data", label: "Data & backups", route: "/data" },
  { name: "recovery", label: "Recovery", route: "/recovery" },
  { name: "memory", label: "Memory & retrieval", route: "/memory" },
  { name: "learnings", label: "Saved guidance", route: "/learnings" },
];

export function DesktopHelp({ commands, navigate, disabled }: { commands: BackendCommand[]; navigate(route: string): void; disabled: boolean }) {
  const modifier = window.gator?.platform === "darwin" ? "⌘" : "Ctrl+";
  const shortcuts = [[`${modifier}K`, "Find a command"], [`${modifier}F`, "Find in conversation"], [`${modifier},`, "Open settings"], ["Enter", "Send message"], ["Shift+Enter", "Add a line"], [`${modifier}Enter`, "Review a form or guide current work"]];
  return <section className="desktop-help" aria-label="Gator desktop help"><header><h3>Using Gator</h3><p>Write a request to get started. Open a page to manage ongoing work, saved knowledge, or settings.</p></header>
    <div className="help-pages">{pages.filter((page) => commands.some((command) => command.name === page.name)).map((page) => <button key={page.name} disabled={disabled} onClick={() => navigate(page.route)}><span>{page.label}</span><ArrowUpRight size={15} /></button>)}</div>
    <section className="help-shortcuts"><h4><Keyboard size={16} />Desktop shortcuts</h4><dl>{shortcuts.map(([key, label]) => <div key={key}><dt>{label}</dt><dd><kbd>{key}</kbd></dd></div>)}</dl></section>
    <details className="help-commands"><summary>All available commands<ChevronDown size={16} /></summary><p>Type a slash command in your conversation, or find it in command search. Commands that need input let you review the values before sending.</p><div>{commands.filter((command) => command.name !== "help").map((command) => <div className="help-command" key={command.name}><div><code>/{command.name}</code><p>{command.description}</p></div><CopyButton text={`/${command.name}${command.needs_input ? " " : ""}`} label={`Copy /${command.name} command`} compact /></div>)}</div></details>
  </section>;
}

export function ShortcutReference({ lines }: { lines: string[] }) {
  const modifier = window.gator?.platform === "darwin" ? "⌘" : "Ctrl+";
  const pairs = lines.flatMap((line) => {
    const segments = line.split(" · ");
    if (segments.length === 2 && !segments[0].includes(" ")) return [[segments[0], segments[1]]];
    return segments.map((segment) => { const split = segment.indexOf(" "); return split < 0 ? [segment, ""] : [segment.slice(0, split), segment.slice(split + 1)]; });
  });
  return <dl className="shortcut-reference">{pairs.map(([key, label], index) => <div key={index}><dt>{label ? label[0].toLocaleUpperCase() + label.slice(1) : "Shortcut"}</dt><dd><kbd>{key.toLowerCase() === "ctrl+r" ? `${modifier}Enter` : key}</kbd></dd></div>)}</dl>;
}
