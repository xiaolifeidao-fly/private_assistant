/**
 * App 接口客户端。统一信封 { success, code, data, error }；code=401 回登录页。
 * 主屏 PWA 的 localStorage 和 Safari 是隔离的，令牌存这里只在 PWA 里有效。
 */
import { basePath, withBase } from "./base-path";

const TOKEN_KEY = "pa_app_token";

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
    else {
      localStorage.removeItem(TOKEN_KEY);
      // 换账号登录时别沿用上一个人的角色（见 role.ts）
      localStorage.removeItem("pa_me");
    }
  } catch {
    /* 隐私模式下不可用 */
  }
}

/** 业务错误带上后端的 code：0 表示网络不通，401 表示登录失效。 */
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
    window.location.replace(withBase("/login"));
  }
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${basePath}/api${path}`, {
      method,
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${getToken()}` },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError("网络不可用，请稍后再试", 0);
  }
  let payload: { success: boolean; code: number; data: T; error: string | null };
  try {
    payload = await response.json();
  } catch {
    throw new ApiError(`服务暂不可用（${response.status}）`, response.status);
  }
  if (payload.code === 401) {
    toLogin();
    throw new ApiError("登录已失效", 401);
  }
  if (!payload.success) throw new ApiError(payload.error || "请求失败", payload.code);
  return payload.data;
}

export const api = {
  get: <T,>(path: string) => request<T>("GET", path),
  post: <T,>(path: string, body?: unknown) => request<T>("POST", path, body ?? {}),
  put: <T,>(path: string, body?: unknown) => request<T>("PUT", path, body ?? {}),
  del: <T,>(path: string) => request<T>("DELETE", path),
};

/** multipart 上传（原始记录 + 截图）。 */
export async function postForm<T>(path: string, form: FormData): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${basePath}/api${path}`, { method: "POST", headers: { Authorization: `Bearer ${getToken()}` }, body: form });
  } catch {
    throw new ApiError("网络不可用，请稍后再试", 0);
  }
  const payload = await response.json().catch(() => null);
  if (!payload) throw new ApiError(`上传失败（${response.status}）`, response.status);
  if (payload.code === 401) {
    toLogin();
    throw new ApiError("登录已失效", 401);
  }
  if (!payload.success) throw new ApiError(payload.error || "上传失败", payload.code);
  return payload.data as T;
}

/** 带令牌读取二进制（附件图片），返回可放进 <img> 的 object URL。 */
export async function fetchObjectURL(path: string): Promise<string> {
  const response = await fetch(`${basePath}/api${path}`, { headers: { Authorization: `Bearer ${getToken()}` } });
  if (!response.ok) throw new Error("图片读取失败");
  return URL.createObjectURL(await response.blob());
}

/**
 * 发消息并逐个回调 SSE 事件。后端在请求断开后仍会把回答写完，
 * 所以这里出错时调用方应重新拉一次消息列表，而不是认为回答丢了。
 */
export async function streamMessage<E>(conversationKey: string, body: { text: string; noteKey?: string }, onEvent: (event: E) => void, signal?: AbortSignal) {
  let response: Response;
  try {
    response = await fetch(`${basePath}/api/conversations/${conversationKey}/messages`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${getToken()}` },
      body: JSON.stringify(body),
      signal,
    });
  } catch {
    throw new ApiError("网络不可用，请稍后再试", 0);
  }
  const type = response.headers.get("Content-Type") || "";
  if (!type.includes("text/event-stream")) {
    const payload = await response.json().catch(() => null);
    if (payload?.code === 401) toLogin();
    throw new ApiError(payload?.error || `发送失败（${response.status}）`, payload?.code ?? response.status);
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
        if (line.startsWith("data:")) {
          try {
            onEvent(JSON.parse(line.slice(5).trim()) as E);
          } catch {
            /* 半截数据，忽略 */
          }
        }
      }
    }
  }
}
