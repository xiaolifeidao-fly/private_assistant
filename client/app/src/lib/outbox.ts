/**
 * 速记的离线暂存：断网时先存在本机，联网后自动补交。
 * 只暂存纯文字的速记（截图太大，不适合放 localStorage）。
 */
import { api, ApiError } from "./api";

const KEY = "pa_outbox";

interface OutboxItem {
  id: string;
  path: string;
  body: unknown;
  createdAt: string;
  /** 被服务端拒绝的次数；数据库抖动也会被拒，所以拒绝几次才丢弃。 */
  rejected?: number;
}

const MAX_REJECTED = 5;

function read(): OutboxItem[] {
  try {
    return JSON.parse(localStorage.getItem(KEY) || "[]") as OutboxItem[];
  } catch {
    return [];
  }
}

function write(items: OutboxItem[]) {
  try {
    localStorage.setItem(KEY, JSON.stringify(items));
  } catch {
    /* 存不下就算了 */
  }
}

export function enqueue(path: string, body: unknown) {
  write([...read(), { id: `${Date.now()}-${Math.random()}`, path, body, createdAt: new Date().toISOString() }]);
}

export function outboxSize(): number {
  return read().length;
}

let flushing = false;

/**
 * 逐条补交，成功的移除。断网、登录失效、服务端不可用就停下等下次；
 * 被服务端拒绝的留着重试，连续拒绝多次才丢弃。返回补交成功的条数。
 */
export async function flushOutbox(): Promise<number> {
  if (flushing || typeof window === "undefined") return 0;
  flushing = true;
  let sent = 0;
  try {
    for (const item of read()) {
      try {
        await api.post(item.path, item.body);
        write(read().filter((x) => x.id !== item.id));
        sent++;
      } catch (error) {
        if (!(error instanceof ApiError) || error.code === 0 || error.code === 401 || error.code >= 500) break;
        const rejected = (item.rejected ?? 0) + 1;
        write(rejected >= MAX_REJECTED ? read().filter((x) => x.id !== item.id) : read().map((x) => (x.id === item.id ? { ...x, rejected } : x)));
      }
    }
  } finally {
    flushing = false;
  }
  return sent;
}

export function isNetworkError(error: unknown): boolean {
  return error instanceof ApiError && error.code === 0;
}
