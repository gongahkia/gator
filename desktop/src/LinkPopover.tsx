import { createContext, useContext, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { ArrowUpRight, Link2, Pencil, X } from "lucide-react";
import { markdownLink, safeWebLink } from "./draftLinks";

export const LinkDraftContext = createContext<((markdown: string) => void) | undefined>(undefined);

export function LinkPopover({ url, label, children, change, disabled = false }: { url: string; label: string; children?: ReactNode; change?(label: string, url: string): void; disabled?: boolean }) {
  const stage = useContext(LinkDraftContext);
  const [mode, setMode] = useState<"menu" | "text" | "link">(), [text, setText] = useState(label), [href, setHref] = useState(url), [issue, setIssue] = useState("");
  const trigger = useRef<HTMLAnchorElement>(null), menu = useRef<HTMLDivElement>(null), initialScope = useRef(stage);
  const [position, setPosition] = useState({ left: 12, top: 12 });
  const close = (restore = false) => { setMode(undefined); if (restore) trigger.current?.focus(); };
  useEffect(() => { setMode(undefined); }, [url, stage, disabled]);
  useLayoutEffect(() => {
    if (!mode) return;
    const rect = trigger.current?.getBoundingClientRect(), box = menu.current?.getBoundingClientRect(); if (!rect || !box) return;
    setPosition({ left: Math.max(12, Math.min(rect.left, innerWidth - box.width - 12)), top: rect.top > box.height + 16 ? rect.top - box.height - 8 : Math.min(rect.bottom + 8, innerHeight - box.height - 12) });
    (menu.current?.querySelector<HTMLInputElement>("input") ?? menu.current?.querySelector<HTMLButtonElement>("button"))?.focus();
  }, [mode]);
  useEffect(() => {
    if (!mode) return;
    const outside = (event: PointerEvent) => { if (!menu.current?.contains(event.target as Node) && !trigger.current?.contains(event.target as Node)) close(); };
    const dismiss = () => close();
    document.addEventListener("pointerdown", outside); window.addEventListener("resize", dismiss); window.addEventListener("scroll", dismiss, true);
    return () => { document.removeEventListener("pointerdown", outside); window.removeEventListener("resize", dismiss); window.removeEventListener("scroll", dismiss, true); };
  }, [mode]);
  const editable = !!change || !!stage;
  return <><a ref={trigger} className="editable-link" href={safeWebLink(url) ? url : undefined} aria-haspopup="dialog" aria-expanded={!!mode} onClick={event => { event.preventDefault(); if (disabled || !safeWebLink(url)) return; initialScope.current = stage; setText(label); setHref(url); setIssue(""); setMode(mode ? undefined : "menu"); }}>{children ?? label}</a>
    {mode && createPortal(<div ref={menu} className="link-popover" role="dialog" aria-label="Link options" style={position} onKeyDown={event => { event.stopPropagation(); if (event.key === "Escape") { event.preventDefault(); close(true); } if (event.key === "Tab") { const buttons = [...(menu.current?.querySelectorAll<HTMLElement>("button, input") ?? [])]; if (event.shiftKey && document.activeElement === buttons[0]) { event.preventDefault(); buttons.at(-1)?.focus(); } else if (!event.shiftKey && document.activeElement === buttons.at(-1)) { event.preventDefault(); buttons[0]?.focus(); } } }}>
      {mode === "menu" ? <div className="link-menu-actions"><button onClick={() => { void window.gator?.openLink(url).catch(error => setIssue(error instanceof Error ? error.message : "This link could not be opened.")); }}><ArrowUpRight size={16} />Open link</button>{editable && <><button onClick={() => setMode("text")}><Pencil size={16} />Edit text</button><button onClick={() => setMode("link")}><Link2 size={16} />Edit link</button></>}<button className="icon-button" aria-label="Close link options" onClick={() => close(true)}><X size={15} /></button></div> : <form onSubmit={event => { event.preventDefault(); try { if (initialScope.current !== stage) throw new Error("The conversation changed. Reopen this link before editing it."); const markdown = markdownLink(text, href); if (change) change(text, href); else stage?.(markdown); close(true); } catch (error) { setIssue(error instanceof Error ? error.message : "The link could not be changed."); } }}>
        <label>{mode === "text" ? "Link text" : "Link address"}<input aria-label={mode === "text" ? "Link text" : "Link address"} value={mode === "text" ? text : href} maxLength={mode === "text" ? 500 : 4096} onChange={event => { if (mode === "text") setText(event.target.value); else { const value = event.target.value; setHref(value); if (text === url) setText(value); } }} /></label>
        {!change && <p>Stage an edited copy in your draft. Sent messages stay unchanged.</p>}
        <footer><button type="button" className="small-button" onClick={() => setMode("menu")}>Back</button><button className="small-button" type="submit">{change ? "Save link" : "Add to draft"}</button></footer>
      </form>}
      {issue && <p className="composer-error" role="alert">{issue}</p>}
    </div>, document.querySelector(".app-shell") ?? document.body)}
  </>;
}
