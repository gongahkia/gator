import solidPlugin from "@opentui/solid/bun-plugin"
import { mkdir } from "node:fs/promises"

await mkdir(new URL("../dist/", import.meta.url), { recursive: true })
const result = await Bun.build({
  entrypoints: [new URL("../src/index.tsx", import.meta.url).pathname],
  outdir: new URL("../dist/", import.meta.url).pathname,
  target: "bun", plugins: [solidPlugin], sourcemap: "external",
  external: ["@opentui/core", "@opentui/solid", "@opentui/keymap", "solid-js", "@gator/application-host"],
})
if (!result.success) { for (const log of result.logs) console.error(log); process.exit(1) }
