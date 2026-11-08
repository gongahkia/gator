import { Keymap } from "../context/keymap"
import { useTheme } from "../context/theme"
import { useNewDesigns } from "../config"
import { DialogHeader, DialogLayout, useDialog } from "./dialog"
import { createStore } from "solid-js/store"
import { For } from "solid-js"
import { Locale } from "../util/locale"
import { useTerminalDimensions } from "@opentui/solid"
import type { ScrollBoxRenderable } from "@opentui/core"

export type DialogConfirmProps = {
  title: string
  message: string
  onConfirm?: () => void
  onCancel?: () => void
  label?: {
    confirm?: string
    cancel?: string
  }
}

export function DialogConfirm(props: DialogConfirmProps) {
  const dialog = useDialog()
  const theme = useTheme().surface("dialog")
  const newDesigns = useNewDesigns()
  const dimensions = useTerminalDimensions()
  let scroll: ScrollBoxRenderable | undefined
  Keymap.createLayer(() => ({ mode: "modal", commands: [
    { bind: "up", title: "Earlier confirmation line", run: () => scroll?.scrollBy(-1) },
    { bind: "down", title: "Later confirmation line", run: () => scroll?.scrollBy(1) },
    { bind: "pageup", title: "Earlier confirmation page", run: () => scroll?.scrollBy(-8) },
    { bind: "pagedown", title: "Later confirmation page", run: () => scroll?.scrollBy(8) },
  ] }))
  const [store, setStore] = createStore({
    active: "confirm" as "confirm" | "cancel",
  })

  const choose = (key: "confirm" | "cancel") => {
    if (key === "confirm") props.onConfirm?.()
    if (key === "cancel") props.onCancel?.()
    dialog.clear()
  }

  Keymap.createLayer(() => ({
    mode: "modal",
    commands: newDesigns()
      ? [
          { bind: "return", title: "Confirm", group: "Dialog", run: () => choose("confirm") },
          { bind: "escape", title: "Cancel", group: "Dialog", run: () => choose("cancel") },
        ]
      : [
          {
            bind: "return",
            title: "Confirm dialog selection",
            group: "Dialog",
            run: () => choose(store.active),
          },
          {
            bind: "left",
            title: "Previous dialog option",
            group: "Dialog",
            run: () => {
              setStore("active", store.active === "confirm" ? "cancel" : "confirm")
            },
          },
          {
            bind: "right",
            title: "Next dialog option",
            group: "Dialog",
            run: () => {
              setStore("active", store.active === "confirm" ? "cancel" : "confirm")
            },
          },
        ],
  }))
  return (
    <DialogLayout
      header={<DialogHeader title={props.title} />}
      footer={{
        actions: {
          primary: [
            // Footer labels are lowercase; the legacy buttons title-case the same labels.
            { title: (props.label?.cancel ?? "cancel").toLowerCase(), key: "esc", onTrigger: () => choose("cancel") },
            {
              title: (props.label?.confirm ?? "confirm").toLowerCase(),
              key: "enter",
              onTrigger: () => choose("confirm"),
            },
          ],
        },
      }}
      legacyFooter={
        <box flexDirection="row" justifyContent="flex-end">
          <For each={["cancel", "confirm"] as const}>
            {(key) => (
              <box
                paddingLeft={1}
                paddingRight={1}
                backgroundColor={key === store.active ? theme.background.action.primary.focused : undefined}
                onMouseUp={() => choose(key)}
              >
                <text fg={key === store.active ? theme.text.action.primary.focused : theme.text.muted}>
                  {Locale.titlecase(props.label?.[key] ?? key)}
                </text>
              </box>
            )}
          </For>
        </box>
      }
    >
      <scrollbox ref={node => { scroll = node }} maxHeight={Math.max(2, dimensions().height - (dimensions().height >= 24 ? Math.floor(dimensions().height / 4) : 0) - 8)}><text fg={theme.text.base}>{props.message}</text></scrollbox>
    </DialogLayout>
  )
}
