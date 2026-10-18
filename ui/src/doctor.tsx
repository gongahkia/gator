import { spawn } from "node:child_process";
import { Box, Text, render, type Instance } from "@claude-code-kit/ink-renderer";
import { Divider } from "@claude-code-kit/ui";
import React from "react";

const green = "#4BAA72";
const mutedGreen = "#86BE9A";
const amber = "#D9AD6C";
const red = "#E08484";
const spinner = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];

function lineColor(value: string): string | undefined {
  const text = value.toLowerCase();
  if (/missing \(required|not detected|expired|unsupported/.test(text)) return red;
  if (/missing|not configured|disabled|unavailable/.test(text)) return amber;
  if (/detected|installed|stored|enabled|configured|not required|set in environment|seatbelt|bubblewrap/.test(text)) return green;
  return undefined;
}

function heading(line: string): string | undefined {
  if (line === "Dependencies:") return "🔧 Dependencies";
  if (line === "Reviewed local-model eligibility (Gator guardrail):") return "🧠 Reviewed local models";
  return undefined;
}

function plainDoctorReport(raw: string): string {
  const lines = raw.trimEnd().split("\n").flatMap((line) => {
    const title = heading(line);
    if (title) return ["", title];
    if (line.startsWith("Local model host:")) return ["", `💻 ${line}`];
    if (line.startsWith("Suggested verification:")) return ["", `🧪 ${line}`];
    if (line.startsWith("  ") && !line.startsWith("    ")) {
      const mark = /: (installed|enabled)/.test(line) ? "✓" : /: (missing|disabled|unsupported)/.test(line) ? "○" : " ";
      return [`  ${mark} ${line.trimStart()}`];
    }
    return [line];
  });
  return `🐊 Gator doctor\n${lines.join("\n")}\n`;
}

export function DoctorReport({ raw }: { raw: string }): React.ReactNode {
  return (
    <Box flexDirection="column">
      {raw.trimEnd().split("\n").map((line, index) => {
        const title = heading(line);
        if (title) return <Box key={index} marginTop={1}><Text color={mutedGreen} bold>{title}</Text></Box>;
        if (line.startsWith("    ")) return <Box key={index} marginLeft={2}><Text dimColor>{line.trim()}</Text></Box>;
        if (line.startsWith("Local-model advice:")) return <Box key={index} marginTop={1}><Text dimColor>{line}</Text></Box>;
        const [label, value] = line.split(/: (.*)/s, 2);
        if (value === undefined) return <Text key={index}>{line}</Text>;
        const indented = label.startsWith("  ");
        const itemLabel = label.trimStart();
        const status = lineColor(value);
        const mark = indented && /^(installed|enabled)/.test(value) ? "✓ " : indented && /^(missing|disabled|unsupported)/.test(value) ? "○ " : "";
        const group = line.startsWith("Local model host:") || line.startsWith("Suggested verification:");
        return <Box key={index} marginTop={group ? 1 : undefined} marginLeft={indented ? 2 : 0}>
          <Text><Text color={indented ? undefined : mutedGreen}>{mark}{itemLabel}: </Text><Text color={status}>{value}</Text></Text>
        </Box>;
      })}
    </Box>
  );
}

function DoctorScreen({ raw, frame }: { raw?: string; frame: number }): React.ReactNode {
  return <Box flexDirection="column" paddingX={1} paddingTop={1}>
    <Text color={green} bold>🐊 Gator doctor</Text>
    <Box marginTop={1}><Divider /></Box>
    {raw === undefined
      ? <Box marginTop={1}><Text color={green}>{spinner[frame % spinner.length]} Checking Gator, models, and device…</Text></Box>
      : <Box marginTop={1}><DoctorReport raw={raw} /></Box>}
  </Box>;
}

function collectDoctor(binary: string, arguments_: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, ["ui-doctor-backend", ...arguments_], {
      env: { ...process.env, GATOR_UI_DOCTOR_BACKEND: "1" },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let report = "";
    let errors = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk: string) => { report += chunk; });
    child.stderr.on("data", (chunk: string) => { errors += chunk; });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) resolve(report);
      else reject(new Error(errors.trim() || `doctor checks exited with status ${code ?? "unknown"}`));
    });
  });
}

export async function runDoctor(binary: string, arguments_: string[]): Promise<void> {
  if (!process.stdout.isTTY) {
    const report = await collectDoctor(binary, arguments_);
    process.stdout.write(plainDoctorReport(report));
    return;
  }
  let frame = 0;
  let app: Instance | undefined;
  let timer: ReturnType<typeof setInterval> | undefined;
  let finalFrame: (() => void) | undefined;
  try {
    app = await render(<DoctorScreen frame={frame} />, { onFrame: () => { finalFrame?.(); finalFrame = undefined; } });
    timer = setInterval(() => app?.rerender(<DoctorScreen frame={++frame} />), 80);
    const report = await collectDoctor(binary, arguments_);
    clearInterval(timer);
    timer = undefined;
    const painted = new Promise<void>((resolve) => { finalFrame = resolve; });
    app.rerender(<DoctorScreen frame={frame} raw={report} />);
    await Promise.race([painted, new Promise((resolve) => setTimeout(resolve, 1000))]);
  } finally {
    if (timer) clearInterval(timer);
    app?.unmount();
    app?.cleanup();
    process.stdin.pause();
  }
}
