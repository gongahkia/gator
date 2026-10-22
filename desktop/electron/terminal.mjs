import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import { shellQuote } from "./policy.mjs";
import { powershellPath, windowsArguments } from "./platform.mjs";

const encodedPowerShell = (value) => Buffer.from(value, "utf16le").toString("base64");
const powershellQuote = (value) => `'${String(value).replaceAll("'", "''")}'`;

export function windowsTerminalScript(job, result) {
  const environment = (job.env ?? []).filter((value) => /^(GATOR_STATE_DIR|GATOR_CONFIG_DIR)=/.test(value));
  const payload = Buffer.from(JSON.stringify({ program: job.program, arguments: windowsArguments(job.args ?? []), dir: job.dir || process.cwd(), environment, result }), "utf8").toString("base64");
  // Decode data, then launch a native executable with an exact command line.
  // PowerShell 5.1's call operator would lose empty arguments and quotes.
  return `$ErrorActionPreference = 'Stop'
$job = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('${payload}')) | ConvertFrom-Json
$code = 1
try {
  $start = New-Object Diagnostics.ProcessStartInfo
  $start.FileName = $job.program
  $start.Arguments = $job.arguments
  $start.WorkingDirectory = $job.dir
  $start.UseShellExecute = $false
  $start.EnvironmentVariables['GATOR_INTERNAL_COMMANDS'] = '1'
  foreach ($entry in $job.environment) { $separator = $entry.IndexOf('='); $start.EnvironmentVariables[$entry.Substring(0, $separator)] = $entry.Substring($separator + 1) }
  $child = [Diagnostics.Process]::Start($start)
  $child.WaitForExit()
  $code = $child.ExitCode
} catch { Write-Host $_ -ForegroundColor Red }
finally { [IO.File]::WriteAllText($job.result, [string]$code) }
Write-Host ''; Write-Host 'Gator action finished. You can close this window.'
exit $code
`;
}

export function windowsTerminalLauncher(job, result) {
  const action = encodedPowerShell(windowsTerminalScript(job, result));
  // Start-Process supplies a real console with stdin. This hidden parent
  // detects the user closing that console before the action writes a result.
  return `$ErrorActionPreference = 'Stop'; try { $child = Start-Process -FilePath (Join-Path $PSHOME 'powershell.exe') -ArgumentList @('-NoProfile', '-EncodedCommand', '${action}') -PassThru; $child.WaitForExit() } finally { if (-not (Test-Path -LiteralPath ${powershellQuote(result)})) { [IO.File]::WriteAllText(${powershellQuote(result)}, '1') } }`;
}

export function terminalScript(job, result) {
  const environment = (job.env ?? []).filter((value) => /^(GATOR_STATE_DIR|GATOR_CONFIG_DIR)=/.test(value));
  // App-owned handoffs can use private runtime handlers; this never starts a Gator terminal UI.
  const command = ["env", "GATOR_INTERNAL_COMMANDS=1", ...environment.map(shellQuote), shellQuote(job.program), ...(job.args ?? []).map(shellQuote)].join(" ");
  const onExit = `gator_exit=$?; printf '%s' "$gator_exit" > ${shellQuote(result)}`;
  return `#!/bin/sh\numask 077\ntrap ${shellQuote(onExit)} EXIT\ntrap 'exit 130' INT TERM HUP\ncd ${shellQuote(job.dir || process.cwd())} || exit 1\n${command}\ngator_exit=$?\nprintf '\\nGator action finished. You can close this window.\\n'\nexit "$gator_exit"\n`;
}

// Only a Go-produced handoff can reach this function. The renderer cannot
// supply a program, argument, directory, environment, or shell fragment.
export async function terminalHandoff(job) {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-terminal-"));
  const script = join(directory, "Gator.command");
  const result = join(directory, "result");
  if (process.platform !== "win32") await writeFile(script, terminalScript(job, result), { mode: 0o700 });
  try {
    const launch = process.platform === "win32"
      ? spawn(powershellPath(), ["-NoProfile", "-NonInteractive", "-EncodedCommand", encodedPowerShell(windowsTerminalLauncher(job, result))], { windowsHide: true, stdio: "ignore" })
      : process.platform === "darwin"
      ? spawn("open", ["-a", "Terminal", script], { stdio: "ignore" })
      : spawn("x-terminal-emulator", ["-e", script], { stdio: "ignore" });
    let launchError;
    launch.once("error", (error) => { launchError = error; });
    if (process.platform !== "win32") await new Promise((resolve, reject) => {
      launch.once("error", reject);
      launch.once("exit", (code) => code === 0 ? resolve() : reject(new Error("Could not open the system terminal.")));
    });
    const deadline = Date.now() + 30 * 60 * 1000;
    while (Date.now() < deadline) {
      if (launchError) throw launchError;
      if (process.platform === "win32" && launch.exitCode !== null && launch.exitCode !== 0) throw new Error("Could not open the Windows terminal action.");
      try {
        const code = (await readFile(result, "utf8")).trim();
        return code === "0" ? "" : `The terminal action ended with status ${code}.`;
      } catch (error) { if (error.code !== "ENOENT") throw error; }
      await new Promise((resolve) => setTimeout(resolve, 350));
    }
    return "The terminal action has not returned. Check its terminal window before trying again.";
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}
