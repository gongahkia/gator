import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createCompanion} from './server.mjs';
const {chromium}=await import(process.env.GATOR_PLAYWRIGHT_MODULE||'../desktop/node_modules/playwright/index.mjs');
const item=(d,snapshot=false)=>({target:{binding:{id:'target-'+d.repeat(32),backend:'external_tmux',owner:'tmux-instance-'+d.repeat(32),generation:d.repeat(32),incarnation:d.repeat(64)},label:'Pane '+d,capabilities:['inspect'],available:true},text:snapshot?`<script>window.injected=true</script> pane-${d} 猫`:'',truncated:snapshot,stale:false,...(snapshot?{captured_at:'2026-10-31T03:00:00Z'}:{})});
test('external mobile snapshots stay text, obey A/B selection, clear revoked content and pause polling',{timeout:60000},async()=>{
 const root=await mkdtemp(join(tmpdir(),'gator-external-browser-')),tokenFile=join(root,'token'),token='a'.repeat(64);let c,browser,release,held=false,hold=false,reads=0,shared=true;
 try{
  await writeFile(tokenFile,token+'\n',{mode:0o600});c=createCompanion({binary:'',workspace:root,tokenFile,queryCatalog:async()=>({items:[],next:'',incomplete:false}),spawnBackend(){throw new Error('External read spawned Work');},queryExternal:async({mode,target})=>{reads++;if(mode==='list')return {targets:shared?[item('a'),item('b')]:[]};if(target===item('a').target.binding.id&&hold){held=true;await new Promise(r=>release=r);}return {targets:shared?[item(target===item('a').target.binding.id?'a':'b',mode==='snapshot')]:[],unavailable:!shared};}});
  const address=await c.listen();browser=await chromium.launch({headless:true});const context=await browser.newContext({viewport:{width:390,height:844}}),page=await context.newPage();await page.goto(`http://127.0.0.1:${address.port}`);await page.locator('#token').fill(token);await page.getByRole('button',{name:'Connect',exact:true}).click();await page.locator('#external-open summary').click();await page.waitForFunction(()=>document.querySelector('#external-select').options.length===3);
  await page.locator('#external-select').selectOption(item('a').target.binding.id);await page.locator('#external-output').filter({hasText:'pane-a'}).waitFor();assert.equal(await page.locator('#external-output script').count(),0);assert.equal(await page.evaluate(()=>window.injected),undefined);
  hold=true;await page.locator('#external-refresh').click();for(let i=0;i<100&&!held;i++)await page.waitForTimeout(20);assert.equal(held,true);await page.locator('#external-select').selectOption(item('b').target.binding.id);hold=false;release();await page.locator('#external-output').filter({hasText:'pane-b'}).waitFor();assert.doesNotMatch(await page.locator('#external-meta').textContent(),/Pane a/);
  shared=false;await page.locator('#external-refresh').click();await page.waitForFunction(()=>document.querySelector('#external-output').textContent==='');
  await page.locator('#external-open summary').click();await page.waitForTimeout(150);const before=reads;await page.waitForTimeout(5200);assert.equal(reads,before,'closed view must not poll');
  shared=true;await page.locator('#external-open summary').click();await page.waitForFunction(()=>document.querySelector('#external-select').options.length===3);await page.locator('#external-select').selectOption(item('a').target.binding.id);await page.locator('#external-output').filter({hasText:'pane-a'}).waitFor();await context.setOffline(true);await page.waitForFunction(()=>document.querySelector('#external-output').textContent==='');await context.setOffline(false);
  await writeFile(tokenFile,'b'.repeat(64)+'\n',{mode:0o600});await page.waitForFunction(()=>document.querySelector('#external-terminals').hidden);assert.equal(await page.locator('#external-output').textContent(),'');
 }finally{release?.();await browser?.close();await c?.close();await rm(root,{recursive:true,force:true});}
});
