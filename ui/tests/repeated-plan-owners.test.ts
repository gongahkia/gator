import {test,expect} from "bun:test"
import {mkdtemp,mkdir,readFile,writeFile,rm} from "node:fs/promises"
import {execFileSync} from "node:child_process"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../src/gator/client"
const plan={outcome:"Venue comparison",steps:[{id:"compare",text:"Compare venues"}],sources:[],effects:["read"]}
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const outcome of ["success","failure","crash"] as const)test.skipIf(outcome==="crash"&&process.platform==="win32")(`delegated repeated correction preserves its original owners through ${outcome}`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-repeat-owners-"));let releaseMain!:()=>void,releaseFirst!:()=>void,releaseLatest!:()=>void
 const main=new Promise<void>(r=>{releaseMain=r}),first=new Promise<void>(r=>{releaseFirst=r}),latest=new Promise<void>(r=>{releaseLatest=r})
 const proposals:any[]=[];let work=0
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let content:string
  if(system.includes("You handle the conversation with Gator")){
   const meta=JSON.parse(body.messages.at(-1).content)
   content=JSON.stringify(meta.message==="Compare venues"?{kind:"task"}:meta.message==="Hello during revision"?{kind:"reply",text:"Both original tasks remain available."}:{kind:"revise_plan",target:meta.tasks.find((t:any)=>t.can_revise_plan).id,instruction:meta.message})
  }else if(system.includes("Propose a plan")){
   const meta=JSON.parse(body.messages.at(-1).content);proposals.push(meta)
   if(!meta.correction)content=JSON.stringify(plan)
   else{await(meta.prior_corrections?.length?latest:first);content=outcome==="failure"&&meta.prior_corrections?.length?"Invalid provider proposal":JSON.stringify({...plan,outcome:"Accessible venues under $2,000"})}
  }else{work++;await main;content="Original unrelated research completed."}
  return Response.json({choices:[{message:{role:"assistant",content},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 const binary=process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env={...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")}
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:`http://127.0.0.1:${server.port}/chat`,models:["fixture"],default_model:"fixture"}]}))
 const options={binary,env,userData:join(root,"app"),workspace:join(root,"workspace")};let client=new GatorClient(new Application(options))
 const send=async(text:string)=>{await client.edit(text);await client.submit();await until(()=>!client.state?.dialogue_busy&&client.state?.dialogue?.at(-1)?.text===text,text);const issue=client.state?.dialogue?.at(-1)?.issue;if(issue)throw Error(issue)}
 const journal=async(id:string)=>JSON.parse(await readFile(join(root,"state/gator/conversations",id,"requests.json"),"utf8"))
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});const parent=client.state!.conversation_id!
  await client.edit("Research another topic");await client.submit();await until(()=>work===1,"parent running")
  const brief=join(root,"brief.txt");await writeFile(brief,"Original selected input: silver otter");await client.attach([brief]);const input=client.draft.uploads![0]!.id
  await client.setPlanFirst(true);await send("Compare venues");await until(()=>!!client.state?.dialogue_peers?.some(p=>p.can_revise_plan),"child review")
  const child=client.state!.dialogue![0]!.task_chat!,before=await journal(child)
  const oldEpoch=client.state!.dialogue_peers!.find(p=>p.conversation_id===child)!.run_id
  await send("Keep venues under $2,000");await until(()=>proposals.length===2&&!!client.state?.dialogue_peers?.some(p=>p.conversation_id===child&&p.can_revise_plan&&p.run_id!==oldEpoch),"first revision held with refreshed exact control")
  await send("Also require accessible venues");await until(()=>proposals.length===3,"latest revision in original child")
  expect(proposals[2].prior_corrections).toEqual(["Keep venues under $2,000"]);expect(proposals[2].work_limits).toEqual(proposals[0].work_limits);expect(proposals[2].selected_source_ids).toEqual(proposals[0].selected_source_ids)
  expect(client.state?.conversation_id).toBe(parent);expect(client.state?.running).toBe(true)
  await send("Hello during revision");expect(work).toBe(1);await client.edit("Keep the requesting chat draft")
  if(outcome==="crash"){
   const children=execFileSync("ps",["-axo","pid=,ppid=,command="],{encoding:"utf8"}).split("\n").map(line=>line.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/)).filter(m=>m&&Number(m[2])===process.pid&&m[3].includes(binary)&&m[3].includes("ui-backend")&&m[3].includes(child))
   expect(children).toHaveLength(1);process.kill(Number(children[0]![1]),"SIGKILL")
   await until(()=>client.snapshot.chatActivity.some(item=>item.id===child&&item.status==="Disconnected"),"crashed child")
  }else{releaseLatest();releaseFirst();await until(()=>client.state?.dialogue?.find(t=>t.text==="Also require accessible venues")?.state===(outcome==="success"?"guided":"failed"),"actual latest outcome")}
  const saved=await journal(child);expect(saved.requests).toHaveLength(1);expect(saved.requests[0].id).toBe(before.requests[0].id);expect(saved.requests[0].options.Uploads).toEqual([input]);expect(saved.requests[0].options.DialogueParent).toBe(parent);expect(saved.requests[0].plan.approval).toBeUndefined();expect(saved.requests[0].plan.edit.prior_corrections[0].instruction).toBe("Keep venues under $2,000")
  expect(client.state?.conversation_id).toBe(parent);expect(client.draft.text).toBe("Keep the requesting chat draft");expect(work).toBe(1)
  releaseMain();releaseFirst();releaseLatest();await client.close();const count=proposals.length
  client=new GatorClient(new Application(options));await client.start(child)
  expect(client.state?.running).toBe(false);expect(client.state?.task_plans?.[0]?.id).toBe(before.requests[0].id);expect(client.state?.task_plans?.[0]?.phase).toBe("review");expect(proposals).toHaveLength(count);expect(work).toBe(1)
  const recovered=await journal(child)
  expect(recovered.requests[0].plan.edit.state).toBe(outcome==="success"?"applied":"failed")
  if(outcome==="crash")expect(recovered.requests[0].plan.edit.issue).toContain("will not replay automatically")
  expect(recovered.requests[0].plan.approval).toBeUndefined()
  await client.close();client=new GatorClient(new Application(options));await client.start(parent);expect(client.draft.text).toBe("Keep the requesting chat draft");expect(proposals).toHaveLength(count);expect(work).toBe(1)
  expect(client.state?.dialogue?.find(t=>t.text==="Keep venues under $2,000")?.state).toBe("interrupted")
  expect(client.state?.dialogue?.find(t=>t.text==="Also require accessible venues")?.state).toBe(outcome==="success"?"guided":"failed")
 }finally{releaseMain();releaseFirst();releaseLatest();await client.close();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
