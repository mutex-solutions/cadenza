import type { Client } from "@cadenza/core";

const DEVICE_KEY = "cadenza.device";

/** Decodes a base64url VAPID key into bytes for pushManager.subscribe. */
export function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padded = base64.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (base64.length % 4)) % 4);
  const raw = atob(padded);
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

export type PushSupport = "supported" | "unsupported" | "ios-needs-install";

/** Whether this browser can get Web Push, and if not, why. */
export function pushSupport(nav: Navigator = navigator, win: Window = window): PushSupport {
  const ios = /iPad|iPhone|iPod/.test(nav.userAgent) || (nav.platform === "MacIntel" && nav.maxTouchPoints > 1);
  const standalone = win.matchMedia?.("(display-mode: standalone)").matches || (nav as Navigator & { standalone?: boolean }).standalone === true;
  if (ios && !standalone) return "ios-needs-install";
  if (!("serviceWorker" in nav) || !("PushManager" in win) || !("Notification" in win)) return "unsupported";
  return "supported";
}

/** A short name for this browser, shown in the notification log. */
export function deviceName(ua: string = navigator.userAgent): string {
  const browser = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Browser";
  const os = /Android/.test(ua) ? "Android" : /iPhone|iPad|iPod/.test(ua) ? "iPhone" : /Mac OS X/.test(ua) ? "Mac" : /Windows/.test(ua) ? "Windows" : /Linux/.test(ua) ? "Linux" : "";
  return os ? `${browser} on ${os}` : browser;
}

export function savedDeviceId(): string | null {
  try {
    return localStorage.getItem(DEVICE_KEY);
  } catch {
    return null;
  }
}

/** Forgets which device this browser is (after it was removed on the server). */
export function forgetDeviceId(): void {
  try {
    localStorage.removeItem(DEVICE_KEY);
  } catch {
    /* ignore */
  }
}

/** Turns on notifications for this browser: asks permission, subscribes, registers with the server. */
export async function enablePush(client: Client, nav: Navigator = navigator): Promise<string> {
  const cfg = await client.call("push-config");
  if (!cfg.providers.includes("webpush") || !cfg.vapid_public_key) {
    throw new Error("This server has browser notifications turned off.");
  }
  const permission = await Notification.requestPermission();
  if (permission !== "granted") {
    throw new Error("Notifications are blocked. Allow them for this site in your browser settings, then try again.");
  }
  const reg = await nav.serviceWorker.register("/sw.js");
  await nav.serviceWorker.ready;
  const key = urlBase64ToUint8Array(cfg.vapid_public_key);
  let sub = await reg.pushManager.getSubscription();
  if (sub) {
    // A subscription made with another key can't receive our messages.
    const old = sub.options.applicationServerKey;
    if (old && !sameBytes(new Uint8Array(old), key)) {
      await sub.unsubscribe();
      sub = null;
    }
  }
  sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
  const out = await client.call("device-register", { body: { provider: "webpush", token: sub.toJSON(), name: deviceName(nav.userAgent) } });
  try {
    localStorage.setItem(DEVICE_KEY, out.device_id);
  } catch {
    /* private mode: the device still works, we just forget its id */
  }
  return out.device_id;
}

/** Turns notifications off for this browser. */
export async function disablePush(client: Client, nav: Navigator = navigator): Promise<void> {
  const id = savedDeviceId();
  if (id) await client.call("device-remove", { params: { id } });
  forgetDeviceId();
  const reg = await nav.serviceWorker?.getRegistration("/sw.js");
  const sub = await reg?.pushManager.getSubscription();
  await sub?.unsubscribe();
}

function sameBytes(a: Uint8Array, b: Uint8Array): boolean {
  return a.length === b.length && a.every((v, i) => v === b[i]);
}
