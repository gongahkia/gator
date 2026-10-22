// Reuse the alligator's 256px PNG from the checked-in ICNS. No raster edits or
// additional image library are needed; modern ICO supports PNG entries.
import { readFile, writeFile } from "node:fs/promises";
const icon = await readFile(new URL("../assets/icon.icns", import.meta.url));
let png;
for (let offset = 8; offset < icon.length;) {
  const length = icon.readUInt32BE(offset + 4);
  if (length < 8 || offset + length > icon.length) throw new Error("Invalid ICNS entry");
  if (icon.toString("ascii", offset, offset + 4) === "ic08") png = icon.subarray(offset + 8, offset + length);
  offset += length;
}
if (!png || png.readUInt32BE(16) !== 256 || png.readUInt32BE(20) !== 256) throw new Error("A 256px PNG icon is required");
const header = Buffer.alloc(22);
header.writeUInt16LE(1, 2); // ICO type
header.writeUInt16LE(1, 4); // one image; zero width/height bytes mean 256
header.writeUInt16LE(1, 10);
header.writeUInt16LE(32, 12);
header.writeUInt32LE(png.length, 14);
header.writeUInt32LE(header.length, 18);
await writeFile(new URL("../assets/icon.ico", import.meta.url), Buffer.concat([header, png]));
