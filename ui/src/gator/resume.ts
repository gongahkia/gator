// Arguments are data, never shell fragments. The same parser serves the source
// entry point and the packaged launcher.
export function resumeID(args: string[]): string | undefined {
  if (!args.length) return
  if (args.length !== 2 || args[0] !== "resume" || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(args[1]!)) throw new Error("Usage: gator resume <chat-id>")
  return args[1]
}

export function resumeCommand(id: string | undefined, launcher: string, executable: string, installed: boolean, windows = process.platform === "win32") {
  if (!id || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(id)) return
  const quote = (value: string) => windows ? `'${value.replaceAll("'", "''")}'` : `'${value.replaceAll("'", "'\\''")}'`
  const prefix = installed ? `${windows ? "& " : ""}${quote(launcher)}` : `${windows ? "& " : ""}${quote(executable)} ${quote(launcher)}`
  return `${prefix} resume ${id}`
}
