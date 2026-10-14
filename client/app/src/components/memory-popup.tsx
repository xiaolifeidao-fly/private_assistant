"use client";

import { useEffect, useState } from "react";
import dayjs from "dayjs";
import { Quote } from "lucide-react";
import { api } from "@/lib/api";
import { personLabel, usePersons } from "@/lib/family";
import { personTone, typeTone } from "@/lib/tone";
import type { Memory } from "@/lib/types";
import { PhotoStrip } from "./photos";
import { Loading, Pill, Sheet } from "./ui";

const statusText: Record<string, string> = { pending: "待确认", active: "生效中", superseded: "已过时", archived: "已归档", rejected: "已驳回" };

/** 点开助理回答里引用的记忆，核对原话和时间。 */
export function MemoryPopup({ memoryKey, onClose }: { memoryKey: string | null; onClose: () => void }) {
  const persons = usePersons();
  const [memory, setMemory] = useState<Memory | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!memoryKey) return;
    setMemory(null);
    setError("");
    api.get<Memory>(`/memories/${memoryKey}`).then(setMemory).catch((e) => setError((e as Error).message));
  }, [memoryKey]);

  const isQuote = memory?.type === "quote";
  const attrs = (memory?.attrs ?? {}) as { location?: string; usage?: string; photos?: { note: string; index: number }[] };
  return (
    <Sheet open={!!memoryKey} onClose={onClose} title="记忆">
      {!memory && !error && <Loading />}
      {error && <div style={{ padding: 20, color: "var(--pa-text-3)" }}>{error}</div>}
      {memory && (
        <div style={{ padding: "4px 20px 8px" }}>
          <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
            <Pill tone={typeTone(memory.type)}>{memory.typeLabel}</Pill>
            <Pill tone={personTone(memory.subjectPerson, persons)} dot>{memory.subjectPerson ? personLabel(persons, memory.subjectPerson) : "全家"}</Pill>
            {memory.status !== "active" && <Pill>{statusText[memory.status]}</Pill>}
          </div>
          <div style={{ fontFamily: "var(--pa-font-display)", fontWeight: 700, fontSize: 21, lineHeight: 1.45, marginTop: 14 }}>{memory.title}</div>
          {memory.content !== memory.title && (
            <div className="memory-body" data-quote={isQuote}>
              {isQuote && <Quote size={18} className="memory-quote-mark" />}
              <div className="pa-pre">{memory.content}</div>
            </div>
          )}
          {!!attrs.photos?.length && <PhotoStrip photos={attrs.photos.map((photo) => ({ noteKey: photo.note, index: photo.index }))} />}
          <div className="memory-meta">
            {attrs.location && <div><span>放在</span><b style={{ textAlign: "right", maxWidth: "75%" }}>{attrs.location}</b></div>}
            {attrs.usage && <div><span>用途</span><span style={{ textAlign: "right", maxWidth: "75%" }}>{attrs.usage}</span></div>}
            <div><span>{memory.type === "item" ? "记录于" : "发生在"}</span>{dayjs(memory.occurredAt ?? memory.createdAt).format("YYYY年M月D日 HH:mm")}</div>
            <div><span>重要程度</span>
              <span className="memory-dots">{Array.from({ length: 5 }, (_, i) => <i key={i} data-on={i < memory.importance} />)}</span>
            </div>
          </div>
        </div>
      )}
      <style jsx global>{`
        .memory-body {
          margin-top: 14px;
          font-size: 15.5px;
          line-height: 1.75;
          color: var(--pa-text);
          position: relative;
        }
        .memory-body[data-quote="true"] {
          padding: 14px 16px 14px 42px;
          background: var(--pa-surface);
          border: 0.5px solid var(--pa-line);
          border-radius: var(--pa-radius);
          font-family: var(--pa-font-display);
          font-size: 16.5px;
        }
        .memory-quote-mark { position: absolute; left: 14px; top: 16px; color: var(--tone-plum); }
        .memory-meta {
          margin-top: 18px;
          border-top: 0.5px solid var(--pa-line);
          padding-top: 12px;
          display: flex;
          flex-direction: column;
          gap: 8px;
          font-size: 13.5px;
        }
        .memory-meta > div { display: flex; justify-content: space-between; align-items: center; }
        .memory-meta > div > span:first-child { color: var(--pa-text-3); }
        .memory-dots { display: inline-flex; gap: 4px; }
        .memory-dots i { width: 8px; height: 8px; border-radius: 4px; background: var(--pa-fill-strong); }
        .memory-dots i[data-on="true"] { background: var(--pa-accent); }
      `}</style>
    </Sheet>
  );
}
