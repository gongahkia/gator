import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,writeFile,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {createCompanion} from './server.mjs';
import {addDevice,revokeDevice} from './auth.mjs';
import {validateExternal} from './external.mjs';
const execute=promisify(execFile);
export function fixture(d='a',snapshot=false){return {target:{binding:{id:'target-'+d.repeat(32),backend:'external_tmux',owner:'tmux-instance-'+d.repeat(32),generation:d.repeat(32),incarnation:d.repeat(64)},label:'Pane <img src=x onerror="window.injected=true"> '+d,capabilities:['inspect'],available:true},text:snapshot?'<script>window.injected=true</script> 猫':'',truncated:snapshot,stale:false,...(snapshot?{captured_at:'2026-10-31T03:00:00Z'}:{})};}
test('external transport isolates native configuration, rejects mutations and revokes queued reads',{timeout:30000},async()=>{
 const root=await mkdtemp(join(tmpdir(),'gator-external-http-')),tokenFile=join(root,'token');let c,release,entered;
 try{
  await writeFile(tokenFile,'a'.repeat(64)+'\n',{mode:0o600});const phone=await addDevice(tokenFile,'Phone',join(root,'phone'));const token=(await readFile(join(root,'phone'),'utf8')).trim();
  let calls=0,hold=false; c=createCompanion({binary:'',workspace:root,tokenFile,spawnBackend(){throw new Error('External read created Work owner');},queryExternal:async({actor,mode,target,generation})=>{calls++;assert.match(actor,/^inspection-[a-f0-9]{64}$/);if(hold){entered?.();await new Promise(r=>release=r);}return {targets:[{...fixture('a',mode==='snapshot'),config:{socket:'/secret',token:'secret'}}]};}});
  const address=await c.listen(),base=`http://127.0.0.1:${address.port}`,headers={Authorization:`Bearer ${token}`},b=fixture().target.binding,path=`/v1/external/${b.id}/snapshot?generation=${b.generation}`;
  const page=await (await fetch(base+'/v1/external',{headers})).json();assert.equal(page.targets.length,1);assert.equal(JSON.stringify(page).includes('/secret'),false);
  assert.equal((await fetch(base+path,{headers})).status,200);
  assert.equal((await fetch(base+path,{headers,method:'POST'})).status,405);
  assert.equal((await fetch(base+path+'&socket=/tmp/other',{headers})).status,400);
  assert.equal((await fetch(base+'/v1/external')).status,401);
  hold=true;const gate=new Promise(r=>entered=r),first=fetch(base+path,{headers});await gate;const second=fetch(base+path,{headers});await new Promise(r=>setTimeout(r,30));await revokeDevice(tokenFile,phone.id);release();assert.equal((await first).status,401);assert.equal((await second).status,401);assert.equal(calls,3,'queued revoked read must not invoke backend');
 }finally{release?.();await c?.close();await rm(root,{recursive:true,force:true});}
});
test('external projection rejects wrong binding, oversized output and Work grants',()=>{const b=fixture('a',true).target.binding;for(const item of [{...fixture('a',true),text:'x'.repeat(16385)},{...fixture('b',true)},{...fixture('a',true),target:{...fixture().target,binding:{...b,backend:'gator_work'}}},{...fixture('a',true),target:{...fixture().target,capabilities:['inspect','text_input']}}])assert.throws(()=>validateExternal({targets:[item]},b.id,b.generation,true));});

test('real isolated tmux pane enrolls, receives device-specific Inspect, captures and invalidates reused IDs',{skip:process.env.GATOR_TMUX_LIVE!=='1'||process.platform==='win32',timeout:120000},async()=>{
 const root=await mkdtemp(join(tmpdir(),'gt3-http-')),socket=join(root,'socket'),binary=join(root,'runtime'),tokenFile=join(root,'token');let c;
 const mux=process.env.GATOR_TEST_TMUX||'/usr/local/bin/tmux';
 const env={...process.env,GATOR_STATE_DIR:join(root,'state'),GATOR_INTERNAL_COMMANDS:'1'};
 const tmux=(...a)=>execute(mux,['-f','/dev/null','-S',socket,...a],{timeout:5000});
 try{
  await execute('go',['build','-o',binary,'./cmd/gator'],{timeout:90000});
  await writeFile(tokenFile,'a'.repeat(64)+'\n',{mode:0o600});const phone=await addDevice(tokenFile,'Phone',join(root,'phone'));const token=(await readFile(join(root,'phone'),'utf8')).trim();
  const {stdout}=await tmux('new-session','-d','-s','fixture','-x','80','-y','24','cat');const pane=(await tmux('display-message','-p','#{pane_id}')).stdout.trim();
  const enrolled=JSON.parse((await execute(binary,['external-tmux','enroll','--reviewed-read-config',mux,socket,pane,'External fixture'],{env})).stdout);
  c=createCompanion({binary,workspace:root,env,tokenFile,spawnBackend(){throw new Error('Inspection must not create Work');}});const address=await c.listen(),base=`http://127.0.0.1:${address.port}`,headers={Authorization:`Bearer ${token}`};
  const get=async path=>{const r=await fetch(base+path,{headers});assert.equal(r.status,200,await r.clone().text());return r.json();};
  assert.deepEqual((await get('/v1/external')).targets,[],'enrollment is not device authorization');
  await execute(process.execPath,[resolve('companion/external-access.mjs'),binary,tokenFile,enrolled.binding.id,phone.id,'60'],{env});
  assert.equal((await get('/v1/external')).targets.length,1);
  await tmux('send-keys','-t',pane,'T3 private <script>猫</script>','Enter');
  const path=`/v1/external/${enrolled.binding.id}/snapshot?generation=${enrolled.binding.generation}`;
  const snap=await get(path);assert.match(snap.targets[0].text,/T3 private/);assert.equal(snap.targets[0].target.binding.backend,'external_tmux');assert.equal(JSON.stringify(snap).includes(socket),false);
  await execute(process.execPath,[resolve('companion/external-access.mjs'),binary,tokenFile,enrolled.binding.id,phone.id,'0'],{env});assert.equal((await get(path)).unavailable,true);
  await execute(process.execPath,[resolve('companion/external-access.mjs'),binary,tokenFile,enrolled.binding.id,phone.id,'60'],{env});
  await execute(binary,['external-tmux','unreview',enrolled.binding.id],{env});
  assert.equal((await get(path)).unavailable,true,'local trust withdrawal prevents output');
  await assert.rejects(execute(process.execPath,[resolve('companion/external-access.mjs'),binary,tokenFile,enrolled.binding.id,phone.id,'60'],{env}));
  const renewed=JSON.parse((await execute(binary,['external-tmux','enroll','--reviewed-read-config',mux,socket,pane,'Renewed trust'],{env})).stdout);
  assert.notEqual(renewed.binding.generation,enrolled.binding.generation);
  await execute(process.execPath,[resolve('companion/external-access.mjs'),binary,tokenFile,renewed.binding.id,phone.id,'60'],{env});
  await tmux('kill-server');await tmux('new-session','-d','-s','fixture','cat');
  const renewedPath=`/v1/external/${renewed.binding.id}/snapshot?generation=${renewed.binding.generation}`;
  assert.equal((await get(renewedPath)).unavailable,true,'replacement does not inherit renewed trust');assert.equal((await get(path)).unavailable,true,'reused pane ID from replacement server must not inherit grant');
  assert.equal((await get('/v1/external')).targets.length,0);
 }finally{await c?.close();await tmux('kill-server').catch(()=>{});await rm(root,{recursive:true,force:true});}
});
