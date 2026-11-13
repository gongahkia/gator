// Headless Go/host contract: rereview a task after its prepared scope changes.
import {expect,test} from "bun:test"
import {execFileSync} from "node:child_process"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../../ui/src/gator/client"
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const failProposal of [false,true])for(const replace of [false,true])test(`${failProposal?"failed-proposal":"prepared"} recovery can ${replace?"replace its selected account":"review its repaired credential"} without duplicating work or budget`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-access-rereview-"));let resumed=false,proposals=0;const planning:any[]=[];const reads={a:0,b:0}
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const path=new URL(request.url).pathname
  if(path==="/a"||path==="/b"){const key=path.slice(1) as "a"|"b";reads[key]++;expect(request.headers.get("authorization")).toBe("Bearer repaired-controlled-token");return Response.json({summary:"verified report"})}
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let message:any
  if(system.includes("Propose a plan")){proposals++;const meta=JSON.parse(body.messages.at(-1).content);planning.push(meta);if(failProposal&&proposals===2)return Response.json({error:{message:"Controlled planning-access rejection"}},{status:403});message={role:"assistant",content:JSON.stringify({outcome:meta.previous_plan?.outcome??"Reviewed report",steps:[{id:"read",text:"Summarize the required report"}],sources:meta.selected_source_ids,effects:["read"]})}}
  else if(!resumed){message=body.messages.at(-1).role==="tool"?{role:"assistant",content:"Source setup is required."}:{role:"assistant",tool_calls:[{id:"need-account",type:"function",function:{name:"gator_request_access",arguments:JSON.stringify({kind:"http_json",operation:"fetch"})}}]}}
  else{const name=`connector_reports_${replace?"b":"a"}_fetch`;message=body.messages.at(-1).role==="tool"&&body.messages.at(-1).name===name?{role:"assistant",content:"The reviewed report is complete."}:{role:"assistant",tool_calls:[{id:"final-read",type:"function",function:{name,arguments:"{}"}}]}}
  return Response.json({choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 const base=`http://127.0.0.1:${server.port}`;await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base+"/chat",models:["fixture"],default_model:"fixture"}]}))
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../../ui/runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_INTERNAL_COMMANDS:"1",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config"),GATOR_INITIAL_ACCESS_TOKEN:"initial-controlled-token",GATOR_REPAIRED_ACCESS_TOKEN:"repaired-controlled-token"},options={binary,env,userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options));const cli=(...args:string[])=>execFileSync(binary,args,{env,encoding:"utf8"})
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});const owner=client.state!.conversation_id!,path=join(root,"state/gator/conversations",owner,"requests.json"),file=join(root,"original.txt");await writeFile(file,"Private original: golden heron");await client.attach([file]);const input=client.draft.uploads![0]!.id;await client.edit("Read the required report");await client.submit();await until(()=>!!client.state?.request_recoveries?.length&&!client.state?.running,"original setup requirement")
  const id=client.state!.request_recoveries![0]!.task_id
  const recover=async(action:"review_access"|"prepare_access",request_id:string,connections?:string[],access_token?:string)=>{const task=client.state?.request_recoveries?.find(r=>r.task_id===id);if(!task)throw Error("Original task has no current recovery projection");return client.result({type:"request_recovery",request_recovery:{conversation_id:owner,task_id:id,token:task.token,action,connections,access_token,request_id}},request_id,s=>s.request_recovery_result)}
  cli("provider","connector","add","reports-a","--kind","json","--url",base+"/a","--auth","bearer");cli("provider","connector","login","reports-a","--from-env","GATOR_INITIAL_ACCESS_TOKEN")
  const first=(await recover("review_access","first-access-review",["reports-a"])).access_review!;await recover("prepare_access","first-access-prepare",["reports-a"],first.token);await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"first plan prepared")
  let before=JSON.parse(await readFile(path,"utf8")).requests[0],old=client.state!.task_plans![0]!,intended="Reviewed report"
  expect(client.state?.request_recoveries?.find(r=>r.task_id===id)?.state).toBe("queued")
  if(replace&&!failProposal){
   const bound=(await recover("review_access","scope-before-plan-edit")).access_review!,request_id="inspect-before-plan-edit"
   const inspect=(await client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:id,revision:old.revision,digest:old.digest,action:"inspect",request_id}},request_id,s=>s.task_plan_result)).review!
   intended="Reviewed report with user correction";const edit="manual-plan-edit";await client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:id,revision:old.revision,digest:old.digest,action:"edit",content:{...inspect.content!,outcome:intended},request_id:edit}},edit,s=>s.task_plan_result)
   old=client.state!.task_plans![0]!;before=JSON.parse(await readFile(path,"utf8")).requests[0]
   await expect(recover("prepare_access","scope-from-old-plan",["reports-a"],bound.token)).rejects.toThrow("changed")
  }
  await client.edit("Preserve the newer draft",[],undefined,true)
  cli("provider","connector","login","reports-a","--from-env","GATOR_REPAIRED_ACCESS_TOKEN")
  const stale="changed-scope-approval";await expect(client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:id,revision:old.revision,digest:old.digest,action:"approve",request_id:stale}},stale,s=>s.task_plan_result)).rejects.toThrow("scope changed");expect(reads).toEqual({a:0,b:0})
  const current=(await recover("review_access","default-current-account")).access_review!;expect(current.requirements[0]!.selected).toBe("reports-a")
  let selected="reports-a"
  if(replace){selected="reports-b";cli("provider","connector","add",selected,"--kind","json","--url",base+"/b","--auth","bearer");cli("provider","connector","login",selected,"--from-env","GATOR_REPAIRED_ACCESS_TOKEN")}
  const review=(await recover("review_access","new-access-review",[selected])).access_review!;expect(review.connections).toEqual([selected]);await expect(recover("prepare_access","old-scope-review",[selected],first.token)).rejects.toThrow("changed")
  await recover("prepare_access","new-access-prepare",[selected],review.token)
  if(failProposal){
   await until(()=>!client.state?.running&&client.state?.dialogue?.at(-1)?.state==="failed","failed replacement proposal retained")
   expect(client.state?.request_recoveries?.find(r=>r.task_id===id)?.state).toBe(replace?"interrupted":"queued")
   expect(JSON.parse(await readFile(path,"utf8")).requests[0].plan.attempts).toEqual(before.plan.attempts)
   const retry=(await recover("review_access","retry-failed-proposal")).access_review!;expect(retry.requirements[0]!.selected).toBe(selected);await recover("prepare_access","prepare-failed-proposal",[selected],retry.token)
  }
  await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running&&proposals===(failProposal?3:2),"replacement plan prepared")
  const saved=JSON.parse(await readFile(path,"utf8")).requests[0];expect(saved.id).toBe(id);expect(saved.options.Uploads).toEqual([input]);expect(saved.options.ConnectorIDs).toEqual([selected]);expect(saved.plan.attempts).toEqual(before.plan.attempts);expect(saved.plan.input_changes).toHaveLength(failProposal?3:2);expect(saved.plan.approval).toBeUndefined();expect(client.draft.text).toBe("Preserve the newer draft")
  const inspect="inspect-access-history",pending=client.state!.task_plans![0]!;const inspected=(await client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:id,revision:pending.revision,digest:pending.digest,action:"inspect",request_id:inspect}},inspect,s=>s.task_plan_result)).review!
  expect(inspected.access_changes).toHaveLength(failProposal?3:2);expect(inspected.access_changes![1]!.prior_plan?.digest).toBe(old.digest);expect(inspected.access_changes![1]!.prior_plan?.content.outcome).toBe(intended);expect(inspected.access_changes![1]!.before).toEqual(["reports-a"]);expect(inspected.access_changes![1]!.after).toEqual([selected]);expect(JSON.stringify(inspected)).not.toContain("credential_pins")
  expect(planning.at(-1).previous_plan?.outcome).toBe(intended);expect(JSON.stringify(planning)).not.toContain("oauth_pins")
  await client.close();client=new GatorClient(new Application(options));await client.start(owner);expect(client.draft.text).toBe("Preserve the newer draft");expect(client.draft.plan_first).toBe(true);expect((await recover("review_access","restored-account-review")).access_review!.requirements[0]!.selected).toBe(selected)
  const stalePlan="old-plan-token";await expect(client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:id,revision:old.revision,digest:old.digest,action:"approve",request_id:stalePlan}},stalePlan,s=>s.task_plan_result)).rejects.toThrow();expect(reads).toEqual({a:0,b:0})
  const latest=client.state!.task_plans![0]!;resumed=true;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:id,revision:latest.revision,digest:latest.digest,action:"approve",request_id:"fresh-final-plan"}});await until(()=>!client.state?.running&&reads[replace?"b":"a"]===1,"only newly reviewed work executes")
  const completed=JSON.parse(await readFile(path,"utf8")).requests[0];expect(completed.id).toBe(id);expect(completed.state).toBe("completed");if(replace)expect(reads.a).toBe(0)
  const revision=JSON.parse(await readFile(join(root,"state/gator/conversations",owner,"revisions",completed.revision_id+".json"),"utf8")),manifest=JSON.parse(await readFile(join(revision.bundle_path,"manifest.json"),"utf8"));expect(manifest.execution_accounting.limits.model_requests).toBe(256-before.plan.attempts[0].usage.model_requests)
 }finally{await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
