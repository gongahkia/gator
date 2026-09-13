import { useEffect, useMemo, useState } from "react";
import type { CustomizationExtension, CustomizationStatusResponse } from "@shared/ipc/contracts";

const snippetStylePrefix = "hcb-user-snippet:";
const extensionMessageType = "hcb-extension-log";

export function CustomizationRuntime(): JSX.Element | null {
  const [status, setStatus] = useState<CustomizationStatusResponse | null>(null);

  useEffect(() => {
    let active = true;
    const load = async (): Promise<void> => {
      const result = await window.hcb?.settings.customizationStatus();
      if (active && result?.ok) {
        setStatus(result.data);
      }
    };
    void load();
    const interval = window.setInterval(() => void load(), 5_000);
    return () => {
      active = false;
      window.clearInterval(interval);
    };
  }, []);

  useEffect(() => {
    applySnippetStyles(status);
  }, [status]);

  useEffect(() => {
    const listener = (event: MessageEvent): void => {
      if (!isExtensionLogMessage(event.data)) {
        return;
      }
      void window.hcb?.settings.logExtensionMessage({
        extensionId: event.data.extensionId,
        level: event.data.level,
        message: event.data.message
      });
    };
    window.addEventListener("message", listener);
    return () => window.removeEventListener("message", listener);
  }, []);

  const extensions = useMemo(
    () => (status?.safeMode ? [] : status?.extensions.filter((extension) =>
      extension.enabled && extension.capabilities.includes("ui.panel") && extension.code
    ) ?? []),
    [status]
  );

  if (extensions.length === 0) {
    return null;
  }

  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 grid max-w-sm gap-3">
      {extensions.map((extension) => (
        <section
          className="pointer-events-auto overflow-hidden rounded-hcbMd border border-border bg-bg-secondary shadow-hcbLg"
          key={extension.id}
        >
          <div className="border-b border-border px-3 py-2 text-[var(--text-sm)] font-semibold text-text-primary">
            {extension.name}
          </div>
          <iframe
            className="h-40 w-80 border-0"
            sandbox="allow-scripts"
            srcDoc={extensionHtml(extension)}
            title={`Extension ${extension.name}`}
          />
        </section>
      ))}
    </div>
  );
}

function applySnippetStyles(status: CustomizationStatusResponse | null): void {
  document.querySelectorAll(`style[data-hcb-snippet^="${snippetStylePrefix}"]`).forEach((node) => {
    node.remove();
  });

  if (!status || status.safeMode) {
    return;
  }

  for (const snippet of status.snippets) {
    if (!snippet.enabled || snippet.error || !snippet.content) {
      continue;
    }

    const style = document.createElement("style");
    style.dataset.hcbSnippet = `${snippetStylePrefix}${snippet.id}`;
    style.textContent = snippet.content;
    document.head.appendChild(style);
  }
}

function extensionHtml(extension: CustomizationExtension): string {
  const bootstrap = `
    const extensionId = ${JSON.stringify(extension.id)};
    const caps = new Set(${JSON.stringify(extension.capabilities)});
    window.hcbExtension = Object.freeze({
      capabilities: Array.from(caps),
      hostInfo() {
        return { app: "Hot Cross Buns", extensionId };
      },
      log(message, level = "info") {
        if (!caps.has("log.write")) return;
        parent.postMessage({
          type: ${JSON.stringify(extensionMessageType)},
          extensionId,
          level,
          message: String(message).slice(0, 500)
        }, "*");
      }
    });
  `;

  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; connect-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'">
    <style>
      html,body{margin:0;background:#111827;color:#f9fafb;font:13px -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
      body{min-width:280px;min-height:120px;padding:12px}
      button,input,select,textarea{font:inherit}
    </style>
  </head>
  <body>
    <script>${bootstrap}</script>
    <script>${extension.code ?? ""}</script>
  </body>
</html>`;
}

function isExtensionLogMessage(value: unknown): value is {
  extensionId: string;
  level: "info" | "warn" | "error";
  message: string;
  type: typeof extensionMessageType;
} {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const message = value as Record<string, unknown>;
  return message.type === extensionMessageType &&
    typeof message.extensionId === "string" &&
    (message.level === "info" || message.level === "warn" || message.level === "error") &&
    typeof message.message === "string";
}
