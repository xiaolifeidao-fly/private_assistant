/**
 * 长辈模式（奶奶）的三件事。每件事是一段对话：
 *  - 万事通、记一下都交给万能通（recall），记一下会在话前面加「记一下：」，让它用 remember 记住；
 *  - 提醒交给提醒助手（remind）。
 * 每件事每天一段对话（按标题区分），老人家打开就是今天的，不用管「新对话」。
 */
import dayjs from "dayjs";
import { AlarmClock, NotebookPen, Search, type LucideIcon } from "lucide-react";
import { api } from "./api";
import type { Tone } from "./tone";
import type { Conversation, Paged } from "./types";

export type ElderMode = "ask" | "note" | "remind";

export interface ElderModeConfig {
  mode: ElderMode;
  bot: string;
  title: string;
  hint: string;
  icon: LucideIcon;
  tone: Tone;
  greeting: string;
  examples: string[];
  /** 发出去之前加在话前面 */
  prefix?: string;
  /** 能不能拍照（记东西放哪） */
  photos?: boolean;
}

export const elderModes: ElderModeConfig[] = [
  {
    mode: "ask",
    bot: "recall",
    title: "万事通",
    hint: "东西放哪了？什么事想不起来，问它",
    icon: Search,
    tone: "sage",
    greeting: "想找什么、想不起什么，按住下面的按钮问我就行。",
    examples: ["老花镜放哪了？", "医保卡在哪里？", "上次体检是哪天？"],
  },
  {
    mode: "note",
    bot: "recall",
    title: "记一下",
    hint: "说一句，帮您记住",
    icon: NotebookPen,
    tone: "clay",
    greeting: "您说，我帮您记着。东西放在哪，也可以先拍张照再说。",
    examples: ["老花镜放在电视柜第二个抽屉", "降压药放在床头柜里"],
    prefix: "记一下：",
    photos: true,
  },
  {
    mode: "remind",
    bot: "remind",
    title: "提醒",
    hint: "到点提醒您吃药、办事",
    icon: AlarmClock,
    tone: "plum",
    greeting: "要提醒您什么？说上时间就行，到点手机会响。",
    examples: ["明天早上 8 点提醒我吃降压药", "周六下午 3 点提醒我去医院拿药", "最近有哪些提醒？"],
  },
];

export function elderMode(mode: string): ElderModeConfig | undefined {
  return elderModes.find((item) => item.mode === mode);
}

/** 找今天这件事的对话，没有就开一段并起好名字。 */
export async function todayConversation(config: ElderModeConfig): Promise<string> {
  const page = await api.get<Paged<Conversation>>(`/conversations?botKey=${config.bot}&size=30`);
  const today = dayjs().format("YYYY-MM-DD");
  const hit = page.items.find((c) => c.title === config.title && dayjs(c.lastMessageAt ?? c.createdAt).format("YYYY-MM-DD") === today);
  if (hit) return hit.conversationKey;
  const created = await api.post<Conversation>("/conversations", { botKey: config.bot });
  await api.put(`/conversations/${created.conversationKey}`, { title: config.title }).catch(() => {});
  return created.conversationKey;
}

/** 加上前缀；已经自己说了「记一下」「记住」的不重复加。 */
export function withPrefix(config: ElderModeConfig, text: string): string {
  if (!config.prefix || /^(记一下|记住|帮我记)/.test(text)) return text;
  return `${config.prefix}${text}`;
}
