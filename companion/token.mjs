import { randomBytes } from 'node:crypto';
import { writeFile, rename, rm, lstat } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { readToken } from './server.mjs';

const [file, operation] = process.argv.slice(2);
if (!file || operation && operation !== '--rotate') throw new Error('Usage: node companion/token.mjs TOKEN_FILE [--rotate]');
const path = resolve(file), parent = await lstat(dirname(path));
if (!parent.isDirectory() || parent.isSymbolicLink() || process.platform !== 'win32' && (parent.mode & 0o077)) throw new Error('Create a private directory (mode 0700) for the token first.');
const value = randomBytes(32).toString('hex') + '\n';
if (operation === '--rotate') {
  await readToken(path);
  const temporary = `${path}.${randomBytes(12).toString('hex')}.tmp`;
  try { await writeFile(temporary, value, { mode: 0o600, flag: 'wx' }); await rename(temporary, path); }
  finally { await rm(temporary, { force: true }); }
} else await writeFile(path, value, { mode: 0o600, flag: 'wx' });
console.log(operation ? 'Companion token rotated. Previous clients no longer have access.' : 'Companion token created. Transfer it privately to the client.');
