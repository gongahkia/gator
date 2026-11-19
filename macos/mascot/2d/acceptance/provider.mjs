// deterministic loopback fixture; no outbound client or effect implementation
import http from 'node:http';
import {appendFile} from 'node:fs/promises';
import {join} from 'node:path';
const root=process.env.GATOR_FIXTURE_CONTROL;
if(!root)throw Error('Disposable fixture root required');
const trace=value=>appendFile(join(root,'provider-trace.jsonl'),JSON.stringify(value)+'\n');
const server=http.createServer(async(req,res)=>{
 try{
  if(req.url!=='/responses'){await trace({kind:'unexpected-path',path:req.url});res.writeHead(404);res.end();return;}
  let raw='';for await(const part of req)raw+=part;
  const body=JSON.parse(raw),system=body.instructions||'',input=JSON.stringify(body.input||[]);
  await trace({kind:'request',stream:!!body.stream,classification:system.includes('Classify'),route:system.includes('You handle the conversation'),tools:(body.tools||[]).map(x=>x.name)});
  let text='## A small, local answer\n\nThe mascot observes this public response. This is a loopback fixture, with no external account or model request.\n\n- Your draft stays in its conversation.\n- The character does not approve anything.\n- Work remains owned by Go.\n\n';
  const response={id:'mascot-fixture',output:[],usage:{input_tokens:1,output_tokens:1}};
  if(system.includes("Classify the user's immediate request"))text='{"mode":"inspect"}';
  else if(system.includes('You handle the conversation with Gator'))text=JSON.stringify({kind:'task'});
  else if(input.includes('Review mascot')&&(body.tools||[]).some(x=>x.name==='web_access')&&!input.includes('function_call_output')){
   response.output=[{type:'function_call',call_id:'mascot-inert-review',name:'web_access',arguments:JSON.stringify({action:'request',scope:'sites',origins:['https://mascot.example.invalid'],reason:'Local mascot layout fixture. No fetch is implemented. Do not approve.'})}];
  }
  if(!response.output.length)response.output=[{type:'message',role:'assistant',content:[{type:'output_text',text}]}];
  if(body.stream&&response.output[0].type==='function_call'){res.writeHead(200,{'Content-Type':'text/event-stream'});res.end('event: response.completed\ndata: '+JSON.stringify({type:'response.completed',response})+'\n\n');}
  else if(body.stream&&response.output[0].type==='message'&&!system.includes('Classify')&&!system.includes('You handle the conversation')){
   res.writeHead(200,{'Content-Type':'text/event-stream'});
   for(const delta of text.match(/.{1,8}/gs)||[]){if(res.destroyed)return;res.write('event: response.output_text.delta\ndata: '+JSON.stringify({type:'response.output_text.delta',delta})+'\n\n');await new Promise(r=>setTimeout(r,220));}
   res.end('event: response.completed\ndata: '+JSON.stringify({type:'response.completed',response})+'\n\n');await trace({kind:'stream-completed'});
  }else{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify(response));}
 }catch(error){await trace({kind:'fixture-error',error:String(error)});if(!res.destroyed){res.writeHead(500);res.end('{}');}}
});
server.listen(Number(process.env.GATOR_FIXTURE_PORT||0),'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
