import test from 'node:test';
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdtemp,writeFile,readFile,rm} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {tmpdir} from 'node:os';
import {createCompanion} from './server.mjs';
import {addDevice,inspectionDeviceActor,revokeDevice} from './auth.mjs';
import {callTerminal,validateTerminalAction} from './external-control.mjs';
const execute=promisify(execFile),defer=()=>{let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve};};
const target='target-'+'a'.repeat(32),generation='b'.repeat(32);
test('terminal gateway rejects untrusted shape and checks revocation after queue waits',async()=>{
 const root=await mkdtemp(join(tmpdir(),'gator-t4-http-')),tokenFile=join(root,'token');let c,gate=defer(),entered=defer(),checks=0,dispatches=0;
 try{
  await writeFile(tokenFile,'a'.repeat(64)+'\n',{mode:0o600});const phone=await addDevice(tokenFile,'Phone',join(root,'phone')),token=(await readFile(join(root,'phone'),'utf8')).trim();
  c=createCompanion({binary:'',workspace:root,tokenFile,controlTerminal:async(mode,input)=>{if(mode==='check'){checks++;if(checks===1){entered.resolve();await gate.promise;}return {authorized:true};}dispatches++;throw new Error('should never dispatch');}});
  const a=await c.listen(),base=`http://127.0.0.1:${a.port}`;
  const action={id:'c'.repeat(32),target,generation,input:{kind:'text',text:'literal'}};
  for(const bad of [{...action,actor:'trusted'},{...action,input:{kind:'text',text:'a\nb'}},{...action,input:{kind:'key',key:'run-shell'}},{...action,input:{kind:'text',text:'x'.repeat(2049)}}])assert.throws(()=>validateTerminalAction(bad,target));
  const first=fetch(`${base}/v1/external/${target}/actions`,{method:'POST',headers:{Authorization:`Bearer ${token}`,'Content-Type':'application/json'},body:JSON.stringify(action)});await entered.promise;await revokeDevice(tokenFile,phone.id);gate.resolve();assert.equal((await first).status,401);assert.equal(dispatches,0);
 }finally{gate.resolve();await c?.close();await rm(root,{recursive:true,force:true});}
});

