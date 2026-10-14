"use client";

import { useEffect, useRef, useState } from "react";
import { App, Button, Drawer, Input, Spin } from "antd";
import { ArrowLeft, FileText, RotateCcw, Send } from "lucide-react";
import { api, streamPost } from "@/lib/api";
import type { PersonaCard, PersonaTurn } from "@/lib/types";
import { Callout } from "@/components/ui";
import { VoiceInput } from "@/components/voice-input";

type StreamEvent = { type: "delta" | "done" | "error"; text?: string; error?: string };

// 对话只存在本机浏览器里：关掉抽屉、刷新页面都能接着聊，存成新版本后清掉。
const storageKey = (personKey: string) => `pa_persona_interview_${personKey}`;

function loadTurns(personKey: string): PersonaTurn[] {
  try {
    const raw = localStorage.getItem(storageKey(personKey));
    return raw ? (JSON.parse(raw) as PersonaTurn[]) : [];
  } catch {
    return [];
  }
}

function storeTurns(personKey: string, turns: PersonaTurn[]) {
  try {
    if (turns.length) localStorage.setItem(storageKey(personKey), JSON.stringify(turns));
    else localStorage.removeItem(storageKey(personKey));
  } catch {
    /* 隐私模式下 localStorage 可能不可用 */
  }
}

