"use client";

import { useEffect, useState } from "react";
import { App, Button, Col, Drawer, Empty, Form, Input, InputNumber, Popconfirm, Row, Select } from "antd";
import { Home, Languages, MapPin, Plus, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { placeKindLabel } from "@/lib/labels";
import type { Place } from "@/lib/types";
import { useConsole } from "@/components/console";
import { Callout, PageHeader, PersonTag, Pill } from "@/components/ui";

export default function PlacesPage() {
  const { places, persons, reloadFamily } = useConsole();
  const [editing, setEditing] = useState<Place | "new" | null>(null);

  return (
    <>
      <PageHeader
        eyebrow="家庭"
        title="几个家"
        description="每个家的概要：谁住在那、说什么方言、有哪些规矩。具体规矩（拜年顺序、红包标准、座次、送礼禁忌）建议在「记忆库」里按条记成「规矩习俗」并关联地点，检索更准。"
        actions={<Button type="primary" icon={<Plus size={16} />} className="pa-icon-text" onClick={() => setEditing("new")}>添加地点</Button>}
      />
      {places.length === 0 ? (
        <div className="pa-panel" style={{ padding: 48 }}><Empty description="还没有添加地点" /></div>
      ) : (
        <div className="pa-grid" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(340px, 1fr))" }}>
          {places.map((place) => {
            const residents = persons.filter((p) => p.homePlaceKey === place.placeKey);
            const tone = place.kind === "home" ? "sage" : place.kind === "hometown" ? "clay" : "slate";
            return (
              <div key={place.placeKey} className="pa-tile" onClick={() => setEditing(place)} style={{ padding: 0, overflow: "hidden" }}>
                <div data-tone={tone} style={{ padding: "18px 20px 16px", background: "linear-gradient(160deg, var(--tone-bg), transparent 85%)" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <span style={{ width: 36, height: 36, borderRadius: 11, display: "grid", placeItems: "center", background: "var(--pa-surface)", color: "var(--tone-fg)", boxShadow: "var(--pa-shadow-sm)" }}>
                      {place.kind === "home" ? <Home size={18} /> : <MapPin size={18} />}
                    </span>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontFamily: "var(--pa-font-display)", fontSize: 19, fontWeight: 700 }}>{place.name}</div>
                      <div style={{ fontSize: 12.5, color: "var(--pa-text-3)", display: "flex", gap: 10 }}>
                        <span>{placeKindLabel[place.kind] ?? place.kind}</span>
                        {place.dialect && <span style={{ display: "inline-flex", alignItems: "center", gap: 3 }}><Languages size={12} />{place.dialect}</span>}
                      </div>
                    </div>
                  </div>
                  {place.address && <div style={{ fontSize: 13, color: "var(--pa-text-2)", marginTop: 10 }}>{place.address}</div>}
                </div>
                <div style={{ padding: "14px 20px 18px", display: "flex", flexDirection: "column", gap: 12, flex: 1 }}>
                  <div>
                    <div style={{ fontSize: 12, color: "var(--pa-text-3)", marginBottom: 8, fontWeight: 600, letterSpacing: "0.06em" }}>住在这里</div>
                    {residents.length ? (
                      <div style={{ display: "flex", flexWrap: "wrap", gap: "8px 14px" }}>
                        {residents.map((p) => <PersonTag key={p.personKey} personKey={p.personKey} size={22} />)}
                      </div>
                    ) : <span className="pa-muted" style={{ fontSize: 13 }}>—</span>}
                  </div>
                  {place.customs && (
                    <div>
                      <div style={{ fontSize: 12, color: "var(--pa-text-3)", marginBottom: 6, fontWeight: 600, letterSpacing: "0.06em" }}>规矩</div>
                      <div className="pa-pre" style={{ fontSize: 13, lineHeight: 1.7, color: "var(--pa-text-2)", display: "-webkit-box", WebkitLineClamp: 4, WebkitBoxOrient: "vertical", overflow: "hidden" }}>{place.customs}</div>
                    </div>
                  )}
                  {place.travelTips && <Pill tone="sky">有行前准备清单</Pill>}
                </div>
              </div>
            );
          })}
        </div>
      )}
      <PlaceEditor place={editing} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await reloadFamily(); }} />
    </>
  );
}

function PlaceEditor({ place, onClose, onSaved }: { place: Place | "new" | null; onClose: () => void; onSaved: () => void }) {
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (!place) return;
    form.resetFields();
    form.setFieldsValue(place === "new" ? { kind: "hometown", sort: 10 } : place);
  }, [place, form]);
  const submit = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      if (place === "new") await api.post("/places", values);
      else if (place) await api.put(`/places/${place.placeKey}`, values);
      message.success("已保存");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };
  const remove = async () => {
    if (!place || place === "new") return;
    try {
      await api.del(`/places/${place.placeKey}`);
      message.success("已删除");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    }
  };
  return (
    <Drawer
      title={place === "new" ? "添加地点" : "编辑地点"}
      width={560}
      open={!!place}
      onClose={onClose}
      destroyOnClose
      footer={
        <div style={{ display: "flex", gap: 8 }}>
          {place && place !== "new" && (
            <Popconfirm title="删除这个地点？" onConfirm={remove} okText="删除" okButtonProps={{ danger: true }}>
              <Button danger type="text" icon={<Trash2 size={15} />} className="pa-icon-text">删除</Button>
            </Popconfirm>
          )}
          <div style={{ flex: 1 }} />
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={saving} onClick={submit}>保存</Button>
        </div>
      }
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}><Input /></Form.Item>
          </Col>
          <Col span={7}>
            <Form.Item name="kind" label="类型">
              <Select options={Object.entries(placeKindLabel).map(([value, label]) => ({ value, label }))} />
            </Form.Item>
          </Col>
          <Col span={5}>
            <Form.Item name="sort" label="排序"><InputNumber style={{ width: "100%" }} /></Form.Item>
          </Col>
        </Row>
        <Row gutter={16}>
          <Col span={16}><Form.Item name="address" label="地址"><Input /></Form.Item></Col>
          <Col span={8}><Form.Item name="dialect" label="方言"><Input /></Form.Item></Col>
        </Row>
        <Form.Item name="description" label="介绍"><Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} /></Form.Item>
        <Form.Item name="customs" label="规矩习俗概要"><Input.TextArea autoSize={{ minRows: 3, maxRows: 10 }} /></Form.Item>
        <Form.Item name="travelTips" label="去之前要准备什么"><Input.TextArea autoSize={{ minRows: 2, maxRows: 8 }} /></Form.Item>
      </Form>
      <Callout tone="sky" style={{ marginTop: 4 }}>具体的规矩建议在「记忆库」里按条记录并关联这个地点，回老家前的简报会一起引用。</Callout>
    </Drawer>
  );
}
