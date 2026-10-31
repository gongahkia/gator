import {createExternalActions} from './external-actions.mjs';
// Snapshots are sensitive, untrusted text. Never retained in storage or chat.
export function createExternalView({getSession,revoked,document=globalThis.document,fetch=globalThis.fetch}){
 const $=id=>document.getElementById(id);
 const actions=createExternalActions({getSession,revoked});
 let owner,selected,catalog=[],epoch=0,controller,timer,busy=false;
 const clear=()=>{$('external-output').textContent='';$('external-meta').textContent='';};
 function sync(){
  const next=getSession();$('external-terminals').hidden=!next;
  if(next!==owner){owner=next;selected=undefined;catalog=[];epoch++;controller?.abort();busy=false;clear();$('external-select').replaceChildren(new Option('Choose a terminal',''));actions.select(owner,undefined,null);}
  const active=owner&&!document.hidden&&navigator.onLine&&$('external-open').open;
  if(!active){clearInterval(timer);timer=undefined;epoch++;controller?.abort();busy=false;clear();actions.stale();if(owner)$('external-status').textContent='Paused · refresh when online and visible.';}
  else if(!timer)timer=setInterval(()=>{if(!busy)void refresh();},5000);
 }
 async function read(path,session,signal){const r=await fetch(path,{cache:'no-store',redirect:'error',headers:{Authorization:`Bearer ${session.token}`},signal});if(r.status===401){const e=new Error('Access revoked');e.revoked=true;throw e;}if(!r.ok)throw new Error('Unavailable');return r.json();}
 async function refresh(){
  if(!owner||document.hidden||!navigator.onLine||!$('external-open').open)return;
  const at=++epoch,session=owner,chosen=selected;controller?.abort();const request=new AbortController();controller=request;busy=true;
  const timeout=setTimeout(()=>request.abort(),25000);$('external-status').textContent='Refreshing terminals…';
  const current=()=>at===epoch&&owner===session&&session===getSession();
  try{
   const page=await read('/v1/external',session,request.signal);if(!current())return;
   catalog=page.targets;$('external-select').replaceChildren(new Option('Choose a terminal',''),...catalog.map(i=>new Option(`${i.target.label} · tmux`,i.target.binding.id)));
   for(const saved of actions.retainedTargets())if(!catalog.some(i=>i.target.binding.id===saved.id))$('external-select').append(new Option('Retained terminal receipt',saved.id));
   if(!chosen){clear();$('external-status').textContent=catalog.length?'Choose a shared terminal.':'No authorized external terminals. Enroll and grant Inspect on the host.';return;}
   if(!catalog.some(i=>i.target.binding.id===chosen.id&&i.target.binding.generation===chosen.generation)){clear();actions.select(owner,chosen,null);$('external-select').value=chosen.id;$('external-status').textContent='Unavailable · enrollment, generation or permission changed.';return;}
   $('external-select').value=chosen.id;
   const detail=await read(`/v1/external/${encodeURIComponent(chosen.id)}/snapshot?generation=${encodeURIComponent(chosen.generation)}`,session,request.signal);if(!current())return;
   const item=detail.targets[0];if(!item||detail.unavailable||item.target.binding.id!==chosen.id||item.target.binding.generation!==chosen.generation)throw new Error('Unavailable');
   $('external-meta').textContent=`${item.target.label} · tmux · ${chosen.id}\nCaptured ${item.captured_at}`;
   $('external-output').textContent=item.text;actions.select(owner,chosen,item);
   $('external-status').textContent=item.truncated?'Available · bounded snapshot, earlier output omitted.':'Available · recent snapshot.';
  }catch(e){if(!current())return;clear();actions.stale();$('external-status').textContent='Unavailable or access withdrawn · refresh to retry.';if(e.revoked)revoked();}
  finally{clearTimeout(timeout);if(current())busy=false;}
 }
 $('external-select').addEventListener('change',()=>{selected=catalog.find(i=>i.target.binding.id===$('external-select').value)?.target.binding||actions.retainedTargets().find(b=>b.id===$('external-select').value);epoch++;controller?.abort();clear();actions.select(owner,selected,null);void refresh();});
 $('external-refresh').addEventListener('click',()=>void refresh());
 $('external-open').addEventListener('toggle',()=>{sync();if($('external-open').open)void refresh();});
 document.addEventListener('visibilitychange',()=>{sync();if(!document.hidden)void refresh();});
 globalThis.addEventListener('offline',sync);globalThis.addEventListener('online',()=>{sync();void refresh();});
 return {sync};
}
