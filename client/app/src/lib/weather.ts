import dayjs from "dayjs";
import { api } from "./api";

/** 宝宝穿衣（server/service/weather）：天气、今天穿什么、几点前到家、某一天的送接安排。 */

export interface WeatherLocation {
  name: string;
  code: string;
  lat: number;
  lon: number;
}

export interface WeatherNow {
  temp: number;
  feels: number;
  humidity: number;
  text: string;
  wind: string;
  aqi: number;
  at: string;
}

export interface WeatherHour {
  at: string;
  temp: number;
  feels: number;
  text: string;
  rain: number;
  wet: boolean;
  windLevel: number;
}

export interface WeatherDay {
  date: string;
  high: number;
  low: number;
  dayText: string;
  nightText: string;
  sunrise: string;
  sunset: string;
}

export interface WeatherWarning {
  type: string;
  level: string;
  text: string;
}

export interface WeatherSnapshot {
  location: WeatherLocation;
  source: string;
  now: WeatherNow;
  hours: WeatherHour[] | null;
  days: WeatherDay[] | null;
  warnings: WeatherWarning[] | null;
  clothing: string;
  fetchedAt: string;
  stale: boolean;
}

export interface Outing {
  from: string;
  to: string;
  note?: string;
}

export interface DayPlan {
  date: string;
  school: boolean;
  drop?: string;
  pickup?: string;
  home?: string;
  outings: Outing[];
  note?: string;
  override: boolean;
}

export interface OutfitPart {
  kind: "drop" | "pickup" | "outing";
  label: string;
  start: string;
  end: string;
  feelsMin: number;
  feelsMax: number;
  level: { key: string; label: string };
  layers: number;
  clothes: string[];
  extras: string[];
  backBy?: string;
  backReason?: string;
  playUntil?: string;
  directHome: boolean;
  skip: boolean;
  skipReason?: string;
  rainFrom?: string;
  notes: string[];
  estimated: boolean;
}

export interface Advice {
  slot: string;
  date: string;
  place: string;
  now: WeatherNow;
  high: number;
  low: number;
  hasRange: boolean;
  sunset: string;
  warnings: WeatherWarning[];
  clothing: string;
  infant: boolean;
  parts: OutfitPart[];
  night?: { low: number; bedding: string; tips: string[] };
  tomorrow?: WeatherDay;
  notes: string[];
  headline: string;
  stale: boolean;
  fetchedAt: string;
}

export interface Brief {
  briefKey: string;
  date: string;
  slot: string;
  slotLabel: string;
  headline: string;
  content: string;
  source: "llm" | "template" | "skipped";
  pushedTo: string[];
  createdAt: string;
}

export interface WeatherToday {
  location: WeatherLocation;
  snapshot: WeatherSnapshot;
  plan: DayPlan;
  tomorrow: DayPlan;
  advice: Advice | null;
  evening: Advice | null;
  briefs: Brief[];
  slots: { morning: string; noon: string; evening: string; prePickup: number };
  child: string;
  error?: string;
}

export interface DayInput {
  date: string;
  noSchool?: boolean;
  drop?: string;
  pickup?: string;
  home?: string;
  outings?: Outing[];
  note?: string;
}

export const weatherToday = () => api.get<WeatherToday>("/weather");
export const saveDay = (input: DayInput) => api.put<DayPlan>("/weather/day", input);
export const resetDay = (date: string) => api.del<DayPlan>(`/weather/day?date=${date}`);

export const deg = (value: number) => `${Math.round(value)}°`;
export const hm = (value?: string) => (value ? dayjs(value).format("HH:mm") : "");

/** 「08:00 送 · 15:50 接 · 16:30 前到家」 */
export function planLine(plan: DayPlan): string {
  const parts: string[] = [];
  if (plan.school) parts.push(`${plan.drop} 送`, `${plan.pickup} 接`, `${plan.home} 前到家`);
  else parts.push("不去幼儿园");
  for (const outing of plan.outings) parts.push(`${outing.from}–${outing.to} ${outing.note || "出去玩"}`);
  return parts.join(" · ");
}

/** 衣服对应的小图标。 */
export function clothIcon(name: string): string {
  const table: [string, string][] = [
    ["羽绒", "🧥"], ["厚外套", "🧥"], ["外套", "🧥"], ["毛衣", "🧶"], ["卫衣", "🧶"], ["秋衣", "👕"], ["保暖", "👕"],
    ["短袖", "👕"], ["长袖", "👕"], ["短裤", "🩳"], ["帽", "🧢"], ["围巾", "🧣"], ["手套", "🧤"], ["伞", "☂️"],
    ["雨", "☂️"], ["防水鞋", "🥾"], ["口罩", "😷"], ["水", "🥤"], ["备用", "🎒"], ["多加", "➕"],
  ];
  return table.find(([word]) => name.includes(word))?.[1] ?? "👚";
}

/** 天气现象对应的图标。 */
export function weatherIcon(text: string): string {
  if (!text) return "🌡️";
  if (text.includes("雷")) return "⛈️";
  if (text.includes("雪")) return "🌨️";
  if (text.includes("雨")) return "🌧️";
  if (text.includes("雾") || text.includes("霾")) return "🌫️";
  if (text.includes("阴")) return "☁️";
  if (text.includes("多云")) return "⛅";
  if (text.includes("晴")) return "☀️";
  return "🌡️";
}

/** 推送标题里的那句：要早点回来就是最要紧的一段。 */
export function keyPart(advice: Advice | null): OutfitPart | undefined {
  if (!advice) return undefined;
  return advice.parts.find((part) => part.kind === "pickup" && (part.directHome || part.backReason)) ?? advice.parts[0];
}
