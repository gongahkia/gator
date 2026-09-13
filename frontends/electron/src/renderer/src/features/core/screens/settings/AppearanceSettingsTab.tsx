import { useEffect, useMemo, useState } from "react";
import {
  calendarEventColorForTheme,
  googleCalendarEventColors,
  type GoogleCalendarEventColorId,
  type SettingsSnapshot,
  type SettingsUpdateRequest
} from "@shared/ipc/contracts";
import type {
  AppColorThemeDefinition,
  AppColorThemeId,
  ColorThemeDefinition
} from "@shared/ipc/themeCatalog";
import { customBackgroundThemeId } from "@shared/ipc/themeCatalog";
import { ArrowDown, ArrowUp, PanelLeft, PanelRight, RotateCcw } from "lucide-react";
import { Button, Input, cx } from "../../../../components/primitives";
import { useI18n } from "../../../../i18n";
import { customBackgroundFromFile } from "./backgroundTheme";
import {
  SegmentedControl,
  SettingsControlRow,
  SettingsGroup,
  SettingsSwitch,
  settingsSelectClass
} from "./SettingsPrimitives";
import {
  calendarViewModes,
  fontSurfaceOptions,
  navigationTabs,
  toolbarActions,
  type CalendarViewModeId,
  type FontSurfaceId,
  type NavigationTabId,
  type ToolbarActionId
} from "./settingsUtils";

type PerSurfaceFontOverride = NonNullable<
  SettingsSnapshot["perSurfaceFontOverrides"][FontSurfaceId]
>;
type CalendarEventColorOverride = NonNullable<
  SettingsSnapshot["calendarEventColorOverrides"][GoogleCalendarEventColorId]
>;
type CropAspect = "16:10" | "16:9" | "4:3";
type PendingBackground = {
  file: File;
  url: string;
};

const cropAspects: Array<{ label: string; value: CropAspect }> = [
  { label: "16:10", value: "16:10" },
  { label: "16:9", value: "16:9" },
  { label: "4:3", value: "4:3" }
];

interface AppearanceSettingsTabProps {
  activeColorTheme: ColorThemeDefinition;
  availableFontFamilies: string[];
  matchingColorThemes: readonly AppColorThemeDefinition[];
  settings: SettingsSnapshot;
  updateBaseTheme: (theme: SettingsSnapshot["theme"]) => void;
  updateSettings: (request: SettingsUpdateRequest) => void;
}

