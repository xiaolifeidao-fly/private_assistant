import { api } from "./api";

interface PushConfig {
  enabled: boolean;
  publicKey: string;
}

function applicationKey(value: string): ArrayBuffer {
  const padding = "=".repeat((4 - (value.length % 4)) % 4);
  const base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const buffer = new ArrayBuffer(raw.length);
  const bytes = new Uint8Array(buffer);
  for (let index = 0; index < raw.length; index += 1) bytes[index] = raw.charCodeAt(index);
  return buffer;
}

export function isStandalone(): boolean {
  return window.matchMedia("(display-mode: standalone)").matches || (navigator as Navigator & { standalone?: boolean }).standalone === true;
}

export async function pushState(): Promise<"unsupported" | "off" | "on" | "denied" | "unconfigured"> {
  if (!("serviceWorker" in navigator) || !("PushManager" in window) || !("Notification" in window)) return "unsupported";
  const config = await api.get<PushConfig>("/push/config");
  if (!config.enabled) return "unconfigured";
  if (Notification.permission === "denied") return "denied";
  const registration = await navigator.serviceWorker.ready;
  return (await registration.pushManager.getSubscription()) ? "on" : "off";
}

export async function enablePush(): Promise<void> {
  const config = await api.get<PushConfig>("/push/config");
  if (!config.enabled || !config.publicKey) throw new Error("服务器还没配置推送密钥");
  const permission = await Notification.requestPermission();
  if (permission !== "granted") throw new Error("没有获得通知权限");
  const registration = await navigator.serviceWorker.ready;
  let subscription = await registration.pushManager.getSubscription();
  subscription ??= await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: applicationKey(config.publicKey) });
  await api.post("/push/subscriptions", subscription.toJSON());
}

export async function disablePush(): Promise<void> {
  const registration = await navigator.serviceWorker.ready;
  const subscription = await registration.pushManager.getSubscription();
  if (!subscription) return;
  await api.post("/push/unsubscribe", { endpoint: subscription.endpoint });
  await subscription.unsubscribe();
}

export async function testPush(): Promise<void> {
  await api.post("/push/test");
}
