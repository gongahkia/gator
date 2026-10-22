import { basename, join } from "node:path";
import { fileReference, readBoundedFile } from "./files.mjs";

export async function selectedFileSource(reference, { state, context, drafts, imported, uploads, files }) {
  const ref = fileReference(reference);
  if (ref.kind === "upload") {
    const sent = state.messages?.some(message => message.input_ids?.includes(ref.id));
    const pending = drafts.drafts?.some(draft => draft.context === context && draft.uploads?.some(file => file.id === ref.id));
    if (!sent && !pending && imported.get(ref.id) !== context) throw new Error("This attachment is not selected in the current conversation.");
    let bytes;
    try { bytes = await readBoundedFile(uploads.root, ref.id); }
    catch (error) { if (error.code !== "ENOENT") throw error; bytes = await files.original(ref); }
    return { name: basename(ref.id), bytes };
  }
  const bundles = [...(state.messages ?? []).flatMap(message => message.bundle ? [message.bundle] : []), ...(state.work_details?.latest_bundle ? [state.work_details.latest_bundle] : [])];
  const bundle = bundles.find(bundle => bundle.Path === ref.bundle), file = bundle?.Artifacts?.find(file => file.Path === ref.path);
  if (!file || !/^[a-f0-9]{64}$/.test(file.SHA256 || "")) throw new Error("This file is not available in the current conversation's retained outputs.");
  return { name: basename(file.Path), root: join(bundle.Path, "output"), path: file.Path, sha256: file.SHA256 };
}
