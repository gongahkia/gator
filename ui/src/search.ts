import type { BackendResult } from "./result.js";
import { bundleResult, type BackendBundle } from "./bundle.js";

export type SearchMessage = { text: string; result?: BackendResult; bundle?: BackendBundle };

function resultText(result: BackendResult): string[] {
  return [
    result.title, result.summary, result.markdown, result.footer,
    ...(result.fields ?? []).flatMap((field) => [field.label, field.value]),
    ...(result.items ?? []).flatMap((item) => [item.label, item.detail, item.state]),
    ...(result.lines ?? []),
    ...(result.sections ?? []).flatMap((section) => [section.title, section.summary, section.markdown,
      ...(section.fields ?? []).flatMap((field) => [field.label, field.value]),
      ...(section.items ?? []).flatMap((item) => [item.label, item.detail, item.state]),
      ...(section.lines ?? []),
    ]),
    result.preview === undefined ? "" : JSON.stringify(result.preview),
  ].filter((value): value is string => !!value);
}

export function searchableMessage(message: SearchMessage): string {
  const bundle = message.bundle;
  if (message.result) return resultText(message.result).join("\n");
  if (bundle) return resultText(bundleResult(bundle)).join("\n");
  return message.text;
}

export type MessageMatch = { index: number; offset: number; length: number };

export function searchMessages(messages: SearchMessage[], query: string): MessageMatch[] {
  const needle = query.trim().toLocaleLowerCase();
  if (!needle) return [];
  return messages.flatMap((message, index) => {
    const content = searchableMessage(message).toLocaleLowerCase();
    const matches: MessageMatch[] = [];
    let offset = content.indexOf(needle);
    while (offset >= 0) {
      matches.push({ index, offset, length: needle.length });
      offset = content.indexOf(needle, offset + Math.max(1, needle.length));
    }
    return matches;
  });
}
