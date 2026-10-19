"use client";

import { useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { Popup } from "antd-mobile";
import { ChevronLeft, ChevronRight, CloudOff, Lightbulb, X } from "lucide-react";
import { useTip } from "@/lib/guide";
import type { Tone } from "@/lib/tone";

/** 弹层关闭动画期间还要显示上一次的内容，这里记住最后一个非空值。 */
export function useLatest<T>(value: T | null | undefined): T | null {
  const ref = useRef<T | null>(null);
  if (value !== null && value !== undefined) ref.current = value;
  return ref.current;
}

/* ---------- 页头 ---------- */

export function PageHeader({ eyebrow, title, subtitle, action, leading }: { eyebrow?: ReactNode; title: ReactNode; subtitle?: ReactNode; action?: ReactNode; leading?: ReactNode }) {
  return (
    <header className="pa-header">
      {eyebrow && <div className="pa-eyebrow">{eyebrow}</div>}
      <div className="pa-header-row">
        {leading ? (
          <div style={{ display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
            {leading}
            <h1 className="pa-title">{title}</h1>
          </div>
        ) : (
          <h1 className="pa-title">{title}</h1>
        )}
        {action}
      </div>
      {subtitle && <div className="pa-subtitle">{subtitle}</div>}
    </header>
  );
}

export function NavBar({ title, subtitle, onBack, leading, left, right }: { title?: ReactNode; subtitle?: ReactNode; onBack?: () => void; leading?: ReactNode; left?: ReactNode; right?: ReactNode }) {
  return (
    <div className="pa-nav">
      <div className="pa-nav-inner">
        <div>
          {leading}
          {!leading && onBack && (
            <button className="pa-icon-btn" onClick={onBack} aria-label="返回">
              <ChevronLeft size={24} strokeWidth={2} />
            </button>
          )}
        </div>
        <div className="pa-nav-center">
          {left}
          <div style={{ minWidth: 0, textAlign: left ? "left" : "center" }}>
            <div className="pa-nav-title">{title}</div>
            {subtitle && <div className="pa-nav-sub">{subtitle}</div>}
          </div>
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end" }}>{right}</div>
      </div>
    </div>
  );
}

/* ---------- 分区 / 卡片 / 行 ---------- */

export function Section({ title, count, countTone, action, children, style }: {
  title?: ReactNode;
  count?: number;
  countTone?: "accent";
  action?: ReactNode;
  children: ReactNode;
  style?: CSSProperties;
}) {
  return (
    <section className="pa-section" style={style}>
      {(title || action) && (
        <div className="pa-section-head">
          <div className="pa-section-title">
            {title}
            {count !== undefined && count > 0 && <span className="pa-count" data-tone={countTone}>{count}</span>}
          </div>
          {action}
        </div>
      )}
      {children}
    </section>
  );
}

export function Card({ children, pad, style, className }: { children: ReactNode; pad?: boolean; style?: CSSProperties; className?: string }) {
  return <div className={`pa-card${pad ? " pa-card-pad" : ""}${className ? ` ${className}` : ""}`} style={style}>{children}</div>;
}

export function Row({ leading, title, subtitle, trailing, onClick, chevron, clampTitle, clampSub, inset, style }: {
  leading?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  trailing?: ReactNode;
  onClick?: () => void;
  chevron?: boolean;
  clampTitle?: boolean;
  clampSub?: boolean;
  /** 分隔线左缩进；有头像时对齐到文字 */
  inset?: number;
  style?: CSSProperties;
}) {
  return (
    <div
      className="pa-row"
      data-clickable={!!onClick}
      onClick={onClick}
      style={{ ...(inset !== undefined ? ({ "--pa-row-inset": `${inset}px` } as CSSProperties) : {}), ...style }}
    >
      {leading}
      <div className="pa-row-main">
        <div className="pa-row-title" data-clamp={clampTitle ? "1" : undefined}>{title}</div>
        {subtitle && <div className="pa-row-sub" data-clamp={clampSub ? "2" : undefined}>{subtitle}</div>}
      </div>
      {trailing !== undefined && <div className="pa-row-trail">{trailing}</div>}
      {chevron && <ChevronRight size={18} className="pa-chevron" />}
    </div>
  );
}

/* ---------- 头像 / 胶囊 ---------- */

export function Avatar({ tone, size = 40, shape, children }: { tone?: Tone; size?: number; shape?: "circle" | "squircle"; children: ReactNode }) {
  const isText = typeof children === "string" && /^[\u4e00-\u9fa5A-Za-z]/.test(children);
  return (
    <div className="pa-avatar" data-tone={tone} data-shape={shape} style={{ "--pa-avatar": `${size}px` } as CSSProperties}>
      <span className={isText ? "pa-avatar-text" : undefined}>{children}</span>
    </div>
  );
}

export function Pill({ tone, children, dot }: { tone?: Tone; children: ReactNode; dot?: boolean }) {
  return (
    <span className="pa-pill" data-tone={tone}>
      {dot && <span className="pa-dot" />}
      {children}
    </span>
  );
}

/* ---------- 按钮 ---------- */

export function Button({ children, onClick, variant, size, block, loading, disabled, type = "button", style }: {
  children: ReactNode;
  onClick?: () => void;
  variant?: "ink" | "accent" | "soft" | "danger";
  size?: "sm" | "md" | "lg";
  block?: boolean;
  loading?: boolean;
  disabled?: boolean;
  type?: "button" | "submit";
  style?: CSSProperties;
}) {
  return (
    <button
      type={type}
      className="pa-btn"
      data-variant={variant}
      data-size={size}
      data-block={block}
      disabled={disabled || loading}
      onClick={onClick}
      style={style}
    >
      {loading && <span className="pa-btn-spin" />}
      {children}
    </button>
  );
}

/* ---------- 胶囊选择 ---------- */

export interface ChipOption {
  value: string;
  label: ReactNode;
  tone?: Tone;
}

export function Chips({ options, value, onChange, multiple, scroll, rows, size }: {
  options: ChipOption[];
  value: string[];
  onChange: (value: string[]) => void;
  multiple?: boolean;
  /** 单行横滑 */
  scroll?: boolean;
  /** 多行横滑（按列排），选项多时省纵向空间 */
  rows?: number;
  size?: "sm";
}) {
  const toggle = (option: string) => {
    if (!multiple) {
      onChange([option]);
      return;
    }
    onChange(value.includes(option) ? value.filter((v) => v !== option) : [...value, option]);
  };
  return (
    <div
      className="pa-chips"
      data-scroll={scroll || !!rows}
      data-rows={rows}
      style={rows ? ({ gridTemplateRows: `repeat(${rows}, auto)` } as CSSProperties) : undefined}
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          className="pa-chip"
          data-size={size}
          data-tone={option.tone}
          data-selected={value.includes(option.value)}
          onClick={() => toggle(option.value)}
        >
          {option.tone && <span className="pa-dot" />}
          {option.label}
        </button>
      ))}
    </div>
  );
}

