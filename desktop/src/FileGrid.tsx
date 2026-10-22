import { useLayoutEffect, useRef, useState, type KeyboardEvent } from "react";
import { ChevronLeft, ChevronRight, Plus } from "lucide-react";
import type { FileView } from "./fileProtocol";

const columnName = (value: number): string => value <= 26 ? String.fromCharCode(64 + value) : columnName(Math.floor((value - 1) / 26)) + String.fromCharCode(65 + (value - 1) % 26);
const columnIndex = (value: string) => [...value].reduce((n, char) => n * 26 + char.charCodeAt(0) - 64, 0) - 1;
export function FileGrid({ view, edits, editable, change, select }: {
  view: FileView; edits: Record<string, string>; editable: boolean;
  change(id: string, value: string, original: string, force?: boolean): void; select(value: string): void;
}) {
  const [sheetID, setSheetID] = useState(view.sheets?.[0]?.id ?? 0);
  const [page, setPage] = useState(0), [column, setColumn] = useState(0);
  const [active, setActive] = useState<[number, number]>();
  const grid = useRef<HTMLDivElement>(null), focusCell = useRef<string | undefined>(undefined);
  const sheet = view.sheets?.find(value => value.id === sheetID);
  const cells = new Map(sheet?.cells.map(cell => [cell.address, cell]) ?? []), rows = view.rows ?? [];
  let rowCount = Math.max(1, sheet?.maxRow ?? rows.length), colCount = Math.max(1, sheet?.maxCol ?? Math.max(0, ...rows.map(row => row.length)));
  for (const key of Object.keys(edits)) {
    const parts = sheet ? key.match(new RegExp(`^${sheetID}!([A-Z]+)(\\d+)$`)) : key.match(/^(\d+):(\d+)$/);
    if (!parts) continue;
    rowCount = Math.max(rowCount, sheet ? Number(parts[2]) : Number(parts[1]) + 1);
    colCount = Math.max(colCount, sheet ? columnIndex(parts[1]) + 1 : Number(parts[2]) + 1);
  }
  const firstRow = page * 50, firstCol = column * 20;
  const original = (row: number, col: number) => {
    const cell = cells.get(`${columnName(col + 1)}${row + 1}`);
    return sheet ? cell?.formula ? `=${cell.formula}` : cell?.value ?? "" : rows[row]?.[col] ?? "";
  };
  const id = (row: number, col: number) => sheet ? `${sheetID}!${columnName(col + 1)}${row + 1}` : `${row}:${col}`;
  const activate = (row: number, col: number) => {
    setActive([row, col]);
    select(`${sheet?.name || view.name} · ${columnName(col + 1)}${row + 1}: ${edits[id(row, col)] ?? original(row, col)}`);
  };
  const move = (row: number, col: number) => {
    if (row < 0 || col < 0 || row >= rowCount || col >= colCount) return;
    focusCell.current = id(row, col); setPage(Math.floor(row / 50)); setColumn(Math.floor(col / 20)); activate(row, col);
  };
  useLayoutEffect(() => {
    if (!focusCell.current) return;
    grid.current?.querySelector<HTMLElement>(`[data-cell="${CSS.escape(focusCell.current)}"]`)?.focus({ preventScroll: true });
    focusCell.current = undefined;
  });
  const navigate = (event: KeyboardEvent<HTMLInputElement>, row: number, col: number) => {
    const target = event.currentTarget;
    let next: [number, number] | undefined;
    if (event.key === "Enter" || event.key === "ArrowDown") next = [row + (event.shiftKey && event.key === "Enter" ? -1 : 1), col];
    else if (event.key === "ArrowUp") next = [row - 1, col];
    else if (event.key === "ArrowLeft" && target.selectionStart === 0 && target.selectionEnd === 0) next = [row, col - 1];
    else if (event.key === "ArrowRight" && target.selectionStart === target.value.length && target.selectionEnd === target.value.length) next = [row, col + 1];
    if (next) { event.preventDefault(); move(...next); }
  };
  const disabled = !editable || !!sheet?.protected;
  const activeID = active && id(...active), activeBase = active ? original(...active) : "";
  return <div ref={grid} className="file-grid">
    {view.sheets && <div className="file-sheet-tabs" aria-label="Workbook sheets">{view.sheets.map(value => <button key={value.id} aria-pressed={sheetID === value.id} onClick={() => { setSheetID(value.id); setPage(0); setColumn(0); setActive(undefined); }}>{value.name}{value.protected ? " · read only" : ""}</button>)}</div>}
    <div className="file-grid-location"><span>{active ? `${columnName(active[1] + 1)}${active[0] + 1}` : "Select a cell"}</span><span>Rows {firstRow + 1}–{Math.min(firstRow + 50, rowCount)} · {columnName(firstCol + 1)}–{columnName(Math.min(firstCol + 20, colCount))}</span></div>
    {activeID && <label className="file-formula-bar"><span>{sheet ? "fx" : "Value"}</span><input aria-label={sheet ? "Cell value or formula" : "Cell value"} readOnly={disabled} value={edits[activeID] ?? activeBase} onChange={event => change(activeID, event.target.value, activeBase)} /></label>}
    <div className="file-grid-scroll"><table><thead><tr><th aria-label="Row" />{Array.from({ length: Math.min(20, Math.max(1, colCount - firstCol)) }, (_, i) => <th key={i} scope="col">{columnName(firstCol + i + 1)}</th>)}</tr></thead><tbody>{Array.from({ length: Math.min(50, Math.max(0, rowCount - firstRow)) }, (_, r) => {
      const row = firstRow + r;
      return <tr key={row}><th scope="row">{row + 1}</th>{Array.from({ length: Math.min(20, Math.max(1, colCount - firstCol)) }, (_, c) => {
        const col = firstCol + c, key = id(row, col), base = original(row, col), value = edits[key] ?? base;
        return <td key={key} data-edited={edits[key] !== undefined}>{disabled ? <button data-cell={key} className="file-cell-preview" onFocus={() => activate(row, col)} onClick={() => activate(row, col)}>{sheet && edits[key] === undefined && cells.get(`${columnName(col + 1)}${row + 1}`)?.formula ? cells.get(`${columnName(col + 1)}${row + 1}`)?.value || base : value}</button> : <input data-cell={key} aria-label={`${sheet?.name || "Table"} cell ${columnName(col + 1)}${row + 1}`} value={value} onFocus={() => activate(row, col)} onChange={event => change(key, event.target.value, base)} onKeyDown={event => navigate(event, row, col)} />}</td>;
      })}</tr>;
    })}</tbody></table></div>
    <footer><button className="icon-button" aria-label="Previous table rows" disabled={!page} onClick={() => setPage(value => value - 1)}><ChevronLeft size={16} /></button><button className="icon-button" aria-label="Next table rows" disabled={firstRow + 50 >= rowCount} onClick={() => setPage(value => value + 1)}><ChevronRight size={16} /></button>{colCount > 20 && <><button className="small-button" disabled={!column} onClick={() => setColumn(value => value - 1)}>Earlier columns</button><button className="small-button" disabled={firstCol + 20 >= colCount} onClick={() => setColumn(value => value + 1)}>More columns</button></>}{editable && !sheet?.protected && <><button className="small-button" disabled={rowCount >= (sheet ? 1048576 : 10000)} onClick={() => { change(id(rowCount, 0), "", "", true); move(rowCount - 1, 0); setPage(Math.floor(rowCount / 50)); }}><Plus size={14} />Row</button><button className="small-button" disabled={colCount >= (sheet ? 16384 : 256)} onClick={() => { change(id(0, colCount), "", "", true); setColumn(Math.floor(colCount / 20)); }}><Plus size={14} />Column</button></>}</footer>
  </div>;
}
