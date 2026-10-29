import assert from "node:assert/strict";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

test("submitted panel secrets are cleared after save and transport failures", { skip: !process.env.GATOR_PANEL_BROWSER, timeout: 60000 }, async () => {
  const harness = `
      import React from 'react';
      import {createRoot} from 'react-dom/client';
      import {Panel} from '/src/Panel.tsx';
      window.gator = {platform:'darwin'};
      window.sent = [];
      function Harness() {
        const [panel, setPanel] = React.useState({id:'fixture',kind:'model-form',title:'Configure fixture',rows:[
          {label:'Model',input:{value:'initial',limit:100}},
          {label:'API key',input:{value:'',secret:true,limit:100}}
        ],hints:[{shortcut:'Enter',action:'save'}]});
        const send = async frame => {
          window.sent.push(structuredClone(frame));
          setPanel(previous => ({...previous,notice:'Save failed; re-enter the key.'}));
          if (window.failTransport) throw new Error('fixture transport failure');
        };
        return React.createElement(Panel,{panel,send});
      }
      createRoot(document.getElementById('root')).render(React.createElement(Harness));
    `;
  const server = await createServer({ root: fileURLToPath(new URL("..", import.meta.url)), server: { host: "127.0.0.1", port: 0 }, plugins: [{ name: "panel-fixture", resolveId(id) { if (id === "virtual:gator-panel-fixture") return "\0gator-panel-fixture"; }, load(id) { if (id === "\0gator-panel-fixture") return harness; } }] });
  let browser;
  try {
    await server.listen();
    browser = await chromium.launch({ executablePath: process.env.GATOR_PANEL_BROWSER === "1" ? undefined : process.env.GATOR_PANEL_BROWSER, headless: true });
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = []; page.on("pageerror", error => errors.push(error.message));
    await page.route("**/fixture", route => route.fulfill({ contentType: "text/html", body: `<!doctype html><div id="root"></div><script type="module" src="/@id/virtual:gator-panel-fixture"></script>` }));
    await page.goto(`http://127.0.0.1:${server.httpServer.address().port}/fixture`);
    const password = page.locator('input[type="password"]');
    await password.fill("synthetic-private-key").catch(error => { throw new Error(`${error.message}; page errors: ${errors.join("; ")}`); });
    await page.getByLabel("Model", { exact: true }).fill("retained-model");
    await page.getByRole("button", { name: /^Save/ }).click();
    await page.waitForFunction(() => window.sent.length === 1 && document.querySelector('input[type="password"]').value === "");
    await page.getByRole("button", { name: /^Save/ }).click();
    await page.waitForFunction(() => window.sent.length === 2);
    let frames = await page.evaluate(() => window.sent);
    assert.equal(frames[0].panel.fields[1], "synthetic-private-key");
    assert.equal(frames[1].panel.fields[1], undefined);
    assert.equal(frames[1].panel.fields[0], "retained-model");
    await page.evaluate(() => { window.failTransport = true; });
    await password.fill("synthetic-retry-key");
    await page.getByRole("button", { name: /^Save/ }).click();
    await page.waitForFunction(() => window.sent.length === 3 && document.querySelector('input[type="password"]').value === "");
    await page.getByRole("button", { name: /^Save/ }).click();
    await page.waitForFunction(() => window.sent.length === 4);
    frames = await page.evaluate(() => window.sent);
    assert.equal(frames[2].panel.fields[1], "synthetic-retry-key");
    assert.equal(frames[3].panel.fields[1], undefined);
  } finally { await browser?.close(); await server.close(); }
});
