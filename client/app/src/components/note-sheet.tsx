"use client";

import { useEffect, useState, type ReactNode } from "react";
import dayjs from "dayjs";
import { ImageViewer, Toast } from "antd-mobile";
import { CircleAlert, CircleCheck, Clock, LoaderCircle, RotateCcw, UserRoundX } from "lucide-react";
import { api, fetchObjectURL } from "@/lib/api";
import { typeTone, type Tone } from "@/lib/tone";
import type { Memory, Note, Paged } from "@/lib/types";
import { Button, Card, Pill, Row, Section, Sheet, useLatest } from "./ui";

export const noteStatus: Record<string, { text: string; tone: Tone; icon: ReactNode }> = {
  queued: { text: "排队中", tone: "slate", icon: <Clock size={16} /> },
  running: { text: "整理中", tone: "sky", icon: <LoaderCircle size={16} /> },
  done: { text: "已整理", tone: "sage", icon: <CircleCheck size={16} /> },
  failed: { text: "失败", tone: "rose", icon: <CircleAlert size={16} /> },
};

const memoryStatusText: Record<string, string> = { pending: "待确认", active: "已记下", superseded: "已过时", archived: "已归档", rejected: "已驳回" };

/** 一条原始记录的详情：原文、截图、整理结果、由它生成的记忆。 */
export function NoteSheet({ note: current, onClose, onChanged, onMemory }: { note: Note | null; onClose: () => void; onChanged: () => void; onMemory: (key: string) => void }) {
  const note = useLatest(current);
  const [memories, setMemories] = useState<Memory[]>([]);
  const [imageUrls, setImageUrls] = useState<string[]>([]);
  const [preview, setPreview] = useState<number | null>(null);

  useEffect(() => {
    if (!note) return;
    setMemories([]);
    setImageUrls([]);
    api.get<Paged<Memory>>(`/memories?source=${note.noteKey}&statuses=pending,active,superseded,rejected,archived&size=100`).then((page) => setMemories(page.items)).catch(() => {});
    // 一张取不到不影响其它几张显示；弹层关掉前就回来的图也要释放
    let alive = true;
    const urls: string[] = [];
    Promise.allSettled(note.attachments.map((_, index) => fetchObjectURL(`/notes/${note.noteKey}/attachments/${index}`))).then((results) => {
      const list = results.flatMap((r) => (r.status === "fulfilled" ? [r.value] : []));
      if (!alive) {
        list.forEach((url) => URL.revokeObjectURL(url));
        return;
      }
      urls.push(...list);
      setImageUrls(list);
    });
    return () => {
      alive = false;
      urls.forEach((url) => URL.revokeObjectURL(url));
    };
  }, [note]);

  const retry = async () => {
    if (!note) return;
    try {
      await api.post(`/notes/${note.noteKey}/retry`);
      Toast.show({ content: "已重新排队" });
      onChanged();
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    }
  };

  const result = note?.result;
  const status = note ? noteStatus[note.extractStatus] : undefined;
  const stats = result
    ? [
        ["新记下", result.added?.length ?? 0],
        ["待确认", result.pending?.length ?? 0],
        ["补充", result.updated?.length ?? 0],
        ["已记过", result.duplicates?.length ?? 0],
      ] as const
    : [];

  return (
    <Sheet open={!!current} onClose={onClose} title="整理结果" height="82vh">
      {note && (
        <div>
          <div style={{ padding: "6px 20px 0", display: "flex", alignItems: "center", gap: 8 }}>
            <Pill tone={status?.tone} dot>{status?.text}</Pill>
            <span style={{ fontSize: 12.5, color: "var(--pa-text-3)" }}>{dayjs(note.createdAt).format("M月D日 HH:mm")} 提交</span>
          </div>
          {result?.summary && <div style={{ padding: "12px 20px 0", fontSize: 17, fontWeight: 600, lineHeight: 1.5 }}>{result.summary}</div>}

          {note.extractError && note.extractStatus !== "done" && (
            <div className="sheet-alert" data-tone="rose"><CircleAlert size={16} />{note.extractError}</div>
          )}
          {!!result?.unknownPersons?.length && (
            <div className="sheet-alert" data-tone="amber">
              <UserRoundX size={16} />
              <span>认不出的人：<b>{result.unknownPersons.join("、")}</b>。到管理端「家庭成员」里加上后，再点「重新整理」。</span>
            </div>
          )}

          {stats.length > 0 && (
            <div className="sheet-stats">
              {stats.map(([label, value]) => (
                <div key={label}>
                  <div className="sheet-stat-value">{value}</div>
                  <div className="sheet-stat-label">{label}</div>
                </div>
              ))}
            </div>
          )}

          {memories.length > 0 && (
            <Section title="整理出的记忆" count={memories.length} style={{ marginTop: 20 }}>
              <Card>
                {memories.map((memory) => (
                  <Row
                    key={memory.memoryKey}
                    leading={<Pill tone={typeTone(memory.type)}>{memory.typeLabel}</Pill>}
                    title={memory.title}
                    trailing={<span style={{ color: memory.status === "pending" ? "var(--pa-warning)" : undefined }}>{memoryStatusText[memory.status]}</span>}
                    onClick={() => onMemory(memory.memoryKey)}
                  />
                ))}
              </Card>
            </Section>
          )}

          {imageUrls.length > 0 && (
            <Section title="截图" style={{ marginTop: 20 }}>
              <div style={{ padding: "0 16px", display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 8 }}>
                {imageUrls.map((url, index) => (
                  <img key={url} src={url} alt="" onClick={() => setPreview(index)} style={{ width: "100%", aspectRatio: "1", objectFit: "cover", borderRadius: 12, cursor: "pointer" }} />
                ))}
              </div>
              <ImageViewer.Multi getContainer={() => document.body} images={imageUrls} visible={preview !== null} defaultIndex={preview ?? 0} onClose={() => setPreview(null)} />
            </Section>
          )}

          {note.rawText && (
            <Section title="原文" style={{ marginTop: 20 }}>
              <Card pad>
                <div className="pa-pre" style={{ fontSize: 14, lineHeight: 1.7, color: "var(--pa-text-2)", maxHeight: 260, overflowY: "auto" }}>{note.rawText}</div>
              </Card>
            </Section>
          )}

          {(note.extractStatus === "failed" || note.extractStatus === "done") && (
            <div style={{ padding: "24px 20px 0" }}>
              <Button block variant="soft" size="md" onClick={retry}><RotateCcw size={16} />重新整理</Button>
            </div>
          )}
        </div>
      )}
      <style jsx global>{`
        .sheet-alert {
          margin: 12px 16px 0;
          padding: 10px 12px;
          border-radius: 12px;
          display: flex;
          gap: 8px;
          align-items: flex-start;
          font-size: 13.5px;
          line-height: 1.55;
          background: var(--tone-bg);
          color: var(--pa-text);
        }
        .sheet-alert svg { color: var(--tone-fg); flex-shrink: 0; margin-top: 2px; }
        .sheet-stats {
          margin: 16px 16px 0;
          display: grid;
          grid-template-columns: repeat(4, 1fr);
          background: var(--pa-surface);
          border: 0.5px solid var(--pa-line);
          border-radius: var(--pa-radius);
          padding: 12px 0;
        }
        .sheet-stats > div { text-align: center; }
        .sheet-stats > div + div { border-left: 0.5px solid var(--pa-line); }
        .sheet-stat-value { font-size: 22px; font-weight: 700; font-variant-numeric: tabular-nums; }
        .sheet-stat-label { font-size: 12px; color: var(--pa-text-3); margin-top: 2px; }
      `}</style>
    </Sheet>
  );
}
