import { spawn } from "node:child_process";
import { Box, Text, render, type Instance } from "@claude-code-kit/ink-renderer";
import { Divider, Spinner } from "@claude-code-kit/ui";
import React from "react";
import { ResultBody, type BackendResult } from "./result.js";
import { GatorPaletteProvider, useGatorPalette } from "./theme.js";

type DoctorPayload = { theme: string; result: BackendResult };

function DoctorScreen({ result }: { result?: BackendResult }): React.ReactNode {
  const palette = useGatorPalette();
  return <Box flexDirection="column" paddingX={1} paddingTop={1}>
    <Text color={palette.accent} bold>🐊 Gator doctor</Text>
    <Box marginTop={1}><Divider color={palette.accent} /></Box>
    <Box marginTop={1}>{result ? <ResultBody result={result} /> : <Spinner verb="Checking Gator, models, and device" color={palette.accent} />}</Box>
  </Box>;
}

function collectDoctor(binary: string, arguments_: string[], structured: boolean): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, ["ui-doctor-backend", ...(structured ? ["--json"] : []), ...arguments_], {
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
    const report = await collectDoctor(binary, arguments_, false);
    process.stdout.write(`🐊 Gator doctor\n${report}`);
    return;
  }
  let app: Instance | undefined;
  let finalFrame: (() => void) | undefined;
  try {
    app = await render(<GatorPaletteProvider><DoctorScreen /></GatorPaletteProvider>, { onFrame: () => { finalFrame?.(); finalFrame = undefined; } });
    const payload = JSON.parse(await collectDoctor(binary, arguments_, true)) as DoctorPayload;
    const painted = new Promise<void>((resolve) => { finalFrame = resolve; });
    app.rerender(<GatorPaletteProvider theme={payload.theme}><DoctorScreen result={payload.result} /></GatorPaletteProvider>);
    await Promise.race([painted, new Promise((resolve) => setTimeout(resolve, 1000))]);
  } finally {
    app?.unmount();
    app?.cleanup();
    process.stdin.pause();
  }
}
