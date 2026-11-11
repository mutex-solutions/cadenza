import { useCallback, useEffect, useState } from "react";
import { timeAgo, type NotificationEntry, type Version } from "@cadenza/core";
import { useServices } from "../app/services.tsx";
import { ErrorText } from "../components/ErrorText.tsx";
import { disablePush, enablePush, pushSupport, savedDeviceId } from "../lib/push.ts";
import { useAction } from "../lib/useAsync.ts";

function Notifications() {
  const { client } = useServices();
  const support = pushSupport();
  const [deviceId, setDeviceId] = useState<string | null>(savedDeviceId());
  const [log, setLog] = useState<NotificationEntry[] | null>(null);
  const [tested, setTested] = useState(false);

  const loadLog = useCallback(() => {
    client
      .call("notifications")
      .then((r) => setLog(r.items))
      .catch(() => setLog([]));
  }, [client]);
  useEffect(loadLog, [loadLog]);

  const enable = useAction(async () => setDeviceId(await enablePush(client)));
  const disable = useAction(async () => {
    await disablePush(client);
    setDeviceId(null);
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
