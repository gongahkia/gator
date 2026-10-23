import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { ArrowDown, ArrowUp, Bookmark, ChevronRight, Clock3, Files, Info, ListPlus, LoaderCircle, MessageSquare, PanelLeftClose, PanelLeftOpen, Plus, Search, Settings2, ShieldCheck, SlidersHorizontal, Square, Target, X } from "lucide-react";
import type { BackendMessage, BackendState } from "./protocol";
import type { DesktopFrame, DesktopSnapshot } from "./bridge";
import { Panel } from "./Panel";
import { Markdown, Result } from "./Result";
import { useModalFocus } from "./useModalFocus";
import { UsagePanel } from "./UsagePanel";
import { ConversationsPanel } from "./ConversationsPanel";
import { IconSwap, Presence, useViewMotion, type PresenceRef } from "./motion";
import { MessageQueue, useQueueAction } from "./MessageQueue";
import { ChatMemory, useChatMemoryAction } from "./ChatMemory";
import type { ChatMemoryAction, ChatMemoryReceipt } from "./protocol";
import { FirstTaskSetup } from "./FirstTaskSetup";
import { SettingsNavigation, settingsRoutes } from "./SettingsNavigation";
import { CopyButton } from "./CopyButton";
import { useDrafts } from "./useDrafts";
import { DraftManager } from "./DraftManager";
import { useConversationSearch } from "./useConversationSearch";
import { RecentConversations } from "./RecentConversations";
import { WorkDetails } from "./WorkDetails";
import { LibraryDrawer } from "./LibraryDrawer";
import { OutputCards } from "./OutputCards";
import { bundleOutputs, conversationBundles } from "./conversationOutputs";
import { DesktopHelp } from "./DesktopHelp";
import { UpdateDialog } from "./UpdateDialog";
import { AsciiWaves } from "./AsciiWaves";
import { ActivityHistory, LiveActivity } from "./ActivityTimeline";
import { AttachButton, InputFiles, PendingAttachments, useAttachmentPicker } from "./Attachments";

import { VoiceControls } from "./VoiceControls";
import { ChatCheckHealth } from "./ChatCheckHealth";
import { ConnectedReceipts, RecoveredRequests, useReceiptInspection, type InspectReceipt } from "./ConnectedReceipts";
import { ChatUpdateCard, chatTimeline, useChatUpdateFeedback } from "./ChatUpdates";
import { ModelPicker } from "./ModelPicker";
import { FileChatContext } from "./FileSurface";
import type { UploadedFile } from "./bridge";
import { shortcutMatches, shortcutLabel } from "./shortcuts";

type Navigation = { command: string; draft?: string } | { conversation: NonNullable<DesktopFrame["conversation"]> } | { session: string };

function Logo({ small = false }: { small?: boolean }) {
  return <span className={`gator-logo ${small ? "small" : ""}`} aria-hidden="true">🐊</span>;
}

function useBackend() {
  const [snapshot, setSnapshot] = useState<DesktopSnapshot>({});
  const [error, setError] = useState("");
  const [reconnecting, setReconnecting] = useState(false);
  const [reconnectError, setReconnectError] = useState("");
  useEffect(() => {
    const api = window.gator;
    if (!api) { setError("Open Gator as a desktop app with npm start from the desktop directory."); return; }
    let live = true;
    let received = false;
    const unsubscribe = api.subscribe((next) => { received = true; if (live) { setSnapshot(next); if (next.connection === "connected" || next.connection === "disconnected") setReconnecting(false); } });
    api.snapshot().then((next) => { if (live && !received) setSnapshot(next); }).catch((err) => { if (live) setError(String(err)); });
    return () => { live = false; unsubscribe(); };
  }, []);
  const send = useCallback(async (frame: DesktopFrame) => {
    try { await window.gator?.send(frame); setError(""); }
    catch (err) { if (frame.type !== "upload_submit" && frame.type !== "composer") setError(err instanceof Error ? err.message : String(err)); throw err; }
  }, []);
  const reconnect = async () => {
    if (reconnecting) return;
    setReconnecting(true); setReconnectError(""); setError("");
    try { await window.gator!.restart(); }
    catch (error) { setReconnectError(error instanceof Error ? error.message : "Could not reconnect. Try again."); setReconnecting(false); }
  };
  return { ...snapshot, error: error || reconnectError || snapshot.error, reconnecting, reconnect, send, clearError: () => { setError(""); setReconnectError(""); } };
}

const Message = memo(function Message({ message, pending, onApproval, commands, navigate, disabled, preview, shortcuts, memories, conversation, memoryBusy, changeMemory, onMemoryEditingChange, inspectReceipt, receiptBusy, receiptDisabled }: { inspectReceipt: InspectReceipt; receiptBusy: boolean; receiptDisabled: boolean; onMemoryEditingChange(id: string, editing: boolean): void; memories?: ChatMemoryReceipt[]; conversation?: string; memoryBusy?: boolean; changeMemory(change: Omit<ChatMemoryAction,"request_id">): Promise<void>; shortcuts?: Record<string,string>; message: BackendMessage; pending?: number; onApproval(approved: boolean): void; commands?: BackendState["commands"]; navigate?(route: string): void; disabled?: boolean; preview(id: string): void }) {
  const user = message.role === "You";
  const result = message.result;
  const outputs = message.bundle ? bundleOutputs(message.bundle) : [];
  // The runtime retains a bundle and context shortcut for every run. Keep
  // those records available to the inspector without repeating them in chat.
  const contextLink = message.role === "Context" && result?.kind === "notice" && result.title === "Context and sources" && result.footer?.startsWith("/context ") && !result.tone;
  if (contextLink || (message.bundle && !result && !outputs.length && !message.bundle.Actions?.length)) return null;
  const approval = result?.kind === "approval" && result.fields?.some((field) => field.label === "Request" && field.value === String(pending)) ? pending : undefined;
  return <article className={`message ${user ? "user-message" : message.role === "Gator" ? "assistant-message" : "result-message"}`} aria-label={user ? "Your message" : `${message.role} message`}>
    <div className="message-content">{message.activity && <ActivityHistory trace={message.activity} />}<div className="message-byline">{user ? "You" : message.role === "Gator" ? "Gator" : message.bundle ? (outputs.length ? "Files & changes" : "Connected requests") : message.role}</div>
      {user && message.reply_to_update_id && <span className="message-reply-label">Reply to a follow-up</span>}{user && <InputFiles names={message.inputs} ids={message.input_ids} />}
      {commands && navigate && result?.title === "Gator commands" && result.kind === "report" ? <DesktopHelp shortcuts={shortcuts} commands={commands} navigate={navigate} disabled={!!disabled} /> : result ? <Result result={result} approval={approval} onApproval={onApproval} /> : message.bundle ? <OutputCards outputs={outputs} preview={preview} /> : message.role === "Gator" ? <div className="assistant-speech"><Markdown text={message.text} /></div> : <div className="literal prose-text">{message.text}</div>}
      {message.bundle && <ConnectedReceipts bundle={message.bundle} inspect={inspectReceipt} busy={receiptBusy} disabled={receiptDisabled} />}
      {message.role === "Gator" && message.run_id && conversation && <ChatMemory receipts={memories ?? []} conversation={conversation} disabled={!!memoryBusy} act={changeMemory} onEditingChange={onMemoryEditingChange} />}
      {!result && message.role === "Gator" && message.text && <div className="message-actions"><CopyButton text={message.text} label="Copy response" compact /></div>}
    </div>
  </article>;
}, (before, after) => before.inspectReceipt === after.inspectReceipt && before.receiptBusy === after.receiptBusy && before.receiptDisabled === after.receiptDisabled && before.conversation === after.conversation && before.memoryBusy === after.memoryBusy && before.changeMemory === after.changeMemory && before.onMemoryEditingChange === after.onMemoryEditingChange && JSON.stringify(before.memories) === JSON.stringify(after.memories) && JSON.stringify(before.message.input_ids) === JSON.stringify(after.message.input_ids) && JSON.stringify(before.message.inputs) === JSON.stringify(after.message.inputs) && JSON.stringify(before.message.activity) === JSON.stringify(after.message.activity) && before.message.reply_to_update_id === after.message.reply_to_update_id && before.pending === after.pending && before.message.run_id === after.message.run_id && before.message.role === after.message.role && before.message.text === after.message.text && JSON.stringify(before.message.result ?? before.message.bundle) === JSON.stringify(after.message.result ?? after.message.bundle) && before.onApproval === after.onApproval && before.disabled === after.disabled && before.commands === after.commands && before.navigate === after.navigate && before.preview === after.preview);

