import { readFileSync, readdirSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";

const rendererSourceDir = join(process.cwd(), "src", "renderer", "src");
const privilegedImportPattern =
  /\b(?:import|export)\b[\s\S]*?\bfrom\s*["'](?:electron|node:[^"']+|fs|path|os|net|http|https|child_process|worker_threads|sqlite3|better-sqlite3|@main(?:\/[^"']*)?|@preload(?:\/[^"']*)?)["']|(?:require|import)\(\s*["'](?:electron|node:[^"']+|fs|path|os|net|http|https|child_process|worker_threads|sqlite3|better-sqlite3|@main(?:\/[^"']*)?|@preload(?:\/[^"']*)?)["']\s*\)/;

describe("renderer privilege boundary", () => {
  it("does not import Electron, Node, main, preload, SQLite, network, or command modules", () => {
    const offenders = rendererFiles(rendererSourceDir).filter((filePath) =>
      privilegedImportPattern.test(readFileSync(filePath, "utf8"))
    );

    expect(offenders.map((filePath) => relative(process.cwd(), filePath))).toEqual([]);
  });
});

function rendererFiles(directory: string): string[] {
  const files: string[] = [];

  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);

    if (entry.isDirectory()) {
      files.push(...rendererFiles(path));
      continue;
    }

    if (
      /\.(?:ts|tsx|js|jsx)$/.test(entry.name) &&
      !entry.name.endsWith(".test.ts") &&
      !entry.name.endsWith(".test.tsx")
    ) {
      files.push(path);
    }
  }

  return files;
}
