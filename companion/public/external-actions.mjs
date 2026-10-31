// Human-reviewed terminal controls. Storage contains only receipt identity.
export function createExternalActions({getSession,revoked}){
 const $=id=>document.getElementById(id),dialog=$('external-action-dialog');
 let owner,binding,item,pending,receipt,busy=false,epoch=0,reviewed,reviewedAt;
 const drafts=new Map(),same=(a,b)=>a?.id===b?.id&&a?.generation===b?.generation;
 const key=b=>`gator.terminal-receipt.v1:${b.id}:${b.generation}`;
 const allowed=()=>owner&&owner===getSession()&&binding&&item&&Date.now()-Date.parse(item.captured_at)<60000&&!document.hidden&&navigator.onLine&&!busy&&!pending;
 const op=kind=>kind==='interrupt'?'interrupt':kind==='key'?'key_input':'text_input';
 function render(){
  $('external-actions').hidden=!binding||(!item?.controls?.length&&!pending);
  $('terminal-draft').hidden=!item?.controls?.includes('text_input');$('terminal-draft-label').hidden=$('terminal-draft').hidden;
  $('terminal-key-choice').hidden=!item?.controls?.includes('key_input');$('terminal-key-label').hidden=$('terminal-key-choice').hidden;
  for(const kind of ['text','text_enter','key','interrupt'])$(`terminal-${kind}`).hidden=!item?.controls?.includes(op(kind));
  for(const kind of ['text','text_enter','key','interrupt'])$(`terminal-${kind}`).disabled=!allowed();
  $('terminal-draft').disabled=!allowed()||!item?.controls?.includes('text_input');
  $('terminal-key-choice').disabled=!allowed()||!item?.controls?.includes('key_input');
  $('terminal-reconcile').hidden=!pending;$('terminal-reconcile').disabled=busy||!owner||!navigator.onLine;
  $('terminal-dismiss').hidden=!receipt||receipt.delivery==='outcome_unknown';
  $('terminal-confirm').disabled=!allowed()||!reviewed;
 }
 function select(next,nextBinding,nextItem){
  if(owner!==next||!same(binding,nextBinding)){
   if(owner===next&&binding)drafts.set(key(binding),$('terminal-draft').value);else drafts.clear();
   owner=next;binding=nextBinding;epoch++;busy=false;pending=receipt=reviewed=undefined;dialog.close();
   $('terminal-draft').value=binding?(drafts.get(key(binding))||''):'';
   $('terminal-status').textContent='No terminal action submitted.';
   try{if(binding){const saved=JSON.parse(sessionStorage.getItem(key(binding))||'null');if(saved&&same(saved.target,binding)&&/^[a-f0-9]{32}$/.test(saved.id)){pending=saved;$('terminal-status').textContent='Retained action identity. Retrieve receipt; no input will be resent.';}}}catch{pending={unavailable:true};$('terminal-status').textContent='Receipt storage unavailable; controls disabled.';}
  }
  item=nextItem;
  if(reviewed&&(!allowed()||!item?.controls?.includes(op(reviewed.input.kind)))){reviewed=undefined;dialog.close();}
  render();
 }
 function stale(){item=undefined;reviewed=undefined;dialog.close();render();}
 function show(r){receipt=r;$('terminal-status').textContent=`${{not_dispatched:'Not dispatched',dispatch_acknowledged:'Dispatch acknowledged · program outcome not established',outcome_unknown:'Outcome unknown · do not resend'}[r.delivery]}\n${r.note||''}`;render();}
 function review(kind){
  if(!allowed()||!item.controls.includes(op(kind)))return;
  const input={kind};if(kind==='text'||kind==='text_enter'){input.text=$('terminal-draft').value;if(!input.text||new TextEncoder().encode(input.text).length>2048||/[\p{Cc}\p{Cf}\p{Cs}\u2028\u2029]/u.test(input.text)){ $('terminal-status').textContent='Enter at most 2048 bytes, without newlines or control characters.';return;}}
  if(kind==='key')input.key=$('terminal-key-choice').value;
  reviewedAt=item.captured_at;reviewed={id:crypto.randomUUID().replaceAll('-',''),target:binding.id,generation:binding.generation,input};
  $('terminal-review').textContent=`Target: ${item.target.label} · tmux\n${binding.id}\nSnapshot age: ${Math.max(0,Math.floor((Date.now()-Date.parse(item.captured_at))/1000))} seconds\nOperation: ${kind==='interrupt'?'Ctrl-C terminal interrupt request (termination is not guaranteed)':kind}\nEnter appended: ${kind==='text_enter'?'yes':'no'}\nExact payload: ${JSON.stringify(input)}\nTerminal input has host-user authority. Dispatch does not prove program completion.`;
  render();dialog.showModal();
 }
 for(const kind of ['text','text_enter','key','interrupt'])$(`terminal-${kind}`).addEventListener('click',()=>review(kind));
 $('terminal-draft').addEventListener('input',()=>{reviewed=undefined;dialog.close();});
 $('terminal-review-cancel').addEventListener('click',()=>{reviewed=undefined;dialog.close();});
 $('terminal-confirm').addEventListener('click',async()=>{
  if(!allowed()||!reviewed||Date.now()-Date.parse(reviewedAt)>60000||!same(binding,{id:reviewed.target,generation:reviewed.generation})||!item.controls.includes(op(reviewed.input.kind)))return;
  const action=reviewed,session=owner,at=epoch;reviewed=undefined;dialog.close();
  const meta={id:action.id,target:{id:action.target,generation:action.generation}};
  try{sessionStorage.setItem(key(binding),JSON.stringify(meta));}catch{$('terminal-status').textContent='Could not retain receipt identity. Nothing sent.';return;}
  pending=meta;busy=true;render();$('terminal-status').textContent='Submitting reviewed input…';
  try{
   const response=await fetch(`/v1/external/${action.target}/actions`,{method:'POST',cache:'no-store',redirect:'error',headers:{Authorization:`Bearer ${session.token}`,'Content-Type':'application/json'},body:JSON.stringify(action),signal:AbortSignal.timeout(25000)});
   const value=await response.json();if(epoch!==at||owner!==session||getSession()!==session)return;
   if(response.status===401){revoked();return;}
   if(response.ok&&value.receipt?.id===action.id&&same(value.receipt.target,meta.target))show(value.receipt);
   else if(value.delivery==='not_dispatched')show({delivery:'not_dispatched',note:'Gateway rejected this request before dispatch. Reconcile any prior use of this ID.'});
   else $('terminal-status').textContent='Unconfirmed action. Retrieve receipt; do not blindly resend.';
  }catch{if(epoch===at&&owner===session)$('terminal-status').textContent='Outcome unknown. Retrieve receipt; input will not be replayed.';}
  finally{if(epoch===at){busy=false;render();}}
 });
 $('terminal-reconcile').addEventListener('click',async()=>{
  if(!pending?.id||!owner||busy)return;const saved=pending,session=owner,at=epoch;busy=true;render();
  try{const r=await fetch(`/v1/external/${saved.target.id}/receipts/${saved.id}?generation=${saved.target.generation}`,{cache:'no-store',redirect:'error',headers:{Authorization:`Bearer ${session.token}`},signal:AbortSignal.timeout(25000)});const v=await r.json();if(at!==epoch||owner!==session||session!==getSession())return;if(r.status===401){revoked();return;}if(!r.ok||v.receipt?.id!==saved.id||!same(v.receipt.target,saved.target))throw new Error();show(v.receipt);}catch{if(at===epoch)$('terminal-status').textContent='Receipt unavailable or unconfirmed. No input resent.';}finally{if(at===epoch){busy=false;render();}}
 });
 $('terminal-dismiss').addEventListener('click',()=>{if(receipt&&receipt.delivery!=='outcome_unknown'){sessionStorage.removeItem(key(binding));pending=receipt=undefined;$('terminal-status').textContent='Receipt dismissed. Inspect before another action.';render();}});
 function retainedTargets(){const results=[];try{for(let i=0;i<sessionStorage.length&&results.length<32;i++){const name=sessionStorage.key(i);if(name.startsWith('gator.terminal-receipt.v1:')){const v=JSON.parse(sessionStorage.getItem(name));if(/^target-[a-f0-9]{32}$/.test(v?.target?.id)&&/^[a-f0-9]{32}$/.test(v.target.generation)&&/^[a-f0-9]{32}$/.test(v.id))results.push(v.target);}}}catch{}return results;}
 return {select,stale,retainedTargets};
}
