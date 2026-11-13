// Actual Go/host model routing; no terminal renderer or live provider accounts.
import {expect,test} from "bun:test"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../../ui/src/gator/client"
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const via of ["manual","conversation"] as const)test(`${via} model change reaches a busy cached owner and only new tasks use the new provider`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-model-selection-"));let release!:()=>void;const held=new Promise<void>(r=>{release=r});let heldCalls=0,newCalls=0;const routes:string[]=[]
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const route=new URL(request.url).pathname,body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"",last=body.messages.at(-1);let message:any
  if(system.includes("You handle the conversation with Gator")){routes.push(route);message={role:"assistant",content:JSON.stringify({kind:"task"})}}
  else if(body.messages.some((m:any)=>m.role==="user"&&m.content==="Use provider B for future requests")){
   if(last.role!=="tool")message={role:"assistant",tool_calls:[{id:"inspect-models",type:"function",function:{name:"gator_app_inspect",arguments:JSON.stringify({topic:"models"})}}]}
   else if(last.name==="gator_app_inspect")message={role:"assistant",tool_calls:[{id:"change-model",type:"function",function:{name:"gator_app_update",arguments:JSON.stringify({setting:"model",value:{provider:"fixture-b",model:"fixture"}})}}]}
   else message={role:"assistant",content:"The model change outcome is retained."}
  }else if(body.messages.some((m:any)=>m.role==="user"&&m.content==="Hold the original request")){heldCalls++;await held;message={role:"assistant",content:"Original request completed."}}
  else{expect(route).toBe("/b");newCalls++;message={role:"assistant",content:"New request completed under provider B."}}
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture-a",model:"fixture"},custom_providers:[{id:"fixture-a",base_url:`http://127.0.0.1:${server.port}/a`,models:["fixture"],default_model:"fixture"},{id:"fixture-b",base_url:`http://127.0.0.1:${server.port}/b`,models:["fixture"],default_model:"fixture"}]}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},options={binary,env,userData:join(root,"app"),workspace:join(root,"workspace")};let application=new Application(options),client=new GatorClient(application)
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});const parent=client.state!.conversation_id!,path=join(root,"state/gator/conversations",parent,"requests.json")
  await client.edit("Hold the original request");await client.submit();await until(()=>heldCalls===1,"original provider A request")
  await client.edit("Queued request retains provider A");await client.submit(true);expect(client.state?.queue).toHaveLength(1)
  if(via==="manual"){
   await client.send({type:"submit",text:"/new"});await until(()=>!!client.state?.conversation_id&&client.state.conversation_id!==parent,"independent settings owner")
   const request_id="save-provider-b";await client.result({type:"composer",composer:{id:client.state!.composer!.id,request_id,action:"model",provider:"fixture-b",model:"fixture"}},request_id,s=>s.composer_result)
   await until(()=>[...application.sessions.workers].some((w:any)=>w.state?.conversation_id===parent&&w.state?.provider==="fixture-b"),"background owner receives model-default refresh");await client.openSession(parent)
  }else{
   await client.edit("Use provider B for future requests");await client.submit()
   await until(()=>client.state?.dialogue?.some(d=>d.task_chat&&client.state!.dialogue_peers?.some(p=>p.conversation_id===d.task_chat&&p.state==="completed"))??false,"delegated model change completes")
  }
  await until(()=>client.state?.provider==="fixture-b","busy cached owner reflects saved provider B")
  expect(client.state?.running).toBe(true);expect(heldCalls).toBe(1);expect(client.state?.composer?.readiness?.processing.destination).toContain("127.0.0.1")
  const captured=JSON.parse(await readFile(path,"utf8")).requests;expect(captured.map((r:any)=>r.options.ModelProvider)).toEqual(["fixture-a","fixture-a"])
  expect(JSON.parse(await readFile(join(root,"config/gator/config.json"),"utf8")).defaults.provider).toBe("fixture-b")
  await client.edit("Start a fresh independent request");await client.submit();await until(()=>newCalls===1,"fresh work uses provider B");expect(routes.at(-1)).toBe("/b");expect(heldCalls).toBe(1)
  await client.edit("Keep after model change",[],undefined,true);await client.close();application=new Application(options);client=new GatorClient(application);await client.start(parent)
  expect(client.state?.provider).toBe("fixture-b");expect(client.draft.text).toBe("Keep after model change");expect(client.draft.plan_first).toBe(true);expect(heldCalls).toBe(1);expect(newCalls).toBe(1)
 }finally{release();await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)

test("a pending plan keeps its reviewed provider after the default model changes",async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-reviewed-model-")),workRoutes:string[]=[]
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const route=new URL(request.url).pathname,body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let message:any
  if(system.includes("Propose a plan"))message={role:"assistant",content:JSON.stringify({outcome:"Review original provider",steps:[{id:"read",text:"Summarize selected context"}],sources:[],effects:["read"]})}
  else if(system.includes("You handle the conversation with Gator"))message={role:"assistant",content:JSON.stringify({kind:"task"})}
  else{workRoutes.push(route);message={role:"assistant",content:"Request complete."}}
  return Response.json({choices:[{message,finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture-a",model:"fixture"},custom_providers:[{id:"fixture-a",base_url:`http://127.0.0.1:${server.port}/a`,models:["fixture"],default_model:"fixture"},{id:"fixture-b",base_url:`http://127.0.0.1:${server.port}/b`,models:["fixture"],default_model:"fixture"}]}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},client=new GatorClient(new Application({binary,env,userData:join(root,"app"),workspace:join(root,"workspace")}))
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});const owner=client.state!.conversation_id!,path=join(root,"state/gator/conversations",owner,"requests.json")
  await client.edit("Prepare a reviewed request",[],undefined,true);await client.submit();await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"original model review")
  const original=JSON.parse(await readFile(path,"utf8")).requests[0],request_id="change-future-model"
  await client.result({type:"composer",composer:{id:client.state!.composer!.id,request_id,action:"model",provider:"fixture-b",model:"fixture"}},request_id,s=>s.composer_result)
  expect(JSON.parse(await readFile(path,"utf8")).requests[0]).toEqual(original);expect(workRoutes).toEqual([])
  const plan=client.state!.task_plans![0]!;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:plan.id,revision:plan.revision,digest:plan.digest,action:"approve",request_id:"approve-original-provider"}})
  await until(()=>workRoutes.length===1&&!client.state?.running,"reviewed model executes");expect(workRoutes).toEqual(["/a"]);expect(client.state?.provider).toBe("fixture-b")
  const completed=JSON.parse(await readFile(path,"utf8")).requests[0];expect(completed.id).toBe(original.id);expect(completed.options.ModelProvider).toBe("fixture-a")
  await client.edit("A later request uses the new default",[],undefined,false);await client.submit();await until(()=>workRoutes.length===2&&!client.state?.running,"later model executes");expect(workRoutes).toEqual(["/a","/b"])
 }finally{await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
