"use client";

import { useEffect, useState, type ReactNode } from "react";
import dayjs from "dayjs";
import { App, Button, Col, Form, Input, InputNumber, Modal, Row, Select, Spin, Switch, Table, Tabs } from "antd";
import { RotateCcw } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import type { SafetyHit, SafetySettings, SegmentRoll } from "@/lib/types";
import { PageHeader, Panel, Pill } from "@/components/ui";

export default function RulesPage() {
  return (
    <>
      <PageHeader
        eyebrow="助理"
        title="对话规则"
        description="危机兜底的词表、每轮能塞多少上下文、系统多久从对话里学一次。改完 30 秒内对所有对话生效（已经发出的历史不会变）。"
      />
      <Panel>
        <Tabs
          destroyInactiveTabPane
          items={[
            { key: "safety", label: "危机兜底", children: <SafetyForm /> },
            { key: "hits", label: "危机命中记录", children: <SafetyHits /> },
            { key: "budget", label: "上下文预算", children: <><BudgetForm /><SegmentRolls /></> },
            { key: "learning", label: "自动学习", children: <LearningForm /> },
          ]}
        />
      </Panel>
    </>
  );
}

/** 一个分组的表单外壳：读、存、恢复出厂值。 */
function SettingForm({ group, children }: { group: string; children: ReactNode }) {
  const { message, modal } = App.useApp();
  const [form] = Form.useForm();
  const { data, loading } = useData(() => api.get<Record<string, unknown>>(`/settings/${group}`), [group]);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (data) form.setFieldsValue(data);
  }, [data, form]);

  const save = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      const result = await api.put<Record<string, unknown>>(`/settings/${group}`, { ...data, ...values });
      form.setFieldsValue(result);
      message.success("已保存，30 秒内生效");
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };
  const reset = () => {
    modal.confirm({
      title: "恢复出厂值？",
      content: "这一组的设置会回到代码里的默认值，立即生效。",
      okText: "恢复",
      onOk: async () => {
        const result = await api.del<Record<string, unknown>>(`/settings/${group}`);
        form.setFieldsValue(result);
        message.success("已恢复");
      },
    });
  };

  if (loading && !data) return <div style={{ padding: 40, textAlign: "center" }}><Spin /></div>;
  return (
    <Form form={form} layout="vertical" requiredMark={false} style={{ maxWidth: 820 }}>
      {children}
      <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
        <Button type="primary" loading={saving} onClick={save}>保存</Button>
        <Button icon={<RotateCcw size={15} />} className="pa-icon-text" onClick={reset}>恢复出厂值</Button>
      </div>
    </Form>
  );
}

const tags = { mode: "tags" as const, tokenSeparators: [",", "，", "、"], open: false, style: { width: "100%" } };

function SafetyForm() {
  return (
    <SettingForm group="safety">
      <Form.Item name="crisisWords" label="危机词" extra="我说的话里出现任意一个，就提醒助理先读 crisis 手册；回车或逗号分隔">
        <Select {...tags} />
      </Form.Item>
      <Form.Item name="excludePhrases" label="排除短语" extra="先从原话里去掉这些再匹配，用来消误报，比如「想死你了」「笑死」">
        <Select {...tags} />
      </Form.Item>
      <Form.Item name="helpWords" label="求助词" extra="回答里出现任意一个，就算助理已经给了求助建议，不再补提示卡">
        <Select {...tags} />
      </Form.Item>
      <Form.Item name="notice" label="提示卡文案" extra="助理漏了求助建议时，在回答下面补这张卡" rules={[{ required: true, message: "不能为空" }]}>
        <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} />
      </Form.Item>
    </SettingForm>
  );
}

function BudgetForm() {
  return (
    <SettingForm group="budget">
      <Row gutter={16}>
        <Col span={8}><Form.Item name="turnContextHits" label="每轮检索条数" extra="自动附在每句话前面的参考"><InputNumber min={1} max={50} style={{ width: "100%" }} /></Form.Item></Col>
        <Col span={8}><Form.Item name="turnContextChars" label="每轮检索字数上限"><InputNumber min={500} step={500} style={{ width: "100%" }} /></Form.Item></Col>
        <Col span={8}><Form.Item name="toolOutputChars" label="单个工具结果字数"><InputNumber min={1000} step={1000} style={{ width: "100%" }} /></Form.Item></Col>
      </Row>
      <Row gutter={16}>
        <Col span={8}>
          <Form.Item name="segmentTokens" label="分段阈值（token）" extra="上一轮输入超过它，就把前文压缩成摘要、开新的一段。对话记录里能看到每次分段漏掉了哪些记忆">
            <InputNumber min={10000} step={10000} style={{ width: "100%" }} />
          </Form.Item>
        </Col>
        <Col span={8}><Form.Item name="smallTalkMaxRunes" label="寒暄最长几个字"><InputNumber min={1} max={40} style={{ width: "100%" }} /></Form.Item></Col>
      </Row>
      <Form.Item name="smallTalkWords" label="寒暄词" extra="一句话只由这些词和标点组成时不检索记忆">
        <Select {...tags} />
      </Form.Item>
    </SettingForm>
  );
}

