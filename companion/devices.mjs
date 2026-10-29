import { addDevice, listDevices, revokeDevice } from './auth.mjs';

const [tokenFile, operation, ...args] = process.argv.slice(2);
if (!tokenFile || !['add', 'list', 'revoke'].includes(operation)) throw new Error('Usage: node companion/devices.mjs OWNER_TOKEN_FILE add NAME NEW_TOKEN_FILE | list | revoke DEVICE_ID');
if (operation === 'add' && args.length === 2) {
  const device = await addDevice(tokenFile, args[0], args[1]);
  console.log(`Created device ${device.id} (${device.name}). Its token was saved to the requested private file. Transfer it privately to that device.`);
} else if (operation === 'list' && args.length === 0) {
  for (const device of await listDevices(tokenFile)) console.log(`${device.id}\t${device.status}\t${device.name}\t${device.created_at}`);
} else if (operation === 'revoke' && args.length === 1) {
  await revokeDevice(tokenFile, args[0]);
  console.log('Device access revoked. Already admitted work is retained; remove any copies of its token yourself.');
} else throw new Error('Use add NAME NEW_TOKEN_FILE, list, or revoke DEVICE_ID.');
