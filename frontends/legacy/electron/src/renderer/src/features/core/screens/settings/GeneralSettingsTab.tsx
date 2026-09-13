import type {
  SettingsRecoveryActionRequest,
  SettingsSnapshot,
  SettingsUpdateRequest
} from "@shared/ipc/contracts";
import { useEffect, useState } from "react";
import {
  CloudDownload,
  CloudUpload,
  HardDrive,
  FileSearch,
  Languages,
  Power,
  RotateCcw,
  Server,
  ShieldCheck,
  Sparkles
} from "lucide-react";
import { Button, Input } from "../../../../components/primitives";
import { languageOptions, useI18n } from "../../../../i18n";
import {
  SettingsControlRow,
  SettingsGroup,
  SettingsSwitch,
  settingsSelectClass
} from "./SettingsPrimitives";
import { retentionOptions } from "./settingsUtils";

interface GeneralSettingsTabProps {
  beginRecoveryAction: (action: SettingsRecoveryActionRequest["action"]) => void;
  customRetentionAmount: string;
  customRetentionUnit: "days" | "months" | "years";
  openDiagnosticsDetails: () => Promise<void>;
  setCustomRetentionAmount: (value: string) => void;
  setCustomRetentionUnit: (value: "days" | "months" | "years") => void;
  settings: SettingsSnapshot;
  settingsMutationPending: boolean;
  updateSettings: (request: SettingsUpdateRequest) => void;
}

