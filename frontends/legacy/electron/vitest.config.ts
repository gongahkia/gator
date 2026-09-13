import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@main": resolve(__dirname, "src/main"),
      "@preload": resolve(__dirname, "src/preload"),
      "@renderer": resolve(__dirname, "src/renderer/src"),
      "@shared": resolve(__dirname, "src/shared")
    }
  },
  test: {
    environment: "node",
    environmentMatchGlobs: [
      ["src/renderer/**/*.test.ts", "jsdom"],
      ["src/renderer/**/*.test.tsx", "jsdom"]
    ],
    include: ["scripts/**/*.test.ts", "src/**/*.test.ts", "src/**/*.test.tsx"],
    testTimeout: 30_000,
    maxWorkers: 2,
    minWorkers: 1,
    setupFiles: ["./vitest.setup.ts"],
    clearMocks: true,
    restoreMocks: true
  }
});
