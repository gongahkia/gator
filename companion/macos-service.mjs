import { access, readFile, stat, lstat, writeFile } from 'node:fs/promises';
import { constants } from 'node:fs';
import { dirname, isAbsolute, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readToken } from './server.mjs';

const escape = value => String(value).replace(/[&<>"']/g, value => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' }[value]));
const text = value => `<string>${escape(value)}</string>`;
const safePath = value => typeof value === 'string' && isAbsolute(value) && !/[\x00-\x1f\x7f]/.test(value);

// Generates a reviewable plan only. Activation is an explicit launchctl step.
export function launchAgent(config) {
  const keys = ['node', 'entry', 'runtime', 'workspace', 'tokenFile', 'configDir', 'stateDir', 'logFile', 'port', 'origin', 'label'];
  if (!config || Object.keys(config).some(key => !keys.includes(key))) throw new Error('Unknown companion service setting.');
  for (const key of keys.slice(0, 8)) if (!safePath(config[key])) throw new Error(`${key} must be an absolute path without control characters.`);
  const label = config.label || 'com.gator.companion';
  if (!/^com\.gator\.companion(?:[.-][a-zA-Z0-9-]+)*$/.test(label)) throw new Error('Use a com.gator.companion service label.');
  if (!Number.isInteger(config.port) || config.port < 1 || config.port > 65535) throw new Error('Choose a fixed port from 1 to 65535.');
  if (config.origin && (new URL(config.origin).origin !== config.origin || new URL(config.origin).protocol !== 'https:')) throw new Error('Use an exact HTTPS origin, with no trailing slash.');
  const args = [config.node, config.entry, config.runtime, config.workspace, config.tokenFile, String(config.port)];
  const environment = { GATOR_CONFIG_DIR: config.configDir, GATOR_STATE_DIR: config.stateDir,
    PATH: `${dirname(config.node)}:/usr/bin:/bin:/usr/sbin:/sbin`, ...(config.origin ? { GATOR_COMPANION_ORIGIN: config.origin } : {}) };
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key>${text(label)}
<key>ProgramArguments</key><array>${args.map(text).join('')}</array>
<key>WorkingDirectory</key>${text(config.workspace)}
<key>EnvironmentVariables</key><dict>${Object.entries(environment).map(([key, value]) => `<key>${key}</key>${text(value)}`).join('')}</dict>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
<key>ThrottleInterval</key><integer>10</integer>
<key>ExitTimeOut</key><integer>30</integer>
<key>Umask</key><integer>63</integer>
<key>ProcessType</key><string>Background</string>
<key>StandardOutPath</key>${text(config.logFile)}
<key>StandardErrorPath</key>${text(config.logFile)}
</dict></plist>\n`;
}

export async function prepare(config, output) {
  const plist = launchAgent(config);
  for (const key of ['node', 'runtime']) {
    if (!(await stat(config[key])).isFile()) throw new Error(`${key} is not a file.`);
    await access(config[key], constants.X_OK);
  }
  if (!(await stat(config.entry)).isFile()) throw new Error('entry is not a file.');
  for (const key of ['workspace', 'configDir', 'stateDir']) if (!(await stat(config[key])).isDirectory()) throw new Error(`${key} is not a directory.`);
  const logs = await stat(dirname(config.logFile));
  if (!logs.isDirectory() || logs.mode & 0o077) throw new Error('Use an existing private log directory (mode 0700).');
  const log = await lstat(config.logFile).catch(error => { if (error.code === 'ENOENT') return null; throw error; });
  if (log && (!log.isFile() || log.mode & 0o077)) throw new Error('An existing log must be a private regular file.');
  await readToken(config.tokenFile);
  await writeFile(output, plist, { mode: 0o600, flag: 'wx' });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [configFile, output, ...extra] = process.argv.slice(2);
  if (!configFile || !output || extra.length) throw new Error('Usage: node companion/macos-service.mjs CONFIG_JSON NEW_PLIST');
  await prepare(JSON.parse(await readFile(configFile, 'utf8')), resolve(output));
  console.log('LaunchAgent file prepared. Review it before installation; no service was started.');
}
