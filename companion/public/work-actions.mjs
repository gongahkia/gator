// Semantic controls only. Pending storage contains receipt identity, never input.
export function createWorkActions({ getSession, revoked }) {
  const $=id=>document.getElementById(id), dialog=$('work-action-dialog');
  let session, binding, run, offers=[], choice, reviewed, pending, receipt, busy=false, fresh=false, epoch=0;
  const key=()=>binding?`gator.work-receipt.v1:${binding.id}:${binding.generation}`:'';
  const same=(a,b)=>a?.id===b?.id&&a?.generation===b?.generation&&a?.owner===b?.owner&&a?.incarnation===b?.incarnation;
  const signature=o=>o?JSON.stringify(o):'';
  const available=()=>session&&getSession()===session&&fresh&&!busy&&!pending&&!document.hidden&&navigator.onLine&&choice&&Date.parse(choice.expires_at)>Date.now();
  function controls(){
    $('work-actions').hidden=!binding;
    $('work-operation').disabled=!fresh||busy||!!pending||!offers.length;
    $('work-action-review').disabled=!available();
    $('work-action-confirm').disabled=!available()||!reviewed;
    $('work-receipt-refresh').hidden=!pending;
    $('work-receipt-refresh').disabled=busy||!session||!navigator.onLine||document.hidden;
    $('work-receipt-dismiss').hidden=!pending||!receipt||receipt.delivery==='outcome_unknown';
  }
  function status(text){$('work-action-status').textContent=text;}
  function renderForm(){
    reviewed=undefined;dialog.close();$('work-instruction').value='';$('work-answer-fields').replaceChildren();$('work-command-preview').textContent='';$('work-decision').value='';
    $('work-steer-fields').hidden=choice?.operation!=='steer';$('work-answer-fields').hidden=choice?.operation!=='answer';$('work-decision-fields').hidden=choice?.operation!=='decision';
    if(choice?.operation==='answer')for(const q of choice.questions){const label=document.createElement('label');label.textContent=q.text;const input=document.createElement('textarea');input.rows=2;input.maxLength=8192;input.dataset.question=q.id;label.append(input);$('work-answer-fields').append(label);}
    if(choice?.operation==='decision')$('work-command-preview').textContent=JSON.stringify(choice.preview,null,2);
    controls();
  }
  function select(nextSession,nextBinding,item){
    const changed=nextSession!==session||!same(binding,nextBinding);
    if(changed){epoch++;session=nextSession;binding=nextBinding;run=item?.run_id;offers=[];choice=undefined;reviewed=undefined;receipt=undefined;pending=undefined;busy=false;fresh=false;dialog.close();
      try{if(binding){const saved=JSON.parse(sessionStorage.getItem(key())||'null');if(saved&&same(saved.target,binding)&&/^[a-f0-9]{32}$/.test(saved.id)&&typeof saved.run_id==='string'){pending=saved;run=saved.run_id;status('Unconfirmed action retained. Retrieve its receipt; no action will be resent.');}else status('No action submitted.');}}catch{status('Receipt storage unavailable. Actions are disabled.');pending={unavailable:true};}
    }
    if(item){run=item.run_id;fresh=true;}else fresh=false;
    const next=item?.controls||[], old=signature(choice);offers=next;
    if(!changed&&choice)choice=offers.find(o=>signature(o)===old);
    if(changed||old&&!choice){choice=undefined;renderForm();}
    const selected=choice?signature(choice):'';
    $('work-operation').replaceChildren(new Option(offers.length?'Choose an authorized operation':'No authorized controls',''));
    for(const o of offers){const name={cancel:'Request Work cancellation',steer:'Steer specialist',answer:'Answer specialist question',decision:'Review command approval'}[o.operation];$('work-operation').append(new Option(`${name}${o.specialist?' · '+o.specialist:''}`,signature(o)));}
    $('work-operation').value=selected;
    if(reviewed&&(!available()||signature(choice)!==reviewed.offer)){reviewed=undefined;dialog.close();}
    controls();
  }
  function stale(){fresh=false;reviewed=undefined;dialog.close();controls();}
  $('work-operation').addEventListener('change',()=>{choice=offers.find(o=>signature(o)===$('work-operation').value);renderForm();});
  $('work-action-review').addEventListener('click',()=>{
    if(!available())return;
    const action={id:crypto.randomUUID().replaceAll('-',''),run_id:run,target:{...binding},operation:choice.operation};
    if(choice.specialist)action.specialist=choice.specialist;
    if(choice.operation==='steer'){action.instruction=$('work-instruction').value;if(!action.instruction.trim())return status('Enter the steering instruction to review.');}
    if(choice.operation==='answer'){action.question=choice.question;action.answers={};for(const input of $('work-answer-fields').querySelectorAll('textarea')){if(!input.value.trim())return status('Answer every displayed question.');action.answers[input.dataset.question]=[input.value];}}
    if(choice.operation==='decision'){const v=$('work-decision').value;if(!v)return status('Choose Approve or Deny.');action.decision_id=choice.decision_id;action.preview_digest=choice.preview_digest;action.approved=v==='approve';}
    const review={action,questions:choice.questions,command:choice.preview};
    reviewed={action,offer:signature(choice)};$('work-action-preview').textContent=JSON.stringify(review,null,2);dialog.showModal();controls();
  });
  $('work-action-back').addEventListener('click',()=>{reviewed=undefined;dialog.close();controls();});
  function showReceipt(value,observation){receipt=value;status(`${value.delivery}: ${value.note}${observation==='work_lease_stopped'?' · Work owner lease observed stopped.':''}`);controls();}
  $('work-action-confirm').addEventListener('click',async()=>{
    if(!available()||!reviewed||signature(choice)!==reviewed.offer)return;
    const action=reviewed.action,at=epoch,owner=session;reviewed=undefined;dialog.close();
    const meta={id:action.id,run_id:action.run_id,target:action.target};
    try{sessionStorage.setItem(key(),JSON.stringify(meta));}catch{return status('Could not retain the action identity. Nothing was sent.');}
    pending=meta;busy=true;controls();status('Submitting reviewed action…');
    try{
      const response=await fetch(`/v1/work/${action.target.id}/actions`,{method:'POST',cache:'no-store',redirect:'error',headers:{Authorization:`Bearer ${owner.token}`,'Content-Type':'application/json'},body:JSON.stringify(action),signal:AbortSignal.timeout(25000)});
      const value=await response.json();if(at!==epoch||owner!==session||owner!==getSession())return;
      if(response.status===401){revoked();return;}
      if(response.ok&&value.receipt?.id===action.id&&same(value.receipt.target,action.target))showReceipt(value.receipt,value.observation);
      else if(value.delivery==='not_dispatched')showReceipt({delivery:'not_dispatched',note:'Rejected at gateway before dispatch.'});
      else status('Outcome unknown. Retrieve the receipt; this action will not be resent automatically.');
    }catch{if(at===epoch&&owner===session)status('Outcome unknown. Retrieve the receipt; do not submit this action again.');}
    finally{if(at===epoch){busy=false;controls();}}
  });
  $('work-receipt-refresh').addEventListener('click',async()=>{
    if(!pending?.id||busy||!session||!navigator.onLine||document.hidden)return;
    const saved=pending,at=epoch,owner=session;busy=true;controls();
    try{const response=await fetch(`/v1/work/${saved.target.id}/receipts/${saved.id}?run_id=${encodeURIComponent(saved.run_id)}`,{cache:'no-store',redirect:'error',headers:{Authorization:`Bearer ${owner.token}`},signal:AbortSignal.timeout(25000)});const value=await response.json();if(at!==epoch||owner!==session||owner!==getSession())return;if(response.status===401){revoked();return;}if(response.ok&&value.receipt?.id===saved.id&&same(value.receipt.target,saved.target))showReceipt(value.receipt,value.observation);else status('No receipt confirmed. The outcome remains unknown; no action was resent.');}catch{if(at===epoch)status('Receipt unavailable. Outcome remains unknown.');}finally{if(at===epoch){busy=false;controls();}}
  });
  $('work-receipt-dismiss').addEventListener('click',()=>{if(!receipt||receipt.delivery==='outcome_unknown')return;try{sessionStorage.removeItem(key());}catch{return;}pending=undefined;receipt=undefined;status('Receipt dismissed. Inspect current Work before another action.');stale();});
  for(const event of ['offline','pagehide'])globalThis.addEventListener(event,stale);
  document.addEventListener('visibilitychange',()=>{if(document.hidden)stale();});
  function retainedTargets(){const values=[];try{for(let i=0;i<sessionStorage.length&&values.length<32;i++){const k=sessionStorage.key(i);if(k?.startsWith('gator.work-receipt.v1:')){const v=JSON.parse(sessionStorage.getItem(k));if(v?.target?.id&&v?.target?.generation&&/^[a-f0-9]{32}$/.test(v.id))values.push(v);}}}catch{}return values;}
  return {select,stale,retainedTargets};
}
