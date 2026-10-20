/**
 * 孩子的饮食：计划（爸爸妈妈能改）和实际吃了什么（谁都能记）。
 * 两边都能拍照 / 说一句，服务端交给模型整理（server/service/diet）。
 */
import dayjs from "dayjs";
import { api, postForm } from "./api";
import type { DietCaptureResult, DietOverview, DietPlanCaptureResult } from "./types";

export const mealLabels: Record<string, string> = {
  day: "全天",
  breakfast: "早饭",
  morning_snack: "上午加餐",
  lunch: "午饭",
  afternoon_snack: "下午加餐",
  dinner: "晚饭",
  night_snack: "睡前",
};

export const amountOptions = [
  { value: "all", label: "吃完了" },
  { value: "most", label: "吃了大半" },
  { value: "half", label: "吃了一半" },
  { value: "little", label: "吃了几口" },
  { value: "none", label: "没吃" },
];

export function amountLabel(value: string): string {
  return amountOptions.find((option) => option.value === value)?.label ?? "";
}

export function dietOverview(from: string, days = 1, child = ""): Promise<DietOverview> {
  const query = new URLSearchParams({ from, days: String(days) });
  if (child) query.set("child", child);
  return api.get<DietOverview>(`/diet?${query}`);
}

/** 实际吃了什么：拍照 / 说一句，模型整理成记录。 */
export function captureRecord(text: string, photos: Blob[], child = ""): Promise<DietCaptureResult> {
  const form = new FormData();
  form.set("text", text);
  form.set("at", new Date().toISOString());
  if (child) form.set("child", child);
  photos.forEach((photo, index) => form.append("images", photo, `${index}.jpg`));
  return postForm<DietCaptureResult>("/diet/records/capture", form);
}

/** 改计划对话里的一轮；连着说时把前面几轮一起带上。 */
export interface PlanTurn {
  role: "parent" | "assistant";
  text: string;
}

/** 计划：说一句 / 拍张食谱，模型改计划。from 是正在看的那天，history 是这次对话前面几轮。 */
export function capturePlan(text: string, photos: Blob[], from: string, days = 7, child = "", history: PlanTurn[] = []): Promise<DietPlanCaptureResult> {
  const form = new FormData();
  if (history.length) form.set("history", JSON.stringify(history.slice(-12)));
  form.set("text", text);
  form.set("from", from);
  form.set("days", String(days));
  if (child) form.set("child", child);
  photos.forEach((photo, index) => form.append("images", photo, `${index}.jpg`));
  return postForm<DietPlanCaptureResult>("/diet/plans/capture", form);
}

export function today(): string {
  return dayjs().format("YYYY-MM-DD");
}
