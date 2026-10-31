import { execFile } from 'node:child_process';
const hex32=/^[a-f0-9]{32}$/, hex64=/^[a-f0-9]{64}$/, specialist=/^subagent-\d{3}$/;
export function validateRemoteAction(a,target){
  const allowed=['id','run_id','target','operation','specialist','question','instruction','answers','decision_id','preview_digest','approved'];
  if(!a||typeof a!=='object'||Array.isArray(a)||Object.keys(a).some(k=>!allowed.includes(k))||!hex32.test(a.id)||!/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(a.run_id)||!['cancel','steer','answer','decision'].includes(a.operation)||Buffer.byteLength(JSON.stringify(a))>24576)throw new Error('Invalid semantic action.');
  const b=a.target;
  if(!b||Object.keys(b).some(k=>!['id','backend','owner','generation','incarnation'].includes(k))||b.id!==target||!/^target-[a-f0-9]{32}$/.test(b.id)||b.backend!=='gator_work'||!hex32.test(b.generation)||typeof b.owner!=='string'||b.owner.length>256||typeof b.incarnation!=='string'||b.incarnation.length>512)throw new Error('Invalid target binding.');
  const input=v=>typeof v==='string'&&v.trim()&&Buffer.byteLength(v)<=8192&&!v.includes('\0');
  if(a.operation!=='steer'&&a.instruction!==undefined||a.operation!=='answer'&&(a.question!==undefined||a.answers!==undefined)||a.operation!=='decision'&&(a.decision_id!==undefined||a.preview_digest!==undefined||a.approved!==undefined)||a.operation==='cancel'&&a.specialist!==undefined)throw new Error('Unrelated semantic fields.');
  if(['steer','answer'].includes(a.operation)&&!specialist.test(a.specialist||''))throw new Error('Select an exact specialist.');
  if(a.operation==='steer'&&!input(a.instruction))throw new Error('Review a bounded steering instruction.');
  if(a.operation==='answer'&&(!hex64.test(a.question)||!a.answers||Array.isArray(a.answers)||typeof a.answers!=='object'||Object.keys(a.answers).length<1||Object.keys(a.answers).length>8||Object.entries(a.answers).some(([k,v])=>!k||k.length>128||!Array.isArray(v)||v.length!==1||!input(v[0]))))throw new Error('Review answers to the exact question.');
  if(a.operation==='decision'&&(a.specialist!==undefined||!Number.isSafeInteger(a.decision_id)||a.decision_id<1||!hex64.test(a.preview_digest)||typeof a.approved!=='boolean'))throw new Error('Use an exact reviewed decision.');
  return a;
}
export function validateControlOffers(offers){
  if(offers===undefined)return [];
  if(!Array.isArray(offers)||offers.length>8||Buffer.byteLength(JSON.stringify(offers))>24576)throw new Error('Invalid control offers.');
  return offers.map(o=>{
    if(!['cancel','steer','answer','decision'].includes(o.operation)||typeof o.scope!=='string'||o.scope.length>256||!Number.isFinite(Date.parse(o.expires_at)))throw new Error('Invalid operation grant.');
    const v={operation:o.operation,scope:o.scope,expires_at:o.expires_at};
    if(['steer','answer'].includes(o.operation)){if(!specialist.test(o.specialist||''))throw new Error('Invalid specialist offer.');v.specialist=o.specialist;}
    if(o.operation==='answer'){
      if(!hex64.test(o.question)||!Array.isArray(o.questions)||o.questions.length<1||o.questions.length>8||Buffer.byteLength(JSON.stringify(o.questions))>8192)throw new Error('Invalid question review.');
      v.question=o.question;v.questions=o.questions.map(q=>{if(typeof q.id!=='string'||q.id.length>128||typeof q.text!=='string'||q.text.length>8192||q.options!==undefined&&(!Array.isArray(q.options)||q.options.some(x=>typeof x!=='string')))throw new Error('Invalid question.');return{id:q.id,text:q.text,options:q.options};});
    }
    if(o.operation==='decision'){if(!Number.isSafeInteger(o.decision_id)||o.decision_id<1||!hex64.test(o.preview_digest)||!Array.isArray(o.preview)||o.preview.length<1||o.preview.length>32||o.preview.some(x=>typeof x!=='string')||Buffer.byteLength(JSON.stringify(o.preview))>8192)throw new Error('Invalid approval review.');v.decision_id=o.decision_id;v.preview_digest=o.preview_digest;v.preview=o.preview;}
    return v;
  });
}
export function validateControlReply(value,target,id){
  if(!value||Buffer.byteLength(JSON.stringify(value))>16384)throw new Error('Invalid control response.');
  if(!value.receipt)return{authorized:value.authorized===true};
  const r=value.receipt;
  if(r.id!==id||r.target?.id!==target||r.target.backend!=='gator_work'||!hex32.test(r.target.generation)||!['not_dispatched','dispatch_acknowledged','outcome_unknown'].includes(r.delivery)||!['claimed','settled'].includes(r.phase)||!['cancel','steer','answer','decision'].includes(r.operation)||typeof r.note!=='string'||r.note.length>256||!hex64.test(r.digest))throw new Error('Invalid action receipt.');
  return{observation:value.observation==='work_lease_stopped'?'work_lease_stopped':undefined,receipt:{version:r.version,id:r.id,run_id:r.run_id,target:{id:r.target.id,backend:'gator_work',owner:r.target.owner,generation:r.target.generation,incarnation:r.target.incarnation},operation:r.operation,scope:r.scope,digest:r.digest,phase:r.phase,delivery:r.delivery,note:r.note,created_at:r.created_at,updated_at:r.updated_at}};
}
export function callControl(binary,workspace,env,mode,input){
  return new Promise((resolve,reject)=>{
    const child=execFile(binary,['companion-control',mode],{cwd:workspace,env:{...env,GATOR_COMPANION_CONTROL:'1'},timeout:20000,maxBuffer:16384,windowsHide:true},(error,stdout)=>{if(error)return reject(Object.assign(new Error('Action status unconfirmed. Retrieve its receipt; do not resend automatically.'),{code:'CONTROL_UNCONFIRMED'}));try{resolve(JSON.parse(stdout));}catch{reject(new Error('Unconfirmed owner reply.'));}});
    child.stdin.on('error',()=>{});child.stdin.end(JSON.stringify(input));
  });
}
