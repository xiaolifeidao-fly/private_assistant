"use client";

import { useEffect, useState } from "react";
import { App, Button, Col, DatePicker, Drawer, Form, Input, InputNumber, Popconfirm, Radio, Rate, Row, Select, Switch } from "antd";
import { Cake, MapPin, Plus, Trash2 } from "lucide-react";
import dayjs from "dayjs";
import { api } from "@/lib/api";
import { sideLabel } from "@/lib/labels";
import { personInitial, personTone } from "@/lib/tone";
import type { Person } from "@/lib/types";
import { useConsole } from "@/components/console";
import { Avatar, PageHeader, Pill } from "@/components/ui";

const sides = ["core", "self", "wife"];

export default function PersonsPage() {
  const { persons, placeName, reloadFamily } = useConsole();
  const [editing, setEditing] = useState<Person | "new" | null>(null);

  const birthdayText = (p: Person) =>
    p.birthdayBy === "lunar" && p.lunarBirthday ? `农历 ${p.lunarBirthday}` : p.birthday ? dayjs(p.birthday).format("M月D日") : p.lunarBirthday ? `农历 ${p.lunarBirthday}` : "";

  const groups = [...sides, ...Array.from(new Set(persons.map((p) => p.side))).filter((s) => !sides.includes(s))];

  return (
    <>
      <PageHeader
        eyebrow="家庭"
        title="家庭成员"
        description="助理靠称呼和别名认人。家里平时怎么叫，就把叫法写进别名；性格和近况写在概况里，细节记成记忆更好检索。"
        actions={<Button type="primary" icon={<Plus size={16} />} className="pa-icon-text" onClick={() => setEditing("new")}>添加人物</Button>}
      />
      {groups.map((side) => {
        const members = persons.filter((p) => p.side === side);
        if (!members.length) return null;
        return (
          <section key={side} className="pa-group">
            <div className="pa-section-label">{sideLabel[side] ?? side}<span className="pa-num">{members.length}</span></div>
            <div className="pa-grid">
              {members.map((p) => {
                const tone = personTone(p.personKey, persons);
                const birthday = birthdayText(p);
                return (
                  <div key={p.personKey} className="pa-tile" onClick={() => setEditing(p)}>
                    <div style={{ display: "flex", gap: 14, alignItems: "center" }}>
                      <Avatar tone={tone} size={48}>{personInitial(p)}</Avatar>
                      <div style={{ minWidth: 0, flex: 1 }}>
                        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                          <span style={{ fontFamily: "var(--pa-font-display)", fontSize: 19, fontWeight: 700 }}>{p.roleLabel}</span>
                          {p.isCore && <Pill tone="clay">核心</Pill>}
                        </div>
                        <div style={{ color: "var(--pa-text-2)", fontSize: 13, marginTop: 2 }}>{p.name && p.name !== p.roleLabel ? p.name : "—"}</div>
                      </div>
                    </div>
                    {p.aliases.length > 0 && (
                      <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginTop: 14 }}>
                        {p.aliases.slice(0, 6).map((alias) => <Pill key={alias}>{alias}</Pill>)}
                      </div>
                    )}
                    {p.profile && (
                      <div style={{ fontSize: 13, color: "var(--pa-text-2)", lineHeight: 1.6, marginTop: 12, display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>
                        {p.profile}
                      </div>
                    )}
                    <div style={{ flex: 1 }} />
                    {(birthday || p.homePlaceKey) && (
                      <div style={{ display: "flex", gap: 14, marginTop: 14, paddingTop: 12, borderTop: "1px solid var(--pa-line)", fontSize: 12.5, color: "var(--pa-text-3)" }}>
                        {birthday && <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}><Cake size={13} />{birthday}</span>}
                        {p.homePlaceKey && <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}><MapPin size={13} />{placeName(p.homePlaceKey)}</span>}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </section>
        );
      })}
      <PersonEditor
        person={editing}
        onClose={() => setEditing(null)}
        onSaved={async () => {
          setEditing(null);
          await reloadFamily();
        }}
      />
    </>
  );
}

function PersonEditor({ person, onClose, onSaved }: { person: Person | "new" | null; onClose: () => void; onSaved: () => void }) {
  const { places, persons } = useConsole();
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!person) return;
    form.resetFields();
    if (person === "new") {
      form.setFieldsValue({ side: "self", importance: 3, birthdayBy: "solar", aliases: [], isCore: false, sort: 100 });
    } else {
      form.setFieldsValue({ ...person, birthday: person.birthday ? dayjs(person.birthday) : undefined });
    }
  }, [person, form]);

  const submit = async () => {
    const values = await form.validateFields();
    const body = { ...values, birthday: values.birthday ? values.birthday.format("YYYY-MM-DD") : "" };
    setSaving(true);
    try {
      if (person === "new") await api.post("/persons", body);
      else if (person) await api.put(`/persons/${person.personKey}`, body);
      message.success("已保存");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const archive = async () => {
    if (!person || person === "new") return;
    try {
      await api.del(`/persons/${person.personKey}`);
      message.success("已删除");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  const existing = person && person !== "new" ? person : null;

  return (
    <Drawer
      title={
        existing ? (
          <span style={{ display: "inline-flex", alignItems: "center", gap: 10 }}>
            <Avatar tone={personTone(existing.personKey, persons)} size={28}>{personInitial(existing)}</Avatar>
            编辑 · {existing.roleLabel}
          </span>
        ) : "添加人物"
      }
      width={560}
      open={!!person}
      onClose={onClose}
      destroyOnClose
      footer={
        <div style={{ display: "flex", gap: 8 }}>
          {existing && existing.personKey !== "me" && (
            <Popconfirm title="删除这个人？" description="和 TA 相关的记忆仍会保留。" onConfirm={archive} okText="删除" okButtonProps={{ danger: true }}>
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
            <Form.Item name="roleLabel" label="身份" rules={[{ required: true, message: "如：老婆、大舅" }]} extra="如：老婆、大舅、二姨">
              <Input />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="name" label="名字" rules={[{ required: true, message: "请输入名字" }]}>
              <Input />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="aliases" label="别名" extra="家里平时怎么叫，助理靠这些认人。输入后回车">
          <Select mode="tags" open={false} placeholder="例如：舅舅、建国" />
        </Form.Item>
        <Row gutter={16}>
          <Col span={6}>
            <Form.Item name="gender" label="性别">
              <Select allowClear options={[{ value: "male", label: "男" }, { value: "female", label: "女" }]} />
            </Form.Item>
          </Col>
          <Col span={9}>
            <Form.Item name="side" label="归属">
              <Select options={Object.entries(sideLabel).map(([value, label]) => ({ value, label }))} />
            </Form.Item>
          </Col>
          <Col span={9}>
            <Form.Item name="homePlaceKey" label="住在">
              <Select allowClear options={places.map((p) => ({ value: p.placeKey, label: p.name }))} />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="birthdayBy" label="过哪个生日">
          <Radio.Group optionType="button" options={[{ value: "solar", label: "公历" }, { value: "lunar", label: "农历" }]} />
        </Form.Item>
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="birthday" label="公历生日">
              <DatePicker style={{ width: "100%" }} />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="lunarBirthday" label="农历生日" extra="MM-DD，如 09-08">
              <Input />
            </Form.Item>
          </Col>
        </Row>
        <Row gutter={16} align="middle">
          <Col span={10}>
            <Form.Item name="importance" label="重要度">
              <Rate />
            </Form.Item>
          </Col>
          <Col span={7}>
            <Form.Item name="isCore" label="核心成员" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Col>
          <Col span={7}>
            <Form.Item name="sort" label="排序">
              <InputNumber style={{ width: "100%" }} />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="profile" label="概况" extra="职业、性格概况、近况等。细节记成记忆更好检索">
          <Input.TextArea autoSize={{ minRows: 3, maxRows: 10 }} />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
