// Host-local grants only. Neither this helper nor its paths are exposed by HTTP.
import { spawnSync } from 'node:child_process';
import { resolve } from 'node:path';
import { inspectionDeviceActor } from './auth.mjs';
const [binary, tokenFile, run, device, operation, scope = '', seconds = '300', ...extra] = process.argv.slice(2);
if (!binary || !tokenFile || !run || !device || !['cancel','steer','answer','decision'].includes(operation) || !/^\d{1,3}$/.test(seconds) || +seconds>900 || extra.length) throw new Error('Usage: node companion/control-access.mjs RUNTIME OWNER_TOKEN RUN DEVICE cancel|steer|answer|decision SCOPE SECONDS (0 revokes; max 900). Cancel scope is empty; steer/answer scope is a specialist ID; decision scope is an interaction ID.');
const actor = await inspectionDeviceActor(tokenFile, device);
const result = spawnSync(resolve(binary), ['control-access', run], { env: { ...process.env, GATOR_INTERNAL_COMMANDS:'1' }, input: JSON.stringify({ actor, operation, scope, seconds:+seconds, authority:{ token_file:resolve(tokenFile), device } }), encoding:'utf8', timeout:15000, maxBuffer:16384 });
if (result.status!==0 || result.error) throw new Error('The live owner refused the local control grant. Check the run, scope and current device.');
console.log(+seconds ? 'Operation access granted for this device, live owner and exact scope.' : 'Matching operation access revoked. Dispatched actions cannot be undone.');
