import { useEffect, useState } from "react";
import type { SettingsSnapshot } from "@shared/ipc/contracts";
import {
  resolveEffectiveColorTheme,
  resolveEffectiveThemeMode,
  semanticThemeVariables
} from "@shared/ipc/themeCatalog";
import { resolveAppLanguage } from "../../i18n";

const systemFontStack = "-apple-system, BlinkMacSystemFont, \"SF Pro Text\", \"Segoe UI\", system-ui, Roboto, \"Helvetica Neue\", Arial, sans-serif";
const monoFontStack = "\"SF Mono\", \"Cascadia Code\", \"Fira Code\", \"JetBrains Mono\", ui-monospace, monospace";

function systemPrefersDark(): boolean {
  return typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function cssFontFamily(fontName: string | null): string {
  const trimmed = fontName?.trim();

  if (!trimmed) {
    return systemFontStack;
  }

  return `"${trimmed.replace(/[\\"]/g, "\\$&")}", ${systemFontStack}`;
}

function cssMonoFontFamily(fontName: string | null): string {
  const trimmed = fontName?.trim();

  if (!trimmed) {
    return monoFontStack;
  }

  return cssFontFamily(trimmed);
}

function textSizeVariables(baseSize: number): Record<string, string> {
  const clamped = Math.min(24, Math.max(9, baseSize));
  const scale = clamped / 13;
  const px = (value: number): string => `${Math.round(value * scale * 100) / 100}px`;

  return {
    "--text-xs": px(11),
    "--text-sm": px(12),
    "--text-base": px(13),
    "--text-md": px(14),
    "--text-lg": px(16),
    "--text-xl": px(20),
    "--text-2xl": px(24)
  };
}

function surfaceTextSize(settings: SettingsSnapshot, surface: keyof SettingsSnapshot["perSurfaceFontOverrides"]): string {
  return `${settings.perSurfaceFontOverrides[surface]?.uiTextSizePoints ?? settings.uiTextSizePoints}px`;
}

function surfaceFontFamily(settings: SettingsSnapshot, surface: keyof SettingsSnapshot["perSurfaceFontOverrides"]): string {
  return cssFontFamily(settings.perSurfaceFontOverrides[surface]?.uiFontName ?? settings.uiFontName);
}

export function useAppliedTheme(settings: SettingsSnapshot): void {
  const [prefersDark, setPrefersDark] = useState(systemPrefersDark);

  useEffect(() => {
    if (typeof window.matchMedia !== "function") {
      return;
    }

    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = (event: MediaQueryListEvent): void => setPrefersDark(event.matches);
    setPrefersDark(media.matches);

    if (typeof media.addEventListener === "function") {
      media.addEventListener("change", listener);
      return () => media.removeEventListener("change", listener);
    }

    media.addListener(listener);
    return () => media.removeListener(listener);
  }, []);

  useEffect(() => {
    const mode = resolveEffectiveThemeMode(settings, prefersDark);
    const colorTheme = resolveEffectiveColorTheme(settings, mode);
    const root = document.documentElement;

    root.dataset.theme = mode;
    root.dataset.colorTheme = colorTheme.id;
    delete root.dataset.performanceMode;
    root.dataset.animations = settings.disableAnimations ? "disabled" : "enabled";
    root.lang = resolveAppLanguage(settings.appLanguage);
    root.style.setProperty("--font-family", cssFontFamily(settings.uiFontName));
    root.style.setProperty("--font-family-mono", cssMonoFontFamily(settings.uiFontName));
    root.style.setProperty("--font-family-sidebar", surfaceFontFamily(settings, "sidebar"));
    root.style.setProperty("--text-sidebar", surfaceTextSize(settings, "sidebar"));
    root.style.setProperty("--font-family-menu-bar", surfaceFontFamily(settings, "menuBar"));
    root.style.setProperty("--text-menu-bar", surfaceTextSize(settings, "menuBar"));
    root.style.setProperty("--app-shell-background", shellBackground(settings));
    root.style.fontSize = `${Math.round(Math.min(1.5, Math.max(0.8, settings.uiLayoutScale)) * 16 * 100) / 100}px`;

    for (const [name, value] of Object.entries(semanticThemeVariables(colorTheme))) {
      root.style.setProperty(name, value);
    }

    for (const [name, value] of Object.entries(textSizeVariables(settings.uiTextSizePoints))) {
      root.style.setProperty(name, value);
    }
  }, [
    prefersDark,
    settings.appLanguage,
    settings.colorTheme,
    settings.customBackground,
    settings.disableAnimations,
    settings.theme,
    settings.useInferredBackgroundTheme,
    settings.uiFontName,
    settings.uiLayoutScale,
    settings.perSurfaceFontOverrides,
    settings.uiTextSizePoints
  ]);
}

function shellBackground(settings: SettingsSnapshot): string {
  if (!settings.customBackground) {
    return "var(--color-bg-primary)";
  }

  const mimeType = settings.customBackground.mimeType.replace(/[")\\]/g, "");
  const image = `url("data:${mimeType};base64,${settings.customBackground.dataBase64}")`;
  const overlay = settings.useInferredBackgroundTheme
    ? "linear-gradient(rgba(0,0,0,0.08),rgba(0,0,0,0.08))"
    : "linear-gradient(rgba(0,0,0,0.18),rgba(0,0,0,0.18))";

  return `${overlay}, ${image} center / cover fixed, var(--color-bg-primary)`;
}
