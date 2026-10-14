"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import dayjs from "dayjs";
import { BookOpen, Cake, ChevronDown, TriangleAlert } from "lucide-react";
import { AuthGate } from "@/components/auth-gate";
import { MemoryPopup } from "@/components/memory-popup";
import { Avatar, Card, Empty, LoadError, Loading, NavBar, Pill, Row, Section } from "@/components/ui";
import { api } from "@/lib/api";
import { personLabel, usePersons } from "@/lib/family";
import { personInitial, personTone, typeTone } from "@/lib/tone";
import type { Memory, Paged, Person, PersonaCard, Relation } from "@/lib/types";

export default function PersonPage() {
  return (
    <AuthGate>
      <PersonDetail />
    </AuthGate>
  );
}

function PersonDetail() {
  const { key } = useParams<{ key: string }>();
  const router = useRouter();
  const persons = usePersons();
  const [person, setPerson] = useState<Person | null>(null);
  const [relations, setRelations] = useState<Relation[]>([]);
  const [card, setCard] = useState<PersonaCard | null>(null);
  const [cardOpen, setCardOpen] = useState(false);
  const [history, setHistory] = useState<PersonaCard[]>([]);
  const [historyAll, setHistoryAll] = useState(false);
  const [memories, setMemories] = useState<Memory[] | null>(null);
  const [viewing, setViewing] = useState<string | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      const [detail, page] = await Promise.all([
        api.get<{ person: Person; relations: Relation[]; card: PersonaCard | null }>(`/persons/${key}`),
        api.get<Paged<Memory>>(`/memories?person=${key}&statuses=active&size=100`),
      ]);
      setPerson(detail.person);
      setCard(detail.card);
      setRelations(detail.relations);
      setMemories(page.items);
      api.get<PersonaCard[]>(`/persons/${key}/cards`).then(setHistory).catch(() => setHistory([]));
    } catch (err) {
      setError((err as Error).message);
    }
  }, [key]);

  useEffect(() => {
    void load();
  }, [load]);

  const groups = (memories ?? []).reduce<Record<string, { type: string; items: Memory[] }>>((acc, memory) => {
    (acc[memory.typeLabel] ??= { type: memory.type, items: [] }).items.push(memory);
    return acc;
  }, {});

  const tone = personTone(key, persons);
  const birthday = person && (person.birthday || person.lunarBirthday)
    ? person.birthdayBy === "lunar" && person.lunarBirthday ? `农历 ${person.lunarBirthday}` : person.birthday ? dayjs(person.birthday).format("M月D日") : `农历 ${person.lunarBirthday}`
    : "";

  return (
    <div className="pa-screen pa-page">
      <div className="pa-scroll" style={{ paddingBottom: "calc(32px + env(safe-area-inset-bottom))" }}>
        <NavBar onBack={() => router.back()} title={person ? personLabel(persons, person.personKey) : ""} />
        {!person ? (
          error ? <LoadError message={error} onRetry={load} /> : <Loading />
        ) : (
          <div className="pa-enter">
            <div className="person-hero" data-tone={tone}>
              <Avatar tone={tone} size={76}>{personInitial(person)}</Avatar>
              <h1 className="person-name">{person.roleLabel}</h1>
              {person.name && person.name !== person.roleLabel && <div className="person-real">{person.name}</div>}
              <div className="person-tags">
                {birthday && <Pill tone={tone}><Cake size={12} strokeWidth={2.4} />{birthday}</Pill>}
                {person.aliases.slice(0, 5).map((alias) => <Pill key={alias}>{alias}</Pill>)}
              </div>
            </div>

            {card && (
              <Section title="TA 现在是这样的">
                <Card>
                  <div className="person-card" data-open={cardOpen}>
                    {card.changes && card.version > 1 && <div className="person-change">最近的变化：{card.changes}</div>}
                    <div className="pa-pre person-card-body">{card.content}</div>
                    {!cardOpen && <div className="person-card-fade" />}
                  </div>
                  <button className="person-card-toggle" onClick={() => setCardOpen(!cardOpen)}>
                    <span>v{card.version} · {dayjs(card.createdAt).format("M月D日")}更新 · 基于 {card.memoryCount} 条记忆</span>
                    <span className="person-card-more">{cardOpen ? "收起" : "展开"}<ChevronDown size={15} style={{ transform: cardOpen ? "rotate(180deg)" : undefined, transition: "transform 0.25s" }} /></span>
                  </button>
                </Card>
              </Section>
            )}

            {history.length > 1 && (
              <Section title="TA 的变化" count={history.length}>
                <div id="changes" style={{ scrollMarginTop: 12 }} />
                <Card pad>
                  <div className="person-timeline">
                    {(historyAll ? history : history.slice(0, 5)).map((item) => (
                      <div key={item.cardKey} className="person-timeline-item" data-current={item.status === "active"}>
                        <div className="person-timeline-dot" />
                        <div className="person-timeline-head">
                          第 {item.version} 版 · {dayjs(item.createdAt).format("YYYY年M月D日")}
                          {item.status === "active" && <span className="person-timeline-now">现在</span>}
                          {item.createdBy === "manual" && <span>· 手工修订</span>}
                          {item.createdBy === "rollback" && <span>· 回滚</span>}
                        </div>
                        <div className="person-timeline-body">{item.version === 1 ? "第一次整理出人设" : item.changes || "（没有写变化说明）"}</div>
                      </div>
                    ))}
                  </div>
                  {history.length > 5 && (
                    <button className="person-card-toggle" style={{ borderTop: 0, padding: "8px 0 0" }} onClick={() => setHistoryAll(!historyAll)}>
                      <span>{historyAll ? "只看最近 5 次" : `还有 ${history.length - 5} 次更早的变化`}</span>
                    </button>
                  )}
                </Card>
              </Section>
            )}

            {person.profile && (
              <Section title="概况">
                <Card pad><div className="pa-pre" style={{ fontSize: 15, lineHeight: 1.75 }}>{person.profile}</div></Card>
              </Section>
            )}

            {relations.length > 0 && (
              <Section title="关系" count={relations.length}>
                <Card>
                  {relations.map((relation) => {
                    const other = relation.fromPerson === person.personKey ? relation.toPerson : relation.fromPerson;
                    const otherPerson = persons.find((p) => p.personKey === other);
                    const text = relation.fromPerson === person.personKey
                      ? <>{personLabel(persons, other)} 是 TA 的<b>{relation.relation}</b></>
                      : <>TA 是 {personLabel(persons, other)} 的<b>{relation.relation}</b></>;
                    const detail = [relation.addressAs && `称呼「${relation.addressAs}」`, relation.notes].filter(Boolean).join(" · ");
                    return (
                      <Row
                        key={relation.relationKey}
                        inset={64}
                        leading={<Avatar tone={personTone(other, persons)} size={36}>{personInitial(otherPerson)}</Avatar>}
                        title={text}
                        subtitle={(detail || relation.sensitivities) && (
                          <>
                            {detail}
                            {relation.sensitivities && (
                              <span className="person-warn"><TriangleAlert size={12} strokeWidth={2.4} />{relation.sensitivities}</span>
                            )}
                          </>
                        )}
                        onClick={otherPerson ? () => router.push(`/person/${other}`) : undefined}
                      />
                    );
                  })}
                </Card>
              </Section>
            )}

            {memories === null ? <Loading /> : memories.length === 0 ? (
              <Section title="记忆">
                <Card><Empty icon={<BookOpen size={22} />} title="还没有关于 TA 的记忆" hint="在「速记」里记下 TA 说过的话、在意的事" compact /></Card>
              </Section>
            ) : (
              Object.entries(groups).map(([label, group]) => (
                <Section key={label} title={<><span className="pa-dot" data-tone={typeTone(group.type)} style={{ background: "var(--tone-fg)" }} />{label}</>} count={group.items.length}>
                  <Card>
                    {group.items.map((memory) => (
                      <Row
                        key={memory.memoryKey}
                        title={memory.title}
                        trailing={
                          <>
                            {memory.importance >= 4 && <span className="person-key">重要</span>}
                            {dayjs(memory.occurredAt ?? memory.createdAt).format("YY.M.D")}
                          </>
                        }
                        onClick={() => setViewing(memory.memoryKey)}
                      />
                    ))}
                  </Card>
                </Section>
              ))
            )}
          </div>
        )}
      </div>
      <MemoryPopup memoryKey={viewing} onClose={() => setViewing(null)} />

      <style jsx global>{`
        .person-hero {
          display: flex;
          flex-direction: column;
          align-items: center;
          text-align: center;
          padding: 20px 20px 4px;
          background: radial-gradient(60% 70% at 50% 0%, color-mix(in srgb, var(--tone-fg) 12%, transparent), transparent 75%);
        }
        .person-name { font-family: var(--pa-font-display); font-size: 28px; font-weight: 700; margin: 14px 0 0; }
        .person-real { font-size: 14px; color: var(--pa-text-2); margin-top: 2px; }
        .person-tags { display: flex; flex-wrap: wrap; justify-content: center; gap: 6px; margin-top: 12px; }
        .person-card { position: relative; padding: 16px 16px 4px; max-height: 220px; overflow: hidden; transition: max-height 0.4s var(--pa-ease); }
        .person-card[data-open="true"] { max-height: 4000px; }
        .person-card-body { font-size: 15px; line-height: 1.8; }
        .person-card-fade { position: absolute; left: 0; right: 0; bottom: 0; height: 64px; background: linear-gradient(180deg, transparent, var(--pa-surface)); }
        .person-change { font-size: 13px; color: var(--pa-accent); background: var(--pa-accent-soft); padding: 8px 10px; border-radius: 10px; margin-bottom: 10px; line-height: 1.5; }
        .person-card-toggle {
          width: 100%;
          border: 0;
          border-top: 0.5px solid var(--pa-line);
          background: none;
          padding: 11px 16px;
          display: flex;
          justify-content: space-between;
          align-items: center;
          gap: 8px;
          font-size: 12px;
          color: var(--pa-text-3);
          cursor: pointer;
          text-align: left;
        }
        .person-card-more { display: inline-flex; align-items: center; gap: 2px; color: var(--pa-accent); font-weight: 600; font-size: 13px; flex-shrink: 0; }
        .person-timeline { position: relative; padding-left: 18px; }
        .person-timeline::before { content: ""; position: absolute; left: 4px; top: 6px; bottom: 6px; width: 1.5px; background: var(--pa-line); }
        .person-timeline-item { position: relative; padding-bottom: 14px; }
        .person-timeline-item:last-child { padding-bottom: 0; }
        .person-timeline-dot { position: absolute; left: -18px; top: 5px; width: 10px; height: 10px; border-radius: 50%; background: var(--pa-surface); border: 2px solid var(--pa-line-strong); }
        .person-timeline-item[data-current="true"] .person-timeline-dot { border-color: var(--pa-accent); background: var(--pa-accent); }
        .person-timeline-head { font-size: 12.5px; color: var(--pa-text-3); display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
        .person-timeline-now { color: var(--pa-accent); font-weight: 600; }
        .person-timeline-body { font-size: 14.5px; line-height: 1.65; margin-top: 3px; }
        .person-warn { display: flex; align-items: flex-start; gap: 4px; margin-top: 4px; color: var(--pa-danger); }
        .person-warn svg { flex-shrink: 0; margin-top: 3px; }
        .person-key { font-size: 11px; font-weight: 600; color: var(--pa-accent); background: var(--pa-accent-soft); padding: 1px 6px; border-radius: 6px; margin-right: 4px; }
      `}</style>
    </div>
  );
}
