import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "electron-vite";

const fromRoot = (path: string) => resolve(__dirname, path);
const buildMetadataDefines = {
  __HCB_BUILD_COMMIT__: JSON.stringify(process.env.HCB_BUILD_COMMIT ?? ""),
  __HCB_BUILD_DATE__: JSON.stringify(process.env.HCB_BUILD_DATE ?? ""),
  __HCB_PACKAGE_TOOL__: JSON.stringify(process.env.HCB_PACKAGE_TOOL ?? "")
};

export default defineConfig({
  main: {
    define: buildMetadataDefines,
    resolve: {
      alias: {
        "@main": fromRoot("src/main"),
        "@shared": fromRoot("src/shared")
      }
    },
    build: {
      rollupOptions: {
        input: {
          index: fromRoot("src/main/index.ts")
        },
        external: ["better-sqlite3"]
      }
    }
  },
  preload: {
    resolve: {
      alias: {
        "@preload": fromRoot("src/preload"),
        "@shared": fromRoot("src/shared")
      }
    },
    build: {
      rollupOptions: {
        input: {
          index: fromRoot("src/preload/index.ts")
        },
        external: ["electron"]
      }
    }
  },
  renderer: {
    root: fromRoot("src/renderer"),
    plugins: [react()],
    resolve: {
      alias: {
        "@renderer": fromRoot("src/renderer/src"),
        "@shared": fromRoot("src/shared")
      }
    }
  }
});
