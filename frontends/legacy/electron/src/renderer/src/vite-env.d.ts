/// <reference types="vite/client" />

import type { HcbApi } from "@shared/ipc/preloadApi";

declare global {
  interface Window {
    hcb?: HcbApi;
  }
}

export {};
