"use client";

import { useEffect, useState } from "react";
import dayjs from "dayjs";
import { App, Button, Empty, Input, Modal, Segmented, Spin } from "antd";
import { Eye, MessagesSquare, Pencil, RotateCcw, Sparkles, Undo2 } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import { personInitial, personTone } from "@/lib/tone";
import type { PersonaCard } from "@/lib/types";
import { useConsole } from "@/components/console";
import { Avatar, Callout, PageHeader, Panel, Pill } from "@/components/ui";
import { InterviewDrawer } from "./interview-drawer";

const createdByLabel: Record<string, string> = { auto: "自动生成", manual: "手工修订", rollback: "回滚", chat: "对话补充" };

export default function PersonasPage() {
  const { persons } = useConsole();
  const core = persons.filter((p) => p.isCore && p.personKey !== "me");
  const [person, setPerson] = useState("");
  useEffect(() => {
    if (!person && core.length) setPerson(core[0].personKey);
  }, [core, person]);

  return (
    <>
      <PageHeader
        eyebrow="家庭"
        title="人设卡"
        description="由记忆自动归纳：有新记忆后每小时检查一次，录完一批记忆 20 分钟后生成。分身用它模拟她的视角，助理用它理解她。也可以点「对话补充」，回答模型的提问把卡补全。每次变化都留一个版本，不准可以修订，错了可以回滚。"
      />
      <div style={{ marginBottom: 16 }}>
        <Segmented
          size="large"
          value={person}
          onChange={(value) => setPerson(String(value))}
          options={core.map((p) => ({
            value: p.personKey,
            label: (
              <span style={{ display: "inline-flex", alignItems: "center", gap: 8, padding: "2px 4px" }}>
                <Avatar tone={personTone(p.personKey, persons)} size={22}>{personInitial(p)}</Avatar>
                {p.roleLabel}
              </span>
            ),
          }))}
        />
      </div>
      {person && <PersonaPanel key={person} personKey={person} roleLabel={core.find((p) => p.personKey === person)?.roleLabel ?? ""} />}
    </>
  );
}

