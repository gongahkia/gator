// Host-local only. Enrollment and a separate device Inspect grant are required.
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { inspectionDeviceActor } from './auth.mjs';
const [binary,tokenFile,target,device,seconds='300',...extra]=process.argv.slice(2);
if(!binary||!tokenFile||!/^target-[a-f0-9]{32}$/.test(target||'')||!device||!/^\d{1,3}$/.test(seconds)||Number(seconds)>900||extra.length)throw new Error('Usage: node companion/external-access.mjs RUNTIME OWNER_TOKEN_FILE TARGET DEVICE_ID_OR_owner [SECONDS: 0 revokes, 1–900 grants]');
await inspectionDeviceActor(tokenFile,device);
execFileSync(resolve(binary),['external-tmux','grant',target],{env:{...process.env,GATOR_INTERNAL_COMMANDS:'1'},input:JSON.stringify({authority:{token_file:resolve(tokenFile),device},seconds:Number(seconds)}),stdio:['pipe','ignore','pipe'],timeout:20000});
console.log(Number(seconds)?'External Inspect access granted for this device and pane only.':'External Inspect access revoked.');