export function GeneralSettingsTab({
  beginRecoveryAction,
  customRetentionAmount,
  customRetentionUnit,
  openDiagnosticsDetails,
  setCustomRetentionAmount,
  setCustomRetentionUnit,
  settings,
  settingsMutationPending,
  updateSettings
}: GeneralSettingsTabProps): JSX.Element {
  const { t } = useI18n();
  const [vaultHostToken, setVaultHostToken] = useState("");
  const [vaultPassphrase, setVaultPassphrase] = useState("");
  const [vaultAllowInsecureHttp, setVaultAllowInsecureHttp] = useState(false);
  const [vaultCredentialsConfigured, setVaultCredentialsConfigured] = useState(false);
  const [vaultHostStatus, setVaultHostStatus] = useState("");
  const [vaultHostPending, setVaultHostPending] = useState(false);

  function retentionPresetValue(daysBack: number): string {
    return retentionOptions.some((option) => option.value === daysBack) ? String(daysBack) : "custom";
  }

  function customRetentionDays(): number {
    const amount = Math.max(1, Math.round(Number(customRetentionAmount) || 1));

    if (customRetentionUnit === "years") {
      return Math.min(3650, amount * 365);
    }

    if (customRetentionUnit === "months") {
      return Math.min(3650, amount * 30);
    }

    return Math.min(3650, amount);
  }

  function applyCustomRetention(): void {
    const days = customRetentionDays();

    updateSettings({
      eventRetentionDaysBack: days,
      completedTaskRetentionDaysBack: days
    });
  }

  useEffect(() => {
    if (settings.storageBackend !== "hcb-hoster" || !settings.hcbHosterEndpoint) {
      setVaultCredentialsConfigured(false);
      return;
    }

    let cancelled = false;
    void window.hcb?.settings.hcbVaultRemoteCredentialStatus({
      endpoint: settings.hcbHosterEndpoint
    }).then((result) => {
      if (!cancelled && result?.ok) {
        setVaultCredentialsConfigured(result.data.configured);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [settings.hcbHosterEndpoint, settings.storageBackend]);

  async function runVaultHostAction(action: "status" | "push" | "pull"): Promise<void> {
    const endpoint = settings.hcbHosterEndpoint?.trim();
    const token = vaultHostToken.trim();
    const passphrase = vaultPassphrase;

    if (!endpoint) {
      setVaultHostStatus("Set a vault host endpoint first.");
      return;
    }

    if (!token && !vaultCredentialsConfigured) {
      setVaultHostStatus("Enter the vault host token.");
      return;
    }

    if (action !== "status" && passphrase.length < 8 && !vaultCredentialsConfigured) {
      setVaultHostStatus("Enter the vault passphrase.");
      return;
    }

    if (action === "pull" && !window.confirm("Pulling replaces local HCB state after creating a backup.")) {
      return;
    }

    setVaultHostPending(true);
    setVaultHostStatus("Working.");

    try {
      if (action === "status") {
        const result = await window.hcb?.settings.hcbVaultRemoteStatus({
          endpoint,
          ...(token ? { token } : {}),
          allowInsecureHttp: vaultAllowInsecureHttp
        });
        setVaultHostStatus(
          result?.ok
            ? `Host reachable. Vault ${result.data.remote.hasVault ? "present" : "empty"}.`
            : result?.error.message ?? "Vault host status failed."
        );
        return;
      }

      if (action === "push") {
        const result = await window.hcb?.settings.pushHcbVaultRemote({
          endpoint,
          ...(token ? { token } : {}),
          ...(passphrase ? { passphrase } : {}),
          allowInsecureHttp: vaultAllowInsecureHttp
        });
        if (result?.ok) {
          updateSettings({ storageBackend: "hcb-hoster", hcbHosterEndpoint: result.data.endpoint });
        }
        setVaultHostStatus(
          result?.ok
            ? `Pushed vault ${new Date(result.data.exportedAt).toLocaleString()}.`
            : result?.error.message ?? "Vault push failed."
        );
        return;
      }

      const result = await window.hcb?.settings.pullHcbVaultRemote({
        endpoint,
        ...(token ? { token } : {}),
        ...(passphrase ? { passphrase } : {}),
        allowInsecureHttp: vaultAllowInsecureHttp,
        confirm: true
      });
      if (result?.ok) {
        updateSettings({ storageBackend: "hcb-hoster", hcbHosterEndpoint: result.data.endpoint });
      }
      setVaultHostStatus(
        result?.ok
          ? `Pulled vault. Backup: ${result.data.backupPath}`
          : result?.error.message ?? "Vault pull failed."
      );
    } finally {
      setVaultHostPending(false);
    }
  }

  async function saveVaultHostCredentials(): Promise<void> {
    const endpoint = settings.hcbHosterEndpoint?.trim();
    if (!endpoint || !vaultHostToken.trim() || vaultPassphrase.length < 8) {
      setVaultHostStatus("Set endpoint, token, and passphrase before saving.");
      return;
    }

    setVaultHostPending(true);
    try {
      const result = await window.hcb?.settings.saveHcbVaultRemoteCredentials({
        endpoint,
        token: vaultHostToken.trim(),
        passphrase: vaultPassphrase
      });
      setVaultCredentialsConfigured(Boolean(result?.ok && result.data.configured));
      setVaultHostStatus(result?.ok ? "Vault host credentials saved." : result?.error.message ?? "Credential save failed.");
    } finally {
      setVaultHostPending(false);
    }
  }

  async function deleteVaultHostCredentials(): Promise<void> {
    const endpoint = settings.hcbHosterEndpoint?.trim();
    if (!endpoint) {
      setVaultHostStatus("Set a vault host endpoint first.");
      return;
    }

    setVaultHostPending(true);
    try {
      const result = await window.hcb?.settings.deleteHcbVaultRemoteCredentials({ endpoint });
      setVaultCredentialsConfigured(Boolean(result?.ok && result.data.configured));
      setVaultHostStatus(result?.ok ? "Vault host credentials removed." : result?.error.message ?? "Credential delete failed.");
    } finally {
      setVaultHostPending(false);
    }
  }

  return (
    <div className="grid gap-5">
      <SettingsGroup title={t("settings.language")}>
        <SettingsControlRow
          description={t("language.description")}
          icon={Languages}
          label={t("settings.appLanguage")}
        >
          <select
            aria-label={t("settings.appLanguage")}
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({ appLanguage: event.target.value as SettingsSnapshot["appLanguage"] })
            }
            value={settings.appLanguage}
          >
            {languageOptions(t).map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title="Startup">
        <SettingsSwitch
          checked={settings.startOnLogin}
          description="Starts the app automatically when you sign in to this Mac."
          icon={Power}
          label="Open Hot Cross Buns at login"
          onChange={(checked) => updateSettings({ startOnLogin: checked })}
        />
      </SettingsGroup>

      <SettingsGroup title="Storage">
        <SettingsControlRow
          description="Switches between Google sync, encrypted local vault mode, and a trusted HCB vault host endpoint."
          icon={HardDrive}
          label="Backend"
        >
          <select
            aria-label="Storage backend"
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({ storageBackend: event.target.value as SettingsSnapshot["storageBackend"] })
            }
            value={settings.storageBackend}
          >
            <option value="google">Google Tasks/Calendar</option>
            <option value="hcb-local">HCB local vault</option>
            <option value="hcb-hoster">HCB local hoster</option>
          </select>
        </SettingsControlRow>
        {settings.storageBackend === "hcb-hoster" ? (
          <>
            <SettingsControlRow
              description="HTTPS endpoint for a trusted HCB vault host. Loopback HTTP is accepted for local testing."
              label="Vault host endpoint"
            >
              <Input
                aria-label="HCB vault host endpoint"
                defaultValue={settings.hcbHosterEndpoint ?? ""}
                onBlur={(event) =>
                  updateSettings({
                    hcbHosterEndpoint:
                      event.currentTarget.value.trim().length > 0
                        ? event.currentTarget.value.trim()
                        : null
                  })
                }
                placeholder="https://pi.local/hcb/v1/vault"
              />
            </SettingsControlRow>
            <SettingsControlRow
              description="Token is used for this action only and is not saved."
              label="Vault host token"
            >
              <Input
                aria-label="HCB vault host token"
                onChange={(event) => setVaultHostToken(event.currentTarget.value)}
                placeholder="host token"
                type="password"
                value={vaultHostToken}
              />
            </SettingsControlRow>
            <SettingsControlRow
              description="Passphrase encrypts and decrypts the .hcbvault payload on this device."
              label="Vault passphrase"
            >
              <Input
                aria-label="HCB vault passphrase"
                onChange={(event) => setVaultPassphrase(event.currentTarget.value)}
                placeholder="vault passphrase"
                type="password"
                value={vaultPassphrase}
              />
            </SettingsControlRow>
            <SettingsControlRow
              description={vaultCredentialsConfigured ? "Saved in OS credential storage for Refresh and scheduled sync." : "Save credentials to let Refresh push the encrypted vault."}
              label="Saved credentials"
            >
              <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
                <Button
                  disabled={vaultHostPending}
                  onClick={() => void saveVaultHostCredentials()}
                  variant="secondary"
                >
                  Save
                </Button>
                <Button
                  disabled={vaultHostPending || !vaultCredentialsConfigured}
                  onClick={() => void deleteVaultHostCredentials()}
                  variant="secondary"
                >
                  Forget
                </Button>
              </div>
            </SettingsControlRow>
            <SettingsSwitch
              checked={vaultAllowInsecureHttp}
              description="Only use for trusted LAN or tunnel endpoints."
              label="Allow non-loopback HTTP"
              onChange={setVaultAllowInsecureHttp}
            />
            <SettingsControlRow
              description={vaultHostStatus || "Status, push, and pull operate on encrypted .hcbvault packages."}
              label="Vault host"
            >
              <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
                <Button
                  disabled={vaultHostPending}
                  onClick={() => void runVaultHostAction("status")}
                  variant="secondary"
                >
                  Check
                </Button>
                <Button
                  disabled={vaultHostPending}
                  onClick={() => void runVaultHostAction("push")}
                  variant="secondary"
                >
                  <CloudUpload aria-hidden="true" size={14} />
                  Push
                </Button>
                <Button
                  disabled={vaultHostPending}
                  onClick={() => void runVaultHostAction("pull")}
                  variant="danger"
                >
                  <CloudDownload aria-hidden="true" size={14} />
                  Pull
                </Button>
              </div>
            </SettingsControlRow>
          </>
        ) : null}
        <SettingsControlRow
          description="Export/import encrypted HCB vaults locally, or push/pull them with the CLI vault host commands."
          label="Vault path"
        >
          <Input
            aria-label="HCB vault path"
            defaultValue={settings.hcbVaultPath ?? ""}
            onBlur={(event) =>
              updateSettings({
                hcbVaultPath:
                  event.currentTarget.value.trim().length > 0
                    ? event.currentTarget.value.trim()
                    : null
              })
            }
            placeholder="unset"
          />
        </SettingsControlRow>
        <SettingsSwitch
          checked={settings.localHostersEnabled}
          description="Starts the loopback HCB signal hoster for local terminal automation."
          label="Signal hoster server"
          onChange={(checked) => updateSettings({ localHostersEnabled: checked })}
        />
        <SettingsControlRow label="Signal hoster port">
          <Input
            aria-label="Signal hoster port"
            defaultValue={String(settings.localHosterPort)}
            max={65535}
            min={0}
            onBlur={(event) => updateSettings({ localHosterPort: Number(event.currentTarget.value) || 0 })}
            type="number"
          />
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title={t("diagnostics.title")}>
        <SettingsControlRow
          description={t("diagnostics.description")}
          icon={ShieldCheck}
          label={t("diagnostics.title")}
        >
          <Button onClick={() => void openDiagnosticsDetails()} variant="secondary">
            <FileSearch aria-hidden="true" size={14} />
            {t("action.viewDiagnostics")}
          </Button>
        </SettingsControlRow>
        <SettingsSwitch
          checked={settings.diagnosticsIncludePerformance}
          description={t("diagnostics.includePerformance.description")}
          label={t("diagnostics.includePerformance")}
          onChange={(checked) => updateSettings({ diagnosticsIncludePerformance: checked })}
        />
        <SettingsSwitch
          checked={settings.rawGoogleDiagnosticsEnabled}
          description={t("diagnostics.includeGooglePayloads.description")}
          label={t("diagnostics.includeGooglePayloads")}
          onChange={(checked) => updateSettings({ rawGoogleDiagnosticsEnabled: checked })}
        />
      </SettingsGroup>

      <SettingsGroup title="Agent access">
        <SettingsSwitch
          checked={settings.mcpEnabled}
          icon={Server}
          label="Local MCP server"
          onChange={(checked) => updateSettings({ mcpEnabled: checked })}
        />
        <SettingsControlRow
          description="MCP clients must follow this write policy before changes apply."
          label="Permission mode"
        >
          <select
            aria-label="MCP permission mode"
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({
                mcpPermissionMode: event.target.value as SettingsSnapshot["mcpPermissionMode"]
              })
            }
            value={settings.mcpPermissionMode}
          >
            <option value="read-only">Read-only</option>
            <option value="confirm-writes">Confirm writes</option>
            <option value="allow-writes">Allow writes</option>
          </select>
        </SettingsControlRow>
        <SettingsControlRow label="Port">
          <Input
            aria-label="MCP port"
            max={65535}
            min={0}
            onBlur={(event) => updateSettings({ mcpPort: Number(event.currentTarget.value) || 0 })}
            defaultValue={String(settings.mcpPort)}
            type="number"
          />
        </SettingsControlRow>
        <SettingsControlRow
          description={settings.mcpEnabled ? "The local MCP server is enabled." : "The local MCP server is disabled."}
          label={settings.mcpEnabled ? "Running" : "Stopped"}
        >
          <Button
            disabled={settingsMutationPending}
            onClick={() => beginRecoveryAction("resetMcpToken")}
            variant="secondary"
          >
            <RotateCcw aria-hidden="true" size={14} />
            Reset token
          </Button>
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title="Sync">
        <SettingsControlRow
          description="Refresh cadence for launch, foreground, and periodic app activity."
          label="Mode"
        >
          <select
            aria-label="Sync mode"
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({ syncMode: event.target.value as SettingsSnapshot["syncMode"] })
            }
            value={settings.syncMode}
          >
            <option value="manual">Manual</option>
            <option value="balanced">Balanced</option>
            <option value="near-real-time">Near real-time</option>
          </select>
        </SettingsControlRow>
        <SettingsControlRow label="Keep past events">
          <select
            aria-label="Keep past events"
            className={settingsSelectClass}
            onChange={(event) => {
              if (event.target.value !== "custom") {
                updateSettings({ eventRetentionDaysBack: Number(event.target.value) });
              }
            }}
            value={retentionPresetValue(settings.eventRetentionDaysBack)}
          >
            {retentionOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
            <option value="custom">Custom</option>
          </select>
        </SettingsControlRow>
        <SettingsControlRow label="Keep completed tasks">
          <select
            aria-label="Keep completed tasks"
            className={settingsSelectClass}
            onChange={(event) => {
              if (event.target.value !== "custom") {
                updateSettings({ completedTaskRetentionDaysBack: Number(event.target.value) });
              }
            }}
            value={retentionPresetValue(settings.completedTaskRetentionDaysBack)}
          >
            {retentionOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
            <option value="custom">Custom</option>
          </select>
        </SettingsControlRow>
        <SettingsControlRow
          description="Applies the same retention window to past events and completed tasks."
          label="Custom"
        >
          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
            <Input
              aria-label="Custom retention amount"
              className="w-24"
              min={1}
              onChange={(event) => setCustomRetentionAmount(event.currentTarget.value)}
              type="number"
              value={customRetentionAmount}
            />
            <select
              aria-label="Custom retention unit"
              className={settingsSelectClass}
              onChange={(event) => setCustomRetentionUnit(event.target.value as "days" | "months" | "years")}
              value={customRetentionUnit}
            >
              <option value="days">Days</option>
              <option value="months">Months</option>
              <option value="years">Years</option>
            </select>
            <Button onClick={applyCustomRetention} variant="primary">Apply</Button>
          </div>
        </SettingsControlRow>
        <div className="flex flex-wrap items-center gap-2 px-3 py-3">
          <Button onClick={() => beginRecoveryAction("refresh")} variant="secondary">
            <RotateCcw aria-hidden="true" size={14} />
            Refresh
          </Button>
          <Button onClick={() => beginRecoveryAction("forceFullResync")} variant="danger">
            Force full resync
          </Button>
        </div>
      </SettingsGroup>

      <SettingsGroup title="Setup">
        <SettingsControlRow
          description="Clears onboarding completion so setup opens again."
          icon={Sparkles}
          label="Setup assistant"
        >
          <Button onClick={() => beginRecoveryAction("resetOnboarding")} variant="secondary">
            Run setup again
          </Button>
        </SettingsControlRow>
      </SettingsGroup>
    </div>
  );
}
