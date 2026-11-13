import {test,expect} from "bun:test"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../src/gator/client"
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const delegated of [false,true])test(`${delegated?"delegated":"local"} ordinary correction retains evidence and spends only remaining budget after fresh review`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-running-plan-"));let release!:()=>void;const held=new Promise<void>(r=>{release=r});let executions=0,parentExecutions=0;const proposals:any[]=[]
 const plan={outcome:"Compare venues",steps:[{id:"compare",text:"Compare venues"}],sources:[],effects:["read"]}
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let content:string
  if(system.includes("You handle the conversation with Gator")){const meta=JSON.parse(body.messages.at(-1).content);content=JSON.stringify(meta.message==="Compare venues"?{kind:"task"}:meta.message==="Hello during reviewed work"?{kind:"reply",text:"The original owners are still working."}:{kind:"revise_plan",target:meta.tasks.find((t:any)=>t.can_revise_plan).id,instruction:meta.message})}
  else if(system.includes("Propose a plan")){const meta=JSON.parse(body.messages.at(-1).content);proposals.push(meta);content=JSON.stringify({...plan,outcome:meta.correction?"Venues under $2,000":plan.outcome})}
  else if(body.messages.some((m:any)=>m.role==="user"&&m.content==="Research an unrelated topic")){parentExecutions++;await held;content="Unrelated original work completed."}
  else{executions++;if(executions===1)await held;content="Reviewed venue comparison completed."}
  return Response.json({choices:[{message:{role:"assistant",content},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:`http://127.0.0.1:${server.port}/chat`,models:["fixture"],default_model:"fixture"}]}))
 const options={binary:process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env:{...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options))
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"})
  const parent=client.state!.conversation_id!
  if(delegated){await client.edit("Research an unrelated topic");await client.submit();await until(()=>parentExecutions===1,"parent work held")}
  await client.edit("Compare venues",[],undefined,true);await client.submit()
  if(delegated){await until(()=>!!client.state?.dialogue_peers?.some(p=>p.can_revise_plan),"child review");const child=client.state!.dialogue![0]!.task_chat!;await client.host.sessions.open({conversation_id:child});await client.loadDraft()}
  await until(()=>client.state?.task_plans?.[0]?.phase==="review","initial review")
  const original=client.state!.task_plans![0]!,owner=client.state!.conversation_id!
  const approve=async(value:typeof original,id:string)=>client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:value.id,revision:value.revision,digest:value.digest,action:"approve",request_id:id}})
  await approve(original,"initial-plan-approval");await until(()=>executions===1&&!!client.state?.running,"reviewed execution held")
  if(delegated){await client.host.sessions.open({conversation_id:parent});await client.loadDraft();await until(()=>!!client.state?.dialogue_peers?.some(p=>p.conversation_id===owner&&p.can_revise_plan),"running child control")}
  await client.edit("Keep venues under $2,000");await client.submit()
  if(delegated){await until(()=>client.state?.dialogue?.at(-1)?.state==="guided","delegated correction result");expect(client.state?.conversation_id).toBe(parent);expect(client.state?.running).toBe(true);await client.edit("Hello during reviewed work");await client.submit();await until(()=>client.state?.dialogue?.at(-1)?.state==="answered","unrelated conversation");await client.edit("Preserve requesting chat draft");await client.host.sessions.open({conversation_id:owner});await client.loadDraft()}
  await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running&&proposals.length===2,"corrected plan after accounting")
  if(!delegated)expect(client.state?.dialogue?.at(-1)?.state).toBe("guided");expect(executions).toBe(1)
  const path=join(root,"state/gator/conversations",owner,"requests.json"),saved=JSON.parse(await readFile(path,"utf8")),r=saved.requests[0]
  expect(saved.requests).toHaveLength(1);expect(r.id).toBe(original.id);expect(r.plan.approval).toBeUndefined();expect(r.plan.attempts).toHaveLength(1);expect(r.plan.draining_run).toBeUndefined()
  expect(r.plan.attempts[0].usage.model_requests).toBeGreaterThan(0)
  await approve(original,"stale-plan-approval");expect(executions).toBe(1)
  const fresh=client.state!.task_plans![0]!
  await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:fresh.id,revision:fresh.revision,digest:fresh.digest,action:"inspect",request_id:"inspect-remaining-budget"}})
  await until(()=>client.state?.task_plan_result?.request_id==="inspect-remaining-budget","remaining budget review")
  expect(client.state?.task_plan_result?.review?.budget.max_steps).toBe(r.plan.budget.max_steps-r.plan.attempts[0].execution.steps)
  expect(client.state?.task_plan_result?.review?.original_budget?.max_steps).toBe(r.plan.budget.max_steps)
  expect(client.state?.task_plan_result?.review?.attempts?.[0]?.run_id).toBe(r.plan.attempts[0].run_id)

  await client.edit("Preserve this unrelated draft")
  await approve(fresh,"fresh-plan-approval");await until(()=>!client.state?.running&&executions===2,"only freshly reviewed replacement executes")
  const finished=JSON.parse(await readFile(path,"utf8")).requests[0]
  expect(finished.id).toBe(original.id);expect(finished.state).toBe("completed")
  const revision=JSON.parse(await readFile(join(root,"state/gator/conversations",owner,"revisions",finished.revision_id+".json"),"utf8"))
  const manifest=JSON.parse(await readFile(join(revision.bundle_path,"manifest.json"),"utf8"))
  expect(manifest.execution_accounting.limits.model_requests).toBe(256-r.plan.attempts[0].usage.model_requests)
  expect(manifest.execution_accounting.max_steps).toBe(r.plan.budget.max_steps-r.plan.attempts[0].execution.steps)
  release();await client.close();client=new GatorClient(new Application(options));await client.start(owner)
  expect(client.draft.text).toBe("Preserve this unrelated draft");expect(executions).toBe(2);expect(proposals).toHaveLength(2)
  if(delegated){await client.host.sessions.open({conversation_id:parent});await client.loadDraft();expect(client.draft.text).toBe("Preserve requesting chat draft");expect(parentExecutions).toBe(1)}
 }finally{release();await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)

