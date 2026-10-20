"use client";

import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import dayjs from "dayjs";
import { App, Button, Input, Segmented, Upload } from "antd";
import { ImagePlus, Send, Sparkles, X } from "lucide-react";
import { amountOf, cellKey, DAY, dietApi, mealLabels, recordDate, weekdays, type PlanTurn } from "@/lib/diet";
import { VoiceInput } from "@/components/voice-input";

type Mode = "plan" | "record";

interface Message {
  id: number;
  from: "me" | "bot";
  mode: Mode;
  text: string;
  photos?: number;
  /** 模型改了 / 记了哪些：一行一条 */
  lines?: string[];
  error?: boolean;
}

const examples: Record<Mode, string[]> = {
  plan: ["这周每天早上一个鸡蛋、一杯奶", "周三晚饭吃清蒸鱼，少盐", "按她最近吃的情况把下周排一下", "周五的计划取消"],
  record: ["今天午饭吃了米饭、西兰花、虾仁，吃了大半", "昨天晚饭面条只吃了几口，有点咳嗽"],
};

function dayLabel(date: string) {
  const day = dayjs(date);
  return `${day.format("M/D")} 周${weekdays[day.day()]}`;
}

/** 右侧的对话框：说一句话（打字 / 说话 / 拍照），交给模型改计划或记实际，改完直接生效。 */
export function DietAssistant({ child, from, onChanged }: { child: string; from: string; onChanged: (keys: string[]) => void }) {
  const { message } = App.useApp();
  const [mode, setMode] = useState<Mode>("plan");
  const [text, setText] = useState("");
  const [images, setImages] = useState<File[]>([]);
  const [previews, setPreviews] = useState<string[]>([]);
  const [sending, setSending] = useState(false);
  const [messages, setMessages] = useState<Message[]>([]);
  const listRef = useRef<HTMLDivElement>(null);
  const seq = useRef(0);

  useEffect(() => {
    const urls = images.map((image) => URL.createObjectURL(image));
    setPreviews(urls);
    return () => urls.forEach((url) => URL.revokeObjectURL(url));
  }, [images]);
  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight, behavior: "smooth" });
  }, [messages, sending]);

  const push = (item: Omit<Message, "id">) => setMessages((list) => [...list, { ...item, id: ++seq.current }]);

  const send = async (value = text) => {
    const content = value.trim();
    if ((!content && images.length === 0) || sending) return;
    const photos = images;
    push({ from: "me", mode, text: content, photos: photos.length });
    setText("");
    setImages([]);
    setSending(true);
    try {
      if (mode === "plan") {
        // 前面几轮改计划的对话一起带上，能接着说「周五也这样」「对，就这么排」
        const history: PlanTurn[] = messages
          .filter((item) => item.mode === "plan" && !item.error)
          .map((item) => ({ role: item.from === "me" ? "parent" : "assistant", text: item.text || `（拍了 ${item.photos ?? 0} 张照片）` }));
        const result = await dietApi.capturePlan(child, content, photos, from, 7, history);
        push({
          from: "bot",
          mode,
          text: result.reply,
          lines: result.changed.map((p) => {
            const meal = p.meal === DAY ? "全天" : mealLabels[p.meal] ?? p.meal;
            return `${dayLabel(p.date)} ${meal}：${p.foods || p.note ? [p.foods, p.note].filter(Boolean).join("；") : "取消"}`;
          }),
        });
        onChanged(result.changed.map((p) => cellKey(p.date, p.meal)));
      } else {
        const result = await dietApi.captureRecord(child, content, photos);
        push({
          from: "bot",
          mode,
          text: result.reply + (result.fallback ? "（模型没整理出来，先按原话记了一条，可以点格子改）" : ""),
          lines: result.records.map((r) => {
            const amount = amountOf(r.amount);
            return `${dayLabel(recordDate(r))} ${mealLabels[r.meal] ?? r.meal} ${dayjs(r.eatenAt).format("HH:mm")}：${r.foods}${amount ? `（${amount.label}）` : ""}`;
          }),
        });
        onChanged(result.records.map((r) => cellKey(recordDate(r), r.meal)));
      }
    } catch (error) {
      push({ from: "bot", mode, text: (error as Error).message, error: true });
    } finally {
      setSending(false);
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && (event.metaKey || event.ctrlKey) && !event.nativeEvent.isComposing) {
      event.preventDefault();
      void send();
    }
  };

  return (
    <div className="pa-panel diet-chat">
      <div className="pa-panel-head">
        <div className="pa-panel-title"><Sparkles size={16} color="var(--pa-accent)" />说一句，让模型改</div>
        {messages.length > 0 && (
          <Button type="text" size="small" style={{ marginLeft: "auto" }} disabled={sending} onClick={() => setMessages([])}>重新开始</Button>
        )}
      </div>
      <div style={{ padding: "12px 16px 0" }}>
        <Segmented block value={mode} onChange={(value) => setMode(value as Mode)}
          options={[{ value: "plan", label: "改计划" }, { value: "record", label: "记实际" }]} />
      </div>

      <div className="diet-chat-list" ref={listRef}>
        {messages.length === 0 && (
          <div className="diet-chat-empty">
            <div style={{ marginBottom: 10 }}>
              {mode === "plan"
                ? "说怎么安排，或者拍一张幼儿园食谱。模型会参考这周已有的计划和最近两周实际吃的，只改你说到的餐，直接生效。"
                : "说一下吃了什么、吃了多少，或者拍张照。模型按餐整理成记录，直接生效。"}
            </div>
            {examples[mode].map((example) => (
              <button key={example} className="diet-chip" onClick={() => setText(example)}>{example}</button>
            ))}
          </div>
        )}
        {messages.map((item) => (
          <div key={item.id} className="diet-msg" data-from={item.from} data-error={item.error}>
            {item.from === "me" && <div className="diet-msg-tag">{item.mode === "plan" ? "改计划" : "记实际"}</div>}
            <div className="diet-bubble">
              {item.text || (item.photos ? "（照片）" : "")}
              {item.from === "me" && !!item.photos && item.text && <div className="diet-msg-photos">附 {item.photos} 张照片</div>}
              {!!item.lines?.length && (
                <ul className="diet-msg-lines">
                  {item.lines.map((line, i) => <li key={i}>{line}</li>)}
                </ul>
              )}
            </div>
          </div>
        ))}
        {sending && (
          <div className="diet-msg" data-from="bot">
            <div className="diet-bubble diet-typing"><i /><i /><i /></div>
          </div>
        )}
      </div>

      <div className="diet-chat-input">
        {previews.length > 0 && (
          <div className="diet-previews">
            {previews.map((url, index) => (
              <span key={url} className="diet-preview">
                <img src={url} alt="" />
                <button onClick={() => setImages((list) => list.filter((_, i) => i !== index))} aria-label="移除"><X size={11} /></button>
              </span>
            ))}
          </div>
        )}
        <Input.TextArea
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          autoSize={{ minRows: 2, maxRows: 6 }}
          placeholder={mode === "plan" ? "比如：下周一到周五早饭都加一个鸡蛋" : "比如：午饭吃了米饭和番茄炒蛋，吃完了"}
          disabled={sending}
        />
        <div className="diet-chat-actions">
          <Upload
            accept="image/*"
            multiple
            showUploadList={false}
            beforeUpload={(file) => {
              if (file.size > 10 << 20) message.warning("照片太大了（超过 10MB）");
              else setImages((list) => [...list, file].slice(0, 4));
              return false;
            }}
          >
            <Button icon={<ImagePlus size={15} />} className="pa-icon-text" disabled={sending}>照片</Button>
          </Upload>
          <VoiceInput disabled={sending} onText={(value) => setText((old) => (old ? `${old}\n${value}` : value))} onSend={(value) => void send(value)} />
          <span style={{ flex: 1 }} />
          <Button type="primary" icon={<Send size={15} />} className="pa-icon-text" loading={sending}
            disabled={!text.trim() && images.length === 0} onClick={() => void send()}>发送</Button>
        </div>
        <div className="pa-muted" style={{ fontSize: 11.5, marginTop: 6 }}>⌘ / Ctrl + 回车发送 · 整理错了点格子改</div>
      </div>

      <style jsx global>{`
        .diet-chat { position: sticky; top: 24px; display: flex; flex-direction: column; max-height: calc(100vh - 48px); }
        @media (max-width: 1240px) { .diet-chat { position: static; max-height: none; } }
        .diet-chat-list { flex: 1; min-height: 220px; overflow-y: auto; padding: 14px 16px; display: flex; flex-direction: column; gap: 12px; }
        .diet-chat-empty { color: var(--pa-text-2); font-size: 13px; line-height: 1.7; }
        .diet-chip {
          display: block; width: 100%; text-align: left; margin-top: 6px; padding: 7px 10px; border-radius: 9px; cursor: pointer;
          background: var(--pa-fill); border: 1px solid var(--pa-line); color: var(--pa-text); font: inherit; font-size: 12.5px;
        }
        .diet-chip:hover { border-color: var(--pa-accent); background: var(--pa-accent-soft); }
        .diet-msg { display: flex; flex-direction: column; align-items: flex-start; }
        .diet-msg[data-from="me"] { align-items: flex-end; }
        .diet-msg-tag { font-size: 11px; color: var(--pa-text-3); margin-bottom: 3px; }
        .diet-bubble { max-width: 92%; padding: 9px 12px; border-radius: 12px; font-size: 13.5px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; background: var(--pa-surface-2); border: 1px solid var(--pa-line); }
        .diet-msg[data-from="me"] .diet-bubble { background: var(--pa-accent-soft); border-color: transparent; }
        .diet-msg[data-error="true"] .diet-bubble { background: var(--pa-danger-soft); color: var(--pa-danger); }
        .diet-msg-photos { font-size: 11.5px; color: var(--pa-text-3); margin-top: 2px; }
        .diet-msg-lines { margin: 8px 0 0; padding: 8px 0 0 16px; border-top: 1px solid var(--pa-line); font-size: 12.5px; color: var(--pa-text-2); }
        .diet-msg-lines li + li { margin-top: 3px; }
        .diet-typing { display: inline-flex; gap: 4px; padding: 12px 14px; }
        .diet-typing i { width: 6px; height: 6px; border-radius: 50%; background: var(--pa-text-3); animation: diet-bounce 1.2s infinite ease-in-out; }
        .diet-typing i:nth-child(2) { animation-delay: 0.15s; }
        .diet-typing i:nth-child(3) { animation-delay: 0.3s; }
        @keyframes diet-bounce { 0%, 80%, 100% { opacity: 0.3; transform: translateY(0); } 40% { opacity: 1; transform: translateY(-3px); } }
        .diet-chat-input { border-top: 1px solid var(--pa-line); padding: 12px 16px 14px; }
        .diet-chat-actions { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
        .diet-previews { display: flex; gap: 6px; margin-bottom: 8px; }
        .diet-preview { position: relative; }
        .diet-preview img { width: 52px; height: 52px; object-fit: cover; border-radius: 8px; display: block; }
        .diet-preview button { position: absolute; top: -5px; right: -5px; width: 18px; height: 18px; border-radius: 50%; border: none; background: var(--pa-text); color: var(--pa-surface); display: grid; place-items: center; cursor: pointer; padding: 0; }
      `}</style>
    </div>
  );
}
