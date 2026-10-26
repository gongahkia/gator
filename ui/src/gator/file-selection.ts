import type { FileView, FileParagraph } from "@gator/application-host/contracts/fileProtocol"
export type FileSelection = { kind: "quote"; text: string; revision: string } | { kind: "cell"; address: string; sheet?: number } | { kind: "paragraph"; id: string }
export const columnName = (index: number): string => index < 26 ? String.fromCharCode(65 + index) : columnName(Math.floor(index / 26) - 1) + String.fromCharCode(65 + index % 26)
export const cellCoordinates = (address: string) => {
  const match = /^([A-Z]{1,3})([1-9]\d{0,6})$/.exec(address.toUpperCase())
  if (!match) throw new Error("Choose an Excel cell address such as B7.")
  const col = Array.from(match[1]).reduce((value, letter) => value * 26 + letter.charCodeAt(0) - 64, 0) - 1, row = Number(match[2]) - 1
  if (col >= 16384 || row >= 1048576) throw new Error("This address exceeds spreadsheet limits.")
  return { row, col }
}
export function fileParagraphs(view: FileView): FileParagraph[] {
  return view.blocks?.flatMap(block => block.type === "paragraph" ? [block] : block.rows.flatMap(row => row.flatMap(cell => cell))) ?? view.slides?.flatMap(slide => slide.shapes.flatMap(shape => shape.paragraphs)) ?? []
}
export function selectedContent(view: FileView, selection?: FileSelection) {
  if (!selection) return ""
  if (selection.kind === "quote") {
    if (selection.revision !== view.revision) throw new Error("Open the selected saved version and select its text before asking about that selection.")
    return selection.text.slice(0, 4000)
  }
  if (selection.kind === "paragraph") {
    const paragraph = fileParagraphs(view).find(paragraph => paragraph.id === selection.id)
    if (!paragraph) throw new Error("This saved version no longer contains the selected paragraph.")
    return paragraph.text.slice(0, 4000)
  }
  const { row, col } = cellCoordinates(selection.address), sheet = view.sheets?.find(sheet => sheet.id === selection.sheet)
  if (selection.sheet !== undefined && !sheet) throw new Error("This saved version no longer contains the selected worksheet.")
  const cell = sheet?.cells.find(cell => cell.address === selection.address.toUpperCase())
  const value = sheet ? cell?.formula ? `=${cell.formula}` : cell?.value ?? "" : view.rows?.[row]?.[col] ?? ""
  return `${sheet?.name || view.name} · ${selection.address.toUpperCase()}: ${value}`.slice(0, 4000)
}
