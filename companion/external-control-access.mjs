// Trusted host-local issuance only; no remote counterpart.
import {execFileSync} from 'node:child_process';
import {resolve} from 'node:path';
import {inspectionDeviceActor} from './auth.mjs';
const [binary,tokenFile,target,device,operation,seconds='300',...extra]=process.argv.slice(2);
if(!binary||!tokenFile||!/^target-[a-f0-9]{32}$/.test(target||'')||!device||!['text_input','key_input','interrupt'].includes(operation)||!/^\d{1,3}$/.test(seconds)||+seconds>900||extra.length)throw new Error('Usage: node companion/external-control-access.mjs RUNTIME OWNER_TOKEN TARGET DEVICE text_input|key_input|interrupt [SECONDS: 0 revokes, max 900]');
await inspectionDeviceActor(tokenFile,device);
try{execFileSync(resolve(binary),['external-tmux','grant-control',target],{env:{...process.env,GATOR_INTERNAL_COMMANDS:'1'},input:JSON.stringify({authority:{token_file:resolve(tokenFile),device},operation,seconds:+seconds}),stdio:['pipe','ignore','pipe'],timeout:20000});}catch{throw new Error('Control grant refused; check local control review, device, target and generation.');}
console.log(+seconds?'Exact target operation access granted.':'Exact target operation access revoked. Already-dispatched input cannot be undone.');
