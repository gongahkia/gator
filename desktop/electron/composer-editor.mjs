import { uploadContext } from "@gator/application-host/uploads";
import { validateFrame } from "@gator/application-host/policy";

// Only the current idle Go backend can prepare the executable and temporary
// file. The renderer supplies draft text and its owner, never a command/path.
export async function editComposer(owner, request, handoff) {
  if (!request || Object.keys(request).some(key => !["context", "text"].includes(key)) || typeof request.context !== "string" || typeof request.text !== "string") throw new Error("Choose the current message draft for editing.");
  validateFrame({ type: "submit", text: request.text });
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
  return { context: request.context, text: owner.state?.input ?? request.text, error };
}
