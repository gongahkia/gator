import {test,expect} from "bun:test"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../src/gator/client"
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const planned of [false,true])for(const status of [401,403,402])test(`${planned?"planned":"direct"} admitted model HTTP ${status} retains original inputs, scope and budget through repaired reviewed retry and restart`,async()=>{
 const credentialRepair=status===401&&!planned
 const root=await mkdtemp(join(tmpdir(),"gator-provider-recovery-"));let repaired=false,work=0,proposals=0;const bodies:string[]=[]
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??""
  if(system.includes("Propose a plan")){proposals++;return Response.json({choices:[{message:{role:"assistant",content:JSON.stringify({outcome:"Read original input after setup repair",steps:[{id:"read",text:"Summarize the original input"}],sources:[],effects:["read"]})},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})}
  work++;bodies.push(JSON.stringify(body));if(credentialRepair ? request.headers.get("authorization")!=="Bearer repaired-controlled-key" : !repaired)return Response.json({error:{message:"Controlled provider rejection"}},{status})
  return Response.json({choices:[{message:{role:"assistant",content:"Original input summary: silver otter."},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:`http://127.0.0.1:${server.port}/chat`,models:["fixture"],default_model:"fixture",...(credentialRepair?{api_key_env:"GATOR_RECOVERY_FIXTURE_KEY"}:{})}]}))
 const options={binary:process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env:{...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config"),GATOR_RECOVERY_FIXTURE_KEY:credentialRepair?"rejected-controlled-key":""},userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options))
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});const owner=client.state!.conversation_id!
  const path=join(root,"state/gator/conversations",owner,"requests.json"),input=join(root,"original.txt");await writeFile(input,"Original private input: silver otter");await client.attach([input]);const upload=client.draft.uploads![0]!.id
  await client.edit("Summarize the original input",client.draft.uploads,undefined,planned);await client.submit();if(planned){await until(()=>client.state?.task_plans?.[0]?.phase==="review","initial plan");const initial=client.state!.task_plans![0]!;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:initial.id,revision:initial.revision,digest:initial.digest,action:"approve",request_id:"initial-approval"}})}await until(()=>!!client.state?.request_recoveries?.length&&!client.state?.running,"retained provider rejection")
  const recovery=client.state!.request_recoveries![0]!,original=JSON.parse(await readFile(path,"utf8")).requests[0]
  expect(original.state).toBe("failed");expect(original.recovery.status).toBe(status);expect(recovery.recovery.retry_safe).toBe(true);expect(original.recovery.scope_digest).toHaveLength(64);expect(work).toBe(1);expect(proposals).toBe(planned?1:0);expect(original.options.Uploads).toEqual([upload])
  const failedRevision=await readFile(join(root,"state/gator/conversations",owner,"revisions",original.revision_id+".json"),"utf8")
  await client.edit("Preserve newer unrelated draft",[],undefined,true)
  await client.close();if(credentialRepair)options.env.GATOR_RECOVERY_FIXTURE_KEY="repaired-controlled-key";client=new GatorClient(new Application(options));await client.start(owner);expect(client.state?.request_recoveries?.[0]?.token).toBe(recovery.token);expect(work).toBe(1);expect(client.draft.text).toBe("Preserve newer unrelated draft");expect(client.draft.plan_first).toBe(true)
  const act=async(token:string,id:string)=>client.result({type:"request_recovery",request_recovery:{conversation_id:owner,task_id:recovery.task_id,token,action:"prepare_retry",request_id:id}},id,s=>s.request_recovery_result)
  await expect(act("0".repeat(64),"stale-repair")).rejects.toThrow("changed");expect(work).toBe(1);expect(proposals).toBe(planned?1:0)
  if(status===401&&!planned){
   const configPath=join(root,"config/gator/config.json"),before=await readFile(configPath,"utf8"),changed=JSON.parse(before);changed.custom_providers[0].models.push("additional-model")
   await writeFile(configPath,JSON.stringify(changed));await expect(act(recovery.token,"changed-scope-recovery")).rejects.toThrow("configuration changed");expect(work).toBe(1);expect(proposals).toBe(0);await writeFile(configPath,before)
  }
  repaired=true
  const accepted=await act(recovery.token,"repair-original");expect(accepted.accepted).toBe(true)
  await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"tool-free recovered plan")
  expect(work).toBe(1);expect(proposals).toBe(planned?2:1);expect(client.state?.dialogue?.at(-1)?.state).toBe("guided");expect(client.draft.text).toBe("Preserve newer unrelated draft");expect(client.draft.plan_first).toBe(true)
  let prepared=JSON.parse(await readFile(path,"utf8")).requests[0];expect(prepared.id).toBe(original.id);expect(prepared.options.Uploads).toEqual([upload]);if(!planned)expect(prepared.plan.input_changes[0].before_options).toEqual(original.options);else expect(prepared.options).toEqual(original.options);expect(prepared.plan.attempts).toHaveLength(1);expect(prepared.plan.attempts[0].run_id).toBe(original.work_id);expect(prepared.plan.attempts[0].usage.model_requests).toBeGreaterThan(0)
  await expect(act(recovery.token,"repeat-old-repair")).rejects.toThrow("changed");expect(proposals).toBe(planned?2:1)
  const fresh=client.state!.task_plans![0]!
  if(status===401&&!planned){
   const configPath=join(root,"config/gator/config.json"),before=await readFile(configPath,"utf8"),changed=JSON.parse(before);changed.custom_providers[0].models.push("additional-model")
   await writeFile(configPath,JSON.stringify(changed));const request_id="changed-scope-approval";await expect(client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:fresh.id,revision:fresh.revision,digest:fresh.digest,action:"approve",request_id}},request_id,s=>s.task_plan_result)).rejects.toThrow("scope changed");expect(work).toBe(1);await writeFile(configPath,before)
  }
  const repeated=planned&&status===401
  if(repeated)repaired=false
  await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:fresh.id,revision:fresh.revision,digest:fresh.digest,action:"approve",request_id:"fresh-recovery-review"}})
  await until(()=>work===2&&!client.state?.running,"fresh reviewed work")
  if(repeated){
   const second=client.state!.request_recoveries![0]!;expect(second.task_id).toBe(original.id);expect(second.recovery.retry_safe).toBe(true);expect(second.token).not.toBe(recovery.token)
   repaired=true;await act(second.token,"second-repaired-attempt");await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"second recovered plan")
   prepared=JSON.parse(await readFile(path,"utf8")).requests[0];expect(prepared.plan.attempts).toHaveLength(2);expect(prepared.plan.edit.prior_corrections).toHaveLength(1)
   const latest=client.state!.task_plans![0]!;await client.send({type:"task_plan",task_plan:{conversation_id:owner,task_id:latest.id,revision:latest.revision,digest:latest.digest,action:"approve",request_id:"second-fresh-review"}});await until(()=>work===3&&!client.state?.running,"second fresh reviewed work")
  }
  const completed=JSON.parse(await readFile(path,"utf8")).requests[0];expect(completed.id).toBe(original.id);expect(completed.state).toBe("completed");expect(bodies[1]).toContain("silver otter");expect(await readFile(join(root,"state/gator/conversations",owner,"revisions",original.revision_id+".json"),"utf8")).toBe(failedRevision)
  const revision=JSON.parse(await readFile(join(root,"state/gator/conversations",owner,"revisions",completed.revision_id+".json"),"utf8")),manifest=JSON.parse(await readFile(join(revision.bundle_path,"manifest.json"),"utf8"))
  expect(manifest.execution_accounting.limits.model_requests).toBe(256-prepared.plan.attempts.reduce((sum:number,attempt:any)=>sum+attempt.usage.model_requests,0))
  await client.close();client=new GatorClient(new Application(options));await client.start(owner);expect(work).toBe(repeated?3:2);expect(client.draft.text).toBe("Preserve newer unrelated draft");expect(client.state?.request_recoveries??[]).toHaveLength(0)
 }finally{await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)

