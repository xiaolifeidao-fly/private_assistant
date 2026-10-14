"use client";

import { useEffect, useState } from "react";
import { App, Button, Checkbox, Col, Drawer, Form, Input, InputNumber, Modal, Row, Select, Spin, Switch } from "antd";
import { Plus, RotateCcw, Sparkles } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import type { Bot, Skill, SkillExample, SkillSuggestion } from "@/lib/types";
import { PageHeader, Pill, Status } from "@/components/ui";

const blank: Skill = { skillKey: "", name: "", whenToUse: "", body: "", botKeys: ["assistant"], enabled: true, sort: 99, version: 0, updatedAt: "" };

export default function SkillsPage() {
  const { data, loading, reload } = useData(() => api.get<Skill[]>("/skills"), []);
  const { data: pending, reload: reloadPending } = useData(() => api.get<SkillExample[]>("/skill-examples?status=pending"), []);
  const pendingOf = (key: string) => (pending ?? []).filter((item) => item.skillKey === key).length;
  const { data: bots } = useData(() => api.get<Bot[]>("/bots"), []);
  const [editing, setEditing] = useState<Skill | null>(null);
  const botName = (key: string) => bots?.find((b) => b.botKey === key)?.name ?? key;

  return (
    <>
      <PageHeader
        eyebrow="助理"
        title="做事手册"
        description="每本手册写一类事该先查什么、怎么想、怎么说、别做什么。系统提示词里只放目录，助理遇到对应场景才读正文，读过的留在这段对话里。改了手册对之后读到它的对话生效。"
        actions={<Button type="primary" icon={<Plus size={15} />} className="pa-icon-text" onClick={() => setEditing(blank)}>新建手册</Button>}
      />
      {loading && !data ? <div style={{ padding: 60, textAlign: "center" }}><Spin /></div> : (
        <div className="pa-grid" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))" }}>
          {(data ?? []).map((skill) => (
            <div key={skill.skillKey} className="pa-tile" data-muted={!skill.enabled} onClick={() => setEditing(skill)}>
              <div style={{ display: "flex", alignItems: "flex-start", gap: 10 }}>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontFamily: "var(--pa-font-display)", fontSize: 18, fontWeight: 700 }}>{skill.name}</div>
                  <div className="pa-mono" style={{ fontSize: 12, color: "var(--pa-text-3)", marginTop: 2 }}>{skill.skillKey}</div>
                </div>
                <Status tone={skill.enabled ? "sage" : "slate"}>{skill.enabled ? "启用" : "停用"}</Status>
              </div>
              <div style={{ fontSize: 13.5, color: "var(--pa-text-2)", lineHeight: 1.65, marginTop: 12, minHeight: 44 }}>{skill.whenToUse}</div>
              <div style={{ flex: 1 }} />
              <div style={{ display: "flex", gap: 6, flexWrap: "wrap", alignItems: "center", marginTop: 14, paddingTop: 12, borderTop: "1px solid var(--pa-line)" }}>
                {skill.botKeys.map((key) => <Pill key={key} tone="slate">{botName(key)}</Pill>)}
                <span style={{ flex: 1 }} />
                {pendingOf(skill.skillKey) > 0 && <Pill tone="amber">{pendingOf(skill.skillKey)} 条待审示例</Pill>}
                <span className="pa-num" style={{ fontSize: 12.5, color: "var(--pa-text-3)" }}>v{skill.version}</span>
              </div>
            </div>
          ))}
        </div>
      )}
      <SkillEditor
        skill={editing}
        bots={bots ?? []}
        onClose={() => { setEditing(null); void reloadPending(); }}
        onSaved={() => { setEditing(null); void reload(); void reloadPending(); }}
      />
    </>
  );
}

