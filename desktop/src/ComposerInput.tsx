import { useState, type ComponentPropsWithoutRef, type RefObject } from "react";
import { Link2 } from "lucide-react";
import { draftLinks, markdownLink } from "./draftLinks";
import { LinkPopover } from "./LinkPopover";

// Keep the native textarea as the editing/IME/undo/selection authority. The
// matching text layer adds link hit targets without an HTML-backed draft store.
export function ComposerInput({ inputRef, value, setValue, ...props }: Omit<ComponentPropsWithoutRef<"textarea">, "value"> & { inputRef: RefObject<HTMLTextAreaElement | null>; value: string; setValue(value: string): void }) {
  const links = draftLinks(value), [scroll, setScroll] = useState({ top: 0, left: 0 }), [focused, setFocused] = useState(false);
  const leading = links[0]?.start === 0;
  const pieces = []; let offset = 0;
  for (const link of links) {
    pieces.push(<span aria-hidden="true" key={`before-${link.start}`}>{value.slice(offset, link.start)}</span>);
    pieces.push(<LinkPopover key={`${link.start}:${link.url}`} url={link.url} label={link.label} disabled={props.disabled} change={(label, url) => { if (value.slice(link.start, link.end) !== link.raw) throw new Error("The draft changed; reopen this link."); const next = value.slice(0, link.start) + markdownLink(label, url) + value.slice(link.end); if (new TextEncoder().encode(next).length > 128 * 1024) throw new Error("The edited draft exceeds 128 KiB."); setValue(next); }}><span>{!focused && link.raw.startsWith("[") ? link.label : link.raw}</span></LinkPopover>);
    offset = link.end;
  }
  pieces.push(<span aria-hidden="true" key="after">{value.slice(offset)}{value.endsWith("\n") ? " " : ""}</span>);
  return <div className="composer-input" data-links={!!links.length} data-leading-link={leading} data-focused={focused}>
    {leading && <Link2 className="composer-leading-link" size={15} aria-hidden="true" />}
    <textarea {...props} ref={inputRef} value={value} onFocus={event => { setFocused(true); props.onFocus?.(event); }} onBlur={event => { setFocused(false); props.onBlur?.(event); }} onScroll={event => setScroll({ top: event.currentTarget.scrollTop, left: event.currentTarget.scrollLeft })} />
    {!!links.length && <div className="composer-link-layer"><div className="composer-link-text" style={{ transform: `translate(${-scroll.left}px, ${-scroll.top}px)` }}>{pieces}</div></div>}
  </div>;
}
