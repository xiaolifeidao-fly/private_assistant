"use client";

import { useEffect, useRef } from "react";

/**
 * 主屏 App 切到后台再回来时页面不会重新加载，数据会停在上次打开的样子。
 * 离开超过 minHiddenMs 再回来就调用一次 callback。
 */
export function useOnResume(callback: () => void, minHiddenMs = 30_000) {
  const ref = useRef(callback);
  ref.current = callback;
  useEffect(() => {
    let hiddenAt = 0;
    const onChange = () => {
      if (document.visibilityState === "hidden") hiddenAt = Date.now();
      else if (hiddenAt && Date.now() - hiddenAt >= minHiddenMs) ref.current();
    };
    document.addEventListener("visibilitychange", onChange);
    return () => document.removeEventListener("visibilitychange", onChange);
  }, [minHiddenMs]);
}

const PENDING_EVENT = "pa:pending-changed";

/** 确认 / 驳回记忆后通知底部标签栏刷新「今天」上的角标。 */
export function notifyPendingChanged() {
  window.dispatchEvent(new Event(PENDING_EVENT));
}

export function onPendingChanged(listener: () => void): () => void {
  window.addEventListener(PENDING_EVENT, listener);
  return () => window.removeEventListener(PENDING_EVENT, listener);
}
