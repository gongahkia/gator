import { useClient } from "../context/client"
import { useDialog } from "../ui/dialog"
import { DialogSelect, type DialogSelectRef, type DialogSelectView } from "../ui/dialog-select"
import type { Choice } from "./actions"
import { literal } from "./text"
import { onCleanup } from "solid-js"

type Choices = Choice[] | (() => Choice[])
type Menu = { title: string; context: string; choices: Choices; disabled: boolean; parent?: Menu; back?: () => void; view?: DialogSelectView<string>; ref?: DialogSelectRef<string> }

// Navigation snapshots contain presentation only. Rebuilt options keep their
// own current backend identities and all original action validation.
export function useMenuNavigation(attempt: (operation: unknown, label?: string) => void) {
  const client = useClient(), dialog = useDialog()
  let active: Menu | undefined, restoring: Menu | undefined
  const reset = () => { active = undefined; restoring = undefined }
  onCleanup(reset)
  const capture = (menu: Menu) => { if (menu.ref) { menu.view = menu.ref.view; menu.ref = undefined } }
  const returnTo = (target?: Menu, refresh?: () => void) => () => {
    if (target && target.context !== client.context) { reset(); dialog.clear(); return }
    restoring = target; active = target
    try { if (refresh) refresh(); else if (target) show(target); else { reset(); dialog.clear() } }
    catch (error) { reset(); client.fail(error) }
  }
  function show(frame: Menu) {
    active = frame; frame.ref = undefined
    const options = () => {
      const duplicates = new Map<string, number>()
      return (typeof frame.choices === "function" ? frame.choices() : frame.choices).map(item => {
        const label = JSON.stringify([item.category, item.title]), duplicate = duplicates.get(label) ?? 0
        duplicates.set(label, duplicate + 1)
        return { title: literal(item.title), description: literal(item.description), category: literal(item.category), disabled: item.disabled,
          value: item.key ?? `${label}:${duplicate}`, onSelect: () => {
            capture(frame); active = frame; dialog.clear()
            try { attempt(item.run(), literal(item.title) + "…") } catch (error) { client.fail(error) }
          } }
      })
    }
    dialog.replace(() => <DialogSelect<string> title={literal(frame.title)} showDisabled={frame.disabled} restore={frame.view} preserveSelection
      ref={ref => { frame.ref = ref }} onCancel={() => { capture(frame); returnTo(frame.parent, frame.back)() }} options={options()} />, () => capture(frame))
  }
  function menu(title: string, choices: Choices, disabled = false, back?: () => void) {
    let matched = restoring ?? (active?.title === title && active.context === client.context ? active : undefined)
    while (matched && (matched.title !== title || matched.context !== client.context)) matched = matched.parent
    restoring = undefined
    const frame: Menu = matched ? { ...matched, choices, disabled, back } : { title, choices, disabled, back, context: client.context, parent: active?.context === client.context ? active : undefined }
    show(frame)
  }
  return { menu, reset, back: (refresh?: () => void) => active || refresh ? returnTo(active, refresh) : undefined }
}
