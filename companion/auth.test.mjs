import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, readFile, rm, chmod, symlink, readdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { authenticate, addDevice, listDevices, revokeDevice } from './auth.mjs';
import { createCompanion } from './server.mjs';

const execute = promisify(execFile);
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'gator-devices-')), ownerFile = join(root, 'owner.token'), owner = 'a'.repeat(64);
  await writeFile(ownerFile, owner + '\n', { mode: 0o600 });
  return { root, ownerFile, owner, close: () => rm(root, { recursive: true, force: true }) };
}
const deferred = () => { let resolve; const promise = new Promise(done => resolve = done); return { promise, resolve }; };

test('device enrollment, individual revocation and owner rotation retain distinct access identities', async () => {
  const f = await fixture();
  try {
    const phone = await addDevice(f.ownerFile, 'Phone', join(f.root, 'phone.token'));
    const tablet = await addDevice(f.ownerFile, 'Tablet', join(f.root, 'tablet.token'));
    const phoneToken = (await readFile(join(f.root, 'phone.token'), 'utf8')).trim(), tabletToken = (await readFile(join(f.root, 'tablet.token'), 'utf8')).trim();
    const registry = await readFile(join(f.ownerFile + '.devices', phone.id + '.json'), 'utf8');
    assert.equal(registry.includes(phoneToken.split('_').at(-1)), false);
    assert.equal(registry.includes(f.owner), false);
    const authorized = value => authenticate(f.ownerFile, `Bearer ${value}`);
    assert.match(await authorized(f.owner), /^owner:/);
    assert.match(await authorized(phoneToken), new RegExp(`^device:${phone.id}:`));
    assert.match(await authorized(tabletToken), new RegExp(`^device:${tablet.id}:`));
    assert.equal(await authorized(phoneToken.slice(0, -1) + (phoneToken.endsWith('0') ? '1' : '0')), null);
    assert.equal(await authorized('gator_device_../escape_' + 'a'.repeat(64)), null);
    assert.equal((await listDevices(f.ownerFile)).length, 2);
    await revokeDevice(f.ownerFile, phone.id);
    assert.equal(await authorized(phoneToken), null);
    assert.ok(await authorized(tabletToken)); assert.ok(await authorized(f.owner));
    await execute(process.execPath, [resolve('companion/token.mjs'), f.ownerFile, '--rotate']);
    assert.equal(await authorized(f.owner), null); assert.equal(await authorized(tabletToken), null);
    assert.equal((await listDevices(f.ownerFile))[0].status, 'revoked by owner rotation');
    const replacement = await addDevice(f.ownerFile, 'New phone', join(f.root, 'replacement.token'));
    assert.ok(await authorized((await readFile(join(f.root, 'replacement.token'), 'utf8')).trim()));
    assert.equal((await listDevices(f.ownerFile)).find(device => device.id === replacement.id).status, 'active');
  } finally { await f.close(); }
});

test('concurrent host CLI enrollment never prints secrets or overwrites an exported token', async () => {
  const f = await fixture();
  try {
    const enroll = index => execute(process.execPath, [resolve('companion/devices.mjs'), f.ownerFile, 'add', `Device ${index}`, join(f.root, `${index}.token`)]);
    const results = await Promise.all([enroll(1), enroll(2), enroll(3)]);
    for (let index = 1; index <= 3; index++) {
      const value = (await readFile(join(f.root, `${index}.token`), 'utf8')).trim();
      assert.ok(await authenticate(f.ownerFile, `Bearer ${value}`));
      assert.equal(results[index - 1].stdout.includes(value), false);
      assert.equal(results[index - 1].stdout.includes(value.split('_').at(-1)), false);
    }
    const before = await readFile(join(f.root, '1.token'), 'utf8');
    await assert.rejects(enroll(1));
    assert.equal(await readFile(join(f.root, '1.token'), 'utf8'), before);
    assert.equal((await listDevices(f.ownerFile)).length, 3);
    await assert.rejects(revokeDevice(f.ownerFile, '../owner.token'));
    assert.equal(await readFile(f.ownerFile, 'utf8'), f.owner + '\n');
  } finally { await f.close(); }
});

