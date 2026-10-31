import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createCompanion} from './server.mjs';
const {chromium}=await import('../desktop/node_modules/playwright/index.mjs');
const item=(d,snapshot)=>({target:{binding:{id:'target-'+d.repeat(32),backend:'external_tmux',owner:'tmux-instance-'+d.repeat(32),generation:d.repeat(32),incarnation:d.repeat(64)},label:'Pane '+d,available:true,capabilities:['inspect']},controls:['text_input','key_input','interrupt'],text:snapshot?'<script>untrusted output</script>':'',captured_at:snapshot?'2026-10-31T00:00:00Z':undefined,truncated:false,stale:false});
test('review binds exact target; drafts stay isolated; unknown receipts reconcile without replay',{timeout:90000},async()=>{
 const root=await mkdtemp(join(tmpdir(),'gt4-browser-')),tokenFile=join(root,'token'),token='a'.repeat(64);let c,browser,hidden=false;const dispatched=[],receipts=new Map();
 try{
  await writeFile(tokenFile,token+'\n',{mode:0o600});
  c=createCompanion({binary:'',workspace:root,tokenFile,queryCatalog:async()=>({items:[],next:'',incomplete:false}),spawnBackend(){throw new Error('T4 spawned Work');},queryExternal:async({mode,target})=>({targets:hidden?[]:(mode==='list'?[item('a'),item('b')]:[{...item(target==='target-'+'a'.repeat(32)?'a':'b',true),captured_at:new Date().toISOString()}])}),controlTerminal:async(mode,{action})=>{
   if(mode==='check')return {authorized:true};if(mode==='receipt')return {receipt:receipts.get(action.id)};
   dispatched.push(action);const receipt={id:action.id,target:{id:action.target,generation:action.generation,backend:'external_tmux'},operation:'text_input',scope:action.input.kind,digest:'c'.repeat(64),phase:'claimed',delivery:'outcome_unknown',note:'Input may have been delivered.',created_at:new Date().toISOString(),updated_at:new Date().toISOString()};receipts.set(action.id,receipt);return {receipt};
  }});
  const address=await c.listen();browser=await chromium.launch({headless:true});const context=await browser.newContext({viewport:{width:390,height:844}}),page=await context.newPage();
  const connect=async()=>{await page.locator('#token').fill(token);await page.getByRole('button',{name:'Connect',exact:true}).click();await page.locator('#external-open summary').click();await page.waitForFunction(min=>document.querySelector('#external-select').options.length>=min,hidden?2:3);};
  await page.goto(`http://127.0.0.1:${address.port}`);await connect();await page.locator('#external-select').selectOption(item('a').target.binding.id);await page.locator('#terminal-draft').fill('A-private-draft');await page.locator('#terminal-text').click();await page.locator('#external-action-dialog').waitFor({state:'visible'});
  assert.match(await page.locator('#terminal-review').textContent(),/A-private-draft/);
  await page.evaluate(id=>{const s=document.querySelector('#external-select');s.value=id;s.dispatchEvent(new Event('change'));},item('b').target.binding.id);
  await page.locator('#external-action-dialog').waitFor({state:'hidden'});await page.locator('#external-meta').filter({hasText:'Pane b'}).waitFor();assert.equal(dispatched.length,0);assert.equal(await page.locator('#terminal-draft').inputValue(),'');
  await page.locator('#external-select').selectOption(item('a').target.binding.id);await page.locator('#external-meta').filter({hasText:'Pane a'}).waitFor();assert.equal(await page.locator('#terminal-draft').inputValue(),'A-private-draft');
  await page.locator('#external-select').selectOption(item('b').target.binding.id);await page.locator('#terminal-draft').fill('B-reviewed-text');await page.locator('#terminal-text_enter').click();assert.match(await page.locator('#terminal-review').textContent(),/Enter appended: yes/);await page.locator('#terminal-confirm').click();
  await page.locator('#terminal-status').filter({hasText:'Outcome unknown'}).waitFor();assert.equal(dispatched.length,1);assert.equal(dispatched[0].target,item('b').target.binding.id);assert.equal(dispatched[0].input.text,'B-reviewed-text');assert.equal(await page.locator('#terminal-dismiss').isVisible(),false);
  const saved=await page.evaluate(()=>JSON.stringify({...sessionStorage}));assert.doesNotMatch(saved,/B-reviewed-text|A-private-draft/);await page.locator('#terminal-reconcile').click();await page.locator('#terminal-status').filter({hasText:'Outcome unknown'}).waitFor();assert.equal(dispatched.length,1);
  hidden=true;await page.locator('#external-refresh').click();await page.waitForFunction(()=>document.querySelector('#external-output').textContent==='');await page.locator('#terminal-reconcile').click();await page.locator('#terminal-status').filter({hasText:'Outcome unknown'}).waitFor();assert.equal(dispatched.length,1);
  await page.reload();await connect();await page.locator('#external-select').selectOption(item('b').target.binding.id);await page.locator('#terminal-reconcile').click();await page.locator('#terminal-status').filter({hasText:'Outcome unknown'}).waitFor();assert.equal(dispatched.length,1,'reload must not replay input');
 }finally{await browser?.close();await c?.close();await rm(root,{recursive:true,force:true});}
});
