// Adapted from OpenCode's command palette. Configuration entries come from
// Gator's registered commands rather than upstream plugin/config discovery.
import { createMemo, createSignal, Show } from "solid-js"
import { DialogSelect } from "../ui/dialog-select"
import { Keymap, COMMAND_PALETTE_COMMAND } from "../context/keymap"
import { useDialog } from "../ui/dialog"
import { commandGroup, commandGroups } from "../gator/command-groups"
export function CommandPaletteDialog() {
  const commands = Keymap.useCommands(), shortcuts = Keymap.useShortcuts(), dialog = useDialog()
  const [section, setSection] = createSignal<string>()
  const [query, setQuery] = createSignal("")
  const options = createMemo(() => commands().flatMap(command => !command.id || !command.palette || command.id === COMMAND_PALETTE_COMMAND ? [] : [{
    title: command.title ?? command.id, description: command.description, category: command.group || commandGroup(command.id.replace(/^gator\./, "")),
    footer: shortcuts.all(command.id), value: command.id,
    onSelect: () => { dialog.clear(); void command.run() },
  }]))
  const roots = () => commandGroups.map(group => ({ title: group, description: "Open menu →", value: `menu:${group}`, onSelect: () => { setQuery(""); setSection(group) } }))
  return <Show when={section()} keyed fallback={<DialogSelect title="Gator commands" searchInTitle titlePlaceholder="Search Gator commands"
    onFilter={setQuery} options={[...roots(), ...(query().trim() ? options().filter(option => !option.value.startsWith("gator.command.") && !["gator.settings", "gator.following", "gator.saved"].includes(option.value)) : [])]} flat />}>
    {group => <DialogSelect title={`Gator › ${group}`} placeholder={`Search ${group.toLowerCase()}`} onCancel={() => { setQuery(""); setSection(undefined) }}
      options={options().filter(option => option.category === group)} flat />}
  </Show>
}
