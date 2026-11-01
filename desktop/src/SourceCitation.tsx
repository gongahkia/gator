import { createPortal } from "react-dom";
import { useContext, useEffect, useId, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import type { SourcePreview } from "@gator/application-host/contracts/sourcePreview";
import { SourceReferenceContext } from "./sourceReferences";

type Reference = { path: string; start?: number; end?: number };
export function SourceCitation({ reference, index, label }: { reference: Reference; index: number; label: string }) {
  const source = useContext(SourceReferenceContext), [openedKey,setOpenedKey]=useState<string>(), [loaded,setLoaded]=useState<{key:string;value?:SourcePreview;issue?:string}>();
  const [position,setPosition]=useState<CSSProperties>({visibility:"hidden"});
  const trigger=useRef<HTMLButtonElement>(null),card=useRef<HTMLDivElement>(null),timer=useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const suppressFocus=useRef(false);
  const id=useId(),key=JSON.stringify([source.context,source.run,reference]);
  const open=openedKey===key, current=loaded?.key===key?loaded:undefined, value=current?.value,issue=current?.issue;
  const close=()=>{clearTimeout(timer.current);setOpenedKey(undefined);};
  const enter=()=>{clearTimeout(timer.current);if(!open){setLoaded(undefined);setOpenedKey(key);}};
  const leave=()=>{clearTimeout(timer.current);timer.current=setTimeout(()=>setOpenedKey(undefined),150);};
  useEffect(()=>()=>clearTimeout(timer.current),[]);
  useEffect(()=>{close();setLoaded(undefined);},[key]);
  useEffect(()=>{
    if(!open || !source.context || !source.run)return;
    let active=true;setLoaded(undefined);
    void window.gator!.sourcePreview({context:source.context,run:source.run,...reference}).then(result=>{if(active)setLoaded({key,value:result});}).catch(error=>{if(active)setLoaded({key,issue:error instanceof Error?error.message:"This source is unavailable."});});
    return()=>{active=false;};
  },[open,key]);
  useLayoutEffect(()=>{
    if(!open)return;
    const place=()=>{const rect=trigger.current?.getBoundingClientRect();if(!rect)return;const width=Math.min(360,innerWidth-24);setPosition({width,left:Math.max(12,Math.min(rect.left,innerWidth-width-12)),top:Math.max(12,Math.min(rect.bottom+8,innerHeight-300)),maxHeight:Math.min(280,innerHeight-24)});};
    place();window.addEventListener("resize",close);window.addEventListener("scroll",close,true);
    const dismiss=(event:KeyboardEvent)=>{if(event.key==="Escape"){event.preventDefault();event.stopPropagation();suppressFocus.current=document.activeElement!==trigger.current;close();trigger.current?.focus({preventScroll:true});}};
    window.addEventListener("keydown",dismiss,true);
    return()=>{window.removeEventListener("resize",close);window.removeEventListener("scroll",close,true);window.removeEventListener("keydown",dismiss,true);};
  },[open]);
  const excerpt = value ? (() => {
    const anchor = value.cited ?? value.reads[0];
    const lines=value.text.split("\n");
    const passage=value.observed_preview ?? (anchor ? lines.slice(Math.max(0,anchor.start-value.firstLine),Math.max(0,anchor.end-value.firstLine+1)).join("\n") : value.text);
    return Array.from(passage).slice(0,1200).join("");
  })() : "";
  const show=()=>{if(source.run){close();source.open?.({run:source.run,...reference});}};
  return <><button ref={trigger} className="source-reference citation-chip" type="button" aria-label={label} aria-describedby={open?id:undefined} title={label} onMouseEnter={enter} onMouseLeave={leave} onFocus={()=>{if(suppressFocus.current)suppressFocus.current=false;else enter();}} onBlur={event=>{if(!card.current?.contains(event.relatedTarget as Node))leave();}} onClick={show} disabled={!source.run || !source.open}>{index}</button>
    {open && createPortal(<div id={id} ref={card} role="region" aria-label="Source excerpt" className="citation-popover" style={position} onMouseEnter={enter} onMouseLeave={leave} onFocus={enter} onBlur={leave}>
      <strong>{value?.label||label}</strong>
      {value?.captured_at && <small>Captured {new Date(value.captured_at).toLocaleString()}</small>}
      {issue ? <p role="status">Source unavailable: {issue}</p> : !value ? <p role="status">Opening retained excerpt…</p> : <><pre>{excerpt}</pre>{value.observed_preview && <small>Normalized preview received by Gator</small>}{value.notice && <p>{value.notice}</p>}{value.truncated && <small>Partial retained excerpt</small>}<button className="small-button" onClick={show}>View in source</button></>}
    </div>,document.body)}
  </>;
}
