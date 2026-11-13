// Headless backend proof: rejected preparation is not a missing execution receipt.
import {expect,test} from "bun:test"
import {execFileSync} from "node:child_process"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../../ui/src/gator/client"
async function until(check:()=>boolean|Promise<boolean>,label:string){const end=Date.now()+15000;while(!await check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const planned of [false,true])for(const replace of [false,true])for(const early of planned?[false,true]:[false])test(`${planned?early?"before first proposal":"reviewed":"direct"} removed-source preparation recovers ${replace?"with replacement":"after repair"} without an invented run`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-preparation-recovery-"));let calls=0,reads=0;const target=replace?"replacement":"reports"
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  if(new URL(request.url).pathname==="/source"){reads++;return Response.json({summary:"original report"})}
  const body=await request.json() as any;calls++;const system=body.messages.find((m:any)=>m.role==="system")?.content??"";let message:any
  if(system.includes("Propose a plan")){const meta=JSON.parse(body.messages.at(-1).content);message={role:"assistant",content:JSON.stringify({outcome:"Reviewed original report",steps:[{id:"read",text:"Summarize the required source"}],sources:meta.selected_source_ids,effects:["read"]})}}
  else{const name=`connector_${target}_fetch`;message=body.messages.at(-1).role==="tool"&&body.messages.at(-1).name===name?{role:"assistant",content:"The original report is available."}:{role:"assistant",tool_calls:[{id:"final-read",type:"function",function:{name,arguments:"{}"}}]}}
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 const base=`http://127.0.0.1:${server.port}`;await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base+"/chat",models:["fixture"],default_model:"fixture"}],connectors:[{version:1,id:"reports",name:"Reports",kind:"http_json",resource:base+"/source",authentication:"none"}]}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},options={binary,env,userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options));const cli=(...args:string[])=>execFileSync(binary,args,{env,encoding:"utf8"})
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});await client.send({type:"submit",text:"/connector add reports"});const owner=client.state!.conversation_id!,path=join(root,"state/gator/conversations",owner,"requests.json"),file=join(root,"original.txt");await writeFile(file,"Private original: golden heron");await client.attach([file]);const input=client.draft.uploads![0]!.id
  if(!planned||early)cli("provider","connector","remove","reports","--yes")
  await client.edit("Summarize the required source",client.draft.uploads,undefined,planned);await client.submit()
  if(planned&&!early){await until(()=>client.state?.task_plans?.[0]?.phase==="review","initial proposal");const p=client.state!.task_plans![0]!;cli("provider","connector","remove","reports","--yes");await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:p.id,revision:p.revision,digest:p.digest,action:"approve",request_id:"original-plan-review"}})}
  await until(async()=>!client.state?.running&&JSON.parse(await readFile(path,"utf8")).requests[0]?.state==="failed","preparation rejection")
  const original=JSON.parse(await readFile(path,"utf8")).requests[0],task=client.state?.request_recoveries?.find(r=>r.task_id===original.id)
  expect(task?.recovery.preparation).toBeDefined();expect(original.work_id).toBeUndefined();expect(original.revision_id).toBeUndefined();expect(calls).toBe(planned&&!early?1:0);expect(reads).toBe(0);expect(original.options.Uploads).toEqual([input])
  await client.edit("Preserve the newer draft",[],undefined,true);await client.close();client=new GatorClient(new Application(options));await client.start(owner);expect(calls).toBe(planned&&!early?1:0);expect(client.draft.text).toBe("Preserve the newer draft")
  const current=()=>client.state!.request_recoveries!.find(r=>r.task_id===original.id)!
  const recover=async(action:"review_access"|"prepare_access",request_id:string,connections?:string[],access_token?:string)=>client.result({type:"request_recovery",request_recovery:{conversation_id:owner,task_id:original.id,token:current().token,action,request_id,connections,access_token}},request_id,s=>s.request_recovery_result)
  const missing=(await recover("review_access","inspect-missing-source")).access_review!;expect(missing.ready).toBe(false);expect(missing.requirements[0]!.need.connector_id).toBe("reports")
  cli("provider","connector","add",target,"--kind","json","--url",base+"/source","--auth","none")
  const review=(await recover("review_access","review-repaired-source",[target])).access_review!;expect(review.ready).toBe(true);expect(review.connections).toEqual([target]);await expect(recover("prepare_access","reject-old-source-review",[target],missing.token)).rejects.toThrow("changed")
  await recover("prepare_access","prepare-original-task",[target],review.token);await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"recovered original plan")
  const saved=JSON.parse(await readFile(path,"utf8")).requests[0];expect(saved.id).toBe(original.id);expect(saved.options.Uploads).toEqual([input]);expect(saved.plan.attempts??[]).toHaveLength(0);expect(saved.plan.draining_run).toBeUndefined();expect(reads).toBe(0)
  if(replace){
   const stale=client.state!.task_plans![0]!;cli("provider","connector","permission",target,"fetch","read","allow")
   const rejected="reject-changed-recovery-scope";await expect(client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:stale.id,revision:stale.revision,digest:stale.digest,action:"approve",request_id:rejected}},rejected,s=>s.task_plan_result)).rejects.toThrow("scope changed")
   const updated=(await recover("review_access","rereview-unstarted-recovery")).access_review!;expect(updated.requirements[0]!.selected).toBe(target)
   await recover("prepare_access","reprepare-unstarted-recovery",[target],updated.token);await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running&&client.state.task_plans[0].revision>stale.revision,"unstarted pending account rereview")
   const again=JSON.parse(await readFile(path,"utf8")).requests[0];expect(again.plan.attempts??[]).toHaveLength(0);expect(again.options.Uploads).toEqual([input]);expect(reads).toBe(0)
  }
  const fresh=client.state!.task_plans![0]!;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:fresh.id,revision:fresh.revision,digest:fresh.digest,action:"approve",request_id:"approve-repaired-task"}});await until(()=>reads===1&&!client.state?.running,"first actual work executes once")
  const completed=JSON.parse(await readFile(path,"utf8")).requests[0];expect(completed.id).toBe(original.id);expect(completed.state).toBe("completed");expect(client.draft.text).toBe("Preserve the newer draft");expect(client.draft.plan_first).toBe(true)
  const revision=JSON.parse(await readFile(join(root,"state/gator/conversations",owner,"revisions",completed.revision_id+".json"),"utf8")),manifest=JSON.parse(await readFile(join(revision.bundle_path,"manifest.json"),"utf8"));expect(manifest.execution_accounting.limits.model_requests).toBe(256)
 }finally{await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)

