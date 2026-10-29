const $ = id => document.getElementById(id);
let session, generation = 0, timer, busy = false, reading = false, fresh = false;
let state, pending, approvalKey = '', transcriptKey = '';
let browsing = false, catalogQuery = { query: '', scope: 'active', after: '' }, anchors = [], nextPage = '';
let creationPending = null, creationIssue = '', newChatSource = '';
const creationKey = 'gator.companion.creation.v1';
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
  const active = !!session?.id && !browsing && fresh && !document.hidden && !busy && !reading && !hostSetup();
  $('send').disabled = !active;
  $('review').disabled = $('deny').disabled = $('confirm').disabled = !active || !currentApproval();
  $('refresh').disabled = !session || busy || reading;
  $('browse').disabled = !session || busy || reading;
  for (const id of ['search-submit', 'catalog-refresh', 'search', 'scope', 'return-chat']) $(id).disabled = !session || busy || reading || document.hidden;
  $('previous').disabled = !session || !fresh || busy || reading || document.hidden || !anchors.length;
  $('next').disabled = !session || !fresh || busy || reading || document.hidden || !nextPage;
  for (const button of $('catalog-items').querySelectorAll('button')) button.disabled = !fresh || busy || reading || document.hidden;
  $('new-chat').disabled = !session || !browsing || !newChatSource || !!creationIssue || busy || reading || document.hidden || (!fresh && !creationPending) || !!creationPending && creationPending.source !== newChatSource;
  $('new-chat').textContent = creationPending ? 'Retry new chat' : 'New chat';
  $('dismiss-creation').hidden = !creationPending && !creationIssue;
  $('dismiss-creation').disabled = !session || busy || reading;
  $('creation-status').textContent = creationIssue || (creationPending
    ? creationPending.source !== newChatSource ? 'The host workspace changed. Dismiss this retry before starting another chat.' : 'Opening the new chat is unconfirmed. Retry uses the same request. Dismiss leaves any created chat in Conversations.'
    : 'Creates an empty chat. Work starts only when you send a message.');
  $('message').readOnly = !!pending;
  $('send').textContent = pending ? 'Retry same message' : 'Send message';
  $('draft-status').textContent = pending
    ? 'Admission is not confirmed. Retry keeps the exact message and request ID; do not submit it as a new message elsewhere.'
    : 'Drafts stay in this browser tab’s session storage until sent. No attachments yet.';
}
function saveDraft() {
  if (!session?.id) return;
  // Persist before dispatch so a reload can never silently invent a new ID.
  sessionStorage.setItem(draftKey(session.id), JSON.stringify({ text: $('message').value, pending }));
}
function readDraft(id) {
  const saved = JSON.parse(sessionStorage.getItem(draftKey(id)) || 'null');
  if (saved) {
    if (typeof saved.text !== 'string' || saved.text.length > 60000 || saved.pending && (saved.pending.upload?.context !== `conversation:${id}` || saved.pending.upload?.text !== saved.text || !saved.pending.upload?.request_id || saved.pending.upload?.files?.length !== 0 || saved.pending.type !== 'upload_submit')) throw new Error('The retained draft is invalid. Review this browser’s session storage before sending.');
  }
  return saved || { text: '', pending: null };
}
function loadDraft() {
  const saved = readDraft(session.id);
  $('message').value = saved.text; pending = saved.pending || null;
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
  $('browse').hidden = false; $('catalog').hidden = true;
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
  if (browsing) return loadCatalog(catalogQuery, anchors);
  const owner = session, at = generation; reading = true; fresh = false; controls();
  try { const next = await request(owner); if (at === generation) { report(''); render(next); } }
  catch (error) { if (at === generation) failed(error); }
  finally { if (at === generation) { reading = false; controls(); } }
}
function disconnect(clear = true) {
  generation++; clearInterval(timer); session = null; state = null; fresh = false; busy = false; reading = false;
  approvalKey = ''; transcriptKey = ''; dialog.close();
  browsing = false; anchors = []; nextPage = ''; catalogQuery = { query: '', scope: 'active', after: '' };
  creationPending = null; creationIssue = ''; newChatSource = ''; $('new-chat-controls').hidden = true; $('new-chat-source').textContent = '';
  $('catalog').hidden = $('browse').hidden = true; $('catalog-items').replaceChildren(); $('catalog-status').textContent = '';
  $('search').value = ''; $('scope').value = 'active';
  $('token').value = ''; $('conversation').readOnly = false; $('token').disabled = false;
  $('connect').disabled = false; $('disconnect').hidden = true;
  $('credentials').hidden = $('connect').hidden = false; $('connected-chat').hidden = true; $('connected-chat').textContent = '';
  $('chat').hidden = true; $('messages').replaceChildren(); $('updates').replaceChildren(); $('queue').replaceChildren();
  $('approval-preview').textContent = $('confirm-preview').textContent = ''; $('message').value = ''; pending = null;
  if (clear) { report(''); $('connection').textContent = 'Disconnected · admitted work stays on the host'; }
  controls();
}

