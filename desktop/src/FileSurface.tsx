import { createContext, useContext, useEffect, useRef, useState, lazy, Suspense, type CSSProperties } from "react";
import { ChevronDown, ChevronLeft, ChevronRight, Download, FileText, History, MessageSquare, Save, X } from "lucide-react";
import { Markdown } from "./Result";
import { FileGrid } from "./FileGrid";
import { TextFileEditor } from "./TextFileEditor";
import type { FileEdit, FileParagraph, FileReference, FileRequest, FileView, WordBlock } from "./fileProtocol";
import type { UploadedFile } from "./bridge";

const PDFPreview = lazy(() => import("./PDFPreview"));
export const FileChatContext = createContext<{ context?: string; dirtyKeys?: Set<string>; blocked: boolean; useFile(file: UploadedFile, key: string, prompt?: string): void; dirty(key: string, value: boolean): void }>({ blocked: true, useFile: () => {}, dirty: () => {} });
const errorText = (error: unknown) => error instanceof Error ? error.message : "This file could not be opened. Try again.";
const keyOf = (ref: FileReference) => JSON.stringify(ref);
const empty = (edit: FileEdit) => edit.text === undefined && !Object.keys(edit.cells ?? {}).length && !Object.keys(edit.paragraphs ?? {}).length;

