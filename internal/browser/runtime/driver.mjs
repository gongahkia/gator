import { chromium } from 'playwright';
import { createInterface } from 'node:readline';

const maxTextBytes = 32 * 1024;
const maxElements = 128;
const maxArtifactBytes = 8 * 1024 * 1024;

let browser;
let context;
let managed = false;
let origins = new Set();
let tabSequence = 0;
const pageIDs = new WeakMap();
const pages = new Map();
const refs = new Map();

function fail(message) {
  throw new Error(message);
}

function tabID(page) {
  let id = pageIDs.get(page);
  if (!id) {
    id = `tab-${++tabSequence}`;
    pageIDs.set(page, id);
    pages.set(id, page);
  }
  return id;
}

function pageFor(tabID) {
  const page = pages.get(tabID);
  if (!page || page.isClosed()) fail('browser tab is unavailable');
  return page;
}

function allowedURL(value) {
  let parsed;
  try { parsed = new URL(value); } catch { return false; }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return false;
  const port = parsed.port || (parsed.protocol === 'http:' ? '80' : '443');
  return origins.has(`${parsed.protocol}//${parsed.hostname.toLowerCase()}:${port}`);
}

async function applyPolicy(page) {
  if (page.__gatorPolicyApplied) return;
  page.__gatorPolicyApplied = true;
  await page.route('**/*', async route => {
    const request = route.request();
    if (allowedURL(request.url())) return route.continue();
    return route.abort('blockedbyclient');
  });
  if (typeof page.routeWebSocket === 'function') {
    await page.routeWebSocket('**/*', async route => {
      if (!allowedURL(route.url())) return route.close();
      return route.connectToServer();
    });
  }
  page.on('popup', popup => { void popup.close(); });
}

async function knownPages() {
  const result = [];
  for (const item of context.pages()) {
    if (item.isClosed()) continue;
    const id = tabID(item);
    result.push({ id, title: await item.title().catch(() => ''), url: item.url() });
  }
  return result;
}

function truncate(value, limit) {
  const bytes = Buffer.from(value, 'utf8');
  if (bytes.length <= limit) return { value, truncated: false };
  return { value: bytes.subarray(0, Math.max(0, limit - 14)).toString('utf8') + '\n[truncated]', truncated: true };
}

async function snapshot(tabID) {
  const page = pageFor(tabID);
  await applyPolicy(page);
  const text = truncate(await page.locator('body').innerText({ timeout: 5000 }).catch(() => ''), maxTextBytes);
  const prior = refs.get(tabID);
  const generation = (prior?.generation || 0) + 1;
  const entries = new Map();
  const metadata = await page.locator('a,button,input,textarea,select,[role="button"],[role="link"]').evaluateAll((nodes, maximum) => nodes.slice(0, maximum).map((node, index) => {
    const element = node;
    const tag = element.tagName.toLowerCase();
    const inputType = tag === 'input' ? (element.getAttribute('type') || 'text').toLowerCase() : '';
    const autocomplete = (element.getAttribute('autocomplete') || '').toLowerCase();
    const identity = [element.id, element.getAttribute('name'), autocomplete].join(' ').toLowerCase();
    const sensitive = inputType === 'password' || inputType === 'file' || /(username|password|webauthn|one-time-code)/.test(identity);
    const role = element.getAttribute('role') || (tag === 'a' ? 'link' : tag === 'button' ? 'button' : tag);
    const name = (element.getAttribute('aria-label') || element.innerText || element.getAttribute('placeholder') || element.getAttribute('name') || '').trim().replace(/\s+/g, ' ').slice(0, 512);
    return { index, role, name, disabled: Boolean(element.disabled), sensitive };
  }), maxElements);
  for (const element of metadata) {
    const ref = `e${generation}-${element.index + 1}`;
    entries.set(ref, page.locator('a,button,input,textarea,select,[role="button"],[role="link"]').nth(element.index));
    element.ref = ref;
    delete element.index;
  }
  refs.set(tabID, { generation, entries });
  return { tab_id: tabID, url: page.url(), title: await page.title().catch(() => ''), text: text.value, elements: metadata, truncated: text.truncated };
}

function locatorFor(tabID, ref) {
  const state = refs.get(tabID);
  if (!state || !state.entries.has(ref)) fail('browser element ref is stale; request a fresh browser snapshot');
  return state.entries.get(ref);
}

