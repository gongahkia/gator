export type SourcePreviewRequest = { context: string; run: string; path: string; start?: number; end?: number };
export type SourceRecord = {id: string; locator: string; kind: "file" | "web" | "connected" | "attachment" | "unsupported"; label: string; captured_at?: string; retained: boolean; original_url?: string};
export type SourceCatalog = {run: string; references: SourceRecord[]; truncated: boolean};
export type SourcePreview = Partial<SourceRecord> & {
  run: string; path: string; sha256: string; text: string;
  firstLine: number; totalLines: number; truncated: boolean;
  cited?: { start: number; end: number };
  reads: { start: number; end: number; truncated: boolean; startColumn?: number; endColumn?: number }[];
  observed_preview?: string;
  notice?: string;
};