test('device access rejects public or linked storage and malformed records', { skip: process.platform === 'win32' }, async () => {
  const f = await fixture();
  try {
    const device = await addDevice(f.ownerFile, 'Phone', join(f.root, 'phone.token'));
    const token = (await readFile(join(f.root, 'phone.token'), 'utf8')).trim(), path = join(f.ownerFile + '.devices', device.id + '.json');
    const authorized = () => authenticate(f.ownerFile, `Bearer ${token}`);
    await chmod(path, 0o644); await assert.rejects(authorized(), { code: 'AUTH_STORAGE' });
    await chmod(path, 0o600); await chmod(f.ownerFile + '.devices', 0o755);
    await assert.rejects(authorized(), { code: 'AUTH_STORAGE' });
    await chmod(f.ownerFile + '.devices', 0o700);
    const saved = await readFile(path, 'utf8');
    await writeFile(path, '{}'); await assert.rejects(authorized(), { code: 'AUTH_STORAGE' });
    await writeFile(path, saved); await rm(path); await symlink(f.ownerFile, path);
    await assert.rejects(authorized(), { code: 'AUTH_STORAGE' });
    assert.equal((await readdir(f.ownerFile + '.devices')).length, 1);
  } finally { await f.close(); }
});

test('revoking one device blocks a pending action and private response while other clients retain access', { timeout: 15000 }, async () => {
  const f = await fixture(), readyStarted = deferred(), readyGate = deferred(), catalogStarted = deferred(), catalogGate = deferred();
  let companion, submitted = 0, blockedCatalog = false;
  try {
    const phone = await addDevice(f.ownerFile, 'Phone', join(f.root, 'phone.token'));
    await addDevice(f.ownerFile, 'Tablet', join(f.root, 'tablet.token'));
    const phoneToken = (await readFile(join(f.root, 'phone.token'), 'utf8')).trim(), tabletToken = (await readFile(join(f.root, 'tablet.token'), 'utf8')).trim();
    const options = { tokenFile: f.ownerFile,
      queryCatalog: async () => { if (blockedCatalog) { catalogStarted.resolve(); await catalogGate.promise; } return { items: [{ id: 'work-test', title: 'Private fixture' }] }; },
      spawnBackend: id => ({ state: { conversation_id: id, running: false }, ready: async () => { readyStarted.resolve(); await readyGate.promise; }, stop: async () => {}, submitUpload: async () => { submitted++; }, apply: async () => { submitted++; } }),
    };
    companion = createCompanion(options); let address = await companion.listen();
    const request = (token, path, frame) => fetch(`http://127.0.0.1:${address.port}${path}`, { method: frame ? 'POST' : 'GET', headers: { authorization: `Bearer ${token}`, ...(frame ? { 'content-type': 'application/json' } : {}) }, body: frame ? JSON.stringify(frame) : undefined });
    const action = request(phoneToken, '/v1/chats/work-test/actions', { type: 'upload_submit', upload: { request_id: 'stable-device-request', context: 'conversation:work-test', text: 'Do this once', files: [] } });
    await readyStarted.promise; await revokeDevice(f.ownerFile, phone.id); readyGate.resolve();
    assert.equal((await action).status, 401); assert.equal(submitted, 0);
    assert.equal((await request(tabletToken, '/v1/chats/work-test')).status, 200);
    assert.equal((await request(phoneToken, '/v1/chats/work-test')).status, 401);
    const second = await addDevice(f.ownerFile, 'Second phone', join(f.root, 'second.token'));
    const secondToken = (await readFile(join(f.root, 'second.token'), 'utf8')).trim();
    blockedCatalog = true;
    const reading = request(secondToken, '/v1/chats');
    await catalogStarted.promise; await revokeDevice(f.ownerFile, second.id); catalogGate.resolve();
    const revoked = await reading; assert.equal(revoked.status, 401);
    assert.equal((await revoked.text()).includes('Private fixture'), false);
    await companion.close(); companion = createCompanion(options); address = await companion.listen();
    assert.equal((await request(tabletToken, '/v1/chats')).status, 200);
    assert.equal((await request(secondToken, '/v1/chats')).status, 401);
    assert.equal(submitted, 0);
  } finally { readyGate.resolve(); catalogGate.resolve(); await companion?.close(); await f.close(); }
});
