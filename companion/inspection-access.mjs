// Explicit host-local grant/revoke. This command is never exposed through HTTP.
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { inspectionDeviceActor } from './auth.mjs';

const [binary, tokenFile, run, device, seconds = '300', ...extra] = process.argv.slice(2);
if (!binary || !tokenFile || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(run || '') || !device || !/^\d{1,3}$/.test(seconds) || Number(seconds) > 900 || extra.length) throw new Error('Usage: node companion/inspection-access.mjs RUNTIME OWNER_TOKEN_FILE RUN_ID DEVICE_ID_OR_owner [SECONDS: 0 revokes, 1–900 grants]');
const actor = await inspectionDeviceActor(tokenFile, device);
execFileSync(resolve(binary), ['inspection-access', run, actor, seconds], { env: { ...process.env, GATOR_INTERNAL_COMMANDS: '1' }, stdio: ['ignore', 'ignore', 'pipe'], timeout: 10000 });
console.log(Number(seconds) ? 'Inspect access granted for this live Work owner only.' : 'Inspect access revoked for this live Work owner.');
