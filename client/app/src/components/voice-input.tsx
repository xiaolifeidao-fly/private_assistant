"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Toast } from "antd-mobile";
import { ArrowUp, Mic, Type, X } from "lucide-react";
import { postForm } from "@/lib/api";
import { canRecord, MAX_SECONDS, VoiceRecorder } from "@/lib/recorder";

type Phase = "idle" | "recording" | "transcribing";

/**
 * 麦克风按钮：点一下开始说，底部弹出录音面板。说完可以
 *  - 「转成文字」：识别结果填进输入框，改一改再发（识别偶尔会听错人名）；
 *  - 「直接发送」（给了 onSend 时）：识别完立刻发给模型。
 * 录音只用来识别，不保存。
 */
export function VoiceButton({ onText, onSend, disabled, className, children }: {
  onText: (text: string) => void;
  onSend?: (text: string) => void;
  disabled?: boolean;
  className?: string;
  /** 按钮里的内容，默认只有一个麦克风图标 */
  children?: ReactNode;
}) {
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
    if (phase !== "idle") return;
    const recorder = new VoiceRecorder();
    try {
      await recorder.start(setLevel);
    } catch {
      recorder.cancel();
      Toast.show({ content: "用不了麦克风：请在系统设置里允许访问，或用键盘上的🎤听写" });
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
        Toast.show({ content: "太短了，没听清" });
        return;
      }
      const form = new FormData();
      form.append("audio", recording.blob, `voice.${recording.extension}`);
      const { text } = await postForm<{ text: string }>("/speech/transcribe", form);
      if (!text) {
        Toast.show({ content: "没听出说了什么，再说一遍？" });
        return;
      }
      if (mode === "send" && onSend) onSend(text);
      else onText(text);
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setPhase("idle");
      setLevel(0);
    }
  };
  finishRef.current = (mode) => void finish(mode);

  if (!supported) return null;
  const clock = `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, "0")}`;

  return (
    <>
      <button type="button" className={className ?? "voice-btn"} onClick={start} disabled={disabled || phase !== "idle"} aria-label="说话">
        {children ?? <Mic size={20} strokeWidth={1.9} />}
      </button>
      {phase !== "idle" && typeof document !== "undefined" && createPortal(
        <div className="voice-panel" role="dialog" aria-label="录音">
          <div className="voice-card">
            {phase === "recording" ? (
              <>
                <div className="voice-orb" style={{ transform: `scale(${1 + level * 0.45})` }}><Mic size={28} strokeWidth={2} /></div>
                <div className="voice-clock pa-num">{clock}<span> / {MAX_SECONDS / 60}:00</span></div>
                <div className="voice-hint">在听…说完点下面的按钮</div>
                <div className="voice-actions">
                  <button type="button" className="voice-action" data-kind="cancel" onClick={cancel}><X size={18} />取消</button>
                  <button type="button" className="voice-action" data-kind="text" onClick={() => void finish("text")}><Type size={18} />转成文字</button>
                  {onSend && <button type="button" className="voice-action" data-kind="send" onClick={() => void finish("send")}><ArrowUp size={18} strokeWidth={2.6} />直接发送</button>}
                </div>
              </>
            ) : (
              <>
                <div className="voice-orb" data-busy="true"><span className="pa-btn-spin" /></div>
                <div className="voice-hint">正在转成文字…</div>
              </>
            )}
          </div>
        </div>,
        document.body,
      )}
      <style jsx global>{`
        .voice-btn {
          flex-shrink: 0;
          width: 36px;
          height: 36px;
          border-radius: 18px;
          border: 0;
          background: none;
          display: grid;
          place-items: center;
          color: var(--pa-text-2);
          cursor: pointer;
        }
        .voice-btn:active { background: var(--pa-fill-strong); }
        .voice-btn:disabled { opacity: 0.4; }
        .voice-panel {
          position: fixed;
          inset: 0;
          z-index: 1200;
          display: flex;
          align-items: flex-end;
          background: rgba(0, 0, 0, 0.28);
          -webkit-backdrop-filter: blur(2px);
          backdrop-filter: blur(2px);
          animation: pa-enter 0.2s var(--pa-ease) both;
        }
        .voice-card {
          width: 100%;
          padding: 28px 20px calc(22px + env(safe-area-inset-bottom));
          border-radius: 24px 24px 0 0;
          background: var(--pa-elevated);
          box-shadow: var(--pa-shadow);
          display: flex;
          flex-direction: column;
          align-items: center;
          gap: 10px;
        }
        .voice-orb {
          width: 76px;
          height: 76px;
          border-radius: 38px;
          display: grid;
          place-items: center;
          background: var(--pa-accent);
          color: var(--pa-accent-ink);
          box-shadow: 0 0 0 10px var(--pa-accent-soft);
          transition: transform 0.08s linear;
        }
        .voice-orb[data-busy="true"] { background: var(--pa-fill-strong); color: var(--pa-text-2); box-shadow: none; }
        .voice-clock { margin-top: 12px; font-size: 22px; font-weight: 700; font-variant-numeric: tabular-nums; }
        .voice-clock span { font-size: 13px; font-weight: 500; color: var(--pa-text-3); }
        .voice-hint { font-size: 13.5px; color: var(--pa-text-3); }
        .voice-actions { display: flex; gap: 10px; width: 100%; margin-top: 14px; }
        .voice-action {
          flex: 1;
          height: 48px;
          border-radius: 16px;
          border: 0;
          display: inline-flex;
          align-items: center;
          justify-content: center;
          gap: 6px;
          font-size: 15px;
          font-weight: 600;
          background: var(--pa-fill);
          color: var(--pa-text);
          cursor: pointer;
        }
        .voice-action[data-kind="send"] { background: var(--pa-accent); color: var(--pa-accent-ink); }
        .voice-action:active { transform: scale(0.97); }
      `}</style>
    </>
  );
}
