// Local-only Responses API fixture. No external network, credentials or accounts.
import http from 'node:http';
const server=http.createServer(async(req,res)=>{
 let raw=''; for await(const part of req)raw+=part;
 const body=JSON.parse(raw||'{}'), input=JSON.stringify(body.input||[]);
 if(process.env.GATOR_FIXTURE_TRACE==='1')console.error(JSON.stringify({tools:(body.tools||[]).map(t=>t.function?.name),roles:(body.messages||[]).map(m=>m.role),system:(body.messages||[]).filter(m=>m.role==='system').map(m=>String(m.content).slice(0,180))}));
 if(process.argv.includes('--approval')){
  if(req.url.includes('/gmail/')){res.writeHead(200,{'Content-Type':'application/json'});res.end('{}');return;}
  const actionName=(body.tools||[]).some(t=>t.function?.name==='connector_google_gmail_send')?'connector_google_gmail_send':'web_access';
  const send=(body.tools||[]).some(t=>t.function?.name===actionName)&&!(body.messages||[]).some(m=>m.role==='tool');
  const classify=(body.messages||[]).some(m=>m.role==='system'&&String(m.content).includes("Classify the user's immediate request"));
  const args=actionName==='web_access'?{action:'request',scope:'sites',origins:['https://example.invalid'],reason:'Controlled native approval fixture. No web request will be made.'}:{payload:{from:'fixture@example.com',to:'venue@example.com',subject:'Native approval fixture',body:'Exact native review content'}};
  const message=classify?{role:'assistant',content:'{"mode":"inspect"}'}:send?{role:'assistant',tool_calls:[{id:'fixture-action',type:'function',function:{name:actionName,arguments:JSON.stringify(args)}}]}:{role:'assistant',content:'The fixture action outcome is retained. No external web request was made.'};
  res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message,finish_reason:send?'tool_calls':'stop'}],usage:{prompt_tokens:1,completion_tokens:1}}));return;
 }
 const text=input.includes('Classify the user')?'{"mode":"inspect"}':'Native integration answer 🐊';
 const response={id:'fixture-response',output:[{type:'message',role:'assistant',content:[{type:'output_text',text}]}],usage:{input_tokens:1,output_tokens:1}};
 if(!body.stream){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify(response));return;}
 res.writeHead(200,{'Content-Type':'text/event-stream'});
 res.write('event: response.output_text.delta\ndata: '+JSON.stringify({type:'response.output_text.delta',delta:'Native integration '})+'\n\n');
 setTimeout(()=>{
  res.write('event: response.output_text.delta\ndata: '+JSON.stringify({type:'response.output_text.delta',delta:'answer 🐊'})+'\n\n');
  res.end('event: response.completed\ndata: '+JSON.stringify({type:'response.completed',response})+'\n\n');
 },350);
});
server.listen(Number(process.env.GATOR_FIXTURE_PORT||0),'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
