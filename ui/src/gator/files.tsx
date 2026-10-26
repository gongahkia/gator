import { createEffect, createSignal, For, onCleanup, Show } from "solid-js"
import { NativeImage, TextareaRenderable } from "@opentui/core"
import { useTerminalDimensions } from "@opentui/solid"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"
import { useDialog, DialogHeader, DialogLayout } from "../ui/dialog"
import { DialogSelect } from "../ui/dialog-select"
import { DialogPrompt } from "../ui/dialog-prompt"
import type { FileReference, FileView, FileEdit, FileRequest, FileParagraph } from "@gator/application-host/contracts/fileProtocol"
import type { Choice } from "./actions"
import { literal } from "./text"

export function useFiles(menu: (title: string, choices: Choice[]) => void, prompt: (title: string, value: string, save: (text: string) => unknown, description?: string) => void) {
  const client = useClient(), dialog = useDialog()
  async function open(ref: FileReference, version?: string) {
    const request: FileRequest = { action: "open", context: client.context, ref, version }
    const view = await client.host.file(request)
    dialog.replace(() => <FileEditor view={view} request={request} reopen={version => open(ref, version)} menu={menu} prompt={prompt} />, undefined, { size: "xlarge" })
  }
  function files() {
    const choices: Choice[] = [], seen = new Set<string>()
    for (const file of client.draft.uploads ?? []) { seen.add(file.id); choices.push({ title: file.name, description: "Unsent private input", run: () => open({ kind: "upload", id: file.id }) }) }
    for (const message of client.state?.messages ?? []) {
      for (const [index, id] of (message.input_ids ?? []).entries()) if (!seen.has(id)) { seen.add(id); choices.push({ title: message.inputs?.[index] || id.split("/").at(-1) || id, description: "Retained input", run: () => open({ kind: "upload", id }) }) }
      for (const file of message.bundle?.Artifacts ?? []) {
        const bundle = message.bundle!.Path, identity = `${bundle}:${file.Path}`
        if (seen.has(identity)) continue; seen.add(identity)
        choices.push({ title: file.Path, description: `Output · ${file.MediaType}`, run: () => open({ kind: "output", bundle, path: file.Path }) })
      }
    }
    menu("Files in this conversation", choices)
  }
  return { files, open }
}

