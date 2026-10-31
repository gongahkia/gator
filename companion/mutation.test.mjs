import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp,mkdir,writeFile,readFile,readdir,rm } from 'node:fs/promises';
import { join,resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { createCompanion } from './server.mjs';
import { addDevice,revokeDevice } from './auth.mjs';
import { Backend } from '../app-host/src/backend.mjs';
import { validateRemoteAction } from './control.mjs';

const execute=promisify(execFile);
const until=async check=>{for(let i=0;i<300;i++){const value=await check();if(value)return value;await new Promise(r=>setTimeout(r,30));}throw new Error('Fixture timeout');};
test('real cross-process cancellation uses owner grants, stable admission and offline receipt reads', {timeout:180000},async()=>{
  const root=await mkdtemp(join(tmpdir(),'gator-remote-control-')),workspace=join(root,'source'),config=join(root,'config'),state=join(root,'state'),binary=join(root,process.platform==='win32'?'runtime.exe':'runtime'),tokenFile=join(root,'owner.token');
  let companion,host,release,held=false,calls=0;
  const gate=new Promise(r=>release=r);
  const provider=createServer(async(req,res)=>{let body='';for await(const chunk of req)body+=chunk;const payload=JSON.parse(body),classify=payload.messages.some(m=>m.role==='system'&&m.content?.includes("Classify the user's immediate request"));calls++;if(!classify&&body.includes('T2-held')){held=true;await gate;}res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message:{role:'assistant',content:classify?'{"mode":"inspect"}':'Done'},finish_reason:'stop'}],usage:{prompt_tokens:1,completion_tokens:1}}));});
  try{
    await mkdir(workspace);await mkdir(join(config,'gator'),{recursive:true});await writeFile(tokenFile,'a'.repeat(64)+'\n',{mode:0o600});const exportFile=join(root,'phone.token'),device=await addDevice(tokenFile,'Phone',exportFile),token=(await readFile(exportFile,'utf8')).trim();
    await new Promise(r=>provider.listen(0,'127.0.0.1',r));await writeFile(join(config,'gator/config.json'),JSON.stringify({version:4,defaults:{provider:'fixture',model:'fixture'},custom_providers:[{id:'fixture',base_url:`http://127.0.0.1:${provider.address().port}/chat`,models:['fixture'],default_model:'fixture'}]}));
    await execute('go',['build','-o',binary,'./cmd/gator'],{timeout:90000});const env={...process.env,GATOR_STATE_DIR:state,GATOR_CONFIG_DIR:config,GATOR_INTERNAL_COMMANDS:'1',GATOR_UI_FRONTEND:'terminal',GATOR_UI_SESSION_HOST:'1'};
    await execute(binary,['work','--source',workspace,'--mode','inspect','Seed conversation'],{env,timeout:30000});const chat=(await readdir(join(state,'gator/conversations'))).find(n=>n.startsWith('work-'));
    host=new Backend(binary,workspace,env,['ui-backend',chat,'']);await host.ready();await host.submitUpload({type:'upload_submit',upload:{request_id:'mutation-test-message',context:`conversation:${chat}`,text:'T2-held inspect this',files:[]}});await until(()=>held);
    const controlDir=join(state,'gator/work-control');const owner=await until(async()=>{for(const name of await readdir(controlDir)){if(name.endsWith('.json')){const v=JSON.parse(await readFile(join(controlDir,name),'utf8'));if(v.inspection)return v;}}});
    let spawned=0;companion=createCompanion({binary,workspace,env,tokenFile,spawnBackend(){spawned++;throw new Error('competing worker');}});const addr=await companion.listen(),base=`http://127.0.0.1:${addr.port}`,headers={Authorization:`Bearer ${token}`};
    await execute(process.execPath,[resolve('companion/inspection-access.mjs'),binary,tokenFile,owner.run_id,device.id,'60'],{env});
    const action={id:'a'.repeat(32),run_id:owner.run_id,target:owner.inspection,operation:'cancel'},post=a=>fetch(`${base}/v1/work/${a.target.id}/actions`,{method:'POST',headers:{...headers,'Content-Type':'application/json'},body:JSON.stringify(a)});
    assert.equal((await post(action)).status,409,'Inspect must not cancel');assert.equal(held,true);assert.equal(host.closed,false);
    await execute(process.execPath,[resolve('companion/control-access.mjs'),binary,tokenFile,owner.run_id,device.id,'cancel','','60'],{env});
    const detail=await(await fetch(`${base}/v1/work/${action.target.id}?generation=${action.target.generation}`,{headers})).json();assert.equal(detail.targets[0].controls[0].operation,'cancel');
    const before=calls;
    const responses=await Promise.all([post(action),post(action)]);for(const response of responses){assert.equal(response.status,200,await response.clone().text());const value=await response.json();assert.ok(['dispatch_acknowledged','outcome_unknown'].includes(value.receipt.delivery));assert.notEqual(value.receipt.note,'Execution terminated');}
    await until(()=>!host.state?.running);
    const path=`${base}/v1/work/${action.target.id}/receipts/${action.id}?run_id=${action.run_id}`;
    const receipt=await(await fetch(path,{headers})).json();assert.equal(receipt.receipt.id,action.id);assert.equal(receipt.observation,'work_lease_stopped');assert.ok(!JSON.stringify(receipt).includes('token_file'));
    await host.stop();const repeat=await post(action);assert.equal(repeat.status,200);assert.equal((await repeat.json()).receipt.digest,receipt.receipt.digest);
    const conflict={...action,operation:'steer',specialist:'subagent-001',instruction:'Different payload'};assert.equal((await post(conflict)).status,409);
    assert.equal(calls,before);assert.equal(spawned,0);
    await revokeDevice(tokenFile,device.id);assert.equal((await fetch(path,{headers})).status,401);
  }finally{release();await host?.stop();await companion?.close();await new Promise(r=>provider.close(r));await rm(root,{recursive:true,force:true});}
});

