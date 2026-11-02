import { createPortal } from "react-dom";
import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { GitBranch, MoreHorizontal, Square, Volume2, X } from "lucide-react";
import type { BranchAction, BranchResult } from "./protocol";
import type { LocalSpeech } from "./useLocalSpeech";
import { useModalFocus } from "./useModalFocus";

export function MessageActions({ conversation, revision, run, text, identity, speech, disabled, result, branch }: {
 conversation?: string; revision?: string; run?: string; text: string; identity: string;
 speech: LocalSpeech; disabled: boolean; result?: BranchResult; branch(action: BranchAction): Promise<void>;
}) {
 const [mode,setMode]=useState<"menu"|"review">(),[busy,setBusy]=useState(false),[issue,setIssue]=useState("");
 const [position,setPosition]=useState<CSSProperties>({visibility:"hidden"});
 const trigger=useRef<HTMLButtonElement>(null),menu=useRef<HTMLDivElement>(null),request=useRef("");
 const alive=useRef(true),focus=useModalFocus(mode==="review");
 const speaking=speech.active===identity;
 useEffect(()=>()=>{alive.current=false;},[]);
 const close=()=>{if(!busy){setMode(undefined);requestAnimationFrame(()=>trigger.current?.focus());}};
 useEffect(()=>{
  if(!mode)return;
  const key=(event:KeyboardEvent)=>{if(event.key==="Escape"){event.preventDefault();event.stopPropagation();close();}};
  const outside=(event:PointerEvent)=>{if(mode==="menu"&&!trigger.current?.contains(event.target as Node)&&!menu.current?.contains(event.target as Node))close();};
  window.addEventListener("keydown",key,true);window.addEventListener("pointerdown",outside);
  return()=>{window.removeEventListener("keydown",key,true);window.removeEventListener("pointerdown",outside);};
 },[mode,busy]);
 useLayoutEffect(()=>{
  if(mode!=="menu")return;
  speech.refresh();
  const rect=trigger.current?.getBoundingClientRect();if(!rect)return;
  const width=Math.min(280,innerWidth-24);
  setPosition({width,left:Math.max(12,Math.min(rect.left,innerWidth-width-12)),top:Math.max(12,Math.min(rect.bottom+6,innerHeight-150))});
  menu.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
  const dismiss=()=>close();window.addEventListener("resize",dismiss);
  return()=>window.removeEventListener("resize",dismiss);
 },[mode]);
 useEffect(()=>{
  if(!busy||result?.request_id!==request.current)return;
  setBusy(false);if(result.error)setIssue(result.error);
 },[result,busy]);
 const create=async()=>{
  if(busy||disabled||!conversation||!revision||!run)return;
  if(!request.current)request.current=crypto.randomUUID();
  setBusy(true);setIssue("");
  try{await branch({conversation_id:conversation,revision_id:revision,run_id:run,request_id:request.current});}
  catch(error){if(alive.current){setBusy(false);setIssue(`Branch status unconfirmed. Check Conversations before trying again. ${error instanceof Error?error.message:""}`);}}
 };
 return <><button ref={trigger} className="icon-button message-overflow" aria-label="Response actions" aria-expanded={!!mode} aria-haspopup="menu" onClick={()=>{setIssue("");setMode(mode?undefined:"menu");}}><MoreHorizontal size={17}/></button>
 {mode==="menu"&&createPortal(<div ref={menu} role="menu" aria-label="Response actions" className="response-menu" style={position} onKeyDown={event=>{if(!["ArrowDown","ArrowUp","Home","End"].includes(event.key))return;event.preventDefault();const buttons=[...event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')];const index=buttons.indexOf(document.activeElement as HTMLButtonElement);buttons[event.key==="Home"?0:event.key==="End"?buttons.length-1:(index+(event.key==="ArrowDown"?1:-1)+buttons.length)%buttons.length]?.focus();}}>
  <button role="menuitem" disabled={disabled||!revision||!run||!conversation} title={revision?"Create a new chat at this saved answer":"Available after a completed answer has been retained"} onClick={()=>{request.current="";setMode("review");}}><GitBranch size={17}/>Branch in new chat</button>
  <button role="menuitem" disabled={!speaking&&!speech.available} title={!speech.available?"No installed local system voice is available":undefined} onClick={()=>{if(speaking)speech.stop();else speech.speak(identity,text);close();}}>{speaking?<Square size={17}/>:<Volume2 size={17}/>} {speaking?"Stop reading":"Read aloud"}</button>
  {!speech.available&&<small>No local system voice is available.</small>}
 </div>,document.body)}
 {mode==="review"&&createPortal(<div className="modal-backdrop branch-backdrop"><section className="branch-dialog" ref={focus} role="dialog" aria-modal="true" aria-labelledby="branch-title" onKeyDown={event=>{if(event.key==="Escape"){event.preventDefault();close();}}}>
  <header><h2 id="branch-title"><GitBranch size={19}/>Branch in new chat</h2><button className="icon-button" aria-label="Close branch review" disabled={busy} onClick={close}><X size={17}/></button></header>
  <p>Create a separate chat through this saved answer. Your original chat and its running work stay intact.</p>
  <blockquote>{text.slice(0,320)}{text.length>320?"…":""}</blockquote>
  <dl><dt>Saved boundary</dt><dd>{run} · {revision}</dd></dl>
  <p>Earlier messages and retained file references are inherited. Historical tool actions are not replayed. Sources can become unavailable if their retained data expires.</p>
  <p>The new chat uses your current profile’s model, voice and privacy settings. Shared saved memories follow those settings. Select connections again; pending approvals, queues, workers, grants and schedules are not copied.</p>
  {issue&&<p className="composer-error" role="alert">{issue}</p>}
  <footer><button className="small-button" disabled={busy} onClick={close}>Keep this chat</button><button className="button" disabled={busy||disabled} onClick={()=>void create()}>{busy?"Creating branch…":"Create branch"}</button></footer>
 </section></div>,document.body)}
 {speaking&&<button className="small-button" onClick={speech.stop}><Square size={13}/>Stop reading</button>}
 {speech.issue&&speech.issueFor===identity&&<span className="composer-error" role="status">{speech.issue}</span>}
 </>;
}