for(const placement of ["before_submission","during_routing","after_review"])test(`delegated source removal ${placement} preserves original admitted scope and independent parent work`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-delegated-preparation-"));let release!:()=>void;const held=new Promise<void>(r=>{release=r});let parentCalls=0,reads=0,childCalls=0;const childBodies:string[]=[]
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  if(new URL(request.url).pathname==="/source"){reads++;return Response.json({summary:"delegated original report"})}
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let message:any
  if(system.includes("You handle the conversation with Gator")){const routed=JSON.parse(body.messages.at(-1).content);if(routed.message==="What is two plus two?"){message={role:"assistant",content:JSON.stringify({kind:"reply",text:"Four."})}}else{if(placement==="during_routing")cli("provider","connector","remove","reports","--yes");message={role:"assistant",content:JSON.stringify({kind:"task"})}}}
  else if(system.includes("Propose a plan")){const meta=JSON.parse(body.messages.at(-1).content);message={role:"assistant",content:JSON.stringify({outcome:"Reviewed original report",steps:[{id:"read",text:"Summarize the required source"}],sources:meta.selected_source_ids,effects:["read"]})}}
  else if(body.messages.some((m:any)=>m.role==="user"&&m.content==="Research an unrelated topic")){parentCalls++;await held;message={role:"assistant",content:"Independent parent finished."}}
  else{childCalls++;childBodies.push(JSON.stringify(body));message=body.messages.at(-1).role==="tool"?{role:"assistant",content:"Delegated original report is complete."}:{role:"assistant",tool_calls:[{id:"repaired-child-read",type:"function",function:{name:"connector_replacement_fetch",arguments:"{}"}}]}}
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 const base=`http://127.0.0.1:${server.port}`;await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base+"/chat",models:["fixture"],default_model:"fixture"}],connectors:[{version:1,id:"reports",name:"Reports",kind:"http_json",resource:base+"/source",authentication:"none"}]}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")};const client=new GatorClient(new Application({binary,env,userData:join(root,"app"),workspace:join(root,"workspace")})),cli=(...args:string[])=>execFileSync(binary,args,{env})
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});await client.send({type:"submit",text:"/connector add reports"});const parent=client.state!.conversation_id!
  await client.edit("Research an unrelated topic");await client.submit();await until(()=>parentCalls===1,"independent parent execution")
  if(placement==="before_submission"){
   cli("provider","connector","remove","reports","--yes");await client.edit("What is two plus two?");await client.submit();await until(()=>client.state?.dialogue?.some(d=>d.response==="Four.")??false,"unrelated conversation with unavailable source")
   expect(parentCalls).toBe(1);expect(childCalls).toBe(0);expect(reads).toBe(0);expect(client.state?.running).toBe(true)
  }
  const file=join(root,"private.txt");await writeFile(file,"Private input: golden heron");await client.attach([file]);const upload=client.draft.uploads![0]!.id
  const taskMessage=()=>client.state?.dialogue?.find(d=>d.text==="Summarize the required source")
  await client.edit("Summarize the required source",client.draft.uploads,undefined,true);await client.submit();await until(()=>!!taskMessage()?.task_chat,"reviewed child delegated")
  const owner=taskMessage()!.task_chat!;await client.edit("Keep parent draft",[],undefined,true);await client.openSession(owner);const path=join(root,"state/gator/conversations",owner,"requests.json")
  if(placement==="after_review"){
   await until(()=>client.state?.task_plans?.[0]?.phase==="review","child review");const before=client.state!.task_plans![0]!;cli("provider","connector","remove","reports","--yes")
   await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:before.id,revision:before.revision,digest:before.digest,action:"approve",request_id:"approve-original-child"}})
  }
  await until(()=>!!client.state?.request_recoveries?.length&&!client.state?.running,"unstarted child recovery").catch(async error=>{const rows=JSON.parse(await readFile(path,"utf8")).requests;throw Error(error.message+": "+JSON.stringify({running:client.state?.running,result:client.state?.task_plan_result,requests:rows.map((r:any)=>({state:r.state,issue:r.issue,recovery:r.recovery,phase:r.plan?.phase}))}))});expect(childCalls).toBe(0);expect(reads).toBe(0)
  const original=JSON.parse(await readFile(path,"utf8")).requests[0];expect(original.recovery.preparation.request_id).toBe(original.id);expect(original.options.Uploads).toEqual([upload]);expect(original.work_id).toBeUndefined()
  cli("provider","connector","add","replacement","--kind","json","--url",base+"/source","--auth","none")
  const task=client.state!.request_recoveries![0]!,frame={conversation_id:owner,task_id:task.task_id,token:task.token,connections:["replacement"]},id="review-child-source"
  const review=(await client.result({type:"request_recovery",request_recovery:{...frame,action:"review_access",request_id:id}},id,s=>s.request_recovery_result)).access_review!;expect(review.ready).toBe(true)
  await client.edit("Keep child draft",[],undefined,true);const prepare="prepare-child-source";await client.result({type:"request_recovery",request_recovery:{...frame,action:"prepare_access",access_token:review.token,request_id:prepare}},prepare,s=>s.request_recovery_result)
  await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"same child fresh review")
  const pending=JSON.parse(await readFile(path,"utf8")).requests[0];expect(pending.id).toBe(original.id);expect(pending.plan.attempts??[]).toHaveLength(0);expect(pending.options.Uploads).toEqual([upload])
  const plan=client.state!.task_plans![0]!;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:plan.id,revision:plan.revision,digest:plan.digest,action:"approve",request_id:"approve-repaired-child"}})
  await until(()=>reads===1&&!client.state?.running,"repaired delegated execution")
  const completed=JSON.parse(await readFile(path,"utf8")).requests[0];expect(completed.state).toBe("completed");expect(completed.id).toBe(original.id);expect(childBodies.at(-1)).toContain("golden heron");expect(client.draft.text).toBe("Keep child draft")
  await client.openSession(parent);expect(client.state?.running).toBe(true);expect(parentCalls).toBe(1);expect(client.draft.text).toBe("Keep parent draft");expect(client.draft.plan_first).toBe(true)
 }finally{release();await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