test('revocation while a mutation waits in the gateway queue prevents dispatch',async()=>{
  const root=await mkdtemp(join(tmpdir(),'gator-control-queued-')),tokenFile=join(root,'owner.token');let companion,release,entered;const gate=new Promise(r=>release=r),started=new Promise(r=>entered=r);let checks=0,dispatches=0;
  try{await writeFile(tokenFile,'a'.repeat(64)+'\n',{mode:0o600});const exported=join(root,'phone.token'),device=await addDevice(tokenFile,'Phone',exported),token=(await readFile(exported,'utf8')).trim();
    companion=createCompanion({binary:'',workspace:root,tokenFile,queryWork:async()=>{entered();await gate;return{targets:[]};},controlWork:async mode=>{if(mode==='dispatch'){dispatches++;return{}}checks++;return{authorized:true};}});const addr=await companion.listen(),base=`http://127.0.0.1:${addr.port}`,headers={Authorization:`Bearer ${token}`};
    const read=fetch(base+'/v1/work',{headers});await started;
    const action={id:'a'.repeat(32),run_id:'run',target:{id:'target-'+'a'.repeat(32),backend:'gator_work',owner:'owner',generation:'a'.repeat(32),incarnation:'one'},operation:'cancel'};
    const post=fetch(base+`/v1/work/${action.target.id}/actions`,{method:'POST',headers:{...headers,'Content-Type':'application/json'},body:JSON.stringify(action)});await until(async()=>{const r=await fetch(base+'/v1/health',{headers});return(await r.json()).pending_requests>=2;});await revokeDevice(tokenFile,device.id);release();await read;assert.equal((await post).status,401);assert.equal(dispatches,0);
  }finally{release();await companion?.close();await rm(root,{recursive:true,force:true});}
});
test('semantic HTTP shapes reject actor, grant, endpoint and terminal input injection',()=>{
  const target='target-'+'a'.repeat(32),a={id:'a'.repeat(32),run_id:'run',target:{id:target,backend:'gator_work',owner:'owner',generation:'b'.repeat(32),incarnation:'one'},operation:'cancel'};
  for(const extra of [{actor:'trusted'},{grant:{}},{endpoint:'http://127.0.0.1:1'},{instruction:'raw input'},{operation:'text_input'},{operation:'interrupt'}])assert.throws(()=>validateRemoteAction({...a,...extra},target));
});
