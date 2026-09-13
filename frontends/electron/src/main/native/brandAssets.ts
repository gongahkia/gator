import { app, nativeImage, type NativeImage } from "electron";
import { existsSync } from "node:fs";
import { join } from "node:path";

export function brandAssetPath(filename: string): string {
  const relativePath = join("assets", "brand", filename);

  return app.isPackaged
    ? join(process.resourcesPath, relativePath)
    : join(app.getAppPath(), relativePath);
}

export function brandImage(filename: string): NativeImage {
  const imagePath = brandAssetPath(filename);

  return existsSync(imagePath) ? nativeImage.createFromPath(imagePath) : nativeImage.createEmpty();
}