for(const uncertain of [false,true])test(`provider rejection after ${uncertain?"uncertain":"completed"} external work cannot replay the original effect`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-effect-recovery-"));let work=0,effects=0
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  if(new URL(request.url).pathname==="/effect"){effects++;return Response.json(uncertain?{error:"Controlled uncertain outcome"}:{id:"confirmed"},{status:uncertain?500:200})}
  await request.json();work++;if(work>1)return Response.json({error:{message:"Controlled credential rejection after effect"}},{status:401})
  return Response.json({choices:[{message:{role:"assistant",tool_calls:[{id:"original-effect",type:"function",function:{name:"connector_release_publish",arguments:JSON.stringify({payload:{text:"Controlled loopback only"}})}}]},finish_reason:"tool_calls"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 const base=`http://127.0.0.1:${server.port}`;await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base+"/chat",models:["fixture"],default_model:"fixture"}],connectors:[{version:1,id:"release",name:"Controlled release",kind:"http_webhook",resource:base+"/effect",authentication:"none"}]}))
 const options={binary:process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env:{...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options))
 try{
  await client.start();await client.send({type:"submit",text:"/mode act"});await client.send({type:"submit",text:"/connector add release"});await until(()=>!!client.state?.work_details?.connector_ids.includes("release"),"selected connector")
  await client.edit("Publish the controlled update");await client.submit();await until(()=>!!client.state?.pending_approval,"exact approval");const approval=client.state!.pending_approval!;expect(effects).toBe(0);await client.send({type:"approval",interaction_id:approval.id,approval_token:approval.token,approved:true})
  await until(()=>!!client.state?.request_recoveries?.length&&!client.state?.running,"failed after effect")
  const recovery=client.state!.request_recoveries![0]!,owner=client.state!.conversation_id!;expect(effects).toBe(1);expect(work).toBe(2);expect(recovery.recovery.retry_safe).toBe(false)
  const frame={type:"request_recovery" as const,request_recovery:{conversation_id:owner,task_id:recovery.task_id,token:recovery.token,action:"prepare_retry" as const,request_id:"unsafe-retry"}}
  await expect(client.result(frame,"unsafe-retry",s=>s.request_recovery_result)).rejects.toThrow("receipts");expect(effects).toBe(1)
  await client.edit("Keep my unrelated draft");await client.close();client=new GatorClient(new Application(options));await client.start(owner);expect(work).toBe(2);expect(effects).toBe(1);expect(client.draft.text).toBe("Keep my unrelated draft");expect(client.state?.request_recoveries?.[0]?.recovery.retry_safe).toBe(false)
  const discard={...frame.request_recovery,action:"discard" as const,request_id:"discard-recovery"};await client.result({type:"request_recovery",request_recovery:discard},discard.request_id,s=>s.request_recovery_result);expect(client.state?.request_recoveries??[]).toHaveLength(0);expect(client.draft.text).toBe("Keep my unrelated draft");expect(effects).toBe(1)
 }finally{await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
