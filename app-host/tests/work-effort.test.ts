// Headless canonical preference and actual execution proof; no terminal renderer.
import {expect,test} from "bun:test"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../../ui/src/gator/client"
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const via of ["manual","conversation"] as const)test(`${via} effort uses the canonical conversation preference through restart and later execution`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-work-effort-"));let calls=0
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const body=await request.json() as any;calls++;let message:any;const last=body.messages.at(-1)
  if(body.messages.filter((m:any)=>m.role==="user").at(-1)?.content==="Use high effort for future requests in this conversation"){
   if(last.role!=="tool")message={role:"assistant",tool_calls:[{id:"inspect-settings",type:"function",function:{name:"gator_app_inspect",arguments:JSON.stringify({topic:"settings"})}}]}
   else if(last.name==="gator_app_inspect")message={role:"assistant",tool_calls:[{id:"save-effort",type:"function",function:{name:"gator_app_update",arguments:JSON.stringify({setting:"work_effort",value:{effort:"high"}})}}]}
   else message={role:"assistant",content:"The setting outcome is retained."}
  }else message={role:"assistant",content:"Later task complete."}
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:`http://127.0.0.1:${server.port}/chat`,models:["fixture"],default_model:"fixture"}],connectors:[{version:1,id:"reports",name:"Reports",kind:"http_json",resource:`http://127.0.0.1:${server.port}/source`,authentication:"none"}]}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},options={binary,env,userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options))
 try{
  await client.start();const owner=client.state!.conversation_id!,dir=join(root,"state/gator/conversations",owner);await client.send({type:"submit",text:"/mode inspect"})
  if(via==="manual"){
   const request_id="save-manual-effort";await client.result({type:"composer",composer:{id:client.state!.composer!.id,request_id,action:"effort",effort:"high"}},request_id,s=>s.composer_result)
   expect(calls).toBe(0)
  }else{
   await client.edit("Use high effort for future requests in this conversation");await client.submit();await until(()=>!client.state?.running&&calls>0,"conversational settings outcome")
   const requests=JSON.parse(await readFile(join(dir,"requests.json"),"utf8")).requests;expect(requests[0].options.MaxSteps).toBe(24)
  }
  expect(JSON.parse(await readFile(join(dir,"preferences.json"),"utf8")).work_effort).toBe("high");expect(client.state?.composer?.budget).toEqual({work_steps:48,code_steps:32})
  const select="select-account-with-budget";await client.result({type:"sources",sources:{id:client.state!.sources!.id,request_id:select,action:"select",ids:["reports"]}},select,s=>s.sources_result)
  expect(JSON.parse(await readFile(join(dir,"preferences.json"),"utf8")).work_effort).toBe("high")
  await client.edit("Preserve my draft",[],undefined,true);await client.close();client=new GatorClient(new Application(options));await client.start(owner)
  expect(client.state?.composer?.budget).toEqual({work_steps:48,code_steps:32});expect(client.state?.sources?.selected).toEqual(["reports"]);expect(client.draft.text).toBe("Preserve my draft");expect(client.draft.plan_first).toBe(true)
  const previousCalls=calls;await client.send({type:"submit",text:"/mode inspect"});await client.edit("Run a later task",[],undefined,false);await client.submit();await until(()=>!client.state?.running&&calls>previousCalls,"later task executes")
  const requests=JSON.parse(await readFile(join(dir,"requests.json"),"utf8")).requests,actual=requests.at(-1);expect(actual.options.MaxSteps).toBe(48);expect(actual.options.Code.MaxSteps).toBe(32)
 }finally{await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)

