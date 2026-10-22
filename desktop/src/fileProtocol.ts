export type FileReference = { kind: "upload"; id: string } | { kind: "output"; bundle: string; path: string };
export type FileEdit = { text?: string; paragraphs?: Record<string, string>; cells?: Record<string, string> };
export type FileRequest = { action: "open" | "save" | "draft" | "discard" | "restore" | "attach" | "export"; context: string; ref: FileReference; expected?: string; version?: string; edit?: FileEdit };
export type FileParagraph = { id: string; text: string; style?: string; align?: string; images?: string[]; runs: { text: string; bold?: boolean; italic?: boolean; underline?: boolean; color?: string; size?: number }[] };
export type WordBlock = ({ type: "paragraph" } & FileParagraph) | { type: "table"; rows: FileParagraph[][][] };
export type FileSheet = { id: number; name: string; protected: boolean; maxRow: number; maxCol: number; merges: string[]; cells: { address: string; row: number; col: number; value: string; formula: string }[] };
export type FileSlide = { id: number; shapes: { id: string; x: number; y: number; width: number; height: number; image?: string; fill?: string; paragraphs: FileParagraph[] }[] };
export type FileView = {
  id: string; name: string; revision: string; current: string; versions: { id: string; hash: string; bytes: number; at: string; author: string }[];
  kind: "text" | "markdown" | "table" | "word" | "workbook" | "slides" | "image" | "pdf" | "unsupported";
  editable: boolean; notice?: string; draft?: FileEdit; staleDraft?: boolean; text?: string; language?: string; rows?: string[][];
  blocks?: WordBlock[]; sheets?: FileSheet[]; slides?: FileSlide[]; width?: number; height?: number; data?: string;
};
