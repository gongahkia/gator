import type {
  DiagnosticsSummaryResponse,
  NativeUpdaterStatus,
  SettingsRecoveryActionRequest,
  SettingsSnapshot
} from "@shared/ipc/contracts";
import { Copy, Download, ExternalLink, Info } from "lucide-react";
import { Badge, Button } from "../../../../components/primitives";
import { SettingsControlRow, SettingsGroup, SettingsSwitch } from "./SettingsPrimitives";

interface AboutSettingsTabProps {
  beginRecoveryAction: (action: SettingsRecoveryActionRequest["action"]) => void;
  diagnostics?: DiagnosticsSummaryResponse;
  nativeUpdaterStatus: NativeUpdaterStatus;
  settings: SettingsSnapshot;
  updateSettings: (request: { lastUpdateCheckAt?: string | null }) => void;
}

export function AboutSettingsTab({
  beginRecoveryAction,
  diagnostics,
  nativeUpdaterStatus,
  settings,
  updateSettings
}: AboutSettingsTabProps): JSX.Element {
  const build = diagnostics?.build;
  const version = build?.version ?? "0.0.0";
  const commit = build?.commit ?? "Not recorded";
  const environment = build?.environment ?? "development";
  const versionInfo = [
    "Hot Cross Buns",
    `Version: ${version}`,
    `Build: ${commit}`,
    `Environment: ${environment}`
  ].join("\n");

  function copyVersionInfo(): void {
    void navigator.clipboard?.writeText(versionInfo);
  }

  function openExternalUrl(url: string): void {
    void window.hcb?.native.openExternalUrl({ url });
  }

  return (
    <div className="grid gap-5">
      <SettingsGroup title="Updates">
        <SettingsSwitch
          checked={settings.lastUpdateCheckAt !== null}
          label="Check GitHub releases automatically"
          onChange={(checked) =>
            updateSettings({ lastUpdateCheckAt: checked ? new Date().toISOString() : null })
          }
        />
        <SettingsControlRow
          description="Checks release metadata through the native updater status path."
          icon={Download}
          label="Manual check"
        >
          <Button onClick={() => beginRecoveryAction("checkForUpdates")} variant="secondary">
            <Download aria-hidden="true" size={14} />
            Check for Updates Now
          </Button>
        </SettingsControlRow>
        <SettingsControlRow label="Update status">
          <span className="flex flex-wrap items-center justify-end gap-2 text-[var(--text-sm)] text-text-muted">
            <Badge tone={nativeUpdaterStatus.updateAvailable ? "warning" : nativeUpdaterStatus.state === "error" ? "danger" : "success"}>
              {nativeUpdaterStatus.updateAvailable ? "Update available" : nativeUpdaterStatus.state}
            </Badge>
            {nativeUpdaterStatus.message ?? "No update status reported"}
          </span>
        </SettingsControlRow>
        {nativeUpdaterStatus.latestVersion ? (
          <SettingsControlRow label="Latest release">
            <span className="text-[var(--text-sm)] text-text-muted">
              {nativeUpdaterStatus.releaseName ?? nativeUpdaterStatus.latestVersion}
            </span>
          </SettingsControlRow>
        ) : null}
        {nativeUpdaterStatus.releaseUrl || nativeUpdaterStatus.downloadUrl ? (
          <SettingsControlRow label="Release actions">
            <div className="flex flex-wrap justify-end gap-2">
              {nativeUpdaterStatus.releaseUrl ? (
                <Button onClick={() => openExternalUrl(nativeUpdaterStatus.releaseUrl as string)} size="sm" variant="secondary">
                  <ExternalLink aria-hidden="true" size={14} />
                  Open release
                </Button>
              ) : null}
              {nativeUpdaterStatus.downloadUrl ? (
                <Button onClick={() => openExternalUrl(nativeUpdaterStatus.downloadUrl as string)} size="sm" variant="secondary">
                  <Download aria-hidden="true" size={14} />
                  Open download
                </Button>
              ) : null}
            </div>
          </SettingsControlRow>
        ) : null}
        <SettingsControlRow label="Last checked">
          <span className="text-[var(--text-sm)] text-text-muted">
            {nativeUpdaterStatus.checkedAt ?? settings.lastUpdateCheckAt ?? "Never"}
          </span>
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title="About">
        <SettingsControlRow icon={Info} label="App">
          <span className="text-[var(--text-md)] font-semibold text-text-secondary">Hot Cross Buns</span>
        </SettingsControlRow>
        <SettingsControlRow label="Version">
          <span className="font-mono text-[var(--text-sm)] text-text-muted">{version}</span>
        </SettingsControlRow>
        <SettingsControlRow label="Build">
          <span className="font-mono text-[var(--text-sm)] text-text-muted">{commit}</span>
        </SettingsControlRow>
        <SettingsControlRow label="Bundle ID">
          <span className="font-mono text-[var(--text-sm)] text-text-muted">
            com.gongahkia.hotcrossbuns.mac
          </span>
        </SettingsControlRow>
        <SettingsControlRow label="Copy version info">
          <Button onClick={copyVersionInfo} variant="secondary">
            <Copy aria-hidden="true" size={14} />
            Copy version info
          </Button>
        </SettingsControlRow>
      </SettingsGroup>
    </div>
  );
}
