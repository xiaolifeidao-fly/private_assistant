"use client";

import { useEffect, useState } from "react";
import dayjs from "dayjs";
import { App, Button, DatePicker, Drawer, Form, Image, Input, Modal, Segmented, Select, Table, Tooltip, Upload } from "antd";
import type { UploadFile } from "antd";
import { CircleAlert, ClipboardPaste, FileUp, ImagePlus, RefreshCw, RotateCcw, UserRoundX } from "lucide-react";
import { api, fetchObjectURL, postForm, query } from "@/lib/api";
import { useData } from "@/lib/hooks";
import { memoryStatus } from "@/lib/labels";
import { typeTone, type Tone } from "@/lib/tone";
import type { Memory, Note, Paged } from "@/lib/types";
import { useConsole } from "@/components/console";
import { Callout, PageHeader, Panel, PersonTag, Pill, Status } from "@/components/ui";

const statusMeta: Record<string, { label: string; tone: Tone }> = {
  queued: { label: "排队中", tone: "slate" },
  running: { label: "整理中", tone: "sky" },
  done: { label: "已整理", tone: "sage" },
  failed: { label: "失败", tone: "rose" },
};

const channelLabel: Record<string, string> = { quick_note: "速记", paste: "粘贴", screenshot: "截图", manager: "管理端", chat: "对话学习", import: "批量导入" };

export default function NotesPage() {
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const [viewing, setViewing] = useState<Note | null>(null);
  const [creating, setCreating] = useState(false);
  const [importing, setImporting] = useState(false);
  const { refreshPending } = useConsole();
  const { data, loading, reload } = useData(() => api.get<Paged<Note>>(`/notes${query({ status, page, size: 20 })}`), [status, page]);

  const busy = data?.items.some((n) => n.extractStatus === "queued" || n.extractStatus === "running");
  // 整理完会多出待确认的记忆，侧边栏角标跟着刷新
  useEffect(() => {
    if (!busy) refreshPending();
  }, [busy, refreshPending]);
  useEffect(() => {
    if (!busy) return;
    const timer = setInterval(() => void reload(), 5000);
    return () => clearInterval(timer);
  }, [busy, reload]);

  return (
    <>
      <PageHeader
        eyebrow="记忆"
        title="原始记录"
        description="粘贴的聊天、截图、长段速记都先原样存在这里，再由后台整理成记忆。原文永不修改，整理有误可以「重新整理」。"
        actions={
          <div style={{ display: "flex", gap: 8 }}>
            <Button icon={<FileUp size={16} />} className="pa-icon-text" onClick={() => setImporting(true)}>批量导入聊天记录</Button>
            <Button type="primary" icon={<ClipboardPaste size={16} />} className="pa-icon-text" onClick={() => setCreating(true)}>粘贴整理</Button>
          </div>
        }
      />
      <Panel>
        <div className="pa-toolbar">
          <Segmented
            value={status}
            onChange={(value) => { setStatus(String(value)); setPage(1); }}
            options={[{ value: "", label: "全部" }, ...Object.entries(statusMeta).map(([value, item]) => ({ value, label: item.label }))]}
          />
          <div className="pa-toolbar-spacer" />
          {busy && <Status tone="sky" pulse>后台整理中，自动刷新</Status>}
          <Tooltip title="刷新"><Button icon={<RefreshCw size={15} />} onClick={() => reload()} /></Tooltip>
        </div>
        <Table<Note>
          rowKey="noteKey"
          scroll={{ x: 820 }}
          loading={loading}
          dataSource={data?.items ?? []}
          onRow={(note) => ({ onClick: () => setViewing(note), style: { cursor: "pointer" } })}
          pagination={{ current: page, pageSize: 20, total: data?.total ?? 0, onChange: setPage, hideOnSinglePage: true, showSizeChanger: false }}
          locale={{ emptyText: "还没有原始记录" }}
          columns={[
            {
              title: "内容",
              render: (_, n) => (
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontWeight: 550, lineHeight: 1.5 }}>{n.result?.summary || n.rawText.slice(0, 60) || `${n.attachments.length} 张截图`}</div>
                  <div style={{ fontSize: 12.5, color: "var(--pa-text-3)", marginTop: 3, display: "flex", gap: 8, alignItems: "center" }}>
                    <span>{channelLabel[n.channel] ?? n.channel}</span>
                    {n.attachments.length > 0 && <span>· {n.attachments.length} 张截图</span>}
                    {n.aboutPersons.length > 0 && <span>· 关于 {n.aboutPersons.length} 人</span>}
                  </div>
                </div>
              ),
            },
            {
              title: "整理结果",
              width: 280,
              render: (_, n) =>
                n.result ? (
                  <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                    <Pill tone="sage">新 {n.result.added?.length ?? 0}</Pill>
                    <Pill tone="amber">待确认 {n.result.pending?.length ?? 0}</Pill>
                    <Pill tone="sky">补充 {n.result.updated?.length ?? 0}</Pill>
                    <Pill>重复 {n.result.duplicates?.length ?? 0}</Pill>
                  </div>
                ) : <span className="pa-muted">—</span>,
            },
            { title: "状态", width: 110, render: (_, n) => <Status tone={statusMeta[n.extractStatus]?.tone} pulse={n.extractStatus === "running"}>{statusMeta[n.extractStatus]?.label}</Status> },
            { title: "提交时间", width: 150, render: (_, n) => <span className="pa-num" style={{ color: "var(--pa-text-2)" }}>{dayjs(n.createdAt).format("YYYY-MM-DD HH:mm")}</span> },
          ]}
        />
      </Panel>
      {viewing && <NoteDetail note={viewing} onClose={() => setViewing(null)} onChanged={reload} />}
      <CreateNote open={creating} onClose={() => setCreating(false)} onCreated={() => { setCreating(false); void reload(); }} />
      <ImportModal open={importing} onClose={() => setImporting(false)} onDone={() => { setImporting(false); setPage(1); void reload(); }} />
    </>
  );
}