export function FileCard({ reference, name, caption, fallback }: { reference: FileReference; name: string; caption?: string; fallback?: string }) {
  const chat = useContext(FileChatContext), [open, setOpen] = useState(false), [view, setView] = useState<FileView>(), [edit, setEdit] = useState<FileEdit>({}), [mode, setMode] = useState<"preview" | "edit">("preview");
  const [busy, setBusy] = useState(false), [issue, setIssue] = useState(""), [status, setStatus] = useState(""), [selection, setSelection] = useState(""), [prompt, setPrompt] = useState(""), [asking, setAsking] = useState(false), [history, setHistory] = useState(false);
  const body = useRef<HTMLDivElement>(null), trigger = useRef<HTMLButtonElement>(null), epoch = useRef(0), owner = useRef(chat), draftWrite = useRef(Promise.resolve());
  owner.current = chat;
  const key = keyOf(reference), dirty = !empty(edit), historical = !!view && view.revision !== view.current;
  const request = <T = FileView>(action: FileRequest["action"], extra: Partial<FileRequest> = {}) => window.gator!.files<T>({ action, context: chat.context!, ref: reference, ...extra });
  useEffect(() => { chat.dirty(key, dirty); return () => chat.dirty(key, false); }, [key, dirty, chat.dirty]);
  useEffect(() => () => { epoch.current++; }, [chat.context]);
  useEffect(() => {
    if (!view || !dirty || historical || !chat.context) return;
    // Keep file edit drafts independently of message drafts, in order and on
    // every edit. Navigation guards remain until Save or Discard completes.
    const context = chat.context, expected = view.current;
    draftWrite.current = draftWrite.current.catch(() => {}).then(async () => { await window.gator!.files({ action: "draft", context, ref: reference, expected, edit }); }).catch(error => { setIssue(`File draft could not be retained: ${errorText(error)}`); });
  }, [edit, view?.current, historical, chat.context]);
  const load = async (version?: string) => {
    const token = ++epoch.current; setBusy(true); setIssue(""); setStatus("");
    try { const next = await request("open", { version }); if (token !== epoch.current) return; setView(next); setEdit(next.draft ?? {}); setMode(next.draft ? "edit" : "preview"); setSelection(""); }
    catch (error) { if (token === epoch.current) setIssue(errorText(error)); }
    finally { if (token === epoch.current) setBusy(false); }
  };
  const changeText = (text: string) => setEdit(current => ({ ...current, text: text === view?.text ? undefined : text }));
  const paragraph = (id: string, value: string, original: string) => setEdit(current => { const paragraphs = { ...current.paragraphs }; if (value === original) delete paragraphs[id]; else paragraphs[id] = value; return { ...current, paragraphs }; });
  const cell = (id: string, value: string, original: string, force = false) => setEdit(current => { const cells = { ...current.cells }; if (value === original && !force) delete cells[id]; else cells[id] = value; return { ...current, cells }; });
  const attach = async (revision: string, instruction?: string) => {
    const file = await request<UploadedFile>("attach", { version: revision });
    if (owner.current.context !== chat.context) throw new Error("Return to this conversation to use the saved file.");
    chat.useFile(file, key, instruction);
  };
  const save = async () => {
    if (!view || busy) return; setBusy(true); setIssue("");
    try {
      await draftWrite.current;
      const next = await request("save", { expected: view.current, edit }); setView(next); setEdit({}); setMode("preview"); trigger.current?.focus({ preventScroll: true }); setStatus("Saved a new version.");
      try { await attach(next.revision); setStatus("Saved · included in your next message"); } catch (error) { setIssue(`Saved. ${errorText(error)}`); }
    } catch (error) { setIssue(errorText(error)); }
    finally { setBusy(false); }
  };
  const discard = async () => {
    if (!view || busy) return; setBusy(true); setIssue("");
    try { await draftWrite.current; const next = await request("discard", { expected: view.staleDraft ?? view.current }); setView(next); setEdit(next.draft ?? {}); setMode("preview"); trigger.current?.focus({ preventScroll: true }); setStatus("Edits discarded."); }
    catch (error) { setIssue(errorText(error)); } finally { setBusy(false); }
  };
  const exportFile = async () => { setBusy(true); setIssue(""); try { await request("export", { version: view?.revision }); } catch (error) { setIssue(errorText(error)); } finally { setBusy(false); } };
  const ask = async () => {
    if (!view || !prompt.trim() || dirty || busy) return; setBusy(true); setIssue("");
    try { await attach(view.revision, `For ${view.name} (version ${view.versions.findIndex(version => version.id === view.revision) + 1})${selection ? `, focus on:\n${selection.slice(0,4000)}\n` : ":\n"}${prompt.trim()}`); setAsking(false); setPrompt(""); setStatus("File and request added to your message. Review before sending."); }
    catch (error) { setIssue(errorText(error)); } finally { setBusy(false); }
  };
  const restore = async () => {
    if (!view || !historical || dirty) return; setBusy(true); setIssue("");
    try { const next = await request("restore", { expected: view.current, version: view.revision }); setView(next); setEdit({}); setHistory(false); setStatus("Restored as a new version."); await attach(next.revision); }
    catch (error) { setIssue(errorText(error)); } finally { setBusy(false); }
  };
  const close = () => { if (dirty) { setIssue("Save or discard your edits before closing. Your file draft is retained."); return; } setOpen(false); trigger.current?.focus({ preventScroll: true }); };
  return <section className="inline-file" data-expanded={open}>
    <button ref={trigger} className="inline-file-card" aria-expanded={open} aria-label={`Open file ${name}`} onClick={() => { if (open) close(); else { setOpen(true); void load(); } }} disabled={!chat.context}>
      <FileText size={18} /><span><strong>{name}</strong><small>{caption || "Attached file"}</small></span><ChevronDown size={16} />
    </button>
    {open && <div className="inline-file-expanded" onKeyDown={event => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); if (asking) setAsking(false); else if (history) setHistory(false); else close(); }
      else if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") { event.preventDefault(); event.stopPropagation(); if (dirty) void save(); }
    }}>
      <header className="inline-file-toolbar">
        <div className="file-view-switch" aria-label="File view"><button aria-pressed={mode === "preview"} onClick={() => setMode("preview")}>Preview</button>{view?.editable && !historical && <button aria-pressed={mode === "edit"} disabled={busy || chat.blocked} onClick={() => setMode("edit")}>Edit</button>}</div>
        <span className="file-save-state">{dirty ? "Unsaved edits" : historical ? "Previous version" : view ? `Version ${view.versions.length}` : busy ? "Opening…" : ""}</span>
        {view && <button className="icon-button" aria-label={`Version history for ${name}`} title="Version history" aria-expanded={history} disabled={busy || dirty} onClick={() => setHistory(value => !value)}><History size={16} /></button>}
        <button className="icon-button" aria-label={`Export ${name}`} title="Save a copy" disabled={busy} onClick={() => void exportFile()}><Download size={16} /></button>
        <button className="icon-button" aria-label={`Close file ${name}`} title="Collapse file" onClick={close}><X size={16} /></button>
      </header>
      {history && view && <div className="file-version-list" aria-label="File versions">{[...view.versions].reverse().map((version, index) => <button key={version.id} aria-current={version.id === view.revision ? "true" : undefined} onClick={() => { void load(version.id); setHistory(false); }}><span>Version {view.versions.length - index} · {version.author}</span><time>{new Date(version.at).toLocaleString()}</time></button>)}</div>}
      {busy && !view && <p className="file-feedback" role="status">Opening file…</p>}
      {!view && fallback && <div className="file-body">{/\.md(?:own|arkdown)?$/i.test(name) ? <Markdown text={fallback} /> : <pre>{fallback}</pre>}<p className="file-feedback">Retained excerpt · full content is unavailable</p></div>}
      {view && <><div ref={body} className={`file-body file-kind-${view.kind}`} aria-label={`${name} ${mode}`} onMouseUp={() => { const selected = window.getSelection(); if (selected?.anchorNode && body.current?.contains(selected.anchorNode)) setSelection(selected.toString().slice(0,4000)); }}>
        {view.kind === "image" ? <img className="file-image" src={view.data} alt={name} /> : view.kind === "pdf" ? <Suspense fallback={<p className="file-feedback">Opening PDF…</p>}><PDFPreview data={view.data!} /></Suspense> : view.kind === "word" ? <WordDocument blocks={view.blocks!} edits={edit.paragraphs ?? {}} editable={mode === "edit" && !busy} revision={`${view.revision}:${mode}`} change={paragraph} /> : view.kind === "workbook" || view.kind === "table" ? <FileGrid key={view.revision} view={view} edits={edit.cells ?? {}} editable={mode === "edit" && !busy} change={cell} select={setSelection} /> : view.kind === "slides" ? <SlideDocument key={view.revision} view={view} edits={edit.paragraphs ?? {}} editable={mode === "edit" && !busy} revision={`${view.revision}:${mode}`} change={paragraph} /> : view.kind === "unsupported" ? <p className="file-feedback">{view.notice}</p> : mode === "edit" ? <TextFileEditor name={name} markdown={view.kind === "markdown"} value={edit.text ?? view.text ?? ""} change={changeText} select={setSelection} /> : view.kind === "markdown" ? <Markdown text={edit.text ?? view.text ?? ""} /> : <pre className="file-source">{edit.text ?? view.text}</pre>}
      </div>{view.notice && view.kind !== "unsupported" && <details className="file-format-note"><summary>Format support</summary><p>{view.notice}</p></details>}{view.staleDraft && <p className="file-feedback" role="status">An earlier edit draft belongs to a previous version. The latest saved file is shown.</p>}{view.staleDraft && <button className="small-button" disabled={busy} onClick={() => void discard()}>Discard earlier draft</button>}</>}
      {issue && <p className="file-feedback file-error" role="alert">{issue}</p>}{status && <p className="file-feedback" role="status">{status}</p>}
      <footer className="inline-file-footer">{view && <button className="small-button" disabled={busy || dirty || chat.blocked} onClick={() => setAsking(value => !value)}><MessageSquare size={15} />{selection ? "Ask Gator about selection" : "Ask Gator"}</button>}<div>{historical ? <button className="button" disabled={busy || chat.blocked} onClick={() => void restore()}>Restore this version</button> : dirty && <><button className="small-button" disabled={busy} onClick={() => void discard()}>Discard</button><button className="button file-save-button" disabled={busy || chat.blocked} onClick={() => void save()}><Save size={15} />{busy ? "Saving…" : "Save"}</button></>}</div></footer>
      {asking && <div className="file-revision-request">{selection && <blockquote>{selection.slice(0,500)}</blockquote>}<textarea autoFocus aria-label={`Request changes to ${name}`} placeholder="What would you like Gator to change?" value={prompt} onChange={event => setPrompt(event.target.value)} /><button className="button" disabled={busy || !prompt.trim() || dirty || chat.blocked} onClick={() => void ask()}>Add to message</button></div>}
    </div>}
  </section>;
}

