export const uploadLimits: Readonly<{ count: number; file: number; image: number; total: number }>;
export function attachmentLimit(name: string): number;
export const attachmentLimitsHint: string;
export const attachmentLimitsError: string;
