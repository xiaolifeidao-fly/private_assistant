"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Toast } from "antd-mobile";
import { Check, Mic, X } from "lucide-react";
import { DICTATION_MAX_SECONDS, LiveTranscriber } from "@/lib/dictation";
import { canRecord } from "@/lib/recorder";

type Phase = "idle" | "starting" | "recording" | "finishing";
type Mode = "hold" | "tap";

const LONG_PRESS_MS = 400;
const MOVE_TOLERANCE = 10;
const CANCEL_DISTANCE = 80;
const TAP_MS = 350;

export interface Dictation {
  supported: boolean;
  phase: Phase;
  mode: Mode;
  /** 录音或识别中：输入框先只读，免得和正在写进去的字打架 */
  busy: boolean;
  /** 放在输入框（或包着它的元素）上：按住输入框就开始说话 */
  holdProps: {
    "data-voice-hold": string;
    onPointerDown: (event: React.PointerEvent) => void;
    onContextMenu: (event: React.MouseEvent) => void;
    onFocusCapture: (event: React.FocusEvent) => void;
    onBlurCapture: (event: React.FocusEvent) => void;
  };
  /** 麦克风按钮按下：点一下=开始/结束，按住=松开结束 */
  pressButton: (event: React.PointerEvent) => void;
  finish: () => void;
  cancel: () => void;
  /** 录音提示，页面里放一份 */
  overlay: ReactNode;
}

/**
 * 说话输入：边说边把识别出的字写进输入框（插在光标处），说完还能改。
 *  - 按住输入框（手机上是输入框没在打字时）≈ 微信按住说话：松开结束，上滑取消；
 *  - 点麦克风：开始说，再点一下（或「完成」）结束；按住麦克风也行。
 * 录音只用来识别，不保存。
 */
