import { useId, useState } from "react";
import type { BackendPanel } from "../../ui/src/protocol";

type Row = NonNullable<BackendPanel["rows"]>[number];

export function PanelField({ row, value, disabled, group, panelID, change }: { row: Row; value: string; disabled: boolean; group?: string; panelID?: string; change(value: string): void }) {
  const id = useId();
  const [choosing, setChoosing] = useState(false);
  const [issue, setIssue] = useState("");
  const field = row.input!;
  const locked = disabled || field.read_only || choosing;
  const help = field.help || (field.secret ? "Leave empty to keep the stored credential." : "");
  const describedBy = help ? `${id}-help` : undefined;
  const choose = async () => {
    setChoosing(true);
    setIssue("");
    try { const path = panelID && await window.gator?.choosePath({ panel_id: panelID, kind: field.kind! }); if (path) change(path); }
    catch (error) { setIssue(error instanceof Error ? error.message : "Could not open the path picker."); }
    finally { setChoosing(false); }
  };
  const control = field.kind === "select" ? <select id={id} aria-label={row.label} aria-describedby={describedBy} value={value} disabled={locked} onChange={(event) => change(event.target.value)}>{field.options?.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select>
    : field.kind === "multi" ? <fieldset className="field-options" aria-describedby={describedBy} disabled={locked}><legend className="visually-hidden">{row.label}</legend>{field.options?.length ? field.options.map((option) => {
      let selected: string[] = []; try { selected = JSON.parse(value) || []; } catch { /* An empty saved choice means local files only. */ }
      return <label key={option.value}><input type="checkbox" checked={selected.includes(option.value)} onChange={(event) => change(JSON.stringify(event.target.checked ? [...selected, option.value] : selected.filter((item) => item !== option.value)))} /><span>{option.label}</span></label>;
    }) : <p className="muted">Local files only. Configure a connection to include mail or calendar.</p>}</fieldset>
    : field.kind === "budget" ? <fieldset className="field-budget" aria-describedby={describedBy} disabled={locked}><legend className="visually-hidden">{row.label}</legend>{field.parts?.map((part, index) => <label key={part.label}><span>{part.label}</span><input type="number" aria-label={part.label} min={part.min} max={part.max} step={1} value={value.split(",")[index] ?? ""} onChange={(event) => { const parts = value.split(","); parts[index] = event.target.value; change(parts.join(",")); }} /></label>)}</fieldset>
    : field.multiline ? <textarea id={id} aria-label={row.label} aria-describedby={describedBy} disabled={locked} value={value} maxLength={field.limit} placeholder={field.placeholder} onChange={(event) => change(event.target.value)} rows={3} />
    : <div className="field-input"><input id={id} aria-label={row.label} aria-describedby={describedBy} disabled={locked} type={field.secret ? "password" : field.kind === "number" ? "number" : "text"} autoComplete="off" value={value} maxLength={field.limit} min={field.kind === "number" ? field.min ?? 0 : undefined} max={field.max || undefined} step={field.kind === "number" ? 1 : undefined} placeholder={field.placeholder} onChange={(event) => change(event.target.value)} />{["directory", "file", "save"].includes(field.kind ?? "") && <button className="button" aria-label={`Browse ${row.label.toLocaleLowerCase()}`} disabled={locked} onClick={() => void choose()}>Browse…</button>}</div>;
  return <>{group && <h2 className="form-group-title">{group}</h2>}<div className="field"><label htmlFor={["multi", "budget"].includes(field.kind ?? "") ? undefined : id}>{row.label}</label>{control}{help && <small id={describedBy}>{help}</small>}{issue && <small role="alert">{issue}</small>}</div></>;
}
