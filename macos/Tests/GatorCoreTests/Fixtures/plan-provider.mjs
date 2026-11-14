// Local controlled model only. No tools, network accounts or external effects.
import http from 'node:http';
import {appendFile,access} from 'node:fs/promises';
import {join} from 'node:path';
const root=process.env.GATOR_FIXTURE_CONTROL;
if(!root)throw Error('Disposable fixture root required');
const trace=value=>appendFile(join(root,'plan-trace.jsonl'),JSON.stringify(value)+'\n');
async function gate(res,name){
 try{await access(join(root,'hold-'+name));}catch{return;}
 while(!res.destroyed){try{await access(join(root,'release-'+name));return;}catch{}await new Promise(r=>setTimeout(r,20));}
}
const server=http.createServer(async(req,res)=>{
 try{
 let raw='';for await(const part of req)raw+=part;const body=JSON.parse(raw||'{}');
 const messages=body.messages||[],system=messages.filter(m=>m.role==='system').map(m=>m.content).join('\n');let message;
 if(system.includes('You handle the conversation with Gator')){
  const context=JSON.parse(messages.at(-1).content);await trace({kind:'route',context});
  let decision;
  if(context.message.includes('plan task'))decision={kind:'revise_plan',target:context.tasks.find(t=>t.can_revise_plan)?.id,instruction:context.message};
  else if(context.message==='Start independent comparison')decision={kind:'task'};
  else decision={kind:'reply',text:'An independent conversation reply while the reviewed work continues.'};
  message={role:'assistant',content:JSON.stringify(decision)};
 }else if(system.includes('Propose a plan')){
  const context=JSON.parse(messages.at(-1).content);await trace({kind:'plan',context});
  if(context.correction)await gate(res,'revision');
  message={role:'assistant',content:JSON.stringify({outcome:context.correction?'Comparison under $2,000':'A reviewed comparison',steps:[{id:'compare',text:context.correction?'Compare within the $2,000 budget':'Compare the selected options'}],sources:[],effects:['read']})};
 }else if(system.includes("Classify the user's immediate request")){
  message={role:'assistant',content:'{"mode":"inspect"}'};
 }else{
  await trace({kind:'execution',users:messages.filter(m=>m.role==='user').map(m=>m.content)});
  await gate(res,'execution');
  message={role:'assistant',content:'The exact reviewed comparison is complete: jade heron.'};
 }
 if(!res.destroyed){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message,finish_reason:'stop'}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));}
 }catch(error){await trace({kind:'fixture-error',error:String(error)});if(!res.destroyed){res.writeHead(500);res.end('Local plan fixture failed');}}
});
server.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
