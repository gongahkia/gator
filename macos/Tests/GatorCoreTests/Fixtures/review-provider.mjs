// Disposable local provider and fake action sink. Never contacts real accounts.
import http from 'node:http';
import { appendFile, access } from 'node:fs/promises';
import { join } from 'node:path';
const root=process.env.GATOR_FIXTURE_CONTROL;
if(!root)throw Error('Disposable fixture directory required');
const trace=async value=>appendFile(join(root,'review-trace.jsonl'),JSON.stringify(value)+'\n');
let actionCalls=0;
async function revisionGate(res){
 try{await access(join(root,'hold-revision'));}catch{return;}
 while(!res.destroyed){try{await access(join(root,'release-revision'));return;}catch{}await new Promise(r=>setTimeout(r,20));}
}
const server=http.createServer(async(req,res)=>{
 try{
 let raw='';for await(const part of req)raw+=part;
 const body=JSON.parse(raw||'{}');
 if(req.url.includes('/gmail/')){
  await trace({kind:'effect',body});res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({id:'local-fixture-only'}));return;
 }
 const messages=body.messages||[],system=messages.filter(m=>m.role==='system').map(m=>m.content).join('\n');
 let message;
 if(system.includes('You handle the conversation with Gator')){
  const context=JSON.parse(messages.at(-1).content);await trace({kind:'route',context});
  let decision;
  if(context.message==='Hello during review')decision={kind:'reply',text:'You can keep talking while review is pending.'};
  else if(context.message==='Send the venue update')decision={kind:'task'};
  else if(context.message.includes('plan task')||context.tasks.some(t=>t.can_revise_plan)&&context.message.includes('budget'))decision={kind:'revise_plan',target:context.tasks.find(t=>t.can_revise_plan)?.id,instruction:context.message};
  else decision={kind:'revise_action',target:context.tasks.find(t=>t.can_correct_action)?.id,instruction:context.message};
  message={role:'assistant',content:JSON.stringify(decision)};
 }else if(system.includes('Propose a plan')){
  const context=JSON.parse(messages.at(-1).content);await trace({kind:'plan',context});
  if(context.correction)await revisionGate(res);
  message={role:'assistant',content:JSON.stringify({outcome:context.correction?'Venue comparison under $2,000':'A reviewed venue comparison',steps:[{id:'compare',text:context.correction?'Compare venues within the $2,000 budget':'Compare venues in the selected scope'}],sources:[],effects:['read']})};
 }else if(system.includes("Classify the user's immediate request")){
  message={role:'assistant',content:'{"mode":"act"}'};
 }else if(JSON.stringify(messages).includes('Hold another task')){
  await trace({kind:'held-work'});
  while(!res.destroyed){try{await access(join(root,'release-main'));break;}catch{}await new Promise(r=>setTimeout(r,20));}
  message={role:'assistant',content:'The unrelated task finished.'};
 }else{
  actionCalls++;await trace({kind:'action-model',call:actionCalls});
  message=actionCalls<=2?{role:'assistant',tool_calls:[{id:'proposal-'+actionCalls,type:'function',function:{name:'connector_google_gmail_send',arguments:JSON.stringify({payload:{from:'me@example.com',to:'venue@example.com',subject:actionCalls===1?'Original subject':'Project Update',body:'Preserved body\n'+('Long review paragraph.\n'.repeat(35))+'FINAL REVIEW LINE'}})}}]}:{role:'assistant',content:'The reviewed action outcome is retained. Inspect its receipt; declined proposals were not sent.'};
 }
 if(!res.destroyed){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message,finish_reason:message.tool_calls?'tool_calls':'stop'}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));}
 }catch(error){await trace({kind:'fixture-error',error:String(error)});if(!res.destroyed){res.writeHead(500);res.end('Controlled fixture failed');}}
});
server.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
