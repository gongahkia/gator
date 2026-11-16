// Controlled native audit data. Loopback only; no credentials or real accounts.
import http from 'node:http';
import {appendFile,access} from 'node:fs/promises';
import {join} from 'node:path';
const root=process.env.GATOR_FIXTURE_CONTROL;if(!root)throw Error('Disposable root required');
const trace=v=>appendFile(join(root,'audit-trace.jsonl'),JSON.stringify(v)+'\n');
const prose='# Your day, with room to breathe\n\nI found three useful things to keep in view. The draft remains yours to review.\n\n- Finish the project update.\n- Leave time for lunch.\n- Review the travel details before booking.\n\n## A small example\n\n```swift\nlet message = "Hello, Gator"\nprint(message)\n```\n\nA longer reference: https://example.invalid/planning/your-personal-assistant/retained-evidence-and-reviewed-changes\n\n**Nothing has been sent or scheduled.** You can ask for a correction or keep talking.';
const server=http.createServer(async(req,res)=>{
 try{
  let raw='';for await(const p of req)raw+=p;const body=JSON.parse(raw||'{}'),messages=body.messages||[];
  if(req.url.includes('/gmail/')){await trace({kind:'external-effect'});res.writeHead(200,{'Content-Type':'application/json'});res.end('{}');return;}
  const system=messages.filter(m=>m.role==='system').map(m=>String(m.content)).join('\n');
  const user=messages.filter(m=>m.role==='user').at(-1)?.content;let text=typeof user==='string'?user:JSON.stringify(user),message;
  if(system.includes('You handle the conversation with Gator'))message={role:'assistant',content:JSON.stringify({kind:'reply',text:'I am here. Your original work continues independently.'})};
  else if(system.includes('Propose a plan'))message={role:'assistant',content:JSON.stringify({outcome:'A concise, reviewed itinerary',steps:[{id:'read',text:'Read the retained trip notes'},{id:'compare',text:'Compare times and prepare a draft itinerary'},{id:'review',text:'Present the result without making any bookings'}],sources:[],effects:['read','draft']})};
  else if(system.includes("Classify the user's immediate request"))message={role:'assistant',content:JSON.stringify({mode:text.includes('Send controlled review')?'act':'inspect'})};
  else if(text==='Prepare controlled fixture scope')message={role:'assistant',content:'Fixture scope retained.'};
  else if(text?.includes('Simulate recovery failure')){await trace({kind:'controlled-failure'});res.writeHead(503,{'Content-Type':'application/json'});res.end('{"error":{"message":"Controlled provider unavailable. Original evidence is retained."}}');return;}
  else if(text?.includes('Send controlled review'))message={role:'assistant',tool_calls:[{id:'native-audit-review',type:'function',function:{name:'connector_google_gmail_send',arguments:JSON.stringify({payload:{from:'me@example.invalid',to:'team@example.invalid',subject:'Project Update — review before sending',body:'Hello team,\n\nThis is a controlled native review. '+('The full exact content remains selectable and needs explicit approval.\n'.repeat(12))+'\nFINAL REVIEW LINE'}})}}]};
  else{
   await trace({kind:'work',objective:text?.slice(0,100)});
   if(text?.includes('Hold native work')){while(!res.destroyed){try{await access(join(root,'release-main'));break;}catch{}await new Promise(r=>setTimeout(r,30));}}
   message={role:'assistant',content:prose};
  }
  if(res.destroyed)return;
  if(body.stream&&!message.tool_calls){res.writeHead(200,{'Content-Type':'text/event-stream'});for(const part of message.content.match(/.{1,45}/gs)||[]){res.write('data: '+JSON.stringify({choices:[{delta:{content:part},finish_reason:null}]})+'\n\n');await new Promise(r=>setTimeout(r,25));}res.end('data: '+JSON.stringify({choices:[{delta:{},finish_reason:'stop'}],usage:{prompt_tokens:10,completion_tokens:10}})+'\n\ndata: [DONE]\n\n');}
  else{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message,finish_reason:message.tool_calls?'tool_calls':'stop'}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));}
 }catch(e){await trace({kind:'fixture-error',error:String(e)});if(!res.destroyed){res.writeHead(500);res.end('{}');}}
});server.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