test("delegated effort refreshes a cached parent without widening running or queued work",async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-cached-effort-"));let release!:()=>void;const held=new Promise<void>(r=>{release=r});let heldCalls=0
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"",last=body.messages.at(-1);let message:any
  if(system.includes("You handle the conversation with Gator"))message={role:"assistant",content:JSON.stringify({kind:"task"})}
  else if(body.messages.some((m:any)=>m.role==="user"&&m.content==="Use high effort for future requests in this conversation")){
   if(last.role!=="tool")message={role:"assistant",tool_calls:[{id:"inspect-parent-settings",type:"function",function:{name:"gator_app_inspect",arguments:JSON.stringify({topic:"settings"})}}]}
   else if(last.name==="gator_app_inspect")message={role:"assistant",tool_calls:[{id:"save-parent-effort",type:"function",function:{name:"gator_app_update",arguments:JSON.stringify({setting:"work_effort",value:{effort:"high"}})}}]}
   else message={role:"assistant",content:"The parent preference outcome is retained."}
  }else{heldCalls++;await held;message={role:"assistant",content:"Original scope complete."}}
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:`http://127.0.0.1:${server.port}/chat`,models:["fixture"],default_model:"fixture"}]}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")};const client=new GatorClient(new Application({binary,env,userData:join(root,"app"),workspace:join(root,"workspace")}))
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});const parent=client.state!.conversation_id!,path=join(root,"state/gator/conversations",parent,"requests.json")
  await client.edit("Hold the original task");await client.submit();await until(()=>heldCalls===1,"original task executing")
  await client.edit("Queued task keeps its captured budget",[],undefined,false);await client.submit(true);expect(client.state?.queue).toHaveLength(1)
  await client.edit("Use high effort for future requests in this conversation");await client.submit();await client.edit("Preserve parent draft",[],undefined,true)
  await until(()=>client.state?.composer?.budget?.work_steps===48,"cached parent receives authoritative preference change")
  expect(client.state?.conversation_id).toBe(parent);expect(client.state?.running).toBe(true);expect(heldCalls).toBe(1);expect(client.draft.text).toBe("Preserve parent draft")
  const journal=JSON.parse(await readFile(path,"utf8"));expect(journal.requests.map((r:any)=>r.options.MaxSteps)).toEqual([24,24]);expect(journal.requests.map((r:any)=>r.options.Code.MaxSteps)).toEqual([16,16])
  expect(JSON.parse(await readFile(join(root,"state/gator/conversations",parent,"preferences.json"),"utf8")).work_effort).toBe("high")
  const child=client.state!.dialogue!.find(d=>d.task_chat)!.task_chat!;await client.openSession(child);await until(()=>!client.state?.running,"delegated settings task ends")
  expect(client.state?.composer?.budget?.work_steps).toBe(24);await client.openSession(parent);expect(client.state?.composer?.budget?.work_steps).toBe(48);expect(client.draft.text).toBe("Preserve parent draft");expect(client.draft.plan_first).toBe(true)
 }finally{release();await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)

test("changing the saved effort never widens a pending reviewed plan",async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-reviewed-effort-"));let workCalls=0
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let message:any
  if(system.includes("Propose a plan"))message={role:"assistant",content:JSON.stringify({outcome:"Reviewed task",steps:[{id:"read",text:"Summarize the selected context"}],sources:[],effects:["read"]})}
  else{workCalls++;message={role:"assistant",content:"Reviewed task complete."}}
  return Response.json({choices:[{message,finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:`http://127.0.0.1:${server.port}/chat`,models:["fixture"],default_model:"fixture"}]}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")};const client=new GatorClient(new Application({binary,env,userData:join(root,"app"),workspace:join(root,"workspace")}))
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});const owner=client.state!.conversation_id!,dir=join(root,"state/gator/conversations",owner)
  await client.edit("Prepare a bounded task",[],undefined,true);await client.submit();await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"original plan review")
  const original=JSON.parse(await readFile(join(dir,"requests.json"),"utf8")).requests[0],request_id="change-future-effort"
  await client.result({type:"composer",composer:{id:client.state!.composer!.id,request_id,action:"effort",effort:"high"}},request_id,s=>s.composer_result)
  const pending=JSON.parse(await readFile(join(dir,"requests.json"),"utf8")).requests[0];expect(pending).toEqual(original);expect(workCalls).toBe(0)
  const plan=client.state!.task_plans![0]!;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:plan.id,revision:plan.revision,digest:plan.digest,action:"approve",request_id:"approve-original-budget"}})
  await until(()=>workCalls===1&&!client.state?.running,"reviewed budget executes")
  const completed=JSON.parse(await readFile(join(dir,"requests.json"),"utf8")).requests[0];expect(completed.id).toBe(original.id);expect(completed.options.MaxSteps).toBe(24)
  const revision=JSON.parse(await readFile(join(dir,"revisions",completed.revision_id+".json"),"utf8")),manifest=JSON.parse(await readFile(join(revision.bundle_path,"manifest.json"),"utf8"));expect(manifest.execution_accounting.max_steps).toBe(24);expect(client.state?.composer?.budget?.work_steps).toBe(48)
 }finally{await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
