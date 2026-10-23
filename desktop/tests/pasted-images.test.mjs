import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { UploadStore, uploadLimits } from "../electron/uploads.mjs";
import { importPastedImages, validatePastedImages } from "../electron/pasted-images.mjs";

test("pasted images are bounded bytes and become independent private uploads", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-paste-"));
  try {
    const store = new UploadStore(root), data = new Uint8Array([1,2,3]);
    const [file] = await importPastedImages(store, [{ type: "image/png", data }], () => Buffer.from("decoded PNG"));
    assert.match(file.name, /^Pasted image .+\.png$/); assert.equal((await readFile(join(root,file.id))).toString(), "decoded PNG");
    for (const images of [[], Array(uploadLimits.count + 1).fill({type:"image/png",data}), [{type:"image/svg+xml",data}], [{type:"image/png",data,path:"/secret"}], [{type:"image/png",data:"path"}], [{type:"image/png",data:new Uint8Array(4*1024*1024+1)}]]) assert.throws(() => validatePastedImages(images));
    const before = await readdir(root);
    await assert.rejects(importPastedImages(store, [{type:"image/png",data}], () => {throw new Error("Invalid image");}), /Invalid image/);
    assert.deepEqual(await readdir(root), before);
  } finally { await rm(root,{recursive:true,force:true}); }
});
