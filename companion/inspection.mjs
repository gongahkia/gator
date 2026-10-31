// Validate the Go display projection before it crosses the remote boundary.
import { validateControlOffers } from './control.mjs';
export function validateInspection(value, target = '', generation = '') {
  const text = (v, limit) => typeof v === 'string' && Buffer.byteLength(v) <= limit;
  const stamp = v => text(v, 64) && Number.isFinite(Date.parse(v));
  if (!value || !Array.isArray(value.targets) || value.targets.length > 16 || Buffer.byteLength(JSON.stringify(value)) > 128 * 1024) throw new Error('Invalid work inspection response.');
  for (const item of value.targets) {
    const t = item.target, b = t?.binding;
    if (!b || !/^target-[a-f0-9]{32}$/.test(b.id) || b.backend !== 'gator_work' || !/^[a-f0-9]{32}$/.test(b.generation) || !text(b.owner, 256) || !text(b.incarnation, 512) || target && (b.id !== target || b.generation !== generation) || t.available !== true || !text(t.label, 256) || JSON.stringify(t.capabilities) !== '["inspect"]' || !text(item.run_id, 96) || item.conversation_id !== undefined && !text(item.conversation_id, 96) || item.status !== 'running' || !stamp(item.started_at) || !stamp(item.updated_at) || !stamp(item.observed_at) || !Array.isArray(item.progress) || item.progress.length > 24 || item.progress.some(v => !text(v, 512)) || !Array.isArray(item.specialists) || item.specialists.length > 16 || !Number.isSafeInteger(item.pending_approvals) || item.pending_approvals < 0 || !Array.isArray(item.limitations) || item.limitations.length > 4 || item.limitations.some(v => !text(v, 512))) throw new Error('Invalid work inspection target.');
    for (const s of item.specialists) if (!text(s.id, 512) || !text(s.role, 512) || !text(s.status, 512) || s.native_status !== undefined && !text(s.native_status, 512) || typeof s.pending_question !== 'boolean') throw new Error('Invalid specialist inspection.');
  }
  if (target && value.targets.length > 1) throw new Error('Ambiguous work inspection.');
  for (const item of value.targets) item.controls = validateControlOffers(item.controls);
  // Explicit projection: never forward arbitrary backend fields or secrets.
  return {
    targets: value.targets.map(i => ({ target: { binding: { id: i.target.binding.id, backend: i.target.binding.backend, owner: i.target.binding.owner, generation: i.target.binding.generation, incarnation: i.target.binding.incarnation }, label: i.target.label, capabilities: ['inspect'], available: true }, run_id: i.run_id, conversation_id: i.conversation_id, status: i.status, started_at: i.started_at, observed_at: i.observed_at, updated_at: i.updated_at, progress: i.progress, specialists: i.specialists.map(s => ({ id: s.id, role: s.role, status: s.status, native_status: s.native_status, pending_question: s.pending_question })), pending_approvals: i.pending_approvals, limitations: i.limitations, controls: i.controls })),
    unavailable: value.unavailable === true,
    reason: value.unavailable ? 'Owner unavailable or access withdrawn.' : '',
    truncated: value.truncated === true,
  };
}
