"use client";

import { useEffect, useState } from "react";
import { App, Button, Col, Form, Input, Modal, Popconfirm, Rate, Row, Select, Table, Tooltip } from "antd";
import { ArrowRight, Pencil, Plus, Trash2, TriangleAlert } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import type { Relation } from "@/lib/types";
import { useConsole } from "@/components/console";
import { PageHeader, Panel, PersonTag } from "@/components/ui";

export default function RelationsPage() {
  const { persons, personName } = useConsole();
  const { message } = App.useApp();
  const [person, setPerson] = useState<string>("");
  const [editing, setEditing] = useState<Relation | "new" | null>(null);
  const { data, loading, reload } = useData(() => api.get<Relation[]>(`/relations${person ? `?person=${person}` : ""}`), [person]);

  const remove = async (relation: Relation) => {
    try {
      await api.del(`/relations/${relation.relationKey}`);
      message.success("已删除");
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  return (
    <>
      <PageHeader
        eyebrow="家庭"
        title="人物关系"
        description="谁是谁的什么人、怎么称呼、女儿怎么叫，以及人情往来的历史和雷区。回老家前的简报主要靠这里。"
        actions={<Button type="primary" icon={<Plus size={16} />} className="pa-icon-text" onClick={() => setEditing("new")}>添加关系</Button>}
      />
      <Panel>
        <div className="pa-toolbar">
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="只看某人的关系"
            style={{ width: 200 }}
            value={person || undefined}
            onChange={(value) => setPerson(value ?? "")}
            options={persons.map((p) => ({ value: p.personKey, label: personName(p.personKey) }))}
          />
          <div className="pa-toolbar-spacer" />
          <span className="pa-muted pa-num" style={{ fontSize: 13 }}>共 {data?.length ?? 0} 条</span>
        </div>
        <Table<Relation>
          rowKey="relationKey"
          scroll={{ x: 900 }}
          loading={loading}
          dataSource={data ?? []}
          pagination={false}
          locale={{ emptyText: "还没有记录关系" }}
          columns={[
            {
              title: "关系",
              width: 360,
              render: (_, r) => (
                <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
                  <PersonTag personKey={r.toPerson} size={26} />
                  <span style={{ color: "var(--pa-text-3)", fontSize: 13, display: "inline-flex", alignItems: "center", gap: 4 }}>
                    是 <ArrowRight size={12} />
                  </span>
                  <PersonTag personKey={r.fromPerson} size={26} muted />
                  <span style={{ color: "var(--pa-text-3)", fontSize: 13 }}>的</span>
                  <b style={{ fontWeight: 650 }}>{r.relation}</b>
                </div>
              ),
            },
            {
              title: "称呼",
              width: 160,
              render: (_, r) => (
                <div style={{ fontSize: 13, lineHeight: 1.7 }}>
                  {r.addressAs ? <div>我叫「{r.addressAs}」</div> : null}
                  {r.childAddressAs ? <div style={{ color: "var(--pa-text-3)" }}>女儿叫「{r.childAddressAs}」</div> : null}
                  {!r.addressAs && !r.childAddressAs && <span className="pa-muted">—</span>}
                </div>
              ),
            },
            {
              title: "亲近",
              width: 90,
              render: (_, r) => (
                <span style={{ display: "inline-flex", gap: 3 }}>
                  {Array.from({ length: 5 }, (_, i) => <i key={i} style={{ width: 6, height: 6, borderRadius: 3, background: i < r.closeness ? "var(--tone-rose)" : "var(--pa-fill-strong)" }} />)}
                </span>
              ),
            },
            {
              title: "要点与雷区",
              render: (_, r) => (
                <div style={{ fontSize: 13, lineHeight: 1.6 }}>
                  {r.notes && <div style={{ color: "var(--pa-text-2)" }}>{r.notes}</div>}
                  {r.sensitivities && (
                    <div style={{ color: "var(--pa-danger)", display: "flex", gap: 5, marginTop: r.notes ? 4 : 0 }}>
                      <TriangleAlert size={13} style={{ flexShrink: 0, marginTop: 4 }} />
                      {r.sensitivities}
                    </div>
                  )}
                  {!r.notes && !r.sensitivities && <span className="pa-muted">—</span>}
                </div>
              ),
            },
            {
              title: "",
              width: 90,
              align: "right",
              render: (_, r) => (
                <div style={{ display: "inline-flex", gap: 2 }}>
                  <Tooltip title="编辑"><Button size="small" type="text" icon={<Pencil size={14} />} onClick={() => setEditing(r)} /></Tooltip>
                  <Popconfirm title="删除这条关系？" onConfirm={() => remove(r)} okText="删除" okButtonProps={{ danger: true }}>
                    <Button size="small" type="text" danger icon={<Trash2 size={14} />} />
                  </Popconfirm>
                </div>
              ),
            },
          ]}
        />
      </Panel>
      <RelationEditor relation={editing} defaultFrom={person} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); void reload(); }} />
    </>
  );
}

function RelationEditor({ relation, defaultFrom, onClose, onSaved }: { relation: Relation | "new" | null; defaultFrom: string; onClose: () => void; onSaved: () => void }) {
  const { persons, personName } = useConsole();
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (!relation) return;
    form.resetFields();
    form.setFieldsValue(relation === "new" ? { fromPerson: defaultFrom || "me", closeness: 3 } : relation);
  }, [relation, defaultFrom, form]);

  const submit = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      if (relation === "new") await api.post("/relations", values);
      else if (relation) await api.put(`/relations/${relation.relationKey}`, values);
      message.success("已保存");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };
  const options = persons.map((p) => ({ value: p.personKey, label: personName(p.personKey) }));
  return (
    <Modal title={relation === "new" ? "添加关系" : "编辑关系"} open={!!relation} onCancel={onClose} onOk={submit} confirmLoading={saving} okText="保存" width={600} destroyOnClose>
      <Form form={form} layout="vertical" requiredMark={false} style={{ marginTop: 16 }}>
        <Row gutter={12}>
          <Col span={9}>
            <Form.Item name="toPerson" label="谁" rules={[{ required: true, message: "选一个人" }]}>
              <Select showSearch optionFilterProp="label" options={options} />
            </Form.Item>
          </Col>
          <Col span={9}>
            <Form.Item name="fromPerson" label="是谁的" rules={[{ required: true, message: "选一个人" }]}>
              <Select showSearch optionFilterProp="label" options={options} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="relation" label="关系" rules={[{ required: true, message: "如：大舅" }]}>
              <Input placeholder="大舅、表姐" />
            </Form.Item>
          </Col>
        </Row>
        <Row gutter={12}>
          <Col span={8}>
            <Form.Item name="addressAs" label="我怎么称呼"><Input /></Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item name="childAddressAs" label="女儿怎么叫"><Input /></Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item name="closeness" label="亲近程度"><Rate /></Form.Item>
          </Col>
        </Row>
        <Form.Item name="notes" label="关系要点" extra="历史渊源、人情往来，比如：当年借钱给岳父开店">
          <Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} />
        </Form.Item>
        <Form.Item name="sensitivities" label="雷区" extra="不要提的事、不要做的事">
          <Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} />
        </Form.Item>
      </Form>
    </Modal>
  );
}
