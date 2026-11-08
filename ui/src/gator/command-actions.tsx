import type { BackendCommand } from "@gator/application-host/contracts/protocol"
import type { GatorClient } from "./client"
import type { useActions } from "./actions"

// These controls submit canonical Go commands. They do not apply permissions,
// settings or external changes in the renderer.
export function runBackendCommand(client: GatorClient, controls: ReturnType<typeof useActions>, command: BackendCommand) {
  if (!command.needs_input) return client.navigate(`/${command.name}`)
  const context = client.context
  const submit = async (value = "") => {
    if (!await client.mayLeave()) return
    if (client.context !== context) throw new Error("The conversation changed. Reopen this command in the intended chat.")
    await client.navigate("/chat")
    if (client.context !== context) throw new Error("The conversation changed. Reopen this command in the intended chat.")
    await client.send({ type: "submit", text: `/${command.name}${value ? " " + value : ""}` })
  }
  if (command.name === "mode") return controls.menu("Permissions for future work", [
    { title: "Inspect", description: "Read and answer without preparing external actions.", run: () => submit("inspect") },
    { title: "Draft", description: "Prepare work for review without sending it.", run: () => submit("draft") },
    { title: "Act", description: "Allow supported actions; consequential actions keep their exact approvals.", run: () => submit("act") },
    { title: "Automatic", description: "Let Gator choose the work mode within current permissions and approval rules.", run: () => submit("auto") },
  ])
  if (command.name === "theme") return controls.actions.appearance()
  if (command.name === "connector") return client.navigate("/sources")
  if (command.name === "jobs") return client.navigate("/followup")
  if (command.name === "inbox") return submit()
  if (command.name === "detach") return controls.menu("Remove a workspace attachment", [
    ...(client.state?.work_details?.attachments ?? []).map(path => ({ title: path, run: () => submit(path) })),
    { title: "Remove all workspace attachments", run: () => submit("all") },
  ])
  if (command.name === "attach" || command.name === "source ignore" || command.name === "source unignore") {
    return controls.prompt(command.description, "", path => {
      const value = path.trim()
      if (!value || value.startsWith("/") || value.split(/[\\/]/).includes("..")) throw new Error("Enter a path inside this workspace, relative to its root.")
      return submit(value)
    }, "Workspace-relative file path, for example notes/brief.md. Changes apply to future messages; the current draft is kept.")
  }
  // Retain advanced syntax while typed controls are introduced for the remaining
  // command families. Keep this visibly explicit rather than pretending a raw
  // command field is a structured form.
  return controls.prompt(command.description, "", submit, `Advanced /${command.name} command values. /help lists available syntax. Your draft stays in the composer.`)
}
