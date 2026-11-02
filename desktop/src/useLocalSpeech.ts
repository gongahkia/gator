import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { VoicePreferences } from "./bridge";

export function spokenText(text: string) {
  return text.replace(/```[\s\S]*?```/g, " Code block omitted. ").replace(/!\[[^\]]*\]\([^)]*\)/g, "").replace(/\[([^\]]*)\]\([^)]*\)/g, "$1").replace(/[`#*>_]/g, " ").trim().slice(0, 12000);
}

// One controller serves explicit message speech and optional spoken replies.
// Only installed local OS voices are used; no text is sent to a voice API.
export function useLocalSpeech(context: string, preferences: VoicePreferences | undefined, disabled: boolean) {
  const [voices, setVoices] = useState<SpeechSynthesisVoice[]>([]);
  const [active, setActive] = useState<string>();
  const [issue, setIssue] = useState("");
  const [issueFor,setIssueFor]=useState<string>();
  const refresh=useCallback(()=>setVoices((window.speechSynthesis?.getVoices()??[]).filter(voice=>voice.localService).slice(0,128)),[]);
  const epoch = useRef(0);
  const stop = useCallback(() => { epoch.current++; window.speechSynthesis?.cancel(); setActive(undefined); }, []);
  useEffect(() => {
    const synth=window.speechSynthesis;
    const update=()=>setVoices((synth?.getVoices()??[]).filter(voice=>voice.localService).slice(0,128));
    update(); synth?.addEventListener?.("voiceschanged",update);
    return()=>synth?.removeEventListener?.("voiceschanged",update);
  },[]);
  useEffect(()=>{stop();setIssue("");return stop;},[context,stop]);
  useEffect(()=>{if(disabled)stop();},[disabled,stop]);
  const speak = useCallback((id: string,text: string) => {
    stop(); setIssue("");setIssueFor(id);
    if(disabled)return;
    const voice=voices.find(voice=>preferences?.system_voice && (voice.voiceURI||voice.name)===preferences.system_voice) ?? voices.find(voice=>preferences?.language && voice.lang.toLowerCase().startsWith(preferences.language.toLowerCase().split("-")[0])) ?? voices[0];
    const synth=window.speechSynthesis;
    if(!synth || !voice){setIssue("No local speech voice is available. Install a system voice to read replies aloud.");return;}
    const token=++epoch.current;
    const utterance=new SpeechSynthesisUtterance(spokenText(text));
    utterance.voice=voice; utterance.lang=preferences?.system_voice?voice.lang:preferences?.language||voice.lang;utterance.rate=preferences?.rate||1;
    setActive(id);
    utterance.onend=()=>{if(token===epoch.current)setActive(undefined);};
    utterance.onerror=event=>{if(token===epoch.current){setActive(undefined);if(!["canceled","interrupted"].includes(event.error))setIssue("The system voice could not read this reply.");}};
    try {synth.speak(utterance);}catch{if(token===epoch.current){setActive(undefined);setIssue("The system voice could not read this reply.");}}
  },[stop,disabled,voices,preferences?.system_voice,preferences?.language,preferences?.rate]);
  return useMemo(()=>({voices,active,issue,issueFor,available:voices.length>0,speak,stop,refresh}),[voices,active,issue,issueFor,speak,stop,refresh]);
}
export type LocalSpeech = ReturnType<typeof useLocalSpeech>;
