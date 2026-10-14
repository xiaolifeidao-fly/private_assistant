import dayjs from "dayjs";

const weekdays = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];

/** 列表里的时间：今天显示时分，昨天/前天显示词，一周内显示周几，再早显示日期。 */
export function shortTime(value: string | Date | null | undefined): string {
  if (!value) return "";
  const time = dayjs(value);
  const now = dayjs();
  const days = now.startOf("day").diff(time.startOf("day"), "day");
  if (days <= 0) {
    const minutes = now.diff(time, "minute");
    if (minutes < 1) return "刚刚";
    if (minutes < 60) return `${minutes} 分钟前`;
    return time.format("HH:mm");
  }
  if (days === 1) return "昨天";
  if (days === 2) return "前天";
  if (days < 7) return weekdays[time.day()];
  if (time.year() === now.year()) return time.format("M月D日");
  return time.format("YYYY年M月D日");
}

/** 未来的时间：今天 / 明天 / 后天 / x 天后 / 日期。 */
export function upcomingDay(value: string | Date): { label: string; days: number } {
  const time = dayjs(value);
  const days = time.startOf("day").diff(dayjs().startOf("day"), "day");
  if (days <= 0) return { label: "今天", days };
  if (days === 1) return { label: "明天", days };
  if (days === 2) return { label: "后天", days };
  if (days < 7) return { label: `${days} 天后`, days };
  return { label: time.format("M月D日"), days };
}

export function greeting(): string {
  const hour = dayjs().hour();
  if (hour < 5) return "夜深了";
  if (hour < 11) return "早上好";
  if (hour < 13) return "中午好";
  if (hour < 18) return "下午好";
  return "晚上好";
}

export function todayLabel(): string {
  const now = dayjs();
  return `${now.format("M月D日")} · ${weekdays[now.day()]}`;
}