async function sensitive(locator) {
  return locator.evaluate(node => {
    const tag = node.tagName.toLowerCase();
    const type = tag === 'input' ? (node.getAttribute('type') || 'text').toLowerCase() : '';
    const identity = [node.id, node.getAttribute('name'), node.getAttribute('autocomplete')].join(' ').toLowerCase();
    return type === 'password' || type === 'file' || /(username|password|webauthn|one-time-code)/.test(identity);
  });
}

async function readDownload(download) {
  const stream = await download.createReadStream();
  if (!stream) fail('browser download stream is unavailable');
  const chunks = [];
  let size = 0;
  for await (const chunk of stream) {
    size += chunk.length;
    if (size > maxArtifactBytes) {
      stream.destroy();
      fail('browser download exceeds 8 MiB');
    }
    chunks.push(chunk);
  }
  return { name: download.suggestedFilename() || 'download', data: Buffer.concat(chunks).toString('base64') };
}

async function start(params) {
  if (browser) fail('browser driver is already started');
  managed = params.mode === 'managed';
  if (managed) {
    browser = await chromium.launch({ headless: !params.headed });
    context = await browser.newContext({ acceptDownloads: true, serviceWorkers: 'block' });
    await context.newPage();
  } else {
    if (typeof params.cdp_endpoint !== 'string') fail('browser CDP endpoint is required');
    browser = await chromium.connectOverCDP(params.cdp_endpoint);
    context = browser.contexts()[0];
    if (!context) fail('attached Chromium browser has no default context');
  }
  await knownPages();
  return { mode: params.mode };
}

async function dispatch(method, params = {}) {
  switch (method) {
    case 'start': return start(params);
    case 'set_origins': {
      if (params.origins !== null && !Array.isArray(params.origins)) fail('browser origins are invalid');
      origins = new Set((params.origins || []).map(item => item.url));
      return { ok: true };
    }
    case 'candidate_tabs': return knownPages();
    case 'snapshot': return snapshot(params.tab_id);
    case 'screenshot': {
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      return { png: (await page.screenshot({ type: 'png' })).toString('base64') };
    }
    case 'navigate': {
      if (!allowedURL(params.url)) fail('browser URL origin is not approved for this session');
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      await page.goto(params.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
      return snapshot(params.tab_id);
    }
    case 'click': {
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      const locator = locatorFor(params.tab_id, params.ref);
      await locator.click({ timeout: 10000 });
      return snapshot(params.tab_id);
    }
    case 'fill': {
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      const locator = locatorFor(params.tab_id, params.ref);
      if (await sensitive(locator)) fail('browser refuses password and autocomplete-sensitive fields; use developer takeover to sign in');
      await locator.fill(params.value, { timeout: 10000 });
      return snapshot(params.tab_id);
    }
    case 'select': {
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      const locator = locatorFor(params.tab_id, params.ref);
      if (await sensitive(locator)) fail('browser refuses sensitive fields');
      await locator.selectOption(params.value, { timeout: 10000 });
      return snapshot(params.tab_id);
    }
    case 'press': {
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      await page.keyboard.press(params.key, { timeout: 10000 });
      return snapshot(params.tab_id);
    }
    case 'download': {
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      const locator = locatorFor(params.tab_id, params.ref);
      const [download] = await Promise.all([page.waitForEvent('download', { timeout: 30000 }), locator.click({ timeout: 10000 })]);
      return readDownload(download);
    }
    case 'upload': {
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      const locator = locatorFor(params.tab_id, params.ref);
      if (!(await locator.evaluate(node => node.tagName.toLowerCase() === 'input' && (node.getAttribute('type') || '').toLowerCase() === 'file'))) fail('browser upload requires a file input from the latest snapshot');
      await locator.setInputFiles(params.path, { timeout: 10000 });
      return snapshot(params.tab_id);
    }
    case 'close': {
      if (browser) await browser.close();
      browser = undefined;
      return { ok: true };
    }
    default: fail('browser driver method is unsupported');
  }
}

const lines = createInterface({ input: process.stdin, crlfDelay: Infinity });
lines.on('line', async line => {
  let request;
  try {
    request = JSON.parse(line);
    const result = await dispatch(request.method, request.params);
    process.stdout.write(JSON.stringify({ id: request.id, result }) + '\n');
  } catch (error) {
    process.stdout.write(JSON.stringify({ id: request?.id || 0, error: error instanceof Error ? error.message : 'browser driver failed' }) + '\n');
  }
});
