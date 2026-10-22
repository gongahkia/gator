import { Fragment } from "react";

export function chatDateGroup(value?: string, now = new Date()) {
  const date = new Date(value || "");
  if (!Number.isFinite(date.valueOf())) return "Undated";
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today); yesterday.setDate(today.getDate() - 1);
  const week = new Date(today); week.setDate(today.getDate() - 7);
  return date >= today ? "Today" : date >= yesterday ? "Yesterday" : date >= week ? "Previous 7 days" : "Older";
}

export function datedChats<T>(items: T[], date: (item: T) => string | undefined) {
  const groups = new Map<string, T[]>();
  for (const item of items) { const group = chatDateGroup(date(item)); groups.set(group, [...groups.get(group) ?? [], item]); }
  return [...groups];
}

// Literal matching: user input never becomes a regular expression or markup.
export function Highlight({ text, query }: { text: string; query: string }) {
  const term = query.trim().toLocaleLowerCase();
  if (!term) return <>{text}</>;
  const source = text.toLocaleLowerCase();
  const parts = [];
  let offset = 0, match = source.indexOf(term);
  while (match !== -1) {
    parts.push(<Fragment key={offset}>{text.slice(offset, match)}<mark>{text.slice(match, match + term.length)}</mark></Fragment>);
    offset = match + term.length; match = source.indexOf(term, offset);
  }
  return <>{parts}{text.slice(offset)}</>;
}
