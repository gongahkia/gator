import { createEffect, createSignal, createMemo, For, onCleanup, Show } from "solid-js"
import { NativeImage, TextareaRenderable, type Renderable } from "@opentui/core"
import { useTerminalDimensions, useRenderer } from "@opentui/solid"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"
import { useDialog, DialogHeader, DialogLayout } from "../ui/dialog"
import { DialogSelect } from "../ui/dialog-select"
import { DialogPrompt } from "../ui/dialog-prompt"
import { DialogConfirm } from "../ui/dialog-confirm"
import type { FileReference, FileView, FileEdit, FileRequest } from "@gator/application-host/contracts/fileProtocol"
import type { Choice } from "./actions"
import { literal } from "./text"
import { cellCoordinates, columnName, fileParagraphs, selectedContent, type FileSelection } from "./file-selection"
import { resolve } from "node:path"

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
  const client = useClient(), theme = useTheme(), dialog = useDialog(), dimensions = useTerminalDimensions(), renderer = useRenderer()
  const [edit, setEdit] = createSignal<FileEdit>(props.view.draft ?? {}), [dirty, setDirty] = createSignal(!!props.view.draft)
  const [notice, setNotice] = createSignal(props.view.staleDraft ? "A stale editor draft is retained. Copy edits before discarding to reload." : props.view.notice || "")
  const [page, setPage] = createSignal(1), [image, setImage] = createSignal<Uint8Array>(), [pages, setPages] = createSignal(1), [busy, setBusy] = createSignal(false)
  const [position, setPosition] = createSignal(client.filePositions.get(props.view.id) ?? { sheet: props.view.sheets?.[0]?.id ?? 0, row: 0, col: 0 })
  const sheet = () => props.view.sheets?.find(sheet => sheet.id === position().sheet) ?? props.view.sheets?.[0]
  const grid = () => props.view.kind === "table" || props.view.kind === "workbook"
  const cells = createMemo(() => new Map(sheet()?.cells.map(cell => [cell.address, cell]) ?? []))
  const size = createMemo(() => {
    let rows = sheet()?.maxRow ?? props.view.rows?.length ?? 0, cols = sheet()?.maxCol ?? Math.max(0, ...(props.view.rows ?? []).map(row => row.length))
    for (const key of Object.keys(edit().cells ?? {})) {
      let point: { row: number; col: number }
      if (sheet()) { const [id, address] = key.split("!"); if (Number(id) !== sheet()!.id) continue; point = cellCoordinates(address) }
      else { const [row, col] = key.split(":").map(Number); point = { row, col } }
      rows = Math.max(rows, point.row + 1); cols = Math.max(cols, point.col + 1)
    }
    return { rows, cols }
  })
  createEffect(() => { client.filePositions.set(props.view.id, position()) })
  const gridValue = (row: number, col: number) => {
    const address = `${columnName(col)}${row + 1}`, cell = cells().get(address), key = sheet() ? `${sheet()!.id}!${address}` : `${row}:${col}`
    return edit().cells?.[key] ?? (sheet() ? cell?.formula ? `=${cell.formula}` : cell?.value ?? "" : props.view.rows?.[row]?.[col] ?? "")
  }
  let textarea: TextareaRenderable | undefined, timer: ReturnType<typeof setTimeout> | undefined
  let active = true
  let body: Renderable | undefined
  let rendering = 0
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
      const selected = page(), generation = ++rendering; setBusy(true)
      void client.host.pdf(props.request, selected).then(result => { if (active && generation === rendering) { setImage(result.image); setPages(result.pages) } }).catch(error => { if (active && generation === rendering) setNotice(String(error)) }).finally(() => { if (active && generation === rendering) setBusy(false) })
    }
  })
  async function save() { clearTimeout(timer); await retain(); await client.host.file(request("save", { edit: edit() })); setDirty(false); await props.reopen() }
  async function discard() { clearTimeout(timer); await client.host.file(request("discard")); setDirty(false); await props.reopen() }
  const selection = (): FileSelection | undefined => {
    const selected = renderer.getSelection()
    const belongs = selected?.selectedRenderables.length && selected.selectedRenderables.every(node => { for (let parent: Renderable | null = node; parent; parent = parent.parent) if (parent === body) return true; return false })
    const text = textarea?.getSelectedText() || (belongs ? selected?.getSelectedText() : "")
    return text ? { kind: "quote", text: text.slice(0, 4000), revision: props.view.revision } : client.fileSelections.get(props.view.id)
  }
  function close() {
    if (!dirty()) { dialog.clear(); return }
    props.menu("Unsaved file edits", [
      { title: "Save a new immutable working version", run: save },
      { title: "Keep edit draft and close", run: async () => { await retain(); setDirty(false); dialog.clear() } },
      { title: "Discard edits", run: discard },
      { title: "Continue editing", run: () => props.reopen(props.view.revision) },
    ])
  }
  const paragraphs = () => fileParagraphs(props.view)
  function selectContent() {
    const stage = async (selection: FileSelection) => { client.fileSelections.set(props.view.id, selection); await props.reopen(props.view.revision) }
    const chooseCell = (sheet?: number) => props.prompt("Select cell for Ask Gator", "A1", async address => { cellCoordinates(address); await stage({ kind: "cell", address: address.toUpperCase(), sheet }) }, "Selection is read from the exact saved version you later attach. It does not edit or send.")
    if (props.view.kind === "table") chooseCell()
    else if (props.view.sheets) props.menu("Select worksheet content", props.view.sheets.map(sheet => ({ title: sheet.name, run: () => chooseCell(sheet.id) })))
    else if (paragraphs().length) props.menu("Select paragraph for Ask Gator", paragraphs().map(paragraph => ({ title: paragraph.text, run: () => stage({ kind: "paragraph", id: paragraph.id }) })))
    else setNotice("Select text in the editor or preview, then use F2 to choose a saved version.")
  }
  function editContent() {
    if (!editable()) return
    if (["word", "slides"].includes(props.view.kind)) {
      const list = paragraphs()
      props.menu("Choose existing text content", list.map(paragraph => ({ title: edit().paragraphs?.[paragraph.id] ?? paragraph.text, description: `${paragraph.id} · ${paragraph.runs.map(run => [run.bold ? "bold" : "", run.italic ? "italic" : "", run.size ? `${run.size}pt` : ""].filter(Boolean).join(" ")).filter(Boolean).join(", ")}`, run: () => props.prompt("Edit " + paragraph.id, edit().paragraphs?.[paragraph.id] ?? paragraph.text, async text => { const next = { ...edit(), paragraphs: { ...edit().paragraphs, [paragraph.id]: text } }; await client.host.file(request("draft", { edit: next })); await props.reopen() }) })))
    } else if (props.view.kind === "table" || props.view.kind === "workbook") {
      const sheets = props.view.sheets
      const pickCell = (sheetID?: number) => props.prompt("Cell address", props.view.kind === "table" ? "1:1" : "A1", address => {
        const key = sheetID === undefined ? address.split(":").map(n => Number(n) - 1).join(":") : `${sheetID}!${address.toUpperCase()}`
        const cell = sheets?.find(s => s.id === sheetID)?.cells.find(c => c.address === address.toUpperCase())
        const previous = edit().cells?.[key] ?? (sheetID === undefined ? (() => { const [r, c] = key.split(":").map(Number); return props.view.rows?.[r]?.[c] })() : cell?.formula ? `=${cell.formula}` : cell?.value) ?? ""
        const cellAddress = sheetID === undefined ? (() => { const [row, col] = key.split(":").map(Number); return `${columnName(col)}${row + 1}` })() : address.toUpperCase()
        client.fileSelections.set(props.view.id, { kind: "cell", address: cellAddress, sheet: sheetID })
        props.prompt("Edit cell " + address, previous, async value => { const next = { ...edit(), cells: { ...edit().cells, [key]: value } }; await client.host.file(request("draft", { edit: next })); await props.reopen() }, "Start a spreadsheet formula with =. Existing formulas and styles are preserved; this editor does not recalculate formulas.")
      }, props.view.kind === "table" ? "Use one-based row:column, e.g. 2:3." : "Use an Excel cell address, e.g. B7.")
      if (sheets) props.menu("Choose worksheet", sheets.map(sheet => ({ title: sheet.name, disabled: sheet.protected, run: () => pickCell(sheet.id) })))
      else pickCell()
    }
  }
  function versions() {
    const selected = selection()
    props.menu("Saved file versions", props.view.versions.map((version, index) => ({ title: `Version ${index + 1} · ${version.author}`, description: `${version.at} · ${version.bytes} bytes · ${version.hash}`, run: () => props.menu("Selected immutable version", [
      { title: "Open this version", run: async () => { await retain(); await props.reopen(version.id) } },
      { title: "Restore as a new working version", run: async () => { await retain(); await client.host.file(request("restore", { version: version.id })); setDirty(false); await props.reopen() } },
      { title: "Compare with current", run: async () => {
        const comparison = await client.host.file({ ...request("compare"), action: "compare", base: version.id, version: props.view.current })
        dialog.replace(() => <DialogLayout header={<DialogHeader title="Saved version comparison" />}><scrollbox height={Math.max(8, dimensions().height - 15)}><For each={comparison.notices}>{line => <text>{literal(line)}</text>}</For><Show when={comparison.truncated}><text>Partial comparison: retained files remain complete.</text></Show><For each={comparison.changes}>{change => <box><text><b>{literal(change.location)}</b></text><text fg={theme.text.feedback.error.base}>− {literal(change.before)}</text><text fg={theme.accent}>+ {literal(change.after)}</text></box>}</For></scrollbox></DialogLayout>, undefined, { size: "xlarge" })
      } },
      { title: "Export exact selected bytes", run: () => props.prompt("Export destination", resolve(props.view.name), async path => {
        const destination = await client.host.reviewExport(path)
        if (!destination.exists) { await client.host.exportFile(request("export", { version: version.id }), destination.path); return }
        dialog.replace(() => <DialogConfirm title="Replace existing export?" message={literal(`${destination.path}\n${destination.bytes} bytes · modified ${destination.modified}\n\nReplace this reviewed file with the exact selected saved version?`)} label={{ confirm: "Replace file" }} onConfirm={() => attempt(client.host.exportFile(request("export", { version: version.id }), destination.path, destination.token))} />, undefined, { size: "xlarge" })
      }, "Choose a file path. Existing files require a separate replacement review.") },
      { title: "Ask Gator about this version", disabled: dirty(), run: async () => {
        const view = await client.host.file(request("open", { version: version.id })), selectedText = selectedContent(view, selected)
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
    { bind: "f6", title: "Select saved content for Ask Gator", run: selectContent },
    { bind: "f7", title: "Choose displayed worksheet", enabled: !!props.view.sheets, run: () => props.menu("Display worksheet", (props.view.sheets ?? []).map(sheet => ({ title: sheet.name, run: async () => { client.filePositions.set(props.view.id, { sheet: sheet.id, row: 0, col: 0 }); await props.reopen(props.view.revision) } }))) },
    { bind: "ctrl+down", title: "Next table rows", enabled: grid, run: () => setPosition(p => ({ ...p, row: Math.min(Math.floor(Math.max(0, size().rows - 1) / 50), p.row + 1) })) },
    { bind: "ctrl+up", title: "Previous table rows", enabled: grid, run: () => setPosition(p => ({ ...p, row: Math.max(0, p.row - 1) })) },
    { bind: "ctrl+right", title: "Next table columns", enabled: grid, run: () => setPosition(p => ({ ...p, col: Math.min(Math.floor(Math.max(0, size().cols - 1) / 20), p.col + 1) })) },
    { bind: "ctrl+left", title: "Previous table columns", enabled: grid, run: () => setPosition(p => ({ ...p, col: Math.max(0, p.col - 1) })) },
    { bind: "ctrl+right", title: "Next PDF page", enabled: props.view.kind === "pdf", run: () => { setPage(p => Math.min(pages(), p + 1)) } },
    { bind: "ctrl+left", title: "Previous PDF page", enabled: props.view.kind === "pdf", run: () => { setPage(p => Math.max(1, p - 1)) } },
  ] }))
  return <DialogLayout header={<DialogHeader title={`${literal(props.view.name)}${dirty() ? " · Unsaved" : ""}`} onClose={close} />}>
    <box id={`file-editor-${props.view.revision}`} ref={node => { body = node }} height={Math.max(8, dimensions().height - 14)} minHeight={0}>
      <Show when={props.view.kind === "text" || props.view.kind === "markdown"} fallback={
        <Show when={image()} fallback={<scrollbox flexGrow={1}>
          <Show when={grid()}><box id="file-grid-page"><text fg={theme.accent}>{literal(sheet()?.name || props.view.name)}{sheet()?.protected ? " · Protected" : ""} · Rows {position().row * 50 + 1}–{Math.min(size().rows, (position().row + 1) * 50)} of {size().rows} · Columns {columnName(position().col * 20)}–{columnName(Math.min(size().cols, (position().col + 1) * 20) - 1)}</text>
            <Show when={sheet()?.merges.length}><text>Merged cells: {sheet()?.merges.join(", ")}</text></Show>
            <For each={Array.from({ length: Math.min(50, Math.max(0, size().rows - position().row * 50)) }, (_, offset) => position().row * 50 + offset)}>{row => <text>{row + 1}: {Array.from({ length: Math.min(20, Math.max(0, size().cols - position().col * 20)) }, (_, offset) => { const col = position().col * 20 + offset, value = literal(gridValue(row, col)); return `${columnName(col)}${row + 1}=${value.slice(0, 160)}${value.length > 160 ? "…" : ""}` }).join(" │ ")}</text>}</For>
            <text fg={theme.text.muted}>Ctrl+↑/↓ rows · Ctrl+←/→ columns · F7 worksheet · F3 edits any cell, including a new row or column.</text>
          </box></Show>
          <For each={paragraphs()}>{paragraph => <box marginBottom={1}><text fg={theme.text.muted}>{literal(paragraph.id)} · {literal(paragraph.style)}</text><text>{literal(edit().paragraphs?.[paragraph.id] ?? paragraph.text)}</text><For each={paragraph.images}>{image => <image source={image} height={8} width="100%" fit="fit" />}</For></box>}</For>
        </scrollbox>}>{bytes => <image id="file-visual-preview" source={bytes()} flexGrow={1} width="100%" fit="fit" onError={error => setNotice(String(error))} />}</Show>
      }>
        <textarea id="file-text-editor" ref={node => { textarea = node; if (!editable()) node.traits = { suspend: true } }} focused={editable()} initialValue={edit().text ?? props.view.text ?? ""} flexGrow={1} wrapMode="word" textColor={theme.text.base}
          onContentChange={() => { if (textarea && editable() && textarea.plainText !== (edit().text ?? props.view.text ?? "")) change({ text: textarea.plainText }) }} />
      </Show>
    </box>
    <Show when={props.view.kind === "pdf"}><text>PDF page {page()} of {pages()}{busy() ? " · Rendering…" : ""} · Ctrl+←/→ pages</text></Show>
    <text fg={theme.text.muted}>{literal(notice())}</text>
    <text fg={theme.text.muted}>Ctrl+S save · F3 edit Office/cell · F2 versions/export/Ask Gator · F6 select content · F4 discard · Esc close</text>
  </DialogLayout>
}