function LearningForm() {
  return (
    <SettingForm group="learning">
      <Row gutter={16}>
        <Col span={8}><Form.Item name="chatExtract" label="从对话里自动学习" valuePropName="checked" extra="对话空闲后，把我说的话交给记忆整理"><Switch /></Form.Item></Col>
        <Col span={8}><Form.Item name="chatIdleMinutes" label="对话空闲多久后整理（分钟）"><InputNumber min={1} style={{ width: "100%" }} /></Form.Item></Col>
        <Col span={8}><Form.Item name="personaSettleMinutes" label="人设卡刷新前等待（分钟）" extra="某人的记忆最后一次变化后等这么久再重写人设卡"><InputNumber min={0} style={{ width: "100%" }} /></Form.Item></Col>
      </Row>
      <Row gutter={16}>
        <Col span={8}><Form.Item name="weeklyInsight" label="每周洞察" valuePropName="checked" extra="每周日晚总结这周她们在意什么"><Switch /></Form.Item></Col>
        <Col span={8}><Form.Item name="mergeSimilar" label="每周找重复记忆" valuePropName="checked" extra="疑似重复的提给我确认合并"><Switch /></Form.Item></Col>
      </Row>
    </SettingForm>
  );
}

/** 原话里命中词前后各取几个字，作为排除短语的建议。 */
function around(content: string, word: string): string {
  const index = content.indexOf(word);
  if (index < 0) return word;
  return content.slice(Math.max(0, index - 2), Math.min(content.length, index + word.length + 2));
}

function SafetyHits() {
  const { message } = App.useApp();
  const { data, loading } = useData(() => api.get<SafetyHit[]>("/safety-hits"), []);
  const [excluding, setExcluding] = useState<string | null>(null);

  const addExclude = async () => {
    const phrase = excluding?.trim();
    if (!phrase) return;
    try {
      const current = await api.get<SafetySettings>("/settings/safety");
      await api.put("/settings/safety", { ...current, excludePhrases: [...current.excludePhrases, phrase] });
      message.success(`已把「${phrase}」加入排除短语`);
      setExcluding(null);
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  return (
    <>
      <div className="pa-muted" style={{ fontSize: 13, marginBottom: 12 }}>
        最近命中危机词的话。是误报的，把那几个字加进排除短语；真正危险却没命中的，回「危机兜底」里补词。
      </div>
      <Table<SafetyHit>
        rowKey="messageKey"
        loading={loading}
        dataSource={data ?? []}
        pagination={{ pageSize: 20, hideOnSinglePage: true }}
        locale={{ emptyText: "还没有命中过" }}
        columns={[
          { title: "时间", width: 140, render: (_, hit) => <span className="pa-num">{dayjs(hit.createdAt).format("MM-DD HH:mm")}</span> },
          { title: "我说的话", render: (_, hit) => <span>{hit.content}</span> },
          { title: "命中", width: 160, render: (_, hit) => <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>{hit.signals.map((word) => <Pill key={word} tone="rose">{word}</Pill>)}</div> },
          {
            title: "助理",
            width: 130,
            render: (_, hit) => hit.helpMentioned === undefined || hit.helpMentioned === null
              ? <Pill tone="slate">未回答</Pill>
              : hit.helpMentioned ? <Pill tone="sage">给了求助建议</Pill> : <Pill tone="amber">漏了，已补卡</Pill>,
          },
          { title: "", width: 110, render: (_, hit) => <Button size="small" onClick={() => setExcluding(around(hit.content, hit.signals[0] ?? ""))}>是误报</Button> },
        ]}
      />
      <Modal title="加入排除短语" open={excluding !== null} onOk={addExclude} onCancel={() => setExcluding(null)} okText="加入">
        <div className="pa-muted" style={{ fontSize: 13, marginBottom: 8 }}>写成足够具体的短语（比如「想死你了」），别直接排除「想死」，免得真正的危险也漏掉。</div>
        <Input value={excluding ?? ""} onChange={(event) => setExcluding(event.target.value)} />
      </Modal>
    </>
  );
}

/** 最近的长对话分段：摘要漏掉了几条助理引用过的记忆，用来判断分段阈值合不合适。 */
function SegmentRolls() {
  const { data, loading } = useData(() => api.get<SegmentRoll[]>("/segment-rolls"), []);
  return (
    <div style={{ marginTop: 32 }}>
      <div style={{ fontWeight: 600, marginBottom: 4 }}>最近的分段</div>
      <div className="pa-muted" style={{ fontSize: 13, marginBottom: 12 }}>
        「漏掉」是助理在前一段回答里引用过、摘要里却没有的记忆。经常漏，就调大分段阈值；从来不分段又很贵，就调小。
      </div>
      <Table<SegmentRoll>
        rowKey="messageKey"
        size="small"
        loading={loading}
        dataSource={data ?? []}
        pagination={{ pageSize: 10, hideOnSinglePage: true }}
        locale={{ emptyText: "还没有对话长到需要分段" }}
        columns={[
          { title: "时间", width: 120, render: (_, roll) => <span className="pa-num">{dayjs(roll.createdAt).format("MM-DD HH:mm")}</span> },
          { title: "对话", render: (_, roll) => <span>{roll.title}（第 {roll.to + 1} 段）</span> },
          { title: "触发时输入", width: 120, render: (_, roll) => <span className="pa-num">{roll.tokens.toLocaleString()} token</span> },
          { title: "摘要", width: 90, render: (_, roll) => <span className="pa-num">{roll.summaryChars} 字</span> },
          {
            title: "漏掉",
            width: 200,
            render: (_, roll) => roll.lost.length === 0
              ? <Pill tone="sage">没有</Pill>
              : <span title={roll.lost.join("、")}><Pill tone="amber">{roll.lost.length} 条记忆</Pill></span>,
          },
        ]}
      />
    </div>
  );
}
