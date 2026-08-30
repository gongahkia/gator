import { chromium } from 'playwright';
import { createInterface } from 'node:readline';
import { lookup } from 'node:dns/promises';
import { isIP } from 'node:net';

const maxTextBytes = 32 * 1024;
const maxElements = 128;
const maxArtifactBytes = 8 * 1024 * 1024;

let browser;
let context;
let managed = false;
let origins = new Set();
let tabSequence = 0;
let managedPolicyApplied = false;
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

function ipv6Groups(value) {
  let address = value.toLowerCase();
  if (isIP(address) !== 6) return null;
  const lastColon = address.lastIndexOf(':');
  const tail = address.slice(lastColon + 1);
  if (isIP(tail) === 4) {
    const bytes = tail.split('.').map(Number);
    address = `${address.slice(0, lastColon)}:${((bytes[0] << 8) | bytes[1]).toString(16)}:${((bytes[2] << 8) | bytes[3]).toString(16)}`;
  }
  const parts = address.split('::');
  if (parts.length > 2) return null;
  const left = parts[0] === '' ? [] : parts[0].split(':');
  const right = parts.length === 1 || parts[1] === '' ? [] : parts[1].split(':');
  const zeroes = 8 - left.length - right.length;
  if (zeroes < 0 || (parts.length === 1 && zeroes !== 0)) return null;
  const groups = [...left, ...Array(zeroes).fill('0'), ...right];
  if (groups.length !== 8 || groups.some(group => !/^[0-9a-f]{1,4}$/.test(group))) return null;
  return groups.map(group => Number.parseInt(group, 16));
}

function mappedIPv4Address(groups) {
  if (!groups || groups.slice(0, 5).some(group => group !== 0) || groups[5] !== 0xffff) return null;
  return `${groups[6] >> 8}.${groups[6] & 0xff}.${groups[7] >> 8}.${groups[7] & 0xff}`;
}

function publicIPv4Address(parts) {
  const [a, b, c] = parts;
  if (a === 0 || a === 10 || a === 127 || a >= 224) return false;
  if (a === 100 && b >= 64 && b <= 127) return false;
  if (a === 169 && b === 254) return false;
  if (a === 172 && b >= 16 && b <= 31) return false;
  if (a === 192 && b === 0 && c <= 255) return false;
  if (a === 192 && b === 2) return false;
  if (a === 192 && b === 88 && c === 99) return false;
  if (a === 192 && b === 168) return false;
  if (a === 198 && (b === 18 || b === 19 || b === 51)) return false;
  if (a === 203 && b === 0 && c === 113) return false;
  return true;
}

function publicAddress(value) {
  const address = value.toLowerCase();
  if (isIP(address) === 4) return publicIPv4Address(address.split('.').map(Number));
  const groups = ipv6Groups(address);
  if (!groups) return false;
  const mapped = mappedIPv4Address(groups);
  if (mapped) return publicIPv4Address(mapped.split('.').map(Number));
  const allZero = groups.every(group => group === 0);
  if (allZero || (groups.slice(0, 7).every(group => group === 0) && groups[7] === 1)) return false;
  if ((groups[0] & 0xfe00) === 0xfc00 || (groups[0] & 0xffc0) === 0xfe80 || (groups[0] & 0xff00) === 0xff00) return false;
  return !(groups[0] === 0x2001 && groups[1] === 0x0db8);
}

function loopbackAddress(value) {
  const address = value.toLowerCase();
  if (isIP(address) === 4) return /^127\./.test(address);
  const groups = ipv6Groups(address);
  const mapped = mappedIPv4Address(groups);
  if (mapped) return /^127\./.test(mapped);
  return groups !== null && groups.slice(0, 7).every(group => group === 0) && groups[7] === 1;
}

async function safeRequestURL(value) {
  if (!allowedURL(value)) return false;
  let parsed;
  try { parsed = new URL(value); } catch { return false; }
  const hostname = parsed.hostname.toLowerCase();
  const networkHost = hostname.replace(/^\[|\]$/g, '');
  let addresses;
  try {
    addresses = isIP(networkHost) ? [{ address: networkHost }] : await lookup(networkHost, { all: true, verbatim: true });
  } catch {
    return false;
  }
  if (!addresses.length) return false;
  const loopbackOrigin = networkHost === 'localhost' || loopbackAddress(networkHost);
  return addresses.every(item => loopbackOrigin ? loopbackAddress(item.address) : publicAddress(item.address));
}

async function applyPolicy(page) {
  if (managed && !managedPolicyApplied) {
    managedPolicyApplied = true;
    await context.route('**/*', async route => {
      if (await safeRequestURL(route.request().url())) return route.continue();
      return route.abort('blockedbyclient');
    });
    if (typeof context.routeWebSocket === 'function') {
      await context.routeWebSocket('**/*', async route => {
        if (!(await safeRequestURL(route.url()))) return route.close();
        return route.connectToServer();
      });
    }
    // Managed contexts begin with only Gator's initial page. Any new page is
    // a popup, and is immediately closed rather than becoming agent-visible.
    context.on('page', candidate => {
      if (candidate !== page) void candidate.close();
    });
    return;
  }
  if (page.__gatorPolicyApplied) return;
  page.__gatorPolicyApplied = true;
  await page.route('**/*', async route => {
    const request = route.request();
    if (await safeRequestURL(request.url())) return route.continue();
    return route.abort('blockedbyclient');
  });
  if (typeof page.routeWebSocket === 'function') {
    await page.routeWebSocket('**/*', async route => {
      if (!(await safeRequestURL(route.url()))) return route.close();
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
      if (!(await safeRequestURL(params.url))) fail('browser URL origin is not approved for this session or no longer resolves to an approved address');
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      await page.goto(params.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
      return snapshot(params.tab_id);
    }
    case 'click': {
      const page = pageFor(params.tab_id);
      await applyPolicy(page);
      const locator = locatorFor(params.tab_id, params.ref);
      let unexpectedDownload;
      const rejectDownload = download => {
        unexpectedDownload = download;
        void download.cancel();
      };
      page.on('download', rejectDownload);
      try {
        await locator.click({ timeout: 10000 });
        await page.waitForTimeout(100);
      } finally {
        page.off('download', rejectDownload);
      }
      if (unexpectedDownload) fail('browser click triggered a download; use browser_download so the developer can approve it explicitly');
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
