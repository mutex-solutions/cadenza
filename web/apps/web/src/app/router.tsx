import { useSyncExternalStore, type AnchorHTMLAttributes, type MouseEvent } from "react";

/** The screens of the app. Thread URLs are /t/<thread-id> (the server and notifications use them). */
export type Route =
  | { name: "inbox" }
  | { name: "archived" }
  | { name: "thread"; id: string }
  | { name: "machines" }
  | { name: "settings" }
  | { name: "notfound"; path: string };

export function match(path: string): Route {
  const p = path.replace(/\/+$/, "") || "/";
  if (p === "/") return { name: "inbox" };
  if (p === "/archived") return { name: "archived" };
  if (p === "/machines") return { name: "machines" };
  if (p === "/settings") return { name: "settings" };
  const t = /^\/t\/([^/]+)$/.exec(p);
  if (t?.[1]) {
    try {
      return { name: "thread", id: decodeURIComponent(t[1]) };
    } catch {
      return { name: "notfound", path };
    }
  }
  return { name: "notfound", path };
}

export function threadPath(id: string): string {
  return `/t/${encodeURIComponent(id)}`;
}

const listeners = new Set<() => void>();
const notify = () => {
  for (const f of listeners) f();
};

function subscribe(f: () => void) {
  listeners.add(f);
  if (listeners.size === 1) window.addEventListener("popstate", notify);
  return () => {
    listeners.delete(f);
    if (listeners.size === 0) window.removeEventListener("popstate", notify);
  };
}

export function navigate(to: string, opts: { replace?: boolean } = {}): void {
  if (to === location.pathname + location.search) return;
  if (opts.replace) history.replaceState(null, "", to);
  else history.pushState(null, "", to);
  window.scrollTo?.(0, 0);
  notify();
}

/** The current path; re-renders on navigation. */
export function usePath(): string {
  return useSyncExternalStore(subscribe, () => location.pathname);
}

export function useRoute(): Route {
  return match(usePath());
}

/** An <a> that navigates without reloading (unless opened in a new tab). */
export function Link({ to, onClick, ...rest }: AnchorHTMLAttributes<HTMLAnchorElement> & { to: string }) {
  const handle = (e: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(e);
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    navigate(to);
  };
  return <a href={to} onClick={handle} {...rest} />;
}
