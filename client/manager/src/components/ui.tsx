"use client";

import type { CSSProperties, ReactNode } from "react";
import { personInitial, personTone, type Tone } from "@/lib/tone";
import { useConsole } from "./console";

export function PageHeader({ eyebrow, title, description, actions }: { eyebrow?: ReactNode; title: ReactNode; description?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="pa-page-head">
      <div style={{ minWidth: 0 }}>
        {eyebrow && <div className="pa-page-eyebrow">{eyebrow}</div>}
        <h1 className="pa-page-title">{title}</h1>
        {description && <div className="pa-page-desc">{description}</div>}
      </div>
      {actions && <div className="pa-page-actions">{actions}</div>}
    </div>
  );
}

export function Panel({ title, extra, children, bodyPadding, style }: { title?: ReactNode; extra?: ReactNode; children: ReactNode; bodyPadding?: boolean; style?: CSSProperties }) {
  return (
    <div className="pa-panel" style={style}>
      {(title || extra) && (
        <div className="pa-panel-head">
          <div className="pa-panel-title">{title}</div>
          {extra && <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>{extra}</div>}
        </div>
      )}
      {bodyPadding ? <div className="pa-panel-body">{children}</div> : children}
    </div>
  );
}

export function Pill({ tone, children, dot, style }: { tone?: Tone; children: ReactNode; dot?: boolean; style?: CSSProperties }) {
  return (
    <span className="pa-pill" data-tone={tone} style={style}>
      {dot && <span className="pa-dot" />}
      {children}
    </span>
  );
}

export function Status({ tone, children, pulse }: { tone?: Tone; children: ReactNode; pulse?: boolean }) {
  return (
    <span className="pa-status" data-tone={tone} data-pulse={pulse}>
      <span className="pa-dot" />
      {children}
    </span>
  );
}

export function Avatar({ tone, size = 32, children, square }: { tone?: Tone; size?: number; children: ReactNode; square?: boolean }) {
  const emoji = typeof children === "string" && !/^[一-龥A-Za-z0-9]/.test(children);
  return (
    <span className="pa-avatar" data-tone={tone} data-emoji={emoji} style={{ "--pa-avatar": `${size}px`, borderRadius: square ? "30%" : undefined } as CSSProperties}>
      {children}
    </span>
  );
}

/** 头像 + 称呼。key 为空时显示「全家」。 */
export function PersonTag({ personKey, size = 26, muted }: { personKey: string; size?: number; muted?: boolean }) {
  const { persons, personName } = useConsole();
  if (!personKey) {
    return (
      <span className="pa-person">
        <Avatar tone="slate" size={size}>家</Avatar>
        <span className="pa-person-name" style={muted ? { color: "var(--pa-text-2)" } : undefined}>全家</span>
      </span>
    );
  }
  const person = persons.find((p) => p.personKey === personKey);
  return (
    <span className="pa-person">
      <Avatar tone={personTone(personKey, persons)} size={size}>{personInitial(person)}</Avatar>
      <span className="pa-person-name" style={muted ? { color: "var(--pa-text-2)" } : undefined}>{personName(personKey)}</span>
    </span>
  );
}

export function Kpi({ label, value, tone }: { label: ReactNode; value: ReactNode; tone?: Tone }) {
  return (
    <div className="pa-kpi" data-tone={tone}>
      <div className="pa-kpi-value" style={tone ? { color: "var(--tone-fg)" } : undefined}>{value}</div>
      <div className="pa-kpi-label">{tone && <span className="pa-dot" />}{label}</div>
    </div>
  );
}

export function Callout({ tone = "amber", icon, children, style }: { tone?: Tone; icon?: ReactNode; children: ReactNode; style?: CSSProperties }) {
  return (
    <div className="pa-callout" data-tone={tone} style={style}>
      {icon}
      <div style={{ minWidth: 0 }}>{children}</div>
    </div>
  );
}
