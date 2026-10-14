"use client";

import { useEffect, useMemo, useState } from "react";
import dayjs from "dayjs";
import { App, Button, Col, DatePicker, Drawer, Dropdown, Form, Input, InputNumber, Popconfirm, Rate, Row, Select, Table, Tabs, Timeline, Tooltip } from "antd";
import { Archive, Check, History, MoreHorizontal, Pencil, Plus, RefreshCw, RotateCcw, Search, X } from "lucide-react";
import { api, query } from "@/lib/api";
import { useData } from "@/lib/hooks";
import { createdByLabel, memoryStatus } from "@/lib/labels";
import { typeTone } from "@/lib/tone";
import type { Memory, MemoryType, Paged, SearchHit } from "@/lib/types";
import { useConsole } from "@/components/console";
import { Callout, PageHeader, Panel, PersonTag, Pill, Status } from "@/components/ui";

export default function MemoriesPage() {
  const { refreshPending } = useConsole();
  const [editing, setEditing] = useState<{ memory?: Memory; supersede?: Memory } | null>(null);
  const [version, setVersion] = useState(0);
  const changed = () => {
    setVersion((v) => v + 1);
    refreshPending();
  };
  const { data: pendingPage } = useData(() => api.get<Paged<Memory>>("/memories?statuses=pending&size=1"), [version]);
  const types = useMemoryTypes();

  return (
    <>
      <PageHeader
        eyebrow="记忆"
        title="记忆库"
        description="她说过的话、在意的事、我的承诺和老家的规矩。助理每次回答都从这里检索；改变了的事用「她变了」记录，旧的会保留成演化轨迹。"
        actions={<Button type="primary" icon={<Plus size={16} />} className="pa-icon-text" onClick={() => setEditing({})}>新增记忆</Button>}
      />
      <Panel>
        <Tabs
          destroyInactiveTabPane
          items={[
            { key: "list", label: "全部记忆", children: <MemoryList version={version} onEdit={setEditing} onChanged={changed} /> },
            {
              key: "pending",
              label: <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>待确认{!!pendingPage?.total && <Pill tone="amber">{pendingPage.total}</Pill>}</span>,
              children: <MemoryList version={version} pendingOnly onEdit={setEditing} onChanged={changed} />,
            },
            { key: "search", label: "检索调试", children: <SearchDebug /> },
          ]}
        />
      </Panel>
      <MemoryEditor
        state={editing}
        types={types}
        onClose={() => setEditing(null)}
        onSaved={() => {
          setEditing(null);
          changed();
        }}
      />
    </>
  );
}

function useMemoryTypes() {
  const { data } = useData(() => api.get<MemoryType[]>("/memory-types"), []);
  return data ?? [];
}

function Importance({ value }: { value: number }) {
  return (
    <Tooltip title={`重要度 ${value}`}>
      <span style={{ display: "inline-flex", gap: 3 }}>
        {Array.from({ length: 5 }, (_, i) => (
          <i key={i} style={{ width: 6, height: 6, borderRadius: 3, background: i < value ? "var(--pa-accent)" : "var(--pa-fill-strong)" }} />
        ))}
      </span>
    </Tooltip>
  );
}

