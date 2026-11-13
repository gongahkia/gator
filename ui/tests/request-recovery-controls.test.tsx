import {expect,test} from "bun:test"
import {testRender} from "@opentui/solid"
import {mkdtemp,mkdir,writeFile,rm} from "node:fs/promises"
import {tmpdir} from "node:os"
import {join,resolve} from "node:path"
import {Application} from "@gator/application-host/application"
import {GatorClient} from "../src/gator/client"
import {App} from "../src/app"
async function until(check:()=>boolean,label:string){const end=Date.now()+15000;while(!check()){if(Date.now()>end)throw Error("Timed out: "+label);await Bun.sleep(20)}}
for(const [width,height] of [[110,34],[80,24],[60,20],[42,14]])test(`native keyboard recovery inspects failure and prepares original task for review at ${width}×${height}`,async()=>{
 const root=await mkdtemp(join(tmpdir(),"gator-recovery-controls-"));let repaired=false,work=0,proposals=0
 const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
  const body=await request.json() as any,system=body.messages.find((m:any)=>m.role==="system")?.content??"";let content:string
  if(system.includes("Propose a plan")){proposals++;content=JSON.stringify({outcome:"Reviewed recovered summary",steps:[{id:"read",text:"Summarize the original input"}],sources:[],effects:["read"]})}
  else{work++;if(!repaired)return Response.json({error:{message:"Controlled credential rejection"}},{status:401});content="Original request completed after review."}
  return Response.json({choices:[{message:{role:"assistant",content},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}})
 }})
 await mkdir(join(root,"config/gator"),{recursive:true});await writeFile(join(root,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:`http://127.0.0.1:${server.port}/chat`,models:["fixture"],default_model:"fixture"}]}))
 const options={binary:process.env.GATOR_TEST_BINARY||resolve(import.meta.dir,"../runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime")),env:{...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")},userData:join(root,"app"),workspace:join(root,"workspace")},client=new GatorClient(new Application(options))
 const setup=await testRender(()=><App client={client} quit={()=>client.close()} clipboard={{read:async()=>({data:"",mime:"text/plain"}),write:async()=>{}}}/>,{width,height})
 const flush=async()=>{await setup.flush();await Bun.sleep(25);await setup.flush()},choose=async(text:string)=>{setup.mockInput.pressKey("u",{ctrl:true});await setup.mockInput.typeText(text);await flush();setup.mockInput.pressEnter();await flush()}
 try{
  await client.start();await client.send({type:"submit",text:"/mode inspect"});await client.edit("Read the original input");await client.submit();await until(()=>!!client.state?.request_recoveries?.length&&!client.state?.running,"retained failure");await flush()
  expect(setup.captureCharFrame()).toContain("/request-recovery")
  await client.edit("Preserve newer draft",[],undefined,true);await client.navigate("/request-recovery");await flush();await choose("Read the original input");await choose("Inspect failure and repair");expect(setup.captureCharFrame()).toContain("Original task")
  setup.mockInput.pressEscape();await flush();await choose("Prepare retry for review");expect(setup.captureCharFrame().replace(/\besc\b/g," ").replace(/[│█▀▄]/g," ").replace(/\s+/g," ")).toContain("Prepare this task for fresh review?");expect(work).toBe(1);expect(proposals).toBe(0)
  repaired=true;setup.mockInput.pressEnter();await until(()=>client.state?.task_plans?.[0]?.phase==="review"&&!client.state?.running,"prepared plan");await flush();expect(work).toBe(1);expect(proposals).toBe(1);expect(client.draft.text).toBe("Preserve newer draft");expect(client.draft.plan_first).toBe(true)
  await client.navigate("/task-plans");await flush();await choose("Reviewed recovered summary");await choose("Approve plan and start");expect(work).toBe(1);setup.mockInput.pressEnter();await until(()=>work===2&&!client.state?.running,"fresh reviewed execution");expect(client.draft.text).toBe("Preserve newer draft")
 }finally{await client.close();if(!setup.renderer.isDestroyed)await setup.dispose();server.stop(true);await rm(root,{recursive:true,force:true})}
},45000)
