import { withExtension, type Client, type ImageConverter, type UploadItem } from "@cadenza/core";

/**
 * resolveFile for the viewer: asks the server for a signed link and reuses it
 * until shortly before it expires.
 */
export function fileResolver(client: Client, now: () => number = Date.now): (sha256: string) => Promise<string> {
  const cache = new Map<string, { url: string; expires: number }>();
  const pending = new Map<string, Promise<string>>();
  return (sha256) => {
    const hit = cache.get(sha256);
    if (hit && hit.expires - 30_000 > now()) return Promise.resolve(hit.url);
    const p = pending.get(sha256);
    if (p) return p;
    const req = client
      .call("file-url", { params: { sha256 } })
      .then((r) => {
        cache.set(sha256, { url: r.url, expires: r.expires });
        return r.url;
      })
      .finally(() => pending.delete(sha256));
    pending.set(sha256, req);
    return req;
  };
}

/** Content types the server stores for other files. */
function storedType(file: File): string {
  if (file.type === "video/mp4") return "video/mp4";
  if (file.type === "application/pdf") return "application/pdf";
  if (file.type === "image/webp") return "image/webp";
  if (file.type === "image/jpeg") return "image/jpeg";
  if (file.type === "image/png") return "image/png";
  return "application/octet-stream";
}

/**
 * Prepares picked files for upload. Photos are converted to WebP (max 2560 px,
 * plus a 640 px preview) on this device, or to JPEG where the browser can't
 * make WebP (older Safari). If conversion fails, the original is sent.
 */
export async function toUploadItems(files: File[], converter: ImageConverter): Promise<UploadItem[]> {
  const out: UploadItem[] = [];
  for (const f of files) {
    if (f.type.startsWith("image/")) {
      try {
        const img = await converter.toWebP(f);
        if (img.type === "image/webp" || img.type === "image/jpeg") {
          const ext = img.type === "image/webp" ? "webp" : "jpg";
          out.push({ data: img.full, name: withExtension(f.name || "photo", ext), type: img.type, preview: img.preview });
          continue;
        }
      } catch {
        // Fall through and send the original.
      }
    }
    out.push({ data: f, name: f.name || "file", type: storedType(f) });
  }
  return out;
}