test("an uncertain original effect blocks replacement approval without replay",async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-uncertain-replan-"));let release!:()=>void;const effect=new Promise<void>(r=>{release=r});let effects=0,work=0
 const plan={outcome:"Publish a reviewed update",steps:[{id:"publish",text:"Publish the exact reviewed update"}],sources:["release"],effects:["read","connected_write"]}
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  if(new URL(request.url).pathname==="/effect"){effects++;await effect;return Response.json({id:"unconfirmed"})}
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let message:any
  if(system.includes("You handle the conversation with Gator")){const meta=JSON.parse(body.messages.at(-1).content);message={role:"assistant",content:JSON.stringify({kind:"revise_plan",target:meta.tasks.find((t:any)=>t.can_revise_plan).id,instruction:meta.message})}}
  else if(system.includes("Propose a plan")){message={role:"assistant",content:JSON.stringify(plan)}}
  else{work++;message=work===1?{role:"assistant",tool_calls:[{id:"original-publish",type:"function",function:{name:"connector_release_publish",arguments:JSON.stringify({payload:{version:"v1"}})}}]}:{role:"assistant",content:"Inspect the original receipt before retrying."}}
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 const base=`http://127.0.0.1:${server.port}`
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base+"/chat",models:["fixture"],default_model:"fixture"}],connectors:[{version:1,id:"release",name:"Controlled release",kind:"http_webhook",resource:base+"/effect",authentication:"none"}]}))
 const options={binary:process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env:{...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options))
 try{
  await client.start();await client.send({type:"submit",text:"/mode act"});await client.send({type:"submit",text:"/connector add release"});await until(()=>!!client.state?.work_details?.connector_ids.includes("release"),"selected connected scope")
  await client.edit("Publish the update",[],undefined,true);await client.submit();await until(()=>client.state?.task_plans?.[0]?.phase==="review","plan review")
  const original=client.state!.task_plans![0]!,owner=client.state!.conversation_id!
  const approve=async(value:typeof original,id:string)=>client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:value.id,revision:value.revision,digest:value.digest,action:"approve",request_id:id}})
  await approve(original,"publish-plan-review");await until(()=>!!client.state?.pending_approval,"exact external review")
  const pending=client.state!.pending_approval!;expect(effects).toBe(0)
  await client.send({type:"approval",interaction_id:pending.id,approval_token:pending.token,approved:true});await until(()=>effects===1,"external request admitted")
  await client.edit("Correct this task before any further publishing");await client.submit()
  await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"replacement plan review")
  const fresh=client.state!.task_plans![0]!
  await approve(fresh,"uncertain-replacement-review")
  await until(()=>client.state?.task_plan_result?.request_id==="uncertain-replacement-review","rejected uncertain replacement")
  expect(client.state?.task_plan_result?.error).toContain("uncertain")
  expect(effects).toBe(1);expect(work).toBe(1)
  const saved=JSON.parse(await readFile(join(root,"state/gator/conversations",owner,"requests.json"),"utf8")).requests[0]
  expect(saved.id).toBe(original.id);expect(saved.plan.approval).toBeUndefined();expect(saved.plan.attempts[0].uncertain).toBe(true)
  release();await client.close();client=new GatorClient(new Application(options));await client.start(owner)
  expect(client.state?.running).toBe(false);expect(effects).toBe(1);expect(work).toBe(1)
  await approve(client.state!.task_plans![0]!,"uncertain-after-restart");expect(client.state?.task_plan_result?.error).toContain("uncertain");expect(effects).toBe(1)
 }finally{release();await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
