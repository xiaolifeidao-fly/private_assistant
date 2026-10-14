"use client";

import { useEffect, useState } from "react";
import { Dialog, Toast } from "antd-mobile";
import { Bell, BellOff, BellRing } from "lucide-react";
import { disablePush, enablePush, isStandalone, pushState, testPush } from "@/lib/push";
import { Card, Row, Section } from "./ui";

type State = Awaited<ReturnType<typeof pushState>> | "loading";

const labels: Record<State, string> = {
  loading: "检查中…",
  on: "主动提醒已开启",
  off: "开启 iPhone 推送",
  denied: "通知权限已被拒绝",
  unsupported: "当前环境不支持推送",
  unconfigured: "服务器还没配置推送密钥",
};

/**
 * 推送开关。没开时放在页面顶部做引导（placement=top），
 * 其它状态收在页面底部的「通知」分区里（placement=bottom）。
 */
export function PushCard({ placement }: { placement: "top" | "bottom" }) {
  const [state, setState] = useState<State>("loading");
  const refresh = () => pushState().then(setState).catch(() => setState("unsupported"));
  useEffect(() => {
    void refresh();
  }, []);

  const enable = async () => {
    if (!isStandalone() && /iPhone|iPad|iPod/.test(navigator.userAgent)) {
      await Dialog.alert({ title: "先添加到主屏幕", content: "iPhone 需要 iOS 16.4 以上，并在 Safari 里点「分享 → 添加到主屏幕」，再从主屏打开。", confirmText: "知道了" });
      return;
    }
    try {
      await enablePush();
      setState("on");
      Toast.show({ icon: "success", content: "主动提醒已开启" });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
      refresh();
    }
  };

  const disable = async () => {
    try {
      await disablePush();
      setState("off");
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    }
  };

  const test = async () => {
    try {
      await testPush();
      Toast.show({ content: "测试通知已发出" });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    }
  };

  if (state === "loading") return null;

  if (placement === "top") {
    if (state !== "off") return null;
    return (
      <div className="push-banner" onClick={enable}>
        <div className="push-banner-icon"><BellRing size={20} /></div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontWeight: 600, fontSize: 15 }}>开启主动提醒</div>
          <div style={{ fontSize: 12.5, color: "var(--pa-text-2)", marginTop: 2 }}>生日、承诺到期、回老家前，提前告诉你</div>
        </div>
        <button className="pa-btn" data-size="sm" data-variant="accent" onClick={(event) => { event.stopPropagation(); void enable(); }}>开启</button>
        <style jsx>{`
          .push-banner {
            margin: 16px 16px 0;
            padding: 14px 14px 14px 16px;
            display: flex;
            align-items: center;
            gap: 12px;
            border-radius: var(--pa-radius);
            background: linear-gradient(135deg, var(--pa-accent-soft), color-mix(in srgb, var(--pa-accent-soft) 35%, var(--pa-surface)));
            border: 0.5px solid color-mix(in srgb, var(--pa-accent) 18%, transparent);
            cursor: pointer;
          }
          .push-banner-icon {
            width: 40px;
            height: 40px;
            border-radius: 12px;
            display: grid;
            place-items: center;
            background: var(--pa-surface);
            color: var(--pa-accent);
            flex-shrink: 0;
          }
        `}</style>
      </div>
    );
  }

  if (state === "off") return null;
  return (
    <Section title="通知">
      <Card>
        <Row
          leading={
            <div style={{ width: 32, height: 32, borderRadius: 10, display: "grid", placeItems: "center", background: state === "on" ? "var(--pa-success-soft)" : "var(--pa-fill-strong)", color: state === "on" ? "var(--pa-success)" : "var(--pa-text-3)" }}>
              {state === "on" ? <Bell size={17} /> : <BellOff size={17} />}
            </div>
          }
          inset={60}
          title={labels[state]}
          subtitle={state === "on" ? "生日、承诺到期和回老家前会提醒" : state === "denied" ? "到系统设置 → 通知里重新允许" : undefined}
          trailing={
            state === "on" ? (
              <span style={{ display: "flex", gap: 6 }}>
                <button className="pa-btn" data-size="sm" data-variant="soft" onClick={test}>测试</button>
                <button className="pa-btn" data-size="sm" data-variant="soft" onClick={disable}>关闭</button>
              </span>
            ) : undefined
          }
        />
      </Card>
    </Section>
  );
}
