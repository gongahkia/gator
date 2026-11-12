import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

for (const [width, height] of [[110,34],[80,24],[60,20],[42,14]]) test(`conversation-only replies expose configured item actions at ${width}×${height}`, async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-dialogue-actions-")), copied:string[]=[]
  const host = new FixtureHost({ binary:"/unused", userData:root, workspace:root, env:{GATOR_STATE_DIR:root,GATOR_CONFIG_DIR:root} }), client = new GatorClient(host)
  host.change({ ...state(), desktop:{shortcuts:{context_actions:"Ctrl+K"}}, dialogue:[{id:"reply-one",sequence:1,text:"My question",response:"A retained direct reply.",state:"answered",created_at:new Date().toISOString(),updated_at:new Date().toISOString()}] })
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{read:async()=>undefined,write:async text=>{copied.push(text)}}}/>,{width,height})
  const flush = async()=>{await setup.flush();await Bun.sleep(35);await setup.flush()}
  try {
    await client.start(); await client.edit("Keep my draft"); await flush()
    expect(host.current.messages).toHaveLength(0)
    const composer = setup.renderer.root.findDescendantById("composer")!
    expect(composer).toBeDefined(); expect(setup.captureCharFrame()).toContain("Ctrl+K actions")
    setup.mockInput.pressKey("k",{ctrl:true});await flush()
    expect(setup.captureCharFrame()).toContain("Conversation item actions")
    await setup.mockInput.typeText("Copy retained reply");await flush();setup.mockInput.pressEnter();await flush()
    expect(copied).toEqual(["A retained direct reply."]);expect(client.draft.text).toBe("Keep my draft");expect(host.sent).toHaveLength(0)
    setup.mockInput.pressKey("k",{ctrl:true});await flush()
    host.change({...host.current,dialogue:[{...host.current.dialogue![0]!,response:"Changed reply."}]});await flush()
    await setup.mockInput.typeText("Copy retained reply");await flush();setup.mockInput.pressEnter();await flush()
    expect(copied).toHaveLength(1);expect(client.issue).toContain("item changed")
  } finally { await client.close();await setup.dispose();await rm(root,{recursive:true,force:true}) }
})
