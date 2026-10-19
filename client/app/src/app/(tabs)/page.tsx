"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Dialog, PullToRefresh, Toast } from "antd-mobile";
import { AlarmClock, ArrowRight, LogOut, Luggage, MessageSquareQuote, NotebookPen, PanelLeft, Search, Utensils, type LucideIcon } from "lucide-react";
import { api } from "@/lib/api";
import { logout, useMe } from "@/lib/role";
import { useOnResume } from "@/lib/events";
import { botTone, type Tone } from "@/lib/tone";
import { greeting, todayLabel } from "@/lib/time";
import type { Conversation, Paged } from "@/lib/types";
import { ConversationDrawer } from "@/components/conversation-drawer";
import { Avatar, PageHeader, Section, Skeleton, Tip } from "@/components/ui";

interface BotSummary {
  botKey: string;
  name: string;
  kind: string;
  avatar: string;
  description: string;
}

/** 首页的快捷入口：点一下就开一段对话，并把开头替你写好，改一改就能发。 */
const shortcuts: { key: string; icon: LucideIcon; label: string; hint: string; tone: Tone; bot: string; prompt: string; href?: string }[] = [
  { key: "note", icon: NotebookPen, label: "记一件事", hint: "原话、承诺、小事", tone: "clay", bot: "assistant", prompt: "记一下：" },
  { key: "find", icon: Search, label: "找东西", hint: "证件钥匙放哪了", tone: "sage", bot: "recall", prompt: "" },
  { key: "say", icon: MessageSquareQuote, label: "这话怎么说", hint: "想清楚再开口", tone: "rose", bot: "assistant", prompt: "这件事我想好好说，帮我想想怎么开口：" },
  { key: "trip", icon: Luggage, label: "回老家", hint: "见谁、注意什么", tone: "sky", bot: "assistant", prompt: "下次回老家要注意什么？" },
  { key: "remind", icon: AlarmClock, label: "提醒", hint: "说一句，到点推送到手机", tone: "plum", bot: "remind", prompt: "提醒我：" },
  // 不是对话：打开饮食页，计划和实际对照着看，两边都能说一句 / 拍张照
  { key: "diet", icon: Utensils, label: "宝宝吃饭", hint: "排计划、看实际吃了啥", tone: "amber", bot: "", prompt: "", href: "/diet" },
];

