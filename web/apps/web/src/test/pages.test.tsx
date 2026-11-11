import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import MachinesPage, { normalizeCode } from "../pages/Machines.tsx";
import SettingsPage from "../pages/Settings.tsx";
import { makeServices, renderWith } from "./helpers.tsx";

const machine = (id: string, name: string) => ({ id, name, created_at: 1, last_seen_at: Date.now() - 120_000, sessions: 2 });
const session = (id: string, machine_id: string, extra = {}) => ({ id, machine_id, machine: "vm", agent: "claude-code", created_at: Date.now() - 3_600_000, last_used_at: Date.now() - 60_000, ...extra });

afterEach(() => vi.unstubAllGlobals());

describe("machines", () => {
  it("approves a pairing code, renames and removes machines", async () => {
    let machines = [machine("m_1", "vm-01")];
    const { services, f } = makeServices({
      "GET /v1/machines": () => ({ body: { machines } }),
      "GET /v1/sessions": () => ({ body: { sessions: [session("s_1", "m_1"), session("s_2", "m_1")] } }),
      "POST /v1/pair/approve": (c) => {
        machines = [...machines, machine("m_2", (c.body as { name: string }).name)];
        return { body: { machine_id: "m_2", name: "vm-03" } };
      },
      "POST /v1/machines/m_1/rename": () => ({ body: {} }),
      "DELETE /v1/machines/m_1": () => ({ body: {} }),
    });
    renderWith(<MachinesPage />, services);
    expect(await screen.findByText("vm-01")).toBeTruthy();
    expect(screen.getByText(/2 agent sessions/)).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Code"), { target: { value: "kq7-4xm " } });
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: " vm-03 " } });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    expect(await screen.findByText(/vm-03 is paired/)).toBeTruthy();
    expect(f.of("POST", "/v1/pair/approve")[0]!.body).toEqual({ code: "KQ7-4XM", name: "vm-03" });
    expect(await screen.findByText("vm-03")).toBeTruthy();

    const first = screen.getByText("vm-01").closest("li")!;
    fireEvent.click(within(first).getByRole("button", { name: "Rename" }));
    fireEvent.change(screen.getByLabelText("New name for vm-01"), { target: { value: "build-box" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(f.of("POST", "/v1/machines/m_1/rename")).toHaveLength(1));
    expect(f.of("POST", "/v1/machines/m_1/rename")[0]!.body).toEqual({ name: "build-box" });

    vi.stubGlobal("confirm", () => true);
    const row = screen.getByText("vm-01").closest("li")!;
    fireEvent.click(within(row).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(f.of("DELETE", "/v1/machines/m_1")).toHaveLength(1));
  });

  it("does not remove when the owner cancels", async () => {
    const { services, f } = makeServices({ "GET /v1/machines": () => ({ body: { machines: [machine("m_1", "vm-01")] } }), "GET /v1/sessions": () => ({ body: { sessions: [] } }) });
    vi.stubGlobal("confirm", () => false);
    renderWith(<MachinesPage />, services);
    fireEvent.click(await screen.findByRole("button", { name: "Remove" }));
    await new Promise((r) => setTimeout(r, 10));
    expect(f.of("DELETE", "/v1/machines/m_1")).toHaveLength(0);
  });

  it("shows a wrong code error", async () => {
    const { services } = makeServices({
      "GET /v1/machines": () => ({ body: { machines: [] } }),
      "GET /v1/sessions": () => ({ body: { sessions: [] } }),
      "POST /v1/pair/approve": () => ({ status: 404, body: { error: { kind: "not_found", message: "No pairing with that code.", fix: "Run cadenza login again for a new code." } } }),
    });
    renderWith(<MachinesPage />, services);
    expect(await screen.findByText("No machines yet.")).toBeTruthy();
    expect(screen.queryByLabelText("Session id")).toBeNull();
    fireEvent.change(screen.getByLabelText("Code"), { target: { value: "AAA-BBB" } });
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    expect(await screen.findByText("No pairing with that code.")).toBeTruthy();
  });

  it("lists each machine's live sessions and revokes one", async () => {
    let sessions = [session("s_1", "m_1", { repo: "github.com/acme/shop" }), session("s_2", "m_2", { agent: "codex", cwd: "/srv/api" })];
    const { services, f } = makeServices({
      "GET /v1/machines": () => ({ body: { machines: [machine("m_1", "vm-01"), machine("m_2", "vm-02")] } }),
      "GET /v1/sessions": () => ({ body: { sessions } }),
      "DELETE /v1/sessions/s_1": () => {
        sessions = sessions.filter((s) => s.id !== "s_1");
        return { body: {} };
      },
    });
    renderWith(<MachinesPage />, services);
    const first = await screen.findByRole("list", { name: "Agent sessions on vm-01" });
    expect(within(first).getByText("github.com/acme/shop")).toBeTruthy();
    expect(within(first).getByText("Claude Code")).toBeTruthy();
    const second = screen.getByRole("list", { name: "Agent sessions on vm-02" });
    expect(within(second).getByText("Codex")).toBeTruthy();
    expect(within(second).getByText("/srv/api")).toBeTruthy();
    expect(screen.getAllByText(/1 agent session$/)).toHaveLength(2);

    fireEvent.click(within(first).getByRole("button", { name: "Revoke Claude Code session in github.com/acme/shop" }));
    await waitFor(() => expect(f.of("DELETE", "/v1/sessions/s_1")).toHaveLength(1));
    await waitFor(() => expect(screen.queryByRole("list", { name: "Agent sessions on vm-01" })).toBeNull());
  });

  it("reloads when a machine changes", async () => {
    const { services, f } = makeServices({ "GET /v1/machines": () => ({ body: { machines: [] } }), "GET /v1/sessions": () => ({ body: { sessions: [] } }) });
    renderWith(<MachinesPage />, services);
    await screen.findByText("No machines yet.");
    // A machine.changed event from the server reaches the page through the sync engine's onEvent.
    await act(async () => {
      (services.sync as unknown as { o: { onEvent: (e: unknown) => void } }).o.onEvent({ seq: 1, kind: "machine.changed", at: 1 });
    });
    await waitFor(() => expect(f.of("GET", "/v1/machines").length).toBe(2));
  });

  it("normalizes codes", () => {
    expect(normalizeCode(" kq7 4xm")).toBe("KQ74XM");
    expect(normalizeCode("kq7-4xm")).toBe("KQ7-4XM");
  });
});

