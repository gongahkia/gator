import { createHash, timingSafeEqual, randomBytes } from 'node:crypto';
import { open, lstat, mkdir, readdir, writeFile, unlink, link, realpath } from 'node:fs/promises';
import { constants } from 'node:fs';
import { dirname, join, resolve } from 'node:path';

const digest = value => createHash('sha256').update(value).digest('hex');
const same = (a, b) => timingSafeEqual(Buffer.from(digest(a)), Buffer.from(digest(b)));
const deviceID = /^[a-f0-9]{32}$/;
const hex = /^[a-f0-9]{64}$/;
const storageError = () => Object.assign(new Error('Companion access storage is unavailable.'), { code: 'AUTH_STORAGE' });

async function privateFile(path, maxSize) {
  const file = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW || 0) | (constants.O_NONBLOCK || 0));
  try {
    const info = await file.stat();
    if (!info.isFile() || info.size > maxSize || process.platform !== 'win32' && info.mode & 0o077) throw storageError();
    const buffer = Buffer.alloc(maxSize + 1);
    const { bytesRead } = await file.read(buffer, 0, buffer.length, 0);
    if (bytesRead > maxSize) throw storageError();
    return buffer.subarray(0, bytesRead).toString('utf8');
  } finally { await file.close(); }
}

export async function readToken(path) {
  let value;
  try { value = await privateFile(path, 65); }
  catch (error) { if (error.code === 'AUTH_STORAGE') throw Object.assign(new Error('Use a private token file.'), { code: 'AUTH_STORAGE' }); throw error; }
  if (Buffer.byteLength(value) !== 65 || !hex.test(value.trim())) throw storageError();
  return value.trim();
}

async function privateDirectory(path) {
  const info = await lstat(path);
  if (!info.isDirectory() || process.platform !== 'win32' && info.mode & 0o077) throw storageError();
}

const directory = tokenFile => `${resolve(tokenFile)}.devices`;
async function readDevice(tokenFile, id) {
  await privateDirectory(directory(tokenFile));
  const value = JSON.parse(await privateFile(join(directory(tokenFile), `${id}.json`), 4096));
  if (!value || value.version !== 1 || value.id !== id || !hex.test(value.secret_hash) || !hex.test(value.owner_hash) || typeof value.name !== 'string' || !value.name.trim() || value.name.length > 80 || /[\x00-\x1f\x7f]/.test(value.name) || typeof value.created_at !== 'string' || !Number.isFinite(Date.parse(value.created_at))) throw storageError();
  return value;
}

// A returned identity is tied to both a device and the current owner-token
// generation. Every dispatch and response must revalidate it; no access cache.
export async function authenticate(tokenFile, authorization) {
  const owner = await readToken(tokenFile), generation = digest(owner);
  if (same(authorization, `Bearer ${owner}`)) return `owner:${generation}`;
  const match = /^Bearer gator_device_([a-f0-9]{32})_([a-f0-9]{64})$/.exec(authorization);
  if (!match) return null;
  let device;
  try { device = await readDevice(tokenFile, match[1]); }
  catch (error) { if (error.code === 'ENOENT') return null; throw storageError(); }
  if (!same(device.owner_hash, generation) || !same(device.secret_hash, digest(match[2]))) return null;
  return `device:${device.id}:${generation}:${device.secret_hash}`;
}

export async function addDevice(tokenFile, name, output) {
  if (typeof name !== 'string' || !name.trim() || name.length > 80 || /[\x00-\x1f\x7f]/.test(name)) throw new Error('Use a device name of 1–80 characters without control characters.');
  tokenFile = resolve(tokenFile); output = resolve(output);
  await privateDirectory(dirname(tokenFile)); await privateDirectory(dirname(output));
  const owner = await readToken(tokenFile), root = directory(tokenFile);
  await mkdir(root, { mode: 0o700 }).catch(error => { if (error.code !== 'EEXIST') throw error; });
  await privateDirectory(root);
  if (await realpath(dirname(output)) === await realpath(root)) throw new Error('Save the device token outside the access registry.');
  const id = randomBytes(16).toString('hex'), secret = randomBytes(32).toString('hex');
  const device = { version: 1, id, name: name.trim(), created_at: new Date().toISOString(), owner_hash: digest(owner), secret_hash: digest(secret) };
  // Publish the private export first. A crash before registry publication leaves
  // an unusable token, never a valid device whose secret was lost on stdout.
  await writeFile(output, `gator_device_${id}_${secret}\n`, { flag: 'wx', mode: 0o600 });
  const temporary = join(root, `.enroll-${id}`);
  try {
    await writeFile(temporary, JSON.stringify(device) + '\n', { flag: 'wx', mode: 0o600 });
    if (owner !== await readToken(tokenFile)) throw new Error('Owner access changed. Enroll this device again.');
    // Link publishes a complete record without replacing another enrollment.
    await link(temporary, join(root, `${id}.json`));
  } catch (error) { await unlink(output); throw error; }
  finally { await unlink(temporary).catch(error => { if (error.code !== 'ENOENT') throw error; }); }
  return { id, name: device.name };
}

export async function listDevices(tokenFile) {
  const ownerHash = digest(await readToken(tokenFile));
  let names;
  try { await privateDirectory(directory(tokenFile)); names = await readdir(directory(tokenFile)); }
  catch (error) { if (error.code === 'ENOENT') return []; throw error; }
  const devices = [];
  for (const name of names.sort()) {
    if (/^\.enroll-[a-f0-9]{32}$/.test(name)) continue;
    if (!/^[a-f0-9]{32}\.json$/.test(name)) throw storageError();
    let value;
    try { value = await readDevice(tokenFile, name.slice(0, -5)); }
    catch (error) { if (error.code === 'ENOENT') continue; throw error; }
    devices.push({ id: value.id, name: value.name, created_at: value.created_at, status: same(value.owner_hash, ownerHash) ? 'active' : 'revoked by owner rotation' });
  }
  return devices;
}

export async function revokeDevice(tokenFile, id) {
  if (!deviceID.test(id)) throw new Error('Use the exact device ID from the device list.');
  await readToken(tokenFile); await readDevice(tokenFile, id);
  await unlink(join(directory(tokenFile), `${id}.json`));
}
