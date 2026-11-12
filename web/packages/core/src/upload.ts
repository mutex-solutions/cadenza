import type { FileRef, Upload } from "./api.gen.ts";
import { ApiError, type Client } from "./client.ts";

/** Lowercase hex SHA-256 of some bytes, using the platform's SubtleCrypto. */
export async function sha256Hex(data: Blob | ArrayBuffer | Uint8Array): Promise<string> {
  let buf: ArrayBuffer;
  if (data instanceof Blob) buf = await data.arrayBuffer();
  else if (data instanceof Uint8Array) buf = data.slice().buffer as ArrayBuffer;
  else buf = data;
  const digest = await globalThis.crypto.subtle.digest("SHA-256", buf);
  let hex = "";
  for (const b of new Uint8Array(digest)) hex += b.toString(16).padStart(2, "0");
  return hex;
}

/** One file to upload. */
export interface UploadItem {
  data: Blob;
  name: string;
  /** Content type; must be one the server stores (image/webp, image/jpeg, image/png, video/mp4, text/html, application/pdf, application/octet-stream). */
  type: string;
  /** A small preview for images (same type as the image); uploaded too and linked as preview_sha256. */
  preview?: Blob;
}

export interface UploadOptions {
  /** fetch for the signed PUTs; defaults to the global fetch. */
  fetch?: typeof fetch;
  /** Called after each file is stored, with how many are done out of the total. */
  onProgress?: (done: number, total: number) => void;
}

/** Makes the FileRef the server expects for some bytes. */
export async function fileRef(data: Blob, name: string, type: string): Promise<FileRef> {
  return { sha256: await sha256Hex(data), size: data.size, type, name };
}

/**
 * Stores files: hashes them, asks the server which are new, PUTs only those to
 * their signed links with exactly the headers given, and returns the FileRefs
 * to send with an answer or reply. Files the server already has are skipped.
 */
export async function uploadFiles(client: Client, items: UploadItem[], opts: UploadOptions = {}): Promise<FileRef[]> {
  if (items.length === 0) return [];
  const doFetch = opts.fetch ?? ((input: RequestInfo | URL, init?: RequestInit) => globalThis.fetch(input, init));
  const refs: FileRef[] = [];
  const all: { ref: FileRef; data: Blob }[] = [];
  for (const it of items) {
    const ref = await fileRef(it.data, it.name, it.type);
    if (it.preview) {
      const base = it.name.replace(/\.[^.]+$/, "");
      const jpeg = it.type === "image/jpeg";
      const pref = await fileRef(it.preview, `${base}.preview.${jpeg ? "jpg" : "webp"}`, jpeg ? "image/jpeg" : "image/webp");
      ref.preview_sha256 = pref.sha256;
      all.push({ ref: pref, data: it.preview });
    }
    refs.push(ref);
    all.push({ ref, data: it.data });
  }
  const res = await client.call("upload", { body: { files: all.map((a) => a.ref) } });
  const bySha = new Map(all.map((a) => [a.ref.sha256, a.data]));
  const total = res.uploads.length;
  let done = 0;
  opts.onProgress?.(done, total);
  await Promise.all(
    res.uploads.map(async (u: Upload) => {
      const data = bySha.get(u.sha256);
      if (!data) throw new ApiError("invalid", `The server asked for a file that was not sent (${u.sha256.slice(0, 12)})`, 0);
      let put: globalThis.Response;
      try {
        put = await doFetch(u.url, { method: u.method || "PUT", headers: u.headers, body: data });
      } catch {
        throw new ApiError("offline", "Upload failed: can't reach the file store. Try again.", 0);
      }
      if (!put.ok) {
        throw new ApiError("unavailable", `Upload failed: the file store answered ${put.status}.`, put.status, "Try again. If it keeps failing, check the server's storage settings (CORS must allow uploads from this app).");
      }
      done++;
      opts.onProgress?.(done, total);
    }),
  );
  return refs;
}
