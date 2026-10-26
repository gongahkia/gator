// Adapted from OpenCode's command palette. Configuration entries come from
// Gator's registered commands rather than upstream plugin/config discovery.
import { createMemo } from "solid-js"
import { DialogSelect } from "../ui/dialog-select"
import { Keymap, COMMAND_PALETTE_COMMAND } from "../context/keymap"
import { useDialog } from "../ui/dialog"
export function CommandPaletteDialog() {
  const commands = Keymap.useCommands(), shortcuts = Keymap.useShortcuts(), dialog = useDialog()
  const options = createMemo(() => commands().flatMap(command => !command.id || !command.palette || command.id === COMMAND_PALETTE_COMMAND ? [] : [{
    title: command.title ?? command.id, description: command.description, category: command.group,
    footer: shortcuts.all(command.id), value: command.id,
    onSelect: () => { dialog.clear(); void command.run() },
  }]))
  return <DialogSelect title="Gator commands" searchInTitle titlePlaceholder="Search Gator commands" options={options()} flat />
}