function Paragraph({ value, initial, editable, change, label }: { value: FileParagraph; initial: string; editable: boolean; change(value: string): void; label: string }) {
  const [firstValue] = useState(initial);
  return <div className="file-paragraph-block"><div className="file-paragraph" data-heading={/heading[1-3]|title/i.test(value.style || "")} contentEditable={editable ? "plaintext-only" : false} suppressContentEditableWarning role={editable ? "textbox" : undefined} aria-label={editable ? label : undefined} aria-multiline={editable ? true : undefined} tabIndex={editable ? 0 : undefined} onInput={event => change(event.currentTarget.innerText)} style={{ textAlign: ["center","right","left","justify"].includes(value.align || "") ? value.align as CSSProperties["textAlign"] : undefined }}>
    {firstValue !== value.text || !value.runs.length ? firstValue || <br /> : value.runs.map((run, index) => <span key={index} style={{ fontWeight: run.bold ? 600 : undefined, fontStyle: run.italic ? "italic" : undefined, textDecoration: run.underline ? "underline" : undefined, color: run.color }}>{run.text}</span>)}
  </div>
    {value.images?.map((source, index) => <img key={index} src={source} alt="Embedded document image" />)}
  </div>;
}
function WordDocument({ blocks, edits, editable, revision, change }: { blocks: WordBlock[]; edits: Record<string,string>; editable: boolean; revision: string; change(id: string,value: string,original: string): void }) {
  const paragraph = (value: FileParagraph) => <Paragraph key={`${revision}:${value.id}`} value={value} initial={edits[value.id] ?? value.text} editable={editable} label={`Document paragraph ${value.id.split(":").at(-1)}`} change={next => change(value.id,next,value.text)} />;
  return <div className="file-document-page">{blocks.map((block,index) => block.type === "paragraph" ? paragraph(block) : <table className="file-word-table" key={index}><tbody>{block.rows.map((row,r) => <tr key={r}>{row.map((cell,c) => <td key={c}>{cell.map(paragraph)}</td>)}</tr>)}</tbody></table>)}</div>;
}
function SlideDocument({ view, edits, editable, revision, change }: { view: FileView; edits: Record<string,string>; editable: boolean; revision:string; change(id:string,value:string,original:string):void }) {
  const [index,setIndex]=useState(0), slides=view.slides ?? [], slide=slides[index];
  if(!slide)return <p className="file-feedback">No supported slide content is available.</p>;
  const height=960*(view.height || 6858000)/(view.width || 12192000);
  return <div className="file-slides"><div className="file-slide-nav"><button className="icon-button" aria-label="Previous slide" disabled={!index} onClick={()=>setIndex(value=>value-1)}><ChevronLeft size={16}/></button><span>Slide {index+1} of {slides.length}</span><button className="icon-button" aria-label="Next slide" disabled={index===slides.length-1} onClick={()=>setIndex(value=>value+1)}><ChevronRight size={16}/></button></div><svg className="file-slide" viewBox={`0 0 960 ${height}`} role="group" aria-label={`Slide ${index+1}`}><rect width="960" height={height} fill="white"/>{slide.shapes.map((shape,i)=><foreignObject key={shape.id || i} x={shape.x*9.6} y={shape.y/100*height} width={shape.width*9.6} height={Math.max(shape.height/100*height,50)}><div className="file-slide-shape" style={{background:shape.fill,fontSize:shape.paragraphs[0]?.runs[0]?.size || 22}}>{shape.image && <img src={shape.image} alt="Slide image"/>}{shape.paragraphs.map(paragraph=><Paragraph key={`${revision}:${paragraph.id}`} value={paragraph} initial={edits[paragraph.id] ?? paragraph.text} editable={editable} label={`Slide ${index+1} text ${paragraph.id.split(":").at(-1)}`} change={value=>change(paragraph.id,value,paragraph.text)}/>)}</div></foreignObject>)}</svg></div>;
}
