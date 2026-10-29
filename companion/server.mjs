import { createServer } from 'node:http';
import { createHash, timingSafeEqual } from 'node:crypto';
import { open, readFile } from 'node:fs/promises';
import { constants } from 'node:fs';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { Backend } from '../app-host/src/backend.mjs';
import { validateFrame } from '../app-host/src/policy.mjs';

const identity = /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/;
const hash = text => createHash('sha256').update(text).digest();
const execute = promisify(execFile);
const idle = worker => !worker.closed && !['running', 'queued', 'pending_approval', 'pending_connection', 'external', 'panel', 'receipt_busy', 'catalog_busy'].some(key => worker.state?.[key]) && !worker.state?.queue?.length;
const assets = new Map([
  ['/', ['index.html', 'text/html; charset=utf-8']],
  ['/client.mjs', ['client.mjs', 'text/javascript; charset=utf-8']],
  ['/styles.css', ['styles.css', 'text/css; charset=utf-8']],
]);
const browserHeaders = {
  'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'no-referrer', 'X-Frame-Options': 'DENY',
  'Content-Security-Policy': "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
};

// Re-read each time: replacing the token file revokes every old client, without
// cancelling admitted work. Only a private regular file can authorize access.
export async function readToken(path) {
  const file = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW || 0) | (constants.O_NONBLOCK || 0));
  try {
    const stat = await file.stat();
    if (!stat.isFile() || stat.size !== 65 || process.platform !== 'win32' && (stat.mode & 0o077)) throw new Error('Use a private token file.');
    const token = (await file.readFile('utf8')).trim();
    if (!/^[a-f0-9]{64}$/.test(token)) throw new Error('Invalid companion token.');
    return token;
  } finally { await file.close(); }
}

export function createCompanion({ binary, workspace, env, tokenFile, origin,
  queryCatalog = async ({ query, scope, after }) => {
    const { stdout } = await execute(binary, ['companion-catalog', query, scope, after], { cwd: workspace, env: { ...env, GATOR_COMPANION_CATALOG: '1' }, timeout: 20000, maxBuffer: 1024 * 1024, windowsHide: true });
    return JSON.parse(stdout);
  },
  spawnBackend = (id) => new Backend(binary, workspace, { ...env, GATOR_UI_SESSION_HOST: '1', GATOR_UI_FRONTEND: 'companion' }, ['ui-backend', id, '']) }) {
  if (origin && (new URL(origin).origin !== origin || new URL(origin).protocol !== 'https:')) throw new Error('The companion browser origin must be an exact HTTPS origin.');
  const workers = new Map();
  let sequence = Promise.resolve(), stopping = false, pending = 0;
  const serial = task => {
    if (pending >= 32) throw new Error('Companion request queue is full.');
    pending++;
    const next = sequence.then(task).finally(() => pending--);
    sequence = next.catch(() => {});
    return next;
  };
  const worker = async id => {
    if (!identity.test(id)) throw new Error('Invalid conversation identity.');
    let current = workers.get(id);
    if (current?.closed) throw new Error('The host process stopped. Review this conversation on the host before reopening it.');
    if (!current) {
      if (workers.size >= 4) {
        const available = [...workers].find(([, value]) => idle(value));
        if (!available) throw new Error('Four conversations have pending work. Finish a task or review its queue on the host before opening another.');
        await available[1].stop();
        workers.delete(available[0]);
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
      const token = await readToken(tokenFile);
      if (!timingSafeEqual(hash(supplied), hash(`Bearer ${token}`))) return respond(res, 401, { error: 'Authentication required.' });
      if (stopping) return respond(res, 503, { error: 'Companion is stopping.' });
      const url = new URL(req.url, localOrigin);
      if (url.pathname === '/v1/chats' && req.method === 'GET') {
        const params = url.searchParams;
        if ([...params.keys()].some(key => !['q', 'scope', 'after'].includes(key) || params.getAll(key).length !== 1)) throw new Error('Unknown or repeated catalog parameter.');
        const query = params.get('q') || '', scope = params.get('scope') || 'active', after = params.get('after') || '';
        if (Buffer.byteLength(query) > 512 || /[\x00-\x1f\x7f]/.test(query) || !['active', 'archived', 'all'].includes(scope) || after.length > 1024) throw new Error('Invalid conversation search.');
        const page = await serial(async () => {
          if (stopping || token !== await readToken(tokenFile)) throw new Error('Access was revoked.');
          return queryCatalog({ query, scope, after });
        });
        if (token !== await readToken(tokenFile)) return respond(res, 401, { error: 'Authentication required.' });
        return respond(res, 200, page);
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
        if (token !== await readToken(tokenFile)) throw new Error('Access was revoked.');
        if (stopping) throw new Error('Companion is stopping.');
        const owner = await worker(match[1]);
        if (frame) {
          if (owner.state?.external || owner.state?.panel || owner.state?.pending_connection) throw new Error('Finish this conversation’s host setup first.');
          if ((frame.type === 'upload_submit' || frame.type === 'queue' && frame.queue.action === 'run') && !owner.state.running && [...workers.values()].filter(w => !w.closed && w.state?.running).length >= 3) throw new Error('Three conversations are working.');
          if (frame.type === 'upload_submit') await owner.submitUpload(frame);
          else await owner.apply(frame);
        }
        return { conversation_id: match[1], execution_host: 'companion', state: owner.state, disconnected: owner.closed, error: owner.closed ? owner.failure : undefined };
      });
      if (token !== await readToken(tokenFile)) return respond(res, 401, { error: 'Authentication required.' });
      respond(res, 200, state);
    } catch (error) {
      // Do not expose filesystem errors or authentication file details.
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
