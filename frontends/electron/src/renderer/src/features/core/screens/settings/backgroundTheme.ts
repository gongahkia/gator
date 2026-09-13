import type { SettingsSnapshot } from "@shared/ipc/contracts";
import { inferColorThemePaletteFromSamples } from "@shared/ipc/themeCatalog";

const maxCustomBackgroundBytes = 8_000_000;
const sampleSize = 64;

type CustomBackground = NonNullable<SettingsSnapshot["customBackground"]>;

export async function customBackgroundFromFile(file: File): Promise<CustomBackground> {
  const mimeType = imageMimeType(file);

  if (!mimeType) {
    throw new Error("Choose a PNG, JPEG, or WebP image.");
  }

  if (file.size > maxCustomBackgroundBytes) {
    throw new Error("Choose an image under 8 MB.");
  }

  const [dataBase64, samples] = await Promise.all([
    fileToBase64(file),
    sampleImage(file)
  ]);

  return {
    fileName: file.name,
    mimeType,
    dataBase64,
    palette: inferColorThemePaletteFromSamples(samples),
    updatedAt: new Date().toISOString()
  };
}

function imageMimeType(file: File): string | null {
  if (["image/png", "image/jpeg", "image/webp"].includes(file.type)) {
    return file.type;
  }

  const name = file.name.toLowerCase();

  if (name.endsWith(".png")) {
    return "image/png";
  }

  if (name.endsWith(".jpg") || name.endsWith(".jpeg")) {
    return "image/jpeg";
  }

  if (name.endsWith(".webp")) {
    return "image/webp";
  }

  return null;
}

async function fileToBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = "";

  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }

  return btoa(binary);
}

async function sampleImage(file: File): Promise<Array<{ red: number; green: number; blue: number; alpha: number }>> {
  if (typeof createImageBitmap === "function") {
    const bitmap = await createImageBitmap(file);

    try {
      return samplesFromDrawable(bitmap, bitmap.width, bitmap.height);
    } finally {
      bitmap.close();
    }
  }

  const image = await loadImage(file);
  return samplesFromDrawable(image, image.naturalWidth, image.naturalHeight);
}

function samplesFromDrawable(
  image: CanvasImageSource,
  imageWidth: number,
  imageHeight: number
): Array<{ red: number; green: number; blue: number; alpha: number }> {
  const scale = Math.min(sampleSize / imageWidth, sampleSize / imageHeight, 1);
  const width = Math.max(1, Math.round(imageWidth * scale));
  const height = Math.max(1, Math.round(imageHeight * scale));
  const canvas = document.createElement("canvas");
  const context = canvas.getContext("2d", { willReadFrequently: true });

  if (!context) {
    throw new Error("Could not read image pixels.");
  }

  canvas.width = width;
  canvas.height = height;
  context.drawImage(image, 0, 0, width, height);
  const data = context.getImageData(0, 0, width, height).data;
  const samples: Array<{ red: number; green: number; blue: number; alpha: number }> = [];

  for (let index = 0; index < data.length; index += 4) {
    samples.push({
      red: data[index] ?? 0,
      green: data[index + 1] ?? 0,
      blue: data[index + 2] ?? 0,
      alpha: data[index + 3] ?? 255
    });
  }

  return samples;
}

function loadImage(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const image = new Image();

    image.onload = () => {
      URL.revokeObjectURL(url);
      resolve(image);
    };
    image.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("Could not load image."));
    };
    image.src = url;
  });
}
