"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Toast } from "antd-mobile";
import { CalendarDays, MessageCircle, PenLine, Users } from "lucide-react";
import { AuthGate } from "@/components/auth-gate";
import { Welcome } from "@/components/welcome";
import { api } from "@/lib/api";
import { onPendingChanged, useOnResume } from "@/lib/events";
import { refreshFamily } from "@/lib/family";
import { markWelcomeSeen, onGuideReset, welcomeSeen } from "@/lib/guide";
import { flushOutbox } from "@/lib/outbox";

const tabs = [
  { key: "/", title: "对话", icon: MessageCircle },
  { key: "/note", title: "速记", icon: PenLine },
  { key: "/today", title: "今天", icon: CalendarDays },
  { key: "/family", title: "家人", icon: Users },
];

export default function TabsLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGate>
      <TabsShell>{children}</TabsShell>
    </AuthGate>
  );
}

function TabsShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [pending, setPending] = useState(0);
  const [welcome, setWelcome] = useState(false);
  const [scrolled, setScrolled] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  // 第一次进来先看欢迎引导；「家人 → 使用指南」可以重看
  useEffect(() => {
    setWelcome(!welcomeSeen());
    return onGuideReset(() => setWelcome(true));
  }, []);

  const refreshPending = useCallback(() => {
    api.get<{ count: number }>("/memories/pending-count").then((r) => setPending(r.count)).catch(() => {});
  }, []);
  useEffect(refreshPending, [pathname, refreshPending]);
  useEffect(() => onPendingChanged(refreshPending), [refreshPending]);

  // 断网时暂存的速记：打开 App、恢复联网、从后台切回来时补交
  const flush = useCallback(
    () =>
      flushOutbox().then((sent) => {
        if (sent > 0) Toast.show({ content: `补交了 ${sent} 条离线速记` });
      }),
    [],
  );
  useEffect(() => {
    void flush();
    window.addEventListener("online", flush);
    return () => window.removeEventListener("online", flush);
  }, [flush]);
  useOnResume(() => {
    void flush();
    refreshPending();
    void refreshFamily();
  });

  // 预取其它标签页，切换时不用等编译 / 下载
  useEffect(() => {
    tabs.forEach((tab) => router.prefetch(tab.key));
  }, [router]);

  useEffect(() => setScrolled(false), [pathname]);
  const onScroll = () => {
    const element = scrollRef.current;
    if (element) setScrolled(element.scrollTop > 64);
  };

  const current = tabs.find((tab) => tab.key === pathname);

  return (
    <div className="pa-screen">
      <div className="pa-minihead" data-show={scrolled} aria-hidden={!scrolled}>
        <div className="pa-minihead-inner">{current?.title}</div>
      </div>
      <div className="pa-scroll pa-tab-scroll" key={pathname} ref={scrollRef} onScroll={onScroll}>
        <div className="pa-enter">{children}</div>
      </div>
      <nav className="pa-tabbar">
        <div className="pa-tabbar-inner">
          {tabs.map((tab) => {
            const active = pathname === tab.key;
            const Icon = tab.icon;
            return (
              <button
                key={tab.key}
                className="pa-tab"
                data-active={active}
                aria-current={active ? "page" : undefined}
                // 已经在这一页时再点一下，回到顶部
                onClick={() => (active ? scrollRef.current?.scrollTo({ top: 0, behavior: "smooth" }) : router.push(tab.key))}
              >
                <span className="pa-tab-icon">
                  <Icon size={22} strokeWidth={active ? 2.2 : 1.8} />
                  {tab.key === "/today" && pending > 0 && <span className="pa-tab-badge">{pending > 99 ? "99+" : pending}</span>}
                </span>
                <span>{tab.title}</span>
              </button>
            );
          })}
        </div>
      </nav>
      {welcome && (
        <Welcome
          onDone={() => {
            markWelcomeSeen();
            setWelcome(false);
          }}
        />
      )}
    </div>
  );
}
