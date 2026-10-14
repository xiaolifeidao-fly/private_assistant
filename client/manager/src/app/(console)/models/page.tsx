"use client";

import { useState } from "react";
import dayjs from "dayjs";
import { App, Button, Col, Form, Input, InputNumber, Modal, Popconfirm, Row, Select, Switch, Table, Tabs, Tooltip } from "antd";
import { FlaskConical, Pencil, Plus, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import { taskLabel } from "@/lib/labels";
import type { LlmCall, Paged, Provider, RouteConfig } from "@/lib/types";
import { Callout, PageHeader, Panel, Pill, Status } from "@/components/ui";

export default function ModelsPage() {
  return (
    <>
      <PageHeader
        eyebrow="系统"
        title="模型"
        description="每类任务走哪个提供商的哪个模型。Claude 用 anthropic 协议；DeepSeek、通义、豆包、Ollama 和向量化服务用 openai 兼容协议。"
      />
      <Panel>
        <Tabs
          destroyInactiveTabPane
          items={[
            { key: "routes", label: "任务路由", children: <Routes /> },
            { key: "providers", label: "提供商", children: <Providers /> },
            { key: "calls", label: "调用日志", children: <Calls /> },
          ]}
        />
      </Panel>
    </>
  );
}

function Providers() {
  const { message } = App.useApp();
  const { data, loading, reload } = useData(() => api.get<Provider[]>("/llm/providers"), []);
  const [editing, setEditing] = useState<Provider | "new" | null>(null);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  const open = (provider: Provider | "new") => {
    setEditing(provider);
    form.resetFields();
    form.setFieldsValue(provider === "new" ? { kind: "openai", enabled: true } : { ...provider, apiKey: "" });
  };
  const submit = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await api.post("/llm/providers", values);
      message.success("已保存");
      setEditing(null);
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };
  const remove = async (provider: Provider) => {
    try {
      await api.del(`/llm/providers/${provider.providerKey}`);
      message.success("已删除");
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  return (
    <>
      <div className="pa-toolbar">
        <span style={{ color: "var(--pa-text-3)", fontSize: 13 }}>API Key 只显示脱敏后的值，编辑时留空表示不修改</span>
        <div className="pa-toolbar-spacer" />
        <Button type="primary" icon={<Plus size={15} />} className="pa-icon-text" onClick={() => open("new")}>添加提供商</Button>
      </div>
      <Table<Provider>
        rowKey="providerKey"
        scroll={{ x: 860 }}
        loading={loading}
        dataSource={data ?? []}
        pagination={false}
        columns={[
          {
            title: "提供商",
            render: (_, p) => (
              <div>
                <div style={{ fontWeight: 550 }}>{p.name}</div>
                <div className="pa-mono" style={{ color: "var(--pa-text-3)" }}>{p.providerKey}</div>
              </div>
            ),
          },
          { title: "协议", width: 120, render: (_, p) => <Pill tone={p.kind === "anthropic" ? "clay" : "sky"}>{p.kind}</Pill> },
          { title: "Base URL", render: (_, p) => (p.baseUrl ? <span className="pa-mono">{p.baseUrl}</span> : <span className="pa-muted">官方默认</span>) },
          { title: "API Key", width: 200, render: (_, p) => (p.hasApiKey ? <span className="pa-mono" style={{ color: "var(--pa-text-2)" }}>{p.apiKey}</span> : <Status tone="rose">未配置</Status>) },
          { title: "状态", width: 90, render: (_, p) => <Status tone={p.enabled ? "sage" : "slate"}>{p.enabled ? "启用" : "停用"}</Status> },
          {
            title: "",
            width: 90,
            align: "right",
            render: (_, p) => (
              <div style={{ display: "inline-flex", gap: 2 }}>
                <Tooltip title="编辑"><Button size="small" type="text" icon={<Pencil size={14} />} onClick={() => open(p)} /></Tooltip>
                <Popconfirm title="删除这个提供商？" onConfirm={() => remove(p)} okText="删除" okButtonProps={{ danger: true }}>
                  <Button size="small" type="text" danger icon={<Trash2 size={14} />} />
                </Popconfirm>
              </div>
            ),
          },
        ]}
      />
      <Modal title={editing === "new" ? "添加提供商" : "编辑提供商"} open={!!editing} onCancel={() => setEditing(null)} onOk={submit} confirmLoading={saving} okText="保存" destroyOnClose>
        <Form form={form} layout="vertical" requiredMark={false} style={{ marginTop: 16 }}>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item name="providerKey" label="标识" rules={[{ required: true, message: "请输入标识" }]} extra="路由里引用的名字">
                <Input disabled={editing !== "new"} placeholder="anthropic、deepseek" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}><Input /></Form.Item>
            </Col>
          </Row>
          <Form.Item name="kind" label="协议" rules={[{ required: true }]}>
            <Select options={[{ value: "anthropic", label: "anthropic（Claude）" }, { value: "openai", label: "openai（OpenAI 兼容）" }]} />
          </Form.Item>
          <Form.Item name="baseUrl" label="Base URL" extra="anthropic 留空即官方地址；openai 兼容协议必填，如 https://api.deepseek.com/v1">
            <Input />
          </Form.Item>
          <Form.Item name="apiKey" label="API Key" extra={editing !== "new" ? "留空表示不修改" : undefined}>
            <Input.Password autoComplete="off" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked"><Switch /></Form.Item>
        </Form>
      </Modal>
    </>
  );
}

function Routes() {
  const { message } = App.useApp();
  const { data, loading, reload } = useData(() => api.get<RouteConfig[]>("/llm/routes"), []);
  const { data: providers } = useData(() => api.get<Provider[]>("/llm/providers"), []);
  const [editing, setEditing] = useState<RouteConfig | null>(null);
  const [testing, setTesting] = useState("");
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  const submit = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await api.put(`/llm/routes/${editing!.task}`, { ...values, temperature: values.temperature ?? null });
      message.success("已保存");
      setEditing(null);
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };
  const test = async (task: string) => {
    setTesting(task);
    try {
      const result = await api.post<{ result: string }>(`/llm/routes/${task}/test`);
      message.success(result.result, 6);
    } catch (error) {
      message.error((error as Error).message, 8);
    } finally {
      setTesting("");
    }
  };

  return (
    <>
      <Table<RouteConfig>
        rowKey="task"
        scroll={{ x: 860 }}
        loading={loading}
        dataSource={data ?? []}
        pagination={false}
        columns={[
          {
            title: "任务",
            render: (_, r) => (
              <div>
                <div style={{ fontWeight: 550 }}>{taskLabel[r.task] ?? r.task}</div>
                <div className="pa-mono" style={{ color: "var(--pa-text-3)" }}>{r.task}</div>
              </div>
            ),
          },
          { title: "提供商", dataIndex: "providerKey", width: 140, render: (v) => <span className="pa-mono">{v}</span> },
          { title: "模型", dataIndex: "model", render: (v) => <Pill tone="clay"><span className="pa-mono" style={{ fontSize: 12 }}>{v}</span></Pill> },
          { title: "max_tokens", dataIndex: "maxTokens", width: 110, render: (v) => <span className="pa-num">{v || "—"}</span> },
          { title: "effort", dataIndex: "effort", width: 90, render: (v) => v || <span className="pa-muted">—</span> },
          {
            title: "",
            width: 150,
            align: "right",
            render: (_, r) => (
              <div style={{ display: "inline-flex", gap: 4 }}>
                <Button size="small" icon={<FlaskConical size={14} />} className="pa-icon-text" loading={testing === r.task} onClick={() => test(r.task)}>测试</Button>
                <Tooltip title="编辑"><Button size="small" type="text" icon={<Pencil size={14} />} onClick={() => { setEditing(r); form.setFieldsValue(r); }} /></Tooltip>
              </div>
            ),
          },
        ]}
      />
      <Modal title={`编辑路由 · ${editing ? taskLabel[editing.task] ?? editing.task : ""}`} open={!!editing} onCancel={() => setEditing(null)} onOk={submit} confirmLoading={saving} okText="保存" destroyOnClose>
        <Form form={form} layout="vertical" requiredMark={false} style={{ marginTop: 16 }}>
          <Form.Item name="providerKey" label="提供商" rules={[{ required: true }]}>
            <Select options={(providers ?? []).map((p) => ({ value: p.providerKey, label: `${p.name}（${p.providerKey}）` }))} />
          </Form.Item>
          <Form.Item name="model" label="模型" rules={[{ required: true }]} extra="Claude：claude-opus-5-5 / claude-sonnet-5-5 / claude-haiku-4-5">
            <Input className="pa-mono" />
          </Form.Item>
          <Row gutter={12}>
            <Col span={8}><Form.Item name="maxTokens" label="max_tokens"><InputNumber min={0} step={1000} style={{ width: "100%" }} /></Form.Item></Col>
            <Col span={8}>
              <Form.Item name="effort" label="effort（Claude）">
                <Select allowClear options={["low", "medium", "high", "xhigh", "max"].map((v) => ({ value: v, label: v }))} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="temperature" label="temperature" tooltip="Claude 新模型不支持，留空">
                <InputNumber min={0} max={2} step={0.1} style={{ width: "100%" }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="description" label="说明"><Input /></Form.Item>
        </Form>
      </Modal>
    </>
  );
}

function Calls() {
  const [page, setPage] = useState(1);
  const { data, loading } = useData(() => api.get<Paged<LlmCall>>(`/llm/calls?page=${page}&size=50`), [page]);
  return (
    <>
      <div style={{ padding: "16px 20px 0" }}>
        <Callout tone="sky">缓存读越高越省钱：同一段对话的系统提示词和历史消息会命中 Claude 的提示词缓存。</Callout>
      </div>
      <Table<LlmCall>
        rowKey="callKey"
        scroll={{ x: 860 }}
        size="small"
        loading={loading}
        dataSource={data?.items ?? []}
        style={{ marginTop: 8 }}
        pagination={{ current: page, pageSize: 50, total: data?.total ?? 0, onChange: setPage, hideOnSinglePage: true, showSizeChanger: false }}
        columns={[
          { title: "时间", width: 140, render: (_, c) => <span className="pa-num">{dayjs(c.createdAt).format("MM-DD HH:mm:ss")}</span> },
          { title: "任务", render: (_, c) => taskLabel[c.task] ?? c.task },
          { title: "模型", dataIndex: "model", render: (v) => <span className="pa-mono">{v}</span> },
          { title: "输入", dataIndex: "inputTokens", align: "right", render: (v) => <span className="pa-num">{v}</span> },
          { title: "缓存读", dataIndex: "cacheReadTokens", align: "right", render: (v) => <span className="pa-num" style={{ color: v ? "var(--pa-success)" : undefined }}>{v}</span> },
          { title: "缓存写", dataIndex: "cacheWriteTokens", align: "right", render: (v) => <span className="pa-num">{v}</span> },
          { title: "输出", dataIndex: "outputTokens", align: "right", render: (v) => <span className="pa-num">{v}</span> },
          { title: "耗时", align: "right", render: (_, c) => <span className="pa-num">{(c.latencyMs / 1000).toFixed(1)}s</span> },
          { title: "结束", dataIndex: "stopReason", render: (v) => <span className="pa-mono pa-muted">{v}</span> },
          {
            title: "状态",
            width: 90,
            render: (_, c) => (c.status === "ok" ? <Status tone="sage">成功</Status> : <Tooltip title={c.error}><span><Status tone="rose">失败</Status></span></Tooltip>),
          },
        ]}
      />
    </>
  );
}
