"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { ArrowRight, Bell, CalendarDays, Camera, Check, Images, MessageCircle, Mic, PenLine } from "lucide-react";
import { withBase } from "@/lib/base-path";

interface Slide {
  key: string;
  eyebrow: string;
  title: string;
  body: ReactNode;
  art: ReactNode;
}

const slides: Slide[] = [
  {
    key: "hello",
    eyebrow: "欢迎",
    title: "家里的事，交给它记",
    body: "家里人说过的话、在意的事、老家的规矩，它都帮你记着。需要的时候，帮你想清楚怎么做、怎么说。",
    art: (
      <div className="wel-art-hello">
        <div className="wel-ring" />
        <div className="wel-ring" style={{ animationDelay: "1.2s" }} />
        <img src={withBase("/icons/icon-192.png")} alt="" width={96} height={96} />
      </div>
    ),
  },
  {
    key: "chat",
    eyebrow: "对话",
    title: "有事就问，像聊天一样",
    body: <>问「下周回老家要注意什么？」「她最近在意什么？」<br />想记东西，直接说「记一下：……」</>,
    art: (
      <div className="wel-art-chat">
        <div className="wel-bubble" data-me="true">下周回老家要注意什么？</div>
        <div className="wel-bubble">
          <b>这次会见到大舅</b>，他腰不好，记得带上上次说的护腰……
        </div>
        <div className="wel-bubble" data-me="true">记一下：她想周末去看展</div>
        <div className="wel-saved"><Check size={12} strokeWidth={3} />记下来了</div>
      </div>
    ),
  },
  {
    key: "note",
    eyebrow: "速记",
    title: "想到就记，不用整理",
    body: "说一段话、拍张照、贴几张聊天截图都行。说得乱也没关系，助理会自己整理好。",
    art: (
      <div className="wel-art-note">
        <div className="wel-tool" data-tone="clay"><Mic size={26} /><span>说一说</span></div>
        <div className="wel-tool" data-tone="sky"><Camera size={26} /><span>拍张照</span></div>
        <div className="wel-tool" data-tone="plum"><Images size={26} /><span>截图</span></div>
      </div>
    ),
  },
  {
    key: "today",
    eyebrow: "今天",
    title: "每天看一眼「今天」",
    body: "助理拿不准的会放在这里，点一下「确认」就行。答应过的事、生日节日也会提前提醒你。",
    art: (
      <div className="wel-art-today">
        <div className="wel-card">
          <div className="wel-card-meta"><span className="wel-pill">诉求</span>老婆 · 昨天</div>
          <div className="wel-card-title">希望周末一起带孩子去公园</div>
          <div className="wel-card-actions"><span>不对</span><span data-primary="true"><Check size={13} strokeWidth={3} />确认</span></div>
        </div>
        <div className="wel-remind"><Bell size={14} />后天是妈妈生日</div>
      </div>
    ),
  },
];

const tabHints = [
  { icon: MessageCircle, label: "对话" },
  { icon: PenLine, label: "速记" },
  { icon: CalendarDays, label: "今天" },
];

