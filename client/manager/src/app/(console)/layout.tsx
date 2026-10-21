"use client";

import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { App, Button, Drawer, Dropdown, Form, Input, Modal, Result, Spin } from "antd";
import {
  Bell,
  BookHeart,
  CalendarCheck,
  Bot,
  BookOpenCheck,
  ChevronsUpDown,
  ShieldCheck,
  Cpu,
  FileText,
  Home,
  IdCard,
  KeyRound,
  LogOut,
  Menu,
  MessagesSquare,
  Network,
  UserCog,
  Users,
  Utensils,
  Shirt,
} from "lucide-react";
import { api, ApiError, getToken, setToken } from "@/lib/api";
import { withBase } from "@/lib/base-path";
import type { Paged, Person, Place, Principal } from "@/lib/types";
import { ConsoleContext, type ConsoleContextValue } from "@/components/console";

const navGroups = [
  {
    label: "记忆",
    items: [
      { key: "/memories", icon: BookHeart, label: "记忆库" },
      { key: "/notes", icon: FileText, label: "原始记录" },
      { key: "/insights", icon: CalendarCheck, label: "每周整理" },
    ],
  },
  {
    label: "家庭",
    items: [
      { key: "/persons", icon: Users, label: "家庭成员" },
      { key: "/relations", icon: Network, label: "人物关系" },
      { key: "/personas", icon: IdCard, label: "人设卡" },
      { key: "/places", icon: Home, label: "几个家" },
      { key: "/diet", icon: Utensils, label: "宝宝饮食" },
      { key: "/weather", icon: Shirt, label: "宝宝穿衣" },
    ],
  },
  {
    label: "助理",
    items: [
      { key: "/bots", icon: Bot, label: "机器人" },
      { key: "/skills", icon: BookOpenCheck, label: "做事手册" },
      { key: "/rules", icon: ShieldCheck, label: "对话规则" },
      { key: "/conversations", icon: MessagesSquare, label: "对话记录" },
      { key: "/reminders", icon: Bell, label: "提醒" },
    ],
  },
  {
    label: "系统",
    items: [
      { key: "/models", icon: Cpu, label: "模型" },
      { key: "/accounts", icon: UserCog, label: "账号" },
    ],
  },
];

export default function ConsoleLayout({ children }: { children: ReactNode }) {
  return (
    <App>
      <ConsoleShell>{children}</ConsoleShell>
    </App>
  );
}

