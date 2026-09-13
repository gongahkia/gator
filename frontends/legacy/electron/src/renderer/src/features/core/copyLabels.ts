export function copiedTitle(title: string, fallback: string): string {
  const base = title.trim() || fallback;
  const match = /^(.*) \(copy(?: (\d+))?\)$/.exec(base);

  if (!match) {
    return `${base} (copy)`;
  }

  const next = match[2] ? Number.parseInt(match[2], 10) + 1 : 2;
  return `${match[1]} (copy ${next})`;
}
