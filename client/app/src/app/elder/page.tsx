"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Dialog, Toast } from "antd-mobile";
import { BellRing, ChevronRight, Utensils } from "lucide-react";
import { elderModes } from "@/lib/elder";
import { enablePush, isStandalone, pushState } from "@/lib/push";
import { logout, useMe } from "@/lib/role";
import { greeting, todayLabel } from "@/lib/time";

/** 长辈模式首页：三张大卡片，点哪个就进哪个。 */
export default function ElderHome() {
  const router = useRouter();
  const { me } = useMe();
  const [push, setPush] = useState<string>("");
  const [enabling, setEnabling] = useState(false);

  useEffect(() => {
    elderModes.forEach((item) => router.prefetch(`/elder/${item.mode}`));
    router.prefetch("/elder/diet");
    pushState().then(setPush).catch(() => setPush(""));
  }, [router]);

  const turnOnPush = async () => {
    if (!isStandalone()) {
      Dialog.alert({ content: "要先把这个页面「添加到主屏幕」，从主屏幕打开，才能收到提醒。可以请家里人帮忙设置一下。", confirmText: "知道了" });
      return;
    }
    setEnabling(true);
    try {
      await enablePush();
      setPush("on");
      Toast.show({ content: "打开了，到点手机会响" });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setEnabling(false);
    }
  };

  const signOut = async () => {
    const confirmed = await Dialog.confirm({ content: "要退出登录吗？退出后要重新输入密码。", confirmText: "退出", cancelText: "不退出" });
    if (!confirmed) return;
    await logout();
    router.replace("/login");
  };

  const name = me?.displayName || "您";

  return (
    <div className="el-home">
      <header className="el-home-head">
        <div className="el-home-date">{todayLabel()}</div>
        <h1 className="el-home-hello">{name}，{greeting()}</h1>
      </header>

      <div className="el-cards">
        {elderModes.map((item) => {
          const Icon = item.icon;
          return (
            <button key={item.mode} className="el-card" data-tone={item.tone} onClick={() => router.push(`/elder/${item.mode}`)}>
              <span className="el-card-icon"><Icon size={40} strokeWidth={2.1} /></span>
              <span className="el-card-text">
                <b>{item.title}</b>
                <small>{item.hint}</small>
              </span>
              <ChevronRight size={32} strokeWidth={2.4} className="el-card-go" />
            </button>
          );
        })}
        {/* 宝宝吃饭不是对话：看今天的计划，喂完拍照、说一句记下来 */}
        <button className="el-card" data-tone="amber" onClick={() => router.push("/elder/diet")}>
          <span className="el-card-icon"><Utensils size={40} strokeWidth={2.1} /></span>
          <span className="el-card-text">
            <b>宝宝吃饭</b>
            <small>看今天吃什么，喂完拍张照、说一句</small>
          </span>
          <ChevronRight size={32} strokeWidth={2.4} className="el-card-go" />
        </button>
      </div>

      {(push === "off" || push === "denied") && (
        <button className="el-push" onClick={turnOnPush} disabled={enabling || push === "denied"}>
          <BellRing size={28} strokeWidth={2.2} />
          <span>{push === "denied" ? "提醒通知被关掉了，请家里人在手机设置里打开" : enabling ? "正在打开…" : "点这里打开提醒，到点手机会响"}</span>
        </button>
      )}

      <button className="el-signout" onClick={signOut}>退出登录</button>
    </div>
  );
}
