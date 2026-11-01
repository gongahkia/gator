export type CodePreviewRequest = { context: string; run: string; candidate: string };
export type CodePreview = { run: string; candidate: string; sha256: string; status: string; changedPaths: string[]; patch: string; truncated: boolean; verification: { argv: string[]; passed: boolean }[] };
