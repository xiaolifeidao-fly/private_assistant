/**
 * 孩子的饮食：计划和实际吃了什么（server/service/diet，接口同 App 端）。
 * 管理端按周对照着看，能手动改，也能说一句话交给模型整理。
 */
import dayjs, { type Dayjs } from "dayjs";
import { api, postForm, query } from "./api";
import type { Tone } from "./tone";

export interface DietMeal {
  key: string;
  label: string;
  clock: string;
}

export interface DietPlan {
  planKey: string;
  childKey: string;
  date: string;
  meal: string;
  foods: string;
  note: string;
  createdBy: string;
  updatedBy: string;
  updatedAt: string;
}

export interface DietRecord {
  recordKey: string;
  childKey: string;
  eatenAt: string;
  meal: string;
  foods: string;
  amount: string;
  note: string;
  source: string;
  rawText: string;
  photoCount: number;
  captureKey: string;
  createdBy: string;
  updatedBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface DietOverview {
  childKey: string;
  childLabel: string;
  from: string;
  days: number;
  canPlan: boolean;
  meals: DietMeal[];
  plans: DietPlan[];
  records: DietRecord[];
}

export interface DietReview {
  reviewKey: string;
  childKey: string;
  weekStart: string;
  content: string;
  createdBy: string;
  updatedAt: string;
}

export interface DietCaptureResult {
  records: DietRecord[];
  reply: string;
  fallback: boolean;
}

export interface DietPlanCaptureResult {
  changed: DietPlan[];
  reply: string;
}

export const DAY = "day";

export const mealLabels: Record<string, string> = {
  day: "全天",
  breakfast: "早饭",
  morning_snack: "上午加餐",
  lunch: "午饭",
  afternoon_snack: "下午加餐",
  dinner: "晚饭",
  night_snack: "睡前",
};

/** 吃了多少，按多到少排；颜色是同一个色相由深到浅（多 = 深）。 */
export const amounts: { value: string; label: string; short: string; tone: Tone; weight: number }[] = [
  { value: "all", label: "吃完了", short: "吃完", tone: "sage", weight: 1 },
  { value: "most", label: "吃了大半", short: "大半", tone: "sage", weight: 0.72 },
  { value: "half", label: "吃了一半", short: "一半", tone: "sage", weight: 0.48 },
  { value: "little", label: "吃了几口", short: "几口", tone: "sage", weight: 0.28 },
  { value: "none", label: "没吃", short: "没吃", tone: "rose", weight: 0 },
];

export function amountOf(value: string) {
  return amounts.find((item) => item.value === value);
}

/** 周一为一周的开始。 */
export function weekStart(day: Dayjs): Dayjs {
  return day.startOf("day").subtract((day.day() + 6) % 7, "day");
}

export const weekdays = ["日", "一", "二", "三", "四", "五", "六"];

export function cellKey(date: string, meal: string) {
  return `${date}|${meal}`;
}

export function recordDate(record: DietRecord) {
  return dayjs(record.eatenAt).format("YYYY-MM-DD");
}

export const dietApi = {
  overview: (child: string, from: string, days = 7) => api.get<DietOverview>(`/diet${query({ child, from, days })}`),
  savePlan: (body: { childKey: string; date: string; meal: string; foods: string; note: string }) =>
    api.put<{ plan: DietPlan | null }>("/diet/plans", body),
  copyPlans: (body: { childKey: string; from: string; to: string; days: number }) => api.post<{ count: number }>("/diet/plans/copy", body),
  createRecord: (body: Partial<DietRecord> & { childKey: string }) => api.post<DietRecord>("/diet/records", body),
  updateRecord: (key: string, body: Partial<DietRecord> & { childKey: string }) => api.put<DietRecord>(`/diet/records/${key}`, body),
  deleteRecord: (key: string) => api.del(`/diet/records/${key}`),
  review: (child: string, date: string) => api.get<{ review: DietReview | null }>(`/diet/review${query({ child, date })}`),
  createReview: (childKey: string, date: string) => api.post<DietReview>("/diet/review", { childKey, date }),
  /** 说一句 / 拍张食谱改计划；from、days 是正在看的那周。 */
  capturePlan: (child: string, text: string, images: File[], from: string, days = 7) => {
    const form = new FormData();
    form.set("child", child);
    form.set("text", text);
    form.set("from", from);
    form.set("days", String(days));
    images.forEach((image) => form.append("images", image, image.name));
    return postForm<DietPlanCaptureResult>("/diet/plans/capture", form);
  },
  /** 说一句 / 拍张照记实际吃了什么。 */
  captureRecord: (child: string, text: string, images: File[]) => {
    const form = new FormData();
    form.set("child", child);
    form.set("text", text);
    form.set("at", new Date().toISOString());
    images.forEach((image) => form.append("images", image, image.name));
    return postForm<DietCaptureResult>("/diet/records/capture", form);
  },
};
