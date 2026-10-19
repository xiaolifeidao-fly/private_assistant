"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import dayjs from "dayjs";
import { Dialog, Toast } from "antd-mobile";
import { api } from "@/lib/api";
import { upcomingDay } from "@/lib/time";
import type { Tone } from "@/lib/tone";
import type { Reminder } from "@/lib/types";
import { Button, Pill, Sheet, useLatest } from "./ui";
import { VisibilitySwitch } from "./visibility";

export const reminderMeta: Record<string, { label: string; tone: Tone }> = {
  daily: { label: "今日", tone: "sage" },
  birthday: { label: "生日", tone: "rose" },
  commitment: { label: "承诺", tone: "clay" },
  dated: { label: "日期", tone: "plum" },
  festival: { label: "节日", tone: "amber" },
  trip: { label: "行程", tone: "sky" },
  manual: { label: "提醒", tone: "slate" },
};

const statusText: Record<string, string> = { scheduled: "待提醒", failed: "推送失败，会重试", sending: "推送中", sent: "已推送" };

/** 一条提醒的详情：完整内容、时间，聊天里建的能回到原对话，还没推送的能取消。 */
export function ReminderSheet({ reminder: current, onClose, onChanged }: { reminder: Reminder | null; onClose: () => void; onChanged: () => void }) {
  const router = useRouter();
  const latest = useLatest(current);
  // 改了可见范围后用新的那份显示
  const [changed, setChanged] = useState<Reminder | null>(null);
  const item = changed && latest && changed.reminderKey === latest.reminderKey ? changed : latest;
  const [busy, setBusy] = useState(false);
  const meta = item ? reminderMeta[item.kind] ?? reminderMeta.manual : reminderMeta.manual;
  const fromChat = !!item?.targetUrl?.startsWith("/chat/");
  const cancellable = item?.status === "scheduled" || item?.status === "failed";

  const cancel = async () => {
    if (!item) return;
    const ok = await Dialog.confirm({ content: `取消「${item.title}」？到点就不会再推送了。`, confirmText: "取消提醒", cancelText: "再想想" });
    if (!ok) return;
    setBusy(true);
    try {
      await api.post(`/reminders/${item.reminderKey}/dismiss`);
      Toast.show({ content: "已取消" });
      onChanged();
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Sheet open={!!current} onClose={onClose} title="提醒">
      {item && (
        <div style={{ padding: "6px 20px 0" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
            <Pill tone={meta.tone} dot>{meta.label}</Pill>
            <span style={{ fontSize: 12.5, color: "var(--pa-text-3)" }}>{statusText[item.status] ?? item.status}</span>
          </div>
          <div style={{ fontSize: 19, fontWeight: 700, marginTop: 12, lineHeight: 1.4 }}>{item.title}</div>
          <div style={{ fontSize: 13.5, color: "var(--pa-text-2)", marginTop: 6 }}>
            {upcomingDay(item.remindAt).label} {dayjs(item.remindAt).format("M月D日 HH:mm")} 提醒
            {item.eventAt && !dayjs(item.eventAt).isSame(item.remindAt, "minute") && <> · 事情在 {dayjs(item.eventAt).format("M月D日")}</>}
          </div>
          <div style={{ marginTop: 10 }}>
            <VisibilitySwitch kind="reminders" itemKey={item.reminderKey} item={item} onChanged={(next) => { setChanged(next); onChanged(); }} />
          </div>
          {item.body && item.body.trim() !== item.title.trim() && (
            <div style={{ marginTop: 14, padding: "12px 14px", borderRadius: 12, background: "var(--pa-fill)", fontSize: 15, lineHeight: 1.65, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
              {item.body}
            </div>
          )}
          <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 20 }}>
            {fromChat && (
              <Button block variant="ink" onClick={() => { onClose(); router.push(item.targetUrl); }}>回到对话（改时间、补充内容）</Button>
            )}
            {cancellable && <Button block variant="danger" loading={busy} onClick={cancel}>取消这条提醒</Button>}
          </div>
        </div>
      )}
    </Sheet>
  );
}
