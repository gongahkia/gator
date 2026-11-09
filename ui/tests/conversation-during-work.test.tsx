import { expect, spyOn, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

async function until(predicate: () => boolean, label: string) { const deadline=Date.now()+15000;while(!predicate()){if(Date.now()>deadline)throw new Error(`Timed out: ${label}`);await Bun.sleep(15)} }

test("ordinary messages converse, steer an original task and delegate work without changing focus; results survive restart",async()=>{
  const root=await mkdtemp(join(tmpdir(),"gator-dialogue-"))
  const binary=process.env.GATOR_TEST_BINARY || resolve(import.meta.dir,"../runtime/"+(process.platform==="win32"?"gator-runtime.exe":"gator-runtime"))
  const env={...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config")}
  let releaseMain!:()=>void,releaseChild!:()=>void,releaseStop!:()=>void
  const mainGate=new Promise<void>(resolve=>{releaseMain=resolve}),childGate=new Promise<void>(resolve=>{releaseChild=resolve})
  const stopGate=new Promise<void>(resolve=>{releaseStop=resolve})
  const work:string[]=[],routes:any[]=[]
  const server=Bun.serve({hostname:"127.0.0.1",port:0,async fetch(request){
    const body=await request.json() as any, serialized=JSON.stringify(body)
    let content=""
    if(body.messages.some((message:any)=>message.role==="system" && message.content.includes("You handle the conversation with Gator"))){
      const context=JSON.parse(body.messages.at(-1).content);routes.push(context)
      if(context.message==="Hello while working")content=JSON.stringify({kind:"reply",text:"I'm here. The venue comparison is still working."})
      else if(context.message==="Keep the venues under $2,000")content=JSON.stringify({kind:"guide",target:context.tasks.find((task:any)=>task.title.includes("Compare venues")).id,instruction:"Keep the venues under $2,000"})
      else if(context.message==="Stop it")content=JSON.stringify({kind:"clarify",text:"Should I stop the venue comparison or the separate brief task?"})
      else if(context.message==="Stop the research task")content=JSON.stringify({kind:"stop",target:context.tasks.find((task:any)=>task.title.includes("Research a stop target")).id})
      else content=JSON.stringify({kind:"task"})
    }else{
      work.push(serialized)
      const raw=body.messages.find((message:any)=>message.role==="user")?.content;const objective=typeof raw==="string"?raw:(raw || []).map((part:any)=>part.text || "").join("\n")
      if(objective==="Research a stop target"){await stopGate;content="Research stopped without an external action."}
      else if(objective.startsWith("Read the separate brief")){await childGate;content="The separate brief says jade heron."}
      else {if(work.length===1)await mainGate;content=serialized.includes("$2,000")?"Venue comparison respects the $2,000 budget.":"Venue comparison prepared."}
    }
    return Response.json({choices:[{message:{role:"assistant",content},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}})
  }})
  const cli=(args:string[])=>execFileSync(binary,args,{env:{...env,GATOR_INTERNAL_COMMANDS:"1"}})
  cli(["provider","add","dialogue-fixture","--base-url",`http://127.0.0.1:${server.port}/v1/chat/completions`,"--model","fixture"])
  cli(["config","set","default-provider","dialogue-fixture"])
  const options={binary,env,userData:join(root,"app"),workspace:join(root,"workspace")}
  const client=new GatorClient(new Application(options))
  const warnings:string[]=[];const warning=spyOn(console,"warn").mockImplementation((...args)=>{warnings.push(args.map(String).join(" "))})
  const setup=await testRender(()=><App client={client} quit={()=>client.close()} clipboard={{read:async()=>({data:"",mime:"text/plain"}),write:async()=>{}}}/>,{width:110,height:34})
  const flush=async()=>{await setup.flush();await Bun.sleep(30);await setup.flush()}
  const send=async(text:string)=>{await client.edit(text);await flush();await client.submit();await until(()=>!client.state?.dialogue_busy,"conversation response: "+text);if(client.state?.dialogue?.at(-1)?.issue)throw new Error(JSON.stringify(client.state.dialogue.at(-1)));await flush()}
  try{
    await client.start();await flush();await client.send({type:"submit",text:"/mode inspect"});const parent=client.state!.conversation_id!
    await client.edit("Compare venues for the trip");await client.submit();await until(()=>work.length===1,"main task at provider")
    await send("Hello while working")
    expect(client.state?.conversation_id).toBe(parent);expect(client.state?.running).toBe(true);expect(client.state?.queue).toHaveLength(0)
    expect(client.state?.dialogue?.at(-1)?.response).toContain("I'm here")
    await send("Keep the venues under $2,000")
    expect(client.state?.dialogue?.at(-1)?.state).toBe("guided");expect(work).toHaveLength(1)
    const brief=join(root,"brief.txt");await writeFile(brief,"jade heron is the separate brief's project")
    await client.attach([brief]);const file=client.draft.uploads![0].id
    await send("Read the separate brief")
    const turn=client.state?.dialogue?.at(-1)!,child=turn.task_chat!
    expect(child).toBeTruthy();expect(child).not.toBe(parent);expect(turn.state).toBe("delegated");expect(turn.input_ids).toEqual([file]);expect(client.state?.conversation_id).toBe(parent)
    await until(()=>work.length===2 && !!client.state?.dialogue_peers?.some(peer=>peer.conversation_id===child && peer.state==="running"),"child executing")
    expect(work[1]).toContain("jade heron");expect(work[1]).not.toContain("Venue comparison prepared")
    await send("Stop it");expect(routes.at(-1).tasks).toHaveLength(2);expect(client.state?.dialogue?.at(-1)?.state).toBe("clarification")
    expect(client.state?.running).toBe(true)
    releaseChild();await until(()=>!!client.state?.dialogue_peers?.some(peer=>peer.conversation_id===child && peer.state==="completed"),"child result returned")
    await send("Research a stop target");const stopChild=client.state!.dialogue!.at(-1)!.task_chat!
    await until(()=>!!client.state?.dialogue_peers?.some(peer=>peer.conversation_id===stopChild && peer.can_stop),"independent stop target ready")
    await send("Stop the research task");expect(client.state?.dialogue?.at(-1)?.state).toBe("stopped");releaseStop()
    await until(()=>!!client.state?.dialogue_peers?.some(peer=>peer.conversation_id===stopChild && peer.state==="stopped"),"exact child stopped")
    expect(client.state?.running).toBe(true);expect(client.state?.conversation_id).toBe(parent)
    await client.edit("Keep this draft while inspecting");await client.navigate("/tasks");await flush()
    expect(setup.captureCharFrame()).toContain("Read the separate brief")
    setup.mockInput.pressEnter();await until(()=>client.state?.conversation_id===child && !client.snapshot.sessionSwitching,"open exact child")
    expect(client.state?.dialogue_task?.parent_id).toBe(parent);expect(client.state?.messages.some(message=>message.text.includes("jade heron"))).toBe(true)
    await client.navigate("/tasks");await flush();setup.mockInput.pressEnter();await until(()=>client.state?.conversation_id===parent && !client.snapshot.sessionSwitching,"return parent")
    await client.loadDraft();expect(client.draft.text).toBe("Keep this draft while inspecting")
    releaseMain();await until(()=>!client.snapshot.allChatsBusy,"main result finished")
    expect(work.some(body=>body.includes("$2,000"))).toBe(true)
    const original=JSON.parse(await readFile(join(env.GATOR_STATE_DIR,"gator/conversations",parent,"requests.json"),"utf8"))
    const independent=JSON.parse(await readFile(join(env.GATOR_STATE_DIR,"gator/conversations",child,"requests.json"),"utf8"))
    expect(original.requests).toHaveLength(1);expect(independent.requests).toHaveLength(1)
    expect(independent.requests[0].options.DialogueParent).toBe(parent)
    for(const [width,height] of [[110,34],[80,24],[60,20],[42,14]]){setup.resize(width,height);await flush();const frame=setup.captureCharFrame();expect(frame).toContain("Keep this draft");expect(frame).not.toContain("[object Object]")}
    const count=work.length+routes.length
    await client.close();await setup.dispose();expect(warnings).toHaveLength(0)
    const restarted=new GatorClient(new Application(options))
    try{await restarted.start();expect(restarted.state?.conversation_id).toBe(parent);expect(restarted.state?.dialogue?.some(message=>message.response?.includes("I'm here"))).toBe(true);expect(restarted.state?.dialogue_peers?.find(peer=>peer.conversation_id===child)?.summary).toContain("jade heron");expect(restarted.draft.text).toBe("Keep this draft while inspecting");expect(work.length+routes.length).toBe(count)}finally{await restarted.close()}
  }catch(error){process.stderr.write("JOURNEY FAILED: "+String(error)+"\n"+JSON.stringify({work:work.length,routes:routes.length,peers:client.state?.dialogue_peers})+"\n");throw new Error(String(error)+"\n"+JSON.stringify({issue:client.issue,state:client.state,work:work.length,routes:routes.length})+"\n"+setup.captureCharFrame())}
  finally{releaseMain();releaseChild();releaseStop();await client.close();if(!setup.renderer.isDestroyed)await setup.dispose();server.stop(true);warning.mockRestore();await rm(root,{recursive:true,force:true})}
},60000)
