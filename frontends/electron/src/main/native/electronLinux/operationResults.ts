import { redactDiagnosticText } from "@shared/redaction";
import type { NativeOperationResult } from "../types";

export function unsupported(message: string): NativeOperationResult {
  return {
    ok: false,
    state: "unsupported",
    message
  };
}

export function pending(message: string): NativeOperationResult {
  return {
    ok: false,
    state: "pending",
    message
  };
}

export function sanitizedFailure(fallback: string): NativeOperationResult {
  return {
    ok: false,
    state: "error",
    message: redactDiagnosticText(fallback).slice(0, 500)
  };
}