export function InterviewDrawer({ personKey, roleLabel, open, onClose, onSaved }: {
  personKey: string;
  roleLabel: string;
  open: boolean;
  onClose: () => void;
  onSaved: (card: PersonaCard) => void;
}) {
  const { message, modal } = App.useApp();
  const [turns, setTurns] = useState<PersonaTurn[]>(() => loadTurns(personKey));
  const [streaming, setStreaming] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [draft, setDraft] = useState<{ content: string; changes: string } | null>(null);
  const [drafting, setDrafting] = useState(false);
  const [saving, setSaving] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const busy = streaming !== null || drafting;

  useEffect(() => storeTurns(personKey, turns), [personKey, turns]);
  useEffect(() => () => abortRef.current?.abort(), []);
  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight });
  }, [turns, streaming]);

  const ask = async (history: PersonaTurn[]) => {
    const controller = new AbortController();
    abortRef.current = controller;
    setStreaming("");
    let reply = "";
    let failed = "";
    try {
      await streamPost<StreamEvent>("/persona-cards/interview", { person: personKey, turns: history }, (event) => {
        if (event.type === "delta") {
          reply += event.text ?? "";
          setStreaming(reply);
        } else if (event.type === "done") {
          reply = event.text ?? reply;
        } else if (event.type === "error") {
          failed = event.error || "模型调用失败";
        }
      }, controller.signal);
    } catch (error) {
      if ((error as Error).name !== "AbortError") failed = (error as Error).message;
    } finally {
      setStreaming(null);
      abortRef.current = null;
    }
    if (failed) {
      message.error(failed, 8);
      return;
    }
    if (reply.trim()) setTurns([...history, { role: "assistant", text: reply.trim() }]);
  };

  // 打开时还没聊过，就让模型先开口
  useEffect(() => {
    if (open && turns.length === 0 && streaming === null) void ask([]);
  }, [open]);

  // spoken 是语音识别出的文字（「直接发送」时接在输入框已有内容后面一起发）
  const send = (spoken?: string) => {
    const text = (spoken === undefined ? input : `${input.trim()}${spoken}`).trim();
    if (!text || busy) return;
    const history: PersonaTurn[] = [...turns, { role: "user", text }];
    setTurns(history);
    setInput("");
    void ask(history);
  };

  const restart = () => {
    modal.confirm({
      title: "清空这段对话重新开始？",
      content: "还没保存的内容会丢掉。",
      okText: "清空",
      onOk: () => {
        abortRef.current?.abort();
        setDraft(null);
        setTurns([]);
        void ask([]);
      },
    });
  };

  const makeDraft = async () => {
    setDrafting(true);
    try {
      setDraft(await api.post<{ content: string; changes: string }>("/persona-cards/interview/draft", { person: personKey, turns }));
    } catch (error) {
      message.error((error as Error).message, 8);
    } finally {
      setDrafting(false);
    }
  };

  const save = async () => {
    if (!draft?.content.trim()) {
      message.warning("人设卡内容不能为空");
      return;
    }
    setSaving(true);
    try {
      const card = await api.post<PersonaCard>("/persona-cards/interview/save", { person: personKey, ...draft });
      message.success(`已保存为 v${card.version}`);
      setDraft(null);
      setTurns([]);
      onSaved(card);
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const hasAnswer = turns.some((turn) => turn.role === "user");

  return (
    <Drawer
      title={`对话补充 · ${roleLabel}的人设卡`}
      open={open}
      onClose={onClose}
      width={720}
      styles={{ body: { padding: 0, display: "flex", flexDirection: "column" } }}
      extra={!draft && <Button icon={<RotateCcw size={15} />} className="pa-icon-text" disabled={busy || turns.length === 0} onClick={restart}>重新开始</Button>}
    >
      {draft ? (
        <div style={{ padding: "16px 24px 24px", display: "flex", flexDirection: "column", gap: 12, flex: 1 }}>
          <Callout tone="sky">这是根据对话、记忆和当前人设卡整理的草稿，可以直接改。保存后成为新的当前版本，原来的版本留在版本记录里。</Callout>
          <Input placeholder="这次补充了什么（可选）" value={draft.changes} onChange={(e) => setDraft({ ...draft, changes: e.target.value })} />
          <Input.TextArea value={draft.content} onChange={(e) => setDraft({ ...draft, content: e.target.value })} autoSize={{ minRows: 18 }} style={{ lineHeight: 1.8 }} />
          <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
            <Button icon={<ArrowLeft size={15} />} className="pa-icon-text" onClick={() => setDraft(null)}>继续聊</Button>
            <Button type="primary" loading={saving} onClick={save}>保存为新版本</Button>
          </div>
        </div>
      ) : (
        <>
          <div ref={listRef} style={{ flex: 1, overflowY: "auto", padding: "20px 24px", display: "flex", flexDirection: "column", gap: 14 }}>
            {turns.map((turn, index) => <Bubble key={index} turn={turn} />)}
            {streaming !== null && (streaming ? <Bubble turn={{ role: "assistant", text: streaming }} /> : <div style={{ padding: 8 }}><Spin size="small" /></div>)}
          </div>
          <div style={{ borderTop: "1px solid var(--pa-line)", padding: "14px 24px 18px", display: "flex", flexDirection: "column", gap: 10 }}>
            <Input.TextArea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  send();
                }
              }}
              placeholder="回答它的问题，或者直接说你想补充的（Enter 发送，Shift+Enter 换行）"
              autoSize={{ minRows: 2, maxRows: 8 }}
            />
            <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
              <VoiceInput disabled={busy} onText={(text) => setInput((current) => (current ? `${current}${text}` : text))} onSend={send} />
              <span style={{ fontSize: 12.5, color: "var(--pa-text-3)" }}>对话只存在这台电脑的浏览器里，保存成人设卡后清空。</span>
              <Button style={{ marginLeft: "auto" }} icon={<FileText size={15} />} className="pa-icon-text" disabled={!hasAnswer || streaming !== null} loading={drafting} onClick={makeDraft}>生成草稿</Button>
              <Button type="primary" icon={<Send size={15} />} className="pa-icon-text" disabled={!input.trim() || busy} onClick={() => send()}>发送</Button>
            </div>
          </div>
        </>
      )}
    </Drawer>
  );
}

function Bubble({ turn }: { turn: PersonaTurn }) {
  const mine = turn.role === "user";
  return (
    <div style={{ display: "flex", justifyContent: mine ? "flex-end" : "flex-start" }}>
      <div
        className="pa-pre"
        style={{
          maxWidth: "82%",
          padding: "10px 14px",
          borderRadius: 12,
          lineHeight: 1.7,
          fontSize: 14,
          background: mine ? "var(--pa-accent-soft)" : "var(--pa-fill)",
          color: "var(--pa-text)",
        }}
      >
        {turn.text}
      </div>
    </div>
  );
}
