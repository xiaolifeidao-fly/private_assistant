import type { Person } from "./types";

/** 识别色名，对应 globals.css 里的 [data-tone]。 */
export type Tone = "rose" | "amber" | "sky" | "sage" | "clay" | "plum" | "slate";

// 核心家人颜色固定，看一眼就知道是谁
const fixed: Record<string, Tone> = { wife: "rose", mom: "amber", daughter: "sky", me: "sage" };
const sideTones: Record<string, Tone[]> = {
  self: ["amber", "clay", "slate"],
  wife: ["rose", "plum", "slate"],
  core: ["sage", "sky"],
};

export function personTone(key: string, persons: Person[] = []): Tone {
  if (!key) return "slate";
  if (fixed[key]) return fixed[key];
  const person = persons.find((p) => p.personKey === key);
  const palette = sideTones[person?.side ?? ""] ?? ["slate", "plum", "clay"];
  let hash = 0;
  for (const char of key) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  return palette[hash % palette.length];
}

/** 机器人的颜色跟它代表的人走；私人助理用陶土色。 */
export function botTone(botKey: string): Tone {
  if (botKey === "assistant") return "clay";
  if (botKey === "recall") return "sage";
  return fixed[botKey] ?? "plum";
}

const typeTones: Record<string, Tone> = {
  quote: "plum",
  need: "rose",
  commitment: "clay",
  event: "slate",
  preference: "amber",
  habit: "sage",
  trait: "sky",
  taboo: "rose",
  custom: "sage",
  health: "sky",
  routine: "slate",
  item: "sky",
};

export function typeTone(type: string): Tone {
  return typeTones[type] ?? "slate";
}

/**
 * 头像里放的字：核心家人有名字取名字末字（小雨→雨），亲戚取称呼末字（大舅→舅），
 * 都没有时取称呼本身（妈）。
 */
export function personInitial(person: Person | undefined, fallback = "家"): string {
  if (!person) return fallback;
  if (person.personKey === "me") return "我";
  const last = (text: string) => Array.from(text.trim()).pop();
  if (person.isCore && person.name && person.name !== person.roleLabel) return last(person.name) ?? fallback;
  return last(person.roleLabel || "") ?? last(person.name || "") ?? fallback;
}
