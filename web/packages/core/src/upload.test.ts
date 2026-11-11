import { describe, expect, it } from "vitest";
import { createClient } from "./client.ts";
import { fileRef, sha256Hex, uploadFiles } from "./upload.ts";
import { fakeFetch } from "./test/fakes.ts";

const ABC_SHA = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";

describe("sha256Hex", () => {
  it("hashes blobs, buffers and byte arrays", async () => {
    expect(await sha256Hex(new Blob(["abc"]))).toBe(ABC_SHA);
    expect(await sha256Hex(new TextEncoder().encode("abc"))).toBe(ABC_SHA);
    expect(await sha256Hex(new TextEncoder().encode("abc").buffer as ArrayBuffer)).toBe(ABC_SHA);
  });
});

describe("uploadFiles", () => {
  it("asks which files are new, PUTs only those with the given headers, and links previews", async () => {
    const full = new Blob(["full-bytes"], { type: "image/webp" });
    const preview = new Blob(["small"], { type: "image/webp" });
    const fullRef = await fileRef(full, "x", "image/webp");
    const previewRef = await fileRef(preview, "x", "image/webp");
    const puts: { url: string; headers: Record<string, string>; method: string }[] = [];
    const api = fakeFetch({
      "POST /v1/uploads": (call) => {
        const files = (call.body as { files: { sha256: string; name: string }[] }).files;
        expect(files.map((f) => f.sha256)).toEqual([previewRef.sha256, fullRef.sha256]);
        expect(files[0]!.name).toBe("photo.preview.webp");
        return {
          body: {
            uploads: [{ sha256: fullRef.sha256, url: "https://s3/put-full", method: "PUT", headers: { "Content-Type": "image/webp", "x-amz-checksum-sha256": "zzz" }, expires: 1 }],
            existing: [previewRef.sha256],
          },
        };
      },
    });
    const storeFetch = (async (url: string, init: RequestInit) => {
      puts.push({ url, headers: init.headers as Record<string, string>, method: init.method! });
      expect(init.body).toBe(full);
      return new Response("", { status: 200 });
    }) as unknown as typeof fetch;
    const progress: [number, number][] = [];
    const refs = await uploadFiles(createClient({ fetch: api.fetch }), [{ data: full, name: "photo.webp", type: "image/webp", preview }], {
      fetch: storeFetch,
      onProgress: (d, t) => progress.push([d, t]),
    });
    expect(refs).toEqual([{ sha256: fullRef.sha256, size: full.size, type: "image/webp", name: "photo.webp", preview_sha256: previewRef.sha256 }]);
    expect(puts).toEqual([{ url: "https://s3/put-full", method: "PUT", headers: { "Content-Type": "image/webp", "x-amz-checksum-sha256": "zzz" } }]);
    expect(progress).toEqual([
      [0, 1],
      [1, 1],
    ]);
  });

  it("does nothing for an empty list", async () => {
    const api = fakeFetch({});
    expect(await uploadFiles(createClient({ fetch: api.fetch }), [])).toEqual([]);
    expect(api.calls).toHaveLength(0);
  });

  it("fails clearly when the store refuses a PUT", async () => {
    const data = new Blob(["a"]);
    const ref = await fileRef(data, "a", "application/octet-stream");
    const api = fakeFetch({ "POST /v1/uploads": () => ({ body: { uploads: [{ sha256: ref.sha256, url: "u", method: "PUT", headers: {}, expires: 1 }], existing: [] } }) });
    const bad = (async () => new Response("", { status: 403 })) as unknown as typeof fetch;
    await expect(uploadFiles(createClient({ fetch: api.fetch }), [{ data, name: "a", type: "application/octet-stream" }], { fetch: bad })).rejects.toThrow(/answered 403/);
  });

  it("fails clearly when the store can't be reached", async () => {
    const data = new Blob(["a"]);
    const ref = await fileRef(data, "a", "application/octet-stream");
    const api = fakeFetch({ "POST /v1/uploads": () => ({ body: { uploads: [{ sha256: ref.sha256, url: "u", method: "PUT", headers: {}, expires: 1 }], existing: [] } }) });
    const down = (async () => {
      throw new TypeError("network");
    }) as unknown as typeof fetch;
    await expect(uploadFiles(createClient({ fetch: api.fetch }), [{ data, name: "a", type: "application/octet-stream" }], { fetch: down })).rejects.toThrow(/can't reach/);
  });

  it("refuses an upload link for a file it didn't send", async () => {
    const api = fakeFetch({ "POST /v1/uploads": () => ({ body: { uploads: [{ sha256: "f".repeat(64), url: "u", method: "PUT", headers: {}, expires: 1 }], existing: [] } }) });
    await expect(uploadFiles(createClient({ fetch: api.fetch }), [{ data: new Blob(["a"]), name: "a", type: "application/pdf" }])).rejects.toThrow(/was not sent/);
  });
});
