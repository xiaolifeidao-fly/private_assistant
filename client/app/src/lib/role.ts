/**
 * 家里每个人用的界面不一样（账号的 role，服务端 identity/role.go 同步限制机器人）：
 *  - self（admin / member）：我自己，全部功能；
 *  - spouse：老婆，对话里只有万能通和提醒，没有「家人」页（人设卡、分身不对她开放，见设计 01 · 8.1）；
 *  - elder：奶奶，进长辈模式 /elder：大字、语音优先，只有万事通、记一下、提醒三件事。
 *
 * 当前账号缓存在本机，打开 App 时先按缓存直接进对应界面，再后台刷新一次。
 */
import { useEffect, useState } from "react";
import { api, getToken, setToken } from "./api";
import type { Principal } from "./types";

export type AppRole = "self" | "spouse" | "elder";

const ME_KEY = "pa_me";

export function roleOf(me: Pick<Principal, "role"> | null | undefined): AppRole {
  if (me?.role === "elder") return "elder";
  if (me?.role === "spouse") return "spouse";
  return "self";
}

/** 每个角色能看到的底部标签页。 */
export const roleTabs: Record<AppRole, string[]> = {
  self: ["/", "/note", "/today", "/family"],
  spouse: ["/", "/note", "/today"],
  elder: [],
};

/** 角色对应的首页。 */
export function homeOf(role: AppRole): string {
  return role === "elder" ? "/elder" : "/";
}

export function cachedMe(): Principal | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = localStorage.getItem(ME_KEY);
    return raw ? (JSON.parse(raw) as Principal) : null;
  } catch {
    return null;
  }
}

function saveMe(me: Principal | null) {
  try {
    if (me) localStorage.setItem(ME_KEY, JSON.stringify(me));
    else localStorage.removeItem(ME_KEY);
  } catch {
    /* 隐私模式下不可用 */
  }
}

let inflight: Promise<Principal> | null = null;

/** 拉一次当前账号并写进缓存；同一时间只发一个请求。 */
export function fetchMe(): Promise<Principal> {
  inflight ??= api
    .get<Principal>("/auth/me")
    .then((me) => {
      saveMe(me);
      return me;
    })
    .finally(() => {
      inflight = null;
    });
  return inflight;
}

/** 退出登录（令牌和角色缓存一起清掉），调用方再跳登录页。 */
export async function logout() {
  await api.post("/auth/logout").catch(() => {});
  setToken("");
}

/**
 * 当前账号：有缓存先用缓存（不闪屏），同时后台刷新；没缓存时 me 为 null，等接口回来。
 * 网络不通时保留缓存。
 */
export function useMe(): { me: Principal | null; role: AppRole; ready: boolean } {
  const [me, setMe] = useState<Principal | null>(null);
  const [ready, setReady] = useState(false);
  useEffect(() => {
    const cached = cachedMe();
    if (cached) {
      setMe(cached);
      setReady(true);
    }
    if (!getToken()) return;
    fetchMe()
      .then(setMe)
      .catch(() => {})
      .finally(() => setReady(true));
  }, []);
  return { me, role: roleOf(me), ready };
}
