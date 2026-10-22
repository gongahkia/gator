import { Mic, Square, Volume2, VolumeX, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { VoicePreferences, VoiceStatus } from "./bridge";
import type { BackendMessage } from "./protocol";
import { shortcutLabel } from "./shortcuts";
import { DictationWaveform } from "./DictationWaveform";

export function spokenText(text: string) {
  return text.replace(/```[\s\S]*?```/g, " Code block omitted. ").replace(/!\[[^\]]*\]\([^)]*\)/g, "").replace(/\[([^\]]*)\]\([^)]*\)/g, "$1").replace(/[`#*>_]/g, " ").trim().slice(0, 12000);
}

type Phase = "idle" | "authorizing" | "recording" | "transcribing";
type Props = { context?: string; speakingContext: string; disabled: boolean; preferences?: VoicePreferences; messages: BackendMessage[]; running: boolean; shortcuts?: Record<string, string>; onTranscript(text: string): void; onBusyChange(busy: boolean): void };

export function VoiceControls(props: Props) {
  const [open, setOpen] = useState(false);
  const [phase, setPhase] = useState<Phase>("idle");
  const [status, setStatus] = useState<VoiceStatus>();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [seconds, setSeconds] = useState(0);
  const [issue, setIssue] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [speaking, setSpeaking] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const epoch = useRef(0), speechEpoch = useRef(0);
  const owner = useRef(props); owner.current = props;
  const recorder = useRef<MediaRecorder | undefined>(undefined);
  const stream = useRef<MediaStream | undefined>(undefined);
  const timers = useRef<{ interval?: ReturnType<typeof setInterval>; limit?: ReturnType<typeof setTimeout> }>({});
  const history = useRef({ context: props.speakingContext, count: props.messages.length });
  const preferences: VoicePreferences = props.preferences ?? status?.preferences ?? { spoken_replies: false };
  const focusTrigger = () => requestAnimationFrame(() => root.current?.querySelector<HTMLButtonElement>(".voice-trigger")?.focus({ preventScroll: true }));

  const stopSpeaking = useCallback(() => { speechEpoch.current++; window.speechSynthesis?.cancel(); setSpeaking(false); }, []);
  const stopTracks = useCallback(() => {
    clearInterval(timers.current.interval); clearTimeout(timers.current.limit); timers.current = {};
    stream.current?.getTracks().forEach(track => track.stop()); stream.current = undefined;
  }, []);
  const cancel = useCallback(() => {
    epoch.current++; const current = recorder.current; recorder.current = undefined;
    if (current && current.state !== "inactive") current.stop();
    stopTracks(); setPhase("idle"); setSeconds(0); owner.current.onBusyChange(false);
    void window.gator?.voice({ action: "cancel" }).catch(() => {});
  }, [stopTracks]);
  useEffect(() => { props.onBusyChange(phase !== "idle"); }, [phase, props.onBusyChange]);
  useEffect(() => () => { cancel(); stopSpeaking(); }, [props.context, cancel, stopSpeaking]);
  useEffect(() => { if (props.disabled && phase !== "idle") cancel(); }, [props.disabled, phase, cancel]);
  useEffect(() => {
    const shortcut = () => { if (owner.current.disabled) return; if (recorder.current?.state === "recording") recorder.current.stop(); else if (phase === "idle") setOpen(true); };
    window.addEventListener("gator:voice-input", shortcut);
    return () => window.removeEventListener("gator:voice-input", shortcut);
  }, [phase]);
  useEffect(() => {
    if (phase !== "idle") root.current?.querySelector<HTMLButtonElement>(phase === "recording" ? ".dictation-finish" : ".dictation-cancel")?.focus({ preventScroll: true });
  }, [phase]);

  const refresh = useCallback(async () => {
    setLoading(true);
    try { setStatus(await window.gator!.voice({ action: "status" })); }
    catch (error) { setIssue(error instanceof Error ? error.message : "Voice settings could not load."); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { if (open) void refresh(); }, [open, props.preferences?.provider, props.preferences?.model, props.preferences?.language, refresh]);
  useEffect(() => {
    if (!open && phase === "idle") return;
    const dismiss = (event: globalThis.KeyboardEvent) => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); cancel(); setOpen(false); focusTrigger(); } };
    const outside = (event: PointerEvent) => { if (phase === "idle" && !root.current?.contains(event.target as Node)) setOpen(false); };
    window.addEventListener("keydown", dismiss, true); window.addEventListener("pointerdown", outside);
    return () => { window.removeEventListener("keydown", dismiss, true); window.removeEventListener("pointerdown", outside); };
  }, [open, phase, cancel]);

  const configure = async (changes: Partial<VoicePreferences>) => {
    if (saving) return;
    setSaving(true); setIssue("");
    try { const result = await window.gator!.voice({ action: "configure", preferences: changes }); setStatus(result); if (changes.spoken_replies === false) stopSpeaking(); }
    catch (error) { setIssue(error instanceof Error ? error.message : "Voice settings could not be saved."); setOpen(true); }
    finally { setSaving(false); }
  };

  const start = async () => {
    const original = owner.current;
    if (!original.context || original.disabled || phase !== "idle") return;
    const token = ++epoch.current;
    setIssue(""); setAnnouncement(""); setOpen(false); setPhase("authorizing"); stopSpeaking();
    try {
      if (!navigator.mediaDevices?.getUserMedia || !globalThis.MediaRecorder) throw new Error("Audio recording is unavailable on this system.");
      const grant = await window.gator!.voice({ action: "authorize", context: original.context });
      if (token !== epoch.current) return;
      if (!grant.allowed) throw new Error("Microphone permission was not granted.");
      const captured = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true }, video: false });
      if (token !== epoch.current || original.context !== owner.current.context) { captured.getTracks().forEach(track => track.stop()); return; }
      stream.current = captured;
      const mime = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"].find(type => MediaRecorder.isTypeSupported(type));
      const current = new MediaRecorder(captured, mime ? { mimeType: mime } : undefined);
      recorder.current = current;
      const chunks: Blob[] = []; let size = 0;
      current.ondataavailable = event => {
        if (token !== epoch.current || !event.data.size) return;
        size += event.data.size;
        if (size > grant.max_audio_bytes) { cancel(); setIssue("The recording reached 8 MiB. Try a shorter clip."); setOpen(true); return; }
        chunks.push(event.data);
      };
      current.onerror = () => { if (token === epoch.current) { cancel(); setIssue("The microphone recording failed. Your draft is kept."); setOpen(true); } };
      current.onstop = async () => {
        if (token !== epoch.current) return;
        stopTracks(); recorder.current = undefined;
        setPhase("transcribing");
        try {
          const clip = new Blob(chunks, { type: current.mimeType || mime || "audio/webm" });
          if (!clip.size) throw new Error("No audio was recorded. Try again.");
          const data = new Uint8Array(await clip.arrayBuffer());
          if (token !== epoch.current) return;
          const result = await window.gator!.voice({ action: "transcribe", context: original.context!, media_type: clip.type, data });
          if (token !== epoch.current || original.context !== owner.current.context) return;
          if (!result.text?.trim()) throw new Error("No speech was recognized. Try again.");
          owner.current.onTranscript(result.text); setAnnouncement("Transcript added to your draft. Review it before sending."); setOpen(false);
        } catch (error) { if (token === epoch.current) { setIssue(error instanceof Error ? error.message : "Transcription failed. Your draft is kept."); setOpen(true); } }
        finally { if (token === epoch.current) { setPhase("idle"); void window.gator?.voice({ action: "release" }).catch(() => {}); } }
      };
      current.start(250); setSeconds(0); setPhase("recording");
      timers.current.interval = setInterval(() => setSeconds(value => value + 1), 1000);
      timers.current.limit = setTimeout(() => { if (current.state === "recording") current.stop(); }, grant.max_seconds * 1000);
    } catch (error) {
      if (token === epoch.current) { cancel(); setIssue(error instanceof Error ? error.message : "Could not open the microphone."); setOpen(true); }
    }
  };

  useEffect(() => {
    window.speechSynthesis?.getVoices();
    if (history.current.context !== props.speakingContext) { history.current = { context: props.speakingContext, count: props.messages.length }; stopSpeaking(); return; }
    if (!preferences.spoken_replies) { stopSpeaking(); if (!props.running) history.current.count = props.messages.length; return; }
    if (props.running || phase !== "idle" || props.disabled) return;
    const fresh = props.messages.slice(history.current.count).filter(message => message.role === "Gator" && !message.result && message.text.trim());
    history.current.count = props.messages.length;
    if (!fresh.length) return;
    const synth = window.speechSynthesis;
    const voices = synth?.getVoices().filter(voice => voice.localService) ?? [];
    const voice = voices.find(voice => preferences.language && voice.lang.toLowerCase().startsWith(preferences.language.toLowerCase().split("-")[0])) ?? voices[0];
    if (!synth || !voice) { setIssue("No local speech voice is available. Dictation still works; install a system voice to hear replies."); return; }
    stopSpeaking(); const token = ++speechEpoch.current;
    const utterance = new SpeechSynthesisUtterance(spokenText(fresh.map(message => message.text).join("\n\n")));
    utterance.voice = voice; utterance.lang = preferences.language || voice.lang; utterance.rate = preferences.rate || 1;
    utterance.onstart = () => { if (token === speechEpoch.current) setSpeaking(true); };
    utterance.onend = () => { if (token === speechEpoch.current) setSpeaking(false); };
    utterance.onerror = event => { if (token === speechEpoch.current) { setSpeaking(false); if (!["canceled", "interrupted"].includes(event.error)) setIssue("The system voice could not read this reply."); } };
    synth.speak(utterance);
  }, [props.messages, props.running, props.speakingContext, props.disabled, preferences.spoken_replies, preferences.language, preferences.rate, phase, stopSpeaking]);

  return <div className="voice-controls" ref={root} data-active={phase !== "idle"}>
    {phase === "idle" ? <button className="icon-button voice-trigger" aria-label="Voice dictation" aria-expanded={open} aria-controls="voice-controls-panel" title={`Voice dictation (${shortcutLabel("voice_input", props.shortcuts)})`} disabled={props.disabled} onClick={() => setOpen(value => !value)}><Mic size={17} /></button> : <div className="dictation-pill" role="group" aria-label="Voice recording" data-phase={phase}>
      <button className="icon-button dictation-cancel" aria-label={phase === "transcribing" ? "Cancel transcription" : "Cancel recording"} title="Cancel dictation (Esc)" onClick={() => { cancel(); setOpen(false); focusTrigger(); }}><X size={16} /></button>
      {phase === "recording" ? <><span className="sr-only" role="status">Recording</span><DictationWaveform stream={stream.current} /><time className="dictation-time" aria-hidden="true">{Math.floor(seconds / 60)}:{String(seconds % 60).padStart(2, "0")}</time><button className="icon-button dictation-finish" aria-label="Stop & transcribe" title="Stop recording and add transcript to draft" onClick={() => { if (recorder.current?.state === "recording") recorder.current.stop(); }}><Square size={13} fill="currentColor" /></button></> : <span className="dictation-progress" role="status">{phase === "transcribing" ? "Transcribing…" : "Opening mic…"}</span>}
    </div>}
    {phase === "idle" && <button className="icon-button" aria-label={speaking ? "Stop speaking" : preferences.spoken_replies ? "Disable spoken replies" : "Enable spoken replies"} aria-pressed={preferences.spoken_replies} title={speaking ? "Stop speaking" : "Spoken replies"} disabled={saving || props.disabled} onClick={() => speaking ? stopSpeaking() : void configure({ spoken_replies: !preferences.spoken_replies })}>{speaking ? <Square size={16} /> : preferences.spoken_replies ? <Volume2 size={17} /> : <VolumeX size={17} />}</button>}
    <span className="sr-only" role="status">{announcement}</span>
    {open && phase === "idle" && <section id="voice-controls-panel" className="voice-panel" role="dialog" aria-label="Voice dictation">
      <header><strong>Voice</strong><button className="icon-button" aria-label="Close voice controls" onClick={() => { cancel(); setOpen(false); focusTrigger(); }}><X size={16} /></button></header>
      <p>{status?.provider === "gemini" ? "Audio is sent to Gemini" : status?.provider === "openai" ? "Audio is sent to OpenAI" : "Choose a dictation provider"} for transcription. Review the transcript before sending.</p>
      {status?.model && <small>{status.model} · up to 2 minutes · audio is not saved</small>}
      <label>Dictation provider<select aria-label="Dictation provider" value={status?.preferences.provider || ""} disabled={saving || phase !== "idle"} onChange={event => void configure({ provider: event.target.value, model: "" })}><option value="">Use chat provider</option><option value="gemini">Gemini</option><option value="openai">OpenAI</option></select></label>
      <label>Language<select aria-label="Dictation language" value={status?.preferences.language || ""} disabled={saving || phase !== "idle"} onChange={event => void configure({ language: event.target.value })}><option value="">Automatic</option><option value="en">English</option><option value="en-SG">English (Singapore)</option><option value="zh">Chinese</option><option value="ms">Malay</option><option value="ta">Tamil</option>{status?.preferences.language && !["en", "en-SG", "zh", "ms", "ta"].includes(status.preferences.language) && <option value={status.preferences.language}>{status.preferences.language}</option>}</select></label>
      <label className="voice-toggle"><input type="checkbox" checked={preferences.spoken_replies} disabled={saving} onChange={event => void configure({ spoken_replies: event.target.checked })} />Read new replies aloud using a system voice</label>
      {(issue || status?.issue) && <p className="voice-issue" role="alert">{issue || status?.issue}</p>}
      <footer><button className="button" disabled={props.disabled || loading || saving || !status?.ready} onClick={() => void start()}><Mic size={16} />{loading ? "Loading…" : "Start recording"}</button></footer>
    </section>}
  </div>;
}
