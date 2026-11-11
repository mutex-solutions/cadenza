import type { FileURL } from "@cadenza/core";

/** Signed links expire; reuse one only while this much time is left. */
export const URL_MARGIN_MS = 60_000;

/**
 * Remembers signed download links per file until shortly before they expire,
 * and shares one request between callers asking for the same file at once.
 */
export class FileUrlCache {
  private urls = new Map<string, FileURL>();
  private inflight = new Map<string, Promise<string>>();

  constructor(
    private readonly fetchUrl: (sha256: string) => Promise<FileURL>,
    private readonly now: () => number = Date.now,
  ) {}

  async get(sha256: string): Promise<string> {
    const hit = this.urls.get(sha256);
    if (hit && hit.expires - URL_MARGIN_MS > this.now()) return hit.url;
    const running = this.inflight.get(sha256);
    if (running) return running;
    const p = this.fetchUrl(sha256)
      .then((u) => {
        this.urls.set(sha256, u);
        return u.url;
      })
      .finally(() => this.inflight.delete(sha256));
    this.inflight.set(sha256, p);
    return p;
  }

  clear(): void {
    this.urls.clear();
  }
}
