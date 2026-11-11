/** The result of checking a server address typed by the owner. */
export type ServerUrlCheck =
  | { ok: true; url: string; warning?: string }
  | { ok: false; error: string };

/** Hosts where plain http is acceptable: this device, home networks and Tailscale. */
export function isPrivateHost(host: string): boolean {
  const h = host.toLowerCase().replace(/^\[|\]$/g, "");
  if (h === "localhost" || h === "127.0.0.1" || h === "::1" || h.endsWith(".local") || h.endsWith(".ts.net")) return true;
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(h);
  if (!m) return false;
  const [a, b] = [Number(m[1]), Number(m[2])];
  if (a === 10 || a === 127) return true;
  if (a === 192 && b === 168) return true;
  if (a === 172 && b >= 16 && b <= 31) return true;
  if (a === 100 && b >= 64 && b <= 127) return true; // Tailscale (CGNAT range)
  return false;
}

/** Checks and normalizes a server address: https, or http on a private network with a warning. */
export function checkServerUrl(input: string): ServerUrlCheck {
  let s = input.trim();
  if (!s) return { ok: false, error: "Enter your Cadenza server address." };
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(s)) s = `https://${s}`;
  let u: URL;
  try {
    u = new URL(s);
  } catch {
    return { ok: false, error: "That doesn't look like a web address." };
  }
  if (u.protocol !== "https:" && u.protocol !== "http:") return { ok: false, error: "Use an https:// address." };
  if (!u.hostname) return { ok: false, error: "That doesn't look like a web address." };
  const url = `${u.protocol}//${u.host}${u.pathname.replace(/\/+$/, "")}`;
  if (u.protocol === "http:") {
    if (!isPrivateHost(u.hostname)) {
      return { ok: false, error: "Use https:// for servers on the internet. Plain http:// only works on your own network." };
    }
    return { ok: true, url, warning: "This connection isn't encrypted. That's fine on your own network or Tailscale." };
  }
  return { ok: true, url };
}
