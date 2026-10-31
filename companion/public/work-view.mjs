// Read-only, independent of chat selection/drafts/approval controls.
import { createWorkActions } from './work-actions.mjs';
export function createWorkView({ getSession, revoked, document = globalThis.document, fetch = globalThis.fetch }) {
  const $ = id => document.getElementById(id);
  let owner, selection, revision = 0, timer, controller, busy = false, catalog = [];
  const actions=createWorkActions({getSession,revoked});
  const clear = () => { $('work-meta').textContent = ''; $('work-progress').textContent = ''; };
  const unavailable = text => { clear(); actions.select(owner,selection,null); $('work-status').textContent = text; };
  function sync() {
    const next = getSession();
    $('owned-work').hidden = !next;
    if (next !== owner) {
      owner = next; selection = undefined; catalog = []; revision++; controller?.abort(); busy = false;
      $('work-select').replaceChildren(new Option('Choose a task', ''));
      unavailable('Refresh to see shared Work.');
    }
    if (!owner || document.hidden || !globalThis.navigator.onLine) {
      revision++; controller?.abort(); busy = false;
      if (owner) $('work-status').textContent = 'Offline or hidden · displayed Work may be stale.';
      actions.stale();
    }
    const polling = owner && !document.hidden && globalThis.navigator.onLine && $('work-open').open;
    if (!polling) { clearInterval(timer); timer = undefined; }
    else if (!timer) timer = setInterval(() => { if (!busy) void refresh(); }, 3000);
  }
  async function read(path, signal, session) {
    const response = await fetch(path, { cache: 'no-store', redirect: 'error', headers: { Authorization: `Bearer ${session.token}` }, signal });
    if (response.status === 401) { const error = new Error('Access expired'); error.revoked = true; throw error; }
    if (!response.ok) throw new Error('Work could not refresh.');
    return response.json();
  }
  async function refresh() {
    if (!owner || document.hidden || !navigator.onLine || !$('work-open').open) return;
    const session = owner, chosen = selection, at = ++revision;
    controller?.abort(); controller = new AbortController(); const abort = controller;
    const timeout = setTimeout(() => abort.abort(), 25000);
    busy = true; $('work-status').textContent = 'Refreshing shared Work…';
    try {
      const page = await read('/v1/work', abort.signal, session);
      if (at !== revision || session !== owner || session !== getSession()) return;
      catalog = page.targets;
      const options = [new Option('Choose a task', '')];
      for (const item of catalog) options.push(new Option(`${item.target.label} · ${item.conversation_id || 'No conversation'}`, item.target.binding.id));
      for(const saved of actions.retainedTargets())if(!catalog.some(i=>i.target.binding.id===saved.target.id)&&saved.target.id!==chosen?.id)options.push(new Option(`Retained action receipt · ${saved.run_id}`,saved.target.id));
      if (chosen && !catalog.some(i => i.target.binding.id === chosen.id)) options.push(new Option('Selected task · unavailable', chosen.id));
      $('work-select').replaceChildren(...options); $('work-select').value = chosen?.id || '';
      if (!chosen) { unavailable(catalog.length ? 'Choose shared Work to inspect.' : 'No shared live Work. Grant Inspect access on the host, then refresh.'); return; }
      const pageDetail = await read(`/v1/work/${encodeURIComponent(chosen.id)}?generation=${encodeURIComponent(chosen.generation)}`, abort.signal, session);
      if (at !== revision || session !== owner || session !== getSession()) return;
      const item = pageDetail.targets[0];
      if (!item || pageDetail.unavailable) { unavailable('Unavailable · owner stopped, target changed, or access withdrawn.'); return; }
      if (item.target.binding.id !== chosen.id || item.target.binding.generation !== chosen.generation) throw new Error('Work identity changed.');
      $('work-status').textContent = `Running · last activity ${item.updated_at}`;
      $('work-meta').textContent = `${item.run_id} · ${item.conversation_id || 'No conversation'} · ${item.target.binding.backend}\nStarted ${item.started_at} · observed ${item.observed_at}\nPending approvals: ${item.pending_approvals}`;
      const tasks = item.specialists.map(s => `${s.role} (${s.id}): ${s.native_status || s.status}${s.pending_question ? ' · question pending' : ''}`);
      $('work-progress').textContent = [...item.progress, ...tasks, ...item.limitations].join('\n');
      actions.select(owner,chosen,item);
    } catch (error) {
      if (at !== revision || session !== owner || session !== getSession()) return;
      if (error.revoked) { unavailable('Access expired.'); revoked(); }
      else { actions.stale(); $('work-status').textContent = 'Work could not refresh · displayed information may be stale. Refresh to retry.'; }
    } finally { clearTimeout(timeout); if (at === revision) busy = false; }
  }
  $('work-select').addEventListener('change', () => {
    selection = catalog.find(i => i.target.binding.id === $('work-select').value)?.target.binding || actions.retainedTargets().find(i=>i.target.id===$('work-select').value)?.target;
    revision++; controller?.abort(); clear(); void refresh();
    actions.select(owner,selection,null);
  });
  $('work-refresh').addEventListener('click', () => void refresh());
  $('work-open').addEventListener('toggle', () => { sync(); if ($('work-open').open) void refresh(); else { revision++; controller?.abort(); busy = false; } });
  document.addEventListener('visibilitychange', () => { sync(); if (!document.hidden) void refresh(); });
  globalThis.addEventListener('offline', sync);
  globalThis.addEventListener('online', () => { sync(); void refresh(); });
  return { sync };
}
