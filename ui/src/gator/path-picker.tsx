import { createSignal, createResource, Show } from "solid-js"
import { readdir, stat } from "node:fs/promises"
import { basename, dirname, join, resolve } from "node:path"
import { useDialog } from "../ui/dialog"
import { DialogSelect, type DialogSelectRef } from "../ui/dialog-select"
import { DialogPrompt } from "../ui/dialog-prompt"
import { Keymap } from "../context/keymap"
import { Spinner } from "../component/spinner"
import { literal } from "./text"

// Selection only fills an existing form or stages a private upload. It does not
// change a source, grant tool access, restore data, or submit work by itself.
export function PathPicker(props: { kind: "file" | "directory" | "save"; initial?: string; choose(path: string): void }) {
  const dialog = useDialog(), [folder, setFolder] = createSignal(resolve(props.initial || process.cwd()))
  let list: DialogSelectRef<string> | undefined
  const navigate = (path: string) => { list?.setFilter(""); setFolder(path) }
  const [entries] = createResource(folder, path => readdir(path, { withFileTypes: true }).then(items => items.sort((a, b) => Number(b.isDirectory()) - Number(a.isDirectory()) || a.name.localeCompare(b.name))))
  const jump = () => dialog.replace(() => <DialogPrompt title="Path" value={folder()} subtitle="Tab in the picker filters loaded filenames. Enter an absolute or relative path here." onConfirm={path => {
    if (props.kind === "save") props.choose(resolve(folder(), path))
    else {
      const selected = resolve(folder(), path)
      void stat(selected).then(info => { if (info.isFile() && props.kind === "file") props.choose(selected); else dialog.replace(() => <PathPicker {...props} initial={selected} />) }).catch(() => dialog.replace(() => <PathPicker {...props} initial={selected} />))
    }
  }} />)
  Keymap.createLayer(() => ({ mode: "modal", priority: 2, commands: [{ bind: "ctrl+l", title: "Enter directory or destination path", run: jump }] }))
  return <Show when={!entries.error} fallback={<DialogPrompt title="Folder could not be listed" subtitle={String(entries.error)} value={folder()} onConfirm={path => { dialog.replace(() => <PathPicker {...props} initial={resolve(path)} />) }} />}>
    <Show when={entries.loading}><box paddingX={3}><Spinner delay={150}>Reading folder…</Spinner></box></Show>
    <DialogSelect ref={value => { list = value }} title={props.kind === "directory" ? "Choose folder" : props.kind === "save" ? "Export destination" : "Choose file"} description={literal(folder())} placeholder="Filter filenames · Ctrl+L enters a path" options={[
      { title: ".. (parent folder)", value: "..", onSelect: () => { navigate(dirname(folder())) } },
      ...(props.kind === "directory" ? [{ title: "Choose this folder", value: ".", onSelect: () => props.choose(folder()) }] : []),
      ...(props.kind === "save" ? [{ title: "Enter destination filename", value: "save", onSelect: jump }] : []),
      ...(entries.loading ? [] : entries() ?? []).filter(entry => props.kind !== "directory" || entry.isDirectory()).map(entry => ({ title: entry.name + (entry.isDirectory() ? "/" : ""), value: entry.name, description: entry.isDirectory() ? "folder" : "file", onSelect: () => {
        if (entry.isDirectory()) navigate(join(folder(), entry.name))
        else props.choose(join(folder(), entry.name))
      } })),
    ]} />
  </Show>
}