function MemoryList({ version, pendingOnly = false, onEdit, onChanged }: {
  version: number;
  pendingOnly?: boolean;
  onEdit: (state: { memory?: Memory; supersede?: Memory }) => void;
  onChanged: () => void;
}) {
  const { persons, personName } = useConsole();
  const { message } = App.useApp();
  const types = useMemoryTypes();
  const [filter, setFilter] = useState({ person: "", types: "", statuses: pendingOnly ? "pending" : "active,pending", keyword: "", page: 1 });
  const [detail, setDetail] = useState<Memory | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [batching, setBatching] = useState(false);

  const { data, loading, reload } = useData(
    () => api.get<Paged<Memory>>(`/memories${query({ ...filter, size: 30 })}`),
    [JSON.stringify(filter), version],
  );

  // 换页、换筛选、数据刷新后清掉勾选，免得批量操作到看不见的行
  useEffect(() => setSelected([]), [data]);

  const setStatus = async (memory: Memory, status: string) => {
    try {
      await api.post(`/memories/${memory.memoryKey}/status`, { status });
      message.success(status === "active" ? "已生效" : status === "rejected" ? "已驳回" : status === "archived" ? "已归档" : "已更新");
      setDetail(null);
      onChanged();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  const setStatusBatch = async (status: "active" | "rejected") => {
    setBatching(true);
    const results = await Promise.allSettled(selected.map((key) => api.post(`/memories/${key}/status`, { status })));
    setBatching(false);
    const failed = results.filter((r) => r.status === "rejected").length;
    const done = results.length - failed;
    if (done) message.success(`${status === "active" ? "确认" : "驳回"}了 ${done} 条`);
    if (failed) message.error(`${failed} 条没有成功，可以再试一次`);
    onChanged();
  };
  const pendingRows = (data?.items ?? []).filter((m) => m.status === "pending");

  return (
    <>
      <div className="pa-toolbar">
        <Input.Search
          allowClear
          placeholder="搜索内容，回车或点放大镜"
          style={{ width: 240 }}
          onSearch={(value) => value.trim() !== filter.keyword && setFilter({ ...filter, keyword: value.trim(), page: 1 })}
          onChange={(e) => !e.target.value && filter.keyword && setFilter({ ...filter, keyword: "", page: 1 })}
        />
        <Select
          allowClear
          placeholder="关于谁"
          style={{ width: 160 }}
          value={filter.person || undefined}
          onChange={(value) => setFilter({ ...filter, person: value ?? "", page: 1 })}
          options={persons.map((p) => ({ value: p.personKey, label: personName(p.personKey) }))}
        />
        <Select
          allowClear
          mode="multiple"
          maxTagCount="responsive"
          placeholder="类型"
          style={{ width: 220 }}
          value={filter.types ? filter.types.split(",") : []}
          onChange={(value: string[]) => setFilter({ ...filter, types: value.join(","), page: 1 })}
          options={types.map((t) => ({ value: t.type, label: t.label }))}
        />
        {!pendingOnly && (
          <Select
            mode="multiple"
            maxTagCount="responsive"
            placeholder="状态"
            style={{ width: 200 }}
            value={filter.statuses ? filter.statuses.split(",") : []}
            onChange={(value: string[]) => setFilter({ ...filter, statuses: value.join(","), page: 1 })}
            options={Object.entries(memoryStatus).map(([value, item]) => ({ value, label: item.label }))}
          />
        )}
        <div className="pa-toolbar-spacer" />
        {selected.length > 0 && (
          <>
            <span className="pa-num" style={{ fontSize: 13 }}>已选 {selected.length} 条</span>
            <Button type="primary" size="small" icon={<Check size={14} />} className="pa-icon-text" loading={batching} onClick={() => setStatusBatch("active")}>批量确认</Button>
            <Popconfirm title={`驳回选中的 ${selected.length} 条？`} onConfirm={() => setStatusBatch("rejected")} okText="驳回" okButtonProps={{ danger: true }}>
              <Button size="small" danger icon={<X size={14} />} className="pa-icon-text" disabled={batching}>批量驳回</Button>
            </Popconfirm>
          </>
        )}
        <span className="pa-muted pa-num" style={{ fontSize: 13 }}>共 {data?.total ?? 0} 条</span>
        <Tooltip title="刷新"><Button icon={<RefreshCw size={15} />} onClick={() => reload()} /></Tooltip>
      </div>
      <Table<Memory>
        rowKey="memoryKey"
        scroll={{ x: 960 }}
        loading={loading}
        dataSource={data?.items ?? []}
        rowSelection={pendingRows.length ? {
          selectedRowKeys: selected,
          onChange: (keys) => setSelected(keys as string[]),
          getCheckboxProps: (memory) => ({ disabled: memory.status !== "pending" }),
          columnWidth: 44,
        } : undefined}
        onRow={(memory) => ({ onClick: () => setDetail(memory), style: { cursor: "pointer" } })}
        pagination={{ current: filter.page, pageSize: 30, total: data?.total ?? 0, showSizeChanger: false, onChange: (page) => setFilter({ ...filter, page }), hideOnSinglePage: true }}
        locale={{ emptyText: pendingOnly ? "没有待确认的记忆" : "没有符合条件的记忆" }}
        columns={[
          {
            title: "内容",
            render: (_, memory) => (
              <div style={{ display: "flex", gap: 12, alignItems: "flex-start" }}>
                <Pill tone={typeTone(memory.type)} style={{ marginTop: 1 }}>{memory.typeLabel}</Pill>
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontWeight: 550, lineHeight: 1.5 }}>{memory.title}</div>
                  {memory.content !== memory.title && (
                    <div style={{ color: "var(--pa-text-3)", fontSize: 13, lineHeight: 1.55, marginTop: 2, display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>
                      {memory.content}
                    </div>
                  )}
                </div>
              </div>
            ),
          },
          { title: "关于", width: 160, render: (_, memory) => <PersonTag personKey={memory.subjectPerson} /> },
          { title: "重要度", width: 96, render: (_, memory) => <Importance value={memory.importance} /> },
          { title: "时间", width: 136, render: (_, memory) => <span className="pa-num" style={{ color: "var(--pa-text-2)" }}>{dayjs(memory.occurredAt ?? memory.createdAt).format("YYYY-MM-DD")}</span> },
          { title: "状态", width: 96, render: (_, memory) => <Status tone={memoryStatus[memory.status]?.tone}>{memoryStatus[memory.status]?.label ?? memory.status}</Status> },
          {
            title: "",
            width: (data?.items ?? []).some((m) => m.status === "pending") ? 200 : 100,
            align: "right",
            render: (_, memory) => (
              <div style={{ display: "inline-flex", gap: 4 }} onClick={(e) => e.stopPropagation()}>
                {memory.status === "pending" && (
                  <>
                    <Button size="small" type="primary" icon={<Check size={14} />} className="pa-icon-text" onClick={() => setStatus(memory, "active")}>确认</Button>
                    <Button size="small" type="text" danger icon={<X size={14} />} className="pa-icon-text" onClick={() => setStatus(memory, "rejected")}>驳回</Button>
                  </>
                )}
                <Tooltip title="编辑"><Button size="small" type="text" icon={<Pencil size={14} />} onClick={() => onEdit({ memory })} /></Tooltip>
                {memory.status === "active" && (
                  <Dropdown
                    trigger={["click"]}
                    menu={{
                      items: [
                        { key: "supersede", icon: <History size={14} />, label: "她变了（记录新情况）", onClick: () => onEdit({ supersede: memory }) },
                        { key: "archive", icon: <Archive size={14} />, label: "归档（不再参与检索）", onClick: () => setStatus(memory, "archived") },
                      ],
                    }}
                  >
                    <Button size="small" type="text" icon={<MoreHorizontal size={15} />} />
                  </Dropdown>
                )}
                {(memory.status === "archived" || memory.status === "rejected") && (
                  <Tooltip title="恢复为生效"><Button size="small" type="text" icon={<RotateCcw size={14} />} onClick={() => setStatus(memory, "active")} /></Tooltip>
                )}
              </div>
            ),
          },
        ]}
      />
      <MemoryDetail memory={detail} onClose={() => setDetail(null)} onEdit={(state) => { setDetail(null); onEdit(state); }} onStatus={setStatus} />
    </>
  );
}

interface EditorValues {
  type: string;
  subjectPerson?: string;
  persons?: string[];
  placeKey?: string;
  title: string;
  content: string;
  importance: number;
  occurredAt?: dayjs.Dayjs;
  attrs?: string;
}

function MemoryEditor({
  state,
  types,
  onClose,
  onSaved,
}: {
  state: { memory?: Memory; supersede?: Memory } | null;
  types: MemoryType[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const { persons, places, personName } = useConsole();
  const { message } = App.useApp();
  const [form] = Form.useForm<EditorValues>();
  const [saving, setSaving] = useState(false);
  const source = state?.memory ?? state?.supersede;
  const selectedType = Form.useWatch("type", form);
  const hint = types.find((t) => t.type === selectedType)?.hint;

  useEffect(() => {
    if (!state) return;
    form.resetFields();
    if (source) {
      form.setFieldsValue({
        type: source.type,
        subjectPerson: source.subjectPerson || undefined,
        persons: source.persons.filter((p) => p !== source.subjectPerson),
        placeKey: source.placeKey || undefined,
        title: source.title,
        content: source.content,
        importance: source.importance,
        occurredAt: state.supersede ? dayjs() : source.occurredAt ? dayjs(source.occurredAt) : undefined,
        attrs: source.attrs ? JSON.stringify(source.attrs, null, 2) : "",
      });
    } else {
      form.setFieldsValue({ type: "quote", importance: 3, occurredAt: dayjs() });
    }
  }, [state, source, form]);

  const submit = async () => {
    const values = await form.validateFields();
    let attrs: unknown = undefined;
    if (values.attrs?.trim()) {
      try {
        attrs = JSON.parse(values.attrs);
      } catch {
        message.error("扩展字段不是合法的 JSON");
        return;
      }
    }
    const body = {
      type: values.type,
      subjectPerson: values.subjectPerson ?? "",
      persons: values.persons ?? [],
      placeKey: values.placeKey ?? "",
      title: values.title,
      content: values.content,
      importance: values.importance,
      occurredAt: values.occurredAt ? values.occurredAt.toISOString() : null,
      attrs,
    };
    setSaving(true);
    try {
      if (state?.supersede) {
        await api.post(`/memories/${state.supersede.memoryKey}/supersede`, body);
      } else if (state?.memory) {
        await api.put(`/memories/${state.memory.memoryKey}`, body);
      } else {
        await api.post("/memories", body);
      }
      message.success("已保存");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const title = state?.supersede ? "记录变化" : state?.memory ? "编辑记忆" : "新增记忆";
  const personOptions = persons.map((p) => ({ value: p.personKey, label: personName(p.personKey) }));

  return (
    <Drawer
      title={title}
      width={600}
      open={!!state}
      onClose={onClose}
      destroyOnClose
      footer={
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={saving} onClick={submit}>保存</Button>
        </div>
      }
    >
      {state?.supersede && (
        <Callout tone="amber" icon={<History size={16} />} style={{ marginBottom: 20 }}>
          旧记忆会标记为「已过时」并保留，新记忆从今天起生效。把内容改成现在的情况再保存。
        </Callout>
      )}
      <Form form={form} layout="vertical" requiredMark={false}>
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="type" label="类型" rules={[{ required: true }]} extra={hint}>
              <Select options={types.map((t) => ({ value: t.type, label: t.label }))} />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="subjectPerson" label="主要关于谁">
              <Select allowClear placeholder="全家通用留空" options={personOptions} />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="persons" label="还涉及">
              <Select mode="multiple" allowClear maxTagCount="responsive" options={personOptions} />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="placeKey" label="相关地点">
              <Select allowClear options={places.map((p) => ({ value: p.placeKey, label: p.name }))} />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="title" label="一句话摘要" rules={[{ required: true, message: "写一句话摘要" }]}>
          <Input maxLength={200} showCount />
        </Form.Item>
        <Form.Item name="content" label="详细内容" extra="原话尽量保留原文" rules={[{ required: true, message: "写点内容" }]}>
          <Input.TextArea autoSize={{ minRows: 4, maxRows: 12 }} />
        </Form.Item>
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="importance" label="重要度">
              <Rate count={5} />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="occurredAt" label="发生时间">
              <DatePicker showTime={{ format: "HH:mm" }} format="YYYY-MM-DD HH:mm" style={{ width: "100%" }} />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="attrs" label="扩展字段（JSON，可选）" extra='例如承诺：{"promisedTo":"wife","dueAt":"2026-10-20","status":"open"}'>
          <Input.TextArea className="pa-mono" autoSize={{ minRows: 2, maxRows: 8 }} />
        </Form.Item>
      </Form>
    </Drawer>
  );
}

function MemoryDetail({ memory, onClose, onEdit, onStatus }: {
  memory: Memory | null;
  onClose: () => void;
  onEdit: (state: { memory?: Memory; supersede?: Memory }) => void;
  onStatus: (memory: Memory, status: string) => void;
}) {
  const { personName, placeName } = useConsole();
  const { data: history } = useData(
    () => (memory ? api.get<Memory[]>(`/memories/${memory.memoryKey}/history`) : Promise.resolve([] as Memory[])),
    [memory?.memoryKey],
  );
  if (!memory) return null;
  return (
    <Drawer
      title="记忆详情"
      width={600}
      open
      onClose={onClose}
      extra={
        <div style={{ display: "flex", gap: 8 }}>
          {memory.status === "pending" && (
            <>
              <Button danger icon={<X size={15} />} className="pa-icon-text" onClick={() => onStatus(memory, "rejected")}>驳回</Button>
              <Button type="primary" icon={<Check size={15} />} className="pa-icon-text" onClick={() => onStatus(memory, "active")}>确认</Button>
            </>
          )}
          {memory.status === "active" && <Button icon={<History size={15} />} className="pa-icon-text" onClick={() => onEdit({ supersede: memory })}>她变了</Button>}
          <Button icon={<Pencil size={15} />} className="pa-icon-text" onClick={() => onEdit({ memory })}>编辑</Button>
        </div>
      }
    >
      <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
        <Pill tone={typeTone(memory.type)}>{memory.typeLabel}</Pill>
        <Status tone={memoryStatus[memory.status]?.tone}>{memoryStatus[memory.status]?.label}</Status>
      </div>
      <h2 style={{ fontFamily: "var(--pa-font-display)", fontSize: 22, lineHeight: 1.45, margin: "14px 0 0" }}>{memory.title}</h2>
      {memory.content !== memory.title && (
        <div className="pa-pre" style={{ marginTop: 12, fontSize: 15, lineHeight: 1.8, color: "var(--pa-text)", padding: "14px 16px", background: "var(--pa-surface-2)", borderRadius: 12, border: "1px solid var(--pa-line)" }}>
          {memory.content}
        </div>
      )}

      <div className="pa-section-label" style={{ marginTop: 28 }}>信息</div>
      <dl className="pa-kv">
        <dt>关于</dt><dd><PersonTag personKey={memory.subjectPerson} size={22} /></dd>
        <dt>还涉及</dt><dd>{memory.persons.filter((p) => p !== memory.subjectPerson).map(personName).join("、") || "—"}</dd>
        <dt>地点</dt><dd>{memory.placeKey ? placeName(memory.placeKey) : "—"}</dd>
        <dt>重要度</dt><dd><Importance value={memory.importance} /> <span className="pa-muted" style={{ marginLeft: 8 }}>置信度 {memory.confidence.toFixed(2)}</span></dd>
        <dt>发生时间</dt><dd className="pa-num">{memory.occurredAt ? dayjs(memory.occurredAt).format("YYYY-MM-DD HH:mm") : "—"}</dd>
        <dt>有效期</dt><dd className="pa-num">{memory.validFrom ? dayjs(memory.validFrom).format("YYYY-MM-DD") : "—"} ~ {memory.validTo ? dayjs(memory.validTo).format("YYYY-MM-DD") : "至今"}</dd>
        <dt>来源</dt><dd>{createdByLabel[memory.createdBy] ?? memory.createdBy} · 被引用 {memory.useCount} 次</dd>
        <dt>编号</dt><dd className="pa-mono" style={{ color: "var(--pa-text-3)" }}>{memory.memoryKey}</dd>
      </dl>

      {memory.attrs && (
        <>
          <div className="pa-section-label">扩展字段</div>
          <pre className="pa-code">{JSON.stringify(memory.attrs, null, 2)}</pre>
        </>
      )}

      {history && history.length > 1 && (
        <>
          <div className="pa-section-label">演化轨迹</div>
          <Timeline
            items={history.map((item) => ({
              color: item.memoryKey === memory.memoryKey ? "var(--pa-accent)" : "var(--pa-text-3)",
              children: (
                <div>
                  <div style={{ fontWeight: item.memoryKey === memory.memoryKey ? 600 : 400 }}>{item.title}</div>
                  <div className="pa-muted pa-num" style={{ fontSize: 12, marginTop: 2 }}>
                    {item.validFrom ? dayjs(item.validFrom).format("YYYY-MM-DD") : dayjs(item.createdAt).format("YYYY-MM-DD")}
                    {" ~ "}
                    {item.validTo ? dayjs(item.validTo).format("YYYY-MM-DD") : "至今"} · {memoryStatus[item.status]?.label}
                  </div>
                </div>
              ),
            }))}
          />
        </>
      )}
    </Drawer>
  );
}

function ScoreBar({ value, tone }: { value: number; tone: string }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <div style={{ flex: 1, height: 6, borderRadius: 3, background: "var(--pa-fill-strong)", overflow: "hidden", minWidth: 40 }}>
        <div style={{ width: `${Math.min(100, Math.max(0, value * 100))}%`, height: "100%", borderRadius: 3, background: tone }} />
      </div>
      <span className="pa-num" style={{ fontSize: 12, color: "var(--pa-text-2)", width: 38, textAlign: "right" }}>{value.toFixed(3)}</span>
    </div>
  );
}

function SearchDebug() {
  const { persons, personName } = useConsole();
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [hits, setHits] = useState<SearchHit[] | null>(null);
  const [loading, setLoading] = useState(false);

  const run = async () => {
    const values = form.getFieldsValue();
    if (!values.text?.trim()) return;
    setLoading(true);
    try {
      setHits(await api.post<SearchHit[]>("/memories/search", { ...values, persons: values.persons ?? [], limit: values.limit ?? 15 }));
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const reindex = async () => {
    try {
      const result = await api.post<{ count: number }>("/memories/reindex");
      message.success(`补算了 ${result.count} 条向量`);
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  const personOptions = useMemo(() => persons.map((p) => ({ value: p.personKey, label: personName(p.personKey) })), [persons, personName]);

  return (
    <>
      <div style={{ padding: "16px 20px 0" }}>
        <Callout tone="sky" icon={<Search size={15} />}>
          输入一个问题，看助理会自动召回哪些记忆、各项得分多少。得分 = 0.45×向量 + 0.20×全文 + 0.15×重要度 + 0.10×新鲜度 + 0.10×人物命中。
        </Callout>
      </div>
      <Form form={form} layout="inline" onFinish={run} className="pa-toolbar" style={{ rowGap: 8 }}>
        <Form.Item name="text" style={{ width: 340, marginInlineEnd: 0 }}>
          <Input prefix={<Search size={15} color="var(--pa-text-3)" />} placeholder="例如：老婆为什么不喜欢我妈管孩子" />
        </Form.Item>
        <Form.Item name="persons" style={{ marginInlineEnd: 0 }}>
          <Select mode="multiple" allowClear maxTagCount="responsive" placeholder="涉及的人" style={{ width: 180 }} options={personOptions} />
        </Form.Item>
        <Form.Item name="visibleTo" style={{ marginInlineEnd: 0 }}>
          <Select allowClear placeholder="以谁的分身视角" style={{ width: 160 }} options={personOptions} />
        </Form.Item>
        <Form.Item name="limit" style={{ marginInlineEnd: 0 }}>
          <InputNumber min={1} max={50} placeholder="条数" style={{ width: 80 }} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={loading}>检索</Button>
        <div className="pa-toolbar-spacer" />
        <Popconfirm title="给还没有向量的记忆补算向量？" onConfirm={reindex}>
          <Button>补算缺失向量</Button>
        </Popconfirm>
      </Form>
      <Table<SearchHit>
        rowKey={(hit) => hit.memory.memoryKey}
        scroll={{ x: 960 }}
        dataSource={hits ?? []}
        pagination={false}
        locale={{ emptyText: hits === null ? "输入问题后点「检索」" : "没有召回任何记忆" }}
        columns={[
          { title: "#", width: 56, render: (_, __, index) => <span className="pa-num pa-muted">{index + 1}</span> },
          {
            title: "内容",
            render: (_, hit) => (
              <div style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
                <Pill tone={typeTone(hit.memory.type)}>{hit.memory.typeLabel}</Pill>
                <span style={{ lineHeight: 1.5 }}>{hit.memory.title}</span>
              </div>
            ),
          },
          { title: "关于", width: 150, render: (_, hit) => <PersonTag personKey={hit.memory.subjectPerson} size={22} /> },
          { title: "总分", width: 150, render: (_, hit) => <ScoreBar value={hit.score} tone="var(--pa-accent)" /> },
          { title: "向量", width: 130, render: (_, hit) => <ScoreBar value={hit.vector} tone="var(--tone-sky)" /> },
          { title: "全文", width: 130, render: (_, hit) => <ScoreBar value={hit.text} tone="var(--tone-sage)" /> },
        ]}
      />
    </>
  );
}
