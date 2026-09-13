import type { NativeOperationResult } from "../types";

export function unsupported(message: string): NativeOperationResult {
  return {
    ok: false,
    state: "unsupported",
    message
  };
}
