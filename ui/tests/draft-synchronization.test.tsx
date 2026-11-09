import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { TextareaRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

test("programmatic drafts wait for native layout and keep the latest text through work snapshots",async()=>{
  for(const [width,height] of [[110,34],[80,24],[60,20],[42,14]]){
    const root=await mkdtemp(join(tmpdir(),"gator-draft-sync-"))
    const host=new FixtureHost({binary:"/unused",workspace:root,userData:root,env:{GATOR_STATE_DIR:root}})
    host.current={...state(),running:true,dialogue_active:true}
    const client=new GatorClient(host)
    const setup=await testRender(()=><App client={client} quit={()=>client.close()} clipboard={{read:async()=>({data:"",mime:"text/plain"}),write:async()=>{}}}/>,{width,height})
    try{
      await client.start()
      // No initial flush: a restored/editor/voice draft can arrive before the
      // first computed native frame. It must not reenter an unlaid-out editor.
      await client.edit("A restored draft before layout")
      for(let index=0;index<10;index++){
        host.change({...host.current,status:`Working ${index}`,loading_frame:index})
        await client.edit(`Newest replacement ${index}\nStill an editable draft`)
      }
      await setup.flush();await setup.flush()
      const input=setup.renderer.root.findDescendantById("message-input")
      expect(input instanceof TextareaRenderable).toBe(true)
      expect((input as TextareaRenderable).plainText).toBe("Newest replacement 9\nStill an editable draft")
      expect(client.draft.text).toBe((input as TextareaRenderable).plainText)
      expect((await host.drafts.request({action:"list"})).drafts[0]?.text).toBe(client.draft.text)
      expect(host.sent).toHaveLength(0)
      expect(setup.captureCharFrame()).toContain("Newest replacement")
    }finally{await client.close();await setup.dispose();await rm(root,{recursive:true,force:true})}
  }
})
