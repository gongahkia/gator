// Disposable local model/source fixture. No production accounts and no external writes.
import http from 'node:http';
import {appendFile,access} from 'node:fs/promises';
import {join} from 'node:path';
const root=process.env.GATOR_FIXTURE_CONTROL;if(!root)throw Error('Disposable root required');
const trace=v=>appendFile(join(root,'recovery-trace.jsonl'),JSON.stringify(v)+'\n');
const exists=async name=>{try{await access(join(root,name));return true;}catch{return false;}};
const server=http.createServer(async(req,res)=>{
 try{
 const url=new URL(req.url,'http://localhost');
 if(url.pathname.startsWith('/source')){await trace({kind:'source',method:req.method,url:req.url});if(req.method!=='GET'){res.writeHead(405);res.end();return;}res.setHeader('Content-Type','application/json');res.end('{"summary":"Local retained source"}');return;}
 let raw='';for await(const part of req)raw+=part;const body=JSON.parse(raw||'{}'),messages=body.messages||[],system=messages.filter(m=>m.role==='system').map(m=>m.content).join('\n');let message;
 if(system.includes('You handle the conversation with Gator')){message={role:'assistant',content:JSON.stringify({kind:'reply',text:'Independent recovery conversation reply.'})};await trace({kind:'dialogue'});}
 else if(system.includes('Propose a plan')){
  const context=JSON.parse(messages.at(-1).content);await trace({kind:'preparation',context});
  while(await exists('hold-preparation')&&!await exists('release-preparation')&&!res.destroyed)await new Promise(r=>setTimeout(r,20));
  message={role:'assistant',content:JSON.stringify({outcome:'Reviewed recovered original request',steps:[{id:'read',text:'Summarize the original input using reviewed scope'}],sources:context.selected_source_ids||[],effects:['read']})};
 }else if(system.includes("Classify the user's immediate request")){message={role:'assistant',content:'{"mode":"inspect"}'};}
 else{
  const text=JSON.stringify(messages);
  if(text.includes('Retain fixture source selection')){message={role:'assistant',content:'Recovery fixture scope retained.'};await trace({kind:'seed'});}
  else{const repaired=await exists('repaired');await trace({kind:repaired?'execution':'failure',messages});
   if(!repaired){res.writeHead(401,{'Content-Type':'application/json'});res.end('{"error":{"message":"Controlled original provider rejection"}}');return;}
   message={role:'assistant',content:'The separately authorized recovery completed: silver otter.'};
  }
 }
 if(!res.destroyed){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({choices:[{message,finish_reason:'stop'}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));}
 }catch(error){await trace({kind:'fixture-error',error:String(error)});if(!res.destroyed){res.writeHead(500);res.end('{}');}}
});server.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:server.address().port})+'\n'));
