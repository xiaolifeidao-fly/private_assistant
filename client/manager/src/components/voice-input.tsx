"use client";

import { useEffect, useRef, useState } from "react";
import { App, Button, Tooltip } from "antd";
import { Mic, Send, Type, X } from "lucide-react";
import { postForm } from "@/lib/api";
import { canRecord, MAX_SECONDS, VoiceRecorder } from "@/lib/recorder";

type Phase = "idle" | "recording" | "transcribing";

/**
 * 对话框里的语音输入：点「说话」开始录，录音时按钮变成一条录音栏（计时、音量、取消 / 转成文字 / 直接发送）。
 * 录音交给服务端语音识别（模型路由 asr），只用来转文字，不保存。
 */
export function VoiceInput({ onText, onSend, disabled }: {
  onText: (text: string) => void;
  onSend?: (text: string) => void;
  disabled?: boolean;
}) {
  const { message } = App.useApp();
  const [phase, setPhase] = useState<Phase>("idle");
  const [level, setLevel] = useState(0);
  const [seconds, setSeconds] = useState(0);
  const [supported, setSupported] = useState(false);
  const recorderRef = useRef<VoiceRecorder | null>(null);
  const finishRef = useRef<(mode: "text" | "send") => void>(() => {});

  useEffect(() => setSupported(canRecord()), []);
  useEffect(() => () => recorderRef.current?.cancel(), []);
  useEffect(() => {
    if (phase !== "recording") return;
    const timer = window.setInterval(() => {
      const elapsed = recorderRef.current?.elapsed() ?? 0;
      setSeconds(elapsed);
      if (elapsed >= MAX_SECONDS) finishRef.current(onSend ? "send" : "text");
    }, 250);
    return () => window.clearInterval(timer);
  }, [phase, onSend]);

  const start = async () => {
    const recorder = new VoiceRecorder();
    try {
      await recorder.start(setLevel);
    } catch {
      recorder.cancel();
      message.error("用不了麦克风：请在浏览器地址栏左边允许麦克风权限（需要 HTTPS 或 localhost）");
      return;
    }
    recorderRef.current = recorder;
    setSeconds(0);
    setPhase("recording");
  };

  const cancel = () => {
    recorderRef.current?.cancel();
    recorderRef.current = null;
    setPhase("idle");
  };

  const finish = async (mode: "text" | "send") => {
    const recorder = recorderRef.current;
    if (!recorder) return;
    recorderRef.current = null;
    setPhase("transcribing");
    try {
      const recording = await recorder.stop();
      if (recording.seconds < 0.6) {
        message.warning("太短了，没听清");
        return;
      }
      const form = new FormData();
      form.append("audio", recording.blob, `voice.${recording.extension}`);
      const { text } = await postForm<{ text: string }>("/speech/transcribe", form);
      if (!text) {
        message.warning("没听出说了什么，再说一遍？");
        return;
      }
      if (mode === "send" && onSend) onSend(text);
      else onText(text);
    } catch (error) {
      message.error((error as Error).message, 6);
    } finally {
      setPhase("idle");
      setLevel(0);
    }
  };
  finishRef.current = (mode) => void finish(mode);

  if (!supported) return null;

  if (phase === "idle") {
    return (
      <Tooltip title="说话，转成文字（最长 5 分钟）">
        <Button icon={<Mic size={15} />} className="pa-icon-text" disabled={disabled} onClick={() => void start()}>说话</Button>
      </Tooltip>
    );
  }

  const clock = `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, "0")}`;
  return (
    <div className="voice-bar" data-phase={phase}>
      {phase === "recording" ? (
        <>
          <span className="voice-dot" style={{ transform: `scale(${1 + level * 0.8})` }} />
          <span className="voice-clock">{clock}</span>
          <span className="voice-level"><i style={{ width: `${Math.round(level * 100)}%` }} /></span>
          <Button size="small" icon={<X size={14} />} className="pa-icon-text" onClick={cancel}>取消</Button>
          <Button size="small" icon={<Type size={14} />} className="pa-icon-text" onClick={() => void finish("text")}>转成文字</Button>
          {onSend && <Button size="small" type="primary" icon={<Send size={14} />} className="pa-icon-text" onClick={() => void finish("send")}>直接发送</Button>}
        </>
      ) : (
        <span className="voice-clock">正在转成文字…</span>
      )}
      <style jsx global>{`
        .voice-bar {
          display: inline-flex;
          align-items: center;
          gap: 8px;
          height: 32px;
          padding: 0 6px 0 12px;
          border-radius: 16px;
          background: var(--pa-fill, rgba(0, 0, 0, 0.04));
        }
        .voice-dot { width: 9px; height: 9px; border-radius: 50%; background: #e5484d; transition: transform 0.08s linear; }
        .voice-clock { font-variant-numeric: tabular-nums; font-size: 13px; color: var(--pa-text-2, inherit); min-width: 34px; }
        .voice-level { width: 60px; height: 4px; border-radius: 2px; background: rgba(0, 0, 0, 0.08); overflow: hidden; }
        .voice-level i { display: block; height: 100%; background: #e5484d; transition: width 0.08s linear; }
      `}</style>
    </div>
  );
}