/* ---------- 分段控件（带滑块） ---------- */

export function Segmented({ options, value, onChange }: { options: { value: string; label: ReactNode }[]; value: string; onChange: (value: string) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const index = Math.max(0, options.findIndex((o) => o.value === value));
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const update = () => setWidth((element.clientWidth - 6) / options.length);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [options.length]);
  return (
    <div className="pa-segmented" ref={ref}>
      {width > 0 && <div className="pa-segmented-thumb" style={{ width, transform: `translateX(${index * width}px)` }} />}
      {options.map((option) => (
        <button key={option.value} type="button" data-active={option.value === value} onClick={() => onChange(option.value)}>
          {option.label}
        </button>
      ))}
    </div>
  );
}

/* ---------- 空状态 / 加载 ---------- */

export function Empty({ icon, title, hint, compact }: { icon?: ReactNode; title: ReactNode; hint?: ReactNode; compact?: boolean }) {
  return (
    <div className="pa-empty" style={compact ? { padding: "20px 24px" } : undefined}>
      {icon && <div className="pa-empty-icon">{icon}</div>}
      <div className="pa-empty-title">{title}</div>
      {hint && <div className="pa-empty-hint">{hint}</div>}
    </div>
  );
}

/** 整页数据没拉下来时用，避免一直转圈。 */
export function LoadError({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <Empty
      icon={<CloudOff size={22} />}
      title="没加载出来"
      hint={
        <>
          {message}
          {onRetry && (
            <div style={{ marginTop: 14 }}>
              <Button size="sm" variant="soft" onClick={onRetry}>重试</Button>
            </div>
          )}
        </>
      }
    />
  );
}

export function Loading() {
  return <div className="pa-loading"><i /><i /><i /></div>;
}

export function Skeleton({ rows = 3 }: { rows?: number }) {
  return (
    <Card>
      {Array.from({ length: rows }, (_, index) => (
        <div key={index} className="pa-row">
          <div className="pa-skeleton" style={{ width: 40, height: 40, borderRadius: 20 }} />
          <div style={{ flex: 1 }}>
            <div className="pa-skeleton" style={{ height: 14, width: `${70 - index * 12}%` }} />
            <div className="pa-skeleton" style={{ height: 11, width: "40%", marginTop: 8 }} />
          </div>
        </div>
      ))}
    </Card>
  );
}

/* ---------- 小提示 ---------- */

/** 第一次用到某个页面时的一句话说明，点「知道了」收起，之后不再出现。 */
export function Tip({ id, icon, title, children, style }: { id: string; icon?: ReactNode; title: ReactNode; children?: ReactNode; style?: CSSProperties }) {
  const [show, dismiss] = useTip(id);
  const [leaving, setLeaving] = useState(false);
  if (!show) return null;
  return (
    <div className="pa-tip" data-leaving={leaving} style={style} role="note">
      <div className="pa-tip-icon">{icon ?? <Lightbulb size={16} strokeWidth={2.2} />}</div>
      <div className="pa-tip-main">
        <div className="pa-tip-title">{title}</div>
        {children && <div className="pa-tip-body">{children}</div>}
        <button
          type="button"
          className="pa-tip-ok"
          onClick={() => {
            setLeaving(true);
            setTimeout(() => {
              dismiss();
              setLeaving(false);
            }, 320);
          }}
        >
          知道了
        </button>
      </div>
    </div>
  );
}

/* ---------- 底部弹层 ---------- */

export function Sheet({ open, onClose, title, extra, children, height }: {
  open: boolean;
  onClose: () => void;
  title?: ReactNode;
  extra?: ReactNode;
  children: ReactNode;
  height?: string;
}) {
  return (
    <Popup
      visible={open}
      onMaskClick={onClose}
      onClose={onClose}
      bodyClassName="pa-sheet"
      bodyStyle={height ? { height } : undefined}
      destroyOnClose
    >
      <div className="pa-sheet-grab" />
      <div className="pa-sheet-head">
        <div className="pa-sheet-title">{title}</div>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          {extra}
          <button className="pa-sheet-close" onClick={onClose} aria-label="关闭"><X size={16} strokeWidth={2.4} /></button>
        </div>
      </div>
      <div className="pa-sheet-body">{children}</div>
    </Popup>
  );
}
