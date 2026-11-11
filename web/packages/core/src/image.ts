/** Longest side of a stored image, in pixels (server limit). */
export const MAX_IMAGE_SIDE = 2560;
/** Longest side of the small preview stored next to each image. */
export const PREVIEW_SIDE = 640;
/** WebP quality used for photos. */
export const WEBP_QUALITY = 0.8;

/** A photo ready to upload. */
export interface ConvertedImage {
  /** The full image, at most MAX_IMAGE_SIDE on its longest side. */
  full: Blob;
  /** A PREVIEW_SIDE preview. */
  preview: Blob;
  /** Content type of both blobs: "image/webp", or another type if the platform can't make WebP. */
  type: string;
  width: number;
  height: number;
}

/** Turns a photo into WebP with a preview. Each platform supplies its own. */
export interface ImageConverter {
  toWebP(input: Blob): Promise<ConvertedImage>;
}

/** The size that fits inside max x max while keeping the aspect ratio; never scales up. */
export function fitWithin(width: number, height: number, max: number): { width: number; height: number } {
  const side = Math.max(width, height);
  if (side <= max || side === 0) return { width, height };
  const k = max / side;
  return { width: Math.max(1, Math.round(width * k)), height: Math.max(1, Math.round(height * k)) };
}

/** Swaps a file name's extension, e.g. "IMG_1.HEIC" -> "IMG_1.webp". */
export function withExtension(name: string, ext: string): string {
  const base = name.replace(/\.[^./]+$/, "") || "photo";
  return `${base}.${ext}`;
}

interface Drawable {
  width: number;
  height: number;
  close?: () => void;
}

/** What the browser converter needs; injectable for tests. */
export interface BrowserImageDeps {
  decode(input: Blob): Promise<Drawable>;
  encode(src: Drawable, width: number, height: number, type: string, quality: number): Promise<Blob>;
}

const browserDeps: BrowserImageDeps = {
  decode: (input) => createImageBitmap(input, { imageOrientation: "from-image" }),
  encode: (src, width, height, type, quality) =>
    new Promise<Blob>((resolve, reject) => {
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      const ctx = canvas.getContext("2d");
      if (!ctx) return reject(new Error("This browser can't draw images."));
      ctx.drawImage(src as unknown as CanvasImageSource, 0, 0, width, height);
      canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("Could not convert the photo."))), type, quality);
    }),
};

/**
 * Browser converter: decodes with createImageBitmap and encodes with
 * canvas.toBlob("image/webp", 0.8). Some browsers (older Safari) can't encode
 * WebP and return PNG instead; then `type` says so and the caller decides.
 */
export function browserImageConverter(deps: BrowserImageDeps = browserDeps): ImageConverter {
  return {
    async toWebP(input) {
      const src = await deps.decode(input);
      try {
        const big = fitWithin(src.width, src.height, MAX_IMAGE_SIDE);
        const small = fitWithin(src.width, src.height, PREVIEW_SIDE);
        const full = await deps.encode(src, big.width, big.height, "image/webp", WEBP_QUALITY);
        const preview = await deps.encode(src, small.width, small.height, "image/webp", WEBP_QUALITY);
        return { full, preview, type: full.type || "image/webp", width: big.width, height: big.height };
      } finally {
        src.close?.();
      }
    },
  };
}
