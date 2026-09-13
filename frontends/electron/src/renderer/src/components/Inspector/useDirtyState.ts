import { useCallback, useEffect, useRef, useState } from "react";

interface DirtyState<T> {
  value: T;
  isDirty: boolean;
  setValue: (next: T) => void;
  patch: (partial: Partial<T>) => void;
  reset: (next?: T) => void; // resets baseline + value
  markClean: () => void; // baseline := current value
}

function shallowEqual<T extends object>(a: T, b: T): boolean {
  const keys = new Set([...Object.keys(a), ...Object.keys(b)]) as Set<keyof T>;
  for (const k of keys) {
    if (a[k] !== b[k]) {
      return false;
    }
  }
  return true;
}

export function useDirtyState<T extends object>(initial: T): DirtyState<T> {
  const [value, setValueState] = useState<T>(initial);
  const baselineRef = useRef<T>(initial);
  const [isDirty, setIsDirty] = useState(false);

  useEffect(() => {
    setIsDirty(!shallowEqual(value, baselineRef.current));
  }, [value]);

  const setValue = useCallback((next: T): void => {
    setValueState(next);
  }, []);

  const patch = useCallback((partial: Partial<T>): void => {
    setValueState((current) => ({ ...current, ...partial }));
  }, []);

  const reset = useCallback((next?: T): void => {
    const target = next ?? baselineRef.current;
    baselineRef.current = target;
    setValueState(target);
    setIsDirty(false);
  }, []);

  const markClean = useCallback((): void => {
    baselineRef.current = value;
    setIsDirty(false);
  }, [value]);

  return { value, isDirty, setValue, patch, reset, markClean };
}
