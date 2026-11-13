// Headless Go/host integration; no terminal renderer or keyboard interaction.
import {test,expect} from "bun:test"
import {execFileSync} from "node:child_process"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../../ui/src/gator/client"
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const planned of [false,true])for(const scenario of ["missing","credentials","permission"] as const)test(`${planned?"planned":"direct"} task retains its original identity through ${scenario} setup, account review and fresh execution`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-task-access-"));let resumed=false,reads=0,modelCalls=0,proposals=0;const workBodies:string[]=[]
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  if(new URL(request.url).pathname==="/reports"){reads++;expect(request.headers.get("authorization")).toBe("Bearer controlled-task-token");return Response.json({summary:"silver otter source"})}
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let message:any
  if(system.includes("Propose a plan")){proposals++;const meta=JSON.parse(body.messages.at(-1).content);message={role:"assistant",content:JSON.stringify({outcome:"Summarize the requested report",steps:[{id:"read",text:"Read the selected report and summarize it"}],sources:planned&&scenario==="missing"&&proposals===1?[]:["reports"],effects:["read"]})}}
  else{
   modelCalls++;workBodies.push(JSON.stringify(body));const last=body.messages.at(-1)
   if(resumed){message=last.role==="tool"&&last.name==="connector_reports_fetch"?{role:"assistant",content:"The report says silver otter."}:{role:"assistant",tool_calls:[{id:"recovered-report",type:"function",function:{name:"connector_reports_fetch",arguments:"{}"}}]}}
   else if(last.role==="tool"){message={role:"assistant",content:"The original task needs account setup before I can finish."}}
   else{const need={kind:"http_json",operation:"fetch",...(scenario!=="missing"?{connector_id:"reports"}:{})};message={role:"assistant",tool_calls:[{id:"required-access",type:"function",function:scenario==="credentials"?{name:"connector_reports_fetch",arguments:"{}"}:{name:"gator_request_access",arguments:JSON.stringify(need)}}]}}
  }
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 const base=`http://127.0.0.1:${server.port}`;await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base+"/chat",models:["fixture"],default_model:"fixture"}],connectors:scenario==="missing"?[]:[{version:1,id:"reports",name:"Requested reports",kind:"http_json",resource:base+"/reports",authentication:"bearer"}],...(scenario==="permission"?{connector_permissions:[{connector_id:"reports",operation:"fetch",read:"deny"}]}:{})}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config"),GATOR_TASK_ACCESS_FIXTURE_TOKEN:"controlled-task-token"},options={binary,env,userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options))
 const cli=(...args:string[])=>execFileSync(binary,args,{env,encoding:"utf8"})
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});if(scenario!=="missing")await client.send({type:"submit",text:"/connector add reports"})
  const owner=client.state!.conversation_id!,path=join(root,"state/gator/conversations",owner,"requests.json"),file=join(root,"original.txt");await writeFile(file,"Private original input: golden heron");await client.attach([file]);const input=client.draft.uploads![0]!.id
  await client.edit("Summarize the requested report",client.draft.uploads,undefined,planned);await client.submit()
  if(planned){await until(()=>client.state?.task_plans?.[0]?.phase==="review","initial review");const p=client.state!.task_plans![0]!;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:p.id,revision:p.revision,digest:p.digest,action:"approve",request_id:"original-review"}})}
  await until(()=>!!client.state?.request_recoveries?.length&&!client.state?.running,"retained task prerequisite")
  const task=client.state!.request_recoveries![0]!,original=JSON.parse(await readFile(path,"utf8")).requests[0];expect(task.recovery.kind).toBe("source_access");expect(task.recovery.needs?.[0]?.operation).toBe("fetch");expect(task.recovery.retry_safe).toBe(true);expect(reads).toBe(0);expect(original.state).toBe("failed");expect(original.options.Uploads).toEqual([input])
  const action=async(action:"review_access"|"prepare_access",id:string,connections?:string[],access_token?:string)=>client.result({type:"request_recovery",request_recovery:{conversation_id:owner,task_id:task.task_id,token:task.token,action,request_id:id,connections,access_token}},id,s=>s.request_recovery_result)
  const first=(await action("review_access","inspect-missing-access")).access_review!;expect(first.ready).toBe(false)
  await client.edit("Keep my newer draft",[],undefined,true);await client.close();client=new GatorClient(new Application(options));await client.start(owner);expect(client.draft.text).toBe("Keep my newer draft");expect(client.state?.request_recoveries?.[0]?.token).toBe(task.token);expect(reads).toBe(0)
  if(scenario==="missing")cli("provider","connector","add","reports","--kind","json","--name","Requested reports","--url",base+"/reports","--auth","bearer")
  cli("provider","connector","login","reports","--from-env","GATOR_TASK_ACCESS_FIXTURE_TOKEN")
  if(scenario==="permission")cli("provider","connector","permission","reports","fetch","read","allow")
  const review=(await action("review_access","review-repaired-access",["reports"])).access_review!;expect(review.ready).toBe(true);expect(review.connections).toEqual(["reports"]);expect(review.requirements[0]!.choices[0]!.destination).toBe(base+"/reports")
  await expect(action("prepare_access","stale-account-review",["reports"],first.token)).rejects.toThrow("changed");expect(reads).toBe(0)
  const outcome=await action("prepare_access","prepare-repaired-task",["reports"],review.token);expect(outcome.accepted).toBe(true);await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"new exact task review")
  const prepared=JSON.parse(await readFile(path,"utf8")).requests[0];expect(prepared.id).toBe(original.id);expect(prepared.options.Uploads).toEqual([input]);expect(prepared.options.ConnectorIDs).toEqual(["reports"]);expect(prepared.plan.attempts).toHaveLength(1);expect(prepared.plan.input_changes.at(-1).access.review_id).toBe(review.token);expect(prepared.plan.input_changes[0].before_options).toEqual(original.options);expect(prepared.plan.approval).toBeUndefined();expect(reads).toBe(0)
  expect(client.draft.text).toBe("Keep my newer draft");expect(client.draft.plan_first).toBe(true);expect(client.state?.work_details?.connector_ids).toEqual(scenario==="missing"?[]:["reports"])
  const fresh=client.state!.task_plans![0]!;resumed=true;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:fresh.id,revision:fresh.revision,digest:fresh.digest,action:"approve",request_id:"fresh-access-plan-review"}});await until(()=>!client.state?.running&&reads===1,"repaired task executes once")
  const completed=JSON.parse(await readFile(path,"utf8")).requests[0];expect(completed.id).toBe(original.id);expect(completed.state).toBe("completed");expect(workBodies.at(-1)).toContain("golden heron")
  await client.close();client=new GatorClient(new Application(options));await client.start(owner);expect(reads).toBe(1);expect(client.draft.text).toBe("Keep my newer draft");expect(client.state?.request_recoveries??[]).toHaveLength(0)
 }finally{await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)