function PersonaPanel({ personKey, roleLabel }: { personKey: string; roleLabel: string }) {
  const { message } = App.useApp();
  const { data: history, loading, reload } = useData(() => api.get<PersonaCard[]>(`/persona-cards?person=${personKey}`), [personKey]);
  const [viewing, setViewing] = useState<PersonaCard | null>(null);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState({ content: "", changes: "" });
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [interviewing, setInterviewing] = useState(false);
  const active = history?.find((card) => card.status === "active");
  const shown = viewing ?? active;
  const isHistory = !!viewing && viewing.status !== "active";

  const generate = async () => {
    setGenerating(true);
    try {
      const result = await api.post<{ card: PersonaCard; changed: boolean }>("/persona-cards/generate", { person: personKey });
      message.success(result.changed ? `已生成 v${result.card.version}` : "和当前版本相比没有实质变化");
      setViewing(null);
      void reload();
    } catch (error) {
      message.error((error as Error).message, 8);
    } finally {
      setGenerating(false);
    }
  };

  const saveManual = async () => {
    if (!draft.content.trim()) {
      message.warning("人设卡内容不能为空");
      return;
    }
    setSaving(true);
    try {
      await api.post("/persona-cards", { person: personKey, ...draft });
      message.success("已保存为新版本");
      setEditing(false);
      setViewing(null);
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const activate = async (card: PersonaCard) => {
    try {
      await api.post(`/persona-cards/${card.cardKey}/activate`);
      message.success(`已回滚到 v${card.version}`);
      setViewing(null);
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  return (
    <div style={{ display: "grid", gridTemplateColumns: "minmax(0, 1fr) 320px", gap: 16, alignItems: "start" }} className="persona-layout">
      <Panel
        title={
          shown ? (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 10 }}>
              <span className="pa-num">v{shown.version}</span>
              {shown.status === "active" ? <Pill tone="sage" dot>当前版本</Pill> : <Pill tone="slate">历史版本</Pill>}
              <span style={{ fontWeight: 400, fontSize: 13, color: "var(--pa-text-3)" }}>{createdByLabel[shown.createdBy]} · {dayjs(shown.createdAt).format("YYYY-MM-DD HH:mm")}</span>
            </span>
          ) : "还没有人设卡"
        }
        extra={
          <>
            {isHistory && <Button icon={<Undo2 size={15} />} className="pa-icon-text" onClick={() => setViewing(null)}>回到当前</Button>}
            {isHistory && <Button icon={<RotateCcw size={15} />} className="pa-icon-text" onClick={() => activate(viewing!)}>设为当前</Button>}
            {!isHistory && (
              <Button icon={<Pencil size={15} />} className="pa-icon-text" disabled={!active} onClick={() => { setDraft({ content: active?.content ?? "", changes: "" }); setEditing(true); }}>修订</Button>
            )}
            <Button icon={<MessagesSquare size={15} />} className="pa-icon-text" onClick={() => setInterviewing(true)}>对话补充</Button>
            <Button type="primary" icon={<Sparkles size={15} />} className="pa-icon-text" loading={generating} onClick={generate}>重新生成</Button>
          </>
        }
      >
        <div style={{ padding: "24px 32px 28px" }}>
          {loading && !history ? <div style={{ textAlign: "center", padding: 40 }}><Spin /></div> : shown ? (
            <>
              {isHistory && <Callout tone="sky" icon={<Eye size={15} />} style={{ marginBottom: 16 }}>正在查看历史版本，点「设为当前」可以回滚到这一版。</Callout>}
              {shown.changes && <Callout tone="clay" icon={<Sparkles size={15} />} style={{ marginBottom: 20 }}>这一版的变化：{shown.changes}</Callout>}
              <div className="pa-pre pa-reading">{shown.content}</div>
              <div style={{ marginTop: 24, paddingTop: 14, borderTop: "1px solid var(--pa-line)", fontSize: 12.5, color: "var(--pa-text-3)" }}>
                基于 {shown.memoryCount} 条记忆{shown.memoryCutoff ? `，记忆截至 ${dayjs(shown.memoryCutoff).format("YYYY-MM-DD HH:mm")}` : ""}
              </div>
            </>
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="关于她的记忆至少 3 条后会自动生成，也可以点「重新生成」，或者点「对话补充」直接聊出一版" />
          )}
        </div>
      </Panel>

      <Panel title="版本记录">
        <div style={{ padding: "8px 8px 10px" }}>
          {(history ?? []).map((card) => {
            const selected = (viewing ?? active)?.cardKey === card.cardKey;
            return (
              <button key={card.cardKey} className="persona-version" data-selected={selected} onClick={() => setViewing(card.status === "active" ? null : card)}>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <span className="pa-num" style={{ fontWeight: 650 }}>v{card.version}</span>
                  {card.status === "active" && <Pill tone="sage">当前</Pill>}
                  <span style={{ marginLeft: "auto", fontSize: 12, color: "var(--pa-text-3)" }} className="pa-num">{dayjs(card.createdAt).format("MM-DD HH:mm")}</span>
                </div>
                <div style={{ fontSize: 12.5, color: "var(--pa-text-2)", marginTop: 4, lineHeight: 1.5, textAlign: "left" }}>
                  {card.changes || createdByLabel[card.createdBy]}
                </div>
              </button>
            );
          })}
          {!history?.length && <div style={{ padding: 16, color: "var(--pa-text-3)", fontSize: 13 }}>暂无版本</div>}
        </div>
      </Panel>

      <Modal title="修订人设卡" open={editing} onCancel={() => setEditing(false)} onOk={saveManual} confirmLoading={saving} width={800} okText="保存为新版本">
        <Input placeholder="这次改了什么（可选）" value={draft.changes} onChange={(e) => setDraft({ ...draft, changes: e.target.value })} style={{ margin: "16px 0 12px" }} />
        <Input.TextArea value={draft.content} onChange={(e) => setDraft({ ...draft, content: e.target.value })} autoSize={{ minRows: 16, maxRows: 30 }} style={{ lineHeight: 1.8 }} />
      </Modal>

      <InterviewDrawer
        personKey={personKey}
        roleLabel={roleLabel}
        open={interviewing}
        onClose={() => setInterviewing(false)}
        onSaved={() => {
          setInterviewing(false);
          setViewing(null);
          void reload();
        }}
      />

      <style jsx global>{`
        .persona-version {
          display: block;
          width: 100%;
          border: 0;
          background: none;
          padding: 10px 12px;
          border-radius: 10px;
          cursor: pointer;
          color: var(--pa-text);
          transition: background 0.15s;
        }
        .persona-version:hover { background: var(--pa-fill); }
        .persona-version[data-selected="true"] { background: var(--pa-accent-soft); }
        @media (max-width: 1100px) { .persona-layout { grid-template-columns: 1fr !important; } }
      `}</style>
    </div>
  );
}
