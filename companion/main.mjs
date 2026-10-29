import { resolve } from 'node:path';
import { createCompanion } from './server.mjs';

const [binary, workspace, tokenFile, port = '7341'] = process.argv.slice(2);
if (!binary || !workspace || !tokenFile || !/^\d+$/.test(port) || Number(port) > 65535) {
  throw new Error('Usage: node companion/main.mjs RUNTIME WORKSPACE TOKEN_FILE [PORT]');
}
const companion = createCompanion({ binary: resolve(binary), workspace: resolve(workspace), tokenFile: resolve(tokenFile), env: process.env, origin: process.env.GATOR_COMPANION_ORIGIN });
const address = await companion.listen(Number(port));
console.log(`Gator companion listening on 127.0.0.1:${address.port}. Client disconnects leave work running.`);
let closing;
for (const signal of ['SIGTERM', 'SIGINT']) process.once(signal, () => { closing ??= companion.close().catch(error => { console.error(error.message); process.exitCode = 1; }); });
