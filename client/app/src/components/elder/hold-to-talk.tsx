"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Toast } from "antd-mobile";
import { Mic, X } from "lucide-react";
import { postForm } from "@/lib/api";
import { VoiceRecorder } from "@/lib/recorder";
import { stopSpeaking, unlockSpeech } from "@/lib/speak";

type Phase = "idle" | "starting" | "recording" | "transcribing";

const MAX_SECONDS = 60;
const CANCEL_DISTANCE = 80;

/**
 * 长辈模式的「按住 说话」：和微信语音一样，按住说、松开发、往上滑取消。
 * 松开后转成文字直接交给 onText（不再让老人家改字）；听错了再说一遍就行。
 */
export function HoldToTalk({ onText, disabled, label = "按住 说话" }: { onText: (text: string) => void; disabled?: boolean; label?: string }) {
  const [phase, setPhase] = useState<Phase>("idle");
  const [level, setLevel] = useState(0);
  const [seconds, setSeconds] = useState(0);
  const [cancelArmed, setCancelArmed] = useState(false);
  const recorderRef = useRef<VoiceRecorder | null>(null);
  const startYRef = useRef(0);
  const releasedRef = useRef(false);
  const cancelRef = useRef(false);
  const releaseRef = useRef<() => void>(() => {});

  useEffect(() => () => recorderRef.current?.cancel(), []);

  useEffect(() => {
    if (phase !== "recording") return;
    const timer = window.setInterval(() => {
      const elapsed = recorderRef.current?.elapsed() ?? 0;
      setSeconds(elapsed);
      if (elapsed >= MAX_SECONDS) releaseRef.current();
    }, 200);
    return () => window.clearInterval(timer);
  }, [phase]);

  const press = async (event: React.PointerEvent<HTMLButtonElement>) => {
    if (disabled || phase !== "idle") return;
    event.currentTarget.setPointerCapture(event.pointerId);
    unlockSpeech();
    stopSpeaking();
    startYRef.current = event.clientY;
    releasedRef.current = false;
    cancelRef.current = false;
    setCancelArmed(false);
    setSeconds(0);
    setPhase("starting");
    navigator.vibrate?.(20);
    const recorder = new VoiceRecorder();
    try {
      await recorder.start(setLevel);
    } catch {
      recorder.cancel();
      setPhase("idle");
      Toast.show({ content: "用不了麦克风，请让家里人在设置里打开麦克风权限" });
      return;
    }
    // 第一次用会先弹「允许麦克风」，那时手已经松开了
    if (releasedRef.current) {
      recorder.cancel();
      setPhase("idle");
      Toast.show({ content: "好了，现在按住按钮说话" });
      return;
    }
    recorderRef.current = recorder;
    setPhase("recording");
  };

  const move = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (phase !== "recording" && phase !== "starting") return;
    const armed = startYRef.current - event.clientY > CANCEL_DISTANCE;
    cancelRef.current = armed;
    setCancelArmed(armed);
  };

  const release = async () => {
    releasedRef.current = true;
    const recorder = recorderRef.current;
    if (!recorder) return;
    recorderRef.current = null;
    if (cancelRef.current) {
      recorder.cancel();
      setPhase("idle");
      setLevel(0);
      return;
    }
    setPhase("transcribing");
    try {
      const recording = await recorder.stop();
      if (recording.seconds < 0.8) {
        Toast.show({ content: "要一直按住，说完再松手" });
        return;
      }
      const form = new FormData();
      form.append("audio", recording.blob, `voice.${recording.extension}`);
      const { text } = await postForm<{ text: string }>("/speech/transcribe", form);
      if (!text) {
        Toast.show({ content: "没听清，再说一遍？" });
        return;
      }
      onText(text);
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setPhase("idle");
      setLevel(0);
    }
  };
  releaseRef.current = () => void release();

  const active = phase !== "idle";

  return (
    <>
      <button
        type="button"
        className="el-talk"
        data-active={phase === "starting" || phase === "recording"}
        disabled={disabled || phase === "transcribing"}
        onPointerDown={press}
        onPointerMove={move}
        onPointerUp={() => void release()}
        onPointerCancel={() => {
          cancelRef.current = true;
          void release();
        }}
        onContextMenu={(event) => event.preventDefault()}
      >
        <Mic size={30} strokeWidth={2.2} />
        <span>{phase === "recording" || phase === "starting" ? "松开 发送" : phase === "transcribing" ? "正在听…" : label}</span>
      </button>
      {active && typeof document !== "undefined" &&
        createPortal(
          <div className="el-talk-panel" data-cancel={cancelArmed} aria-live="polite">
            <div className="el-talk-card">
              {phase === "transcribing" ? (
                <>
                  <div className="el-talk-orb" data-busy="true"><span className="pa-btn-spin" /></div>
                  <div className="el-talk-title">正在听您说的…</div>
                </>
              ) : (
                <>
                  <div className="el-talk-orb" style={{ transform: `scale(${1 + level * 0.5})` }}>
                    {cancelArmed ? <X size={44} strokeWidth={2.6} /> : <Mic size={44} strokeWidth={2.2} />}
                  </div>
                  <div className="el-talk-title">{phase === "starting" ? "准备好了就说" : cancelArmed ? "松开手，不发了" : "在听，您说"}</div>
                  <div className="el-talk-hint">{cancelArmed ? "" : `说完松开手 · 不想发就往上滑`}</div>
                  {phase === "recording" && <div className="el-talk-clock pa-num">{Math.floor(seconds)} 秒</div>}
                </>
              )}
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}
