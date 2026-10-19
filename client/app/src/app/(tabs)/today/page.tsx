"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import dayjs from "dayjs";
import { PullToRefresh, Toast } from "antd-mobile";
import { Check, Inbox, PartyPopper, Plane, Plus, RefreshCw } from "lucide-react";
import { api } from "@/lib/api";
import { notifyPendingChanged, useOnResume } from "@/lib/events";
import { personLabel, usePersons } from "@/lib/family";
import { personInitial, personTone, typeTone, type Tone } from "@/lib/tone";
import { shortTime, todayLabel, upcomingDay } from "@/lib/time";
import type { Memory, Note, Paged, PersonaCard, Reminder, Trip } from "@/lib/types";
import { MemoryPopup } from "@/components/memory-popup";
import { NoteSheet, noteStatus } from "@/components/note-sheet";
import { PushCard } from "@/components/push-card";
import { ReminderSheet, reminderMeta } from "@/components/reminder-sheet";
import { TripSheet } from "@/components/trip-sheet";
import { useMe } from "@/lib/role";
import { Avatar, Card, Empty, PageHeader, Pill, Row, Section, Skeleton, Tip } from "@/components/ui";

/**
 * 今天：近期提醒和行程 + 待我确认的记忆 + 最近交给助理整理的记录。
 */
