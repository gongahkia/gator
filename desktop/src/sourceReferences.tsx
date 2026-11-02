import { createContext } from "react";
import type { SourceRecord } from "@gator/application-host/contracts/sourcePreview";

export type SourceReference = { run: string; path: string; start?: number; end?: number };
export type SourceSelection = SourceReference & { context: string; request: number };
export const SourceReferenceContext = createContext<{ inspectCode?(path:string,candidate?:string):void; codeAvailable?(path:string,candidate?:string):boolean; context?: string; references?: SourceRecord[]; run?: string; open?(reference: SourceReference): void }>({});

type Node = { type: string; value?: string; url?: string; children?: Node[] };
function citation(value: string, following = "") {
  if (/^evidence:[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(value) || /^connector:[a-zA-Z0-9_-]+\/[a-zA-Z0-9_-]+$/.test(value)) return `gator-source:${encodeURIComponent(value)}`;
  const match = value.match(/^(source\/[^\x00-\x1f]+?)(?::(\d+)(?:[-–—](\d+))?)?$/);
  if (!match) return;
  const range = following.match(/^\s*,?\s*lines?\s+(\d+)(?:\s*[-–—]\s*(\d+))?/i);
  const start = match[2] ?? range?.[1], end = match[3] ?? range?.[2] ?? start;
  return `gator-source:${encodeURIComponent(match[1])}${start ? `?start=${start}&end=${end}` : ""}`;
}
// Turn a cited workspace path into an application reference, never a file URL.
// The host still requires matching retained evidence in the answer's own run.
export function sourceReferences() {
  return (tree: Node) => {
    const visit = (node: Node) => {
      if (!node.children || ["link", "code", "html"].includes(node.type)) return;
      node.children = node.children.flatMap((child, index, children) => {
        if (child.type === "link" && child.url?.startsWith("gator-evidence:")) {
          const id = child.url.slice("gator-evidence:".length);
          if (/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(id)) child.url = `gator-source:${encodeURIComponent("evidence:"+id)}`;
        }
        if (child.type === "inlineCode" && child.value) {
          const url = citation(child.value, children[index + 1]?.value);
          if (url) return [{ type: "link", url, children: [child] }];
        }
        if (child.type === "text" && child.value) {
          const value = child.value, nodes: Node[] = []; let offset = 0;
          for (const match of value.matchAll(/\bsource\/[^\s`<>"',;)\]]+/g)) {
            if (match.index > offset) nodes.push({ type: "text", value: value.slice(offset, match.index) });
            const path = match[0].replace(/[.]+$/, "");
            nodes.push({ type: "link", url: citation(path, value.slice(match.index + path.length)), children: [{ type: "inlineCode", value: path }] });
            offset = match.index + path.length;
          }
          if (nodes.length) { if (offset < value.length) nodes.push({ type: "text", value: value.slice(offset) }); return nodes; }
        }
        visit(child); return [child];
      });
    };
    visit(tree);
  };
}

export function parseSourceReference(href: string) {
  const [encoded, query] = href.slice("gator-source:".length).split("?");
  const parameters = new URLSearchParams(query);
  return { path: decodeURIComponent(encoded), start: parameters.has("start") ? Number(parameters.get("start")) : undefined, end: parameters.has("end") ? Number(parameters.get("end")) : undefined };
}