function NoteDetail({ note, onClose, onChanged }: { note: Note; onClose: () => void; onChanged: () => void }) {
  const { message } = App.useApp();
  const [images, setImages] = useState<string[]>([]);
  const { data: memories } = useData(
    () => api.get<Paged<Memory>>(`/memories?source=${note.noteKey}&statuses=pending,active,superseded,archived,rejected&size=100`),
    [note.noteKey],
  );
  useEffect(() => {
    // 一张取不到不影响其它几张显示；抽屉关掉前就回来的图也要释放
    let alive = true;
    const urls: string[] = [];
    Promise.allSettled(note.attachments.map((_, index) => fetchObjectURL(`/notes/${note.noteKey}/attachments/${index}`))).then((results) => {
      const list = results.flatMap((r) => (r.status === "fulfilled" ? [r.value] : []));
      if (!alive) {
        list.forEach((url) => URL.revokeObjectURL(url));
        return;
      }
      urls.push(...list);
      setImages(list);
    });
    return () => {
      alive = false;
      urls.forEach((url) => URL.revokeObjectURL(url));
    };
  }, [note]);

  const retry = async () => {
    try {
      await api.post(`/notes/${note.noteKey}/retry`);
      message.success("已重新排队");
      onChanged();
      onClose();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  const result = note.result;
  return (
    <Drawer
      title="原始记录"
      width={760}
      open
      onClose={onClose}
      extra={(note.extractStatus === "done" || note.extractStatus === "failed") && <Button icon={<RotateCcw size={15} />} className="pa-icon-text" onClick={retry}>重新整理</Button>}
    >
      <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
        <Status tone={statusMeta[note.extractStatus]?.tone}>{statusMeta[note.extractStatus]?.label} · 第 {note.extractAttempts} 次</Status>
        <span className="pa-muted">·</span>
        <span style={{ color: "var(--pa-text-2)" }}>{channelLabel[note.channel] ?? note.channel}</span>
        <span className="pa-muted">·</span>
        <span className="pa-num" style={{ color: "var(--pa-text-2)" }}>{note.occurredAt ? `发生于 ${dayjs(note.occurredAt).format("YYYY-MM-DD HH:mm")}` : dayjs(note.createdAt).format("YYYY-MM-DD HH:mm")}</span>
      </div>
      {result?.summary && <h2 style={{ fontFamily: "var(--pa-font-display)", fontSize: 21, lineHeight: 1.5, margin: "12px 0 0" }}>{result.summary}</h2>}
      {note.aboutPersons.length > 0 && (
        <div style={{ display: "flex", gap: 14, flexWrap: "wrap", marginTop: 12 }}>
          {note.aboutPersons.map((key) => <PersonTag key={key} personKey={key} size={22} muted />)}
        </div>
      )}

      {note.extractError && note.extractStatus !== "done" && (
        <Callout tone="rose" icon={<CircleAlert size={16} />} style={{ marginTop: 16 }}>{note.extractError}</Callout>
      )}
      {!!result?.unknownPersons?.length && (
        <Callout tone="amber" icon={<UserRoundX size={16} />} style={{ marginTop: 16 }}>
          认不出的人：<b>{result.unknownPersons.join("、")}</b>。到「家庭成员」里添加（别名写上原文里的叫法），再点「重新整理」。
        </Callout>
      )}

      {result && (
        <div className="pa-kpis" style={{ marginTop: 20, gridTemplateColumns: "repeat(4, 1fr)" }}>
          {[["新记下", result.added?.length ?? 0, "sage"], ["待确认", result.pending?.length ?? 0, "amber"], ["补充已有", result.updated?.length ?? 0, "sky"], ["已记过", result.duplicates?.length ?? 0, "slate"]].map(([label, value, tone]) => (
            <div key={label as string} className="pa-kpi" data-tone={tone as Tone}>
              <div className="pa-kpi-value">{value}</div>
              <div className="pa-kpi-label"><span className="pa-dot" />{label}</div>
            </div>
          ))}
        </div>
      )}

      <div className="pa-section-label">整理出的记忆</div>
      <div className="pa-panel">
        <Table<Memory>
          rowKey="memoryKey"
          size="small"
          pagination={false}
          dataSource={memories?.items ?? []}
          locale={{ emptyText: note.extractStatus === "done" ? "这条记录里没有值得长期记住的内容" : "还没整理完" }}
          columns={[
            {
              title: "内容",
              render: (_, m) => (
                <div style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
                  <Pill tone={typeTone(m.type)}>{m.typeLabel}</Pill>
                  <span style={{ lineHeight: 1.5 }}>{m.title}</span>
                </div>
              ),
            },
            { title: "关于", width: 140, render: (_, m) => <PersonTag personKey={m.subjectPerson} size={20} /> },
            { title: "置信度", width: 80, render: (_, m) => <span className="pa-num">{m.confidence.toFixed(2)}</span> },
            { title: "状态", width: 90, render: (_, m) => <Status tone={memoryStatus[m.status]?.tone}>{memoryStatus[m.status]?.label}</Status> },
          ]}
        />
      </div>

      {images.length > 0 && (
        <>
          <div className="pa-section-label">截图</div>
          <Image.PreviewGroup>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(120px, 1fr))", gap: 10 }}>
              {images.map((url) => (
                <Image key={url} src={url} width="100%" height={120} style={{ objectFit: "cover", borderRadius: 10 }} />
              ))}
            </div>
          </Image.PreviewGroup>
        </>
      )}
      {note.rawText && (
        <>
          <div className="pa-section-label">原文</div>
          <pre className="pa-code" style={{ fontFamily: "var(--pa-font)", fontSize: 14, color: "var(--pa-text)", lineHeight: 1.75 }}>{note.rawText}</pre>
        </>
      )}
    </Drawer>
  );
}

function CreateNote({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: () => void }) {
  const { persons, personName } = useConsole();
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [files, setFiles] = useState<UploadFile[]>([]);
  const [saving, setSaving] = useState(false);

  const submit = async () => {
    const values = await form.validateFields();
    if (!values.text?.trim() && files.length === 0) {
      message.warning("粘贴内容或上传截图");
      return;
    }
    const data = new FormData();
    data.set("text", values.text ?? "");
    data.set("channel", files.length ? "screenshot" : "manager");
    data.set("aboutPersons", (values.aboutPersons ?? []).join(","));
    if (values.occurredAt) data.set("occurredAt", values.occurredAt.toISOString());
    files.forEach((file) => file.originFileObj && data.append("images", file.originFileObj, file.name));
    setSaving(true);
    try {
      await postForm("/notes", data);
      message.success("已提交整理");
      form.resetFields();
      setFiles([]);
      onCreated();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal title="粘贴整理" open={open} onCancel={onClose} onOk={submit} confirmLoading={saving} width={680} okText="交给助理整理" destroyOnClose>
      <Form form={form} layout="vertical" initialValues={{ aboutPersons: ["wife"], occurredAt: dayjs() }} style={{ marginTop: 16 }}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 220px", gap: 16 }}>
          <Form.Item name="aboutPersons" label="和谁的聊天 / 关于谁">
            <Select mode="multiple" maxTagCount="responsive" options={persons.filter((p) => p.personKey !== "me").map((p) => ({ value: p.personKey, label: personName(p.personKey) }))} />
          </Form.Item>
          <Form.Item name="occurredAt" label="大概什么时候的事" tooltip="原文里的「昨天」「下周」按这个时间换算">
            <DatePicker showTime={{ format: "HH:mm" }} format="YYYY-MM-DD HH:mm" style={{ width: "100%" }} />
          </Form.Item>
        </div>
        <Form.Item name="text" label="内容">
          <Input.TextArea autoSize={{ minRows: 8, maxRows: 18 }} placeholder="粘贴聊天记录或写下发生的事，越原汁原味越好" />
        </Form.Item>
        <Form.Item label="截图" extra="JPG / PNG，单张 5MB 以内，最多 9 张">
          <Upload listType="picture-card" accept="image/png,image/jpeg,image/webp" multiple fileList={files}
            beforeUpload={() => false} onChange={({ fileList }) => setFiles(fileList.slice(0, 9))}>
            {files.length < 9 && <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 6, color: "var(--pa-text-3)" }}><ImagePlus size={20} /><span style={{ fontSize: 12 }}>添加</span></div>}
          </Upload>
        </Form.Item>
      </Form>
    </Modal>
  );
}

