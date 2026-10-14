"use client";

import { useEffect, useState } from "react";

/**
 * 新手引导的进度存在本机：欢迎页看过没有、每个页面的小提示关掉没有。
 * 换手机或清了数据会重新出现一次，不影响使用。
 */
const WELCOME_KEY = "pa_guide_welcome_v1";
const TIP_PREFIX = "pa_guide_tip_";
const RESET_EVENT = "pa:guide-reset";

function read(key: string): boolean {
  try {
    return localStorage.getItem(key) === "1";
  } catch {
    return true;
  }
}

function write(key: string) {
  try {
    localStorage.setItem(key, "1");
  } catch {
    /* 隐私模式下不可用 */
  }
}

export function welcomeSeen(): boolean {
  return read(WELCOME_KEY);
}

export function markWelcomeSeen() {
  write(WELCOME_KEY);
}

/** 「使用指南」里点「重新看一遍」：欢迎页和所有小提示都恢复。 */
export function resetGuide() {
  try {
    Object.keys(localStorage)
      .filter((key) => key === WELCOME_KEY || key.startsWith(TIP_PREFIX))
      .forEach((key) => localStorage.removeItem(key));
  } catch {
    /* ignore */
  }
  window.dispatchEvent(new Event(RESET_EVENT));
}

export function onGuideReset(listener: () => void): () => void {
  window.addEventListener(RESET_EVENT, listener);
  return () => window.removeEventListener(RESET_EVENT, listener);
}

/** 某条小提示是否还要显示；第一次渲染时先当作不显示，避免闪一下。 */
export function useTip(id: string): [boolean, () => void] {
  const [show, setShow] = useState(false);
  useEffect(() => {
    const sync = () => setShow(!read(TIP_PREFIX + id));
    sync();
    return onGuideReset(sync);
  }, [id]);
  return [
    show,
    () => {
      write(TIP_PREFIX + id);
      setShow(false);
    },
  ];
}
