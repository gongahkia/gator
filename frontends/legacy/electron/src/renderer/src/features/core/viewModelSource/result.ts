import type { HcbResult } from "@shared/ipc/result";

export async function unwrap<T>(result: HcbResult<T>, label: string): Promise<T> {
  if (result.ok) {
    return result.data;
  }

  throw new Error(`${label}: ${result.error.message}`);
}
