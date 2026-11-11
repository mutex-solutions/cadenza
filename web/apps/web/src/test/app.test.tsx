import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../app/App.tsx";
import { navigate } from "../app/router.tsx";
import { makeServices, renderWith, thread } from "./helpers.tsx";

vi.mock("@cadenza/viewer", () => ({
  renderPost: (el: HTMLElement) => {
    el.textContent = "viewer";
    return { element: el, pickSpot: async () => null, cancelPick() {}, setMarkers() {}, show() {}, destroy() {} };
  },
}));

const version = (setup_needed = false) => ({ body: { server: "1.0.0", api: 1, min_cli_version: "1.0.0", setup_needed } });
const unauthorized = { status: 401, body: { error: { kind: "unauthorized", message: "no credentials", fix: "log in" } } };

beforeEach(() => {
  history.replaceState(null, "", "/");
});

describe("first run and login", () => {
  it("asks for a new password on first run and saves it", async () => {
    const { services, f } = makeServices({
      "GET /v1/version": () => version(true),
      "POST /v1/auth/setup": () => ({ body: { token: "t", expires: 1 } }),
      "GET /v1/threads": () => ({ body: { threads: [] } }),
    });
    renderWith(<App />, services);
    expect(await screen.findByRole("heading", { name: "Set your password" })).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "hunter22" } });
    fireEvent.change(screen.getByLabelText("Type it again"), { target: { value: "different" } });
    fireEvent.click(screen.getByRole("button", { name: "Save password" }));
    expect(await screen.findByText("The two passwords are different.")).toBeTruthy();
    expect(f.of("POST", "/v1/auth/setup")).toHaveLength(0);

    fireEvent.change(screen.getByLabelText("Type it again"), { target: { value: "hunter22" } });
    fireEvent.click(screen.getByRole("button", { name: "Save password" }));
    expect(await screen.findByText("Nothing here yet.")).toBeTruthy();
    const call = f.of("POST", "/v1/auth/setup")[0]!;
    expect(call.body).toEqual({ password: "hunter22" });
    expect(call.headers["Content-Type"]).toBe("application/json");
  });

  it("logs in, shows the server's error with its fix, then the inbox", async () => {
    let good = false;
    const { services, f } = makeServices({
      "GET /v1/version": () => version(),
      "GET /v1/me": () => unauthorized,
      "POST /v1/auth/login": (c) => {
        good = (c.body as { password: string }).password === "right";
        return good ? { body: { token: "t", expires: 1 } } : { status: 401, body: { error: { kind: "unauthorized", message: "Wrong password.", fix: "Check caps lock." } } };
      },
      "GET /v1/threads": () => ({ body: { threads: [thread("t1", { title: "Checkout redesign" })] } }),
    });
    renderWith(<App />, services);
    fireEvent.change(await screen.findByLabelText("Password"), { target: { value: "wrong" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByText("Wrong password.")).toBeTruthy();
    expect(screen.getByText("Check caps lock.")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "right" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByText("Checkout redesign")).toBeTruthy();
    expect(f.of("POST", "/v1/auth/login")).toHaveLength(2);
  });

  it("goes back to login when the session expires", async () => {
    let expired = false;
    const { services } = makeServices({
      "GET /v1/version": () => version(),
      "GET /v1/me": () => ({ body: { owner_id: "o" } }),
      "GET /v1/threads": () => (expired ? unauthorized : { body: { threads: [] } }),
    });
    renderWith(<App />, services);
    expect(await screen.findByText("Nothing here yet.")).toBeTruthy();
    expired = true;
    await act(() => services.sync.refreshInbox());
    expect(await screen.findByRole("heading", { name: "Log in" })).toBeTruthy();
  });

  it("shows a retry screen when the server can't be reached on first open", async () => {
    const { services } = makeServices({}, { fetch: (async () => { throw new TypeError("offline"); }) as typeof fetch });
    renderWith(<App />, services);
    expect(await screen.findByText("Can't open Cadenza")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
  });
});

describe("inbox", () => {
  it("lists threads from every machine with status, open questions and unread", async () => {
    const now = Date.now();
    let threads = [
      thread("t1", { title: "Checkout redesign", status: "ready", open_questions: 2, machine: "vm-03", updated_at: now - 1000 }),
      thread("t2", { title: "Flaky test", agent: "codex", machine: "vm-07", status: "approved", updated_at: now - 5000 }),
    ];
    const { services } = makeServices({
      "GET /v1/version": () => version(),
      "GET /v1/me": () => ({ body: { owner_id: "o" } }),
      "GET /v1/threads": () => ({ body: { threads } }),
    });
    renderWith(<App />, services);
    const list = await screen.findByRole("list", { name: "Threads" });
    const rows = within(list).getAllByRole("link");
    expect(rows).toHaveLength(2);
    expect(rows[0]!.textContent).toContain("Checkout redesign");
    expect(rows[0]!.textContent).toContain("Claude Code · vm-03");
    expect(rows[0]!.textContent).toContain("Ready to review");
    expect(rows[0]!.textContent).toContain("2 questions");
    expect(rows[1]!.textContent).toContain("Codex · vm-07");
    expect(rows[1]!.textContent).toContain("Approved");
    expect(rows[0]!.getAttribute("href")).toBe("/t/t1");
    expect(screen.getByText("2 open")).toBeTruthy();
    // Threads present on first load count as seen.
    expect(rows[0]!.getAttribute("data-unread")).toBe("false");

    // A change arrives: that thread becomes unread.
    threads = [{ ...threads[0]!, updated_at: now + 1000 }, threads[1]!];
    await act(() => services.sync.refreshInbox());
    await waitFor(() => expect(screen.getAllByRole("link", { name: /Checkout redesign/ })[0]!.getAttribute("data-unread")).toBe("true"));
    expect(screen.getByText("Unread:")).toBeTruthy();
  });

  it("opens a thread without reloading the page", async () => {
    const { services } = makeServices({
      "GET /v1/version": () => version(),
      "GET /v1/me": () => ({ body: { owner_id: "o" } }),
      "GET /v1/threads": () => ({ body: { threads: [thread("t1", { title: "Checkout redesign" })] } }),
      "GET /v1/threads/t1": () => ({ body: { thread: thread("t1", { title: "Checkout redesign" }), posts: [], questions: [], replies: [] } }),
    });
    renderWith(<App />, services);
    fireEvent.click(await screen.findByRole("link", { name: /Checkout redesign/ }));
    expect(location.pathname).toBe("/t/t1");
    expect(await screen.findByText("The agent hasn't shown anything in this thread yet.")).toBeTruthy();
  });

  it("shows archived threads on their own page", async () => {
    const { services, f } = makeServices({
      "GET /v1/version": () => version(),
      "GET /v1/me": () => ({ body: { owner_id: "o" } }),
      "GET /v1/threads": (c) => ({
        body: { threads: c.url.includes("archived=1") ? [thread("old", { title: "Old work", status: "archived" }), thread("t1")] : [thread("t1")] },
      }),
    });
    renderWith(<App />, services);
    await screen.findByRole("list", { name: "Threads" });
    act(() => navigate("/archived"));
    const list = await screen.findByRole("list", { name: "Archived threads" });
    expect(within(list).getAllByRole("link")).toHaveLength(1);
    expect(within(list).getByText("Old work")).toBeTruthy();
    expect(f.calls.some((c) => c.url === "/v1/threads?archived=1")).toBe(true);
  });

  it("shows a not-found page for unknown paths", async () => {
    history.replaceState(null, "", "/nope");
    const { services } = makeServices({
      "GET /v1/version": () => version(),
      "GET /v1/me": () => ({ body: { owner_id: "o" } }),
      "GET /v1/threads": () => ({ body: { threads: [] } }),
    });
    renderWith(<App />, services);
    expect(await screen.findByText("There's no page at /nope.")).toBeTruthy();
  });

  it("logs out from settings", async () => {
    history.replaceState(null, "", "/settings");
    const { services, f } = makeServices({
      "GET /v1/version": () => version(),
      "GET /v1/me": () => ({ body: { owner_id: "o" } }),
      "GET /v1/threads": () => ({ body: { threads: [] } }),
      "GET /v1/notifications": () => ({ body: { items: [] } }),
      "POST /v1/auth/logout": () => ({ body: {} }),
    });
    renderWith(<App />, services);
    fireEvent.click(await screen.findByRole("button", { name: "Log out" }));
    expect(await screen.findByRole("heading", { name: "Log in" })).toBeTruthy();
    expect(f.of("POST", "/v1/auth/logout")).toHaveLength(1);
  });
});
