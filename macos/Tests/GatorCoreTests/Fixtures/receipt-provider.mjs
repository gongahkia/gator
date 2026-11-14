// Read-only fake Gmail service. Reject every non-GET; never forwards to a real service.
import http from 'node:http';
import {appendFile,access} from 'node:fs/promises';
import {join} from 'node:path';
const root=process.env.GATOR_FIXTURE_CONTROL;if(!root)throw Error('disposable root required');
const server=http.createServer(async(req,res)=>{
 await appendFile(join(root,'receipt-trace.jsonl'),JSON.stringify({method:req.method,url:req.url})+'\n');
 res.setHeader('Content-Type','application/json');
 if(req.method!=='GET'){res.writeHead(405);res.end('{}');return;}
 const url=new URL(req.url,'http://localhost');
 if(url.pathname.endsWith('/messages')){
  const correlation=(url.searchParams.get('q')||'').match(/gator-([a-f0-9]{32})@/)?.[1];
  await appendFile(join(root,'correlation'),correlation||'');
  let empty=false;try{await access(join(root,'unconfirmed'));empty=true;}catch{}
  res.end(JSON.stringify({messages:empty?[]:[{id:'sent-local'}]}));return;
 }
 if(url.pathname.endsWith('/messages/sent-local')){
  const {readFile}=await import('node:fs/promises');const correlation=await readFile(join(root,'correlation'),'utf8');
  res.end(JSON.stringify({id:'sent-local',labelIds:['SENT'],payload:{headers:[{name:'Message-ID',value:`<gator-${correlation.slice(-32)}@gator.invalid>`}]}}));return;
 }
 res.writeHead(404);res.end('{}');
});server.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
