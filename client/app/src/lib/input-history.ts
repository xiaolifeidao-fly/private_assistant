import { useRef } from "react";
import type { KeyboardEvent } from "react";

// 发过的话存在本机，各个对话框共用；上下键翻出来改了再发
const STORAGE_KEY = "pa_input_history";
const LIMIT = 10;

function load(): string[] {
  try {
    const list = JSON.parse(localStorage.getItem(STORAGE_KEY) || "[]");
    return Array.isArray(list) ? list.filter((item) => typeof item === "string") : [];
  } catch {
    return [];
  }
}

/** 记下一句发出去的话：最新的在最前，重复的挪到最前，只留 10 条。 */
export function rememberInput(text: string) {
  const value = text.trim();
  if (!value) return;
  try {
    const list = [value, ...load().filter((item) => item !== value)].slice(0, LIMIT);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
  } catch {
    /* ignore */
  }
}

/**
 * 输入框里按 ↑ 翻更早的、↓ 翻回来，翻到底恢复正在写的那段。
 * 光标在第一行按 ↑ / 最后一行按 ↓ 才翻，不抢多行文字里的上下移动；翻出来以后没改过就一直能接着翻。
 * onKeyDown 返回 true 表示这次按键已经处理了。
 */
export function useInputHistory(value: string, setValue: (value: string) => void) {
  const index = useRef(-1); // -1 = 没在翻
  const stash = useRef("");
  const shown = useRef("");

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return false;
    if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.nativeEvent.isComposing) return false;
    const element = event.currentTarget;
    if (index.current >= 0 && value !== shown.current) index.current = -1; // 翻出来又改了，就当新写的
    const browsing = index.current >= 0;
    const { selectionStart, selectionEnd } = element;
    if (!browsing && selectionStart !== selectionEnd) return false;

    let next: string;
    if (event.key === "ArrowUp") {
      if (!browsing && value.slice(0, selectionStart).includes("\n")) return false;
      const list = load();
      if (index.current + 1 >= list.length) return false;
      if (!browsing) stash.current = value;
      index.current += 1;
      next = list[index.current];
    } else {
      if (!browsing) return false;
      index.current -= 1;
      next = index.current < 0 ? stash.current : load()[index.current] ?? stash.current;
    }
    event.preventDefault();
    shown.current = next;
    setValue(next);
    requestAnimationFrame(() => element.setSelectionRange(next.length, next.length));
    return true;
  };

  /** 发出去时调用：记进历史，翻页状态归零。 */
  const remember = (text: string) => {
    rememberInput(text);
    index.current = -1;
    stash.current = "";
  };

  return { onKeyDown, remember };
}