test('real isolated tmux: separate trust, grants, literal input, keys, interrupt and cross-helper deduplication',{skip:process.env.GATOR_TMUX_LIVE!=='1',timeout:180000},async()=>{
 const root=await mkdtemp(join(tmpdir(),'gt4-')),socket=join(root,'socket'),binary=join(root,'runtime'),tokenFile=join(root,'token');let c;
 const mux=process.env.GATOR_TEST_TMUX||'/usr/local/bin/tmux',env={...process.env,GATOR_STATE_DIR:join(root,'state'),GATOR_INTERNAL_COMMANDS:'1'};
 const tmux=(...a)=>execute(mux,['-f','/dev/null','-S',socket,...a],{timeout:5000});
 try{
  await execute('go',['build','-o',binary,'./cmd/gator'],{timeout:90000});await writeFile(tokenFile,'a'.repeat(64)+'\n',{mode:0o600});
  const phone=await addDevice(tokenFile,'Phone',join(root,'phone')),token=(await readFile(join(root,'phone'),'utf8')).trim(),actor=await inspectionDeviceActor(tokenFile,phone.id);
  await tmux('new-session','-d','-s','fixture','-x','120','-y','32',process.execPath,'-e',"process.on('SIGINT',()=>console.log('T4_INTERRUPTED'));process.stdin.on('data',b=>console.log('READ:'+b.toString('hex')));setInterval(()=>{},1000)");
  const pane=(await tmux('display-message','-p','#{pane_id}')).stdout.trim();await tmux('split-window','-d','cat');const other=(await tmux('list-panes','-F','#{pane_id}')).stdout.trim().split('\n').find(p=>p!==pane);await tmux('select-pane','-t',other);
  const enrolled=JSON.parse((await execute(binary,['external-tmux','enroll','--reviewed-read-config',mux,socket,pane,'T4 intended pane'],{env})).stdout),b=enrolled.binding;
  await execute(process.execPath,[resolve('companion/external-access.mjs'),binary,tokenFile,b.id,phone.id,'60'],{env});
  c=createCompanion({binary,workspace:root,env,tokenFile,spawnBackend(){throw new Error('External control created Work');}});const address=await c.listen(),base=`http://127.0.0.1:${address.port}`,headers={Authorization:`Bearer ${token}`,'Content-Type':'application/json'};
  const snapshot=async()=>{const r=await fetch(`${base}/v1/external/${b.id}/snapshot?generation=${b.generation}`,{headers});assert.equal(r.status,200);return r.json();};
  const action=(id,input)=>({id:id.repeat(32),target:b.id,generation:b.generation,input});
  const post=async a=>{const r=await fetch(`${base}/v1/external/${b.id}/actions`,{method:'POST',headers,body:JSON.stringify(a)});return {status:r.status,value:await r.json()};};
  assert.deepEqual((await snapshot()).targets[0].controls,[]);
  assert.notEqual((await post(action('a',{kind:'text',text:'unauthorized'}))).status,200);
  await execute(binary,['external-tmux','review-control',b.id],{env});
  const grant=op=>execute(process.execPath,[resolve('companion/external-control-access.mjs'),binary,tokenFile,b.id,phone.id,op,'60'],{env});
  await grant('text_input');const literal='Enter Escape ; $(LITERAL_ONLY) 猫;' ;const a=action('b',{kind:'text',text:literal});
  const sent=await post(a);assert.equal(sent.status,200,JSON.stringify(sent.value));assert.equal(sent.value.receipt.delivery,'dispatch_acknowledged');
  let out=(await snapshot()).targets[0].text;assert.match(out,/LITERAL_ONLY/);assert.doesNotMatch(out,/READ:/,'no Enter must be appended');assert.doesNotMatch((await tmux('capture-pane','-p','-t',other)).stdout,/LITERAL_ONLY/);
  assert.notEqual((await post(action('c',{kind:'interrupt'}))).status,200,'text grant cannot interrupt');
  const dup=await post(a);assert.equal(dup.value.receipt.id,sent.value.receipt.id);
  assert.notEqual((await post({...a,input:{kind:'text',text:'conflict'}})).status,200);
  await grant('key_input');const enter=await post(action('d',{kind:'key',key:'Enter'}));assert.equal(enter.value.receipt.delivery,'dispatch_acknowledged');
  out=(await snapshot()).targets[0].text;assert.match(out,new RegExp('READ:'+Buffer.from(literal+'\n').toString('hex')));
  const concurrent=action('e',{kind:'text',text:'ONCE_ONLY'});
  const replies=await Promise.all(Array.from({length:6},()=>callTerminal(binary,root,env,'dispatch',{actor,action:concurrent})));
  assert.ok(replies.every(r=>r.receipt.id===concurrent.id&&['dispatch_acknowledged','outcome_unknown'].includes(r.receipt.delivery)));const reconciled=await callTerminal(binary,root,env,'receipt',{actor,action:{id:concurrent.id,target:b.id,generation:b.generation}});assert.equal(reconciled.receipt.delivery,'dispatch_acknowledged',JSON.stringify(reconciled));out=(await snapshot()).targets[0].text;assert.equal(out.split('ONCE_ONLY').length-1,1,'cross-process duplicates typed once');
  const withEnter=await post(action('f',{kind:'text_enter',text:'TEXT_PLUS_ENTER'}));assert.equal(withEnter.value.receipt.delivery,'dispatch_acknowledged');
  await grant('interrupt');const interrupt=await post(action('1',{kind:'interrupt'}));assert.equal(interrupt.value.receipt.delivery,'dispatch_acknowledged');
  for(let i=0;i<50;i++){out=(await snapshot()).targets[0].text;if(out.includes('T4_INTERRUPTED'))break;await new Promise(r=>setTimeout(r,20));}assert.match(out,/T4_INTERRUPTED/);
  const receiptPath=join(root,'state/gator/external-actions','.terminal','receipts',a.id+'.json');assert.equal((await readFile(receiptPath,'utf8')).includes(literal),false);
  await execute(binary,['external-tmux','unreview-control',b.id],{env});assert.notEqual((await post(action('2',{kind:'text',text:'DENIED'}))).status,200);assert.ok((await snapshot()).targets[0]);
  const receipt=await fetch(`${base}/v1/external/${b.id}/receipts/${a.id}?generation=${b.generation}`,{headers});assert.equal(receipt.status,200,'receipt remains reconciliable after control withdrawal');
 }finally{await c?.close();await tmux('kill-server').catch(()=>{});await rm(root,{recursive:true,force:true});}
});

test('lost dispatch reply reconciles retained receipt without repeating input',async()=>{
 const root=await mkdtemp(join(tmpdir(),'gt4-lost-')),tokenFile=join(root,'token');let c,retained,dispatches=0;
 try{await writeFile(tokenFile,'a'.repeat(64)+'\n',{mode:0o600});c=createCompanion({binary:'',workspace:root,tokenFile,controlTerminal:async(mode,{action})=>{
 if(mode==='check')return {authorized:true};if(mode==='receipt')return {receipt:retained};dispatches++;retained={id:action.id,target:{id:action.target,backend:'external_tmux',generation:action.generation},operation:'text_input',scope:'text',digest:'d'.repeat(64),phase:'settled',delivery:'dispatch_acknowledged',note:'tmux accepted dispatch; program outcome unproven.',created_at:new Date().toISOString(),updated_at:new Date().toISOString()};throw new Error('Simulated loss after dispatch');
 }});const address=await c.listen(),base=`http://127.0.0.1:${address.port}`,headers={Authorization:'Bearer '+'a'.repeat(64),'Content-Type':'application/json'},action={id:'9'.repeat(32),target,generation,input:{kind:'text',text:'once'}};
 const lost=await fetch(`${base}/v1/external/${target}/actions`,{method:'POST',headers,body:JSON.stringify(action)});assert.notEqual(lost.status,200);
 for(let i=0;i<3;i++){const response=await fetch(`${base}/v1/external/${target}/receipts/${action.id}?generation=${generation}`,{headers});assert.equal(response.status,200);assert.equal((await response.json()).receipt.delivery,'dispatch_acknowledged');}
 assert.equal(dispatches,1);
 }finally{await c?.close();await rm(root,{recursive:true,force:true});}
});
