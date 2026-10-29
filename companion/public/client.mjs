const $ = id => document.getElementById(id);
let session, generation = 0, timer, busy = false, reading = false, fresh = false;
let state, pending, approvalKey = '', transcriptKey = '';
const dialog = $('approval-dialog');
const draftKey = id => `gator.companion.draft.v1:${id}`;
const report = message => { $('error').textContent = message; $('error').hidden = !message; };
const node = (tag, text, className) => { const element = document.createElement(tag); element.textContent = text; if (className) element.className = className; return element; };
const hostSetup = () => state?.panel || state?.external || state?.pending_connection;
const currentApproval = () => {
  const approval = state?.pending_approval;
  const message = state?.messages?.[approval?.message_index];
  if (!approval?.token || !message || message.result?.kind !== 'approval' || message.result.fields?.[0]?.value !== String(approval.id)) return null;
  return { approval, preview: message.text + '\n\n' + JSON.stringify(message.result, null, 2) };
};
function controls() {
  const active = !!session && fresh && !busy && !reading && !hostSetup();
  $('send').disabled = !active;
  $('review').disabled = $('deny').disabled = $('confirm').disabled = !active || !currentApproval();
  $('refresh').disabled = !session || busy || reading;
  $('message').readOnly = !!pending;
  $('send').textContent = pending ? 'Retry same message' : 'Send message';
  $('draft-status').textContent = pending
    ? 'Admission is not confirmed. Retry keeps the exact message and request ID; do not submit it as a new message elsewhere.'
    : 'Drafts stay in this browser tab’s session storage until sent. No attachments yet.';
}
function saveDraft() {
  if (!session) return;
  // Persist before dispatch so a reload can never silently invent a new ID.
  sessionStorage.setItem(draftKey(session.id), JSON.stringify({ text: $('message').value, pending }));
}
function loadDraft() {
  pending = null; $('message').value = '';
  const saved = JSON.parse(sessionStorage.getItem(draftKey(session.id)) || 'null');
  if (saved) {
    if (typeof saved.text !== 'string' || saved.text.length > 60000 || saved.pending && (saved.pending.upload?.context !== `conversation:${session.id}` || saved.pending.upload?.text !== saved.text || !saved.pending.upload?.request_id || saved.pending.upload?.files?.length !== 0 || saved.pending.type !== 'upload_submit')) throw new Error('The retained draft is invalid. Review this browser’s session storage before sending.');
    $('message').value = saved.text; pending = saved.pending || null;
  }
}
async function request(owner, frame) {
  const response = await fetch(`/v1/chats/${owner.id}${frame ? '/actions' : ''}`, {
    method: frame ? 'POST' : 'GET', cache: 'no-store', redirect: 'error',
    headers: { Authorization: `Bearer ${owner.token}`, ...(frame ? { 'Content-Type': 'application/json' } : {}) },
    body: frame ? JSON.stringify(frame) : undefined, signal: AbortSignal.timeout(30000),
  });
  const result = await response.json();
  if (!response.ok) {
    const error = new Error(result.error || 'The host did not confirm this request.');
    error.unauthorized = response.status === 401;
    throw error;
  }
  if (result.conversation_id !== owner.id || result.state?.conversation_id !== owner.id) throw new Error('The host returned a different conversation.');
  if (result.disconnected) throw new Error('The host process stopped. Open Gator on the host to review recovery.');
  return result.state;
}
function render(next) {
  state = next; fresh = !document.hidden;
  $('connection').textContent = 'Connected · work runs on your host';
  $('credentials').hidden = $('connect').hidden = true;
  $('connected-chat').hidden = false; $('connected-chat').textContent = `Conversation: ${session.id}`;
  $('chat').hidden = false; $('chat-title').textContent = state.title || session.id;
  $('host-status').textContent = state.running ? 'Working on the host…' : state.pending_approval ? 'Waiting for your decision' : 'Ready';
  $('host-notice').textContent = hostSetup() ? 'Finish setup or the open panel in Gator on the host before sending another action.' : state.pending_approval && !currentApproval() ? 'The current approval preview is unavailable. Review this action in Gator on the host.' : state.notice || state.requests_issue || '';
  $('host-notice').hidden = !$('host-notice').textContent;
  const messages = state.messages || [], nextTranscript = JSON.stringify(messages);
  if (nextTranscript !== transcriptKey) {
    transcriptKey = nextTranscript;
    $('messages').replaceChildren(...messages.map(message => {
      const article = node('article', '');
      article.append(node('h3', message.role === 'user' ? 'You' : message.role === 'assistant' ? 'Gator' : message.role), node('p', message.text || '', 'message-text'));
      for (const [label, value] of [['Result details', message.result], ['Files and action receipts', message.bundle]]) if (value) {
        const details = node('details', ''); details.append(node('summary', label), node('pre', JSON.stringify(value, null, 2))); article.append(details);
      }
      return article;
    }));
  }
  $('updates').replaceChildren(...(state.chat_updates || []).map(update => {
    const card = node('article', ''); card.append(node('h3', update.title), node('p', update.summary), node('p', `Status: ${update.status}`, 'hint')); return card;
  }));
  $('queue').replaceChildren(...(state.queue || []).map(item => node('p', `${item.interrupted ? 'Interrupted · review on host' : 'Queued'}: ${item.text}`)));
  const review = currentApproval(), nextKey = review ? JSON.stringify(review) : '';
  if (nextKey !== approvalKey) { dialog.close(); approvalKey = nextKey; }
  $('approval').hidden = !review;
  $('approval-preview').textContent = review?.preview || '';
  controls();
}
function failed(error) {
  fresh = false; dialog.close();
  $('connection').textContent = error.unauthorized ? 'Access expired · enter the current host token' : 'Disconnected · the host may still be working';
  report(error.message); controls();
  if (error.unauthorized) disconnect(false);
}
async function refresh() {
  if (!session || reading || busy) return;
  const owner = session, at = generation; reading = true; fresh = false; controls();
  try { const next = await request(owner); if (at === generation) { report(''); render(next); } }
  catch (error) { if (at === generation) failed(error); }
  finally { if (at === generation) { reading = false; controls(); } }
}
function disconnect(clear = true) {
  generation++; clearInterval(timer); session = null; state = null; fresh = false; busy = false; reading = false;
  approvalKey = ''; transcriptKey = ''; dialog.close();
  $('token').value = ''; $('conversation').readOnly = false; $('token').disabled = false;
  $('connect').disabled = false; $('disconnect').hidden = true;
  $('credentials').hidden = $('connect').hidden = false; $('connected-chat').hidden = true; $('connected-chat').textContent = '';
  $('chat').hidden = true; $('messages').replaceChildren(); $('updates').replaceChildren(); $('queue').replaceChildren();
  $('approval-preview').textContent = $('confirm-preview').textContent = ''; $('message').value = ''; pending = null;
  if (clear) { report(''); $('connection').textContent = 'Disconnected · admitted work stays on the host'; }
  controls();
}
$('connect-form').addEventListener('submit', async event => {
  event.preventDefault();
  if (session) return;
  if (!window.isSecureContext) return report('Open the companion through HTTPS, or use its loopback address on the host.');
  generation++; session = { id: $('conversation').value.trim(), token: $('token').value.trim() };
  $('token').value = ''; $('token').disabled = true; $('conversation').readOnly = true;
  $('connect').disabled = true; $('disconnect').hidden = false;
  try { loadDraft(); } catch (error) { disconnect(); return report(error.message); }
  await refresh();
  if (session) timer = setInterval(() => { if (!document.hidden) void refresh(); }, 2000);
});
$('disconnect').addEventListener('click', () => disconnect());
$('refresh').addEventListener('click', refresh);
$('message').addEventListener('input', () => { try { saveDraft(); } catch { report('This browser cannot retain the draft. Sending is unavailable until session storage works.'); } });
$('send-form').addEventListener('submit', async event => {
  event.preventDefault(); if (!session || !fresh || busy || reading || hostSetup() || !$('message').value.trim()) return;
  const owner = session, at = generation;
  pending ||= { type: 'upload_submit', upload: { request_id: crypto.randomUUID(), context: `conversation:${owner.id}`, text: $('message').value, files: [] } };
  if (new TextEncoder().encode(pending.upload.text).length > 128 * 1024 || new TextEncoder().encode(JSON.stringify(pending)).length > 256 * 1024) {
    pending = null; report('This message is too large. Shorten it before sending.'); controls(); return;
  }
  try { saveDraft(); } catch { report('Could not retain the request ID. Nothing was sent; keep this tab open and restore session storage.'); controls(); return; }
  busy = true; report(''); controls();
  try {
    const next = await request(owner, pending);
    if (at !== generation) return;
    sessionStorage.removeItem(draftKey(owner.id)); pending = null; $('message').value = ''; render(next);
  } catch (error) { if (at === generation) failed(error); }
  finally { if (at === generation) { busy = false; controls(); } }
});
$('review').addEventListener('click', () => {
  const review = currentApproval(); if (!review || !fresh || busy || reading) return;
  $('confirm-preview').textContent = review.preview; dialog.showModal();
});
$('cancel').addEventListener('click', () => dialog.close());
async function decide(approved) {
  const review = currentApproval(); if (!session || !review || !fresh || busy || reading || hostSetup()) return;
  if (approved && (!dialog.open || $('confirm-preview').textContent !== review.preview)) return;
  const owner = session, at = generation;
  busy = true; dialog.close(); controls(); report('');
  try {
    const next = await request(owner, { type: 'approval', interaction_id: review.approval.id, approval_token: review.approval.token, approved });
    if (at === generation) render(next);
  } catch (error) { if (at === generation) failed(error); }
  finally { if (at === generation) { busy = false; controls(); } }
}
$('confirm').addEventListener('click', () => decide(true));
$('deny').addEventListener('click', () => decide(false));
document.addEventListener('visibilitychange', () => {
  fresh = false; dialog.close(); controls();
  if (!document.hidden) void refresh();
});
window.addEventListener('offline', () => { fresh = false; dialog.close(); $('connection').textContent = 'Offline · the host may still be working'; controls(); });
window.addEventListener('online', refresh);
window.addEventListener('pagehide', () => disconnect());
controls();
