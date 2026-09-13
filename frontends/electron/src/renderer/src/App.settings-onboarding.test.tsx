import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { AutoTagRule } from "@shared/ipc/contracts";
import { ok } from "@shared/ipc/result";
import App from "./App";
import {
  connectedGoogleStatus,
  goToSection,
  installHcb,
  now,
  onboardingHcb,
  seededHcb,
  signedOutGoogleStatus,
  testSettings
} from "./test/appTestHelpers";

function autoTagRule(patch: Partial<AutoTagRule> = {}): AutoTagRule {
  return {
    id: "rule-coding",
    name: "Coding",
    enabled: true,
    targetKinds: ["task", "event", "note"],
    matchField: "title",
    matchType: "prefix",
    pattern: "CODING",
    tags: ["coding"],
    stripMatchedPrefix: true,
    eventColorId: null,
    overrideExistingEventColor: false,
    createdAt: now,
    updatedAt: now,
    ...patch
  };
}

describe("App settings and onboarding", () => {
  it("renders required settings sections and section controls", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");

    const dialog = await screen.findByRole("dialog", { name: "Settings" });
    expect(within(dialog).getByRole("button", { name: "General" })).toHaveAttribute("aria-pressed", "true");
    expect(within(dialog).getByRole("button", { name: "Profile" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Appearance" })).toBeInTheDocument();

    expect(screen.getByRole("combobox", { name: "App language" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Open Hot Cross Buns at login" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Sync mode" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Local MCP server" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Profile" }));
    expect(screen.getByRole("textbox", { name: "Google OAuth client ID" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Add Google Account/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Task lists" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Appearance" }));
    expect(screen.getByRole("combobox", { name: "Theme" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Color theme" })).toBeInTheDocument();
    expect(screen.queryByText("Performance")).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Translucent background" })).not.toBeInTheDocument();
    expect(screen.queryByRole("slider", { name: "App surface opacity" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Layout" })).toBeInTheDocument();
  });

  it("filters settings results and switches to the matching tab", async () => {
    installHcb(seededHcb());
    render(<App />);

    await goToSection("Settings");

    const dialog = await screen.findByRole("dialog", { name: "Settings" });
    const search = within(dialog).getByRole("textbox", { name: "Search settings" });

    fireEvent.change(search, { target: { value: "menubar" } });

    await waitFor(() => {
      expect(within(dialog).getByRole("button", { name: "Alerts" })).toHaveAttribute("aria-pressed", "true");
    });
    expect(within(dialog).getByRole("heading", { level: 2, name: "Menu bar" })).toBeInTheDocument();
    expect(within(dialog).getByRole("checkbox", { name: "Menu bar extra" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("heading", { level: 2, name: "Notifications" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("heading", { level: 2, name: "Language" })).not.toBeInTheDocument();

    fireEvent.change(search, { target: { value: "performance" } });

    await waitFor(() => {
      expect(within(dialog).getByRole("button", { name: "General" })).toHaveAttribute("aria-pressed", "true");
    });
    expect(within(dialog).getByRole("heading", { level: 2, name: "Diagnostics" })).toBeInTheDocument();
    expect(within(dialog).getByRole("checkbox", { name: "Include performance diagnostics" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("checkbox", { name: "Include field-redacted Google payloads in local logs" })).not.toBeInTheDocument();
  });

  it("applies base theme and color theme settings", async () => {
    const api = seededHcb();
    let settings = testSettings({
      theme: "dark",
      colorTheme: "dracula",
      uiFontName: "Inter",
      uiTextSizePoints: 15
    });
    api.settings.get = vi.fn(async () => ok(settings));
    api.settings.update = vi.fn(async (request) => {
      settings = testSettings({
        ...settings,
        ...request
      });

      return ok(settings);
    });
    api.native.listFontFamilies = vi.fn(async () =>
      ok({
        platform: "darwin" as const,
        families: ["Avenir", "JetBrains Mono", "SF Pro Text"]
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await waitFor(() => {
      expect(document.documentElement).toHaveAttribute("data-theme", "dark");
      expect(document.documentElement).toHaveAttribute("data-color-theme", "dracula");
      expect(document.documentElement.style.getPropertyValue("--color-accent")).toBe("#FF79C6");
      expect(document.documentElement.style.getPropertyValue("--font-family")).toContain("\"Inter\"");
      expect(document.documentElement.style.getPropertyValue("--font-family-mono")).toContain("\"Inter\"");
      expect(document.documentElement.style.getPropertyValue("--text-base")).toBe("15px");
    });

    await goToSection("Settings");
    await user.click(screen.getByRole("button", { name: "Appearance" }));
    expect(screen.getByLabelText("Lavender background")).toHaveValue("#bd93f9");
    await user.selectOptions(screen.getByLabelText("Theme"), "light");

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith({
        theme: "light",
        colorTheme: "notion"
      });
      expect(document.documentElement).toHaveAttribute("data-theme", "light");
      expect(document.documentElement).toHaveAttribute("data-color-theme", "notion");
      expect(screen.getByLabelText("Lavender background")).toHaveValue("#9b8afb");
    });

    await user.selectOptions(screen.getByLabelText("Color theme"), "githubLight");

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith({ colorTheme: "githubLight" });
      expect(document.documentElement).toHaveAttribute("data-color-theme", "githubLight");
    });
    expect(screen.queryByText("Light themes")).not.toBeInTheDocument();
    await waitFor(() => expect(api.native.listFontFamilies).toHaveBeenCalled());
    await waitFor(() => {
      expect(screen.getAllByRole("option", { name: "Avenir" }).length).toBeGreaterThan(0);
    });

    await user.selectOptions(screen.getByRole("combobox", { name: "Font family" }), "JetBrains Mono");

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith({ uiFontName: "JetBrains Mono" });
      expect(document.documentElement.style.getPropertyValue("--font-family")).toContain("\"JetBrains Mono\"");
      expect(document.documentElement.style.getPropertyValue("--font-family-mono")).toContain("\"JetBrains Mono\"");
    });

    fireEvent.change(screen.getByLabelText("Text size points"), { target: { value: "16" } });

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith({ uiTextSizePoints: 16 });
      expect(document.documentElement.style.getPropertyValue("--text-base")).toBe("16px");
    });
  });

  it("uses inferred custom background theme until a built-in theme is selected", async () => {
    const api = seededHcb();
    const customBackground = {
      fileName: "desk.png",
      mimeType: "image/png",
      dataBase64: "abc",
      palette: {
        isDark: false,
        ember: "#D65D0E",
        moss: "#79740E",
        blue: "#076678",
        ink: "#3C3836",
        cream: "#FBF1C7",
        cardStroke: "#D5C4A1",
        dominant: "#FBF1C7",
        accents: ["#D65D0E", "#79740E", "#076678"]
      },
      updatedAt: "2026-06-10T00:00:00.000Z"
    };
    let settings = testSettings({
      customBackground,
      useInferredBackgroundTheme: true
    });
    api.settings.get = vi.fn(async () => ok(settings));
    api.settings.update = vi.fn(async (request) => {
      settings = testSettings({
        ...settings,
        ...request
      });

      return ok(settings);
    });
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await waitFor(() => {
      expect(document.documentElement).toHaveAttribute("data-color-theme", "customBackground");
      expect(document.documentElement.style.getPropertyValue("--color-accent")).toBe("#D65D0E");
      expect(document.documentElement.style.getPropertyValue("--app-shell-background")).toContain("data:image/png;base64,abc");
    });

    await goToSection("Settings");
    await user.click(screen.getByRole("button", { name: "Appearance" }));
    expect(screen.getByLabelText("Color theme")).toHaveValue("customBackground");
    expect(screen.getByRole("img", { name: "Custom background preview" })).toBeInTheDocument();
    expect(screen.getByLabelText("Infer theme from background")).toBeChecked();

    await user.selectOptions(screen.getByLabelText("Color theme"), "githubLight");

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith({
        colorTheme: "githubLight",
        useInferredBackgroundTheme: false
      });
      expect(document.documentElement).toHaveAttribute("data-color-theme", "githubLight");
    });

    await user.selectOptions(screen.getByLabelText("Color theme"), "customBackground");

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith({ useInferredBackgroundTheme: true });
      expect(document.documentElement).toHaveAttribute("data-color-theme", "customBackground");
    });

    await user.click(screen.getByRole("button", { name: "Clear" }));

    expect(api.settings.update).toHaveBeenCalledWith({
      customBackground: null,
      useInferredBackgroundTheme: true
    });
  });

  it("opens the diagnostics workspace from settings", async () => {
    const api = seededHcb();
    const base = await api.diagnostics.summary();
    if (!base.ok) {
      throw new Error("Missing diagnostics fixture");
    }
    api.diagnostics.summary = vi.fn(async () =>
      ok({
        ...base.data,
        dangerousToken: "raw-google-token"
      } as typeof base.data & { dangerousToken: string })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    await user.click(screen.getByRole("button", { name: /View diagnostics/ }));

    const dialog = await screen.findByRole("dialog", { name: "Diagnostics" });
    expect(dialog).toBeInTheDocument();
    expect(dialog).not.toHaveTextContent("raw-google-token");
    expect(within(dialog).getByRole("button", { name: "Overview" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Support" })).toBeInTheDocument();
  });

  it("updates general settings controls through settings IPC", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    await user.selectOptions(screen.getByRole("combobox", { name: "Sync mode" }), "manual");
    await user.click(screen.getByRole("checkbox", { name: "Local MCP server" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "MCP permission mode" }), "allow-writes");

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith({ syncMode: "manual" });
      expect(api.settings.update).toHaveBeenCalledWith({ mcpEnabled: true });
      expect(api.settings.update).toHaveBeenCalledWith({ mcpPermissionMode: "allow-writes" });
    });
  });

  it("checks and pushes a configured HCB vault host from Settings", async () => {
    const api = seededHcb();
    let settings = testSettings({
      storageBackend: "hcb-hoster",
      hcbHosterEndpoint: "https://pi.local/hcb/v1/vault"
    });
    const manifest = {
      formatVersion: 1 as const,
      kind: "hot-cross-buns-vault" as const,
      exportedAt: "2026-05-22T00:00:00.000Z",
      appVersion: "0.0.0",
      stateEncoding: "hcb-portable-state-json" as const,
      stateSha256: "0".repeat(64),
      payloadFile: "payload.hcbenc" as const,
      payloadSha256: "1".repeat(64),
      encryption: {
        algorithm: "scrypt-AES-256-GCM" as const,
        kdf: "scrypt" as const,
        saltBase64: "salt",
        ivBase64: "iv",
        tagBase64: "tag",
        keyLength: 32 as const,
        cost: 32768,
        blockSize: 8,
        parallelization: 1
      },
      notes: ["Test vault."]
    };
    const remote = {
      kind: "hcbVaultHostInfo" as const,
      protocolVersion: 1 as const,
      hcbVaultFormatVersions: [1 as const],
      routes: ["/hcb/v1/vault/info", "/hcb/v1/vault"],
      hasVault: true,
      vaultName: "current.hcbvault",
      maxPackageBytes: 134217728,
      manifest
    };
    api.settings.get = vi.fn(async () => ok(settings));
    api.settings.update = vi.fn(async (request) => {
      settings = testSettings({ ...settings, ...request });
      return ok(settings);
    });
    api.settings.hcbVaultRemoteStatus = vi.fn(async (request) =>
      ok({ endpoint: request.endpoint ?? settings.hcbHosterEndpoint ?? "", remote })
    );
    api.settings.pushHcbVaultRemote = vi.fn(async (request) =>
      ok({
        endpoint: request.endpoint ?? settings.hcbHosterEndpoint ?? "",
        exportedAt: manifest.exportedAt,
        path: "/tmp/hcb-test.hcbvault",
        manifest,
        remote
      })
    );
    api.settings.saveHcbVaultRemoteCredentials = vi.fn(async (request) =>
      ok({
        endpoint: request.endpoint ?? settings.hcbHosterEndpoint ?? "",
        configured: true,
        secretStore: {
          ok: true,
          state: "ready",
          message: "Test credential storage is available."
        }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    fireEvent.change(screen.getByLabelText("HCB vault host token"), { target: { value: "remote-host-token" } });
    fireEvent.change(screen.getByLabelText("HCB vault passphrase"), { target: { value: "vault-passphrase" } });
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.settings.saveHcbVaultRemoteCredentials).toHaveBeenCalledWith({
        endpoint: "https://pi.local/hcb/v1/vault",
        token: "remote-host-token",
        passphrase: "vault-passphrase"
      });
    });

    await user.click(screen.getByRole("button", { name: "Check" }));

    await waitFor(() => {
      expect(api.settings.hcbVaultRemoteStatus).toHaveBeenCalledWith({
        endpoint: "https://pi.local/hcb/v1/vault",
        token: "remote-host-token",
        allowInsecureHttp: false
      });
    });

    await user.click(screen.getByRole("button", { name: /Push/ }));

    await waitFor(() => {
      expect(api.settings.pushHcbVaultRemote).toHaveBeenCalledWith({
        endpoint: "https://pi.local/hcb/v1/vault",
        token: "remote-host-token",
        passphrase: "vault-passphrase",
        allowInsecureHttp: false
      });
      expect(api.settings.update).toHaveBeenCalledWith({
        storageBackend: "hcb-hoster",
        hcbHosterEndpoint: "https://pi.local/hcb/v1/vault"
      });
    });
  });

  it("shows auto-tag regex validation, preview output, and rule conflicts", async () => {
    const api = seededHcb();
    let settings = testSettings({
      autoTagRules: [
        autoTagRule(),
        autoTagRule({
          id: "rule-github",
          name: "Github",
          matchType: "contains",
          pattern: "github",
          tags: ["github"],
          stripMatchedPrefix: false
        }),
        autoTagRule({
          id: "rule-invalid",
          name: "Invalid",
          matchType: "regex",
          pattern: "[",
          tags: ["bad"],
          stripMatchedPrefix: false
        })
      ]
    });
    api.settings.get = vi.fn(async () => ok(settings));
    api.settings.update = vi.fn(async (request) => {
      settings = testSettings({ ...settings, ...request });
      return ok(settings);
    });
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    const dialog = await screen.findByRole("dialog", { name: "Settings" });
    await user.click(within(dialog).getByRole("button", { name: "Advanced" }));

    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Auto-tag background reapply mode" }), "silent");
    await waitFor(() => {
      expect(settings.autoTagBackgroundReapplyMode).toBe("silent");
      expect(api.settings.update).toHaveBeenCalledWith({ autoTagBackgroundReapplyMode: "silent" });
    });
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Auto-tag background reapply mode" }), "manual");
    await waitFor(() => expect(settings.autoTagBackgroundReapplyMode).toBe("manual"));

    expect(within(dialog).getByTitle("1 issue")).toBeInTheDocument();
    expect(await within(dialog).findByText("1 auto-tag rule error need review.")).toBeInTheDocument();
    expect(within(dialog).getByText(/Invalid regex:/)).toBeInTheDocument();
    await waitFor(() => {
      expect(settings.autoTagRules.find((rule) => rule.id === "rule-invalid")?.enabled).toBe(false);
    });
    expect(within(dialog).getByRole("checkbox", { name: "Auto tag enabled Invalid" })).not.toBeChecked();

    fireEvent.change(within(dialog).getByRole("textbox", { name: "Auto tag preview title" }), {
      target: { value: "CODING: Look into github alternatives" }
    });
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Auto tag preview existing tags" }), {
      target: { value: "coding, ops" }
    });
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Auto tag preview explicit tags" }), {
      target: { value: "manual, ops" }
    });

    expect(within(dialog).getByText("2 matched")).toBeInTheDocument();
    expect(within(dialog).getByText("multiple rules")).toBeInTheDocument();
    expect(within(dialog).getByText("Title: Look into github alternatives")).toBeInTheDocument();
    expect(within(dialog).getByText("Tags: coding, ops, manual, github")).toBeInTheDocument();
    expect(within(dialog).getByText("preview invalid")).toBeInTheDocument();

    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Auto tag preview target" }), "event");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Auto tag event color Coding" }), "5");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Auto tag preview requested event color" }), "3");
    await waitFor(() => expect(within(dialog).getByText("color kept")).toBeInTheDocument());
    expect(within(dialog).getByText("color 3")).toBeInTheDocument();

    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Auto tag preview local kind" }), "birthday");
    expect(within(dialog).getByText("Rules skipped for birthday preview.")).toBeInTheDocument();
    expect(within(dialog).getByText("Title: CODING: Look into github alternatives")).toBeInTheDocument();
    expect(within(dialog).getByText("Tags: coding, ops, manual")).toBeInTheDocument();

    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Auto tag preview local kind" }), "normal");
    await user.click(within(dialog).getByRole("button", { name: "Move auto tag rule Github up" }));
    await waitFor(() => {
      expect(settings.autoTagRules.map((rule) => rule.id)).toEqual(["rule-github", "rule-coding", "rule-invalid"]);
    });
  });

  it("previews and applies background auto-tag reapply after rule changes", async () => {
    const api = seededHcb();
    let settings = testSettings({
      autoTagRules: [
        autoTagRule(),
        autoTagRule({
          id: "rule-github",
          name: "Github",
          matchType: "contains",
          pattern: "github",
          tags: ["github"],
          stripMatchedPrefix: false
        })
      ]
    });
    api.settings.get = vi.fn(async () => ok(settings));
    api.settings.update = vi.fn(async (request) => {
      settings = testSettings({ ...settings, ...request });
      return ok(settings);
    });
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    const dialog = await screen.findByRole("dialog", { name: "Settings" });
    await user.click(within(dialog).getByRole("button", { name: "Advanced" }));
    await user.click(within(dialog).getByRole("button", { name: "Move auto tag rule Github up" }));

    expect(await within(dialog).findByText("Auto-tag reapply preview", undefined, { timeout: 2_500 })).toBeInTheDocument();
    expect(within(dialog).getByText("Rules changed. 1 task needs reapply.")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Apply" }));

    await waitFor(() => {
      expect(api.tags.previewAutoReapply).toHaveBeenCalledWith({ kind: "task", scope: "all" });
      expect(api.tags.previewAutoReapply).toHaveBeenCalledWith({ kind: "event", scope: "all" });
      expect(api.tags.previewAutoReapply).toHaveBeenCalledWith({ kind: "note", scope: "all" });
      expect(api.tags.applyAutoReapply).toHaveBeenCalledWith({ kind: "task", scope: "all", confirm: true });
    });
    expect(within(dialog).getByText("1 updated.")).toBeInTheDocument();
  });

  it("edits task and event templates from settings dialogs", async () => {
    const api = seededHcb();
    let settings = testSettings({ taskTemplates: [], eventTemplates: [] });
    api.settings.get = vi.fn(async () => ok(settings));
    api.settings.update = vi.fn(async (request) => {
      settings = testSettings({ ...settings, ...request });
      return ok(settings);
    });
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    const dialog = await screen.findByRole("dialog", { name: "Settings" });
    await user.click(within(dialog).getByRole("button", { name: "Advanced" }));

    await user.click(within(dialog).getByRole("button", { name: "New Task Template" }));
    const taskDialog = await screen.findByRole("dialog", { name: "Edit task template" });
    const taskTitle = within(taskDialog).getByRole("textbox", { name: "Task template title" });
    await user.clear(taskTitle);
    await user.type(taskTitle, "Ship title");
    fireEvent.change(within(taskDialog).getByRole("textbox", { name: "Task template due expression" }), {
      target: { value: "{{today}} 18:00" }
    });
    await user.click(within(taskDialog).getByRole("button", { name: "Done" }));
    await waitFor(() => {
      expect(settings.taskTemplates[0]).toEqual(expect.objectContaining({
        title: "Ship title",
        dueExpression: "{{today}} 18:00"
      }));
    });

    await user.click(within(dialog).getByRole("button", { name: "New Event Template" }));
    const eventDialog = await screen.findByRole("dialog", { name: "Edit event template" });
    const eventTitle = within(eventDialog).getByRole("textbox", { name: "Event template title" });
    await user.clear(eventTitle);
    await user.type(eventTitle, "Review topic");
    fireEvent.change(within(eventDialog).getByRole("textbox", { name: "Event template attendees" }), {
      target: { value: "ada@example.com, bad" }
    });
    await user.click(within(eventDialog).getByRole("button", { name: "Done" }));
    await waitFor(() => {
      expect(settings.eventTemplates[0]).toEqual(expect.objectContaining({
        title: "Review topic",
        attendeeEmails: ["ada@example.com"]
      }));
    });
  });

  it("hides signed-out local Google account rows and empty account filters", async () => {
    const api = seededHcb();
    const google = connectedGoogleStatus({
      accounts: [
        connectedGoogleStatus().accounts[0],
        {
          accountId: "local-google-account",
          connectionState: "signed_out",
          grantedScopes: [],
          missingScopes: [],
          updatedAt: now
        }
      ]
    });
    api.google.status = vi.fn(async () => ok(google));
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    const dialog = await screen.findByRole("dialog", { name: "Settings" });
    await user.click(within(dialog).getByRole("button", { name: "Profile" }));

    expect(within(dialog).getByText("Planner Test")).toBeInTheDocument();
    expect(within(dialog).queryByText("signed_out")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Account" })).not.toBeInTheDocument();
  });

  it("shows onboarding for a fresh database and completes setup through settings IPC", async () => {
    const { api, getSettings } = onboardingHcb();
    api.google.status = vi.fn(async () => ok(connectedGoogleStatus()));
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    const dialog = await screen.findByRole("dialog", { name: "First-run setup" });

    expect(within(dialog).getByText("1. Backend")).toBeInTheDocument();
    expect(within(dialog).getByText("2. Google account")).toBeInTheDocument();
    expect(within(dialog).getByText("Connected as Planner Test.")).toBeInTheDocument();
    expect(within(dialog).getByText("3. Task lists")).toBeInTheDocument();
    expect(within(dialog).getByText("4. Calendars")).toBeInTheDocument();
    expect(within(dialog).getByText("5. Sync mode")).toBeInTheDocument();
    expect(within(dialog).getByText("6. Notifications")).toBeInTheDocument();
    expect(within(dialog).getByText("7. MCP access")).toBeInTheDocument();

    await user.selectOptions(within(dialog).getByLabelText("Onboarding sync mode"), "near-real-time");
    await user.click(within(dialog).getByLabelText("Local notifications"));
    await user.click(within(dialog).getByLabelText("Enable MCP"));
    await user.click(within(dialog).getByRole("button", { name: "Finish setup" }));

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith(
        expect.objectContaining({
          selectedTaskListIds: ["list-inbox", "list-planning"],
          selectedCalendarIds: ["cal-product"],
          storageBackend: "google",
          syncMode: "near-real-time",
          notificationsEnabled: true,
          mcpEnabled: true,
          setupCompletedAt: expect.any(String)
        })
      );
      expect(getSettings().setupCompletedAt).toEqual(expect.any(String));
      expect(screen.queryByRole("dialog", { name: "First-run setup" })).not.toBeInTheDocument();
    });
  });

  it("allows local backend setup without Google", async () => {
    const { api } = onboardingHcb();
    api.google.status = vi.fn(async () => ok(signedOutGoogleStatus()));
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    const dialog = await screen.findByRole("dialog", { name: "First-run setup" });
    expect(within(dialog).getByRole("button", { name: "Finish setup" })).toBeDisabled();

    await user.selectOptions(within(dialog).getByLabelText("Onboarding storage backend"), "hcb-local");
    expect(within(dialog).getByRole("button", { name: "Finish setup" })).toBeEnabled();
    await user.click(within(dialog).getByRole("button", { name: "Finish setup" }));

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith(
        expect.objectContaining({
          storageBackend: "hcb-local",
          hcbHosterEndpoint: null,
          setupCompletedAt: expect.any(String)
        })
      );
    });
  });

  it("saves Google OAuth client from first-run setup", async () => {
    const { api } = onboardingHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    const dialog = await screen.findByRole("dialog", { name: "First-run setup" });
    const connectButton = await within(dialog).findByRole("button", { name: "Connect Google" });
    expect(connectButton).toBeDisabled();

    await user.type(
      within(dialog).getByRole("textbox", { name: "Google OAuth client ID" }),
      "desktop-client-id.apps.googleusercontent.com"
    );
    await user.click(within(dialog).getByRole("button", { name: "Save OAuth Client" }));

    await waitFor(() => {
      expect(api.google.saveOAuthClient).toHaveBeenCalledWith({
        clientId: "desktop-client-id.apps.googleusercontent.com"
      });
      expect(within(dialog).getByText("Google OAuth client saved.")).toBeInTheDocument();
    });
    expect(api.google.beginOAuth).not.toHaveBeenCalled();
  });

  it("starts Google OAuth from first-run setup", async () => {
    const { api } = onboardingHcb();
    api.google.status = vi.fn(async () =>
      ok(signedOutGoogleStatus({
        oauthClientConfigured: true,
        clientId: "desktop-client-id.apps.googleusercontent.com"
      }))
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    const dialog = await screen.findByRole("dialog", { name: "First-run setup" });
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "Connect Google" })).toBeEnabled()
    );

    await user.click(within(dialog).getByRole("button", { name: "Connect Google" }));

    await waitFor(() => {
      expect(api.google.beginOAuth).toHaveBeenCalled();
      expect(within(dialog).getByText("Google authorization opened in the browser.")).toBeInTheDocument();
      expect(within(dialog).getByRole("button", { name: "Finish setup" })).toBeDisabled();
    });
  });

  it("requires Google when setup was completed but the account is signed out", async () => {
    const { api } = onboardingHcb({ setupCompletedAt: now });
    api.google.status = vi.fn(async () => ok(signedOutGoogleStatus()));
    installHcb(api);
    render(<App />);

    const dialog = await screen.findByRole("dialog", { name: "First-run setup" });
    expect(within(dialog).queryByRole("button", { name: "Continue without sync" })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Finish setup" })).toBeDisabled();
    expect(api.settings.update).not.toHaveBeenCalled();
  });

  it("resets onboarding from Settings without deleting planner data", async () => {
    const { api } = onboardingHcb({ setupCompletedAt: now });
    api.google.status = vi.fn(async () => ok(connectedGoogleStatus()));
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    expect(await screen.findByText("Startup data flow")).toBeInTheDocument();

    await goToSection("Settings");
    await user.click(screen.getByRole("button", { name: "Run setup again" }));

    await waitFor(() => {
      expect(api.settings.recoveryAction).toHaveBeenCalledWith({ action: "resetOnboarding" });
    });
    expect(await screen.findByRole("dialog", { name: "First-run setup" })).toBeInTheDocument();
    expect(api.notes.delete).not.toHaveBeenCalled();
  });

  it("updates startup setting from Settings", async () => {
    const api = seededHcb();
    api.settings.get = vi.fn(async () => ok(testSettings({ startOnLogin: false })));
    api.settings.update = vi.fn(async (request) =>
      ok(testSettings({ startOnLogin: request.startOnLogin ?? false }))
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    await user.click(screen.getByRole("checkbox", { name: "Open Hot Cross Buns at login" }));

    await waitFor(() => {
      expect(api.settings.update).toHaveBeenCalledWith({ startOnLogin: true });
    });
  });

  it("requires confirmation before destructive sync recovery actions", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Settings");
    await user.click(screen.getByRole("button", { name: /Force full resync/ }));

    expect(api.settings.recoveryAction).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "Confirm destructive action" })).toBeInTheDocument();

    const confirmButton = screen.getByRole("button", { name: "Confirm" });
    expect(confirmButton).toBeDisabled();

    await user.type(screen.getByRole("textbox", { name: "Confirmation phrase" }), "FULL RESYNC");
    await user.click(confirmButton);

    await waitFor(() => {
      expect(api.settings.recoveryAction).toHaveBeenCalledWith({
        action: "forceFullResync",
        confirmation: {
          accepted: true,
          phrase: "FULL RESYNC"
        }
      });
    });
  });
});
