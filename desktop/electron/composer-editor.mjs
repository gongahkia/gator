import { uploadContext } from "@gator/application-host/uploads";
import { validateFrame } from "@gator/application-host/policy";

// Only the current idle Go backend can prepare the executable and temporary
// file. The renderer supplies draft text and its owner, never a command/path.
export async function editComposer(owner, request, handoff, drafts) {
  if (!request || Object.keys(request).some(key => !["context", "text"].includes(key)) || typeof request.context !== "string" || typeof request.text !== "string") throw new Error("Choose the current message draft for editing.");
  validateFrame({ type: "submit", text: request.text });
  if (Buffer.byteLength(request.text) > 128 * 1024) throw new Error("The message draft exceeds 128 KiB.");
  if (!owner || owner.closed || uploadContext(owner.state) !== request.context || owner.state?.running || owner.state?.queued || owner.state?.panel || owner.state?.pending_approval || owner.state?.external) throw new Error("Return to the idle conversation before editing its draft externally.");
  await owner.prepareComposerEditor(request.text);
  const job = owner.state?.external;
  if (job?.kind !== "editor") throw new Error(owner.state?.notice || "Gator could not prepare the editor. Your draft is kept.");
  let error = "";
  try { error = await handoff(job); }
  catch (failure) { error = `Could not complete the editor: ${failure.message}`; }
  // Await Go's completion, including reading/removing its own temporary file.
  // A nonzero editor exit can still leave useful saved text for human review.
  await owner.completeExternal(error);
  // Go omits an empty input field from its display projection. That is an
  // intentionally emptied editor result, not a reason to restore old text.
  const text = owner.state?.input ?? "";
  // Retain a normal returned edit before acknowledging it to the renderer.
  // Reload/disconnect must recover it from the existing draft store. Never
  // override retention being disabled, newer text, attachments or reply binding.
  if (drafts && Buffer.byteLength(text) <= 128 * 1024) {
    const saved = await drafts.request({ action: "list" });
    if (saved.enabled) {
      const prior = saved.drafts.find(draft => draft.context === request.context);
      const combined = prior && prior.text !== request.text ? [prior.text, text].filter(Boolean).join("\n\n") : text;
      if (Buffer.byteLength(combined) <= 128 * 1024) {
        const { updated, ...retained } = prior ?? { context: request.context, title: owner.state?.title || "Conversation", source: owner.state?.source || "", uploads: [] };
        await drafts.request({ action: "save", draft: { ...retained, text: combined } });
      }
    }
  }
  return { context: request.context, text, error };
}
