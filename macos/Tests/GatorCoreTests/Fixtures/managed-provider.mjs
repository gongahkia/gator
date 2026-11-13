// Controlled Chat Completions provider, exclusively for disposable native tests.
// Held work is released by files in GATOR_FIXTURE_CONTROL; no connected services.
import http from 'node:http';
import { appendFile, access } from 'node:fs/promises';
import { join } from 'node:path';
const control = process.env.GATOR_FIXTURE_CONTROL;
if (!control) throw Error('A disposable GATOR_FIXTURE_CONTROL directory is required');
const trace = async value => appendFile(join(control,'trace.jsonl'), JSON.stringify(value)+'\n');
async function held(name, res) {
  while (!res.destroyed) {
    try { await access(join(control,'release-'+name)); return; } catch {}
    await new Promise(resolve=>setTimeout(resolve,25));
  }
}
const server=http.createServer(async(req,res)=>{
 try {
  let raw='';for await(const part of req)raw+=part;
  const body=JSON.parse(raw||'{}'), messages=body.messages||[];
  const system=messages.filter(m=>m.role==='system').map(m=>String(m.content)).join('\n');
  let content;
  if(system.includes('You handle the conversation with Gator')) {
    const context=JSON.parse(messages.at(-1).content);await trace({kind:'route',message:context.message,tasks:context.tasks});
    if(context.message==='Hello while working')content={kind:'reply',text:'I am here. Your original venue comparison is still working.'};
    else if(context.message==='Keep the venues under $2,000')content={kind:'guide',target:context.tasks.find(t=>t.title.includes('Compare venues'))?.id,instruction:'Keep the venues under $2,000'};
    else if(context.message==='Stop the separate task')content={kind:'stop',target:context.tasks.find(t=>t.title.includes('Read separate brief'))?.id};
    else if(context.message==='Guide research locally')content={kind:'guide',target:context.tasks.find(t=>t.title.includes('Research a stop target')&&t.can_guide)?.id,instruction:'Keep the research local'};
    else if(context.message==='Stop research target')content={kind:'stop',target:context.tasks.find(t=>t.title.includes('Research a stop target')&&t.can_stop)?.id};
    else if(context.message==='What did the separate brief say?')content={kind:'reply',text:context.tasks.some(t=>t.state==='completed'&&t.summary?.includes('jade heron'))?'The separate brief says jade heron.':'The separate brief result is unconfirmed.'};
    else content={kind:'task'};
    content=JSON.stringify(content);
  } else if(system.includes("Classify the user's immediate request")) {
    content='{"mode":"inspect"}';
  } else {
    const user=messages.find(m=>m.role==='user')?.content;
    const objective=typeof user==='string'?user:(user||[]).map(p=>p.text||'').join('\n');
    const name=objective.includes('Read separate brief')?'child':objective.includes('Research a stop target')?'stop':'main';
    await trace({kind:'work',name,objective});
    await held(name,res);
    if(res.destroyed)return;
    content=name==='child'?'The separate brief says jade heron.':JSON.stringify(messages).includes('$2,000')?'Venue comparison respects the $2,000 budget.':'Venue comparison prepared.';
  }
  if(!res.destroyed){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message:{role:'assistant',content},finish_reason:'stop'}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));}
 } catch(error) {await trace({kind:'fixture-error',error:String(error)});if(!res.destroyed){res.writeHead(500);res.end('Controlled fixture failed');}}
});
server.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
