import type { BackendState, Frame } from "../../ui/src/protocol";

export type DesktopSnapshot = { state?: BackendState; error?: string; terminalBusy?: boolean; fullScreen?: boolean };
export type DesktopFrame = Omit<Frame, "version">;

declare global {
  interface Window {
    gator?: {
      platform: string;
      snapshot(): Promise<DesktopSnapshot>;
      subscribe(callback: (snapshot: DesktopSnapshot) => void): () => void;
      send(frame: DesktopFrame): Promise<void>;
      chooseFolder(): Promise<string | null>;
      openLink(url: string): Promise<void>;
      continueInTerminal(): Promise<void>;
      restart(): Promise<void>;
    };
  }
}
