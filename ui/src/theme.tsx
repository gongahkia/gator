import React, { createContext, useContext } from "react";

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

export const GatorPaletteContext = createContext<GatorPalette>(palettes.gator);
export function useGatorPalette(): GatorPalette { return useContext(GatorPaletteContext); }

export function GatorPaletteProvider({ theme, children }: { theme?: string; children: React.ReactNode }): React.ReactNode {
  return <GatorPaletteContext.Provider value={paletteForTheme(theme)}>{children}</GatorPaletteContext.Provider>;
}
