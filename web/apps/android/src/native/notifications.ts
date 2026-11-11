import Constants from "expo-constants";
import * as Device from "expo-device";
import * as Notifications from "expo-notifications";
import type { Client } from "@cadenza/core";
import type { SessionStore } from "../lib/session";

Notifications.setNotificationHandler({
  handleNotification: async () => ({ shouldShowBanner: true, shouldShowList: true, shouldPlaySound: true, shouldSetBadge: false }),
});

/** The EAS project id this build was made with, or "" when the self-hoster hasn't set one. */
export function projectId(): string {
  const extra = Constants.expoConfig?.extra as { eas?: { projectId?: string } } | undefined;
  return extra?.eas?.projectId ?? Constants.easConfig?.projectId ?? "";
}

/** Asks for permission, gets this phone's Expo push token and registers it with the server. */
export async function enableNotifications(client: Client, session: SessionStore): Promise<void> {
  const id = projectId();
  if (!id) {
    throw new Error(
      "This build has no EAS project id, so it can't get notifications. Whoever built the app must set expo.extra.eas.projectId in app.json and rebuild (see the app's README).",
    );
  }
  await Notifications.setNotificationChannelAsync("default", { name: "Cadenza", importance: Notifications.AndroidImportance.HIGH });
  let perm = await Notifications.getPermissionsAsync();
  if (!perm.granted) perm = await Notifications.requestPermissionsAsync();
  if (!perm.granted) throw new Error("Notifications are off for Cadenza. Turn them on in Android settings.");
  const token = await Notifications.getExpoPushTokenAsync({ projectId: id });
  const out = await client.call("device-register", {
    body: { provider: "expo", token: token.data, name: Device.modelName ?? "Android phone" },
  });
  await session.setDeviceId(out.device_id);
}

/** Stops notifications to this phone. */
export async function disableNotifications(client: Client, session: SessionStore): Promise<void> {
  const id = await session.deviceId();
  if (id) await client.call("device-remove", { params: { id } }).catch(() => undefined);
  await session.clearDeviceId();
}

/** Calls onThread when the owner taps a notification, including the one that opened the app. */
export function onNotificationTap(onData: (data: unknown) => void): () => void {
  const last = Notifications.getLastNotificationResponse();
  if (last) onData(last.notification.request.content.data);
  const sub = Notifications.addNotificationResponseReceivedListener((r) => onData(r.notification.request.content.data));
  return () => sub.remove();
}
