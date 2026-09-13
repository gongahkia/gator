export function formatDateTime(value: string | null | undefined): string {
  if (!value) {
    return "Never";
  }

  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(date);
}

export function formatTime(value: string): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    timeStyle: "medium"
  }).format(date);
}

export function durationText(value: number | undefined): string {
  return value === undefined ? "No sample yet" : `${Math.round(value * 10) / 10} ms`;
}

export function selectionText(
  selections: Array<{ selected: boolean }> | undefined,
  fallbackTotal: number
): string {
  if (!selections || selections.length === 0) {
    return fallbackTotal === 0 ? "Not loaded" : `0 of ${fallbackTotal}`;
  }

  return `${selections.filter((selection) => selection.selected).length} of ${selections.length}`;
}

export function operationLabel(operation: string): string {
  return operation
    .split(".")
    .map((part) => `${part[0]?.toUpperCase() ?? ""}${part.slice(1)}`)
    .join(" ");
}
