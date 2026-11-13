import {test,expect} from "bun:test"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../src/gator/client"
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
test("ordinary correction stops reviewed execution, retains evidence and spends only remaining budget after fresh review",async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-running-plan-"));let release!:()=>void;const held=new Promise<void>(r=>{release=r});let executions=0;const proposals:any[]=[]
 const plan={outcome:"Compare venues",steps:[{id:"compare",text:"Compare venues"}],sources:[],effects:["read"]}
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let content:string
  if(system.includes("You handle the conversation with Gator")){const meta=JSON.parse(body.messages.at(-1).content);content=JSON.stringify({kind:"revise_plan",target:meta.tasks.find((t:any)=>t.can_revise_plan).id,instruction:meta.message})}
  else if(system.includes("Propose a plan")){const meta=JSON.parse(body.messages.at(-1).content);proposals.push(meta);content=JSON.stringify({...plan,outcome:meta.correction?"Venues under $2,000":plan.outcome})}
  else{executions++;if(executions===1)await held;content="Reviewed venue comparison completed."}
  return Response.json({choices:[{message:{role:"assistant",content},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:`http://127.0.0.1:${server.port}/chat`,models:["fixture"],default_model:"fixture"}]}))
 const options={binary:process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env:{...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options))
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"})
  await client.edit("Compare venues",[],undefined,true);await client.submit();await until(()=>client.state?.task_plans?.[0]?.phase==="review","initial review")
  const original=client.state!.task_plans![0]!,owner=client.state!.conversation_id!
  const approve=async(value:typeof original,id:string)=>client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:value.id,revision:value.revision,digest:value.digest,action:"approve",request_id:id}})
  await approve(original,"initial-plan-approval");await until(()=>executions===1&&!!client.state?.running,"reviewed execution held")
  await client.edit("Keep venues under $2,000");await client.submit()
  await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running&&proposals.length===2,"corrected plan after accounting")
  expect(client.state?.dialogue?.at(-1)?.state).toBe("guided");expect(executions).toBe(1)
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
 }finally{release();await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