export function AppearanceSettingsTab({
  activeColorTheme,
  availableFontFamilies,
  matchingColorThemes,
  settings,
  updateBaseTheme,
  updateSettings
}: AppearanceSettingsTabProps): JSX.Element {
  const { t } = useI18n();
  const [customBackgroundMessage, setCustomBackgroundMessage] = useState<string | null>(null);
  const [pendingBackground, setPendingBackground] = useState<PendingBackground | null>(null);
  const [cropAspect, setCropAspect] = useState<CropAspect>("16:10");
  const [cropZoom, setCropZoom] = useState(1);
  const [cropX, setCropX] = useState(50);
  const [cropY, setCropY] = useState(50);
  const inferredThemeActive = activeColorTheme.id === customBackgroundThemeId;
  const customBackgroundPreviewUrl = customBackgroundPreview(settings);
  const cropAspectValue = useMemo(() => cropAspectNumber(cropAspect), [cropAspect]);

  useEffect(() => {
    return () => {
      if (pendingBackground) {
        URL.revokeObjectURL(pendingBackground.url);
      }
    };
  }, [pendingBackground]);

  function updateNavigationTab(tabId: NavigationTabId, visible: boolean): void {
    const hidden = new Set(settings.hiddenNavigationTabs);

    if (visible) {
      hidden.delete(tabId);
    } else {
      hidden.add(tabId);
    }

    if (navigationTabs.every((tab) => hidden.has(tab.id))) {
      return;
    }

    updateSettings({ hiddenNavigationTabs: [...hidden] });
  }

  function updateCalendarViewMode(viewId: CalendarViewModeId, visible: boolean): void {
    const hidden = new Set(settings.hiddenCalendarViewModes);

    if (visible) {
      hidden.delete(viewId);
    } else {
      hidden.add(viewId);
    }

    if (calendarViewModes.every((mode) => hidden.has(mode.id))) {
      return;
    }

    updateSettings({ hiddenCalendarViewModes: [...hidden] });
  }

  function updatePerSurfaceFont(
    surface: FontSurfaceId,
    patch: Partial<PerSurfaceFontOverride>
  ): void {
    const current = settings.perSurfaceFontOverrides[surface] ?? {
      uiFontName: null,
      uiTextSizePoints: null
    };

    updateSettings({
      perSurfaceFontOverrides: {
        ...settings.perSurfaceFontOverrides,
        [surface]: {
          ...current,
          ...patch
        }
      }
    });
  }

  function resetPerSurfaceFont(surface: FontSurfaceId): void {
    const next = { ...settings.perSurfaceFontOverrides };
    delete next[surface];
    updateSettings({ perSurfaceFontOverrides: next });
  }

  function moveNavigationTab(tabId: NavigationTabId, direction: -1 | 1): void {
    updateSettings({
      navigationTabOrder: moveItem(settings.navigationTabOrder, tabId, direction)
    });
  }

  function moveToolbarAction(actionId: ToolbarActionId, direction: -1 | 1): void {
    updateSettings({
      toolbarActionOrder: moveItem(settings.toolbarActionOrder, actionId, direction)
    });
  }

  function updateCalendarEventColorOverride(
    colorId: GoogleCalendarEventColorId,
    patch: Partial<CalendarEventColorOverride>
  ): void {
    const current = settings.calendarEventColorOverrides[colorId] ?? calendarEventColorDefault(colorId);

    updateSettings({
      calendarEventColorOverrides: {
        ...settings.calendarEventColorOverrides,
        [colorId]: {
          ...current,
          ...patch
        }
      }
    });
  }

  function resetCalendarEventColorOverride(colorId: GoogleCalendarEventColorId): void {
    const next = { ...settings.calendarEventColorOverrides };
    delete next[colorId];
    updateSettings({ calendarEventColorOverrides: next });
  }

  function calendarEventColorDefault(colorId: GoogleCalendarEventColorId): CalendarEventColorOverride {
    const themeColor = calendarEventColorForTheme(activeColorTheme, colorId);
    const googleColor = googleCalendarEventColors.find((color) => color.id === colorId);

    return {
      background: themeColor?.background ?? googleColor?.background ?? "#5484ed",
      foreground: themeColor?.foreground ?? googleColor?.foreground ?? "#ffffff"
    };
  }

  async function selectCustomBackground(file: File | null): Promise<void> {
    if (!file) {
      return;
    }

    setCustomBackgroundMessage("Reading background.");

    try {
      const customBackground = await customBackgroundFromFile(file);

      updateSettings({
        customBackground,
        useInferredBackgroundTheme: true
      });
      setPendingBackground(null);
      setCustomBackgroundMessage("Background applied.");
    } catch (error) {
      setCustomBackgroundMessage(error instanceof Error ? error.message : "Background import failed.");
    }
  }

  function previewCustomBackground(file: File | null): void {
    if (!file) {
      return;
    }

    setPendingBackground({ file, url: URL.createObjectURL(file) });
    setCropAspect("16:10");
    setCropZoom(1);
    setCropX(50);
    setCropY(50);
    setCustomBackgroundMessage("Preview ready.");
  }

  async function applyCroppedCustomBackground(): Promise<void> {
    if (!pendingBackground) {
      return;
    }

    setCustomBackgroundMessage("Cropping background.");

    try {
      const blob = await cropImageToBlob({
        aspectRatio: cropAspectValue,
        mimeType: pendingBackground.file.type || "image/png",
        positionX: cropX,
        positionY: cropY,
        url: pendingBackground.url,
        zoom: cropZoom
      });
      const file = new File([blob], croppedBackgroundFileName(pendingBackground.file.name, blob.type), {
        type: blob.type
      });

      await selectCustomBackground(file);
    } catch (error) {
      setCustomBackgroundMessage(error instanceof Error ? error.message : "Background crop failed.");
    }
  }

  function updateColorTheme(value: string): void {
    if (value === customBackgroundThemeId) {
      updateSettings({ useInferredBackgroundTheme: true });
      return;
    }

    updateSettings({
      colorTheme: value as AppColorThemeId,
      ...(settings.customBackground ? { useInferredBackgroundTheme: false } : {})
    });
  }

  return (
    <div className="grid gap-5">
      <SettingsGroup title="Appearance">
        <SettingsControlRow
          description="Choose whether app chrome resolves as dark, light, or follows macOS."
          label="Base colour scheme"
        >
          <select
            aria-label="Theme"
            className={settingsSelectClass}
            onChange={(event) => updateBaseTheme(event.target.value as SettingsSnapshot["theme"])}
            value={settings.theme}
          >
            <option value="system">System</option>
            <option value="light">Light</option>
            <option value="dark">Dark</option>
          </select>
        </SettingsControlRow>
        <SettingsControlRow
          description="Palette used by cards, text, and app surfaces."
          label="Theme"
        >
          <select
            aria-label="Color theme"
            className={settingsSelectClass}
            onChange={(event) => updateColorTheme(event.target.value)}
            value={activeColorTheme.id}
          >
            {settings.customBackground ? (
              <option value={customBackgroundThemeId}>Inferred from background</option>
            ) : null}
            {matchingColorThemes.map((theme) => (
              <option key={theme.id} value={theme.id}>
                {theme.title}
              </option>
            ))}
          </select>
        </SettingsControlRow>
        <SettingsControlRow
          description="Use a local image as the app backdrop and infer the palette from its pixels."
          label="Custom background"
        >
          <div className="grid min-w-0 gap-3 sm:min-w-[28rem]">
            <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
              <label
                className={cx(
                  "inline-flex h-8 shrink-0 cursor-pointer items-center justify-center rounded-hcbMd border border-border bg-surface-0 px-3 text-[var(--text-base)] font-medium leading-none text-text-primary transition-colors duration-fast ease-hcb hover:bg-surface-1",
                  "focus-within:outline focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-accent"
                )}
              >
                Choose Image...
                <input
                  accept="image/png,image/jpeg,image/webp"
                  aria-label="Custom background image"
                  className="sr-only"
                  onChange={(event) => previewCustomBackground(event.currentTarget.files?.[0] ?? null)}
                  type="file"
                />
              </label>
              <span className="min-w-0 max-w-48 truncate text-[var(--text-sm)] text-text-muted">
                {pendingBackground?.file.name ?? settings.customBackground?.fileName ?? "No image selected"}
              </span>
              <Button
                disabled={!settings.customBackground}
                onClick={() => {
                  updateSettings({ customBackground: null, useInferredBackgroundTheme: true });
                  setCustomBackgroundMessage("Background cleared.");
                }}
                variant="secondary"
              >
                Clear
              </Button>
            </div>
            {pendingBackground ? (
              <div className="grid gap-3 rounded-hcbMd border border-border bg-surface-0 p-2">
                <div
                  className="relative w-full overflow-hidden rounded-hcbSm border border-border bg-bg-secondary"
                  style={{ aspectRatio: cropAspectValue }}
                >
                  <img
                    alt="Selected background crop preview"
                    className="h-full w-full object-cover"
                    src={pendingBackground.url}
                    style={{
                      transform: `scale(${cropZoom})`,
                      transformOrigin: `${cropX}% ${cropY}%`
                    }}
                  />
                </div>
                <div className="grid gap-2 text-[var(--text-xs)] text-text-muted sm:grid-cols-3">
                  <label className="grid gap-1">
                    <span>Zoom</span>
                    <input
                      aria-label="Crop zoom"
                      className="accent-[var(--color-accent)]"
                      max={3}
                      min={1}
                      onChange={(event) => setCropZoom(Number(event.currentTarget.value))}
                      step={0.05}
                      type="range"
                      value={cropZoom}
                    />
                  </label>
                  <label className="grid gap-1">
                    <span>Horizontal</span>
                    <input
                      aria-label="Crop horizontal position"
                      className="accent-[var(--color-accent)]"
                      max={100}
                      min={0}
                      onChange={(event) => setCropX(Number(event.currentTarget.value))}
                      step={1}
                      type="range"
                      value={cropX}
                    />
                  </label>
                  <label className="grid gap-1">
                    <span>Vertical</span>
                    <input
                      aria-label="Crop vertical position"
                      className="accent-[var(--color-accent)]"
                      max={100}
                      min={0}
                      onChange={(event) => setCropY(Number(event.currentTarget.value))}
                      step={1}
                      type="range"
                      value={cropY}
                    />
                  </label>
                </div>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <select
                    aria-label="Crop rectangle"
                    className={settingsSelectClass}
                    onChange={(event) => setCropAspect(event.currentTarget.value as CropAspect)}
                    value={cropAspect}
                  >
                    {cropAspects.map((aspect) => (
                      <option key={aspect.value} value={aspect.value}>{aspect.label}</option>
                    ))}
                  </select>
                  <div className="flex items-center gap-2">
                    <Button onClick={() => void selectCustomBackground(pendingBackground.file)} variant="ghost">
                      Use full image
                    </Button>
                    <Button onClick={() => void applyCroppedCustomBackground()} variant="primary">
                      Apply crop
                    </Button>
                  </div>
                </div>
              </div>
            ) : customBackgroundPreviewUrl ? (
              <img
                alt="Custom background preview"
                className="h-24 w-full rounded-hcbSm border border-border object-cover"
                src={customBackgroundPreviewUrl}
              />
            ) : null}
            {settings.customBackground ? (
              <div className="flex flex-wrap justify-end gap-1">
                {[
                  settings.customBackground.palette.ember,
                  settings.customBackground.palette.moss,
                  settings.customBackground.palette.blue,
                  settings.customBackground.palette.cream,
                  settings.customBackground.palette.ink
                ].map((color) => (
                  <span
                    aria-hidden="true"
                    className="h-5 w-5 rounded-hcbSm border border-border"
                    key={color}
                    style={{ backgroundColor: color }}
                  />
                ))}
              </div>
            ) : null}
            {customBackgroundMessage ? (
              <div className="text-right text-[var(--text-xs)] text-text-muted">{customBackgroundMessage}</div>
            ) : null}
          </div>
        </SettingsControlRow>
        <SettingsControlRow
          description="Use generated colors for surfaces and default event/tag mappings."
          label="Infer theme from background"
        >
          <input
            aria-label="Infer theme from background"
            checked={settings.useInferredBackgroundTheme && Boolean(settings.customBackground)}
            className="h-5 w-9 accent-[var(--color-accent)] disabled:opacity-50"
            disabled={!settings.customBackground}
            onChange={(event) => updateSettings({ useInferredBackgroundTheme: event.currentTarget.checked })}
            type="checkbox"
          />
        </SettingsControlRow>
        {inferredThemeActive && settings.customBackground ? (
          <SettingsControlRow label="Inferred palette">
            <div className="flex flex-wrap items-center justify-end gap-2 text-[var(--text-sm)] text-text-muted">
              <span>{settings.customBackground.palette.isDark ? "Dark" : "Light"}</span>
              <span className="font-mono">{settings.customBackground.palette.dominant}</span>
            </div>
          </SettingsControlRow>
        ) : null}
        <SettingsSwitch
          checked={settings.disableAnimations}
          description="Turns off app-driven transitions, panel motion, and animated state changes."
          label="Disable animations"
          onChange={(checked) => updateSettings({ disableAnimations: checked })}
        />
        <SettingsControlRow label="Layout scale">
          <div className="flex min-w-0 items-center gap-3">
            <input
              aria-label="Layout scale"
              className="w-56 accent-[var(--color-accent)]"
              max={1.5}
              min={0.8}
              onChange={(event) => updateSettings({ uiLayoutScale: Number(event.target.value) })}
              step={0.05}
              type="range"
              value={settings.uiLayoutScale}
            />
            <span className="w-12 text-right font-mono text-[var(--text-xs)] text-text-muted">
              {Math.round(settings.uiLayoutScale * 100)}%
            </span>
          </div>
        </SettingsControlRow>
        <SettingsControlRow label="Text size">
          <div className="flex min-w-0 items-center gap-3">
            <Input
              aria-label="Text size points"
              className="w-24"
              max={24}
              min={9}
              onChange={(event) =>
                updateSettings({
                  uiTextSizePoints: Math.min(24, Math.max(9, Number(event.target.value) || 13))
                })
              }
              type="number"
              value={settings.uiTextSizePoints}
            />
            <Button onClick={() => updateSettings({ uiTextSizePoints: 13 })} variant="ghost">
              Reset
            </Button>
          </div>
        </SettingsControlRow>
        <SettingsControlRow label="UI font">
          <select
            aria-label="Font family"
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({
                uiFontName: event.target.value.trim() ? event.target.value : null
              })
            }
            value={settings.uiFontName ?? ""}
          >
            <option value="">System</option>
            {availableFontFamilies.map((fontName) => (
              <option key={fontName} value={fontName}>
                {fontName}
              </option>
            ))}
          </select>
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title="Calendar event colors">
        {googleCalendarEventColors.map((color) => {
          const override = settings.calendarEventColorOverrides[color.id];
          const current = override ?? calendarEventColorDefault(color.id);

          return (
            <SettingsControlRow key={color.id} label={color.label}>
              <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
                <span
                  aria-hidden="true"
                  className="h-7 w-7 shrink-0 rounded-hcbSm border border-border"
                  style={{ backgroundColor: current.background }}
                />
                <input
                  aria-label={`${color.label} background`}
                  className="h-8 w-10 rounded-hcbSm border border-border bg-surface-0 p-0.5"
                  onChange={(event) =>
                    updateCalendarEventColorOverride(color.id, { background: event.target.value })
                  }
                  type="color"
                  value={current.background}
                />
                <input
                  aria-label={`${color.label} text`}
                  className="h-8 w-10 rounded-hcbSm border border-border bg-surface-0 p-0.5"
                  onChange={(event) =>
                    updateCalendarEventColorOverride(color.id, { foreground: event.target.value })
                  }
                  type="color"
                  value={current.foreground}
                />
                <Button
                  disabled={!override}
                  onClick={() => resetCalendarEventColorOverride(color.id)}
                  variant="ghost"
                >
                  Reset
                </Button>
              </div>
            </SettingsControlRow>
          );
        })}
      </SettingsGroup>

      <SettingsGroup title="Per-surface fonts">
        {fontSurfaceOptions.map((surface) => {
          const override = settings.perSurfaceFontOverrides[surface.id];

          return (
            <SettingsControlRow key={surface.id} label={surface.label}>
              <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
                <select
                  aria-label={`${surface.label} font`}
                  className={settingsSelectClass}
                  onChange={(event) =>
                    updatePerSurfaceFont(surface.id, {
                      uiFontName: event.target.value.trim() ? event.target.value : null
                    })
                  }
                  value={override?.uiFontName ?? ""}
                >
                  <option value="">(inherit)</option>
                  {availableFontFamilies.map((fontName) => (
                    <option key={fontName} value={fontName}>
                      {fontName}
                    </option>
                  ))}
                </select>
                <Input
                  aria-label={`${surface.label} size`}
                  className="w-20"
                  max={24}
                  min={9}
                  onChange={(event) =>
                    updatePerSurfaceFont(surface.id, {
                      uiTextSizePoints: event.target.value ? Number(event.target.value) : null
                    })
                  }
                  placeholder="Size"
                  type="number"
                  value={override?.uiTextSizePoints ?? ""}
                />
                <Button onClick={() => resetPerSurfaceFont(surface.id)} variant="ghost">
                  Reset
                </Button>
              </div>
            </SettingsControlRow>
          );
        })}
      </SettingsGroup>

      <SettingsGroup title={t("settings.layout")}>
        <SettingsControlRow
          description={t("layout.navigationPlacement.description")}
          label={t("layout.navigationPlacement")}
        >
          <SegmentedControl
            options={[
              { label: "Left", value: "left", icon: PanelLeft },
              { label: "Right", value: "right", icon: PanelRight }
            ]}
            onChange={(value) =>
              updateSettings({ navigationPlacement: value as SettingsSnapshot["navigationPlacement"] })
            }
            value={settings.navigationPlacement}
          />
        </SettingsControlRow>
        <div className="grid gap-1 border-b border-border px-3 py-3 last:border-b-0">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-[var(--text-md)] font-semibold text-text-primary">{t("layout.navigationTabs")}</h3>
            <Button
              onClick={() => updateSettings({ navigationTabOrder: navigationTabs.map((tab) => tab.id) })}
              size="sm"
              variant="ghost"
            >
              <RotateCcw aria-hidden="true" size={13} />
              {t("action.reset")}
            </Button>
          </div>
          {settings.navigationTabOrder.map((tabId, index) => {
            const tab = navigationTabs.find((item) => item.id === tabId);
            if (!tab) {
              return null;
            }

            return (
              <SettingsSwitch
                checked={!settings.hiddenNavigationTabs.includes(tab.id)}
                key={tab.id}
                label={navigationLabel(tab.id, t)}
                onChange={(checked) => updateNavigationTab(tab.id, checked)}
                trailing={
                  <div className="flex items-center gap-1">
                    <Button
                      aria-label={`${t("action.moveUp")} ${navigationLabel(tab.id, t)}`}
                      disabled={index === 0}
                      onClick={() => moveNavigationTab(tab.id, -1)}
                      size="sm"
                      variant="ghost"
                    >
                      <ArrowUp aria-hidden="true" size={13} />
                    </Button>
                    <Button
                      aria-label={`${t("action.moveDown")} ${navigationLabel(tab.id, t)}`}
                      disabled={index === settings.navigationTabOrder.length - 1}
                      onClick={() => moveNavigationTab(tab.id, 1)}
                      size="sm"
                      variant="ghost"
                    >
                      <ArrowDown aria-hidden="true" size={13} />
                    </Button>
                  </div>
                }
              />
            );
          })}
        </div>
        <div className="grid gap-1 border-b border-border px-3 py-3 last:border-b-0">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-[var(--text-md)] font-semibold text-text-primary">{t("layout.toolbarActions")}</h3>
            <Button
              onClick={() => updateSettings({ toolbarActionOrder: toolbarActions.map((action) => action.id) })}
              size="sm"
              variant="ghost"
            >
              <RotateCcw aria-hidden="true" size={13} />
              {t("action.reset")}
            </Button>
          </div>
          {settings.toolbarActionOrder.map((actionId, index) => {
            const action = toolbarActions.find((item) => item.id === actionId);
            if (!action) {
              return null;
            }

            return (
              <SettingsControlRow key={action.id} label={toolbarLabel(action.id, t)}>
                <div className="flex items-center justify-end gap-1">
                  <Button
                    aria-label={`${t("action.moveUp")} ${toolbarLabel(action.id, t)}`}
                    disabled={index === 0}
                    onClick={() => moveToolbarAction(action.id, -1)}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowUp aria-hidden="true" size={13} />
                  </Button>
                  <Button
                    aria-label={`${t("action.moveDown")} ${toolbarLabel(action.id, t)}`}
                    disabled={index === settings.toolbarActionOrder.length - 1}
                    onClick={() => moveToolbarAction(action.id, 1)}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowDown aria-hidden="true" size={13} />
                  </Button>
                </div>
              </SettingsControlRow>
            );
          })}
        </div>
        <div className="grid gap-1 border-b border-border px-3 py-3 last:border-b-0">
          <h3 className="text-[var(--text-md)] font-semibold text-text-primary">{t("layout.calendarViewModes")}</h3>
          {calendarViewModes.map((mode) => (
            <SettingsSwitch
              checked={!settings.hiddenCalendarViewModes.includes(mode.id)}
              key={mode.id}
              label={mode.label}
              onChange={(checked) => updateCalendarViewMode(mode.id, checked)}
            />
          ))}
        </div>
        <SettingsControlRow
          description="Month view will not navigate beyond these loaded month bounds."
          label="Month scroll range"
        >
          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
            <span className="text-[var(--text-sm)] text-text-muted">Past</span>
            <Input
              aria-label="Past months"
              className="w-20"
              max={24}
              min={0}
              onChange={(event) => updateSettings({ monthScrollPastMonths: Number(event.target.value) || 0 })}
              type="number"
              value={settings.monthScrollPastMonths}
            />
            <span className="text-[var(--text-sm)] text-text-muted">Future</span>
            <Input
              aria-label="Future months"
              className="w-20"
              max={24}
              min={0}
              onChange={(event) => updateSettings({ monthScrollFutureMonths: Number(event.target.value) || 0 })}
              type="number"
              value={settings.monthScrollFutureMonths}
            />
          </div>
        </SettingsControlRow>
        <SettingsControlRow
          description="Controls hour row height in day, multi-day, and week views."
          label="Calendar timeline density"
        >
          <select
            aria-label="Calendar timeline density"
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({
                calendarTimelineDensity: event.target.value as SettingsSnapshot["calendarTimelineDensity"]
              })
            }
            value={settings.calendarTimelineDensity}
          >
            <option value="compact">Compact</option>
            <option value="comfortable">Comfortable</option>
            <option value="spacious">Spacious</option>
          </select>
        </SettingsControlRow>
        <SettingsSwitch
          checked={settings.quickCreateExpandedByDefault}
          description="Calendar quick creation opens with optional fields visible when supported."
          label="Show all quick-create fields by default"
          onChange={(checked) => updateSettings({ quickCreateExpandedByDefault: checked })}
        />
        <SettingsSwitch
          checked={settings.restoreWindowStateEnabled}
          description="Keeps window restoration enabled across app launches."
          label="Restore previous session"
          onChange={(checked) => updateSettings({ restoreWindowStateEnabled: checked })}
        />
      </SettingsGroup>
    </div>
  );
}