export default function ChatsPage() {
  const router = useRouter();
  const { role } = useMe();
  const [bots, setBots] = useState<BotSummary[] | null>(null);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [starting, setStarting] = useState("");
  const [drawer, setDrawer] = useState(false);

  const load = useCallback(async () => {
    try {
      const [botList, page] = await Promise.all([
        api.get<BotSummary[]>("/bots"),
        api.get<Paged<Conversation>>("/conversations?size=50"),
      ]);
      setBots(botList);
      setConversations(page.items);
    } catch (error) {
      setBots((value) => value ?? []);
      Toast.show({ content: (error as Error).message });
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);
  useOnResume(() => void load());

  /** prompt 会填进对话的输入框（不会自动发出去）。 */
  const start = async (botKey: string, prompt = "", busyKey = botKey) => {
    if (starting) return;
    const query = prompt ? `?q=${encodeURIComponent(prompt)}` : "";
    // 上次点开没说话的空对话直接接着用，不再堆一串「新对话」
    const blank = conversations.find((c) => c.botKey === botKey && c.messageCount === 0);
    if (blank) {
      router.push(`/chat/${blank.conversationKey}${query}`);
      return;
    }
    setStarting(busyKey);
    try {
      const conversation = await api.post<Conversation>("/conversations", { botKey });
      router.push(`/chat/${conversation.conversationKey}${query}`);
    } catch (error) {
      Toast.show({ content: (error as Error).message });
      setStarting("");
    }
  };

  const assistant = bots?.find((b) => b.botKey === "assistant") ?? bots?.find((b) => b.kind === "assistant");
  // 其它助理类机器人（万能通……）排在主助理下面，一行一张
  const helpers = bots?.filter((b) => b.kind === "assistant" && b !== assistant) ?? [];
  const personas = bots?.filter((b) => b.kind !== "assistant") ?? [];
  const botOf = (key: string) => bots?.find((b) => b.botKey === key);
  // 快捷入口指向的机器人没开时，交给私人助理
  const shortcutBot = (key: string) => (botOf(key) ? key : assistant?.botKey ?? "");
  // 老婆的账号没有私人助理：只留指向她能用的机器人的入口（「记一件事」交给万能通）
  const visibleShortcuts = role === "self" ? shortcuts : shortcuts.filter((item) => item.key === "note" || item.href || botOf(item.bot));

  const signOut = async () => {
    const confirmed = await Dialog.confirm({ title: "退出登录？", content: "退出后需要重新输入账号密码。", confirmText: "退出", cancelText: "取消" });
    if (!confirmed) return;
    await logout();
    router.replace("/login");
  };

  return (
    <PullToRefresh onRefresh={load}>
      <PageHeader
        eyebrow={`${greeting()} · ${todayLabel()}`}
        title="对话"
        action={
          role !== "self" ? (
            <button className="pa-icon-btn" onClick={signOut} aria-label="退出登录">
              <LogOut size={20} strokeWidth={1.9} />
            </button>
          ) : undefined
        }
        leading={
          <button className="pa-icon-btn" style={{ marginLeft: -10 }} onClick={() => setDrawer(true)} aria-label="最近的对话">
            <PanelLeft size={22} strokeWidth={1.9} />
          </button>
        }
      />

      {!bots ? (
        <div style={{ marginTop: 8 }}><Skeleton rows={3} /></div>
      ) : (
        <>
          {assistant && (
            <div className="chat-hero" data-tone={botTone(assistant.botKey)}>
              <div className="chat-hero-top">
                <Avatar tone={botTone(assistant.botKey)} size={48} shape="squircle">{assistant.avatar}</Avatar>
                <div style={{ minWidth: 0 }}>
                  <div className="chat-hero-name">{assistant.name}</div>
                  <div className="chat-hero-desc">{assistant.description}</div>
                </div>
              </div>
              <button className="chat-hero-ask" onClick={() => start(assistant.botKey)} data-busy={starting === assistant.botKey}>
                <span>问点什么，或者说「记一下：……」</span>
                <span className="chat-hero-go">
                  {starting === assistant.botKey ? <span className="pa-btn-spin" /> : <ArrowRight size={17} strokeWidth={2.4} />}
                </span>
              </button>
            </div>
          )}

          {assistant && (
            <div className="chat-shortcuts pa-stagger">
              {visibleShortcuts.map((item, index) => {
                const Icon = item.icon;
                const bot = shortcutBot(item.bot);
                return (
                  <button
                    key={item.key}
                    className="chat-shortcut"
                    data-tone={item.tone}
                    style={{ "--i": index } as React.CSSProperties}
                    onClick={() => (item.href ? router.push(item.href) : bot && start(bot, item.prompt, item.key))}
                  >
                    <span className="chat-shortcut-icon">
                      {starting === item.key ? <span className="pa-btn-spin" /> : <Icon size={18} strokeWidth={2} />}
                    </span>
                    <span className="chat-shortcut-text">
                      <b>{item.label}</b>
                      <small>{item.hint}</small>
                    </span>
                  </button>
                );
              })}
            </div>
          )}

          <Tip id="chat-home" title="不知道从哪开始？点上面任意一个" style={{ marginTop: 14 }}>
            每个入口都会帮你把开头写好，改一改就能发。打字嫌慢，就点输入框旁边的<b>麦克风</b>直接说。
          </Tip>

          {helpers.length > 0 && (
            <Section title="更多助手">
              {helpers.map((bot) => (
                <button key={bot.botKey} className="chat-helper" data-tone={botTone(bot.botKey)} onClick={() => start(bot.botKey)}>
                  <Avatar tone={botTone(bot.botKey)} size={44} shape="squircle">{bot.avatar}</Avatar>
                  <div className="chat-helper-text">
                    <div className="chat-helper-name">{bot.name}</div>
                    <div className="chat-helper-desc">{bot.description}</div>
                  </div>
                  {starting === bot.botKey ? <span className="pa-btn-spin" style={{ color: "var(--pa-text-3)" }} /> : <ArrowRight size={18} strokeWidth={2.2} style={{ color: "var(--pa-text-3)", flexShrink: 0 }} />}
                </button>
              ))}
            </Section>
          )}

          {personas.length > 0 && (
            <Section title="家人分身" action={<span className="chat-section-note">先练一练再开口</span>}>
              <div className="chat-personas">
                {personas.map((bot) => (
                  <button key={bot.botKey} className="chat-persona" data-tone={botTone(bot.botKey)} onClick={() => start(bot.botKey)}>
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                      <Avatar tone={botTone(bot.botKey)} size={40}>{bot.avatar}</Avatar>
                      {starting === bot.botKey && <span className="pa-btn-spin" style={{ color: "var(--pa-text-3)" }} />}
                    </div>
                    <div className="chat-persona-name">{bot.name}</div>
                    <div className="chat-persona-desc">{bot.description}</div>
                  </button>
                ))}
              </div>
            </Section>
          )}
        </>
      )}

      <ConversationDrawer
        open={drawer}
        onClose={() => setDrawer(false)}
        onDeleted={(key) => setConversations((list) => list.filter((c) => c.conversationKey !== key))}
      />

      <style jsx>{`
        .chat-hero {
          margin: 8px 16px 0;
          padding: 18px 16px 16px;
          border-radius: var(--pa-radius-lg);
          background:
            radial-gradient(120% 90% at 100% 0%, color-mix(in srgb, var(--tone-fg) 18%, transparent) 0%, transparent 60%),
            linear-gradient(180deg, var(--pa-surface) 0%, color-mix(in srgb, var(--tone-bg) 60%, var(--pa-surface)) 100%);
          border: 0.5px solid var(--pa-line);
          box-shadow: var(--pa-shadow);
        }
        .chat-hero-top { display: flex; gap: 14px; align-items: center; padding: 0 2px; }
        .chat-hero-name { font-family: var(--pa-font-display); font-size: 21px; font-weight: 700; line-height: 1.3; }
        .chat-hero-desc { font-size: 13.5px; color: var(--pa-text-2); margin-top: 2px; line-height: 1.5; }
        .chat-hero-ask {
          appearance: none;
          width: 100%;
          margin-top: 16px;
          height: 52px;
          padding: 0 6px 0 18px;
          border-radius: 26px;
          border: 0.5px solid var(--pa-line-strong);
          background: var(--pa-elevated);
          box-shadow: 0 1px 2px rgba(48, 36, 20, 0.04), 0 8px 20px -10px color-mix(in srgb, var(--tone-fg) 40%, transparent);
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: 10px;
          color: var(--pa-text-3);
          font-size: 15px;
          text-align: left;
          cursor: pointer;
          transition: transform 0.2s var(--pa-spring);
        }
        .chat-hero-ask > span:first-child { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .chat-hero-ask:active { transform: scale(0.98); }
        .chat-hero-go {
          width: 40px;
          height: 40px;
          border-radius: 20px;
          display: grid;
          place-items: center;
          background: var(--pa-accent);
          color: var(--pa-accent-ink);
          flex-shrink: 0;
        }
        .chat-shortcuts { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; padding: 12px 16px 0; }
        .chat-shortcut:last-child:nth-child(odd) { grid-column: 1 / -1; }
        .chat-shortcut {
          appearance: none;
          display: flex;
          align-items: center;
          gap: 10px;
          padding: 12px;
          border-radius: var(--pa-radius);
          border: 0.5px solid var(--pa-line);
          background: var(--pa-surface);
          box-shadow: var(--pa-shadow-sm);
          color: var(--pa-text);
          text-align: left;
          cursor: pointer;
          min-width: 0;
          transition: transform 0.2s var(--pa-spring), background 0.15s;
        }
        .chat-shortcut:active { transform: scale(0.96); background: var(--pa-surface-2); }
        .chat-shortcut-icon {
          width: 36px;
          height: 36px;
          border-radius: 12px;
          display: grid;
          place-items: center;
          background: var(--tone-bg);
          color: var(--tone-fg);
          flex-shrink: 0;
        }
        .chat-shortcut-text { display: flex; flex-direction: column; min-width: 0; }
        .chat-shortcut-text b { font-size: 14.5px; font-weight: 650; line-height: 1.35; }
        .chat-shortcut-text small { font-size: 11.5px; color: var(--pa-text-3); line-height: 1.4; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .chat-section-note { font-size: 12px; color: var(--pa-text-3); }
        .chat-helper {
          appearance: none;
          display: flex;
          align-items: center;
          gap: 12px;
          width: calc(100% - 32px);
          margin: 0 16px;
          padding: 12px 14px;
          text-align: left;
          border: 0.5px solid var(--pa-line);
          border-radius: var(--pa-radius);
          background: linear-gradient(90deg, color-mix(in srgb, var(--tone-bg) 60%, var(--pa-surface)) 0%, var(--pa-surface) 70%);
          box-shadow: var(--pa-shadow-sm);
          color: var(--pa-text);
          cursor: pointer;
          transition: transform 0.2s var(--pa-spring);
        }
        .chat-helper + .chat-helper { margin-top: 10px; }
        .chat-helper:active { transform: scale(0.985); }
        .chat-helper-text { flex: 1; min-width: 0; }
        .chat-helper-name { font-weight: 650; font-size: 16px; }
        .chat-helper-desc { font-size: 12.5px; color: var(--pa-text-3); margin-top: 2px; line-height: 1.5; }
        .chat-personas {
          display: flex;
          gap: 10px;
          padding: 2px 16px 6px;
          overflow-x: auto;
          scroll-snap-type: x mandatory;
          scroll-padding: 0 16px;
          scrollbar-width: none;
        }
        .chat-personas::-webkit-scrollbar { display: none; }
        .chat-persona {
          appearance: none;
          text-align: left;
          border: 0.5px solid var(--pa-line);
          background:
            radial-gradient(100% 70% at 100% 0%, color-mix(in srgb, var(--tone-fg) 10%, transparent), transparent 70%),
            var(--pa-surface);
          border-radius: var(--pa-radius);
          padding: 14px;
          box-shadow: var(--pa-shadow-sm);
          cursor: pointer;
          transition: transform 0.2s var(--pa-spring), background 0.2s;
          min-height: 132px;
          width: 148px;
          flex-shrink: 0;
          scroll-snap-align: start;
          display: flex;
          flex-direction: column;
        }
        .chat-persona:active { transform: scale(0.97); }
        .chat-persona-name { font-weight: 600; font-size: 15.5px; margin-top: 12px; }
        .chat-persona-desc {
          font-size: 12.5px;
          color: var(--pa-text-3);
          margin-top: 3px;
          line-height: 1.5;
          display: -webkit-box;
          -webkit-line-clamp: 2;
          -webkit-box-orient: vertical;
          overflow: hidden;
        }
      `}</style>
    </PullToRefresh>
  );
}
