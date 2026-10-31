import {execFile} from 'node:child_process';
const hex=/^[a-f0-9]{32}$/,target=/^target-[a-f0-9]{32}$/;
export const terminalKeys=['Enter','Escape','Tab','Backspace','Up','Down','Left','Right'];
export function validateTerminalAction(a,id){
 if(!a||Object.keys(a).some(k=>!['id','target','generation','input'].includes(k))||!hex.test(a.id)||a.target!==id||!target.test(id)||!hex.test(a.generation)||!a.input||Object.keys(a.input).some(k=>!['kind','text','key'].includes(k)))throw new Error('Invalid reviewed terminal action.');
 const i=a.input;
 if(['text','text_enter'].includes(i.kind)) {if(typeof i.text!=='string'||!i.text||Buffer.byteLength(i.text)>2048||/[\p{Cc}\p{Cf}\p{Cs}\u2028\u2029]/u.test(i.text)||i.key!==undefined)throw new Error('Text must be bounded plain UTF-8 without control characters or newlines.');}
 else if(i.kind==='key'){if(!terminalKeys.includes(i.key)||i.text!==undefined)throw new Error('Unsupported terminal key.');}
 else if(i.kind!=='interrupt'||i.text!==undefined||i.key!==undefined)throw new Error('Unsupported terminal action.');
 return {id:a.id,target:id,generation:a.generation,input:{kind:i.kind,...(i.text!==undefined?{text:i.text}:{}),...(i.key!==undefined?{key:i.key}:{})}};
}
export function validateTerminalReply(value,id,action){
 const r=value?.receipt;if(!r)return {authorized:value?.authorized===true};
 if(r.id!==action||r.target?.id!==id||r.target.backend!=='external_tmux'||!hex.test(r.target.generation)||!['text_input','key_input','interrupt'].includes(r.operation)||!['text','text_enter','key','interrupt'].includes(r.scope)||!['claimed','settled'].includes(r.phase)||!['not_dispatched','dispatch_acknowledged','outcome_unknown'].includes(r.delivery)||!/^[a-f0-9]{64}$/.test(r.digest)||typeof r.note!=='string'||r.note.length>256)throw new Error('Invalid terminal receipt.');
 return {authorized:false,receipt:{id:r.id,target:{id:r.target.id,generation:r.target.generation},operation:r.operation,kind:r.scope,digest:r.digest,phase:r.phase,delivery:r.delivery,note:r.note,created_at:r.created_at,updated_at:r.updated_at}};
}
export function callTerminal(binary,workspace,env,mode,input){return new Promise((resolve,reject)=>{
 const child=execFile(binary,['companion-external-control',mode],{cwd:workspace,env:{...env,GATOR_COMPANION_EXTERNAL_CONTROL:'1'},timeout:20000,maxBuffer:16384,windowsHide:true},(err,out)=>{if(err)return reject(new Error('Terminal action unconfirmed. Retrieve its receipt; do not resend.'));try{resolve(JSON.parse(out));}catch{reject(new Error('Terminal receipt unconfirmed.'));}});
 child.stdin.on('error',()=>{});child.stdin.end(JSON.stringify(input));
});}
