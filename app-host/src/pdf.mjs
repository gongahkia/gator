import { createRequire } from "node:module";
import { dirname, join } from "node:path";

export async function renderPDF(data, pageNumber, width = 1200) {
  if (!(data instanceof Uint8Array) || data.byteLength > 16 * 1024 * 1024 || !Number.isSafeInteger(pageNumber) || pageNumber < 1 || !Number.isFinite(width) || width < 100 || width > 2000) throw new Error("Invalid PDF preview request.");
  const { createCanvas } = await import("@napi-rs/canvas");
  const { getDocument } = await import("pdfjs-dist/legacy/build/pdf.mjs");
  const require = createRequire(import.meta.url);
  const root = dirname(require.resolve("pdfjs-dist/package.json"));
  const loading = getDocument({ data: new Uint8Array(data), useSystemFonts: true, disableFontFace: true, useWasm: false, isEvalSupported: false, standardFontDataUrl: join(root, "standard_fonts/") });
  let document;
  try {
    document = await loading.promise;
    if (pageNumber > document.numPages) throw new Error("This PDF page does not exist.");
    const page = await document.getPage(pageNumber), base = page.getViewport({ scale: 1 });
    const viewport = page.getViewport({ scale: Math.min(width / base.width, Math.sqrt(32000000 / (base.width * base.height))) });
    const canvas = createCanvas(Math.ceil(viewport.width), Math.ceil(viewport.height));
    await page.render({ canvas, canvasContext: canvas.getContext("2d"), viewport }).promise;
    return { pages: document.numPages, page: pageNumber, image: new Uint8Array(canvas.toBuffer("image/png")) };
  } finally { await loading.destroy(); }
}
