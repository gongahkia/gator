const MAX_HYDRATION_ERROR_LENGTH = 500;

const secretAssignmentPattern =
  /\b[A-Za-z0-9_-]*(?:access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|mcp[_-]?token|bearer[_-]?token|api[_-]?key|password|credential|secret|token)[A-Za-z0-9_-]*\b\s*([:=])\s*["']?[^"',\s)}\]]+/gi;
const bearerPattern = /\bBearer\s+[A-Za-z0-9._~+/=-]+/gi;
const emailPattern = /\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/gi;
const macUserPathPattern = /\/Users\/[^/\s]+/g;
const windowsUserPathPattern = /[A-Z]:\\Users\\[^\\\s]+/gi;

export function safeHydrationErrorMessage(error: unknown): string {
  return sanitizeHydrationErrorMessage(
    error instanceof Error ? error.message : "Background hydration failed."
  );
}

export function sanitizeHydrationErrorMessage(message: string): string {
  return message
    .replace(secretAssignmentPattern, (match: string, separator: string) => {
      const key = match.slice(0, match.indexOf(separator)).trim();
      return `${key}${separator}[redacted]`;
    })
    .replace(bearerPattern, "Bearer [redacted]")
    .replace(emailPattern, "[redacted]")
    .replace(macUserPathPattern, "~")
    .replace(windowsUserPathPattern, "~")
    .replace(/[\r\n]+/g, " ")
    .trim()
    .slice(0, MAX_HYDRATION_ERROR_LENGTH);
}
