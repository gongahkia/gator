export function readLocalStorageStringArray(key: string): string[] {
  try {
    const value = window.localStorage?.getItem?.(key);
    const parsed = JSON.parse(value ?? "[]");

    return Array.isArray(parsed)
      ? parsed.filter((item): item is string => typeof item === "string")
      : [];
  } catch {
    return [];
  }
}

export function readLocalStorageNumberRecord(key: string): Record<string, number> {
  try {
    const value = window.localStorage?.getItem?.(key);
    const parsed = JSON.parse(value ?? "{}");

    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return {};
    }

    return Object.fromEntries(
      Object.entries(parsed).filter((entry): entry is [string, number] =>
        typeof entry[0] === "string" && typeof entry[1] === "number"
      )
    );
  } catch {
    return {};
  }
}

export function writeLocalStorageJSON(key: string, value: unknown): void {
  try {
    window.localStorage?.setItem?.(key, JSON.stringify(value));
  } catch {
    // Local storage is best-effort UI state only.
  }
}
