"use client";

import { useRouter } from "next/navigation";
import { Dialog, PullToRefresh } from "antd-mobile";
import { Compass, LogOut, Monitor } from "lucide-react";
import { api, setToken } from "@/lib/api";
import { resetGuide } from "@/lib/guide";
import { personLabel, personsError, refreshFamily, usePersons } from "@/lib/family";
import { personInitial, personTone } from "@/lib/tone";
import { Avatar, Card, LoadError, PageHeader, Row, Section, Skeleton, Tip } from "@/components/ui";

const sides: { key: string; title: string }[] = [
  { key: "core", title: "我们家" },
  { key: "self", title: "我这边" },
  { key: "wife", title: "老婆那边" },
];

export default function FamilyPage() {
  const router = useRouter();
  const persons = usePersons();

  const logout = async () => {
    const confirmed = await Dialog.confirm({ title: "退出登录？", content: "退出后需要重新输入账号密码。", confirmText: "退出", cancelText: "取消" });
    if (!confirmed) return;
    await api.post("/auth/logout").catch(() => {});
    setToken("");
    router.replace("/login");
  };

  const core = persons.filter((p) => p.side === "core");
  const relatives = persons.filter((p) => p.side !== "core").length;

  const error = persons.length === 0 ? personsError() : "";

  return (
    <PullToRefresh onRefresh={refreshFamily}>
      <PageHeader
        eyebrow={persons.length ? `${core.length} 位家人 · ${relatives} 位亲戚` : "家里的人"}
        title="家人"
      />

      {persons.length === 0 ? (
        error ? <LoadError message={error} onRetry={() => void refreshFamily()} /> : <div style={{ marginTop: 8 }}><Skeleton rows={4} /></div>
      ) : (
        <>
          <Tip id="family" title="点开一位家人，看助理对 TA 的了解">
            TA 说过的话、在意的事、喜好和雷区都在里面，记得越多越准。
          </Tip>

          {core.length > 0 && (
            <Section title="我们家" style={{ marginTop: 16 }}>
              <div className="family-core pa-stagger">
                {core.map((person, index) => (
                  <button key={person.personKey} style={{ "--i": index } as React.CSSProperties} className="family-tile" data-tone={personTone(person.personKey, persons)} onClick={() => router.push(`/person/${person.personKey}`)}>
                    <Avatar tone={personTone(person.personKey, persons)} size={44}>{personInitial(person)}</Avatar>
                    <div className="family-tile-name">{person.roleLabel}</div>
                    <div className="family-tile-sub">{person.name && person.name !== person.roleLabel ? person.name : person.aliases.slice(0, 2).join("、") || " "}</div>
                  </button>
                ))}
              </div>
            </Section>
          )}

          {sides.slice(1).map((side) => {
            const members = persons.filter((p) => p.side === side.key);
            if (members.length === 0) return null;
            return (
              <Section key={side.key} title={side.title} count={members.length}>
                <Card>
                  {members.map((person) => (
                    <Row
                      key={person.personKey}
                      inset={68}
                      leading={<Avatar tone={personTone(person.personKey, persons)} size={40}>{personInitial(person)}</Avatar>}
                      title={personLabel(persons, person.personKey)}
                      subtitle={person.aliases.length ? `也叫 ${person.aliases.slice(0, 4).join("、")}` : undefined}
                      clampTitle
                      chevron
                      onClick={() => router.push(`/person/${person.personKey}`)}
                    />
                  ))}
                </Card>
              </Section>
            );
          })}

          <Section title="帮助与设置">
            <Card>
              <Row
                inset={60}
                leading={<div className="family-icon" data-tone="clay"><Compass size={17} /></div>}
                title="使用指南"
                subtitle="重新看一遍新手引导和页面小提示"
                chevron
                onClick={resetGuide}
              />
              <Row
                inset={60}
                leading={<div className="family-icon"><Monitor size={17} /></div>}
                title="添加亲戚、修改档案"
                subtitle="在电脑上打开管理端操作"
              />
              <Row
                inset={60}
                leading={<div className="family-icon" data-danger="true"><LogOut size={17} /></div>}
                title={<span style={{ color: "var(--pa-danger)" }}>退出登录</span>}
                onClick={logout}
              />
            </Card>
          </Section>
        </>
      )}

      <style jsx global>{`
        .family-core { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; padding: 0 16px; }
        .family-tile {
          appearance: none;
          text-align: left;
          border: 0.5px solid var(--pa-line);
          border-radius: var(--pa-radius);
          padding: 16px 14px 14px;
          background:
            radial-gradient(100% 80% at 100% 0%, color-mix(in srgb, var(--tone-fg) 12%, transparent), transparent 70%),
            var(--pa-surface);
          box-shadow: var(--pa-shadow-sm);
          cursor: pointer;
          transition: transform 0.18s var(--pa-ease);
        }
        .family-tile:active { transform: scale(0.96); }
        .family-tile-name { font-family: var(--pa-font-display); font-weight: 700; font-size: 18px; margin-top: 12px; color: var(--pa-text); }
        .family-tile-sub { font-size: 12.5px; color: var(--pa-text-3); margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .family-icon {
          width: 32px;
          height: 32px;
          border-radius: 10px;
          display: grid;
          place-items: center;
          background: var(--pa-fill-strong);
          color: var(--pa-text-2);
          flex-shrink: 0;
        }
        .family-icon[data-tone] { background: var(--tone-bg); color: var(--tone-fg); }
        .family-icon[data-danger="true"] { background: var(--pa-danger-soft); color: var(--pa-danger); }
      `}</style>
    </PullToRefresh>
  );
}
