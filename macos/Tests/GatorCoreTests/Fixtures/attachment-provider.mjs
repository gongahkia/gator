// Disposable loopback-only attachment provider. No accounts or external writes.
import http from 'node:http';
import {appendFile,access} from 'node:fs/promises';
import {join} from 'node:path';
const root=process.env.GATOR_FIXTURE_CONTROL;
if(!root)throw Error('Disposable fixture directory required');
const trace=async v=>appendFile(join(root,'attachment-trace.jsonl'),JSON.stringify(v)+'\n');
const server=http.createServer(async(req,res)=>{
 try{
  let raw='';for await(const p of req)raw+=p;
  const body=JSON.parse(raw||'{}'),messages=body.messages||[],system=messages.filter(m=>m.role==='system').map(m=>String(m.content)).join('\n');
  let content,kind;
  if(system.includes('You handle the conversation with Gator')){
   const context=JSON.parse(messages.at(-1).content);kind='route';
   content=JSON.stringify(context.message.includes('Read separate attachment')?{kind:'task'}:{kind:'reply',text:'Attached input retained while the original work continues.'});
  }else if(system.includes("Classify the user's immediate request")){kind='classify';content='{"mode":"inspect"}';}
  else{
   kind='work';await trace({kind:'received',body});
   const user=messages.filter(m=>m.role==='user').at(-1)?.content,text=typeof user==='string'?user:JSON.stringify(user);
   if(text?.includes('Hold original task')){while(!res.destroyed){try{await access(join(root,'release-main'));break;}catch{}await new Promise(r=>setTimeout(r,20));}}
   content='Controlled attachment received: '+(raw.includes('Original jade heron')?'Original jade heron.':'input inspected.');
  }
  await trace({kind,body});if(res.destroyed)return;
  res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message:{role:'assistant',content},finish_reason:'stop'}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));
 }catch(e){await trace({kind:'error',error:String(e)});if(!res.destroyed){res.writeHead(500);res.end('Controlled fixture failed');}}
});server.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
