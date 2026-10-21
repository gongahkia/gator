import { useEffect, useRef, useState, type RefObject } from "react";

// Native text ranges leave React, Markdown, copying, and the full transcript
// intact. Literal matching cannot inject markup or execute a regular expression.
export function useConversationSearch(root: RefObject<HTMLDivElement | null>, query: string | null, identity: string, enabled: boolean) {
  const ranges = useRef<Range[]>([]);
  const position = useRef(0);
  const [count, setCount] = useState(0);
  const [active, setActive] = useState(0);
  const capped = useRef(false);
  const show = (index: number, scroll: boolean) => {
    const range = ranges.current[index];
    if (!range) return;
    position.current = index; setActive(index);
    CSS.highlights.set("gator-search-active", new Highlight(range));
    if (scroll && root.current) {
      let parent = range.startContainer.parentElement;
      while (parent && parent !== root.current) { if (parent instanceof HTMLDetailsElement) parent.open = true; parent = parent.parentElement; }
      const rect = range.getBoundingClientRect(), viewport = root.current.getBoundingClientRect();
      root.current.scrollBy({ top: rect.top - viewport.top - viewport.height / 2 + rect.height / 2 });
    }
  };
  useEffect(() => {
    ranges.current = []; position.current = 0; setActive(0); setCount(0);
    CSS.highlights.delete("gator-search"); CSS.highlights.delete("gator-search-active");
    const container = root.current;
    if (!enabled || !query || !container) return;
    let frame = 0;
    const expression = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "giu");
    const rebuild = (scroll: boolean) => {
      const found: Range[] = [];
      for (const message of container.querySelectorAll(".message-content")) {
        const walker = document.createTreeWalker(message, NodeFilter.SHOW_TEXT, { acceptNode(node) {
          return node.parentElement?.closest('button, [aria-hidden="true"], .visually-hidden, .message-byline') ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT;
        } });
        const nodes: { node: Node; start: number; end: number }[] = [];
        let text = "", node: Node | null;
        while ((node = walker.nextNode())) { const start = text.length; text += node.textContent; nodes.push({ node, start, end: text.length }); }
        for (const match of text.matchAll(expression)) {
          const start = match.index, end = start + match[0].length;
          const first = nodes.find((item) => item.end > start), last = nodes.find((item) => item.end >= end);
          if (first && last) { const range = document.createRange(); range.setStart(first.node, start - first.start); range.setEnd(last.node, end - last.start); found.push(range); }
          if (found.length >= 1000) break;
        }
        if (found.length >= 1000) break;
      }
      capped.current = found.length >= 1000;
      ranges.current = found; setCount(found.length);
      CSS.highlights.set("gator-search", new Highlight(...found));
      CSS.highlights.delete("gator-search-active");
      if (found.length) show(Math.min(position.current, found.length - 1), scroll);
    };
    rebuild(true);
    const observer = new MutationObserver((changes) => {
      if (!changes.some((change) => {
        const target = change.target instanceof Element ? change.target : change.target.parentElement;
        return target?.closest(".message-content") && !target.closest("button, .visually-hidden") || [...change.addedNodes].some((node) => node instanceof Element && (node.matches(".message") || node.querySelector(".message-content")));
      })) return;
      cancelAnimationFrame(frame); frame = requestAnimationFrame(() => rebuild(false));
    });
    observer.observe(container, { childList: true, characterData: true, subtree: true });
    return () => { observer.disconnect(); cancelAnimationFrame(frame); CSS.highlights.delete("gator-search"); CSS.highlights.delete("gator-search-active"); };
  }, [root, query, identity, enabled]);
  const move = (delta: number) => { if (ranges.current.length) show((position.current + delta + ranges.current.length) % ranges.current.length, true); };
  return { count, active, capped: capped.current, move };
}
