export type SourcePreviewRequest = { context: string; run: string; path: string; start?: number; end?: number };
export type SourcePreview = {
  run: string; path: string; sha256: string; text: string;
  firstLine: number; totalLines: number; truncated: boolean;
  cited?: { start: number; end: number };
  reads: { start: number; end: number; truncated: boolean; endColumn?: number }[];
  notice?: string;
};
