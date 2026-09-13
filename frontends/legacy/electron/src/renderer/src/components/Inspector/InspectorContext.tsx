import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState
} from "react";
import type { ReactNode, RefObject } from "react";

export type InspectorItemKind =
  | "task"
  | "event"
  | "note"
  | "settings"
  | "diagnostics";

export interface InspectorItem {
  kind: InspectorItemKind;
  id: string; // stable id for the underlying record. "new" allowed for create flows
  ownerId?: string;
  title: string;
  subtitle?: string;
  body: ReactNode;
  hideHeader?: boolean; // body owns title/close affordance
  returnFocus?: RefObject<HTMLElement> | null; // restored on close
  dirty?: boolean; // whether body holds unsaved edits
  onConfirmClose?: () => Promise<boolean> | boolean; // return false to keep open
  actions?: ReactNode; // footer actions
}

interface InspectorContextValue {
  current: InspectorItem | null;
  open: (item: InspectorItem) => void;
  close: () => Promise<void>;
  update: (patch: Partial<Pick<InspectorItem, "title" | "subtitle" | "dirty" | "body" | "actions" | "hideHeader">>) => void;
  isOpen: boolean;
}

const InspectorReactContext = createContext<InspectorContextValue | null>(null);

export function InspectorProvider({ children }: { children: ReactNode }): JSX.Element {
  const [current, setCurrent] = useState<InspectorItem | null>(null);
  const currentRef = useRef<InspectorItem | null>(null);
  currentRef.current = current;

  const open = useCallback((item: InspectorItem): void => {
    setCurrent(item);
  }, []);

  const close = useCallback(async (): Promise<void> => {
    const active = currentRef.current;

    if (active?.onConfirmClose) {
      const allow = await active.onConfirmClose();
      if (!allow) {
        return;
      }
    }

    setCurrent(null);
    queueMicrotask(() => active?.returnFocus?.current?.focus());
  }, []);

  const update = useCallback<InspectorContextValue["update"]>((patch) => {
    setCurrent((existing) => (existing ? { ...existing, ...patch } : existing));
  }, []);

  const value = useMemo<InspectorContextValue>(
    () => ({ current, open, close, update, isOpen: current !== null }),
    [current, open, close, update]
  );

  return <InspectorReactContext.Provider value={value}>{children}</InspectorReactContext.Provider>;
}

export function useInspector(): InspectorContextValue {
  const value = useContext(InspectorReactContext);
  if (!value) {
    throw new Error("useInspector requires InspectorProvider");
  }
  return value;
}