function SkillEditor({ skill, bots, onClose, onSaved }: { skill: Skill | null; bots: Bot[]; onClose: () => void; onSaved: () => void }) {
  const { message, modal } = App.useApp();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  const [suggesting, setSuggesting] = useState(false);
  const [suggestion, setSuggestion] = useState<SkillSuggestion | null>(null);
  useEffect(() => {
    if (skill) {
      form.resetFields();
      form.setFieldsValue(skill);
    }
  }, [skill, form]);
  if (!skill) return null;
  const creating = skill.version === 0;

  const submit = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await api.put(`/skills/${creating ? values.skillKey : skill.skillKey}`, { ...skill, ...values });
      message.success("已保存");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const restore = () => {
    modal.confirm({
      title: "恢复出厂内容？",
      content: "何时用和正文会被替换成出厂版本，保存后才生效。",
      okText: "恢复",
      onOk: async () => {
        try {
          const result = await api.get<Skill>(`/skills/${skill.skillKey}/default`);
          form.setFieldsValue({ name: result.name, whenToUse: result.whenToUse, body: result.body, botKeys: result.botKeys });
        } catch (error) {
          message.error((error as Error).message);
        }
      },
    });
  };

  const suggest = async () => {
    setSuggesting(true);
    try {
      setSuggestion(await api.post<SkillSuggestion>(`/skills/${skill.skillKey}/suggest`));
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSuggesting(false);
    }
  };

  return (
    <Drawer
      title={creating ? "新建手册" : skill.name}
      width={760}
      open
      onClose={onClose}
      destroyOnClose
      footer={
        <div style={{ display: "flex", gap: 8 }}>
          {!creating && <Button icon={<RotateCcw size={15} />} className="pa-icon-text" onClick={restore}>恢复出厂内容</Button>}
          {!creating && (
            <Button icon={<Sparkles size={15} />} className="pa-icon-text" loading={suggesting} onClick={suggest}>
              按家里的记忆改写
            </Button>
          )}
          <div style={{ flex: 1 }} />
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={saving} onClick={submit}>保存</Button>
        </div>
      }
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        <Row gutter={16}>
          <Col span={9}>
            <Form.Item name="skillKey" label="标识" extra="load_skill 的参数" rules={[{ required: true, pattern: /^[a-z0-9_]{3,40}$/, message: "小写字母、数字、下划线，3-40 位" }]}>
              <Input className="pa-mono" disabled={!creating} />
            </Form.Item>
          </Col>
          <Col span={7}><Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}><Input /></Form.Item></Col>
          <Col span={4}><Form.Item name="sort" label="排序"><InputNumber style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={4}><Form.Item name="enabled" label="启用" valuePropName="checked"><Switch /></Form.Item></Col>
        </Row>
        <Form.Item name="whenToUse" label="什么时候用" extra="一句话，会进系统提示词里的目录，助理靠它判断要不要读" rules={[{ required: true, message: "请写什么时候用" }]}>
          <Input />
        </Form.Item>
        <Form.Item name="botKeys" label="哪些机器人能用" extra="机器人还需要开启「读做事手册」工具">
          <Checkbox.Group options={bots.map((b) => ({ value: b.botKey, label: b.name }))} />
        </Form.Item>
        <Form.Item name="body" label="正文" rules={[{ required: true, message: "正文不能为空" }]} extra="建议按「先查 / 怎么想 / 怎么说 / 别做 / 收尾」写；提到工具时写工具名">
          <Input.TextArea autoSize={{ minRows: 16, maxRows: 36 }} className="pa-mono" style={{ fontSize: 12.5, lineHeight: 1.7 }} />
        </Form.Item>
      </Form>
      {!creating && <Examples skillKey={skill.skillKey} />}
      <Modal
        title="按家里的记忆改写的建议"
        width={860}
        open={suggestion !== null}
        onCancel={() => setSuggestion(null)}
        okText="采用（填进正文，保存后生效）"
        onOk={() => {
          form.setFieldValue("body", suggestion?.body);
          setSuggestion(null);
        }}
      >
        {suggestion && (
          <>
            <div className="pa-muted" style={{ fontSize: 13, marginBottom: 10 }}>参考了 {suggestion.memories} 条记忆。先核对括号里的记忆编号，确认没有说错再采用。</div>
            {suggestion.changes.length > 0 && (
              <>
                <div style={{ fontWeight: 600, marginBottom: 4 }}>改了什么</div>
                <ul style={{ margin: "0 0 12px", paddingLeft: 20, fontSize: 13.5 }}>{suggestion.changes.map((item) => <li key={item}>{item}</li>)}</ul>
              </>
            )}
            {suggestion.gaps.length > 0 && (
              <>
                <div style={{ fontWeight: 600, marginBottom: 4 }}>记忆里还缺（建议补记）</div>
                <ul style={{ margin: "0 0 12px", paddingLeft: 20, fontSize: 13.5 }}>{suggestion.gaps.map((item) => <li key={item}>{item}</li>)}</ul>
              </>
            )}
            <pre className="pa-code" style={{ maxHeight: 420 }}>{suggestion.body}</pre>
          </>
        )}
      </Modal>
    </Drawer>
  );
}

