import { useCallback, useEffect, useState } from "react";
import { timeAgo, type Device, type NotificationEntry, type Version } from "@cadenza/core";
import { useServices } from "../app/services.tsx";
import { ErrorText } from "../components/ErrorText.tsx";
import { disablePush, enablePush, forgetDeviceId, pushSupport, savedDeviceId } from "../lib/push.ts";
import { useAction } from "../lib/useAsync.ts";

function Notifications() {
  const { client } = useServices();
  const support = pushSupport();
  const [deviceId, setDeviceId] = useState<string | null>(savedDeviceId());
  const [devices, setDevices] = useState<Device[] | null>(null);
  const [log, setLog] = useState<NotificationEntry[] | null>(null);
  const [tested, setTested] = useState(false);

  // The server's device list is the truth; the saved id only says which one is this browser.
  const loadDevices = useCallback(() => {
    client
      .call("devices")
      .then((r) => {
        setDevices(r.devices);
        const mine = savedDeviceId();
        if (mine && !r.devices.some((d) => d.id === mine)) {
          forgetDeviceId();
          setDeviceId(null);
        }
      })
      .catch(() => setDevices([]));
  }, [client]);
  useEffect(loadDevices, [loadDevices]);

  const loadLog = useCallback(() => {
    client
      .call("notifications")
      .then((r) => setLog(r.items))
      .catch(() => setLog([]));
  }, [client]);
  useEffect(loadLog, [loadLog]);

  const enable = useAction(async () => {
    setDeviceId(await enablePush(client));
    loadDevices();
  });
  const disable = useAction(async () => {
    await disablePush(client);
    setDeviceId(null);
    loadDevices();
  });
  const removeDevice = useAction(async (id: string) => {
    if (id === deviceId) {
      await disablePush(client);
      setDeviceId(null);
    } else {
      await client.call("device-remove", { params: { id } });
    }
    loadDevices();
  });
  const test = useAction(async () => {
    setTested(false);
    await client.call("notification-test");
    setTested(true);
    setTimeout(loadLog, 1500);
  });

  return (
    <section className="panel" aria-labelledby="notif-title">
      <h2 id="notif-title">Notifications</h2>
      <p className="small muted">You get a notification when an agent asks you something, has work ready to look at, or is finished.</p>
      {support === "ios-needs-install" && (
        <p className="notice">On iPhone, first add Cadenza to your Home Screen (Share, then Add to Home Screen), and open it from there. Then turn notifications on.</p>
      )}
      {support === "unsupported" && <p className="notice">This browser can't show notifications from Cadenza.</p>}
      {support === "supported" && (
        <div className="row">
          {deviceId ? (
            <>
              <span className="small">Notifications are on for this browser.</span>
              <span className="spacer" />
              <button onClick={() => void disable.run()} disabled={disable.busy}>
                Turn off
              </button>
            </>
          ) : (
            <button className="primary" onClick={() => void enable.run()} disabled={enable.busy}>
              {enable.busy ? "Turning on…" : "Turn on notifications"}
            </button>
          )}
        </div>
      )}
      <ErrorText error={enable.error ?? disable.error} />
      <h3 style={{ marginTop: 16 }}>Devices that get notifications</h3>
      {devices === null && <p className="muted small">Loading…</p>}
      {devices?.length === 0 && <p className="muted small">No devices yet.</p>}
      {devices && devices.length > 0 && (
        <ul className="list" aria-label="Devices">
          {devices.map((d) => (
            <li key={d.id} className="row">
              <div style={{ flex: 1, minWidth: 0 }}>
                <strong>{d.name || d.provider}</strong>
                {d.id === deviceId && <span className="pill" style={{ marginLeft: 6 }}>This browser</span>}
                <div className="small muted">
                  {d.provider === "webpush" ? "Browser" : d.provider === "expo" ? "Android app" : d.provider} · added {timeAgo(d.created_at)} ·{" "}
                  {d.last_ok_at ? `last delivered ${timeAgo(d.last_ok_at)}` : "nothing delivered yet"}
                </div>
              </div>
              <button onClick={() => void removeDevice.run(d.id)} disabled={removeDevice.busy} aria-label={`Remove ${d.name || d.provider}`}>
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
      <ErrorText error={removeDevice.error} />
      <div className="row" style={{ marginTop: 12 }}>
        <button onClick={() => void test.run()} disabled={test.busy}>
          Send a test notification
        </button>
        {tested && (
          <span className="small" role="status">
            Sent. It should arrive within a few seconds.
          </span>
        )}
      </div>
      <ErrorText error={test.error} />
      <h3 style={{ marginTop: 16 }}>Sent notifications</h3>
      {log === null && <p className="muted small">Loading…</p>}
      {log?.length === 0 && <p className="muted small">None yet.</p>}
      {log && log.length > 0 && (
        <div style={{ overflowX: "auto" }}>
          <table className="log">
            <thead>
              <tr>
                <th scope="col">When</th>
                <th scope="col">Message</th>
                <th scope="col">Where</th>
                <th scope="col">Result</th>
              </tr>
            </thead>
            <tbody>
              {log.map((n) => (
                <tr key={n.id}>
                  <td>{timeAgo(n.created_at)}</td>
                  <td>
                    <strong>{n.title}</strong>
                    <div className="muted">{n.body}</div>
                  </td>
                  <td>
                    {n.device} ({n.provider})
                  </td>
                  <td className={n.status === "failed" ? "failed" : ""}>
                    {n.status === "failed" ? `Failed${n.error ? `: ${n.error}` : ""}` : "Sent"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/** Notifications, account and server info. */
export default function SettingsPage({ onLogout }: { onLogout: () => void }) {
  const { client } = useServices();
  const [version, setVersion] = useState<Version | null>(null);
  useEffect(() => {
    client.call("version").then(setVersion, () => undefined);
  }, [client]);
  return (
    <div className="page-narrow stack">
      <h1>Settings</h1>
      <Notifications />
      <section className="panel" aria-labelledby="account-title">
        <h2 id="account-title">Account</h2>
        <div className="row">
          <span className="small muted">{version ? `Server ${version.server} · API ${version.api}` : ""}</span>
          <span className="spacer" />
          <button onClick={onLogout}>Log out</button>
        </div>
      </section>
    </div>
  );
}
