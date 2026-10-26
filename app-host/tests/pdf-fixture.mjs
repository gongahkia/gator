import { createCanvas } from "@napi-rs/canvas";
export function pngFixture() {
  const canvas = createCanvas(32, 32), context = canvas.getContext("2d");
  context.fillStyle = "#22aa66"; context.fillRect(0, 0, 32, 32);
  context.fillStyle = "#ffcc22"; context.fillRect(8, 8, 16, 16);
  return canvas.toBuffer("image/png");
}
export function pdfFixture() {
  const objects = [
    "<< /Type /Catalog /Pages 2 0 R >>",
    "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 5 0 R >> >> /Contents 6 0 R >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 5 0 R >> >> /Contents 7 0 R >>",
    "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    ...["First page", "Second page"].map(text => { const content = `BT /F1 18 Tf 20 100 Td (${text}) Tj ET`; return `<< /Length ${content.length} >>\nstream\n${content}\nendstream`; }),
  ];
  let output = "%PDF-1.4\n", offsets = [0];
  objects.forEach((object, index) => { offsets.push(Buffer.byteLength(output)); output += `${index + 1} 0 obj\n${object}\nendobj\n`; });
  const xref = Buffer.byteLength(output); output += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n` + offsets.slice(1).map(offset => `${String(offset).padStart(10, "0")} 00000 n \n`).join("");
  output += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return new Uint8Array(Buffer.from(output));
}
