// Loopback-only acceptance fixture. It requests access but never fetches or sends.
import http from 'node:http';
import {appendFile, access} from 'node:fs/promises';
import {join} from 'node:path';
const root = process.env.GATOR_FIXTURE_CONTROL;
if (!root) throw Error('Disposable fixture directory required');
const record = value => appendFile(join(root, 'provider-trace.jsonl'), JSON.stringify(value) + '\n');
const server = http.createServer(async (req, res) => {
  try {
    if (req.url !== '/chat') {
      await record({kind: 'unexpected-effect', path: req.url, method: req.method});
      res.writeHead(404); res.end(); return;
    }
    let raw = ''; for await (const part of req) raw += part;
    const body = JSON.parse(raw), messages = body.messages || [];
    const system = messages.filter(m => m.role === 'system').map(m => m.content).join('\n');
    const text = messages.filter(m => m.role === 'user').at(-1)?.content || '';
    const tools = (body.tools || []).map(t => t.function?.name);
    let message;
    if (system.includes('You handle the conversation with Gator')) {
      const context = JSON.parse(messages.at(-1).content);
      await record({kind: 'route', message: context.message});
      message = {role: 'assistant', content: JSON.stringify(context.message === 'Begin inert pending approval' ? {kind: 'task'} : {kind: 'reply', text: 'The unrelated task continues.'})};
    } else if (system.includes("Classify the user's immediate request")) {
      message = {role: 'assistant', content: '{"mode":"inspect"}'};
    } else if (text === 'Hold unrelated work') {
      await record({kind: 'held-work', tools});
      req.on('close', () => {});
      while (!res.destroyed) {
        try { await access(join(root, 'release-work')); break; } catch {}
        await new Promise(resolve => setTimeout(resolve, 25));
      }
      if (res.destroyed) { await record({kind: 'held-work-disconnected'}); return; }
      await record({kind: 'held-work-released'});
      message = {role: 'assistant', content: 'The unrelated fixture task completed.'};
    } else if (text === 'Begin inert pending approval' && !messages.some(m => m.role === 'tool')) {
      await record({kind: 'proposal', tools, selectedToolAvailable: tools.includes('web_access')});
      if (!tools.includes('web_access')) throw Error('Required inert tool was not advertised');
      message = {role: 'assistant', tool_calls: [{id: 'inert-access-proposal', type: 'function', function: {name: 'web_access', arguments: JSON.stringify({action: 'request', scope: 'sites', origins: ['https://acceptance.example.invalid'], reason: 'Inert exact-review fixture. Decline this request. No fetch, search, send or external account operation is implemented by this fixture.'})}}]};
    } else {
      await record({kind: 'terminal', hasToolResult: messages.some(m => m.role === 'tool')});
      message = {role: 'assistant', content: 'The inert review outcome is retained. No external action was executed.'};
    }
    res.writeHead(200, {'Content-Type': 'application/json'});
    res.end(JSON.stringify({choices: [{message, finish_reason: message.tool_calls ? 'tool_calls' : 'stop'}], usage: {prompt_tokens: 1, completion_tokens: 1, total_tokens: 2}}));
  } catch (error) {
    await record({kind: 'fixture-error', error: String(error)});
    if (!res.destroyed) { res.writeHead(500); res.end('{}'); }
  }
});
server.listen(0, '127.0.0.1', () => process.stdout.write(JSON.stringify({port: server.address().port}) + '\n'));
