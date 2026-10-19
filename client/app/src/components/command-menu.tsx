"use client";

import { AtSign, BellPlus, BellRing, ClipboardList, Drama, NotebookPen, Search, X } from "lucide-react";
import type { LucideIcon } from "lucide-react";

export interface Command {
  label: string;
  hint: string;
  /** 选中后替换掉「@xx」填进输入框的内容；以冒号结尾的是开头，接着写，否则是一句能直接发的话 */
  text: string;
  /** @ 后面可以输入的拼音首字母等简写 */
  keys: string;
  icon: LucideIcon;
}

const remember: Command = { label: "记一下", hint: "存进长期记忆", text: "记一下：", keys: "jyx ji remember", icon: NotebookPen };
const remind: Command = { label: "提醒我", hint: "到点推送提醒", text: "提醒我：", keys: "txw tx remind", icon: BellPlus };
const upcoming: Command = { label: "最近的提醒", hint: "看看接下来要做的", text: "最近有哪些提醒？", keys: "zjdtx tx zj", icon: BellRing };

const commands: Record<string, Command[]> = {
  assistant: [
    remember,
    remind,
    { label: "查记忆", hint: "她说过的话、以前的事", text: "帮我查查：", keys: "cjy cx search", icon: Search },
    { label: "没做完的事", hint: "我答应过、还没办的", text: "我答应过的事还有哪些没做？", keys: "mzwds dy cn", icon: ClipboardList },
    upcoming,
    { label: "预演一句话", hint: "先让她的分身听听", text: "我打算这么跟老婆说：", keys: "yyyjh yy", icon: Drama },
  ],
  recall: [
    remember,
    { label: "找东西", hint: "东西放哪了", text: "帮我找找：", keys: "zdx fnl zz find", icon: Search },
    upcoming,
  ],
  remind: [
    remind,
    upcoming,
    { label: "取消提醒", hint: "不用再提醒了", text: "取消提醒：", keys: "qxtx qx cancel", icon: X },
  ],
  wife: [{ label: "预演一句话", hint: "听听她的第一反应", text: "我打算这么跟你说：", keys: "yyyjh yy", icon: Drama }],
  mom: [{ label: "预演一句话", hint: "听听她的第一反应", text: "我打算这么跟你说：", keys: "yyyjh yy", icon: Drama }],
};

export function commandsFor(botKey: string | undefined, kind: string | undefined): Command[] {
  if (!botKey) return [];
  return commands[botKey] ?? (kind === "assistant" ? commands.assistant : []);
}

/** 光标前紧挨着的「@xx」（@ 前面不能是字母数字，免得邮箱也弹菜单），返回 @ 的位置和后面已输入的字 */
export function findMention(value: string, caret: number): { start: number; query: string } | null {
  const match = /(^|[^A-Za-z0-9])[@＠]([^\s@＠]{0,8})$/.exec(value.slice(0, caret));
  if (!match) return null;
  return { start: caret - match[2].length - 1, query: match[2] };
}

export function filterCommands(list: Command[], query: string): Command[] {
  const q = query.trim().toLowerCase();
  if (!q) return list;
  return list.filter((c) => c.label.includes(q) || c.text.includes(q) || c.keys.split(" ").some((key) => key.startsWith(q)));
}

export function CommandMenu({ items, active, onPick, onHover }: {
  items: Command[];
  active: number;
  onPick: (command: Command) => void;
  onHover: (index: number) => void;
}) {
  if (!items.length) return null;
  return (
    <div className="cmd-menu" role="listbox">
      <div className="cmd-menu-title"><AtSign size={12} strokeWidth={2.4} />快捷指令</div>
      {items.map((command, index) => {
        const Icon = command.icon;
        return (
          <button
            key={command.label}
            className="cmd-item"
            role="option"
            aria-selected={index === active}
            data-active={index === active}
            // 不让按钮抢走焦点，键盘保持弹着
            onMouseDown={(event) => event.preventDefault()}
            onMouseEnter={() => onHover(index)}
            onClick={() => onPick(command)}
          >
            <span className="cmd-icon"><Icon size={16} strokeWidth={2} /></span>
            <span className="cmd-label">{command.label}</span>
            <span className="cmd-hint">{command.hint}</span>
          </button>
        );
      })}
      <style jsx global>{`
        .cmd-menu {
          margin: 0 0 8px;
          padding: 6px;
          border-radius: 18px;
          background: var(--pa-elevated);
          border: 0.5px solid var(--pa-line-strong);
          box-shadow: var(--pa-shadow);
          max-height: 46vh;
          overflow-y: auto;
          animation: pa-enter 0.2s var(--pa-ease) both;
        }
        .cmd-menu-title {
          display: flex;
          align-items: center;
          gap: 4px;
          padding: 4px 10px 6px;
          font-size: 12px;
          color: var(--pa-text-3);
        }
        .cmd-item {
          display: flex;
          align-items: center;
          gap: 10px;
          width: 100%;
          padding: 9px 10px;
          border: 0;
          border-radius: 12px;
          background: none;
          color: var(--pa-text);
          font-size: 15px;
          text-align: left;
          cursor: pointer;
        }
        .cmd-item[data-active="true"] { background: var(--pa-fill); }
        .cmd-item:active { background: var(--pa-fill-strong); }
        .cmd-icon {
          flex-shrink: 0;
          width: 28px;
          height: 28px;
          border-radius: 9px;
          display: grid;
          place-items: center;
          background: var(--pa-accent-soft);
          color: var(--pa-accent);
        }
        .cmd-label { font-weight: 600; flex-shrink: 0; }
        .cmd-hint { font-size: 12.5px; color: var(--pa-text-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      `}</style>
    </div>
  );
}
