"use client";

import { Fragment, memo, type ReactNode } from "react";

/**
 * 轻量 Markdown 渲染：标题、有序/无序列表、引用、分隔线、加粗、行内代码，
 * 以及记忆引用 [mem_xxx]（可点开）。助理的回答结构固定、格式简单，用不着整套 Markdown 库。
 */
/** 流式输出时上面已经渲染好的历史消息不用每个字都重新解析一遍。 */
export const RichText = memo(function RichText({ text, onMemory, onNote, streaming }: { text: string; onMemory?: (key: string) => void; onNote?: (key: string) => void; streaming?: boolean }) {
  const lines = text.split("\n");
  const blocks: ReactNode[] = [];
  let list: { ordered: boolean; items: ReactNode[] } | null = null;
  let quote: ReactNode[] = [];
  // 同一条记忆在一段回答里被引用多次时用同一个编号
  const counter: Counter = new Map();

  const flushList = () => {
    if (!list) return;
    const Tag = list.ordered ? "ol" : "ul";
    blocks.push(<Tag key={`l-${blocks.length}`} className="rt-list">{list.items}</Tag>);
    list = null;
  };
  const flushQuote = () => {
    if (!quote.length) return;
    blocks.push(<blockquote key={`q-${blocks.length}`} className="rt-quote">{quote}</blockquote>);
    quote = [];
  };

  lines.forEach((raw, index) => {
    const line = raw.trimEnd();
    const quoted = line.match(/^\s*>\s?(.*)$/);
    if (quoted) {
      flushList();
      quote.push(<div key={index}>{inline(quoted[1], onMemory, counter, onNote)}</div>);
      return;
    }
    flushQuote();
    const bullet = line.match(/^\s*([-*•])\s+(.*)$/);
    const numbered = line.match(/^\s*(\d+)[.、)]\s*(.*)$/);
    if (bullet || numbered) {
      const ordered = !!numbered;
      if (!list || list.ordered !== ordered) {
        flushList();
        list = { ordered, items: [] };
      }
      list.items.push(<li key={index}>{inline((bullet ? bullet[2] : numbered![2]) ?? "", onMemory, counter, onNote)}</li>);
      return;
    }
    flushList();
    const heading = line.match(/^(#{1,4})\s+(.*)$/);
    if (heading) {
      blocks.push(<div key={index} className="rt-h" data-level={heading[1].length}>{inline(heading[2], onMemory, counter, onNote)}</div>);
    } else if (/^\s*([-*_])\1{2,}\s*$/.test(line)) {
      blocks.push(<hr key={index} className="rt-hr" />);
    } else if (line.trim() === "") {
      blocks.push(<div key={index} className="rt-gap" />);
    } else {
      blocks.push(<p key={index} className="rt-p">{inline(line, onMemory, counter, onNote)}</p>);
    }
  });
  flushList();
  flushQuote();

  return (
    <div className="rt" data-streaming={streaming}>
      {blocks}
      <style jsx global>{`
        .rt { line-height: 1.75; word-break: break-word; font-size: 16px; color: var(--pa-text); }
        .rt-p { margin: 0; }
        .rt-gap { height: 10px; }
        .rt-h { font-weight: 700; margin: 14px 0 4px; font-size: 16px; letter-spacing: 0.01em; }
        .rt-h[data-level="1"], .rt-h[data-level="2"] { font-family: var(--pa-font-display); font-size: 18px; }
        .rt > .rt-h:first-child { margin-top: 0; }
        .rt-list { margin: 4px 0; padding-left: 1.4em; }
        .rt-list li { margin: 4px 0; padding-left: 2px; }
        .rt-list li::marker { color: var(--pa-text-3); font-variant-numeric: tabular-nums; }
        ul.rt-list li::marker { color: var(--pa-accent); }
        .rt-quote {
          margin: 8px 0;
          padding: 8px 14px;
          border-left: 3px solid var(--pa-accent);
          background: var(--pa-accent-soft);
          border-radius: 0 10px 10px 0;
          color: var(--pa-text);
        }
        .rt-hr { border: 0; border-top: 0.5px solid var(--pa-line-strong); margin: 14px 0; }
        .rt strong { font-weight: 650; }
        .rt code {
          font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
          font-size: 0.88em;
          padding: 1px 5px;
          border-radius: 5px;
          background: var(--pa-fill-strong);
        }
        .rt-ref {
          display: inline-grid;
          place-items: center;
          min-width: 17px;
          height: 17px;
          padding: 0 4px;
          margin: 0 1px;
          border-radius: 6px;
          font-size: 10.5px;
          font-weight: 700;
          line-height: 1;
          vertical-align: 2px;
          color: var(--pa-accent);
          background: var(--pa-accent-soft);
          cursor: pointer;
          font-variant-numeric: tabular-nums;
        }
        .rt-ref:active { opacity: 0.6; }
        .rt-ref-note { font-weight: 600; padding: 0 5px; }
        .rt[data-streaming="true"] > :last-child::after {
          content: "";
          display: inline-block;
          width: 8px;
          height: 1.05em;
          margin-left: 3px;
          vertical-align: -2px;
          border-radius: 2px;
          background: var(--pa-accent);
          animation: rt-caret 1s steps(2) infinite;
        }
        @keyframes rt-caret { 50% { opacity: 0; } }
      `}</style>
    </div>
  );
});

type Counter = Map<string, number>;

function inline(text: string, onMemory: ((key: string) => void) | undefined, counter: Counter, onNote?: (key: string) => void): ReactNode {
  const parts = text.split(/(\*\*[^*]+\*\*|`[^`]+`|\[mem_[0-9a-f]+(?:[,，、\s]+mem_[0-9a-f]+)*\]|\[note_[0-9a-f]+\])/g);
  return parts.map((part, index) => {
    // 引用的原始记录原文（整理成记忆时漏掉的细节）
    if (part.startsWith("[note_")) {
      const key = part.slice(1, -1);
      return (
        <span key={index} className="rt-ref rt-ref-note" role="button" onClick={() => onNote?.(key)}>原文</span>
      );
    }
    if (part.startsWith("**") && part.endsWith("**") && part.length > 4) return <strong key={index}>{part.slice(2, -2)}</strong>;
    if (part.startsWith("`") && part.endsWith("`") && part.length > 2) return <code key={index}>{part.slice(1, -1)}</code>;
    if (part.startsWith("[mem_")) {
      const keys = Array.from(new Set(part.slice(1, -1).split(/[,，、\s]+/).filter(Boolean)));
      return (
        <Fragment key={index}>
          {keys.map((key) => {
            if (!counter.has(key)) counter.set(key, counter.size + 1);
            return (
              <span key={key} className="rt-ref" role="button" onClick={() => onMemory?.(key)}>
                {counter.get(key)}
              </span>
            );
          })}
        </Fragment>
      );
    }
    return <Fragment key={index}>{part}</Fragment>;
  });
}
