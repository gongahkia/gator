import { rm } from "node:fs/promises";
import { join } from "node:path";
import { uploadLimits } from "./uploads.mjs";

export function validatePastedImages(images) {
  if (!Array.isArray(images) || !images.length || images.length > uploadLimits.count) throw new Error(`Paste up to ${uploadLimits.count} images per message.`);
  let total = 0;
  for (const image of images) {
    if (!image || typeof image !== "object" || Object.keys(image).some(key => !["type", "data"].includes(key)) || !["image/png", "image/jpeg", "image/webp"].includes(image.type) || !(image.data instanceof Uint8Array)) throw new Error("Paste a PNG, JPEG or WebP image.");
    if (!image.data.byteLength || image.data.byteLength > uploadLimits.image) throw new Error("Pasted images must be no larger than 4 MiB each.");
    total += image.data.byteLength;
  }
  if (total > uploadLimits.total) throw new Error(`Attachments must total no more than ${uploadLimits.total / 1048576} MiB per message.`);
}

// Clipboard bytes enter the same private upload store as chosen files. The
// native decoder supplies inert PNG bytes; paths and HTML are never accepted.
export async function importPastedImages(store, images, decode) {
  validatePastedImages(images);
  const decoded = images.map(image => decode(Buffer.from(image.data)));
  if (decoded.some(bytes => !bytes.length || bytes.length > uploadLimits.image) || decoded.reduce((sum, bytes) => sum + bytes.length, 0) > uploadLimits.total) throw new Error("These images are too large to attach after decoding. Use images up to 4 MiB each and 16 MiB total.");
  const imported = [];
  try {
    for (const [index, bytes] of decoded.entries()) imported.push(await store.importContent(`Pasted image ${new Date().toISOString().replace(/[:.]/g, "-")}-${index + 1}.png`, bytes));
    return imported;
  } catch (error) {
    await Promise.all(imported.map(file => rm(join(store.root, file.id.split("/")[0]), { recursive: true, force: true })));
    throw error;
  }
}