describe("settings", () => {
  it("lists devices, removes another one, and forgets a saved id the server no longer has", async () => {
    stubPush();
    localStorage.setItem("cadenza.device", "gone");
    let devices = [{ id: "d_9", provider: "expo", name: "Pixel 6a", created_at: Date.now(), last_ok_at: 0 }];
    const { services, f } = makeServices({
      "GET /v1/notifications": () => ({ body: { items: [] } }),
      "GET /v1/version": () => ({ body: { server: "1", api: 1, min_cli_version: "1", setup_needed: false } }),
      "GET /v1/devices": () => ({ body: { devices } }),
      "DELETE /v1/devices/d_9": () => {
        devices = [];
        return { body: {} };
      },
    });
    renderWith(<SettingsPage onLogout={() => {}} />, services);
    const list = await screen.findByRole("list", { name: "Devices" });
    expect(within(list).getByText(/Android app · added now · nothing delivered yet/)).toBeTruthy();
    expect(within(list).queryByText("This browser")).toBeNull();
    expect(localStorage.getItem("cadenza.device")).toBeNull();
    expect(screen.getByRole("button", { name: "Turn on notifications" })).toBeTruthy();
    fireEvent.click(within(list).getByRole("button", { name: "Remove Pixel 6a" }));
    await waitFor(() => expect(f.of("DELETE", "/v1/devices/d_9")).toHaveLength(1));
    expect(await screen.findByText("No devices yet.")).toBeTruthy();
  });

  function stubPush() {
    const subscription = { toJSON: () => ({ endpoint: "https://push.example/abc", keys: { p256dh: "k", auth: "a" } }), options: { applicationServerKey: null }, unsubscribe: vi.fn(async () => true) };
    const pushManager = { getSubscription: vi.fn(async () => null as typeof subscription | null), subscribe: vi.fn(async () => subscription) };
    const reg = { pushManager };
    vi.stubGlobal("navigator", {
      userAgent: "Mozilla/5.0 (Linux; Android 14) Chrome/130.0",
      platform: "Linux",
      maxTouchPoints: 5,
      serviceWorker: { register: vi.fn(async () => reg), ready: Promise.resolve(reg), getRegistration: vi.fn(async () => ({ pushManager: { getSubscription: async () => subscription } })) },
    });
    vi.stubGlobal("PushManager", function PushManager() {});
    vi.stubGlobal("Notification", { requestPermission: vi.fn(async () => "granted") });
    return { subscription, pushManager };
  }

  it("turns on Web Push with the server's VAPID key and registers the device", async () => {
    const { pushManager } = stubPush();
    let devices = [{ id: "d_0", provider: "expo", name: "Pixel 6a", created_at: Date.now() - 86_400_000, last_ok_at: Date.now() - 60_000 }];
    const { services, f } = makeServices({
      "GET /v1/notifications": () => ({ body: { items: [{ id: "n1", device: "Chrome on Android", provider: "webpush", title: "Claude Code · vm-03", body: "Checkout redesign: keep promo?", status: "failed", error: "gone", created_at: Date.now() }] } }),
      "GET /v1/version": () => ({ body: { server: "1.2.3", api: 1, min_cli_version: "1", setup_needed: false } }),
      "GET /v1/push/config": () => ({ body: { providers: ["webpush"], vapid_public_key: "BAEC" } }),
      "GET /v1/devices": () => ({ body: { devices } }),
      "POST /v1/devices": () => {
        devices = [...devices, { id: "d_1", provider: "webpush", name: "Chrome on Android", created_at: Date.now(), last_ok_at: 0 }];
        return { body: { device_id: "d_1" } };
      },
      "DELETE /v1/devices/d_1": () => {
        devices = devices.filter((d) => d.id !== "d_1");
        return { body: {} };
      },
      "POST /v1/notifications/test": () => ({ body: {} }),
    });
    renderWith(<SettingsPage onLogout={() => {}} />, services);
    expect(await screen.findByText("Checkout redesign: keep promo?")).toBeTruthy();
    expect(screen.getByText("Failed: gone")).toBeTruthy();
    expect(await screen.findByText("Server 1.2.3 · API 1")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Turn on notifications" }));
    expect(await screen.findByText("Notifications are on for this browser.")).toBeTruthy();
    const sub = (pushManager.subscribe.mock.calls[0] as unknown[])[0] as { userVisibleOnly: boolean; applicationServerKey: Uint8Array };
    expect(sub.userVisibleOnly).toBe(true);
    expect(Array.from(sub.applicationServerKey)).toEqual([4, 1, 2]);
    expect(f.of("POST", "/v1/devices")[0]!.body).toEqual({ provider: "webpush", token: { endpoint: "https://push.example/abc", keys: { p256dh: "k", auth: "a" } }, name: "Chrome on Android" });
    expect(localStorage.getItem("cadenza.device")).toBe("d_1");
    const list = await screen.findByRole("list", { name: "Devices" });
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(2));
    expect(within(list).getByText("Pixel 6a")).toBeTruthy();
    expect(within(list).getByText("This browser")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Send a test notification" }));
    expect(await screen.findByText(/Sent. It should arrive/)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Turn off" }));
    expect(await screen.findByRole("button", { name: "Turn on notifications" })).toBeTruthy();
    expect(f.of("DELETE", "/v1/devices/d_1")).toHaveLength(1);
  });

  it("explains when the server has browser notifications off, or the test reaches no device", async () => {
    stubPush();
    const { services } = makeServices({
      "GET /v1/notifications": () => ({ body: { items: [] } }),
      "GET /v1/version": () => ({ body: { server: "1", api: 1, min_cli_version: "1", setup_needed: false } }),
      "GET /v1/push/config": () => ({ body: { providers: ["expo"] } }),
      "GET /v1/devices": () => ({ body: { devices: [] } }),
      "POST /v1/notifications/test": () => ({ status: 503, body: { error: { kind: "unavailable", message: "no device received the test notification", fix: "allow notifications in this app" } } }),
    });
    renderWith(<SettingsPage onLogout={() => {}} />, services);
    fireEvent.click(await screen.findByRole("button", { name: "Turn on notifications" }));
    expect(await screen.findByText("This server has browser notifications turned off.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Send a test notification" }));
    expect(await screen.findByText("no device received the test notification")).toBeTruthy();
  });
});