const exampleStatus: Record<string, { label: string; tone: "amber" | "sage" | "slate" }> = {
  pending: { label: "待审", tone: "amber" },
  approved: { label: "生效", tone: "sage" },
  rejected: { label: "不用", tone: "slate" },
};

/** 手册示例：点踩反馈自动整理的待审示例，以及手工添加的。生效的（最新 5 条）随正文一起给模型。 */
function Examples({ skillKey }: { skillKey: string }) {
  const { message } = App.useApp();
  const { data, reload } = useData(() => api.get<SkillExample[]>(`/skill-examples?skillKey=${skillKey}`), [skillKey]);
  const [editing, setEditing] = useState<Partial<SkillExample> | null>(null);
  const [form] = Form.useForm();

  const setStatus = async (item: SkillExample, status: string) => {
    try {
      await api.post(`/skill-examples/${item.exampleKey}/status`, { status });
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    }
  };
  const open = (item: Partial<SkillExample>) => {
    setEditing(item);
    form.resetFields();
    form.setFieldsValue(item);
  };
  const save = async () => {
    const values = await form.validateFields();
    try {
      const body = { ...editing, ...values, skillKey };
      if (editing?.exampleKey) await api.put(`/skill-examples/${editing.exampleKey}`, body);
      else await api.post("/skill-examples", body);
      setEditing(null);
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  return (
    <div style={{ marginTop: 28, paddingTop: 20, borderTop: "1px solid var(--pa-line)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6 }}>
        <span style={{ fontWeight: 600 }}>我们家的示例</span>
        <span style={{ flex: 1 }} />
        <Button size="small" icon={<Plus size={13} />} className="pa-icon-text" onClick={() => open({ status: "approved" })}>添加示例</Button>
      </div>
      <div className="pa-muted" style={{ fontSize: 12.5, marginBottom: 12 }}>
        在手机上点踩并写了「哪里不对」的回答，会自动整理成待审示例。生效的示例（最新 5 条）会跟正文一起给助理看。
      </div>
      {(data ?? []).length === 0 && <div className="pa-muted" style={{ fontSize: 13 }}>还没有示例。</div>}
      {(data ?? []).map((item) => (
        <div key={item.exampleKey} style={{ border: "1px solid var(--pa-line)", borderRadius: 12, padding: "10px 14px", marginBottom: 10, opacity: item.status === "rejected" ? 0.55 : 1 }}>
          <div style={{ display: "flex", gap: 6, alignItems: "center", marginBottom: 6 }}>
            <Pill tone={exampleStatus[item.status]?.tone}>{exampleStatus[item.status]?.label ?? item.status}</Pill>
            {item.source === "feedback" && <Pill tone="slate">来自点踩</Pill>}
            <span style={{ flex: 1 }} />
            {item.status !== "approved" && <Button size="small" type="link" onClick={() => setStatus(item, "approved")}>生效</Button>}
            {item.status !== "rejected" && <Button size="small" type="link" onClick={() => setStatus(item, "rejected")}>不用</Button>}
            <Button size="small" type="link" onClick={() => open(item)}>编辑</Button>
          </div>
          <div style={{ fontSize: 13, lineHeight: 1.7 }}>
            <div><b>情况</b>：{item.situation}</div>
            <div><b>好的说法</b>：{item.goodResponse}</div>
            {item.badResponse && <div><b>别这么说</b>：{item.badResponse}</div>}
            {item.why && <div><b>为什么</b>：{item.why}</div>}
          </div>
        </div>
      ))}
      <Modal title={editing?.exampleKey ? "编辑示例" : "添加示例"} open={editing !== null} onOk={save} onCancel={() => setEditing(null)} okText="保存" width={640}>
        <Form form={form} layout="vertical" requiredMark={false}>
          <Form.Item name="situation" label="情况" rules={[{ required: true, message: "请写是什么情况" }]}><Input.TextArea autoSize={{ minRows: 2 }} /></Form.Item>
          <Form.Item name="goodResponse" label="好的说法 / 做法" rules={[{ required: true, message: "请写好的说法" }]}><Input.TextArea autoSize={{ minRows: 2 }} /></Form.Item>
          <Form.Item name="badResponse" label="别这么说"><Input.TextArea autoSize={{ minRows: 1 }} /></Form.Item>
          <Form.Item name="why" label="为什么"><Input /></Form.Item>
          <Form.Item name="status" label="状态">
            <Select options={Object.entries(exampleStatus).map(([value, item]) => ({ value, label: item.label }))} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
