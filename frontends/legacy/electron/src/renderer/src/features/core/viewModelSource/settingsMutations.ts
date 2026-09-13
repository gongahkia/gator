import { useCallback, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import type {
  SettingsRecoveryActionRequest,
  SettingsRecoveryActionResponse,
  SettingsUpdateRequest
} from "@shared/ipc/contracts";
import type { CoreDataLoadState, SettingsMutationUiState } from "./types";

interface UseSettingsMutationsOptions {
  load: () => void;
  refreshDiagnosticsSummary: () => void;
  refreshSyncStatus: () => void;
  setLoadState: Dispatch<SetStateAction<CoreDataLoadState>>;
}

export function useSettingsMutations({
  load,
  refreshDiagnosticsSummary,
  refreshSyncStatus,
  setLoadState
}: UseSettingsMutationsOptions): {
  settingsMutation: SettingsMutationUiState;
  updateSettings: (request: SettingsUpdateRequest) => Promise<boolean>;
  runRecoveryAction: (
    request: SettingsRecoveryActionRequest
  ) => Promise<SettingsRecoveryActionResponse | null>;
} {
  const [settingsMutation, setSettingsMutation] = useState<SettingsMutationUiState>({
    pending: false
  });

  const updateSettings = useCallback(
    async (request: SettingsUpdateRequest): Promise<boolean> => {
      if (!window.hcb) {
        setSettingsMutation({
          pending: false,
          error: "Settings require the preload bridge."
        });
        return false;
      }

      setSettingsMutation({ pending: true });
      const result = await window.hcb.settings.update(request);

      if (result.ok) {
        const nativeResult = await window.hcb.native.capabilities();
        setLoadState((current) => ({
          ...current,
          snapshot: {
            ...current.snapshot,
            settings: result.data,
            ...(nativeResult.ok ? { native: nativeResult.data } : {})
          }
        }));
        setSettingsMutation({ pending: false });
        refreshDiagnosticsSummary();
        return true;
      }

      setSettingsMutation({
        pending: false,
        error: result.error.message
      });
      return false;
    },
    [refreshDiagnosticsSummary, setLoadState]
  );

  const runRecoveryAction = useCallback(
    async (
      request: SettingsRecoveryActionRequest
    ): Promise<SettingsRecoveryActionResponse | null> => {
      if (!window.hcb) {
        setSettingsMutation({
          pending: false,
          error: "Recovery actions require the preload bridge."
        });
        return null;
      }

      setSettingsMutation({ pending: true });
      const result = await window.hcb.settings.recoveryAction(request);

      if (result.ok) {
        const [settingsResult, nativeResult] = await Promise.all([
          window.hcb.settings.get(),
          window.hcb.native.capabilities()
        ]);
        setLoadState((current) => ({
          ...current,
          snapshot: {
            ...current.snapshot,
            ...(settingsResult.ok ? { settings: settingsResult.data } : {}),
            ...(nativeResult.ok ? { native: nativeResult.data } : {})
          }
        }));
        setSettingsMutation({ pending: false });
        refreshSyncStatus();
        refreshDiagnosticsSummary();
        if (
          request.action === "clearGoogleCache" ||
          request.action === "forceFullResync" ||
          request.action === "resetOnboarding"
        ) {
          load();
        }
        return result.data;
      }

      setSettingsMutation({
        pending: false,
        error: result.error.message
      });
      return null;
    },
    [load, refreshDiagnosticsSummary, refreshSyncStatus, setLoadState]
  );

  return {
    settingsMutation,
    updateSettings,
    runRecoveryAction
  };
}
