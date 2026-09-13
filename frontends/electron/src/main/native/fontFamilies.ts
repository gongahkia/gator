export function normalizeFontFamilies(families: readonly string[]): string[] {
  const unique = new Set<string>();

  for (const family of families) {
    const trimmed = family.trim();

    if (trimmed.length > 0 && trimmed.length <= 120) {
      unique.add(trimmed);
    }
  }

  return [...unique].sort((left, right) => left.localeCompare(right)).slice(0, 2_000);
}
