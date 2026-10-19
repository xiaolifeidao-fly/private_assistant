"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Dialog, Popup, SwipeAction, Toast } from "antd-mobile";
import dayjs from "dayjs";
import { House, MessagesSquare, Search, X } from "lucide-react";
import { api } from "@/lib/api";
import { botTone } from "@/lib/tone";
import { shortTime } from "@/lib/time";
import type { Conversation, Paged } from "@/lib/types";
import { Avatar, Empty } from "@/components/ui";

interface BotSummary {
  botKey: string;
  name: string;
  avatar: string;
}

/** 按最后说话的时间分组：今天 / 昨天 / 近 7 天 / 更早。 */
function groupOf(conversation: Conversation): string {
  const days = dayjs().startOf("day").diff(dayjs(conversation.lastMessageAt ?? conversation.createdAt).startOf("day"), "day");
  if (days <= 0) return "今天";
  if (days === 1) return "昨天";
  if (days < 7) return "近 7 天";
  return "更早";
}

/**
 * 从左边滑出来的侧边栏，列出最近的对话，点一下切过去。
 * 每次打开都重新拉一次列表，保证刚聊过的排在最上面。
 */
export function ConversationDrawer({ open, onClose, activeKey, showHome, onDeleted }: {
  open: boolean;
  onClose: () => void;
  /** 当前所在的对话，高亮显示 */
  activeKey?: string;
  /** 在对话页里打开时，顶部给一个回首页的入口 */
  showHome?: boolean;
  onDeleted?: (conversationKey: string) => void;
}) {
  const router = useRouter();
  const [bots, setBots] = useState<BotSummary[]>([]);
  const [conversations, setConversations] = useState<Conversation[] | null>(null);
  const [keyword, setKeyword] = useState("");

  useEffect(() => {
    if (!open) return;
    setKeyword("");
    Promise.all([api.get<BotSummary[]>("/bots"), api.get<Paged<Conversation>>("/conversations?size=100")])
      .then(([botList, page]) => {
        setBots(botList);
        setConversations(page.items);
      })
      .catch((error) => {
        setConversations((list) => list ?? []);
        Toast.show({ content: (error as Error).message });
      });
  }, [open]);

  const botOf = (key: string) => bots.find((b) => b.botKey === key);

  const groups = useMemo(() => {
    const word = keyword.trim().toLowerCase();
    const list = (conversations ?? []).filter((c) => {
      if (!word) return true;
      const name = bots.find((b) => b.botKey === c.botKey)?.name ?? "";
      return c.title.toLowerCase().includes(word) || name.toLowerCase().includes(word);
    });
    const result: { label: string; items: Conversation[] }[] = [];
    for (const conversation of list) {
      const label = groupOf(conversation);
      const last = result[result.length - 1];
      if (last?.label === label) last.items.push(conversation);
      else result.push({ label, items: [conversation] });
    }
    return result;
  }, [conversations, bots, keyword]);

  const go = (path: string) => {
    onClose();
    router.push(path);
  };

  const remove = async (conversation: Conversation) => {
    const confirmed = await Dialog.confirm({ title: "删除这段对话？", content: "从这段对话里记下的记忆不受影响。", confirmText: "删除", cancelText: "取消" });
    if (!confirmed) return;
    try {
      await api.del(`/conversations/${conversation.conversationKey}`);
      setConversations((list) => (list ?? []).filter((c) => c.conversationKey !== conversation.conversationKey));
      onDeleted?.(conversation.conversationKey);
      if (conversation.conversationKey === activeKey) go("/");
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    }
  };

  return (
    <Popup visible={open} position="left" onMaskClick={onClose} onClose={onClose} bodyClassName="pa-drawer">
      <div className="pa-drawer-head">
        <div className="pa-drawer-title">最近的对话</div>
        <button className="pa-sheet-close" onClick={onClose} aria-label="关闭"><X size={16} strokeWidth={2.4} /></button>
      </div>

      <label className="pa-drawer-search">
        <Search size={16} strokeWidth={2} />
        <input value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="搜对话标题或助手" />
      </label>

      {showHome && (
        <button className="pa-drawer-home" onClick={() => go("/")}>
          <House size={18} strokeWidth={2} />
          <span>回到首页</span>
        </button>
      )}

      <div className="pa-drawer-list">
        {conversations === null ? (
          <div className="pa-hint" style={{ padding: "24px 16px", textAlign: "center" }}>加载中…</div>
        ) : groups.length === 0 ? (
          <Empty icon={<MessagesSquare size={22} />} title={keyword ? "没找到" : "还没有对话"} hint={keyword ? "换个词试试" : "回首页点任意一个入口开始聊"} compact />
        ) : (
          groups.map((group) => (
            <div key={group.label} className="pa-drawer-group">
              <div className="pa-drawer-group-label">{group.label}</div>
              {group.items.map((conversation) => {
                const bot = botOf(conversation.botKey);
                const active = conversation.conversationKey === activeKey;
                return (
                  <SwipeAction
                    key={conversation.conversationKey}
                    rightActions={[{ key: "delete", text: "删除", color: "danger", onClick: () => remove(conversation) }]}
                  >
                    <button
                      className="pa-drawer-item"
                      data-active={active}
                      aria-current={active ? "page" : undefined}
                      onClick={() => (active ? onClose() : go(`/chat/${conversation.conversationKey}`))}
                    >
                      <Avatar tone={botTone(conversation.botKey)} size={34}>{bot?.avatar ?? "💬"}</Avatar>
                      <span className="pa-drawer-item-text">
                        <b>{conversation.title}</b>
                        <small>{bot?.name ?? ""}{bot ? " · " : ""}{shortTime(conversation.lastMessageAt ?? conversation.createdAt)}</small>
                      </span>
                    </button>
                  </SwipeAction>
                );
              })}
            </div>
          ))
        )}
        {groups.length > 0 && <div className="pa-hint" style={{ padding: "12px 16px 4px", textAlign: "center" }}>向左滑可以删除</div>}
      </div>
    </Popup>
  );
}