export function useDictation({ value, onChange, disabled }: { value: string; onChange: (value: string) => void; disabled?: boolean }): Dictation {
  const [supported, setSupported] = useState(false);
  const [phase, setPhase] = useState<Phase>("idle");
  const [mode, setMode] = useState<Mode>("hold");
  const [level, setLevel] = useState(0);
  const [seconds, setSeconds] = useState(0);
  const [cancelArmed, setCancelArmed] = useState(false);

  const phaseRef = useRef<Phase>("idle");
  const valueRef = useRef(value);
  const onChangeRef = useRef(onChange);
  valueRef.current = value;
  onChangeRef.current = onChange;
  const transcriberRef = useRef<LiveTranscriber | null>(null);
  const baseRef = useRef({ original: "", prefix: "", suffix: "" });
  const caretRef = useRef<number | null>(null);
  const releasedRef = useRef(false);
  const cancelArmedRef = useRef(false);
  const cleanupRef = useRef<() => void>(() => {});
  const modeRef = useRef<Mode>("hold");
  const inputRef = useRef<HTMLTextAreaElement | null>(null);

  const setPhaseBoth = (next: Phase) => {
    phaseRef.current = next;
    setPhase(next);
  };
  const setModeBoth = (next: Mode) => {
    modeRef.current = next;
    setMode(next);
  };

  useEffect(() => setSupported(canRecord()), []);
  useEffect(() => () => {
    transcriberRef.current?.cancel();
    cleanupRef.current();
  }, []);

  useEffect(() => {
    if (phase !== "recording") return;
    const timer = window.setInterval(() => {
      const elapsed = transcriberRef.current?.elapsed() ?? 0;
      setSeconds(elapsed);
      if (elapsed >= DICTATION_MAX_SECONDS) void finish();
    }, 250);
    return () => window.clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase]);

  const write = (middle: string) => {
    const { prefix, suffix } = baseRef.current;
    onChangeRef.current(prefix + middle + suffix);
  };

  const start = async (nextMode: Mode, input: HTMLTextAreaElement | null) => {
    if (phaseRef.current !== "idle" || disabled) return;
    const current = valueRef.current;
    let caret = current.length;
    if (input && document.activeElement === input) caret = input.selectionStart ?? caret;
    else if (caretRef.current !== null) caret = caretRef.current;
    caret = Math.min(caret, current.length);
    baseRef.current = { original: current, prefix: current.slice(0, caret), suffix: current.slice(caret) };
    input?.blur();
    window.getSelection()?.removeAllRanges();

    releasedRef.current = false;
    cancelArmedRef.current = false;
    setCancelArmed(false);
    setModeBoth(nextMode);
    setSeconds(0);
    setPhaseBoth("starting");
    navigator.vibrate?.(20);

    const transcriber = new LiveTranscriber(
      ({ text, listening }) => {
        if (transcriberRef.current !== transcriber) return;
        write(text + (listening && phaseRef.current === "recording" ? "…" : ""));
      },
      setLevel,
      (message) => Toast.show({ content: message }),
    );
    try {
      await transcriber.start();
    } catch {
      transcriber.cancel();
      setPhaseBoth("idle");
      Toast.show({ content: "用不了麦克风：请在系统设置里允许访问，或用键盘上的🎤听写" });
      return;
    }
    // 第一次用会先弹「允许麦克风」，那时手已经松开了
    if (modeRef.current === "hold" && releasedRef.current) {
      transcriber.cancel();
      setPhaseBoth("idle");
      Toast.show({ content: "好了，现在按住说话" });
      return;
    }
    transcriberRef.current = transcriber;
    setPhaseBoth("recording");
  };

  const finish = async () => {
    const transcriber = transcriberRef.current;
    if (!transcriber || phaseRef.current !== "recording") return;
    setPhaseBoth("finishing");
    const text = await transcriber.stop();
    if (transcriberRef.current === transcriber) {
      transcriberRef.current = null;
      write(text);
      if (!text) Toast.show({ content: "没听清，再说一遍？" });
      else caretRef.current = baseRef.current.prefix.length + text.length;
    }
    setPhaseBoth("idle");
  };

  const cancel = () => {
    const transcriber = transcriberRef.current;
    transcriberRef.current = null;
    transcriber?.cancel();
    onChangeRef.current(baseRef.current.original);
    setPhaseBoth("idle");
  };

  const release = () => {
    releasedRef.current = true;
    if (phaseRef.current !== "recording") return;
    if (cancelArmedRef.current) cancel();
    else void finish();
  };

  /**
   * 按住期间：跟着手指判断上滑取消，松开结束。监听挂在 window 上，按钮换掉了也收得到松手。
   * 给了 pressedAt（按的是麦克风按钮）时，很快松开算「点一下」：一直录到再点一次。
   */
  const followHold = (startY: number, pressedAt?: number) => {
    cleanupRef.current();
    const move = (event: PointerEvent) => {
      const armed = startY - event.clientY > CANCEL_DISTANCE;
      cancelArmedRef.current = armed;
      setCancelArmed(armed);
    };
    // 不让页面跟着手指滚动（滚动会让浏览器发 pointercancel）
    const block = (event: TouchEvent) => event.preventDefault();
    const up = () => {
      cleanupRef.current();
      if (pressedAt !== undefined && Date.now() - pressedAt < TAP_MS) {
        setModeBoth("tap");
        return;
      }
      release();
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
    window.addEventListener("touchend", up);
    window.addEventListener("touchcancel", up);
    window.addEventListener("touchmove", block, { passive: false });
    cleanupRef.current = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      window.removeEventListener("touchend", up);
      window.removeEventListener("touchcancel", up);
      window.removeEventListener("touchmove", block);
      cleanupRef.current = () => {};
    };
  };

  const onPointerDown = (event: React.PointerEvent) => {
    const target = event.target;
    if (!supported || disabled || phaseRef.current !== "idle" || event.button !== 0) return;
    if (!(target instanceof HTMLTextAreaElement)) return;
    inputRef.current = target;
    // 手机上正在打字时，长按是挪光标/选字，不抢
    if (event.pointerType !== "mouse" && document.activeElement === target) return;
    const startX = event.clientX;
    const startY = event.clientY;
    cleanupRef.current();
    const timer = window.setTimeout(() => {
      cleanupRef.current();
      followHold(startY);
      void start("hold", target);
    }, LONG_PRESS_MS);
    const move = (moveEvent: PointerEvent) => {
      if (Math.hypot(moveEvent.clientX - startX, moveEvent.clientY - startY) > MOVE_TOLERANCE) cleanupRef.current();
    };
    const up = () => cleanupRef.current();
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
    cleanupRef.current = () => {
      window.clearTimeout(timer);
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      cleanupRef.current = () => {};
    };
  };

  const pressButton = (event: React.PointerEvent) => {
    if (!supported || disabled || event.button !== 0) return;
    if (phaseRef.current === "recording" && modeRef.current === "tap") {
      void finish();
      return;
    }
    if (phaseRef.current !== "idle") return;
    event.preventDefault();
    followHold(event.clientY, Date.now());
    void start("hold", inputRef.current);
  };

  const busy = phase !== "idle";
  const clock = `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, "0")}`;
  const overlay = busy && typeof document !== "undefined" ? createPortal(
    <div className="dictation-overlay" data-mode={mode}>
      <div className="dictation-card" data-cancel={cancelArmed}>
        {phase === "finishing" ? (
          <>
            <div className="dictation-orb" data-busy="true"><span className="pa-btn-spin" /></div>
            <div className="dictation-hint">正在转成文字…</div>
          </>
        ) : (
          <>
            <div className="dictation-orb" style={{ transform: `scale(${1 + level * 0.4})` }}>
              {cancelArmed ? <X size={26} strokeWidth={2.4} /> : <Mic size={26} strokeWidth={2} />}
            </div>
            <div className="dictation-clock pa-num">{phase === "starting" ? "…" : clock}</div>
            <div className="dictation-hint">
              {phase === "starting" ? "正在打开麦克风" : mode === "tap" ? "说的话会出现在输入框里" : cancelArmed ? "松开手指，取消这段" : "松开结束 · 上滑取消"}
            </div>
            {mode === "tap" && phase === "recording" && (
              <div className="dictation-actions">
                <button type="button" onClick={cancel}><X size={17} />取消</button>
                <button type="button" data-kind="done" onClick={() => void finish()}><Check size={17} strokeWidth={2.6} />完成</button>
              </div>
            )}
          </>
        )}
      </div>
      <style jsx global>{`
        .dictation-overlay {
          position: fixed;
          left: 0;
          right: 0;
          top: 34%;
          z-index: 1200;
          display: flex;
          justify-content: center;
          pointer-events: none;
          animation: pa-enter 0.18s var(--pa-ease) both;
        }
        .dictation-card {
          min-width: 168px;
          padding: 18px 18px 14px;
          border-radius: 22px;
          background: rgba(28, 28, 30, 0.82);
          -webkit-backdrop-filter: blur(12px);
          backdrop-filter: blur(12px);
          color: #fff;
          display: flex;
          flex-direction: column;
          align-items: center;
          gap: 6px;
          box-shadow: 0 10px 40px rgba(0, 0, 0, 0.25);
        }
        .dictation-orb {
          width: 60px;
          height: 60px;
          border-radius: 30px;
          display: grid;
          place-items: center;
          background: var(--pa-accent);
          color: var(--pa-accent-ink);
          box-shadow: 0 0 0 8px rgba(255, 255, 255, 0.12);
          transition: transform 0.08s linear, background 0.15s;
        }
        .dictation-card[data-cancel="true"] .dictation-orb { background: var(--pa-danger, #e5484d); color: #fff; }
        .dictation-orb[data-busy="true"] { background: rgba(255, 255, 255, 0.14); color: #fff; box-shadow: none; }
        .dictation-clock { margin-top: 8px; font-size: 18px; font-weight: 700; font-variant-numeric: tabular-nums; }
        .dictation-hint { font-size: 12.5px; color: rgba(255, 255, 255, 0.72); }
        .dictation-actions { display: flex; gap: 8px; margin-top: 10px; pointer-events: auto; }
        .dictation-actions button {
          height: 36px;
          padding: 0 14px;
          border-radius: 18px;
          border: 0;
          display: inline-flex;
          align-items: center;
          gap: 4px;
          font-size: 14px;
          font-weight: 600;
          background: rgba(255, 255, 255, 0.14);
          color: #fff;
          cursor: pointer;
        }
        .dictation-actions button[data-kind="done"] { background: var(--pa-accent); color: var(--pa-accent-ink); }
      `}</style>
    </div>,
    document.body,
  ) : null;

  return {
    supported,
    phase,
    mode,
    busy,
    holdProps: {
      "data-voice-hold": "",
      onPointerDown,
      onContextMenu: (event) => {
        if (phaseRef.current !== "idle") event.preventDefault();
      },
      onFocusCapture: (event) => {
        if (event.target instanceof HTMLTextAreaElement) inputRef.current = event.target;
      },
      onBlurCapture: (event) => {
        if (event.target instanceof HTMLTextAreaElement) caretRef.current = event.target.selectionStart;
      },
    },
    pressButton,
    finish: () => void finish(),
    cancel,
    overlay,
  };
}

/** 麦克风按钮：点一下开始、再点结束；或者按住说、松开结束。 */
export function VoiceButton({ dictation, disabled, className, children }: {
  dictation: Dictation;
  disabled?: boolean;
  className?: string;
  /** 按钮里的内容，默认只有一个麦克风图标 */
  children?: ReactNode;
}) {
  if (!dictation.supported) return null;
  const active = dictation.phase === "recording" || dictation.phase === "starting";
  return (
    <>
      <button
        type="button"
        className={className ?? "voice-btn"}
        data-recording={active}
        onPointerDown={dictation.pressButton}
        onContextMenu={(event) => event.preventDefault()}
        disabled={disabled || dictation.phase === "finishing"}
        aria-label={active ? "说完了" : "说话"}
      >
        {children ?? <Mic size={20} strokeWidth={1.9} />}
      </button>
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
        button[data-recording="true"] { color: var(--pa-accent); }
        button[data-recording] { touch-action: none; -webkit-touch-callout: none; -webkit-user-select: none; user-select: none; }
      `}</style>
    </>
  );
}
