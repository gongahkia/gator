import { createServer } from 'node:http';
import { createHash, timingSafeEqual } from 'node:crypto';
import { open } from 'node:fs/promises';
import { constants } from 'node:fs';
import { Backend } from '../app-host/src/backend.mjs';
import { validateFrame } from '../app-host/src/policy.mjs';

const identity = /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/;
const hash = text => createHash('sha256').update(text).digest();

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

export function createCompanion({ binary, workspace, env, tokenFile, origin, spawnBackend = (id) => new Backend(binary, workspace, { ...env, GATOR_UI_SESSION_HOST: '1', GATOR_UI_FRONTEND: 'companion' }, ['ui-backend', id, '']) }) {
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
      if (workers.size >= 4) throw new Error('Four conversations are open on the companion.');
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
    res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff' });
    res.end(JSON.stringify(value));
  };
  const server = createServer(async (req, res) => {
    try {
      // No ambient cookie authentication or permissive CORS. A browser must be
      // served from the configured origin; CLI/SSH clients may omit Origin.
      if (req.headers.origin && req.headers.origin !== origin) return respond(res, 403, { error: 'Origin is not allowed.' });
      const supplied = req.headers.authorization || '';
      const token = await readToken(tokenFile);
      if (!timingSafeEqual(hash(supplied), hash(`Bearer ${token}`))) return respond(res, 401, { error: 'Authentication required.' });
      if (stopping) return respond(res, 503, { error: 'Companion is stopping.' });
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
