import { validateExternal } from './external.mjs';
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { Backend } from '../app-host/src/backend.mjs';
import { validateFrame } from '../app-host/src/policy.mjs';
import { authenticate, readToken, inspectionActor } from './auth.mjs';
import { validateInspection } from './inspection.mjs';
import { callControl, validateRemoteAction, validateControlReply } from './control.mjs';
export { readToken } from './auth.mjs';

const identity = /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/;
const creationIdentity = /^[a-zA-Z0-9][a-zA-Z0-9_-]{15,95}$/;
const execute = promisify(execFile);
const recoverable = worker => !!worker.state && !worker.unconfirmedAction && !['running', 'queued', 'pending_approval', 'pending_connection', 'external', 'panel', 'receipt_busy', 'catalog_busy'].some(key => worker.state?.[key]) && !worker.state?.queue?.length;
const idle = worker => !worker.closed && recoverable(worker);
const assets = new Map([
  ['/', ['index.html', 'text/html; charset=utf-8']],
  ['/external-view.mjs', ['external-view.mjs', 'text/javascript; charset=utf-8']],
  ['/work-view.mjs', ['work-view.mjs', 'text/javascript; charset=utf-8']],
  ['/work-actions.mjs', ['work-actions.mjs', 'text/javascript; charset=utf-8']],
  ['/client.mjs', ['client.mjs', 'text/javascript; charset=utf-8']],
  ['/styles.css', ['styles.css', 'text/css; charset=utf-8']],
]);
const browserHeaders = {
  'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'no-referrer', 'X-Frame-Options': 'DENY',
  'Content-Security-Policy': "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
};

