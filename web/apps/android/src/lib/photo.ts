import { MAX_IMAGE_SIDE, PREVIEW_SIDE, WEBP_QUALITY, fitWithin, withExtension, type UploadItem } from "@cadenza/core";

/** A picked photo: a local file URI and its size. */
export interface PickedPhoto {
  uri: string;
  width: number;
  height: number;
  fileName?: string | null;
}

/** A converted image file on the device. */
export interface SavedImage {
  uri: string;
  width: number;
  height: number;
}

/** Platform pieces, injected so the logic can be tested without a phone. */
export interface PhotoDeps {
  /** Resizes to exactly width x height and saves as WebP at the given quality (0-1). */
  toWebP(uri: string, width: number, height: number, quality: number): Promise<SavedImage>;
  /** Reads a local file as a Blob. */
  readBlob(uri: string): Promise<Blob>;
}

/** The full and preview sizes for a photo. */
export function photoSizes(width: number, height: number) {
  return { full: fitWithin(width, height, MAX_IMAGE_SIDE), preview: fitWithin(width, height, PREVIEW_SIDE) };
}

/** A file name for the converted photo. */
export function photoName(p: PickedPhoto, index: number, now: number = Date.now()): string {
  const base = p.fileName?.trim() || `photo-${now}-${index + 1}.jpg`;
  return withExtension(base.replace(/[\\/:*?"<>|]/g, "_"), "webp");
}

/** Converts a picked photo to WebP (≤ 2560 px) plus a 640 px preview, ready for uploadFiles. */
export async function preparePhoto(p: PickedPhoto, index: number, deps: PhotoDeps): Promise<UploadItem> {
  const s = photoSizes(p.width, p.height);
  const full = await deps.toWebP(p.uri, s.full.width, s.full.height, WEBP_QUALITY);
  const preview = await deps.toWebP(p.uri, s.preview.width, s.preview.height, WEBP_QUALITY);
  const [data, previewBlob] = await Promise.all([deps.readBlob(full.uri), deps.readBlob(preview.uri)]);
  return { data, preview: previewBlob, name: photoName(p, index), type: "image/webp" };
}
