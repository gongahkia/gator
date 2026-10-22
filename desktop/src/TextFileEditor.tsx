import { useRef } from "react";
import { Bold, Heading2, Italic, List } from "lucide-react";

export function TextFileEditor({ name, value, markdown, change, select }: {
  name: string; value: string; markdown: boolean; change(value: string): void; select(value: string): void;
}) {
  const editor = useRef<HTMLTextAreaElement>(null);
  const format = (before: string, after = "") => {
    const target = editor.current;
    if (!target) return;
    const start = target.selectionStart, end = target.selectionEnd;
    const selected = value.slice(start, end), replacement = `${before}${selected}${after}`;
    change(value.slice(0, start) + replacement + value.slice(end));
    requestAnimationFrame(() => {
      target.focus({ preventScroll: true });
      target.setSelectionRange(start + before.length, end + before.length);
    });
  };
  return <div className="file-text-surface">
    {markdown && <div className="file-markdown-tools" role="toolbar" aria-label="Markdown formatting">
      <button className="icon-button" aria-label="Bold" title="Bold" onClick={() => format("**", "**")}><Bold size={16} /></button>
      <button className="icon-button" aria-label="Italic" title="Italic" onClick={() => format("_", "_")}><Italic size={16} /></button>
      <button className="icon-button" aria-label="Heading" title="Heading" onClick={() => format("\n## ")}><Heading2 size={16} /></button>
      <button className="icon-button" aria-label="List item" title="List item" onClick={() => format("\n- ")}><List size={16} /></button>
      <span>Markdown · preview to see formatting</span>
    </div>}
    <textarea ref={editor} className={`file-text-editor ${markdown ? "document-text-editor" : ""}`} aria-label={`Edit ${name}`} spellCheck={markdown} value={value} onChange={event => change(event.target.value)} onSelect={event => select(event.currentTarget.value.slice(event.currentTarget.selectionStart, event.currentTarget.selectionEnd).slice(0, 4000))} />
  </div>;
}