export function createCompanion({ binary, workspace, env, tokenFile, origin,
  controlWork = (mode, input) => callControl(binary, workspace, env, mode, input),
  queryExternal = async ({ actor, mode, target, generation }) => {
    try {
      const {stdout}=await execute(binary,['companion-external',actor,mode,target,generation],{cwd:workspace,env:{...env,GATOR_COMPANION_EXTERNAL:'1'},timeout:20000,maxBuffer:64*1024,windowsHide:true});
      return JSON.parse(stdout);
    } catch {throw new Error('External inspection unavailable.');}
  },
  queryWork = async ({ actor, target, generation }) => {
    try {
      const { stdout } = await execute(binary, ['companion-inspect', actor, target, generation], { cwd: workspace, env: { ...env, GATOR_COMPANION_INSPECT: '1' }, timeout: 20000, maxBuffer: 128 * 1024, windowsHide: true });
      return JSON.parse(stdout);
    } catch { throw Object.assign(new Error('Work inspection is unavailable.'), { code: 'INSPECTION_UNAVAILABLE' }); }
  },
  queryCatalog = async ({ query, scope, after }) => {
    try {
      const { stdout } = await execute(binary, ['companion-catalog', query, scope, after], { cwd: workspace, env: { ...env, GATOR_COMPANION_CATALOG: '1' }, timeout: 20000, maxBuffer: 1024 * 1024, windowsHide: true });
      return JSON.parse(stdout);
    } catch {
      // execFile timeout errors can have a null code and embed the full command.
      // Keep host paths and diagnostics out of transport errors in every case.
      throw Object.assign(new Error('Conversation catalog is unavailable.'), { code: 'CATALOG_UNAVAILABLE' });
    }
  },
  createChat = async requestID => {
    try {
      const { stdout } = await execute(binary, ['companion-create-chat', requestID], { cwd: workspace, env: { ...env, GATOR_COMPANION_CREATE: '1' }, timeout: 20000, maxBuffer: 32768, windowsHide: true });
      return JSON.parse(stdout);
    } catch {
      throw Object.assign(new Error('Chat creation is unconfirmed. Retry the same request.'), { code: 'CREATION_UNCONFIRMED' });
    }
  },
  now = () => Date.now(),
  spawnBackend = (id) => new Backend(binary, workspace, { ...env, GATOR_UI_SESSION_HOST: '1', GATOR_UI_FRONTEND: 'companion' }, ['ui-backend', id, '']) }) {
  if (origin && (new URL(origin).origin !== origin || new URL(origin).protocol !== 'https:')) throw new Error('The companion browser origin must be an exact HTTPS origin.');
  const workers = new Map();
  const recoveries = new Map();
  const startedAt = now();
  let sequence = Promise.resolve(), stopping = false, pending = 0;
  const serial = task => {
    if (pending >= 32) throw new Error('Companion request queue is full.');
    pending++;
    const next = sequence.then(task).finally(() => pending--);
    sequence = next.catch(() => {});
    return next;
  };
  const worker = async (id, readOnly) => {
    if (!identity.test(id)) throw new Error('Invalid conversation identity.');
    let current = workers.get(id);
    if (current?.closed) {
      if (!readOnly || !recoverable(current)) throw new Error('The host process stopped with work that needs review. Review this conversation on the host before reopening it.');
      const attempts = (recoveries.get(id) || []).filter(at => now() - at < 300000);
      if (attempts.length >= 3 || attempts.length && now() - attempts.at(-1) < 10000) throw new Error('Idle session recovery is cooling down. Retry this read later; no action was replayed.');
      attempts.push(now());
      recoveries.set(id, attempts);
      // Keep the closed worker until a replacement confirms its identity. Failed
      // starts therefore remain bounded on subsequent browser polls too.
      const replacement = spawnBackend(id);
      try {
        await replacement.ready();
        if (replacement.state?.conversation_id !== id) throw new Error('The host opened a different conversation.');
        workers.set(id, replacement);
        current = replacement;
      } catch (error) { await replacement.stop(); throw error; }
    }
    if (!current) {
      if (workers.size >= 4) {
        const available = [...workers].find(([, value]) => idle(value));
        if (!available) throw new Error('Four conversations have pending work. Finish a task or review its queue on the host before opening another.');
        await available[1].stop();
        workers.delete(available[0]);
        recoveries.delete(available[0]);
      }
      current = spawnBackend(id);
      try {
        await current.ready();
        if (current.state?.conversation_id !== id) throw new Error('The host opened a different conversation.');
        workers.set(id, current);
      } catch (error) { await current.stop(); throw error; }
    }
    return current;
  };
  const respond = (res, status, value) => {
    res.writeHead(status, { ...browserHeaders, 'Content-Type': 'application/json' });
    res.end(JSON.stringify(value));
  };
  const server = createServer(async (req, res) => {
    try {
      const localOrigin = `http://127.0.0.1:${server.address().port}`;
      if (![new URL(localOrigin).host, origin && new URL(origin).host].includes(req.headers.host)) return respond(res, 421, { error: 'Host is not allowed.' });
      // The static shell is public; all conversation data still requires a token.
      // Restrict both Host (DNS rebinding) and Origin. No cookies or CORS.
      if (req.headers.origin && ![localOrigin, origin].includes(req.headers.origin)) return respond(res, 403, { error: 'Origin is not allowed.' });
      const asset = assets.get(req.url);
      if (asset && req.method === 'GET') {
        const body = await readFile(new URL(`./public/${asset[0]}`, import.meta.url));
        res.writeHead(200, { ...browserHeaders, 'Content-Type': asset[1] });
        return res.end(body);
      }
      const supplied = req.headers.authorization || '';
      const access = await authenticate(tokenFile, supplied);
      const authorized = async () => access !== null && access === await authenticate(tokenFile, supplied);
      if (!access) return respond(res, 401, { error: 'Authentication required.' });
      if (stopping) return respond(res, 503, { error: 'Companion is stopping.' });
      const url = new URL(req.url, localOrigin);
      const actionRoute = /^\/v1\/work\/(target-[a-f0-9]{32})\/actions$/.exec(url.pathname);
      const receiptRoute = /^\/v1\/work\/(target-[a-f0-9]{32})\/receipts\/([a-f0-9]{32})$/.exec(url.pathname);
      if (receiptRoute) {
        if (req.method !== 'GET' || [...url.searchParams.keys()].length !== 1 || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(url.searchParams.get('run_id')||'')) return respond(res,400,{error:'Use an exact receipt and run identity.'});
        const input={actor:inspectionActor(access),target:receiptRoute[1],id:receiptRoute[2],run_id:url.searchParams.get('run_id')};
        const value=await serial(async()=>{if(!await authorized())throw Object.assign(new Error('Access revoked.'),{code:'AUTH_REVOKED'});return controlWork('receipt',input);});
        if(!await authorized())return respond(res,401,{error:'Authentication required.'});
        return respond(res,200,validateControlReply(value,input.target,input.id));
      }
      if(actionRoute){
        if(req.method!=='POST'||url.search)return respond(res,405,{error:'Use a reviewed semantic action.'});
        if(req.headers['content-type']!=='application/json')return respond(res,415,{error:'Use application/json.'});
        const chunks=[];let size=0;for await(const chunk of req){size+=chunk.length;if(size>24576)return respond(res,413,{error:'Action exceeds 24 KiB.'});chunks.push(chunk);}
        const action=validateRemoteAction(JSON.parse(Buffer.concat(chunks).toString('utf8')),actionRoute[1]);
        const input={actor:inspectionActor(access),action};
        // Owner-authorized preflight at the gateway, then again after queueing.
        let first;
        try{if(!await authorized())return respond(res,401,{error:'Authentication required.'});first=await serial(async()=>{if(!await authorized())throw Object.assign(new Error('Access revoked.'),{code:'AUTH_REVOKED'});return validateControlReply(await controlWork('check',input),action.target.id,action.id);});}
        catch(error){if(error.code==='AUTH_REVOKED')return respond(res,401,{error:'Authentication required.'});return respond(res,409,{error:'Preflight rejected: authorization, scope, action identity or owner is unavailable. Retrieve any existing receipt before another attempt.'});}
        if(!await authorized())return respond(res,401,{error:'Authentication required.'});
        if(first.receipt)return respond(res,200,first);
        if(!first.authorized)return respond(res,403,{error:'Operation access is not authorized.',delivery:'not_dispatched'});
        const result=await serial(async()=>{
          if(!await authorized())throw Object.assign(new Error('Access revoked.'),{code:'AUTH_REVOKED'});
          const checked=validateControlReply(await controlWork('check',input),action.target.id,action.id);
          if(checked.receipt)return checked;
          if(!checked.authorized)throw new Error('Operation access is no longer authorized.');
          if(!await authorized())throw Object.assign(new Error('Access revoked.'),{code:'AUTH_REVOKED'});
          return validateControlReply(await controlWork('dispatch',input),action.target.id,action.id);
        });
        if(!await authorized())return respond(res,401,{error:'Authentication required. The action may already have been dispatched.'});
        return respond(res,200,result);
      }
      const external = /^\/v1\/external(?:\/(target-[a-f0-9]{32})(\/snapshot)?)?$/.exec(url.pathname);
      if(external){
        if(req.method!=='GET')return respond(res,405,{error:'External inspection is read-only.'});
        const target=external[1]||'',generation=url.searchParams.get('generation')||'',snapshot=!!external[2];
        if(target ? [...url.searchParams.keys()].length!==1||!/^[a-f0-9]{32}$/.test(generation) : url.search)throw new Error('Use the selected target generation.');
        const value=await serial(async()=>{
          if(stopping||!await authorized())throw Object.assign(new Error('Access revoked.'),{code:'AUTH_REVOKED'});
          const result=await queryExternal({actor:inspectionActor(access),mode:target?(snapshot?'snapshot':'metadata'):'list',target,generation});
          if(!await authorized())throw Object.assign(new Error('Access revoked.'),{code:'AUTH_REVOKED'});
          return validateExternal(result,target,generation,snapshot);
        });
        if(!await authorized())return respond(res,401,{error:'Authentication required.'});
        return respond(res,200,value);
      }
      const work = /^\/v1\/work(?:\/(target-[a-f0-9]{32})(?:\/activity)?)?$/.exec(url.pathname);
      if (work) {
        if (req.method !== 'GET') return respond(res, 405, { error: 'Work inspection is read-only.' });
        const target = work[1] || '', generation = url.searchParams.get('generation') || '';
        if (target ? [...url.searchParams.keys()].length !== 1 || !/^[a-f0-9]{32}$/.test(generation) : url.search) throw new Error('Use the selected target generation.');
        const value = await serial(async () => {
          if (stopping || !await authorized()) throw Object.assign(new Error('Access was revoked.'), { code: 'AUTH_REVOKED' });
          const result = await queryWork({ actor: inspectionActor(access), target, generation });
          if (!await authorized()) throw Object.assign(new Error('Access was revoked.'), { code: 'AUTH_REVOKED' });
          return validateInspection(result, target, generation);
        });
        if (!await authorized()) return respond(res, 401, { error: 'Authentication required.' });
        return respond(res, 200, value);
      }
      if (req.url === '/v1/health' && req.method === 'GET') {
        if (!await authorized()) return respond(res, 401, { error: 'Authentication required.' });
        return respond(res, 200, { protocol: 1, status: 'ready', uptime_seconds: Math.max(0, Math.floor((now() - startedAt) / 1000)), sessions: workers.size, stopped_sessions: [...workers.values()].filter(w => w.closed).length, pending_requests: pending });
      }
      if (url.pathname === '/v1/chats' && req.method === 'GET') {
        const params = url.searchParams;
        if ([...params.keys()].some(key => !['q', 'scope', 'after'].includes(key) || params.getAll(key).length !== 1)) throw new Error('Unknown or repeated catalog parameter.');
        const query = params.get('q') || '', scope = params.get('scope') || 'active', after = params.get('after') || '';
        if (Buffer.byteLength(query) > 512 || /[\x00-\x1f\x7f]/.test(query) || !['active', 'archived', 'all'].includes(scope) || after.length > 1024) throw new Error('Invalid conversation search.');
        const page = await serial(async () => {
          if (stopping || !await authorized()) throw Object.assign(new Error('Access was revoked.'), { code: 'AUTH_REVOKED' });
          return queryCatalog({ query, scope, after });
        });
        if (!await authorized()) return respond(res, 401, { error: 'Authentication required.' });
        return respond(res, 200, { ...page, new_chat_source: workspace });
      }
      if (req.url === '/v1/chats' && req.method === 'POST') {
        if (req.headers['content-type'] !== 'application/json') return respond(res, 415, { error: 'Use application/json.' });
        const chunks = []; let size = 0;
        for await (const chunk of req) {
          size += chunk.length;
          if (size > 4096) return respond(res, 413, { error: 'New-chat request exceeds 4 KiB.' });
          chunks.push(chunk);
        }
        const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
        if (!body || Object.keys(body).length !== 1 || typeof body.request_id !== 'string' || !creationIdentity.test(body.request_id)) throw new Error('Use a stable new-chat request ID, without source or message fields.');
        const created = await serial(async () => {
          if (stopping || !await authorized()) throw Object.assign(new Error('Access was revoked.'), { code: 'AUTH_REVOKED' });
          return createChat(body.request_id);
        });
        if (!await authorized()) return respond(res, 401, { error: 'Authentication required.' });
        if (created.request_id !== body.request_id || !identity.test(created.conversation_id)) throw new Error('The host did not confirm this chat identity. Retry the same request.');
        return respond(res, 200, created);
      }
      const match = /^\/v1\/chats\/([a-zA-Z0-9_-]+)(\/actions)?$/.exec(req.url);
      if (!match || !['GET', 'POST'].includes(req.method) || (req.method === 'POST') !== !!match[2]) return respond(res, 404, { error: 'Unknown operation.' });
      let frame;
      if (req.method === 'POST') {
        if (req.headers['content-type'] !== 'application/json') return respond(res, 415, { error: 'Use application/json.' });
        const chunks = []; let size = 0;
        for await (const chunk of req) {
          size += chunk.length;
          if (size > 256 * 1024) return respond(res, 413, { error: 'Request exceeds 256 KiB.' });
          chunks.push(chunk);
        }
        frame = JSON.parse(Buffer.concat(chunks).toString('utf8'));
        validateFrame(frame);
        if (!['upload_submit', 'approval', 'queue', 'update', 'receipt'].includes(frame.type)) throw new Error('This operation must be performed on the host.');
        if (frame.type === 'upload_submit' && (frame.upload.files.length || frame.upload.context !== `conversation:${match[1]}`)) throw new Error('Use this conversation with no remote attachments.');
        for (const value of [frame.update, frame.receipt]) if (value && value.conversation_id !== match[1]) throw new Error('Action belongs to a different conversation.');
      }
      const state = await serial(async () => {
        // Authorization may have been revoked while waiting for another action.
        if (!await authorized()) throw Object.assign(new Error('Access was revoked.'), { code: 'AUTH_REVOKED' });
        if (stopping) throw new Error('Companion is stopping.');
        const owner = await worker(match[1], !frame);
        if (!await authorized()) throw Object.assign(new Error('Access was revoked.'), { code: 'AUTH_REVOKED' });
        if (frame) {
          const admission = frame.type === 'upload_submit' ? JSON.stringify(frame) : undefined;
          if (owner.unconfirmedAction && (!admission || admission !== owner.unconfirmedAdmission)) throw new Error('The previous action is unconfirmed. Review it on the host, or retry the exact same message request.');
          if (owner.state?.external || owner.state?.panel || owner.state?.pending_connection) throw new Error('Finish this conversation’s host setup first.');
          if ((frame.type === 'upload_submit' || frame.type === 'queue' && frame.queue.action === 'run') && !owner.state.running && [...workers.values()].filter(w => !w.closed && w.state?.running).length >= 3) throw new Error('Three conversations are working.');
          owner.unconfirmedAction = true;
          owner.unconfirmedAdmission = admission;
          if (frame.type === 'upload_submit') await owner.submitUpload(frame);
          else await owner.apply(frame);
          owner.unconfirmedAction = false;
          owner.unconfirmedAdmission = undefined;
        }
        return { conversation_id: match[1], execution_host: 'companion', state: owner.state, disconnected: owner.closed, error: owner.closed ? owner.failure : undefined };
      });
      if (!await authorized()) return respond(res, 401, { error: 'Authentication required.' });
      respond(res, 200, state);
    } catch (error) {
      // Do not expose filesystem errors or authentication file details.
      if (error.code === 'AUTH_REVOKED') return respond(res, 401, { error: 'Authentication required.' });
      respond(res, error.code ? 503 : 400, { error: error.code ? 'Companion is unavailable.' : error.message });
    }
  });
  server.requestTimeout = 30000;
  server.headersTimeout = 10000;
  return {
    server,
    async listen(port = 0) {
      await readToken(tokenFile);
      await new Promise((resolve, reject) => { server.once('error', reject); server.listen(port, '127.0.0.1', resolve); });
      return server.address();
    },
    async close() {
      stopping = true;
      const closed = new Promise(resolve => server.close(resolve));
      server.closeIdleConnections();
      await sequence;
      await Promise.all([...workers.values()].map(w => w.stop()));
      await closed;
    },
  };
}
