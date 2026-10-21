export function workspaceName(source: string, sources: string[] = []) {
  const parts = source.split(/[\\/]/).filter(Boolean);
  const other = [...new Set(sources)].filter((item) => item !== source).map((item) => item.split(/[\\/]/).filter(Boolean));
  for (let depth = 1; depth <= parts.length; depth++) {
    const label = parts.slice(-depth).join("/");
    if (!other.some((candidate) => candidate.slice(-depth).join("/") === label)) return label;
  }
  return source || "Workspace";
}

export function shortDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "" : date.toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}
