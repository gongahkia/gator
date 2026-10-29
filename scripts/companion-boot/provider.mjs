import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { appendFile, readFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';

assert.equal(await readFile('/gator-boot-fixture', 'utf8'), 'gator disposable boot fixture\n');
createServer(async (req, res) => {
  try {
    let body = ''; for await (const chunk of req) body += chunk;
    const payload = JSON.parse(body);
    assert.ok(payload.messages.some(message => typeof message.content === 'string' && message.content.includes('unattended boot fixture')));
    await appendFile('/home/node/fixture/requests.jsonl', JSON.stringify({ at: new Date().toISOString() }) + '\n', { mode: 0o600 });
    res.writeHead(200, { 'content-type': 'application/json' });
    res.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: 'Scheduled fixture completed after unattended boot.' }, finish_reason: 'stop' }], usage: { prompt_tokens: 10, completion_tokens: 10 } }));
  } catch (error) {
    console.error(error.message);
    res.writeHead(500); res.end('Fixture failed.');
  }
}).listen(7342, '127.0.0.1', () => {
  execFileSync('systemd-notify', ['--ready'], { timeout: 10000 });
});