function FileEditor(props: { view: FileView; request: FileRequest; reopen(version?: string): Promise<void>; menu(title: string, choices: Choice[]): void; prompt(title: string, value: string, save: (text: string) => unknown, description?: string): void }) {
  const client = useClient(), theme = useTheme(), dialog = useDialog(), dimensions = useTerminalDimensions()
  const [edit, setEdit] = createSignal<FileEdit>(props.view.draft ?? {}), [dirty, setDirty] = createSignal(!!props.view.draft)
  const [notice, setNotice] = createSignal(props.view.staleDraft ? "A stale editor draft is retained. Copy edits before discarding to reload." : props.view.notice || "")
  const [page, setPage] = createSignal(1), [image, setImage] = createSignal<Uint8Array>(), [pages, setPages] = createSignal(1), [busy, setBusy] = createSignal(false)
  let textarea: TextareaRenderable | undefined, timer: ReturnType<typeof setTimeout> | undefined
  let active = true
  const editable = () => props.view.editable && props.view.revision === props.view.current
  const request = (action: FileRequest["action"], extra: Partial<FileRequest> = {}): FileRequest => ({ ...props.request, action, expected: props.view.current, version: props.view.revision, ...extra })
  const attempt = (operation: Promise<unknown>) => { void operation.catch(error => { setNotice(error instanceof Error ? error.message : String(error)); client.fail(error) }) }
  async function retain() { if (dirty() && editable()) await client.host.file(request("draft", { edit: edit() })) }
  function change(next: FileEdit) {
    if (!editable()) return
    setEdit(next); setDirty(true); clearTimeout(timer)
    timer = setTimeout(() => attempt(retain()), 200)
  }
  onCleanup(() => { active = false; clearTimeout(timer); if (dirty()) attempt(retain()) })
  createEffect(() => {
    if (props.view.kind === "image" && props.view.data) { const encoded = props.view.data.split(",")[1]; setImage(new Uint8Array(Buffer.from(encoded, "base64"))) }
    if (props.view.kind === "pdf") {
      const selected = page(); setBusy(true)
      void client.host.pdf(props.request, selected).then(result => { if (active) { setImage(result.image); setPages(result.pages) } }).catch(error => { if (active) setNotice(String(error)) }).finally(() => { if (active) setBusy(false) })
    }
  })
  async function save() { clearTimeout(timer); await retain(); await client.host.file(request("save", { edit: edit() })); setDirty(false); await props.reopen() }
  async function discard() { clearTimeout(timer); await client.host.file(request("discard")); setDirty(false); await props.reopen() }
  const selection = () => textarea?.getSelectedText() || ""
  function close() {
    if (!dirty()) { dialog.clear(); return }
    props.menu("Unsaved file edits", [
      { title: "Save a new immutable working version", run: save },
      { title: "Keep edit draft and close", run: async () => { await retain(); setDirty(false); dialog.clear() } },
      { title: "Discard edits", run: discard },
      { title: "Continue editing", run: () => props.reopen(props.view.revision) },
    ])
  }
  function paragraphs(): FileParagraph[] {
    if (props.view.blocks) return props.view.blocks.flatMap(block => block.type === "paragraph" ? [block] : block.rows.flatMap(row => row.flatMap(cell => cell)))
    return props.view.slides?.flatMap(slide => slide.shapes.flatMap(shape => shape.paragraphs)) ?? []
  }
  function editContent() {
    if (!editable()) return
    if (["word", "slides"].includes(props.view.kind)) {
      const list = paragraphs()
      props.menu("Choose existing text content", list.map(paragraph => ({ title: edit().paragraphs?.[paragraph.id] ?? paragraph.text, description: `${paragraph.id} · ${paragraph.runs.map(run => [run.bold ? "bold" : "", run.italic ? "italic" : "", run.size ? `${run.size}pt` : ""].filter(Boolean).join(" ")).filter(Boolean).join(", ")}`, run: () => props.prompt("Edit " + paragraph.id, edit().paragraphs?.[paragraph.id] ?? paragraph.text, async text => { const next = { ...edit(), paragraphs: { ...edit().paragraphs, [paragraph.id]: text } }; change(next); await client.host.file(request("draft", { edit: next })); await props.reopen() }) })))
    } else if (props.view.kind === "table" || props.view.kind === "workbook") {
      const sheets = props.view.sheets
      const pickCell = (sheetID?: number) => props.prompt("Cell address", props.view.kind === "table" ? "1:1" : "A1", address => {
        const key = sheetID === undefined ? address.split(":").map(n => Number(n) - 1).join(":") : `${sheetID}!${address.toUpperCase()}`
        const cell = sheets?.find(s => s.id === sheetID)?.cells.find(c => c.address === address.toUpperCase())
        const previous = edit().cells?.[key] ?? (sheetID === undefined ? (() => { const [r, c] = key.split(":").map(Number); return props.view.rows?.[r]?.[c] })() : cell?.formula ? `=${cell.formula}` : cell?.value) ?? ""
        client.fileSelections.set(props.view.id, `Selected cell ${address}${sheetID === undefined ? "" : ` in worksheet ${sheetID + 1}`}: ${previous}`)
        props.prompt("Edit cell " + address, previous, async value => { const next = { ...edit(), cells: { ...edit().cells, [key]: value } }; change(next); await client.host.file(request("draft", { edit: next })); await props.reopen() }, "Start a spreadsheet formula with =. Existing formulas and styles are preserved; this editor does not recalculate formulas.")
      }, props.view.kind === "table" ? "Use one-based row:column, e.g. 2:3." : "Use an Excel cell address, e.g. B7.")
      if (sheets) props.menu("Choose worksheet", sheets.map(sheet => ({ title: sheet.name, disabled: sheet.protected, run: () => pickCell(sheet.id) })))
      else pickCell()
    }
  }
  function versions() {
    const selectedText = selection() || client.fileSelections.get(props.view.id) || ""
    props.menu("Saved file versions", props.view.versions.map((version, index) => ({ title: `Version ${index + 1} · ${version.author}`, description: `${version.at} · ${version.bytes} bytes · ${version.hash}`, run: () => props.menu("Selected immutable version", [
      { title: "Open this version", run: async () => { await retain(); await props.reopen(version.id) } },
      { title: "Restore as a new working version", run: async () => { await retain(); await client.host.file(request("restore", { version: version.id })); setDirty(false); await props.reopen() } },
      { title: "Compare with current", run: async () => {
        const comparison = await client.host.file({ ...request("compare"), action: "compare", base: version.id, version: props.view.current })
        dialog.replace(() => <DialogLayout header={<DialogHeader title="Saved version comparison" />}><scrollbox height={Math.max(8, dimensions().height - 15)}><For each={comparison.notices}>{line => <text>{literal(line)}</text>}</For><Show when={comparison.truncated}><text>Partial comparison: retained files remain complete.</text></Show><For each={comparison.changes}>{change => <box><text><b>{literal(change.location)}</b></text><text fg={theme.text.feedback.error.base}>− {literal(change.before)}</text><text fg={theme.accent}>+ {literal(change.after)}</text></box>}</For></scrollbox></DialogLayout>, undefined, { size: "xlarge" })
      } },
      { title: "Export exact selected bytes", run: () => props.prompt("Export destination (new file)", props.view.name, path => client.host.exportFile(request("export", { version: version.id }), path)) },
      { title: "Ask Gator about this version", run: async () => {
        const file = await client.host.file({ ...request("attach", { version: version.id }), action: "attach" }); await client.stage(file, selectedText ? `About ${props.view.name}, selected content:\n${selectedText}` : ""); client.route = "chat"; client.emit("change")
      } },
    ]) })))
  }
  Keymap.createLayer(() => ({ mode: "modal", priority: 3, commands: [
    { bind: "escape", title: "Close file with draft review", run: close },
    { bind: "ctrl+s", title: "Save file version", enabled: editable, run: () => attempt(save()) },
    { bind: "f3", title: "Edit Office/table content", run: editContent },
    { bind: "f2", title: "File versions", run: () => { attempt(retain().then(versions)) } },
    { bind: "f4", title: "Discard file edits", run: () => attempt(discard()) },
    { bind: "ctrl+right", title: "Next PDF page", enabled: props.view.kind === "pdf", run: () => { setPage(p => Math.min(pages(), p + 1)) } },
    { bind: "ctrl+left", title: "Previous PDF page", enabled: props.view.kind === "pdf", run: () => { setPage(p => Math.max(1, p - 1)) } },
  ] }))
  return <DialogLayout header={<DialogHeader title={`${literal(props.view.name)}${dirty() ? " · Unsaved" : ""}`} onClose={close} />}>
    <box height={Math.max(8, dimensions().height - 14)} minHeight={0}>
      <Show when={props.view.kind === "text" || props.view.kind === "markdown"} fallback={
        <Show when={image()} fallback={<scrollbox flexGrow={1}>
          <For each={props.view.rows}>{(row, index) => <text>{index() + 1}: {row.map((cell, col) => literal(edit().cells?.[`${index()}:${col}`] ?? cell)).join(" │ ")}</text>}</For>
          <For each={props.view.sheets}>{sheet => <box marginBottom={1}><text fg={theme.accent}>{literal(sheet.name)}{sheet.protected ? " · Protected" : ""} · merges {sheet.merges.join(", ")}</text><For each={sheet.cells}>{cell => <text>{cell.address}: {literal(edit().cells?.[`${sheet.id}!${cell.address}`] ?? (cell.formula ? `=${cell.formula}` : cell.value))}</text>}</For></box>}</For>
          <For each={paragraphs()}>{paragraph => <box marginBottom={1}><text fg={theme.text.muted}>{literal(paragraph.id)} · {literal(paragraph.style)}</text><text>{literal(edit().paragraphs?.[paragraph.id] ?? paragraph.text)}</text><For each={paragraph.images}>{image => <image source={image} height={8} width="100%" fit="fit" />}</For></box>}</For>
        </scrollbox>}>{bytes => <image source={bytes()} flexGrow={1} width="100%" fit="fit" onError={error => setNotice(String(error))} />}</Show>
      }>
        <textarea id="file-text-editor" ref={node => { textarea = node; if (!editable()) node.traits = { suspend: true } }} focused={editable()} initialValue={edit().text ?? props.view.text ?? ""} flexGrow={1} wrapMode="word" textColor={theme.text.base}
          onContentChange={() => { if (textarea && editable()) change({ text: textarea.plainText }) }} />
      </Show>
    </box>
    <Show when={props.view.kind === "pdf"}><text>PDF page {page()} of {pages()}{busy() ? " · Rendering…" : ""} · Ctrl+←/→ pages</text></Show>
    <text fg={theme.text.muted}>{literal(notice())}</text>
    <text fg={theme.text.muted}>Ctrl+S save · F3 edit Office/cell · F2 versions/export/Ask Gator · F4 discard · Esc close</text>
  </DialogLayout>
}
