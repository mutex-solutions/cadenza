import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { ApiError } from "@cadenza/core";
import { Link, useRoute, type Route } from "./router.tsx";
import { useServices, useSync } from "./services.tsx";
import { ErrorText } from "../components/ErrorText.tsx";
import { Archived, Inbox } from "../pages/Inbox.tsx";
import { Login } from "../pages/Login.tsx";

// Everything past the inbox loads on demand, keeping the first load small.
const ThreadPage = lazy(() => import("../pages/Thread.tsx"));
const MachinesPage = lazy(() => import("../pages/Machines.tsx"));
const SettingsPage = lazy(() => import("../pages/Settings.tsx"));

type Auth = "checking" | "setup" | "login" | "in" | "error";
const SIGNED_IN = "cadenza.signedIn";

function remember(on: boolean) {
  try {
    if (on) localStorage.setItem(SIGNED_IN, "1");
    else localStorage.removeItem(SIGNED_IN);
  } catch {
    /* private mode */
  }
}

function remembered(): boolean {
  try {
    return localStorage.getItem(SIGNED_IN) === "1";
  } catch {
    return false;
  }
}

export function App() {
  const services = useServices();
  const { client, sync } = services;
  const [auth, setAuth] = useState<Auth>("checking");
  const [error, setError] = useState<unknown>(null);

  const check = useCallback(async () => {
    setAuth("checking");
    try {
      const v = await client.call("version");
      if (v.setup_needed) return setAuth("setup");
      await client.call("me");
      remember(true);
      setAuth("in");
    } catch (e) {
      if (e instanceof ApiError && e.unauthorized) {
        remember(false);
        setAuth("login");
      } else if (e instanceof ApiError && e.offline && remembered()) {
        setAuth("in"); // offline: show the saved copy
      } else {
        setError(e);
        setAuth("error");
      }
    }
  }, [client]);

  useEffect(() => {
    services.unauthorized.handler = () => {
      remember(false);
      setAuth("login");
    };
    void check();
  }, [check, services]);

  useEffect(() => {
    if (auth !== "in") return;
    void sync.start();
    const online = () => sync.reconnect();
    window.addEventListener("online", online);
    return () => {
      window.removeEventListener("online", online);
      sync.stop();
    };
  }, [auth, sync]);

  const logout = async () => {
    try {
      await client.call("logout");
    } catch {
      /* logged out anyway */
    }
    remember(false);
    await sync.clear();
    setAuth("login");
  };

  switch (auth) {
    case "checking":
      return <p className="muted" style={{ padding: 16 }}>Loading…</p>;
    case "setup":
    case "login":
      return (
        <Login
          setup={auth === "setup"}
          onDone={() => {
            remember(true);
            setAuth("in");
          }}
        />
      );
    case "error":
      return (
        <div className="auth">
          <h1>Can't open Cadenza</h1>
          <ErrorText error={error} />
          <button onClick={() => void check()}>Try again</button>
        </div>
      );
    case "in":
      return <Shell onLogout={logout} />;
  }
}

function Shell({ onLogout }: { onLogout: () => void }) {
  const route = useRoute();
  const s = useSync();
  const current = (name: Route["name"]) => (route.name === name ? "page" : undefined);
  const connText = { connecting: "Connecting", live: "Live", polling: "Checking every few seconds", offline: "Offline" }[s.connection];
  return (
    <>
      <header className="topbar">
        <div className="topbar-inner">
          <Link to="/" className="brand">
            <span className="brand-mark" aria-hidden="true" />
            Cadenza
          </Link>
          <span className="conn" data-state={s.connection} title={connText}>
            <span className="sr-only">Connection: </span>
            {s.connection === "live" ? "" : connText}
          </span>
          <nav className="nav" aria-label="Main">
            <Link to="/" aria-current={current("inbox")}>
              Inbox
            </Link>
            <Link to="/machines" aria-current={current("machines")}>
              Machines
            </Link>
            <Link to="/settings" aria-current={current("settings")}>
              Settings
            </Link>
          </nav>
        </div>
      </header>
      <main>
        <Suspense fallback={<p className="muted">Loading…</p>}>
          <Page route={route} onLogout={onLogout} />
        </Suspense>
      </main>
    </>
  );
}

function Page({ route, onLogout }: { route: Route; onLogout: () => void }) {
  switch (route.name) {
    case "inbox":
      return <Inbox />;
    case "archived":
      return <Archived />;
    case "thread":
      return <ThreadPage id={route.id} />;
    case "machines":
      return <MachinesPage />;
    case "settings":
      return <SettingsPage onLogout={onLogout} />;
    case "notfound":
      return (
        <div className="empty">
          <p>There's no page at {route.path}.</p>
          <Link to="/">Go to the inbox</Link>
        </div>
      );
  }
}
