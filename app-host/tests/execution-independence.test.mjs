import test from 'node:test';
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdtemp,mkdir,writeFile,readdir,rm,access,symlink} from 'node:fs/promises';
import {constants} from 'node:fs';
import {tmpdir} from 'node:os';
import {join,delimiter} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createServer} from 'node:http';
import {Backend} from '../src/backend.mjs';
const execute=promisify(execFile),source=fileURLToPath(new URL('../../',import.meta.url));

test('ordinary Work and desktop/OpenTUI hosts need no tmux, Onibi, phone or external catalog',{skip:process.platform==='win32',timeout:180000},async()=>{
 const root=await mkdtemp(join(tmpdir(),'gator-independent-')),bin=join(root,'bin'),workspace=join(root,'workspace'),config=join(root,'config'),state=join(root,'state');let host,requests=0;
 const provider=createServer(async(req,res)=>{let body='';for await(const part of req)body+=part;const p=JSON.parse(body);requests++;
  const classify=p.messages?.some(m=>m.role==='system'&&m.content?.includes("Classify the user's immediate request"));
  res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message:{role:'assistant',content:classify?'{"mode":"inspect"}':'Independent local fixture completed.'},finish_reason:'stop'}],usage:{prompt_tokens:1,completion_tokens:1}}));
 });
 try{
  await mkdir(bin);await mkdir(workspace);await mkdir(join(config,'gator'),{recursive:true});
  // Allow only Git on PATH: native terminal programs and network helpers are
  // actually absent, rather than mocked as healthy. Go runtime uses its absolute path.
  let git;for(const dir of (process.env.PATH||'').split(delimiter)){const path=join(dir,'git');try{await access(path,constants.X_OK);git=path;break;}catch{}}
  assert.ok(git,'existing source capture uses Git');await symlink(git,join(bin,'git'));
  for(const tool of ['tmux','onibi','tailscale'])await assert.rejects(access(join(bin,tool)));
  const binary=process.env.GATOR_INDEPENDENCE_BINARY||join(root,'gator-runtime');
  if(!process.env.GATOR_INDEPENDENCE_BINARY)await execute('go',['build','-o',binary,'./cmd/gator'],{cwd:source,timeout:90000});
  await new Promise(r=>provider.listen(0,'127.0.0.1',r));
  await writeFile(join(config,'gator/config.json'),JSON.stringify({version:4,defaults:{provider:'fixture',model:'fixture'},custom_providers:[{id:'fixture',base_url:`http://127.0.0.1:${provider.address().port}/chat`,models:['fixture'],default_model:'fixture'}]}));
  const env={PATH:bin,HOME:process.env.HOME,GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:'/dev/null',GATOR_CONFIG_DIR:config,GATOR_STATE_DIR:state,GATOR_INTERNAL_COMMANDS:'1',GATOR_UI_SESSION_HOST:'1'};
  await execute(binary,['work','--source',workspace,'--mode','inspect','Independent fixture'],{cwd:workspace,env,timeout:45000});
  assert.ok(requests>0,'real Work must reach the fixture provider');
  const id=(await readdir(join(state,'gator/conversations'))).find(n=>n.startsWith('work-'));assert.ok(id);
  for(const frontend of ['desktop','terminal']){
   host=new Backend(binary,workspace,{...env,GATOR_UI_FRONTEND:frontend},['ui-backend',id,'']);await host.ready();assert.equal(host.state.conversation_id,id);await host.stop();host=undefined;
  }
  await execute(binary,['note','add','--title','Independent','--body','Local-only data'],{cwd:workspace,env,timeout:10000});
  assert.match((await execute(binary,['note','list'],{cwd:workspace,env,timeout:10000})).stdout,/Independent/);
  await assert.rejects(access(join(state,'gator/external-targets')),'ordinary startup must not open the external catalog');
  await assert.rejects(access(join(state,'gator/external-actions/.terminal')),'ordinary Work must not initialize terminal admission');
 }finally{await host?.stop();await new Promise(r=>provider.close(r));await rm(root,{recursive:true,force:true});}
});