/** 第一次打开时的欢迎引导：左右滑动或点「下一步」，四页讲清楚这个 App 怎么用。 */
export function Welcome({ onDone }: { onDone: () => void }) {
  const trackRef = useRef<HTMLDivElement>(null);
  const [index, setIndex] = useState(0);
  const [closing, setClosing] = useState(false);
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  const onScroll = () => {
    const track = trackRef.current;
    if (track) setIndex(Math.round(track.scrollLeft / track.clientWidth));
  };

  const go = (next: number) => {
    const track = trackRef.current;
    if (track) track.scrollTo({ left: next * track.clientWidth, behavior: "smooth" });
  };

  const finish = () => {
    setClosing(true);
    setTimeout(onDone, 380);
  };

  const last = index === slides.length - 1;
  if (!mounted) return null;

  return createPortal(
    <div className="wel" data-closing={closing} role="dialog" aria-label="使用引导">
      <div className="wel-top">
        <span className="wel-step pa-num">{index + 1} / {slides.length}</span>
        {!last && <button type="button" className="wel-skip" onClick={finish}>跳过</button>}
      </div>

      <div className="wel-track" ref={trackRef} onScroll={onScroll}>
        {slides.map((slide, i) => (
          <section key={slide.key} className="wel-slide" data-active={i === index} aria-hidden={i !== index}>
            <div className="wel-art">{slide.art}</div>
            <div className="wel-text">
              <div className="wel-eyebrow">{slide.eyebrow}</div>
              <h2 className="wel-title">{slide.title}</h2>
              <p className="wel-body">{slide.body}</p>
            </div>
          </section>
        ))}
      </div>

      <div className="wel-foot">
        <div className="wel-dots">
          {slides.map((slide, i) => (
            <button key={slide.key} type="button" data-on={i === index} onClick={() => go(i)} aria-label={`第 ${i + 1} 页`} />
          ))}
        </div>
        {last && (
          <div className="wel-tabs">
            {tabHints.map(({ icon: Icon, label }) => (
              <span key={label}><Icon size={14} />{label}</span>
            ))}
            <em>都在屏幕最下面</em>
          </div>
        )}
        <button type="button" className="pa-btn" data-variant={last ? "accent" : undefined} data-block="true" onClick={() => (last ? finish() : go(index + 1))}>
          {last ? "开始使用" : "下一步"}
          <ArrowRight size={18} strokeWidth={2.4} />
        </button>
      </div>

      <style jsx global>{`
        .wel {
          position: fixed;
          inset: 0;
          z-index: 2000;
          display: flex;
          flex-direction: column;
          background:
            radial-gradient(90% 50% at 50% 0%, color-mix(in srgb, var(--pa-accent) 14%, transparent), transparent 70%),
            var(--pa-bg);
          padding-top: var(--pa-safe-top);
          animation: pa-fade 0.35s ease both;
          transition: opacity 0.35s, transform 0.4s var(--pa-ease);
        }
        .wel[data-closing="true"] { opacity: 0; transform: scale(1.04); }
        .wel-top { height: 52px; display: flex; align-items: center; justify-content: space-between; padding: 0 20px; flex-shrink: 0; }
        .wel-step { font-size: 13px; color: var(--pa-text-3); font-weight: 600; }
        .wel-skip { border: 0; background: var(--pa-fill-strong); color: var(--pa-text-2); height: 32px; padding: 0 14px; border-radius: 16px; font-size: 13.5px; font-weight: 600; cursor: pointer; }
        .wel-track {
          flex: 1;
          display: flex;
          overflow-x: auto;
          overflow-y: hidden;
          scroll-snap-type: x mandatory;
          scrollbar-width: none;
          overscroll-behavior: contain;
        }
        .wel-track::-webkit-scrollbar { display: none; }
        .wel-slide {
          flex: 0 0 100%;
          scroll-snap-align: center;
          scroll-snap-stop: always;
          display: flex;
          flex-direction: column;
          padding: 0 28px;
          min-height: 0;
        }
        .wel-art { flex: 1; display: grid; place-items: center; min-height: 220px; }
        .wel-art > * { transition: transform 0.6s var(--pa-ease), opacity 0.5s; }
        .wel-slide:not([data-active="true"]) .wel-art > * { transform: scale(0.92) translateY(10px); opacity: 0.4; }
        .wel-text { padding-bottom: 8px; }
        .wel-eyebrow { font-size: 13px; font-weight: 700; color: var(--pa-accent); letter-spacing: 0.12em; }
        .wel-title { font-family: var(--pa-font-display); font-size: 30px; line-height: 1.25; margin: 8px 0 0; font-weight: 700; }
        .wel-body { font-size: 16px; line-height: 1.75; color: var(--pa-text-2); margin: 12px 0 0; min-height: 84px; }
        .wel-foot { padding: 12px 24px calc(20px + var(--pa-safe-bottom)); flex-shrink: 0; }
        .wel-dots { display: flex; gap: 6px; justify-content: center; margin-bottom: 18px; }
        .wel-dots button { width: 7px; height: 7px; border-radius: 4px; border: 0; padding: 0; background: var(--pa-line-strong); cursor: pointer; transition: width 0.35s var(--pa-ease), background 0.3s; }
        .wel-dots button[data-on="true"] { width: 22px; background: var(--pa-accent); }
        .wel-tabs { display: flex; align-items: center; justify-content: center; gap: 10px; font-size: 12.5px; color: var(--pa-text-2); margin-bottom: 14px; flex-wrap: wrap; animation: pa-enter 0.4s var(--pa-ease) both; }
        .wel-tabs span { display: inline-flex; align-items: center; gap: 4px; padding: 4px 9px; border-radius: 12px; background: var(--pa-fill-strong); font-weight: 600; }
        .wel-tabs em { font-style: normal; color: var(--pa-text-3); }

        /* 插图 */
        .wel-art-hello { position: relative; width: 200px; height: 200px; display: grid; place-items: center; }
        .wel-art-hello img { position: relative; border-radius: 26px; box-shadow: var(--pa-shadow-lg); }
        .wel-ring { position: absolute; inset: 0; border-radius: 50%; border: 1.5px solid color-mix(in srgb, var(--pa-accent) 35%, transparent); animation: wel-ring 2.4s ease-out infinite; }
        @keyframes wel-ring { from { transform: scale(0.5); opacity: 1; } to { transform: scale(1.25); opacity: 0; } }

        .wel-art-chat { width: 100%; max-width: 320px; display: flex; flex-direction: column; gap: 10px; }
        .wel-bubble { align-self: flex-start; max-width: 84%; padding: 10px 14px; border-radius: 18px 18px 18px 6px; background: var(--pa-surface); border: 0.5px solid var(--pa-line); box-shadow: var(--pa-shadow-sm); font-size: 14px; line-height: 1.6; }
        .wel-bubble[data-me="true"] { align-self: flex-end; border-radius: 18px 18px 6px 18px; background: var(--pa-ink-btn); color: var(--pa-ink-btn-text); border: 0; }
        .wel-bubble b { color: var(--pa-accent); }
        .wel-saved { align-self: flex-start; display: inline-flex; align-items: center; gap: 5px; height: 24px; padding: 0 10px; border-radius: 12px; background: var(--pa-success-soft); color: var(--pa-success); font-size: 12px; font-weight: 600; }

        .wel-art-note { display: flex; gap: 14px; }
        .wel-tool { width: 88px; height: 104px; border-radius: 24px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; background: var(--pa-surface); border: 0.5px solid var(--pa-line); box-shadow: var(--pa-shadow); color: var(--tone-fg); font-size: 13px; font-weight: 600; }
        .wel-tool span { color: var(--pa-text); }
        .wel-tool:nth-child(2) { transform: translateY(-14px); }

        .wel-art-today { width: 100%; max-width: 320px; display: flex; flex-direction: column; gap: 12px; }
        .wel-card { padding: 14px 16px 12px; border-radius: var(--pa-radius); background: var(--pa-surface); border: 0.5px solid var(--pa-line); box-shadow: var(--pa-shadow); }
        .wel-card-meta { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--pa-text-3); }
        .wel-pill { height: 20px; padding: 0 7px; border-radius: 10px; display: inline-grid; place-items: center; font-size: 11px; font-weight: 700; background: var(--tone-rose-bg); color: var(--tone-rose); }
        .wel-card-title { font-size: 15.5px; font-weight: 600; margin-top: 8px; }
        .wel-card-actions { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-top: 12px; }
        .wel-card-actions span { height: 32px; border-radius: 16px; display: inline-flex; align-items: center; justify-content: center; gap: 4px; font-size: 13px; font-weight: 600; background: var(--pa-fill-strong); }
        .wel-card-actions span[data-primary="true"] { background: var(--pa-accent); color: var(--pa-accent-ink); animation: wel-tap 2.4s ease-in-out infinite 0.8s; }
        @keyframes wel-tap { 0%, 70%, 100% { transform: none; box-shadow: none; } 80% { transform: scale(0.94); box-shadow: 0 0 0 6px var(--pa-accent-soft); } }
        .wel-remind { align-self: center; display: inline-flex; align-items: center; gap: 6px; padding: 8px 14px; border-radius: 16px; background: var(--tone-sky-bg); color: var(--tone-sky); font-size: 13px; font-weight: 600; }
      `}</style>
    </div>,
    document.body,
  );
}
