"use client";

import { useEffect, useState } from "react";
import { App, Button, Checkbox, Col, Drawer, Form, Input, InputNumber, Radio, Row, Select, Spin, Switch } from "antd";
import { Cpu, Database, RotateCcw, Wrench } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import { taskLabel, toolLabel } from "@/lib/labels";
import { botTone } from "@/lib/tone";
import type { Bot, RouteConfig } from "@/lib/types";
import { useConsole } from "@/components/console";
import { Avatar, PageHeader, Pill, Status } from "@/components/ui";

export default function BotsPage() {
  const { personName } = useConsole();
  const { data, loading, reload } = useData(() => api.get<Bot[]>("/bots"), []);
  const { data: routes } = useData(() => api.get<RouteConfig[]>("/llm/routes"), []);
  const [editing, setEditing] = useState<Bot | null>(null);
  const modelOf = (task: string) => routes?.find((r) => r.task === task)?.model;

  return (
    <>
      <PageHeader
        eyebrow="助理"
        title="机器人"
        description="提示词只写「它是谁、怎么做事」；家庭档案和记忆会在每次新对话开始时自动拼进去。改了提示词只对之后新建的对话生效，进行中的对话保持原样，保证上下文前后一致。"
      />
      {loading && !data ? <div style={{ padding: 60, textAlign: "center" }}><Spin /></div> : (
        <div className="pa-grid" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(320px, 1fr))" }}>
          {(data ?? []).map((bot) => (
            <div key={bot.botKey} className="pa-tile" data-muted={!bot.enabled} onClick={() => setEditing(bot)}>
              <div style={{ display: "flex", alignItems: "flex-start", gap: 14 }}>
                <Avatar tone={botTone(bot.botKey)} size={52} square>{bot.avatar}</Avatar>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontFamily: "var(--pa-font-display)", fontSize: 19, fontWeight: 700 }}>{bot.name}</div>
                  <div style={{ marginTop: 6, display: "flex", gap: 6, flexWrap: "wrap" }}>
                    {bot.kind === "assistant" ? <Pill tone="clay">私人助理</Pill> : <Pill tone={botTone(bot.botKey)}>分身 · {personName(bot.personKey)}</Pill>}
                  </div>
                </div>
                <Status tone={bot.enabled ? "sage" : "slate"}>{bot.enabled ? "启用" : "停用"}</Status>
              </div>
              <div style={{ fontSize: 13.5, color: "var(--pa-text-2)", lineHeight: 1.65, marginTop: 14, minHeight: 44 }}>{bot.description}</div>
              <div style={{ flex: 1 }} />
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8, marginTop: 16, paddingTop: 14, borderTop: "1px solid var(--pa-line)", fontSize: 12.5, color: "var(--pa-text-3)" }}>
                <span style={{ display: "inline-flex", alignItems: "center", gap: 6, minWidth: 0 }}>
                  <Cpu size={13} style={{ flexShrink: 0 }} />
                  <span style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }} className="pa-mono">{modelOf(bot.routeTask) ?? taskLabel[bot.routeTask] ?? bot.routeTask}</span>
                </span>
                <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                  <Database size={13} />{bot.memoryScope === "all" ? "全部记忆" : "仅本人 + 公共"}
                </span>
                <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                  <Wrench size={13} />{bot.tools.length} 个工具
                </span>
                <span className="pa-num">提示词 v{bot.version}</span>
              </div>
            </div>
          ))}
        </div>
      )}
      <BotEditor bot={editing} routes={routes ?? []} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); void reload(); }} />
    </>
  );
}

function BotEditor({ bot, routes, onClose, onSaved }: { bot: Bot | null; routes: RouteConfig[]; onClose: () => void; onSaved: () => void }) {
  const { message, modal } = App.useApp();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (bot) {
      form.resetFields();
      form.setFieldsValue(bot);
    }
  }, [bot, form]);
  if (!bot) return null;

  const submit = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await api.put(`/bots/${bot.botKey}`, { ...bot, ...values });
      message.success("已保存，新对话开始生效");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const restore = () => {
    modal.confirm({
      title: "恢复出厂提示词？",
      content: "当前编辑框里的提示词会被替换，保存后才生效。",
      okText: "恢复",
      onOk: async () => {
        const result = await api.get<{ systemPrompt: string }>(`/bots/${bot.botKey}/default-prompt`);
        form.setFieldValue("systemPrompt", result.systemPrompt);
      },
    });
  };

  return (
    <Drawer
      title={
        <span style={{ display: "inline-flex", alignItems: "center", gap: 10 }}>
          <Avatar tone={botTone(bot.botKey)} size={28} square>{bot.avatar}</Avatar>
          {bot.name}
        </span>
      }
      width={760}
      open
      onClose={onClose}
      destroyOnClose
      footer={
        <div style={{ display: "flex", gap: 8 }}>
          <Button icon={<RotateCcw size={15} />} className="pa-icon-text" onClick={restore}>恢复默认提示词</Button>
          <div style={{ flex: 1 }} />
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={saving} onClick={submit}>保存</Button>
        </div>
      }
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        <Row gutter={16}>
          <Col span={4}><Form.Item name="avatar" label="头像"><Input style={{ textAlign: "center", fontSize: 18 }} /></Form.Item></Col>
          <Col span={10}><Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}><Input /></Form.Item></Col>
          <Col span={5}><Form.Item name="sort" label="排序"><InputNumber style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={5}><Form.Item name="enabled" label="启用" valuePropName="checked"><Switch /></Form.Item></Col>
        </Row>
        <Form.Item name="description" label="简介"><Input /></Form.Item>
        <Form.Item name="greeting" label="开场白"><Input.TextArea autoSize={{ minRows: 2 }} /></Form.Item>
        <Form.Item name="systemPrompt" label="系统提示词" rules={[{ required: true, message: "提示词不能为空" }]} extra="分身提示词里可用 {{person_name}}、{{person_role}}">
          <Input.TextArea autoSize={{ minRows: 14, maxRows: 32 }} className="pa-mono" style={{ fontSize: 12.5, lineHeight: 1.7 }} />
        </Form.Item>
        <Row gutter={16}>
          <Col span={14}>
            <Form.Item name="routeTask" label="模型路由" rules={[{ required: true }]}>
              <Select options={routes.filter((r) => r.task !== "embedding").map((r) => ({ value: r.task, label: `${taskLabel[r.task] ?? r.task}（${r.model}）` }))} />
            </Form.Item>
          </Col>
          <Col span={10}>
            <Form.Item name="memoryScope" label="记忆范围">
              <Radio.Group optionType="button" options={[{ value: "all", label: "全部" }, { value: "person", label: "仅本人 + 公共" }]} />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="tools" label="可用工具">
          <Checkbox.Group style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 10 }} options={Object.entries(toolLabel).map(([value, label]) => ({ value, label }))} />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
