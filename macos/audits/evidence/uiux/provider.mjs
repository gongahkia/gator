// Audit fixture only. All requests terminate on loopback; no external action is implemented.
import http from 'node:http';
import {appendFile,access} from 'node:fs/promises';
import {join} from 'node:path';
const root=process.env.GATOR_FIXTURE_CONTROL;
if(!root)throw Error('Disposable audit directory required');
const record=v=>appendFile(join(root,'provider-trace.jsonl'),JSON.stringify(v)+'\n');
const normal='# A little room in your day\n\nHere are three things to keep in view. **Nothing has been sent or booked.**\n\n- Finish the project update.\n  - Check the dates with the team.\n  - Keep the draft for your review.\n- Leave time for lunch.\n- Compare the travel options.\n\n## A small example\n\n```swift\nlet plan = "Keep the afternoon clear"\nprint(plan)\n```\n\nYou can keep talking while another task runs. A [reference link](https://example.invalid/review) is included for layout only.';
const server=http.createServer(async(req,res)=>{
 try{
  if(!['/chat','/responses'].includes(req.url)){await record({kind:'unexpected-effect',method:req.method,path:req.url});res.writeHead(404);res.end();return;}
  let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);
  const responses=req.url==='/responses';
  const messages=responses?(body.input||[]).map(i=>({role:i.type==='function_call_output'?'tool':i.role,content:typeof i.content==='string'?i.content:Array.isArray(i.content)?i.content.map(c=>c.text||'').join('\n'):i.output||''})):(body.messages||[]);
  const system=responses?body.instructions||'':messages.filter(m=>m.role==='system').map(m=>m.content).join('\n');
  const user=messages.filter(m=>m.role==='user').at(-1)?.content||'';
  const tools=(body.tools||[]).map(t=>t.name||t.function?.name);let text=normal,call;
  if(system.includes('You handle the conversation with Gator')){
   const context=JSON.parse(messages.at(-1).content);await record({kind:'dialogue',message:context.message});
   text=JSON.stringify(context.message==='Audit exact approval'?{kind:'task'}:{kind:'reply',text:'I am here. The original task continues independently.'});
  }else if(system.includes("Classify the user's immediate request")){text='{"mode":"inspect"}';}
  else if(system.includes('Propose a plan')){text=JSON.stringify({outcome:'A clear, reviewed afternoon itinerary',steps:[{id:'read',text:'Read the selected trip notes and compare the available times'},{id:'draft',text:'Prepare a draft itinerary without making a booking'},{id:'review',text:'Present the result and any uncertainty for your review'}],sources:[],effects:['read','draft']});}
  else if(user==='Audit recovery'){
   await record({kind:'controlled-provider-error'});res.writeHead(503,{'Content-Type':'application/json'});res.end('{"error":{"message":"Controlled fixture unavailable; no external operation occurred.","code":"server_error"}}');return;
  }else if(user==='Hold audit work'){
   await record({kind:'held-work'});
   while(!res.destroyed){try{await access(join(root,'release-work'));break;}catch{}await new Promise(r=>setTimeout(r,25));}
   if(res.destroyed){await record({kind:'held-disconnected'});return;}text='The independent audit task completed.';
  }else if(user==='Audit exact approval'&&!messages.some(m=>m.role==='tool')){
   if(!tools.includes('web_access'))throw Error('Inert review tool not advertised');
   call={id:'audit-inert-review',name:'web_access',arguments:JSON.stringify({action:'request',scope:'sites',origins:['https://audit.example.invalid'],reason:'Audit fixture: review this exact access request and decline. No fetch, search, booking or send is performed.'})};await record({kind:'inert-proposal'});
  }else if(user==='Audit exact approval'){text='Your decline is retained. No external action was performed.';}
  else if(user.startsWith('History item ')){text=`## Retained note ${user.slice(13)}\n\n`+'A readable personal-assistant note for checking conversation navigation and reading position. '.repeat(3);}
  await record({kind:'model-response',surface:responses?'responses':'chat',stream:!!body.stream,tool:!!call});
  if(!responses){const message=call?{role:'assistant',tool_calls:[{id:call.id,type:'function',function:{name:call.name,arguments:call.arguments}}]}:{role:'assistant',content:text};res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message,finish_reason:call?'tool_calls':'stop'}],usage:{prompt_tokens:1,completion_tokens:1,total_tokens:2}}));return;}
  const response={id:'audit-response',output:call?[{type:'function_call',id:call.id,call_id:call.id,name:call.name,arguments:call.arguments}]:[{type:'message',role:'assistant',content:[{type:'output_text',text}]}],usage:{input_tokens:1,output_tokens:1}};
  if(!body.stream){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify(response));return;}
  res.writeHead(200,{'Content-Type':'text/event-stream'});
  let delay=0;try{await access(join(root,'seed-fast'));}catch{if(user==='Audit normal conversation')delay=80;if(user==='Audit streaming')delay=260;}
  if(!call)for(const delta of text.match(/.{1,36}/gs)||[]){if(res.destroyed)return;res.write('event: response.output_text.delta\ndata: '+JSON.stringify({type:'response.output_text.delta',delta})+'\n\n');if(delay)await new Promise(r=>setTimeout(r,delay));}
  res.end('event: response.completed\ndata: '+JSON.stringify({type:'response.completed',response})+'\n\n');
 }catch(error){await record({kind:'fixture-error',error:String(error)});if(!res.destroyed){res.writeHead(500);res.end('{}');}}
});
server.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