/** 批量导入：一大段导出的聊天记录按行切块，每块一条原始记录，后台慢慢整理。 */
function ImportModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const { message } = App.useApp();
  const { persons } = useConsole();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  const readFile = async (file: File) => {
    form.setFieldValue("text", await file.text());
    if (!form.getFieldValue("note")) form.setFieldValue("note", file.name.replace(/\.[^.]+$/, ""));
    return false;
  };
  const submit = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      const result = await api.post<{ notes: number }>("/notes/import", values);
      message.success(`已切成 ${result.notes} 段，后台会逐段整理`);
      form.resetFields();
      onDone();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };
  return (
    <Modal title="批量导入聊天记录" open={open} onCancel={onClose} onOk={submit} okText="开始导入" confirmLoading={saving} width={720} destroyOnClose>
      <div className="pa-muted" style={{ fontSize: 13, marginBottom: 12 }}>
        比如从微信导出的和老婆、妈妈的聊天记录（纯文本）。会按行切成每段约 6000 字，逐段整理成记忆；整理出的和已有记忆冲突的进「待确认」。一次最多 30 万字。
      </div>
      <Form form={form} layout="vertical" requiredMark={false}>
        <Form.Item name="note" label="说明" extra="会放在每段开头，帮助理解，比如「和老婆的微信，2025 年」">
          <Input />
        </Form.Item>
        <Form.Item name="aboutPersons" label="主要关于谁">
          <Select mode="multiple" options={persons.map((p) => ({ value: p.personKey, label: p.roleLabel }))} />
        </Form.Item>
        <Form.Item label="从文件读取">
          <Upload accept=".txt,.csv,.md" maxCount={1} showUploadList={false} beforeUpload={readFile}>
            <Button icon={<FileUp size={15} />} className="pa-icon-text">选择文本文件</Button>
          </Upload>
        </Form.Item>
        <Form.Item name="text" label="内容" rules={[{ required: true, message: "粘贴或选择文件" }]}>
          <Input.TextArea autoSize={{ minRows: 8, maxRows: 16 }} />
        </Form.Item>
      </Form>
    </Modal>
  );
}