function ConsoleShell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [me, setMe] = useState<Principal | null>(null);
  const [persons, setPersons] = useState<Person[]>([]);
  const [places, setPlaces] = useState<Place[]>([]);
  const [pending, setPending] = useState(0);
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [navOpen, setNavOpen] = useState(false);
  const [bootError, setBootError] = useState("");

  const reloadFamily = useCallback(async () => {
    const [personList, placeList] = await Promise.all([api.get<Person[]>("/persons"), api.get<Place[]>("/places")]);
    setPersons(personList);
    setPlaces(placeList);
  }, []);

  const boot = useCallback(() => {
    if (!getToken()) {
      router.replace("/login");
      return;
    }
    setBootError("");
    // 401 由 request 统一跳登录页；其它错误（后端没起来、网络断了）留在这里给个重试
    Promise.all([api.get<Principal>("/auth/me"), reloadFamily()])
      .then(([principal]) => setMe(principal))
      .catch((error) => {
        if (!(error instanceof ApiError && error.code === 401)) setBootError((error as Error).message);
      });
  }, [router, reloadFamily]);
  useEffect(boot, [boot]);

  // 待确认的记忆数挂在导航上：切页面、回到这个标签页、页面里改了记忆状态时刷新
  const refreshPending = useCallback(() => {
    if (!getToken()) return;
    api.get<Paged<unknown>>("/memories?statuses=pending&size=1").then((page) => setPending(page.total)).catch(() => {});
  }, []);
  useEffect(() => {
    refreshPending();
    setNavOpen(false);
  }, [pathname, refreshPending]);
  useEffect(() => {
    const onVisible = () => document.visibilityState === "visible" && refreshPending();
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [refreshPending]);

  const value = useMemo<ConsoleContextValue | null>(() => {
    if (!me) return null;
    const personMap = new Map(persons.map((p) => [p.personKey, p]));
    const placeMap = new Map(places.map((p) => [p.placeKey, p]));
    return {
      me,
      persons,
      places,
      reloadFamily,
      refreshPending,
      personName: (key) => {
        const person = personMap.get(key);
        if (!person) return key || "—";
        return person.name && person.name !== person.roleLabel ? `${person.roleLabel}·${person.name}` : person.roleLabel;
      },
      placeName: (key) => placeMap.get(key)?.name || key || "—",
    };
  }, [me, persons, places, reloadFamily, refreshPending]);

  if (!value && bootError) {
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <Result status="warning" title="管理端没加载出来" subTitle={bootError} extra={<Button type="primary" onClick={boot}>重试</Button>} />
      </div>
    );
  }

  if (!value) {
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <Spin />
      </div>
    );
  }

  const logout = async () => {
    try {
      await api.post("/auth/logout");
    } finally {
      setToken("");
      router.replace("/login");
    }
  };

  const sidebar = (
    <>
      <div className="pa-brand">
        <img src={withBase("/icon.png")} alt="" />
        <div>
          <div className="pa-brand-name">家庭助理</div>
          <div className="pa-brand-sub">管理端</div>
        </div>
      </div>
      <nav className="pa-nav">
        {navGroups.map((group) => (
          <div key={group.label} className="pa-nav-group">
            <div className="pa-nav-label">{group.label}</div>
            {group.items.map((item) => {
              const Icon = item.icon;
              const active = pathname.startsWith(item.key);
              return (
                <Link key={item.key} href={item.key} className="pa-nav-item" data-active={active}>
                  <Icon size={17} strokeWidth={active ? 2.1 : 1.8} />
                  {item.label}
                  {item.key === "/memories" && pending > 0 && <span className="pa-nav-badge">{pending}</span>}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>
      <Dropdown
        trigger={["click"]}
        placement="topLeft"
        menu={{
          items: [
            { key: "password", icon: <KeyRound size={15} />, label: "修改密码", onClick: () => setPasswordOpen(true) },
            { type: "divider" },
            { key: "logout", icon: <LogOut size={15} />, label: "退出登录", danger: true, onClick: logout },
          ],
        }}
      >
        <div className="pa-user">
          <div className="pa-user-avatar">{Array.from(value.me.displayName || value.me.username)[0]}</div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="pa-user-name">{value.me.displayName || value.me.username}</div>
            <div className="pa-user-role">{value.me.role === "admin" ? "管理员" : "成员"}</div>
          </div>
          <ChevronsUpDown size={15} color="var(--pa-text-3)" />
        </div>
      </Dropdown>
    </>
  );

  return (
    <ConsoleContext.Provider value={value}>
      <div className="pa-shell">
        <aside className="pa-sidebar">{sidebar}</aside>
        <main className="pa-main">
          <div className="pa-topbar">
            <Button type="text" icon={<Menu size={20} />} onClick={() => setNavOpen(true)} aria-label="菜单" />
            <img src={withBase("/icon.png")} alt="" width={26} height={26} style={{ borderRadius: 7 }} />
            <span style={{ fontFamily: "var(--pa-font-display)", fontWeight: 700, fontSize: 16 }}>家庭助理</span>
          </div>
          <div className="pa-content" key={pathname}>{children}</div>
        </main>
      </div>
      <Drawer placement="left" width={264} open={navOpen} onClose={() => setNavOpen(false)} closable={false} className="pa-drawer-nav">
        {sidebar}
      </Drawer>
      <PasswordModal open={passwordOpen} onClose={() => setPasswordOpen(false)} onDone={logout} />
    </ConsoleContext.Provider>
  );
}

function PasswordModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const [form] = Form.useForm();
  const { message } = App.useApp();
  const submit = async () => {
    const values = await form.validateFields();
    try {
      await api.post("/auth/password", values);
      message.success("密码已修改，请重新登录");
      onDone();
    } catch (error) {
      message.error((error as Error).message);
    }
  };
  return (
    <Modal title="修改密码" open={open} onCancel={onClose} onOk={submit} okText="保存" destroyOnClose width={420}>
      <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
        <Form.Item name="oldPassword" label="原密码" rules={[{ required: true, message: "请输入原密码" }]}>
          <Input.Password />
        </Form.Item>
        <Form.Item name="newPassword" label="新密码" rules={[{ required: true, min: 8, message: "至少 8 位" }]}>
          <Input.Password />
        </Form.Item>
      </Form>
    </Modal>
  );
}