export function App() {
  const { state, error, connection, sessionID, sessionSwitching, sessionIssue, chatActivity, allChatsBusy, allChatsQueued, terminalBusy, fullScreen, updates, updatesOpen, updatePreparing, reconnecting, reconnect, send, clearError } = useBackend();
  const commands = useMemo(() => (state?.commands ?? []), [state?.commands]);
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("gator:sidebar-collapsed") === "true");
  const [details, setDetails] = useState(false);
  const [filesOnly, setFilesOnly] = useState(false);
  const [detailKind, setDetailKind] = useState<"info" | "following" | "saved">("info");
  const libraryKind = useRef<"following" | "saved">("saved");
  if (detailKind !== "info") libraryKind.current = detailKind;
  const [requestedOutput, setRequestedOutput] = useState<{ conversation: string; output: string; request: number }>();
  const drawerReturn = useRef<HTMLElement | null>(null);
  const drafts = useDrafts(state);
  const draft = drafts.text, setDraft = drafts.setText;
  const [showDrafts, setShowDrafts] = useState(false);
  const originalSession = useRef<string | undefined>(undefined);
  const [sessionChanged, setSessionChanged] = useState(false);
  const [search, setSearch] = useState<string | null>(null);
  const [navigation, setNavigation] = useState<{ target: Navigation; queued: boolean } | null>(null);
  const [showCommands, setShowCommands] = useState(false);
  const commandReturn = useRef<HTMLElement | null>(null);
  const [sending, setSending] = useState(false);
  const [navigationPending, setNavigationPending] = useState(false);
  const admission = useRef<{ key: string; id: string } | null>(null);
  const [voiceBusy, setVoiceBusy] = useState(false);
  const [modelBusy, setModelBusy] = useState(false);
  const [approvalBusy, setApprovalBusy] = useState(false);
  const [draftError, setDraftError] = useState("");
  const [queueEditing, setQueueEditing] = useState(false);
  const [memoryEditing, setMemoryEditing] = useState<Set<string>>(()=>new Set());
  const onMemoryEditingChange = useCallback((id: string, editing: boolean)=>setMemoryEditing(previous=>{if(previous.has(id)===editing)return previous;const next=new Set(previous);if(editing)next.add(id);else next.delete(id);return next;}),[]);
  const [dirtyFiles, setDirtyFiles] = useState<Set<string>>(() => new Set());
  const fileAttachments = useRef(new Map<string,string>());
  const attachmentKeys = useRef(new Map<string,string>());
  const attachmentKey = useCallback((id: string) => attachmentKeys.current.get(id) ?? id, []);
  const queueAction = useQueueAction(state, send, error);
  const memoryAction = useChatMemoryAction(state, send, error);
  const updateFeedback = useChatUpdateFeedback(state, send, error);
  const receiptInspection = useReceiptInspection(state, send, error);
  const attachments = useAttachmentPicker(drafts.context, drafts.uploads, (files) => { drafts.setUploads(files); setDraftError(""); }, sending || queueAction.busy || !!error || !!state?.external || !!updatePreparing || !drafts.ready);
  const composer = useRef<HTMLTextAreaElement>(null);
  const newChatFocus = useRef<string | undefined>(undefined);
  const detailsTrigger = useRef<HTMLButtonElement>(null);
  const closeDetails = () => { setDetails(false); (drawerReturn.current?.isConnected ? drawerReturn.current : detailsTrigger.current)?.focus({ preventScroll: true }); };
  const openDetails = (kind: typeof detailKind) => { drawerReturn.current = document.activeElement as HTMLElement; setRequestedOutput(undefined); setDetailKind(kind); setFilesOnly(false); setDetails(!(details && detailKind === kind && !filesOnly)); };
  const transcript = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const [atLatest, setAtLatest] = useState(true);
  const [newActivity, setNewActivity] = useState(false);
  const previousTranscript = useRef({ identity: "", content: "", running: false });
  const transcriptPositions = useRef(new Map<string, { top: number; latest: boolean }>());
  const returningToTranscript = useRef(false);
  const backendInput = useRef<string | undefined>(undefined);
  const view = useRef<HTMLDivElement>(null);
  const panel = state?.panel;
  const conversationIdentity = state?.conversation_id || `${state?.primary ? "primary" : "topic"}:${state?.source || ""}`;
  const openOutput = useCallback((output: string) => { drawerReturn.current = document.activeElement as HTMLElement; setRequestedOutput({ conversation: conversationIdentity, output, request: Date.now() }); setDetailKind("info"); setFilesOnly(false); setDetails(true); }, [conversationIdentity]);
  const openFiles = () => { drawerReturn.current = document.activeElement as HTMLElement; setRequestedOutput(undefined); setDetailKind("info"); setFilesOnly(true); setDetails(!(details && detailKind === "info" && filesOnly)); };
  const showOutputInChat = (output: string) => {
    const element = [...(transcript.current?.querySelectorAll<HTMLElement>("[data-chat-output]") ?? [])].find((value) => value.dataset.chatOutput === output);
    const button = element?.matches("button") ? element : element?.querySelector<HTMLButtonElement>(".inline-file-card");
    if (!button) return;
    setSearch(null); setDetails(false);
    requestAnimationFrame(() => { if (button.classList.contains("inline-file-card") && button.getAttribute("aria-expanded") !== "true") button.click(); button.focus({ preventScroll: true }); button.scrollIntoView({ block: "center" }); });
  };
  const conversationSearch = useConversationSearch(transcript, search, conversationIdentity, !panel);
  const settingsPage = settingsRoutes.some((route) => route.command === state?.destination);
  const modifier = window.gator?.platform === "darwin" ? "⌘" : "Ctrl+";
  const viewKey = state ? `${state.destination || (state.home && !state.messages.length ? "home" : "chat")}:${panel?.kind ?? ""}:${panel?.title ?? ""}` : undefined;
  useViewMotion(viewKey, view);
  const fileOwner = useRef({ context: drafts.context, uploads: drafts.uploads, setUploads: drafts.setUploads, setText: setDraft });
  fileOwner.current = { context: drafts.context, uploads: drafts.uploads, setUploads: drafts.setUploads, setText: setDraft };
  const fileDirty = useCallback((key: string, value: boolean) => {
    setDirtyFiles(previous => { if (previous.has(key) === value) return previous; const next = new Set(previous); if (value) next.add(key); else next.delete(key); return next; });
    if (value) { atBottom.current = false; setAtLatest(false); }
  }, []);
  const useFile = useCallback((file: UploadedFile, key: string, prompt?: string) => {
    const owner = fileOwner.current;
    const original = JSON.parse(key) as { kind: string; id?: string };
    const previous = fileAttachments.current.get(`${owner.context}:${key}`);
    const position = owner.uploads.findIndex(value => value.id === previous || original.kind === "upload" && value.id === original.id);
    if (position >= 0) attachmentKeys.current.set(file.id, attachmentKeys.current.get(owner.uploads[position].id) ?? owner.uploads[position].id);
    const selected = [...owner.uploads]; if (position < 0) selected.push(file); else selected[position] = file;
    owner.uploads = selected; owner.setUploads(selected);
    fileAttachments.current.set(`${owner.context}:${key}`, file.id);
    if (prompt) { owner.setText(current => current ? `${current}\n\n${prompt}` : prompt); requestAnimationFrame(() => composer.current?.focus()); }
  }, []);
  const fileChat = useMemo(() => ({ context: drafts.context, dirtyKeys: dirtyFiles, attachmentKey, blocked: sending || queueAction.busy || !!error || !!state?.external || !!updatePreparing || !drafts.ready, useFile, dirty: fileDirty }), [drafts.context, sending, queueAction.busy, error, state?.external, updatePreparing, drafts.ready, useFile, fileDirty, dirtyFiles, attachmentKey]);

  const hasUnsavedView = memoryEditing.size > 0 || dirtyFiles.size > 0 || queueEditing || panel?.rows?.some((row) => row.input) || panel?.kind === "confirmation" || panel?.kind === "status-line";
  const navigationBusy = navigationPending || !!sessionSwitching || sending || queueAction.busy || memoryAction.busy || updateFeedback.busy || receiptInspection.busy || !!state?.receipt_busy || voiceBusy || attachments.busy;

  useEffect(() => { localStorage.setItem("gator:sidebar-collapsed", String(collapsed)); }, [collapsed]);
  useEffect(() => { if (sessionID) { if (originalSession.current && originalSession.current !== sessionID) setSessionChanged(true); originalSession.current = sessionID; } }, [sessionID]);
  useEffect(() => {
    if (state && (state.input ?? "") !== backendInput.current) {
      backendInput.current = state.input ?? "";
      if (state.input) { setDraft(state.input); composer.current?.focus(); }
    }
  }, [state?.input]);
  useEffect(() => {
    if (newChatFocus.current && newChatFocus.current !== drafts.context && drafts.ready && !panel && composer.current && !composer.current.disabled) {
      composer.current.focus(); newChatFocus.current = undefined;
    }
  }, [drafts.context, drafts.ready, panel, sending, navigationPending, sessionSwitching]);
  useLayoutEffect(() => {
    if (!state) return;
    if (panel) { returningToTranscript.current = true; return; }
    const content = JSON.stringify([state.messages, state.activity, state.chat_updates?.map(update => update.id)]);
    const changed = previousTranscript.current.identity !== conversationIdentity;
    if (changed) {
      const promoted = previousTranscript.current.identity === `${state.primary ? "primary" : "topic"}:${state.source}` && state.conversation_id && previousTranscript.current.running;
      const position = transcriptPositions.current.get(promoted ? previousTranscript.current.identity : conversationIdentity);
      atBottom.current = position?.latest ?? true; setAtLatest(atBottom.current); setNewActivity(false);
      if (!atBottom.current && position) transcript.current?.scrollTo({ top: position.top });
    } else if (returningToTranscript.current && !atBottom.current) {
      transcript.current?.scrollTo({ top: transcriptPositions.current.get(conversationIdentity)?.top ?? 0 });
    }
    if (!changed && previousTranscript.current.content !== content && !atBottom.current) setNewActivity(true);
    previousTranscript.current = { identity: conversationIdentity, content, running: state.running };
    returningToTranscript.current = false;
    if (atBottom.current && search === null) transcript.current?.scrollTo({ top: transcript.current.scrollHeight });
  }, [state?.messages, state?.activity, state?.running, state?.chat_updates, panel, conversationIdentity, search]);
  useEffect(() => { if (composer.current) { composer.current.style.height = "auto"; composer.current.style.height = `${Math.min(180, composer.current.scrollHeight)}px`; } }, [draft]);

  const attempt = useCallback((frame: DesktopFrame) => { void send(frame).catch(() => {}); }, [send]);
  const performNavigation = (target: Navigation) => {
    setNavigationPending(true); setSearch(null); setShowCommands(false); setDetails(false);
    void (async () => {
      if ("session" in target) { await window.gator!.openSession(target.session); return; }
      if ("conversation" in target) { await send({ type: "conversation", conversation: target.conversation }); return; }
      if (target.command === "/new") newChatFocus.current = drafts.context;
      await send({ type: "navigate", text: target.command });
      if (target.draft !== undefined) setDraft(target.draft);
    })().catch(error => setDraftError(String(error))).finally(() => setNavigationPending(false));
  };
  const requestNavigation = (target: Navigation) => {
    if (navigationBusy) return;
    if (hasUnsavedView) { setNavigation({ target, queued: false }); return; }
    performNavigation(target);
  };
  const navigate = (command: string, prefill?: string) => requestNavigation({ command, draft: prefill });
  const openRecent = (id: string) => {
    if (id === state?.conversation_id) { if (panel) navigate("/chat"); return; }
    if (state?.recent_id) requestNavigation({ conversation: { id, list_id: state.recent_id } });
  };
  useEffect(() => {
    const keyboard = (event: globalThis.KeyboardEvent) => {
      if (event.defaultPrevented || showDrafts || updatesOpen || updatePreparing) return;
      if (navigation && event.key !== "Escape") return;
      const bindings = state?.desktop?.shortcuts;
      if (shortcutMatches(event, "new_chat", bindings)) { event.preventDefault(); navigate("/new"); }
      else if (shortcutMatches(event, "command_search", bindings)) { event.preventDefault(); if (!showCommands) commandReturn.current = document.activeElement as HTMLElement | null; setShowCommands(current => !current); }
      else if (shortcutMatches(event, "conversation_search", bindings)) { event.preventDefault(); setSearch(current => current === null ? "" : null); }
      else if (shortcutMatches(event, "settings", bindings)) { event.preventDefault(); navigate("/model"); }
      else if (!panel && shortcutMatches(event, "help", bindings)) { event.preventDefault(); navigate("/help"); }
      else if (shortcutMatches(event, "voice_input", bindings)) { event.preventDefault(); window.dispatchEvent(new CustomEvent("gator:voice-input")); }
      if (event.key === "Escape") {
        if (navigation) setNavigation(null);
        else if (showCommands) setShowCommands(false);
        else if (search !== null) setSearch(null);
        else if (details) { closeDetails(); return; }
        else if (panel?.id) attempt({ type: "panel", key: "esc", panel: { id: panel.id, action: "key" } });
        else if (state?.running) attempt({ type: "key", key: "esc" });
        composer.current?.focus();
      }
    };
    window.addEventListener("keydown", keyboard);
    return () => window.removeEventListener("keydown", keyboard);
  });

  const approve = useCallback(async (approved: boolean) => {
    if (!state?.pending_approval || approvalBusy) return;
    setApprovalBusy(true);
    try { await send({ type: "approval", interaction_id: state.pending_approval.id, approval_token: state.pending_approval.token, approved }); }
    catch {} finally { setApprovalBusy(false); }
  }, [state?.pending_approval?.id, state?.pending_approval?.token, approvalBusy, send]);

  const retainedDraft = drafts.saved.drafts.find((item) => item.context === drafts.context);
  const unsavedComposer = (draft.trim() || drafts.uploads.length) && (!drafts.saved.enabled || retainedDraft?.text !== draft || JSON.stringify(retainedDraft?.uploads ?? []) !== JSON.stringify(drafts.uploads));
  const updateDialog = <Presence open={!!updatesOpen && !!updates}>{(open, presenceRef) => <UpdateDialog open={open} presenceRef={presenceRef} state={updates!} blocked={receiptInspection.busy || state?.receipt_busy ? "Wait for the receipt check to finish." : updateFeedback.busy ? "Wait for update feedback to finish saving." : memoryAction.busy ? "Wait for memory changes to finish saving." : attachments.busy ? "Wait for attachments to finish preparing." : drafts.saving ? "Wait for your message draft to finish saving." : drafts.issue ? "Save or copy your unsent message before restarting." : unsavedComposer ? "Your unsent message or attachments are not saved. Save or clear them before restarting." : hasUnsavedView ? "Save or close the form or queue editor before restarting." : allChatsBusy || allChatsQueued || state?.running || state?.queued || state?.pending_approval || state?.external || terminalBusy ? "Finish work and queued messages in all chats before restarting." : connection !== "connected" ? "Reconnect before restarting for an update." : ""} close={() => void window.gator?.updates({ action: "close" })} />}</Presence>;

  if (!state) return <><main className="boot-screen"><Logo /><h1>{reconnecting ? "Reconnecting to Gator…" : error ? "Gator could not connect" : "Getting your workspace ready"}</h1><p>{error || "Opening your local Gator profile. No messages will be submitted."}</p>{(!error || reconnecting) && <LoaderCircle className="spin accent" size={20} />}{error && window.gator && <button className="button" disabled={reconnecting} onClick={() => void reconnect()}>{reconnecting ? "Reconnecting…" : "Reconnect"}</button>}</main>{updateDialog}</>;

  const queueFull = state.running && state.queued >= 16 && !draft.trimStart().startsWith("/");
  const submit = async (guide = false) => {
    if (navigationPending || sessionSwitching || voiceBusy || modelBusy || dirtyFiles.size) return;
    if ((!draft.trim() && !drafts.uploads.length) || !drafts.ready || sending || attachments.busy || queueAction.busy || panel || error || state.external || (guide ? !state.guidance_id : queueFull)) return;
    if (drafts.replyToUpdateID && (guide || draft.trimStart().startsWith("/"))) { setDraftError("Send this update reply as a message or queued follow-up."); return; }
    if (drafts.uploads.length && (guide || draft.trimStart().startsWith("/"))) { setDraftError("Send attachments with a message or queued follow-up."); return; }
    if (!state.running && !state.provider && !draft.trimStart().startsWith("/")) { navigate("/model"); return; }
    setSending(true);
    setDraftError("");
    const files = drafts.uploads, context = drafts.context!;
    try {
      if (guide) await queueAction.act({ id: state.guidance_id!, action: "guide", text: draft });
      else if (draft.trimStart().startsWith("/") || draft.trim() === "exit") await send({ type: "submit", text: draft });
      else {
        const ids = files.map(file => file.id), key = JSON.stringify([context, draft, ids, drafts.replyToUpdateID]);
        if (admission.current?.key !== key) admission.current = { key, id: crypto.randomUUID() };
        await send({ type: "upload_submit", upload: { request_id: admission.current.id, context, text: draft, files: ids, reply_to_update_id: drafts.replyToUpdateID } });
        admission.current = null;
      }
      drafts.clearSent(context, files); setShowCommands(false); atBottom.current = true;
    } catch (err) { setDraftError(err instanceof Error ? err.message : String(err)); }
    finally { setSending(false); composer.current?.focus(); }
  };
  const composerKeys = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.nativeEvent.isComposing) return;
    if (shortcutMatches(event, "guide_work", state.desktop?.shortcuts) && state.running) { event.preventDefault(); void submit(true); return; }
    if (shortcutMatches(event, "send_message", state.desktop?.shortcuts)) { event.preventDefault(); void submit(); return; }
    if (shortcutMatches(event, "newline", state.desktop?.shortcuts)) {
      event.preventDefault(); const input = event.currentTarget;
      input.setRangeText("\n", input.selectionStart, input.selectionEnd, "end"); setDraft(input.value);
    }
  };
  const home = state.home && !state.messages.length && !state.chat_updates?.length && !panel;
  const fileCount = conversationBundles(state).reduce((count, bundle) => count + bundleOutputs(bundle).length, 0);
  const conversationBrowser = state.destination === "/conversations" || state.destination === "/history";
  const messages = state.messages;
  const jumpToLatest = () => { setSearch(null); atBottom.current = true; setAtLatest(true); setNewActivity(false); transcript.current?.scrollTo({ top: transcript.current.scrollHeight }); composer.current?.focus({ preventScroll: true }); };
  const slash = draft.startsWith("/") && !draft.includes(" ") ? commands.filter((command) => command.name.startsWith(draft.slice(1))).slice(0, 7) : [];
  const headerTitle = panel ? conversationBrowser ? state.destination === "/history" ? "Task runs" : "Conversations" : panel.title : state.title || (home ? "New chat" : "Gator");

  return <FileChatContext.Provider value={fileChat}><div className={`app-shell ${collapsed ? "sidebar-collapsed" : ""} ${details ? "details-open" : ""}`} data-conversation-id={state.conversation_id} data-theme={state.theme} data-platform={window.gator?.platform} data-fullscreen={fullScreen}>
    <div className="titlebar" aria-hidden="true"><span>Gator</span></div>
    <aside className="sidebar" aria-label="Main navigation">
      <div className="sidebar-brand"><button className="brand-button" onClick={() => navigate("/gator")} aria-label="Gator home"><Logo /><span>Gator</span></button><button className="icon-button collapse-button" onClick={() => setCollapsed(!collapsed)} aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}><IconSwap active={collapsed} size={17} on={<PanelLeftOpen size={17} />} off={<PanelLeftClose size={17} />} /></button></div>
      {settingsPage ? <SettingsNavigation destination={state.destination} collapsed={collapsed} navigate={navigate} about={() => void window.gator?.updates({ action: "open" })} /> : <>
      <button className="nav-button new-chat" aria-label="New chat" title={`New chat (${shortcutLabel("new_chat", state.desktop?.shortcuts)})`} onClick={() => navigate("/new")} disabled={navigationBusy || !!state.external || !!error}><Plus size={18}/><span>New chat</span><kbd>{shortcutLabel("new_chat",state.desktop?.shortcuts)}</kbd></button>
      <button className="nav-button primary-chat" aria-label="Talk with Gator" aria-current={!panel ? "page" : undefined} onClick={() => navigate("/chat")}><MessageSquare size={18} /><span>Talk with Gator</span></button>
      <div className="sidebar-scroll"><nav className="personal-navigation" aria-label="Your Gator"><button className="nav-button" aria-label="Following" aria-expanded={details && detailKind === "following"} aria-controls="library-panel" onClick={() => openDetails("following")} title={collapsed ? "Following" : undefined}><Target size={18} /><span>Following</span></button><button className="nav-button" aria-label="Saved" aria-expanded={details && detailKind === "saved"} aria-controls="library-panel" onClick={() => openDetails("saved")} title={collapsed ? "Saved" : undefined}><Bookmark size={18} /><span>Saved</span></button></nav>
      <RecentConversations state={state} activity={chatActivity} disabled={navigationBusy || !!error} open={openRecent} browse={() => navigate("/conversations")} />
      </div>
      <div className="sidebar-bottom"><button className="nav-button" aria-label="Settings" title={collapsed ? "Settings" : `Settings (${shortcutLabel("settings", state.desktop?.shortcuts)})`} onClick={() => navigate("/model")} disabled={navigationBusy}><Settings2 size={17} /><span>Settings</span><kbd>{shortcutLabel("settings", state.desktop?.shortcuts)}</kbd></button></div>
      </>}
    </aside>
    <main className="main-area">
      {chatActivity?.some(item => item.id !== state.conversation_id) && <nav className="chat-activity-strip" aria-label="Work in other chats">{chatActivity.filter(item => item.id !== state.conversation_id).map(item => <button key={item.id} className="small-button" disabled={navigationBusy} onClick={() => requestNavigation({ session: item.id })}><span>{item.title}</span><small>{item.status}</small></button>)}</nav>}
      {(sessionSwitching || sessionIssue) && <div className="session-notice" role="status">{sessionSwitching ? "Opening chat… Your other tasks keep working." : sessionIssue}</div>}
      <header className="main-header"><div className="breadcrumbs">{settingsPage && <><span>Settings</span><ChevronRight size={13} /></>}<strong title={headerTitle}>{headerTitle}</strong></div><div className="header-actions">{!panel && fileCount > 0 && <button className="small-button files-trigger" aria-label={`Files in this conversation (${fileCount})`} aria-controls="work-details-panel" aria-expanded={details && detailKind === "info" && filesOnly} onClick={openFiles}><Files size={16} /><span>Files</span><small>{fileCount}</small></button>}{!settingsPage && <button ref={detailsTrigger} className="icon-button" aria-controls="work-details-panel" aria-label={details && detailKind === "info" && !filesOnly ? "Hide conversation information" : "Conversation information"} aria-expanded={details && detailKind === "info" && !filesOnly} onClick={() => openDetails("info")}><Info size={18} /></button>}</div></header>
      {error && <div className="connection-error" role="alert"><div><strong>{connection === "disconnected" ? "Connection lost" : "Action could not complete"}</strong><p>{connection === "disconnected" ? "This view may be out of date. Reconnecting opens a new local session and does not resume or resubmit work." : error}</p>{connection === "disconnected" && <details><summary>Technical details</summary><p>{error}</p></details>}</div>{connection === "disconnected" ? <button className="button" disabled={reconnecting} aria-busy={reconnecting} onClick={() => void reconnect()}>{reconnecting ? "Reconnecting…" : "Reconnect"}</button> : <button className="icon-button" aria-label="Dismiss action error" onClick={clearError}><X size={16} /></button>}</div>}
      {sessionChanged && !error && <div className="session-notice" role="status"><span>A new local session is ready. Previous work was not resumed. Saved conversations and drafts remain available.</span><button className="icon-button" aria-label="Dismiss session notice" onClick={() => setSessionChanged(false)}><X size={16} /></button></div>}
      {state.notice && <div className="view-status" role="status">{state.notice}</div>}
      {panel && state.pending_approval && <div className="approval-reminder panel-approval"><ShieldCheck size={16} /><span>A current action needs your review.</span><button className="small-button" onClick={() => navigate("/chat")}>Review action<ChevronRight size={13} /></button></div>}
      {state.external && <div className="terminal-banner" role="status"><div><strong>Continue this step in Terminal</strong><p>Gator needs an interactive terminal for this {state.external.kind === "connector" ? "connection" : "action"}. Return here when it finishes.</p></div><button className="button" disabled={terminalBusy} onClick={() => void window.gator?.continueInTerminal().catch(() => {})}>{terminalBusy ? "Waiting for Terminal…" : "Open Terminal"}</button></div>}
      <div className="view-content" data-home={home} data-settings={settingsPage} ref={view}>
      {panel ? conversationBrowser ? <ConversationsPanel panel={panel} send={send} issue={state.recent_issue} /> : panel.kind === "usage" && panel.usage ? <UsagePanel panel={panel} usage={panel.usage} send={send} /> : <Panel panel={panel} send={send} /> : <>
        {search !== null && <div className="transcript-search" role="search" aria-label="Find in conversation"><Search size={16} /><input autoFocus aria-label="Search conversation" maxLength={256} value={search} onChange={(event) => setSearch(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.nativeEvent.isComposing) { event.preventDefault(); conversationSearch.move(event.shiftKey ? -1 : 1); } }} placeholder="Find in this conversation…" /><span className="search-count" role="status">{!search ? "Find text" : !conversationSearch.count ? "No matches" : `${conversationSearch.active + 1} of ${conversationSearch.count}${conversationSearch.capped ? "+" : ""}`}</span><button className="icon-button" aria-label="Previous match" title="Previous match (Shift+Enter)" disabled={!conversationSearch.count} onClick={() => conversationSearch.move(-1)}><ArrowUp size={16} /></button><button className="icon-button" aria-label="Next match" title="Next match (Enter)" disabled={!conversationSearch.count} onClick={() => conversationSearch.move(1)}><ArrowDown size={16} /></button><button className="icon-button" aria-label="Close search" onClick={() => { setSearch(null); composer.current?.focus(); }}><X size={16} /></button></div>}
        <div className={`transcript-stage ${home ? "home-stage" : ""}`}><div className="conversation-atmosphere" aria-hidden="true"><AsciiWaves /></div><div className={`conversation-scroll ${home ? "home-scroll" : ""}`} ref={transcript} onScroll={(event) => { const el = event.currentTarget; const near = el.scrollHeight - el.scrollTop - el.clientHeight < 100; atBottom.current = near; setAtLatest(near); if (near) setNewActivity(false); transcriptPositions.current.delete(conversationIdentity); transcriptPositions.current.set(conversationIdentity, { top: el.scrollTop, latest: near }); if (transcriptPositions.current.size > 32) transcriptPositions.current.delete(transcriptPositions.current.keys().next().value!); }}>
          {home ? <><div className={`home-content${!state.provider ? " home-onboarding" : ""}`}><div className="home-art"><Logo /></div><h1>Hi. I’m Gator.</h1><p>Bring a question, a plan, or something you’d like a hand with.</p>
            {!state.provider && <FirstTaskSetup hasDraft={!!draft.trim()} hasFiles={drafts.uploads.length > 0} disabled={navigationBusy || !drafts.ready || attachments.busy} suggest={text => { if (!draft.trim()) setDraft(text); composer.current?.focus(); }} attach={() => void attachments.choose()} models={() => navigate("/model")} connections={() => navigate("/connections")} />}
            </div></> : <div className="transcript-content" aria-label="Conversation">{chatTimeline(messages, state.chat_updates ?? []).map(item => item.kind === "update" ? <ChatUpdateCard feedback={updateFeedback.act} feedbackBusy={updateFeedback.busy} key={item.update.id} update={item.update} conversation={state.conversation_id!} send={send} disabled={!!error || !!sessionSwitching || navigationPending || sending || !drafts.ready} reply={update => { drafts.setReplyToUpdateID(update.id); composer.current?.focus(); }} /> : (({ message, index }) => <Message key={`${conversationIdentity}:${index}`} message={message} inspectReceipt={receiptInspection.inspect} receiptBusy={receiptInspection.busy || !!state.receipt_busy} receiptDisabled={!!error || !!sessionSwitching || navigationPending || sending || !drafts.ready || !!updatePreparing} memories={state.chat_memories?.filter(memory=>message.run_id && memory.run_id===message.run_id)} conversation={state.conversation_id} memoryBusy={memoryAction.busy || !!error || !!sessionSwitching} changeMemory={memoryAction.act} onMemoryEditingChange={onMemoryEditingChange} shortcuts={state.desktop?.shortcuts} preview={openOutput} pending={!approvalBusy && state.pending_approval?.message_index === index ? state.pending_approval.id : undefined} onApproval={approve} commands={message.result?.title === "Gator commands" ? commands : undefined} navigate={message.result?.title === "Gator commands" ? navigate : undefined} disabled={message.result?.title === "Gator commands" ? state.running || !!error : undefined} />)(item))}{state.chat_memory_issue && <p role="status" className="chat-memory-scope">{state.chat_memory_issue}</p>}{state.running && <LiveActivity trace={state.activity} status={state.cancelling ? "Stopping…" : state.pending_approval ? "Waiting for your approval" : state.status || "Gator is working…"} elapsed={state.elapsed} attention={state.cancelling || !!state.pending_approval} />}</div>}
        </div>{!home && !atLatest && <button className="jump-latest" aria-label={newActivity ? "New activity. Jump to latest" : "Jump to latest"} onClick={jumpToLatest}><ArrowDown size={16} />{newActivity && <span>New activity</span>}</button>}</div>
        <div className="composer-area" key={conversationIdentity}><RecoveredRequests state={state} inspect={receiptInspection.inspect} busy={receiptInspection.busy || !!state.receipt_busy} disabled={navigationBusy || !!error} />{!!(state.chat_checks?.length || state.chat_checks_issue) && <ChatCheckHealth key={conversationIdentity} state={state} disabled={navigationBusy || !!error} manage={() => navigate("/followup")} />}{state.pending_approval && <div className="approval-reminder"><ShieldCheck size={16} /><span>An action is waiting for your review.</span><button className="small-button" onClick={jumpToLatest}>View request<ArrowDown size={13} /></button></div>}
          {!!slash.length && <div className="slash-suggestions" aria-label="Command suggestions">{slash.map((command) => <button key={command.name} onClick={() => { setDraft(`/${command.name}${command.needs_input ? " " : ""}`); composer.current?.focus(); }}><span>/{command.name}</span><span>{command.description}</span></button>)}</div>}
          {state.chat_updates_issue && <p className="composer-error" role="alert">{state.chat_updates_issue}</p>}
          {(state.updates_have_older || state.updates_have_newer) && <nav className="update-pages" aria-label="Follow-up update pages"><button className="small-button" disabled={!state.updates_have_older || !!sessionSwitching} onClick={() => attempt({ type: "update", update: { conversation_id: state.conversation_id!, id: state.chat_updates![0].id, action: "older", version: 0 } })}>Earlier updates</button><span>{state.chat_updates?.length} of {state.chat_updates_total} updates</span><button className="small-button" disabled={!state.updates_have_newer || !!sessionSwitching} onClick={() => attempt({ type: "update", update: { conversation_id: state.conversation_id!, id: state.chat_updates!.at(-1)!.id, action: "newer", version: 0 } })}>Newer updates</button></nav>}
          {drafts.replyToUpdateID && <div className="update-reply-chip" role="status"><span>Replying to <strong>{state.chat_updates?.find(update => update.id === drafts.replyToUpdateID)?.title || "a saved follow-up"}</strong></span><button className="icon-button" aria-label="Cancel update reply" disabled={sending || !drafts.ready} onClick={() => drafts.setReplyToUpdateID(undefined)}><X size={15} /></button></div>}
          <MessageQueue key={conversationIdentity} recovered={state.requests_recovered} issue={state.requests_issue} items={state.queue ?? []} running={state.running} busy={queueAction.busy || sending || !!error || !!state.external} act={queueAction.act} onEditingChange={setQueueEditing} onUseDraft={(text, replyToUpdateID) => { drafts.setReplyToUpdateID(replyToUpdateID); setDraft((current) => current ? `${current}\n\n${text}` : text); composer.current?.focus(); }} />
          {(drafts.recovered || drafts.issue) && <p className="draft-recovery" role="status">{drafts.issue || "Draft recovered. Review it before sending."}</p>}
          <div className="composer" data-file-drop={attachments.dragging} data-voice-busy={voiceBusy} {...attachments.drop}><PendingAttachments files={drafts.uploads} disabled={sending || attachments.busy} remove={(id) => { drafts.setUploads(drafts.uploads.filter((file) => file.id !== id)); setDraftError(""); composer.current?.focus(); }} /><textarea ref={composer} value={draft} onChange={(event) => { setDraft(event.target.value); setDraftError(""); }} onKeyDown={composerKeys} onPaste={event => attachments.paste(event, value => { setDraft(value); setDraftError(""); })} placeholder={state.running ? "Add a follow-up…" : "Message Gator…"} aria-label="Message Gator" disabled={navigationPending || sessionSwitching || sending || !drafts.ready || !!state.external} rows={2} /><div className="composer-toolbar"><AttachButton busy={attachments.busy} disabled={sending || queueAction.busy || !!error || !!state.external || !!updatePreparing || !drafts.ready || drafts.uploads.length >= 4} choose={() => void attachments.choose().then(() => composer.current?.focus())} /><button className="icon-button composer-options" aria-label="Conversation options" title="Conversation options" onClick={() => openDetails("info")}><SlidersHorizontal size={17} /></button><button className="icon-button draft-trigger" aria-label="Saved drafts" title={drafts.saving ? "Saving draft…" : "Saved drafts and local retention"} onClick={() => { setShowDrafts(true); void drafts.manage({ action: "list" }).catch(() => {}); }}><Clock3 size={16} /></button><div className="composer-actions"><ModelPicker state={state} disabled={sending || voiceBusy || queueAction.busy || !!error || !!state.external || !!updatePreparing || !drafts.ready} send={send} manage={() => navigate("/model")} onBusyChange={setModelBusy} /><VoiceControls context={drafts.context} speakingContext={state.primary ? `primary:${state.source}` : conversationIdentity} disabled={sending || queueAction.busy || !!error || !!state.external || !!updatePreparing || !drafts.ready || showCommands || showDrafts || !!updatesOpen || !!navigation} preferences={state.desktop?.voice} messages={state.messages} running={state.running} shortcuts={state.desktop?.shortcuts} onBusyChange={setVoiceBusy} onTranscript={text => { setDraft(current => current ? `${current}${/\s$/.test(current) ? "" : " "}${text}` : text); setDraftError(""); requestAnimationFrame(() => composer.current?.focus()); }} />
            {state.running && <button className="small-button guide-button" title={drafts.uploads.length ? "Files will be sent with a queued follow-up" : `Guide the current task (${shortcutLabel("guide_work", state.desktop?.shortcuts)})`} disabled={!draft.trim() || !!drafts.replyToUpdateID || !!drafts.uploads.length || sending || modelBusy || dirtyFiles.size > 0 || attachments.busy || queueAction.busy || !state.guidance_id || !!error || !!state.external} onClick={() => void submit(true)}>Guide now</button>}
            {state.running && <button className="send-button stop" aria-label="Stop current work" disabled={state.cancelling} onClick={() => attempt({ type: "key", key: "esc" })}><Square size={13} fill="currentColor" /></button>}
            <button className="send-button" aria-label={state.running ? "Queue message" : "Send message"} title={queueFull ? "Queue is full (16 messages)" : state.running ? "Queue for after this task (Enter)" : "Send message"} aria-busy={sending} disabled={(!draft.trim() && !drafts.uploads.length) || !drafts.ready || sending || modelBusy || voiceBusy || dirtyFiles.size > 0 || attachments.busy || queueAction.busy || queueFull || !!error || !!state.external} onClick={() => void submit()}><IconSwap active={sending} on={<LoaderCircle size={18} className="spin" />} off={state.running ? <ListPlus size={18} /> : <ArrowUp size={18} />} /></button>
          </div></div></div>
          {attachments.dragging && <p className="attachment-status" role="status">Drop files to attach to this message</p>}
          {(draftError || attachments.issue) && <p className="composer-error" role="alert">{draftError || attachments.issue}</p>}
          {queueFull && <p className="composer-error" role="status">Queue full. Edit or remove a message to make room.</p>}
        </div>
      </>}
      </div>
    </main>
    <Presence open={details && detailKind === "info"}>{(open, presenceRef) => <WorkDetails pendingFiles={drafts.uploads.map((file) => file.name)} filesOnly={filesOnly} showInChat={showOutputInChat} state={state} identity={conversationIdentity} requestedOutput={requestedOutput} open={open} presenceRef={presenceRef} close={closeDetails} navigate={navigate} />}</Presence>
    <Presence open={details && detailKind !== "info"}>{(open, presenceRef) => <LibraryDrawer kind={libraryKind.current} state={state} open={open} presenceRef={presenceRef} close={closeDetails} navigate={navigate} preview={openOutput} drafts={() => { closeDetails(); setShowDrafts(true); void drafts.manage({ action: "list" }).catch(() => {}); }} />}</Presence>
    {updateDialog}
    {panel && <div className="panel-draft-anchor"><MessageSquare size={14} /><span>{draft ? "Your message draft is kept while you’re here." : "Return to the conversation to write a message."}</span><button className="small-button" onClick={() => attempt({ type: "panel", key: "esc", panel: { id: panel.id!, action: "key" } })}>Back<kbd>Esc</kbd></button></div>}
    <Presence open={showCommands}>{(open, presenceRef) => <CommandMenu open={open} presenceRef={presenceRef} commands={commands} close={() => setShowCommands(false)} select={(name, needsInput) => {
      setShowCommands(false);
      if (name === "@following" || name === "@saved") { drawerReturn.current = commandReturn.current?.isConnected ? commandReturn.current : composer.current; setRequestedOutput(undefined); setDetailKind(name === "@following" ? "following" : "saved"); setDetails(true); }
      else if (name === "@gator") navigate("/chat");
      else if (needsInput) navigate("/chat", `/${name} `);
      else navigate(`/${name}`);
    }} />}</Presence>
    <Presence open={showDrafts}>{(open, presenceRef) => <DraftManager open={open} presenceRef={presenceRef} saved={drafts.saved} issue={drafts.issue} manage={drafts.manage} close={() => setShowDrafts(false)} restore={(retained) => { if (retained.reply_to_update_id && retained.context !== drafts.context) { setDraftError("Open this reply’s original chat to restore it."); return; } if (retained.reply_to_update_id) drafts.setReplyToUpdateID(retained.reply_to_update_id); try { drafts.setUploads([...new Map([...drafts.uploads, ...(retained.uploads ?? [])].map((file) => [file.id, file])).values()]); setDraft((current) => !current ? retained.text : current === retained.text ? current : `${current}\n\n${retained.text}`); } catch (error) { setDraftError(error instanceof Error ? error.message : "Couldn’t restore attachments."); } composer.current?.focus(); }} />}</Presence>
    <Presence open={!!navigation}>{(open, presenceRef) => <DiscardDialog open={open} presenceRef={presenceRef} queued={navigation?.queued ?? false} fileEdits={dirtyFiles.size > 0} cancel={() => setNavigation(null)} confirm={() => { if (!navigation) return; performNavigation(navigation.target); setNavigation(null); }} />}</Presence>
  </div></FileChatContext.Provider>;
}

