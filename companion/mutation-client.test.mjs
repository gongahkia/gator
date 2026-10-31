import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp,writeFile,rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { createCompanion } from './server.mjs';
const {chromium}=await import(process.env.GATOR_PLAYWRIGHT_MODULE||'../desktop/node_modules/playwright/index.mjs');

test('semantic phone controls bind review to selection, invalidate revoked grants and reconcile unknown receipts without replay', {timeout:90000},async()=>{
  const root=await mkdtemp(join(tmpdir(),'gator-control-browser-')),tokenFile=join(root,'owner.token'),token='a'.repeat(64);let companion,browser,granted=true,online=true,dispatches=0,reads=0,sent,receipt;
  const offer={operation:'steer',scope:'subagent-001',expires_at:new Date(Date.now()+60000).toISOString(),specialist:'subagent-001'};
  const item=d=>({target:{binding:{id:'target-'+d.repeat(32),backend:'gator_work',owner:'owner-'+d,generation:d.repeat(32),incarnation:'one'},label:'Work '+d,available:true,capabilities:['inspect']},run_id:'run-'+d,conversation_id:'chat-'+d,status:'running',started_at:new Date().toISOString(),observed_at:new Date().toISOString(),updated_at:new Date().toISOString(),progress:[],specialists:[],pending_approvals:0,limitations:['Fixture'],controls:granted?[offer]:[]});
  try{
    await writeFile(tokenFile,token+'\n',{mode:0o600});companion=createCompanion({binary:'',workspace:root,tokenFile,queryCatalog:async()=>({items:[]}),queryWork:async({target})=>online?{targets:target?[item(target.endsWith('a')?'a':'b')]:[item('a'),item('b')]}:{targets:[],unavailable:true},controlWork:async(mode,input)=>{
      if(mode==='check')return{authorized:granted};if(mode==='receipt'){reads++;return{receipt};}
      dispatches++;sent=input.action;receipt={version:1,id:sent.id,run_id:sent.run_id,target:sent.target,operation:sent.operation,scope:offer.scope,digest:'c'.repeat(64),phase:'claimed',delivery:'outcome_unknown',note:'Dispatch may have started; reconcile without replay.',created_at:new Date().toISOString(),updated_at:new Date().toISOString()};return{receipt};
    }});const addr=await companion.listen(),url=`http://127.0.0.1:${addr.port}`;browser=await chromium.launch({headless:true});const page=await browser.newPage({viewport:{width:390,height:844}});
    async function connect(){await page.locator('#token').fill(token);await page.getByRole('button',{name:'Connect',exact:true}).click();if(!await page.locator('#work-open').evaluate(e=>e.open))await page.locator('#work-title').click();await page.waitForFunction(()=>document.querySelector('#work-select').options.length>=3);}
    await page.goto(url);await connect();await page.locator('#work-select').selectOption('target-'+'a'.repeat(32));await page.waitForFunction(()=>document.querySelector('#work-operation').options.length===2);
    async function draft(text){await page.locator('#work-operation').selectOption(JSON.stringify(offer));await page.locator('#work-instruction').fill(text);await page.locator('#work-action-review').click();await page.locator('#work-action-dialog').waitFor({state:'visible'});}
    await draft('Must stay with A');
    await page.evaluate(()=>{const s=document.querySelector('#work-select');s.value='target-'+'b'.repeat(32);s.dispatchEvent(new Event('change'));});
    await page.locator('#work-action-dialog').waitFor({state:'hidden'});await page.locator('#work-meta').filter({hasText:'chat-b'}).waitFor();assert.equal(await page.locator('#work-instruction').inputValue(),'');assert.equal(dispatches,0);
    await page.locator('#work-select').selectOption('target-'+'a'.repeat(32));await page.locator('#work-meta').filter({hasText:'chat-a'}).waitFor();await draft('Revoke before confirmation');granted=false;
    await page.locator('#work-action-dialog').waitFor({state:'hidden'});assert.equal(await page.locator('#work-action-review').isDisabled(),true);assert.equal(dispatches,0);
    granted=true;await page.locator('#work-refresh').click();await page.waitForFunction(()=>document.querySelector('#work-operation').options.length===2);await draft('Sensitive instruction for A');await page.locator('#work-action-confirm').click();
    await page.locator('#work-action-status').filter({hasText:'outcome_unknown'}).waitFor();assert.equal(dispatches,1);assert.equal(sent.target.id,'target-'+'a'.repeat(32));
    const saved=await page.evaluate(()=>Object.keys(sessionStorage).filter(k=>k.startsWith('gator.work-receipt.')).map(k=>sessionStorage.getItem(k)).join(''));assert.doesNotMatch(saved,/Sensitive instruction/);
    await page.locator('#work-receipt-refresh').click();for(let i=0;i<100&&reads<1;i++)await page.waitForTimeout(20);assert.equal(dispatches,1);assert.ok(reads>=1);assert.equal(await page.locator('#work-receipt-dismiss').isVisible(),false);
    online=false;await page.reload();await page.locator('#token').fill(token);await page.getByRole('button',{name:'Connect',exact:true}).click();await page.locator('#work-title').click();
    await page.waitForFunction(()=>Array.from(document.querySelector('#work-select').options).some(o=>o.textContent.includes('Retained action receipt')));
    await page.locator('#work-select').selectOption(sent.target.id);await page.locator('#work-receipt-refresh').waitFor({state:'visible'});await page.locator('#work-receipt-refresh').click();await page.locator('#work-action-status').filter({hasText:'outcome_unknown'}).waitFor();assert.equal(dispatches,1,'reload/receipt read must never resend');
  }finally{await browser?.close();await companion?.close();await rm(root,{recursive:true,force:true});}
});
