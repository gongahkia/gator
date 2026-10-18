import React, { createContext, useContext } from "react";
import { getTheme } from "@claude-code-kit/ui";

export type GatorPalette = {
  accent: string;
  secondary: string;
  warning: string;
  error: string;
};

const palettes: Record<string, GatorPalette> = {
  gator: { accent: "#4BAA72", secondary: "#86BE9A", warning: "#D6AD69", error: "#DF7777" },
  contrast: { accent: "#78E6A3", secondary: "#C4F1D3", warning: "#FFD879", error: "#FF9292" },
  mono: { accent: "#E6E6E6", secondary: "#B8B8B8", warning: "#E6E6E6", error: "#FFFFFF" },
};

export function paletteForTheme(name?: string): GatorPalette {
  return palettes[name ?? "gator"] ?? palettes.gator;
}

// The kit's Markdown renderer reads getTheme("dark") directly for inline code
// and links. It does not accept a custom palette, so keep its public theme
// object aligned with Gator before rendering kit components.
export function synchronizeKitTheme(palette: GatorPalette): void {
  const kit = getTheme("dark");
  kit.accent = palette.accent;
  kit.assistant = palette.accent;
  kit.permission = palette.accent;
  kit.focus = palette.accent;
  kit.link = palette.accent;
  kit.code = palette.accent;
  kit.info = palette.secondary;
  kit.spinnerColor = palette.accent;
  kit.success = palette.accent;
  kit.warning = palette.warning;
  kit.error = palette.error;
}

export const GatorPaletteContext = createContext<GatorPalette>(palettes.gator);
export function useGatorPalette(): GatorPalette { return useContext(GatorPaletteContext); }

export function GatorPaletteProvider({ theme, children }: { theme?: string; children: React.ReactNode }): React.ReactNode {
  const palette = paletteForTheme(theme);
  synchronizeKitTheme(palette);
  return <GatorPaletteContext.Provider value={palette}>{children}</GatorPaletteContext.Provider>;
}