function CommandMenu({ commands, open, presenceRef, close, select }: { commands: BackendState["commands"]; open: boolean; presenceRef: PresenceRef; close(): void; select(name: string, needsInput: boolean): void }) {
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);
  const focus = useModalFocus(open);
  const choices: (BackendState["commands"][number] & { label?: string })[] = [
    { name: "@gator", label: "Talk with Gator", description: "Return to the current chat", advanced: false, needs_input: false },
    { name: "@following", label: "Following", description: "Ongoing goals, configured checks and updates", advanced: false, needs_input: false },
    { name: "@saved", label: "Saved", description: "Knowledge, files, conversations and task runs", advanced: false, needs_input: false },
    ...commands.map(command => command.name === "new" ? { ...command, label: "New chat" } : command),
  ];
  const search = query.trim().replace(/^\/+/, "").toLowerCase();
  const shown = choices.filter((command) => `${command.name} ${command.label || ""} ${command.description}`.toLowerCase().includes(search));
  return <div ref={presenceRef} className="modal-backdrop" data-open={open} inert={!open} aria-hidden={!open} onClick={close}><section ref={focus} className="command-menu" role="dialog" aria-modal="true" aria-label="Search commands" onClick={(event) => event.stopPropagation()}><div className="command-search"><Search size={18} /><input autoFocus placeholder="Find a command…" aria-label="Find a command" value={query} onChange={(event) => { setQuery(event.target.value); setSelected(0); }} onKeyDown={(event) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); const next = Math.max(0, Math.min(shown.length - 1, selected + (event.key === "ArrowDown" ? 1 : -1))); setSelected(next); focus.current?.querySelectorAll(".command-list button")[next]?.scrollIntoView({ block: "nearest" }); }
    if (event.key === "Enter" && shown[selected]) { event.preventDefault(); select(shown[selected].name, shown[selected].needs_input); }
  }} /><button className="icon-button" aria-label="Close commands" onClick={close}><X size={16} /></button></div><div className="command-list">{shown.map((command, index) => <button key={command.name} data-selected={index === selected} onClick={() => select(command.name, command.needs_input)}><div><strong>{command.label || `/${command.name}`}</strong><p>{command.description}</p></div>{command.advanced && <span className="badge">Advanced</span>}<ChevronRight size={15} /></button>)}{!shown.length && <p className="empty-hint">No commands match “{query}”.</p>}</div><footer><span>↑↓ choose · Enter open · Tab move focus</span><kbd>Esc</kbd></footer></section></div>;
}

function DiscardDialog({ open, presenceRef, queued, fileEdits, cancel, confirm }: { open: boolean; presenceRef: PresenceRef; queued: boolean; fileEdits: boolean; cancel(): void; confirm(): void }) {
  const focus = useModalFocus(open);
  return <div ref={presenceRef} className="modal-backdrop" data-open={open} inert={!open} aria-hidden={!open}><section ref={focus} role="alertdialog" aria-modal="true" aria-labelledby="discard-title" className="confirm-dialog"><h2 id="discard-title">{queued ? "Leave queued messages?" : "Leave this edit?"}</h2><p>{queued ? fileEdits ? "Opening another conversation discards queued messages. Your file edits are retained as drafts; reopen the file when you return." : "Queued messages belong to this session. Opening another conversation will discard them and any unsaved edits. Your composer drafts remain saved if local retention is enabled." : fileEdits ? "Your file edits are retained as drafts. Save them before sending the file to Gator, or reopen the file when you return." : "Your unsaved changes will be discarded."}</p><div className="flex justify-end gap-2"><button className="button" onClick={cancel}>{queued ? "Keep working" : "Keep editing"}</button><button className="button primary" onClick={confirm}>{queued ? "Leave conversation" : "Leave view"}</button></div></section></div>;
}