export default function TodayPage() {
  const router = useRouter();
  const persons = usePersons();
  // 人设卡只给我自己看（老婆的账号不显示「她们最近的变化」）
  const { role } = useMe();
  const [loaded, setLoaded] = useState(false);
  const [pending, setPending] = useState<Memory[]>([]);
  const [viewing, setViewing] = useState<string | null>(null);
  const [notes, setNotes] = useState<Note[]>([]);
  const [openNote, setOpenNote] = useState<Note | null>(null);
  const [reminders, setReminders] = useState<Reminder[]>([]);
  const [trips, setTrips] = useState<Trip[]>([]);
  const [openReminder, setOpenReminder] = useState<Reminder | null>(null);
  const [tripOpen, setTripOpen] = useState(false);
  const [leaving, setLeaving] = useState<Record<string, string>>({});
  const [changes, setChanges] = useState<PersonaCard[]>([]);

  const load = useCallback(async () => {
    try {
      const [pendingPage, notePage, reminderList, tripList] = await Promise.all([
        api.get<Paged<Memory>>("/memories?statuses=pending&size=100"),
        api.get<Paged<Note>>("/notes?size=8"),
        api.get<Reminder[]>("/reminders?days=30"),
        api.get<Trip[]>("/trips"),
      ]);
      setPending(pendingPage.items);
      setNotes(notePage.items);
      setReminders(reminderList);
      setTrips(tripList);
      setLeaving({});
      // 人设变化是附加信息，拉不到不影响页面
      api.get<PersonaCard[]>("/persona-changes?days=7").then(setChanges).catch(() => setChanges([]));
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);
  useOnResume(() => void load());

  // 点承诺到期的推送会带 ?memory=xxx 进来，直接打开那条记忆
  useEffect(() => {
    const key = new URLSearchParams(window.location.search).get("memory");
    if (!key) return;
    setViewing(key);
    window.history.replaceState(null, "", window.location.pathname);
  }, []);

  // 由记忆派生的打开记忆；生日、节日、行程跳到对应页面；其余（含聊天里建的）在弹层里看全文、取消、回对话
  const openReminderItem = (item: Reminder) => {
    if (item.sourceType === "memory" && item.sourceKey) setViewing(item.sourceKey);
    else if (item.targetUrl && !item.targetUrl.startsWith("/today") && !item.targetUrl.startsWith("/chat/")) router.push(item.targetUrl);
    else setOpenReminder(item);
  };

  // 有正在整理的记录时每 5 秒只刷新记录列表；全部整理完再整页刷新一次，把新的待确认带出来
  const busy = notes.some(isBusy);
  useEffect(() => {
    if (!busy) return;
    const timer = setInterval(async () => {
      try {
        const page = await api.get<Paged<Note>>("/notes?size=8");
        if (page.items.some(isBusy)) {
          setNotes(page.items);
        } else {
          await load();
          notifyPendingChanged();
        }
      } catch {
        /* 下一轮再试 */
      }
    }, 5000);
    return () => clearInterval(timer);
  }, [busy, load]);

  /** 先播放离场动画，再从列表里拿掉。 */
  const dismiss = (key: string, how: string, after: () => void) => {
    setLeaving((value) => ({ ...value, [key]: how }));
    setTimeout(after, 320);
  };

  const setStatus = async (memory: Memory, status: string) => {
    try {
      await api.post(`/memories/${memory.memoryKey}/status`, { status });
      dismiss(memory.memoryKey, status === "active" ? "ok" : "no", () => setPending((list) => list.filter((m) => m.memoryKey !== memory.memoryKey)));
      notifyPendingChanged();
      Toast.show({ content: status === "active" ? "已确认，记下了" : "已驳回" });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    }
  };

  const summary = [
    pending.length ? `${pending.length} 条待你确认` : "",
  ].filter(Boolean).join("，");
  const nextReminder = reminders[0];
  const allClear = pending.length === 0;

  return (
    <PullToRefresh onRefresh={load}>
      <PageHeader
        eyebrow={todayLabel()}
        title="今天"
        subtitle={loaded ? (summary ? `${summary}。` : "今天可以轻松一点。") : " "}
      />

      <div className="today-stats">
        <Stat
          label={nextReminder ? `下次 · ${upcomingDay(nextReminder.remindAt).label}` : "近期提醒"}
          value={reminders.length}
          tone="sky"
          onClick={() => scrollTo("today-reminders")}
        />
        <Stat label="待确认" value={pending.length} tone="amber" onClick={() => scrollTo("today-pending")} />
      </div>

      {!loaded ? (
        <div style={{ marginTop: 26 }}><Skeleton rows={3} /></div>
      ) : (
        <>
          <PushCard placement="top" />

          <Section
            title="近期提醒"
            count={reminders.length}
            action={<button className="pa-link-btn" onClick={() => setTripOpen(true)}><Plus size={15} strokeWidth={2.4} />回老家行程</button>}
          >
            <div id="today-reminders" />
            {trips.length > 0 && (
              <div className="today-trips">
                {trips.map((trip) => {
                  const day = upcomingDay(trip.startAt);
                  return (
                    <button key={trip.tripKey} className="today-trip" onClick={() => router.push(`/trip/${trip.tripKey}`)}>
                      <div className="today-trip-icon"><Plane size={18} /></div>
                      <div style={{ flex: 1, minWidth: 0, textAlign: "left" }}>
                        <div className="today-trip-title">{trip.title}</div>
                        <div className="today-trip-sub">{dayjs(trip.startAt).format("M月D日 HH:mm")} 出发 · 行前简报</div>
                      </div>
                      <div className="today-trip-days">{day.days > 0 ? <><b>{day.days}</b>天后</> : <b>{day.label}</b>}</div>
                    </button>
                  );
                })}
              </div>
            )}
            {reminders.length === 0 ? (
              <Card><Empty icon={<Inbox size={22} />} title="未来 30 天没有提醒" hint="生日、节日、承诺到期会自动出现在这里" compact /></Card>
            ) : (
              <Card>
                {reminders.slice(0, 12).map((item) => {
                  const day = upcomingDay(item.remindAt);
                  const meta = reminderMeta[item.kind] ?? reminderMeta.manual;
                  return (
                    <Row
                      key={item.reminderKey}
                      inset={72}
                      leading={
                        <div className="today-date" data-tone={meta.tone} data-soon={day.days <= 1}>
                          <span className="today-date-m">{dayjs(item.remindAt).format("M月")}</span>
                          <span className="today-date-d">{dayjs(item.remindAt).format("D")}</span>
                        </div>
                      }
                      title={item.title}
                      clampTitle
                      subtitle={
                        <>
                          <span style={{ color: `var(--tone-${meta.tone})`, fontWeight: 600 }}>{meta.label}</span>
                          {" · "}{day.label} {dayjs(item.remindAt).format("HH:mm")}
                          {reminderDescription(item) && <span className="today-remind-desc">{reminderDescription(item)}</span>}
                        </>
                      }
                      chevron
                      onClick={() => openReminderItem(item)}
                    />
                  );
                })}
              </Card>
            )}
            {reminders.length > 12 && <div className="pa-hint" style={{ marginTop: 10, textAlign: "center" }}>只显示最近 12 条，还有 {reminders.length - 12} 条</div>}
          </Section>

          {pending.length > 0 && (
            <Tip id="today-review" title="助理拿不准的，会在这里等你确认" style={{ marginTop: 16 }}>
              点<b>确认</b>才会记下；记错了点<b>不对，驳回</b>。点卡片可以看完整内容。
            </Tip>
          )}
          {allClear && (
            <div className="today-clear">
              <div className="today-clear-icon"><PartyPopper size={22} /></div>
              <div>
                <div className="today-clear-title">今天都处理好了</div>
                <div className="today-clear-sub">有新的需要你确认的，这里会第一时间出现</div>
              </div>
            </div>
          )}

          {!allClear && <Section title="待确认的记忆" count={pending.length} countTone="accent" style={{ scrollMarginTop: 12 }}>
            <div id="today-pending" />
            {pending.length === 0 ? (
              <Card><Empty icon={<Check size={22} />} title="没有需要确认的" hint="助理拿不准的记忆会放在这里等你确认" compact /></Card>
            ) : (
              <div className="today-stack pa-stagger">
                {pending.map((memory, index) => (
                  <div key={memory.memoryKey} className="today-review" data-leaving={leaving[memory.memoryKey]} style={{ "--i": index } as React.CSSProperties}>
                    <div onClick={() => setViewing(memory.memoryKey)} style={{ cursor: "pointer" }}>
                      <div className="today-review-meta">
                        <Pill tone={typeTone(memory.type)}>{memory.typeLabel}</Pill>
                        <span className="today-who" data-tone={personTone(memory.subjectPerson, persons)}>
                          <span className="pa-dot" />
                          {memory.subjectPerson ? personLabel(persons, memory.subjectPerson) : "全家"}
                        </span>
                        <span className="today-when">{shortTime(memory.occurredAt ?? memory.createdAt)}</span>
                      </div>
                      <div className="today-review-title">{memory.title}</div>
                      {memory.content !== memory.title && <div className="today-review-body">{memory.content}</div>}
                      {memory.supersedesKey && (
                        <div className="today-review-change"><RefreshCw size={12} strokeWidth={2.4} />会替代一条旧记忆——她的想法变了</div>
                      )}
                    </div>
                    <div className="today-review-actions">
                      <button className="pa-btn" data-size="sm" data-variant="soft" onClick={() => setStatus(memory, "rejected")}>不对，驳回</button>
                      <button className="pa-btn" data-size="sm" data-variant="accent" onClick={() => setStatus(memory, "active")}><Check size={15} strokeWidth={2.6} />确认</button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </Section>}

          {role === "self" && changes.length > 0 && (
            <Section title="她们最近的变化" count={changes.length}>
              <Card>
                {changes.map((card) => {
                  const person = persons.find((p) => p.personKey === card.personKey);
                  return (
                    <Row
                      key={card.cardKey}
                      inset={64}
                      leading={<Avatar tone={personTone(card.personKey, persons)} size={36}>{personInitial(person)}</Avatar>}
                      title={card.changes || "人设卡更新了"}
                      clampTitle
                      subtitle={`${personLabel(persons, card.personKey)} · 第 ${card.version} 版 · ${shortTime(card.createdAt)}`}
                      onClick={() => router.push(`/person/${card.personKey}#changes`)}
                      chevron
                    />
                  );
                })}
              </Card>
            </Section>
          )}

          {notes.length > 0 && (
            <Section title="最近交给助理整理的">
              <Card>
                {notes.map((note) => {
                  const status = noteStatus[note.extractStatus];
                  const result = note.result;
                  return (
                    <Row
                      key={note.noteKey}
                      inset={60}
                      leading={<div className="today-note-icon" data-tone={status?.tone} data-busy={isBusy(note)}>{status?.icon}</div>}
                      title={result?.summary || note.rawText.slice(0, 40) || `${note.attachments.length} 张截图`}
                      clampTitle
                      subtitle={
                        note.extractStatus === "done" && result
                          ? `记下 ${result.added?.length ?? 0} · 待确认 ${result.pending?.length ?? 0} · 补充 ${result.updated?.length ?? 0}`
                          : `${status?.text ?? ""} · ${shortTime(note.createdAt)}`
                      }
                      chevron
                      onClick={() => setOpenNote(note)}
                    />
                  );
                })}
              </Card>
            </Section>
          )}

          <PushCard placement="bottom" />
        </>
      )}

      <MemoryPopup memoryKey={viewing} onClose={() => setViewing(null)} />
      <ReminderSheet reminder={openReminder} onClose={() => setOpenReminder(null)} onChanged={load} />
      <NoteSheet note={openNote} onClose={() => setOpenNote(null)} onChanged={load} onMemory={setViewing} />
      <TripSheet open={tripOpen} onClose={() => setTripOpen(false)} onCreated={() => void load()} />

      <style jsx global>{`
        .today-stats { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; padding: 10px 16px 0; }
        .today-stat-value { transition: color 0.3s; }
        .today-stat {
          appearance: none;
          text-align: left;
          border: 0.5px solid var(--pa-line);
          background: var(--pa-surface);
          border-radius: var(--pa-radius);
          padding: 12px 14px 11px;
          box-shadow: var(--pa-shadow-sm);
          cursor: pointer;
          transition: transform 0.15s var(--pa-ease);
          min-width: 0;
        }
        .today-stat:active { transform: scale(0.96); }
        .today-stat-value { font-size: 28px; font-weight: 700; line-height: 1.1; font-variant-numeric: tabular-nums; color: var(--pa-text); letter-spacing: -0.02em; }
        .today-stat[data-zero="false"] .today-stat-value { color: var(--tone-fg); }
        .today-stat-label { font-size: 12px; color: var(--pa-text-3); margin-top: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: flex; align-items: center; gap: 5px; }

        .today-stack { display: flex; flex-direction: column; gap: 10px; padding: 0 16px; }
        .today-clear {
          margin: 16px 16px 0;
          padding: 16px;
          display: flex;
          align-items: center;
          gap: 14px;
          border-radius: var(--pa-radius);
          background: linear-gradient(135deg, var(--pa-success-soft), color-mix(in srgb, var(--pa-success-soft) 30%, var(--pa-surface)));
          border: 0.5px solid color-mix(in srgb, var(--pa-success) 20%, transparent);
          animation: pa-enter 0.5s var(--pa-ease) both;
        }
        .today-clear-icon { width: 44px; height: 44px; border-radius: 22px; display: grid; place-items: center; background: var(--pa-surface); color: var(--pa-success); flex-shrink: 0; box-shadow: var(--pa-shadow-sm); }
        .today-clear-title { font-weight: 650; font-size: 16px; }
        .today-clear-sub { font-size: 12.5px; color: var(--pa-text-2); margin-top: 2px; }
        .today-review {
          background: var(--pa-surface);
          border: 0.5px solid var(--pa-line);
          border-radius: var(--pa-radius);
          box-shadow: var(--pa-shadow-sm);
          padding: 14px 16px 12px;
          transition: opacity 0.3s, transform 0.3s var(--pa-ease);
        }
        .today-review[data-leaving="ok"] { opacity: 0; transform: translateX(24px) scale(0.98); }
        .today-review[data-leaving="no"] { opacity: 0; transform: translateX(-24px) scale(0.98); }
        .today-review-meta { display: flex; align-items: center; gap: 8px; }
        .today-who { display: inline-flex; align-items: center; gap: 5px; font-size: 12.5px; color: var(--pa-text-2); font-weight: 500; }
        .today-when { margin-left: auto; font-size: 12px; color: var(--pa-text-3); }
        .today-review-title { font-size: 16px; font-weight: 600; line-height: 1.5; margin-top: 10px; }
        .today-review-body {
          font-size: 14px;
          color: var(--pa-text-2);
          line-height: 1.6;
          margin-top: 4px;
          display: -webkit-box;
          -webkit-line-clamp: 3;
          -webkit-box-orient: vertical;
          overflow: hidden;
        }
        .today-review-change {
          display: inline-flex;
          align-items: center;
          gap: 5px;
          margin-top: 8px;
          font-size: 12px;
          font-weight: 500;
          color: var(--pa-warning);
        }
        .today-review-actions {
          display: grid;
          grid-template-columns: 1fr 1fr;
          gap: 8px;
          margin-top: 14px;
          padding-top: 12px;
          border-top: 0.5px solid var(--pa-line);
        }

        .today-trips { display: flex; flex-direction: column; gap: 10px; padding: 0 16px 10px; }
        .today-trip {
          appearance: none;
          display: flex;
          align-items: center;
          gap: 12px;
          padding: 14px 16px;
          border: 0;
          border-radius: var(--pa-radius);
          background: linear-gradient(135deg, var(--tone-sky-bg), color-mix(in srgb, var(--tone-sky-bg) 30%, var(--pa-surface)));
          color: var(--pa-text);
          cursor: pointer;
          box-shadow: var(--pa-shadow-sm);
          transition: transform 0.15s;
        }
        .today-trip:active { transform: scale(0.98); }
        .today-trip-icon { width: 38px; height: 38px; border-radius: 12px; display: grid; place-items: center; background: var(--pa-surface); color: var(--tone-sky); flex-shrink: 0; }
        .today-trip-title { font-weight: 600; font-size: 15.5px; }
        .today-trip-sub { font-size: 12.5px; color: var(--pa-text-2); margin-top: 2px; }
        .today-trip-days { font-size: 12px; color: var(--pa-text-2); text-align: right; flex-shrink: 0; }
        .today-trip-days b { font-size: 22px; font-weight: 700; color: var(--tone-sky); margin-right: 2px; font-variant-numeric: tabular-nums; }

        .today-date {
          width: 44px;
          height: 46px;
          border-radius: 12px;
          background: var(--pa-surface-2);
          border: 0.5px solid var(--pa-line);
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          flex-shrink: 0;
          line-height: 1.1;
        }
        .today-date[data-soon="true"] { background: var(--tone-bg); border-color: transparent; }
        .today-date-m { font-size: 10px; font-weight: 600; color: var(--tone-fg); letter-spacing: 0.02em; }
        .today-date-d { font-size: 19px; font-weight: 700; font-variant-numeric: tabular-nums; color: var(--pa-text); }
        .today-remind-desc { display: block; margin-top: 2px; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }

        .today-note-icon {
          width: 32px;
          height: 32px;
          border-radius: 10px;
          display: grid;
          place-items: center;
          background: var(--tone-bg);
          color: var(--tone-fg);
          flex-shrink: 0;
        }
        .today-note-icon[data-busy="true"] svg { animation: pa-spin 1.2s linear infinite; }
      `}</style>
    </PullToRefresh>
  );
}

function isBusy(note: Note): boolean {
  return note.extractStatus === "queued" || note.extractStatus === "running";
}

function scrollTo(id: string) {
  document.getElementById(id)?.parentElement?.scrollIntoView({ behavior: "smooth", block: "start" });
}

function Stat({ label, value, tone, onClick }: { label: string; value: number; tone: Tone; onClick: () => void }) {
  return (
    <button className="today-stat" data-tone={tone} data-zero={value === 0} onClick={onClick}>
      <div className="today-stat-value">{value}</div>
      <div className="today-stat-label"><span className="pa-dot" />{label}</div>
    </button>
  );
}

function reminderDescription(item: Reminder): string {
  if (item.kind === "trip") return "打开查看行前简报";
  const text = item.body.replace(/\s+/g, " ").trim();
  return text.length > 80 ? `${text.slice(0, 80)}…` : text;
}
