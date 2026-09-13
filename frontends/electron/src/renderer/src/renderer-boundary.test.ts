import { builtinModules } from "node:module";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const rendererRoot = join(process.cwd(), "src", "renderer", "src");
const nodeBuiltins = new Set([
  ...builtinModules,
  ...builtinModules.map((moduleName) => `node:${moduleName}`)
]);
const forbiddenAliases = ["@main/", "@preload/"];
const allowedSharedContracts = [
  "@shared/ipc",
  "@shared/plannerLinks",
  "@shared/preloadApi",
  "@shared/result",
  "@shared/search"
];
const importPattern = /\b(?:import|export)\s+(?:type\s+)?(?:[^"']*?\s+from\s+)?["']([^"']+)["']/g;

function relativeToRepo(path: string): string {
  return relative(process.cwd(), path).split("\\").join("/");
}

function forbiddenImportReason(filePath: string, specifier: string): string | undefined {
  if (specifier === "electron" || nodeBuiltins.has(specifier)) {
    return `imports ${specifier}`;
  }

  if (forbiddenAliases.some((alias) => specifier.startsWith(alias))) {
    return `imports ${specifier}`;
  }

  if (
    specifier.startsWith("@shared/") &&
    !allowedSharedContracts.some(
      (allowed) => specifier === allowed || specifier.startsWith(`${allowed}/`)
    )
  ) {
    return `imports non-contract shared module ${specifier}`;
  }

  if (!specifier.startsWith(".")) {
    return undefined;
  }

  const resolved = relativeToRepo(resolve(dirname(filePath), specifier));

  if (resolved.startsWith("src/main/") || resolved.startsWith("src/preload/")) {
    return `imports privileged module ${specifier}`;
  }

  return undefined;
}

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const entryPath = join(directory, entry.name);

    if (entry.isDirectory()) {
      return sourceFiles(entryPath);
    }

    if (
      !entry.name.endsWith(".ts") &&
      !entry.name.endsWith(".tsx") &&
      !entry.name.endsWith(".d.ts")
    ) {
      return [];
    }

    if (entry.name.endsWith(".test.ts") || entry.name.endsWith(".test.tsx")) {
      return [];
    }

    return [entryPath];
  });
}

describe("renderer privilege boundary", () => {
  it("does not import Electron, Node builtins, main, or preload modules", () => {
    const violations = sourceFiles(rendererRoot).flatMap((filePath) => {
      const contents = readFileSync(filePath, "utf8");
      const imports = [...contents.matchAll(importPattern)].map((match) => match[1]);

      return imports.flatMap((specifier) => {
        const reason = forbiddenImportReason(filePath, specifier);

        return reason ? [`${relativeToRepo(filePath)} ${reason}`] : [];
      });
    });

    expect(violations).toEqual([]);
  });

  it("keeps core screens behind the swappable view-model source", () => {
    const coreScreensRoot = join(rendererRoot, "features", "core");
    const contents = sourceFiles(coreScreensRoot)
      .filter((filePath) => {
        const relativePath = relative(coreScreensRoot, filePath).split("\\").join("/");

        return (
          relativePath === "CoreScreens.tsx" ||
          relativePath === "coreScreenShared.tsx" ||
          relativePath.startsWith("screens/")
        );
      })
      .map((filePath) => readFileSync(filePath, "utf8"))
      .join("\n");

    expect(contents).toContain("coreViewModelSource");
    expect(contents).not.toContain("./mockCoreViewModels");
    expect(contents).not.toContain("../mockCoreViewModels");
  });
});
