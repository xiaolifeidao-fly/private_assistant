"use client";

import { useState } from "react";
import { Toast } from "antd-mobile";
import { ChevronDown, Lock, Users } from "lucide-react";
import { api } from "@/lib/api";
import { useMe } from "@/lib/role";

export type Visibility = "private" | "family";

const choices: { value: Visibility; label: string; hint: string; icon: typeof Lock }[] = [
  { value: "private", label: "只有我", hint: "别的家人登录看不到，也搜不到", icon: Lock },
  { value: "family", label: "全家都能看到", hint: "家里每个账号都能看到、搜到；提醒会推到每个人手机上", icon: Users },
];

/**
 * 记录、提醒的「谁能看到」：默认只有我，收起成一行；点开可以改成全家都能看到。
 * 奶奶的账号只能私有，不显示这个选项（服务端也会强制）。
 */
export function VisibilityOption({ value, onChange, noun = "这条记录" }: { value: Visibility; onChange: (value: Visibility) => void; noun?: string }) {
  const { role } = useMe();
  const [open, setOpen] = useState(false);
  if (role === "elder") return null;
  const current = choices.find((choice) => choice.value === value) ?? choices[0];
  const Icon = current.icon;
  return (
    <div className="pa-field">
      <button type="button" className="vis-row" data-open={open} data-value={value} onClick={() => setOpen(!open)} aria-expanded={open}>
        <Icon size={15} strokeWidth={2.2} />
        <span>{noun}：<b>{current.label}</b></span>
        <ChevronDown size={16} strokeWidth={2.2} className="vis-chevron" />
      </button>
      {open && (
        <div className="vis-choices pa-enter">
          {choices.map((choice) => {
            const ChoiceIcon = choice.icon;
            return (
              <button key={choice.value} type="button" className="vis-choice" data-on={choice.value === value}
                onClick={() => {
                  onChange(choice.value);
                  setOpen(false);
                }}
              >
                <ChoiceIcon size={18} strokeWidth={2.1} />
                <span><b>{choice.label}</b><small>{choice.hint}</small></span>
              </button>
            );
          })}
        </div>
      )}
      <VisibilityStyles />
    </div>
  );
}

/**
 * 详情页里的可见范围：显示现在谁能看到；是自己的记录就能切换。
 * kind 决定调哪个接口（/memories/:key/visibility、/reminders/:key/visibility）。
 */
export function VisibilitySwitch<T extends { ownerAccount: string; visibility: string }>({ kind, itemKey, item, onChanged }: {
  kind: "memories" | "reminders";
  itemKey: string;
  item: T;
  onChanged: (item: T) => void;
}) {
  const { me, role } = useMe();
  const [busy, setBusy] = useState(false);
  const shared = item.visibility === "family";
  const mine = !!me && me.accountKey === item.ownerAccount;
  const canChange = mine && role !== "elder";
  const label = kind === "reminders" ? (shared ? "全家都会收到" : mine ? "只提醒我" : "只提醒设它的人") : shared ? "全家都能看到" : "只有我看得到";

  const toggle = async () => {
    setBusy(true);
    try {
      const next = await api.put<T>(`/${kind}/${itemKey}/visibility`, { visibility: shared ? "private" : "family" });
      onChanged(next);
      Toast.show({ content: kind === "reminders" ? (shared ? "只提醒你自己了" : "全家都会收到这条提醒") : shared ? "改成只有你自己了" : "全家都能看到了" });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="vis-switch" data-value={item.visibility}>
      {shared ? <Users size={15} strokeWidth={2.2} /> : <Lock size={15} strokeWidth={2.2} />}
      <span>{label}</span>
      {canChange && (
        <button type="button" className="pa-link-btn" onClick={toggle} disabled={busy}>
          {busy ? "…" : shared ? (kind === "reminders" ? "改成只提醒我" : "改成只有我") : kind === "reminders" ? "全家都提醒" : "让全家看到"}
        </button>
      )}
      <VisibilityStyles />
    </div>
  );
}

function VisibilityStyles() {
  return (
    <style jsx global>{`
      .vis-row {
        appearance: none;
        display: inline-flex;
        align-items: center;
        gap: 6px;
        height: 34px;
        padding: 0 12px;
        border-radius: 17px;
        border: 0.5px solid var(--pa-line-strong);
        background: var(--pa-surface);
        color: var(--pa-text-2);
        font-size: 13px;
        cursor: pointer;
      }
      .vis-row b { color: var(--pa-text); font-weight: 650; }
      .vis-row[data-value="family"] { color: var(--tone-sky); border-color: color-mix(in srgb, var(--tone-sky) 35%, transparent); }
      .vis-chevron { transition: transform 0.2s var(--pa-ease); color: var(--pa-text-3); }
      .vis-row[data-open="true"] .vis-chevron { transform: rotate(180deg); }
      .vis-choices { display: flex; flex-direction: column; gap: 8px; margin-top: 10px; }
      .vis-choice {
        appearance: none;
        display: flex;
        align-items: flex-start;
        gap: 10px;
        padding: 12px 14px;
        border-radius: var(--pa-radius-sm);
        border: 1px solid var(--pa-line);
        background: var(--pa-surface);
        color: var(--pa-text);
        text-align: left;
        cursor: pointer;
      }
      .vis-choice svg { margin-top: 2px; flex-shrink: 0; color: var(--pa-text-2); }
      .vis-choice span { display: flex; flex-direction: column; gap: 2px; }
      .vis-choice b { font-size: 14.5px; font-weight: 650; }
      .vis-choice small { font-size: 12px; color: var(--pa-text-3); line-height: 1.5; }
      .vis-choice[data-on="true"] { border-color: var(--pa-accent); background: var(--pa-accent-soft); }
      .vis-choice[data-on="true"] svg { color: var(--pa-accent); }
      .vis-switch {
        display: flex;
        align-items: center;
        gap: 6px;
        font-size: 13px;
        color: var(--pa-text-2);
      }
      .vis-switch[data-value="family"] { color: var(--tone-sky); }
      .vis-switch .pa-link-btn { margin-left: auto; }
    `}</style>
  );
}
