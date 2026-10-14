import type { Tone } from "./tone";

export const memoryStatus: Record<string, { label: string; tone: Tone }> = {
  pending: { label: "待确认", tone: "amber" },
  active: { label: "生效", tone: "sage" },
  superseded: { label: "已过时", tone: "slate" },
  archived: { label: "已归档", tone: "slate" },
  rejected: { label: "已驳回", tone: "rose" },
};

export const createdByLabel: Record<string, string> = {
  manual: "手工",
  chat: "对话",
  extract: "自动抽取",
  reflect: "归纳",
};

export const sideLabel: Record<string, string> = {
  core: "核心小家",
  self: "我这边",
  wife: "老婆那边",
};

export const placeKindLabel: Record<string, string> = {
  home: "现在住的家",
  hometown: "老家",
  other: "其他",
};

export const toolLabel: Record<string, string> = {
  search_memory: "检索记忆",
  get_person: "查看人物档案",
  get_relations: "查看社会关系",
  get_place: "查看地点",
  list_open_items: "未完成的承诺/诉求",
  remember: "记录记忆",
  ask_persona: "让分身预演",
  list_upcoming: "查看近期提醒",
  create_reminder: "建立提醒",
  cancel_reminder: "取消提醒",
  load_skill: "读做事手册",
  now: "当前时间",
};

export const taskLabel: Record<string, string> = {
  "chat.assistant": "私人助理对话",
  "chat.persona": "分身对话",
  "memory.extract": "记忆抽取",
  "persona.card": "人设卡生成",
  "persona.interview": "对话补充人设卡",
  "chat.summary": "长对话分段摘要",
  "eval.judge": "评测打分",
  "skill.rewrite": "按记忆改写手册",
  embedding: "向量化",
};
