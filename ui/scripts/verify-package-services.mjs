// Build-time fixture harness. Every service module and dependency under test is
// resolved from the production artifact; fixture generators stay in the checkout.
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { mkdtemp, mkdir, readFile, writeFile, rm, symlink, realpath, copyFile, readdir, readlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, relative, resolve, isAbsolute } from "node:path";
import { pathToFileURL } from "node:url";
import { createHash } from "node:crypto";
import { ClassicLevel } from "../../app-host/node_modules/classic-level/index.js";
import { pdfFixture } from "../../app-host/tests/pdf-fixture.mjs";
import { wordFixture, workbookFixture, slidesFixture } from "../../app-host/tests/office-fixtures.mjs";

assert.ok(process.versions.bun, "Run this probe with the installed Bun runtime.");
const artifact = await realpath(resolve(process.argv[2]));
// A dependency can resolve successfully before installation while an absolute
// link still points at the build tree. Reject such artifacts before shipping.
async function checkLinks(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isSymbolicLink()) {
      assert.ok(!isAbsolute(await readlink(path)), `Artifact contains an absolute symlink: ${path}`);
      const part = relative(artifact, await realpath(path));
      assert.ok(!isAbsolute(part) && part !== ".." && !part.startsWith("../") && !part.startsWith("..\\"), `Artifact symlink escapes its installation: ${path}`);
    } else if (entry.isDirectory()) await checkLinks(path);
  }
}
await checkLinks(artifact);
const module = join(artifact, "ui/node_modules/@gator/application-host/src/application.mjs"), require = createRequire(module);
const metadata = JSON.parse(await readFile(join(artifact, "app-host/package.json"), "utf8"));
for (const dependency of Object.keys(metadata.dependencies)) {
  const resolved = await realpath(require.resolve(dependency)), part = relative(artifact, resolved);
  assert.ok(!isAbsolute(part) && part !== ".." && !part.startsWith("../") && !part.startsWith("..\\"), `${dependency} escaped the artifact: ${resolved}`);
}
const { Application } = await import(pathToFileURL(module).href);
const { inspectContent } = await import(pathToFileURL(join(artifact, "ui/node_modules/@gator/application-host/src/file-content.mjs")).href);
const root = await mkdtemp(join(tmpdir(), "gator-production-services-"));
let host;
try {
  const userData = join(root, "app"), legacy = new ClassicLevel(join(userData, "Local Storage/leveldb"), { keyEncoding: "buffer", valueEncoding: "buffer" });
  await legacy.put(Buffer.from("_gator://app\0\x01gator:sidebar-collapsed", "latin1"), Buffer.from("\x01true", "latin1")); await legacy.close();
  await mkdir(join(root, "path"));
  if (process.platform === "win32") await copyFile(process.execPath, join(root, "path/bun.exe"));
  else await symlink(process.execPath, join(root, "path/bun"));
  const env = { ...process.env, PATH: join(root, "path"), GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") };
  host = new Application({ binary: join(artifact, "runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), userData, workspace: join(root, "workspace"), env });
  await host.start(); assert.equal((await host.preferences.read()).sidebarCollapsed, true);
  assert.match(await host.version(), /gator terminal/i);
  const specimens = [
    { name: "document.pdf", bytes: Buffer.from(pdfFixture()) },
    { name: "notes.txt", bytes: Buffer.from("Original complete content") },
    { name: "document.docx", bytes: wordFixture(), edit: { paragraphs: { "p:0": "Production Word change" } }, match: "Production Word change" },
    { name: "budget.xlsx", bytes: workbookFixture(), edit: { cells: { "0!C1": "=B1*7" } }, match: "B1*7" },
    { name: "slides.pptx", bytes: slidesFixture(), edit: { paragraphs: { "s:0:p:0": "Production slide change" } }, match: "Production slide change" },
  ];
  for (const specimen of specimens) await writeFile(join(root, specimen.name), specimen.bytes);
  const files = await host.importFiles(specimens.map(file => join(root, file.name)), host.context);
  const request = file => ({ context: host.context, ref: { kind: "upload", id: file.id } });
  const first = await host.pdf(request(files[0]), 1), second = await host.pdf(request(files[0]), 2);
  assert.equal(first.pages, 2); assert.equal(second.pages, 2);
  const hash = bytes => createHash("sha256").update(bytes).digest("hex");
  assert.notEqual(hash(first.image), hash(second.image)); assert.ok(first.image.length > 1000);
  for (let index = 1; index < specimens.length; index++) {
    const specimen = specimens[index], selected = request(files[index]), original = await host.file({ ...selected, action: "open" });
    const edit = specimen.edit ?? { text: "Production text change" }, saved = await host.file({ ...selected, action: "save", expected: original.current, edit });
    assert.equal(saved.versions.length, 2);
    const output = join(root, "export-" + specimen.name); await host.exportFile({ ...selected, action: "export", version: saved.revision }, output);
    assert.ok(JSON.stringify(inspectContent(specimen.name, await readFile(output))).includes(specimen.match ?? "Production text change"));
    const selectedVersion = await host.file({ ...selected, action: "attach", version: saved.revision });
    assert.equal((await host.file({ context: host.context, ref: { kind: "upload", id: selectedVersion.id }, action: "open" })).current, saved.current);
    assert.deepEqual(await readFile(join(root, specimen.name)), specimen.bytes);
  }
  assert.equal(host.snapshot().state.messages.length, 0);
  console.log("PASS: production dependency closure, real Go host, Bun-only child PATH, legacy preference import, distinct rendered PDF pages, native Office/text save/export/selected-version handoff, unchanged originals and no model submission.");
} finally { await host?.close(); await rm(root, { recursive: true, force: true }); }
