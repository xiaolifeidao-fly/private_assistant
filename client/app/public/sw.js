// 最小 Service Worker：静态资源走「缓存优先、后台更新」，页面走「网络优先、断网回退缓存」。
// 接口（/api）一律不缓存——对话和记忆必须是最新的。
const CACHE = "pa-app-v5";
// 访问前缀（next.config.mjs 的 basePath），从注册时的 scope 推出来，例如 "/assistant"；挂在根路径时是 ""。
const BASE = new URL(self.registration.scope).pathname.replace(/\/$/, "");
// 后端给的跳转地址（如 "/today"）是不带前缀的应用内路径
const withBase = (path) => (path.startsWith("/") && !path.startsWith(`${BASE}/`) ? `${BASE}${path}` : path);
// 每次部署静态资源的文件名都会变，旧的留在缓存里只会越攒越多，超过这个数就删最早的。
const MAX_ENTRIES = 160;

async function trim() {
  const cache = await caches.open(CACHE);
  const keys = await cache.keys();
  await Promise.all(keys.slice(0, Math.max(0, keys.length - MAX_ENTRIES)).map((key) => cache.delete(key)));
}

self.addEventListener("install", (event) => {
  self.skipWaiting();
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll([`${BASE}/`, `${BASE}/manifest.webmanifest`, `${BASE}/icons/icon-192.png`])).catch(() => {}));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key)))).then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  const url = new URL(request.url);
  if (request.method !== "GET" || url.origin !== self.location.origin || url.pathname.startsWith(`${BASE}/api/`)) return;

  if (url.pathname.startsWith(`${BASE}/_next/static/`) || url.pathname.startsWith(`${BASE}/icons/`)) {
    event.respondWith(
      caches.match(request).then((cached) => {
        const network = fetch(request).then((response) => {
          if (response.ok) {
            const copy = response.clone();
            caches.open(CACHE).then((cache) => cache.put(request, copy)).then(trim);
          }
          return response;
        });
        return cached || network;
      }),
    );
    return;
  }

  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request)
        .then((response) => {
          if (response.ok) {
            const copy = response.clone();
            caches.open(CACHE).then((cache) => cache.put(request, copy));
          }
          return response;
        })
        .catch(() => caches.match(request).then((cached) => cached || caches.match(`${BASE}/`))),
    );
  }
});

self.addEventListener("push", (event) => {
  let payload = {};
  try {
    payload = event.data ? event.data.json() : {};
  } catch {
    payload = { title: "家庭助理", body: event.data ? event.data.text() : "有一条新提醒" };
  }
  event.waitUntil(
    self.registration.showNotification(payload.title || "家庭助理", {
      body: payload.body || "有一条新提醒",
      icon: `${BASE}/icons/icon-192.png`,
      badge: `${BASE}/icons/icon-192.png`,
      tag: payload.tag || "pa-reminder",
      data: { url: payload.url || "/today" },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const target = new URL(withBase(event.notification.data?.url || "/today"), self.location.origin).href;
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if ("focus" in client) {
          if ("navigate" in client) client.navigate(target);
          return client.focus();
        }
      }
      return self.clients.openWindow(target);
    }),
  );
});
