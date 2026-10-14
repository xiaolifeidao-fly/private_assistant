/**
 * 管理端接口客户端。后端统一信封：{ success, code, data, error }，失败也是 HTTP 200；
 * code=401 表示令牌失效，清掉令牌回登录页。
 */
import { basePath, withBase } from "./base-path";

const TOKEN_KEY = "pa_manager_token";

export function getToken(): string {
  if (typeof window === "undefined") return "";
  try {
    return localStorage.getItem(TOKEN_KEY) || "";
  } catch {
    return "";
  }
}

export function setToken(token: string) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* 隐私模式下 localStorage 可能不可用 */
  }
}

export class ApiError extends Error {
  code: number;
  constructor(message: string, code: number) {
    super(message);
    this.code = code;
  }
}

function toLogin() {
  setToken("");
  if (typeof window !== "undefined" && !window.location.pathname.startsWith(withBase("/login"))) {
    window.location.href = withBase("/login");
  }
}

async function send(path: string, init: RequestInit): Promise<Response> {
  try {
    return await fetch(`${basePath}/api${path}`, init);
  } catch {
    throw new ApiError("连不上服务，请检查网络或后端是否启动", 0);
  }
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const response = await send(path, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${getToken()}`,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let payload: { success: boolean; code: number; data: T; error: string | null };
  try {
    payload = await response.json();
  } catch {
    throw new ApiError(`服务暂不可用（HTTP ${response.status}）`, response.status);
  }
  if (payload.code === 401) {
    toLogin();
    throw new ApiError("登录已失效", 401);
  }
  if (!payload.success) {
    throw new ApiError(payload.error || "请求失败", payload.code);
  }
  return payload.data;
}

export const api = {
  get: <T,>(path: string) => request<T>("GET", path),
  post: <T,>(path: string, body?: unknown) => request<T>("POST", path, body ?? {}),
  put: <T,>(path: string, body?: unknown) => request<T>("PUT", path, body ?? {}),
  del: <T,>(path: string) => request<T>("DELETE", path),
};

export function query(params: Record<string, string | number | undefined | null>): string {
  const search = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") search.set(key, String(value));
  });
  const text = search.toString();
  return text ? `?${text}` : "";
}

/** multipart 上传（原始记录 + 截图）。 */
export async function postForm<T>(path: string, form: FormData): Promise<T> {
  const response = await send(path, { method: "POST", headers: { Authorization: `Bearer ${getToken()}` }, body: form });
  const payload = await response.json().catch(() => null);
  if (!payload) throw new ApiError(`上传失败（HTTP ${response.status}）`, response.status);
  if (payload.code === 401) {
    toLogin();
    throw new ApiError("登录已失效", 401);
  }
  if (!payload.success) throw new ApiError(payload.error || "上传失败", payload.code);
  return payload.data as T;
}

/**
 * POST 一个 SSE 接口并逐个回调事件（每个事件一行 data: {json}）。
 * 后端出错时如果还没开始推流，会返回普通的 JSON 信封，这里转成异常。
 */
export async function streamPost<E>(path: string, body: unknown, onEvent: (event: E) => void, signal?: AbortSignal) {
  let response: Response;
  try {
    response = await fetch(`${basePath}/api${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${getToken()}` },
      body: JSON.stringify(body),
      signal,
    });
  } catch (error) {
    if ((error as Error).name === "AbortError") throw error;
    throw new ApiError("连不上服务，请检查网络或后端是否启动", 0);
  }
  if (!(response.headers.get("Content-Type") || "").includes("text/event-stream")) {
    const payload = await response.json().catch(() => null);
    if (payload?.code === 401) toLogin();
    throw new ApiError(payload?.error || `请求失败（HTTP ${response.status}）`, payload?.code ?? response.status);
  }
  const reader = response.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let index: number;
    while ((index = buffer.indexOf("\n\n")) >= 0) {
      const chunk = buffer.slice(0, index);
      buffer = buffer.slice(index + 2);
      for (const line of chunk.split("\n")) {
        if (!line.startsWith("data:")) continue;
        try {
          onEvent(JSON.parse(line.slice(5).trim()) as E);
        } catch {
          /* 半截数据，忽略 */
        }
      }
    }
  }
}

/** 带令牌读取附件，返回 object URL。 */
export async function fetchObjectURL(path: string): Promise<string> {
  const response = await send(path, { headers: { Authorization: `Bearer ${getToken()}` } });
  if (!response.ok) throw new ApiError("图片读取失败", response.status);
  return URL.createObjectURL(await response.blob());
}