function moveItem<T>(items: readonly T[], item: T, direction: -1 | 1): T[] {
  const next = [...items];
  const index = next.indexOf(item);
  const target = index + direction;

  if (index < 0 || target < 0 || target >= next.length) {
    return next;
  }

  [next[index], next[target]] = [next[target], next[index]];
  return next;
}

function customBackgroundPreview(settings: SettingsSnapshot): string | null {
  if (!settings.customBackground) {
    return null;
  }

  return `data:${settings.customBackground.mimeType};base64,${settings.customBackground.dataBase64}`;
}

function cropAspectNumber(aspect: CropAspect): number {
  if (aspect === "16:9") {
    return 16 / 9;
  }

  if (aspect === "4:3") {
    return 4 / 3;
  }

  return 16 / 10;
}

function croppedBackgroundFileName(fileName: string, mimeType: string): string {
  const baseName = fileName.replace(/\.[^.]+$/, "") || "background";
  const extension = mimeType === "image/jpeg" ? "jpg" : mimeType === "image/webp" ? "webp" : "png";

  return `${baseName}-crop.${extension}`;
}

async function cropImageToBlob({
  aspectRatio,
  mimeType,
  positionX,
  positionY,
  url,
  zoom
}: {
  aspectRatio: number;
  mimeType: string;
  positionX: number;
  positionY: number;
  url: string;
  zoom: number;
}): Promise<Blob> {
  const image = await loadCropImage(url);
  const sourceAspect = image.naturalWidth / image.naturalHeight;
  const baseWidth = sourceAspect > aspectRatio ? image.naturalHeight * aspectRatio : image.naturalWidth;
  const baseHeight = sourceAspect > aspectRatio ? image.naturalHeight : image.naturalWidth / aspectRatio;
  const cropWidth = Math.max(1, baseWidth / zoom);
  const cropHeight = Math.max(1, baseHeight / zoom);
  const sourceX = ((image.naturalWidth - cropWidth) * positionX) / 100;
  const sourceY = ((image.naturalHeight - cropHeight) * positionY) / 100;
  const outputWidth = 1600;
  const outputHeight = Math.round(outputWidth / aspectRatio);
  const canvas = document.createElement("canvas");
  const context = canvas.getContext("2d");

  if (!context) {
    throw new Error("Could not crop image.");
  }

  canvas.width = outputWidth;
  canvas.height = outputHeight;
  context.drawImage(image, sourceX, sourceY, cropWidth, cropHeight, 0, 0, outputWidth, outputHeight);

  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) {
        resolve(blob);
        return;
      }

      reject(new Error("Could not export cropped image."));
    }, ["image/jpeg", "image/png", "image/webp"].includes(mimeType) ? mimeType : "image/png", 0.92);
  });
}

function loadCropImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image();

    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error("Could not load image."));
    image.src = url;
  });
}

function navigationLabel(tabId: NavigationTabId, t: ReturnType<typeof useI18n>["t"]): string {
  if (tabId === "calendar") {
    return t("nav.calendar");
  }

  if (tabId === "tasks") {
    return t("nav.tasks");
  }

  return t("nav.notes");
}

function toolbarLabel(actionId: ToolbarActionId, t: ReturnType<typeof useI18n>["t"]): string {
  if (actionId === "commandPalette") {
    return t("action.commandPalette");
  }

  if (actionId === "notifications") {
    return t("action.notifications");
  }

  if (actionId === "diagnostics") {
    return t("action.diagnostics");
  }

  if (actionId === "splitPane") {
    return t("action.splitView");
  }

  if (actionId === "refresh") {
    return t("action.refresh");
  }

  return t("action.settings");
}
