import type { KeyboardEvent } from "react";

/** ⌘+回车（Windows 上 Ctrl+回车）= 发送；输入法还在选字时不算。 */
export function isSendKey(event: KeyboardEvent) {
  return event.key === "Enter" && (event.metaKey || event.ctrlKey) && !event.nativeEvent.isComposing;
}
