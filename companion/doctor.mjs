import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readToken } from './auth.mjs';

// Probe the authenticated listener without opening a session or invoking a model.
export async function checkListener(baseURL, tokenFile) {
  const url = new URL(baseURL);
  if (url.username || url.password || url.search || url.hash || url.pathname !== '/' ||
      !(url.protocol === 'https:' || url.protocol === 'http:' && url.hostname === '127.0.0.1')) {
    throw new Error('Use an HTTPS origin or the loopback HTTP origin, without credentials or a path.');
  }
  const token = await readToken(tokenFile);
  let response;
  try {
    response = await fetch(new URL('/v1/health', url), {
      headers: { authorization: `Bearer ${token}` }, redirect: 'error', cache: 'no-store',
      signal: AbortSignal.timeout(5000),
    });
  } catch { throw new Error('Listener unreachable or health request timed out.'); }
  if (response.status === 401) throw new Error('Listener rejected access. Check the host token generation.');
  if (!response.ok) throw new Error(`Listener health returned HTTP ${response.status}.`);
  const health = await response.json();
  if (health.protocol !== 1 || health.status !== 'ready' || !Number.isInteger(health.stopped_sessions)) throw new Error('Unrecognized listener health response.');
  return health;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [origin, tokenFile, ...extra] = process.argv.slice(2);
  try {
    if (!origin || !tokenFile || extra.length) throw new Error('Usage: node companion/doctor.mjs ORIGIN OWNER_TOKEN_FILE');
    const health = await checkListener(origin, tokenFile);
    console.log(JSON.stringify(health, null, 2));
    console.log('Listener reachable. This does not verify provider credentials or the separate job supervisor.');
    if (health.stopped_sessions > 0) process.exitCode = 2;
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
