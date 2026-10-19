"use client";

import { useState } from "react";
import dayjs from "dayjs";
import { App, Button, Form, Input, Modal, Popconfirm, Select, Table, Tabs, Tooltip } from "antd";
import { Lock, Pencil, Plus, Power } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import type { Account } from "@/lib/types";
import { useConsole } from "@/components/console";
import { Avatar, Callout, PageHeader, Panel, PersonTag, Pill, Status } from "@/components/ui";

export default function AccountsPage() {
  const { me } = useConsole();
  return (
    <>
      <PageHeader
        eyebrow="系统"
        title="账号"
        description="手机端账号用于日常对话和速记，需要对应到家里的某个人；管理端账号用于维护档案和配置。"
      />
      {me.role !== "admin" ? (
        <Callout tone="amber" icon={<Lock size={15} />}>只有管理员可以管理账号。</Callout>
      ) : (
        <Panel>
          <Tabs
            items={[
              { key: "app", label: "手机端账号", children: <AccountTable realm="app" /> },
              { key: "manager", label: "管理端账号", children: <AccountTable realm="manager" /> },
            ]}
          />
        </Panel>
      )}
    </>
  );
}

const managerRoles = [{ value: "member", label: "成员" }, { value: "admin", label: "管理员" }];
// 手机端的角色决定界面和能用的机器人（服务端 identity/role.go 同步限制）
const appRoles = [
  { value: "admin", label: "我自己（全部功能）" },
  { value: "member", label: "成员（全部功能）" },
  { value: "spouse", label: "老婆（万能通、提醒）" },
  { value: "elder", label: "奶奶 · 长辈模式（大字、语音）" },
];

function RolePill({ role }: { role: string }) {
  if (role === "admin") return <Pill tone="clay">管理员</Pill>;
  if (role === "spouse") return <Pill tone="rose">老婆</Pill>;
  if (role === "elder") return <Pill tone="amber">长辈模式</Pill>;
  return <Pill>成员</Pill>;
}

function AccountTable({ realm }: { realm: "app" | "manager" }) {
  const { me, persons, personName } = useConsole();
  const { message } = App.useApp();
  const { data, loading, error, reload } = useData(() => api.get<Account[]>(`/accounts?realm=${realm}`), [realm], { silent: true });
  const [editing, setEditing] = useState<Account | "new" | null>(null);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  const open = (account: Account | "new") => {
    setEditing(account);
    form.resetFields();
    form.setFieldsValue(account === "new" ? { role: "member", personKey: realm === "app" ? "me" : undefined } : { ...account, password: "" });
  };
  const submit = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      if (editing === "new") await api.post("/accounts", { ...values, realm });
      else if (editing) await api.put(`/accounts/${editing.accountKey}`, values);
      message.success("已保存");
      setEditing(null);
      void reload();
    } catch (err) {
      message.error((err as Error).message);
    } finally {
      setSaving(false);
    }
  };
  const toggle = async (account: Account) => {
    try {
      await api.put(`/accounts/${account.accountKey}`, { ...account, status: account.status === "active" ? "disabled" : "active", password: "" });
      message.success(account.status === "active" ? "已停用" : "已启用");
      void reload();
    } catch (err) {
      message.error((err as Error).message);
    }
  };

  return (
    <>
      <div className="pa-toolbar">
        {error && <Callout tone="rose">{error}</Callout>}
        <div className="pa-toolbar-spacer" />
        <Button type="primary" icon={<Plus size={15} />} className="pa-icon-text" onClick={() => open("new")}>新建账号</Button>
      </div>
      <Table<Account>
        rowKey="accountKey"
        scroll={{ x: 760 }}
        loading={loading}
        dataSource={data ?? []}
        pagination={false}
        columns={[
          {
            title: "账号",
            render: (_, a) => (
              <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <Avatar tone={a.role === "admin" ? "clay" : "slate"} size={34}>{Array.from(a.displayName || a.username)[0]}</Avatar>
                <div>
                  <div style={{ fontWeight: 550, display: "flex", alignItems: "center", gap: 8 }}>
                    {a.displayName || a.username}
                    {a.accountKey === me.accountKey && <Pill tone="sky">我</Pill>}
                  </div>
                  <div className="pa-mono" style={{ color: "var(--pa-text-3)" }}>{a.username}</div>
                </div>
              </div>
            ),
          },
          { title: "角色", width: 110, render: (_, a) => <RolePill role={a.role} /> },
          ...(realm === "app" ? [{ title: "对应家人", width: 180, render: (_: unknown, a: Account) => (a.personKey ? <PersonTag personKey={a.personKey} size={22} /> : <span className="pa-muted">—</span>) }] : []),
          { title: "状态", width: 100, render: (_, a) => <Status tone={a.status === "active" ? "sage" : "slate"}>{a.status === "active" ? "正常" : "停用"}</Status> },
          { title: "最近登录", width: 170, render: (_, a) => <span className="pa-num" style={{ color: "var(--pa-text-2)" }}>{a.lastLoginAt ? dayjs(a.lastLoginAt).format("YYYY-MM-DD HH:mm") : "从未登录"}</span> },
          {
            title: "",
            width: 90,
            align: "right",
            render: (_, a) => (
              <div style={{ display: "inline-flex", gap: 2 }}>
                <Tooltip title="编辑"><Button size="small" type="text" icon={<Pencil size={14} />} onClick={() => open(a)} /></Tooltip>
                {a.accountKey !== me.accountKey && (
                  a.status === "active" ? (
                    <Popconfirm title="停用这个账号？" description="停用后这个账号马上不能登录。" onConfirm={() => toggle(a)} okText="停用" okButtonProps={{ danger: true }}>
                      <Tooltip title="停用"><Button size="small" type="text" danger icon={<Power size={14} />} /></Tooltip>
                    </Popconfirm>
                  ) : (
                    <Tooltip title="启用"><Button size="small" type="text" icon={<Power size={14} />} onClick={() => toggle(a)} /></Tooltip>
                  )
                )}
              </div>
            ),
          },
        ]}
      />
      <Modal title={editing === "new" ? "新建账号" : "编辑账号"} open={!!editing} onCancel={() => setEditing(null)} onOk={submit} confirmLoading={saving} okText="保存" destroyOnClose>
        <Form form={form} layout="vertical" requiredMark={false} style={{ marginTop: 16 }}>
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: "请输入用户名" }]}>
            <Input disabled={editing !== "new"} autoComplete="off" />
          </Form.Item>
          <Form.Item name="displayName" label="显示名"><Input /></Form.Item>
          <Form.Item name="password" label={editing === "new" ? "密码" : "重置密码"} rules={editing === "new" ? [{ required: true, min: 8, message: "至少 8 位" }] : [{ min: 8, message: "至少 8 位" }]} extra={editing !== "new" ? "留空表示不修改" : undefined}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="role" label="角色" extra={realm === "app" ? "老婆：只有万能通和提醒；奶奶：大字、语音优先，只有万事通、记一下、提醒" : undefined}>
            <Select options={realm === "app" ? appRoles : managerRoles} />
          </Form.Item>
          {realm === "app" && (
            <Form.Item name="personKey" label="对应家里哪个人">
              <Select allowClear options={persons.map((p) => ({ value: p.personKey, label: personName(p.personKey) }))} />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </>
  );
}