async function loadCatalog(query = { query: '', scope: 'active', after: '' }, history = []) {
  if (!session || busy || reading) return;
  const owner = session, at = generation;
  try { saveDraft(); } catch { return report('Could not retain this draft. Keep the current chat open until session storage works.'); }
  browsing = true; reading = true; fresh = false; dialog.close();
  catalogQuery = query; anchors = history; nextPage = '';
  $('catalog').hidden = false; $('chat').hidden = true; $('return-chat').hidden = !session.id;
  $('catalog-items').replaceChildren(); $('catalog-status').textContent = 'Loading conversations…'; controls();
  try {
    const params = new URLSearchParams({ q: query.query, scope: query.scope, after: query.after });
    const response = await fetch(`/v1/chats?${params}`, { cache: 'no-store', redirect: 'error', headers: { Authorization: `Bearer ${owner.token}` }, signal: AbortSignal.timeout(30000) });
    const page = await response.json();
    if (!response.ok) { const error = new Error(page.error || 'Could not load conversations.'); error.unauthorized = response.status === 401; throw error; }
    if (at !== generation) return;
    if (!Array.isArray(page.items) || page.items.some(item => !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(item.id))) throw new Error('The host returned an invalid conversation list.');
    newChatSource = typeof page.new_chat_source === 'string' ? page.new_chat_source : '';
    $('new-chat-controls').hidden = !newChatSource;
    $('new-chat-source').textContent = `New chats use this host workspace: ${newChatSource}`;
    nextPage = page.next || ''; fresh = !document.hidden;
    $('credentials').hidden = $('connect').hidden = true; $('browse').hidden = false;
    $('connection').textContent = 'Connected · work runs on your host';
    $('catalog-status').textContent = `${page.items.length ? `${page.items.length} ${page.items.length === 1 ? 'conversation' : 'conversations'} on this page.` : 'No conversations match.'}${page.incomplete || page.skipped ? ' Some retained conversations could not be checked; this list may be incomplete.' : ''}`;
    $('catalog-items').replaceChildren(...page.items.map(item => {
      const button = node('button', '', 'secondary chat-choice'); button.type = 'button';
      button.append(node('span', item.title || item.id), node('span', `${item.pinned ? 'Pinned · ' : ''}${item.archived ? 'Archived · ' : ''}${item.source || ''}`, 'hint'));
      button.addEventListener('click', () => openChat(item.id)); return button;
    }));
    report('');
  } catch (error) { if (at === generation) { $('catalog-status').textContent = 'Conversations unavailable. Refresh the list to retry.'; failed(error); } }
  finally { if (at === generation) { reading = false; controls(); } }
}

async function createNewChat() {
  if ($('new-chat').disabled || !session) return;
  const at = generation, token = session.token;
  try {
    saveDraft();
    creationPending ||= { request_id: crypto.randomUUID(), source: newChatSource };
    // Keep this identity until both creation and opening are acknowledged.
    sessionStorage.setItem(creationKey, JSON.stringify(creationPending));
  } catch { report('Could not retain the new-chat request. Nothing was sent; restore session storage before retrying.'); controls(); return; }
  const creation = creationPending;
  busy = true; dialog.close(); controls(); report('');
  try {
    const response = await fetch('/v1/chats', { method: 'POST', cache: 'no-store', redirect: 'error', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ request_id: creation.request_id }), signal: AbortSignal.timeout(30000) });
    const result = await response.json();
    if (!response.ok) { const error = new Error(result.error || 'Chat creation is unconfirmed. Retry the same request.'); error.unauthorized = response.status === 401; throw error; }
    if (at !== generation) return;
    if (result.request_id !== creation.request_id || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(result.conversation_id)) throw new Error('The host did not confirm this chat identity. Retry the same request.');
    const owner = { id: result.conversation_id, token }, draft = readDraft(owner.id), next = await request(owner);
    if (at !== generation) return;
    sessionStorage.removeItem(creationKey); creationPending = null;
    session = owner; browsing = false; pending = draft.pending || null; $('message').value = draft.text;
    transcriptKey = ''; approvalKey = ''; report(''); render(next); $('message').focus();
  } catch (error) { if (at === generation) failed(error); }
  finally { if (at === generation) { busy = false; controls(); } }
}
$('new-chat').addEventListener('click', createNewChat);
$('dismiss-creation').addEventListener('click', () => {
  if (!session || busy || reading) return;
  try { sessionStorage.removeItem(creationKey); creationPending = null; creationIssue = ''; report('Retry dismissed. Any created chat remains in Conversations.'); controls(); }
  catch { report('Could not dismiss the retained request. Restore session storage before starting another chat.'); }
});

