import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { BackendState } from "./protocol";
import type { DraftRequest, DraftSnapshot, SavedDraft } from "./bridge";

export function useDrafts(state?: BackendState) {
  const context = state ? state.conversation_id ? `conversation:${state.conversation_id}` : `workspace:${state.primary === false ? "topic:" : ""}${state.source}` : undefined;
  const identity = useRef<Omit<SavedDraft, "text" | "updated"> | undefined>(undefined);
  const values = useRef(new Map<string, string>());
  const current = useRef("");
  const generation = useRef(0);
  const previousRunning = useRef(false);
  const [text, renderText] = useState("");
  const [saved, setSaved] = useState<DraftSnapshot>({ enabled: true, drafts: [] });
  const [issue, setIssue] = useState("");
  const [saving, setSaving] = useState(false);
  const [ready, setReady] = useState(false);
  const [recovered, setRecovered] = useState(false);
  const live = useRef(true);
  const remember = (context: string, value: string) => {
    values.current.delete(context); values.current.set(context, value);
    if (values.current.size > 32) values.current.delete(values.current.keys().next().value!);
  };
  useEffect(() => { live.current = true; return () => { live.current = false; }; }, []);
  const request = useCallback(async (value: DraftRequest) => {
    const result = await window.gator!.drafts(value);
    if (live.current) { setSaved(result); setIssue(""); }
    return result;
  }, []);
  const setText = useCallback((value: string | ((previous: string) => string)) => {
    const next = typeof value === "function" ? value(current.current) : value;
    current.current = next; renderText(next); setRecovered(false);
    const owner = identity.current;
    if (!owner) return;
    remember(owner.context, next);
    const version = ++generation.current;
    setSaving(true);
    // Each input is handed to the host immediately. Its serialized atomic
    // writes survive renderer reloads; the window awaits them before closing.
    void request({ action: "save", draft: { ...owner, text: next } }).catch(() => {
      if (live.current) setIssue("Your text is kept in this window, but could not be saved on this device.");
    }).finally(() => { if (live.current && version === generation.current) setSaving(false); });
  }, [request]);

  useLayoutEffect(() => {
    if (!state || !context) return;
    let active = true;
    const before = identity.current;
    const promoted = before && (before.context.startsWith("workspace:") || before.context.startsWith("topic:")) && state.conversation_id && before.source === state.source && previousRunning.current;
    if (promoted) {
      remember(context, current.current);
    }
    identity.current = { context, source: state.source, title: state.primary ? "Gator" : state.conversation_id ? state.title : "New topic" };
    const cached = values.current.get(context);
    current.current = cached ?? ""; renderText(current.current); setRecovered(false); setReady(false);
    const owner = identity.current;
    if (promoted && before) void request({ action: "save", draft: { ...owner, text: current.current } })
      .then(() => request({ action: "remove", context: before.context }))
      .catch(() => { if (live.current) setIssue("Your text could not be saved on this device."); });
    void request({ action: "list" }).then((result) => {
      if (!active) return;
      if (!values.current.has(context)) {
        const retained = result.drafts.find((draft) => draft.context === context);
        current.current = retained?.text ?? ""; remember(context, current.current); renderText(current.current); setRecovered(!!retained);
      }
    }).catch((error) => { if (active) setIssue(error instanceof Error ? error.message : "Saved drafts are unavailable."); })
      .finally(() => { if (active) setReady(true); });
    return () => { active = false; };
  }, [context, request]);
  useLayoutEffect(() => {
    previousRunning.current = state?.running ?? false;
    if (identity.current && state) { identity.current.source = state.source; identity.current.title = state.primary ? "Gator" : state.conversation_id ? state.title : "New topic"; }
  });

  const manage = async (action: DraftRequest) => {
    try {
      const result = await request(action);
      if (action.action === "remove" && action.context !== identity.current?.context) values.current.delete(action.context);
      if (action.action === "clear" || action.action === "configure" && !action.enabled) { values.current.clear(); if (identity.current) remember(identity.current.context, current.current); }
      return result;
    }
    catch (error) { setIssue(error instanceof Error ? error.message : "Saved drafts are unavailable."); throw error; }
  };
  return { text, setText, saved, issue, saving, ready, recovered, manage, context };
}
