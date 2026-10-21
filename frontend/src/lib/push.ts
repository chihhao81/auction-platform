// Browser side of Web Push. The browser permission prompt is only ever shown
// as a direct result of the user clicking "enable" (never automatically).
import { api } from "./api";

export type PushState = "unsupported" | "server-disabled" | "denied" | "enabled" | "disabled";

const SW_URL = "/sw.js";

export function pushSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

// Converts a base64url VAPID public key into the byte array PushManager wants.
export function urlBase64ToUint8Array(base64url: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (base64url.length % 4)) % 4);
  const base64 = (base64url + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

async function currentSubscription(): Promise<PushSubscription | null> {
  const reg = await navigator.serviceWorker.getRegistration(SW_URL);
  return reg ? reg.pushManager.getSubscription() : null;
}

export async function getPushState(): Promise<PushState> {
  if (!pushSupported()) return "unsupported";
  const { enabled } = await api.get<{ enabled: boolean }>("/api/push/public-key");
  if (!enabled) return "server-disabled";
  if (Notification.permission === "denied") return "denied";
  return (await currentSubscription()) ? "enabled" : "disabled";
}

export async function enablePush(): Promise<PushState> {
  if (!pushSupported()) return "unsupported";
  const permission = await Notification.requestPermission();
  if (permission !== "granted") return permission === "denied" ? "denied" : "disabled";
  const { public_key, enabled } = await api.get<{ public_key: string; enabled: boolean }>("/api/push/public-key");
  if (!enabled) return "server-disabled";
  const reg = await navigator.serviceWorker.register(SW_URL);
  await navigator.serviceWorker.ready;
  const sub =
    (await reg.pushManager.getSubscription()) ??
    (await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: urlBase64ToUint8Array(public_key) }));
  const json = sub.toJSON();
  await api.post("/api/push/subscriptions", { endpoint: json.endpoint, keys: json.keys });
  return "enabled";
}

// Removes this browser's subscription from the server and the browser.
export async function disablePush(): Promise<void> {
  if (!pushSupported()) return;
  const sub = await currentSubscription();
  if (!sub) return;
  try {
    await api.del("/api/push/subscriptions", { endpoint: sub.endpoint });
  } finally {
    await sub.unsubscribe();
  }
}