for(const scenario of ["missing","permission"] as const)test(`delegated ${scenario} access repair stays in the original child and preserves independent parent work`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-delegated-access-"));let release!:()=>void;const held=new Promise<void>(r=>{release=r});let resumed=false,reads=0,parentCalls=0
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  if(new URL(request.url).pathname==="/reports"){reads++;return Response.json({summary:"Controlled report"})}
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let message:any
  if(system.includes("You handle the conversation with Gator")){message={role:"assistant",content:JSON.stringify({kind:"task"})}}
  else if(system.includes("Propose a plan")){message={role:"assistant",content:JSON.stringify({outcome:"Reviewed child report",steps:[{id:"read",text:"Summarize the requested report"}],sources:["reports"],effects:["read"]})}}
  else if(body.messages.some((m:any)=>m.role==="user"&&m.content==="Research an unrelated topic")){parentCalls++;await held;message={role:"assistant",content:"Independent parent work completed."}}
  else if(resumed){message=body.messages.at(-1).role==="tool"&&body.messages.at(-1).name==="connector_reports_fetch"?{role:"assistant",content:"The delegated report is complete."}:{role:"assistant",tool_calls:[{id:"child-read",type:"function",function:{name:"connector_reports_fetch",arguments:"{}"}}]}}
  else{message=body.messages.at(-1).role==="tool"?{role:"assistant",content:"The child task needs source access."}:{role:"assistant",tool_calls:[{id:"child-requirement",type:"function",function:{name:"gator_request_access",arguments:JSON.stringify({kind:"http_json",operation:"fetch",...(scenario==="permission"?{connector_id:"reports"}:{})})}}]}}
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 const base=`http://127.0.0.1:${server.port}`;await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base+"/chat",models:["fixture"],default_model:"fixture"}],connectors:scenario==="missing"?[]:[{version:1,id:"reports",name:"Requested reports",kind:"http_json",resource:base+"/reports",authentication:"none"}],...(scenario==="permission"?{connector_permissions:[{connector_id:"reports",operation:"fetch",read:"deny"}]}:{})}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},options={binary,env,userData:join(root,"app"),workspace:join(root,"workspace")};const client=new GatorClient(new Application(options)),cli=(...args:string[])=>execFileSync(binary,args,{env})
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});if(scenario==="permission")await client.send({type:"submit",text:"/connector add reports"})
  const parent=client.state!.conversation_id!;await client.edit("Research an unrelated topic");await client.submit();await until(()=>parentCalls===1,"independent original work")
  await client.edit("Summarize the requested report");await client.submit();await until(()=>!!client.state?.dialogue?.[0]?.task_chat,"child dispatched");const owner=client.state!.dialogue![0]!.task_chat!;await client.edit("Preserve parent draft",[],undefined,true);await client.openSession(owner);await until(()=>!!client.state?.request_recoveries?.length&&!client.state?.running,"child blocked on account setup")
  const task=client.state!.request_recoveries![0]!,path=join(root,"state/gator/conversations",owner,"requests.json"),original=JSON.parse(await readFile(path,"utf8")).requests[0]
  if(scenario==="missing")cli("provider","connector","add","reports","--kind","json","--url",base+"/reports","--auth","none")
  else cli("provider","connector","permission","reports","fetch","read","allow")
  const request_id="review-delegated-access",frame={conversation_id:owner,task_id:task.task_id,token:task.token,connections:["reports"]}
  const review=(await client.result({type:"request_recovery",request_recovery:{...frame,action:"review_access",request_id}},request_id,s=>s.request_recovery_result)).access_review!;expect(review.ready).toBe(true)
  await client.edit("Keep child draft");const prepare="prepare-delegated-access";await client.result({type:"request_recovery",request_recovery:{...frame,action:"prepare_access",access_token:review.token,request_id:prepare}},prepare,s=>s.request_recovery_result);await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"revised delegated task")
  if(scenario==="missing"){
   const previous=client.state!.task_plans![0]!,before=JSON.parse(await readFile(path,"utf8")).requests[0]
   cli("provider","connector","permission","reports","fetch","read","allow")
   const stale="stale-delegated-scope";await expect(client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:previous.id,revision:previous.revision,digest:previous.digest,action:"approve",request_id:stale}},stale,s=>s.task_plan_result)).rejects.toThrow("scope changed");expect(reads).toBe(0)
   const current=client.state!.request_recoveries!.find(r=>r.task_id===task.task_id)!,reviewID="rereview-delegated-scope",next={conversation_id:owner,task_id:task.task_id,token:current.token}
   const updated=(await client.result({type:"request_recovery",request_recovery:{...next,action:"review_access",request_id:reviewID}},reviewID,s=>s.request_recovery_result)).access_review!;expect(updated.requirements[0]!.selected).toBe("reports")
   const prepareID="prepare-delegated-scope";await client.result({type:"request_recovery",request_recovery:{...next,action:"prepare_access",connections:["reports"],access_token:updated.token,request_id:prepareID}},prepareID,s=>s.request_recovery_result)
   await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running&&client.state.task_plans[0].revision>previous.revision,"delegated scope refreshed")
   expect(JSON.parse(await readFile(path,"utf8")).requests[0].plan.attempts).toEqual(before.plan.attempts)
  }
  resumed=true;const plan=client.state!.task_plans![0]!;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:plan.id,revision:plan.revision,digest:plan.digest,action:"approve",request_id:"approve-delegated-access"}});await until(()=>reads===1&&!client.state?.running,"delegated read executes under new reviewed scope")
  const completed=JSON.parse(await readFile(path,"utf8")).requests[0];expect(completed.state).toBe("completed");expect(completed.id).toBe(original.id);expect(client.draft.text).toBe("Keep child draft");await client.openSession(parent);expect(client.draft.text).toBe("Preserve parent draft");expect(client.draft.plan_first).toBe(true);expect(client.state?.running).toBe(true);expect(parentCalls).toBe(1)
 }finally{release();await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
