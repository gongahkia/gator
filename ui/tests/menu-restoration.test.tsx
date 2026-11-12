import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { InputRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { join } from "node:path"
import { tmpdir } from "node:os"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

for (const [width,height] of [[110,34],[80,24],[60,20],[42,14]]) test(`Back restores filtered parent selection and draft at ${width}×${height}`,async()=>{
  const root=await mkdtemp(join(tmpdir(),"gator-menu-back-")),host=new FixtureHost({binary:"/unused",workspace:root,userData:root,env:{GATOR_STATE_DIR:root}}),client=new GatorClient(host)
  host.change({...state(),composer:{id:"a".repeat(64),effort:"standard",models:[{provider:"fixture",model:"model",label:"Fixture model"}]}})
  const setup=await testRender(()=><App client={client} quit={()=>client.close()} clipboard={{read:async()=>undefined,write:async()=>{}}}/>,{width,height})
  const flush=async()=>{await setup.flush();await Bun.sleep(35);await setup.flush()}
  const filter=()=>{const editor=setup.renderer.currentFocusedEditor;expect(editor).toBeInstanceOf(InputRenderable);return(editor as InputRenderable).value}
  try{
    await client.start();await client.edit("Keep my\nindependent draft");await client.setPlanFirst(true)
    await client.navigate("/models");await flush();await setup.mockInput.typeText("Choose");await flush()
    setup.mockInput.pressArrow("down");await flush();setup.mockInput.pressEnter();await flush()
    expect(setup.captureCharFrame()).toContain("Effort: low")
    await setup.mockInput.typeText("high");await flush();expect(filter()).toBe("high")
    setup.mockInput.pressEscape();await flush();expect(filter()).toBe("Choose")
    expect(client.draft.text).toBe("Keep my\nindependent draft");expect(client.draft.plan_first).toBe(true)
    setup.mockInput.pressEnter();await flush();expect(setup.captureCharFrame()).toContain("Effort: low")
    setup.mockInput.pressEscape();await flush();expect(filter()).toBe("Choose")
    setup.mockInput.pressEscape();await flush();expect(setup.renderer.currentFocusedEditor).not.toBeInstanceOf(InputRenderable)
    expect(host.sent).toHaveLength(0)
    // A separately opened root does not inherit a previous menu's query.
    await client.navigate("/models");await flush();expect(filter()).toBe("")
  }finally{await client.close();await setup.dispose();await rm(root,{recursive:true,force:true})}
})

for (const [width,height] of [[110,34],[80,24],[60,20],[42,14]]) test(`nested result Back restores reading position and survives changed rows at ${width}×${height}`,async()=>{
  const root=await mkdtemp(join(tmpdir(),"gator-menu-results-")),host=new FixtureHost({binary:"/unused",workspace:root,userData:root,env:{GATOR_STATE_DIR:root}}),client=new GatorClient(host)
  const bundle={Path:root,Status:"completed",RunID:"run-one",ManifestSHA256:"b".repeat(64),Candidates:null,Artifacts:Array.from({length:40},(_,i)=>({Path:`report-${String(i).padStart(2,"0")}.md`,MediaType:"text/markdown",SHA256:"a".repeat(64),Valid:true,Preview:`Retained preview ${i}`}))}
  host.change({...state(),work_details:{mode:"draft",attachments:[],connector_ids:[],latest_bundle:bundle}})
  const setup=await testRender(()=><App client={client} quit={()=>client.close()} clipboard={{read:async()=>undefined,write:async()=>{}}}/>,{width,height})
  const flush=async()=>{await setup.flush();await Bun.sleep(35);await setup.flush()}
  const choose=async(text:string)=>{await setup.mockInput.typeText(text);await flush();setup.mockInput.pressEnter();await flush()}
  const filter=()=> (setup.renderer.currentFocusedEditor as InputRenderable).value
  try{
    await client.start();await client.edit("Draft stays here");await client.navigate("/outputs");await flush()
    await setup.mockInput.typeText("report");await flush()
    for(let step=0;step<40 && !setup.captureCharFrame().split("\n").some(line=>line.includes("┃") && line.includes("report-37.md"));step++){setup.mockInput.pressArrow("down");await flush()}
    const original=setup.captureCharFrame();expect(original).toContain("report-37.md")
    setup.mockInput.pressEnter();await flush();await choose("Inspect retained preview")
    expect(setup.captureCharFrame()).toContain("report-37.md")
    setup.mockInput.pressEscape();await flush();expect(filter()).toBe("Inspect retained preview")
    setup.mockInput.pressEscape();await flush();expect(filter()).toBe("report")
    expect(setup.captureCharFrame()).toBe(original)
    setup.mockInput.pressEnter();await flush()
    host.change({...host.current,work_details:{...host.current.work_details!,latest_bundle:{...bundle,Artifacts:bundle.Artifacts.slice(20).toReversed()}}});await flush()
    setup.mockInput.pressEscape();await flush();expect(filter()).toBe("report")
    setup.mockInput.pressEnter();await flush();expect(setup.captureCharFrame()).toContain("report-37.md")
    expect(client.draft.text).toBe("Draft stays here");expect(host.sent).toHaveLength(0)
  }finally{await client.close();await setup.dispose();await rm(root,{recursive:true,force:true})}
})