async function openChat(id) {
  if (!session || busy || reading || !fresh || document.hidden) return;
  const at = generation, owner = { id, token: session.token };
  busy = true; controls();
  try {
    const draft = readDraft(id), next = await request(owner);
    if (at !== generation) return;
    session = owner; browsing = false; pending = draft.pending || null; $('message').value = draft.text;
    transcriptKey = ''; approvalKey = ''; report(''); render(next); $('chat-title').focus();
  } catch (error) { if (at === generation) failed(error); }
  finally { if (at === generation) { busy = false; controls(); } }
}
$('browse').addEventListener('click', () => { void loadCatalog({ query: $('search').value, scope: $('scope').value, after: '' }).then(() => { if (session && browsing) $('catalog-title').focus(); }); });
$('search-form').addEventListener('submit', event => { event.preventDefault(); void loadCatalog({ query: $('search').value, scope: $('scope').value, after: '' }); });
$('catalog-refresh').addEventListener('click', () => loadCatalog(catalogQuery, anchors));
$('next').addEventListener('click', () => loadCatalog({ ...catalogQuery, after: nextPage }, [...anchors, catalogQuery.after]));
$('previous').addEventListener('click', () => loadCatalog({ ...catalogQuery, after: anchors.at(-1) || '' }, anchors.slice(0, -1)));
$('return-chat').addEventListener('click', async () => { if (!session?.id || busy || reading) return; browsing = false; $('catalog').hidden = true; await refresh(); if (session && !browsing) $('chat-title').focus(); });
$('connect-form').addEventListener('submit', async event => {
  event.preventDefault();
  if (session) return;
  if (!window.isSecureContext) return report('Open the companion through HTTPS, or use its loopback address on the host.');
  generation++; session = { id: $('conversation').value.trim(), token: $('token').value.trim() };
  try {
    const saved = JSON.parse(sessionStorage.getItem(creationKey) || 'null');
    if (saved && (typeof saved.request_id !== 'string' || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{15,95}$/.test(saved.request_id) || typeof saved.source !== 'string' || !saved.source)) throw new Error('Invalid retained creation request.');
    creationPending = saved;
  } catch { creationIssue = 'The retained new-chat request is unavailable. Restore session storage or dismiss it before starting another chat.'; }
  $('token').value = ''; $('token').disabled = true; $('conversation').readOnly = true;
  $('connect').disabled = true; $('disconnect').hidden = false;
  if (session.id) {
    try { loadDraft(); } catch (error) { disconnect(); return report(error.message); }
    await refresh();
  } else await loadCatalog();
  if (session) timer = setInterval(() => { if (!document.hidden && !browsing) void refresh(); }, 2000);
});
$('disconnect').addEventListener('click', () => disconnect());
$('refresh').addEventListener('click', refresh);
$('message').addEventListener('input', () => { try { saveDraft(); } catch { report('This browser cannot retain the draft. Sending is unavailable until session storage works.'); } });
$('send-form').addEventListener('submit', async event => {
  event.preventDefault(); if (!session?.id || browsing || document.hidden || !fresh || busy || reading || hostSetup() || !$('message').value.trim()) return;
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
  const review = currentApproval(); if (!review || browsing || document.hidden || !fresh || busy || reading) return;
  $('confirm-preview').textContent = review.preview; dialog.showModal();
});
$('cancel').addEventListener('click', () => dialog.close());
async function decide(approved) {
  const review = currentApproval(); if (!session?.id || browsing || document.hidden || !review || !fresh || busy || reading || hostSetup()) return;
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
