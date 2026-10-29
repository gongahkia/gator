import { access, readFile, stat, lstat, writeFile } from 'node:fs/promises';
import { constants } from 'node:fs';
import { dirname, posix, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readToken } from './server.mjs';

// systemd expands percent specifiers even inside quotes. ExecStart's colon
// prefix separately disables environment substitution (including ${NAME}).
const quote = value => `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/%/g, '%%')}"`;
const paths = ['node', 'entry', 'runtime', 'workspace', 'tokenFile', 'configDir', 'stateDir'];
export function userService(config) {
  if (!config || Array.isArray(config) || typeof config !== 'object' || Object.keys(config).some(key => ![...paths, 'port', 'origin'].includes(key))) throw new Error('Unknown companion service setting.');
  for (const key of paths) {
    const value = config[key];
    if (typeof value !== 'string' || !posix.isAbsolute(value) || /[\x00-\x1f\x7f]/.test(value) || value.split('/').includes('..')) throw new Error(`${key} must be an absolute Linux path without control characters or parent traversal.`);
  }
  if (posix.dirname(config.node).includes(':')) throw new Error('The Node directory must not contain a PATH separator.');
  if (!Number.isInteger(config.port) || config.port < 1 || config.port > 65535) throw new Error('Choose a fixed port from 1 to 65535.');
  if (config.origin !== undefined && (typeof config.origin !== 'string' || new URL(config.origin).origin !== config.origin || new URL(config.origin).protocol !== 'https:')) throw new Error('Use an exact HTTPS origin, with no trailing slash.');
  const args = [config.node, config.entry, config.runtime, config.workspace, config.tokenFile, config.port];
  const environment = {
    GATOR_CONFIG_DIR: config.configDir, GATOR_STATE_DIR: config.stateDir,
    GATOR_COMPANION_ORIGIN: config.origin || '',
    PATH: `${posix.dirname(config.node)}:/usr/local/bin:/usr/bin:/bin`,
  };
  // WorkingDirectory is a raw path, unlike ExecStart/Environment. A final slash
  // keeps trailing spaces or a literal backslash from becoming unit syntax.
  const workingDirectory = config.workspace.replace(/%/g, '%%') + '/';
  return `[Unit]
Description=Gator companion listener

[Service]
Type=exec
ExecStart=:${args.map(quote).join(' ')}
WorkingDirectory=${workingDirectory}
${Object.entries(environment).map(([key, value]) => `Environment=${quote(`${key}=${value}`)}`).join('\n')}
UMask=0077
Restart=always
RestartSec=10
TimeoutStopSec=30
KillMode=mixed
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
`;
}

// Preparation never invokes systemctl or changes login/boot policy.
export async function prepare(config, output) {
  const unit = userService(config);
  for (const key of ['node', 'runtime']) {
    if (!(await stat(config[key])).isFile()) throw new Error(`${key} is not a file.`);
    await access(config[key], constants.X_OK);
  }
  if (!(await stat(config.entry)).isFile()) throw new Error('entry is not a file.');
  for (const key of ['workspace', 'configDir', 'stateDir']) if (!(await stat(config[key])).isDirectory()) throw new Error(`${key} is not a directory.`);
  const parent = await lstat(dirname(config.tokenFile));
  if (!parent.isDirectory() || parent.mode & 0o077) throw new Error('Use a private token directory (mode 0700).');
  await readToken(config.tokenFile);
  await writeFile(output, unit, { mode: 0o600, flag: 'wx' });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [configFile, output, ...extra] = process.argv.slice(2);
  if (!configFile || !output || extra.length) throw new Error('Usage: node companion/linux-service.mjs CONFIG_JSON NEW_SERVICE');
  await prepare(JSON.parse(await readFile(configFile, 'utf8')), resolve(output));
  console.log('User service file prepared. Review it before installation; no service or boot policy was changed.');
}
