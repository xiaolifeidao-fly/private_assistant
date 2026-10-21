"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import dayjs from "dayjs";
import { PullToRefresh, Toast } from "antd-mobile";
import { AlertTriangle, CalendarClock, ChevronRight, MapPin, Moon } from "lucide-react";
import { AuthGate } from "@/components/auth-gate";
import { chartMarks, DayPlanSheet, HourlyChart, LocationSheet, PartCard, WeatherStyles } from "@/components/weather";
import { LoadError, Loading, NavBar, Section, Tip } from "@/components/ui";
import { useOnResume } from "@/lib/events";
import { useMe } from "@/lib/role";
import { deg, planLine, weatherIcon, weatherToday, type DayPlan, type WeatherToday } from "@/lib/weather";

export default function WeatherPage() {
  return <AuthGate><WeatherBoard /></AuthGate>;
}

/** 宝宝穿衣：现在的天气、今天穿什么、几点前到家；今天的安排可以临时改；今天推过的提醒。 */
function WeatherBoard() {
  const router = useRouter();
  const { role, ready } = useMe();
  const [data, setData] = useState<WeatherToday | null>(null);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<DayPlan | null>(null);
  const [locating, setLocating] = useState(false);

  useEffect(() => {
    if (ready && role === "elder") router.replace("/elder/weather");
  }, [ready, role, router]);

  const load = useCallback(async () => {
    try {
      setData(await weatherToday());
      setError("");
    } catch (err) {
      setError((err as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  useOnResume(() => void load());

  const snapshot = data?.snapshot;
  const advice = data?.advice;
  const today = snapshot?.days?.find((day) => day.date === dayjs().format("YYYY-MM-DD"));
  const todayHours = (snapshot?.hours ?? []).filter((hour) => dayjs(hour.at).isSame(dayjs(), "day"));
  const evening = data?.evening;
  // 今天要出门的都过去了，就把重点放到今晚和明早
  const restOfToday = advice?.parts ?? [];

  return (
    <div className="pa-screen pa-page wx-page">
      <NavBar
        onBack={() => (window.history.length > 1 ? router.back() : router.push("/"))}
        title={`${data?.child ?? "宝宝"}穿什么`}
        subtitle={data?.location.name}
        right={role === "self" ? (
          <button className="pa-icon-btn" onClick={() => setLocating(true)} aria-label="换地方"><MapPin size={20} /></button>
        ) : undefined}
      />
      <PullToRefresh onRefresh={load}>
        {!data && !error && <Loading />}
        {error && <LoadError message={error} onRetry={() => void load()} />}
        {data && (
          <div style={{ paddingBottom: 40 }}>
            {data.error ? (
              <div className="wx-hero" data-tone="slate">
                <div className="wx-hero-text">天气暂时查不到：{data.error}</div>
                <small>出门前看一眼窗外，给宝宝备一件外套。</small>
              </div>
            ) : snapshot && advice && (
              <div className="wx-hero" data-tone={advice.parts.some((part) => part.directHome || part.skip) || advice.warnings.length ? "rose" : "amber"}>
                <div className="wx-hero-now">
                  <span className="wx-hero-icon">{weatherIcon(snapshot.now.text)}</span>
                  <div>
                    <div className="wx-hero-temp pa-num">{deg(snapshot.now.temp)}</div>
                    <div className="wx-hero-sub">
                      体感 {deg(snapshot.now.feels)} · {snapshot.now.text} · {snapshot.now.wind}
                      {snapshot.now.aqi > 0 && ` · 空气 ${snapshot.now.aqi}`}
                    </div>
                    {today && <div className="wx-hero-sub">今天 {deg(today.low)}–{deg(today.high)} · {today.sunset} 天黑</div>}
                  </div>
                </div>
                {advice.warnings.map((warning) => (
                  <div key={warning.type + warning.level} className="wx-warning"><AlertTriangle size={15} /> {warning.type}{warning.level}预警</div>
                ))}
                <div className="wx-hero-text">{advice.headline}</div>
                {snapshot.stale && <small>（暂时取不到新的，用的是 {dayjs(snapshot.fetchedAt).format("HH:mm")} 的天气）</small>}
              </div>
            )}

            <button className="wx-plan" onClick={() => setEditing(data.plan)}>
              <CalendarClock size={18} />
              <span>
                <b>今天{data.plan.override ? "（改过）" : ""}</b>
                <small>{planLine(data.plan)}</small>
              </span>
              <em>改</em>
              <ChevronRight size={16} />
            </button>

            {todayHours.length > 1 && advice && (
              <div className="wx-chart-card">
                <HourlyChart hours={todayHours} marks={chartMarks(advice.parts)} sunset={today?.sunset} />
                <div className="wx-chart-legend"><i data-kind="go" />出门 <i data-kind="back" />最晚到家 <i data-kind="rain" />有雨</div>
              </div>
            )}

            {restOfToday.length > 0 && (
              <Section title="今天">
                <div className="wx-parts">
                  {restOfToday.map((part) => <PartCard key={part.kind + part.start} part={part} />)}
                </div>
              </Section>
            )}

            {evening && (
              <Section title="今晚和明早">
                <div className="wx-parts">
                  {evening.night && (
                    <section className="wx-part">
                      <div className="wx-part-head"><b><Moon size={15} style={{ verticalAlign: -2 }} /> 今晚睡觉</b><span>最低 {deg(evening.night.low)}</span></div>
                      <div style={{ fontSize: 14.5 }}>{evening.night.bedding}</div>
                      {evening.night.tips.length > 0 && <div className="wx-notes">{evening.night.tips.join("；")}</div>}
                    </section>
                  )}
                  {evening.parts.map((part) => <PartCard key={part.kind + part.start} part={part} />)}
                  <button className="wx-plan" style={{ margin: 0, width: "100%" }} onClick={() => setEditing(data.tomorrow)}>
                    <CalendarClock size={18} />
                    <span><b>明天{data.tomorrow.override ? "（改过）" : ""}</b><small>{planLine(data.tomorrow)}</small></span>
                    <em>改</em>
                    <ChevronRight size={16} />
                  </button>
                </div>
              </Section>
            )}

            <Section title="今天的提醒">
              {data.briefs.length === 0 ? (
                <div className="pa-hint">每天 {data.slots.morning}、{data.slots.noon}{data.slots.prePickup > 0 ? `、接之前 ${data.slots.prePickup} 分钟` : ""}、{data.slots.evening} 会推到手机上。</div>
              ) : (
                <div className="wx-parts">
                  {[...data.briefs].reverse().map((brief) => (
                    <div key={brief.briefKey} className="wx-brief" data-skipped={brief.source === "skipped"}>
                      <div className="wx-brief-head"><b>{brief.slotLabel}</b><small className="pa-num">{dayjs(brief.createdAt).format("HH:mm")}</small></div>
                      <div className="pa-pre">{brief.content}</div>
                    </div>
                  ))}
                </div>
              )}
            </Section>

            {snapshot?.clothing && <div className="pa-hint" style={{ marginTop: 16 }}>中国天气网穿衣指数（参考）：{snapshot.clothing}</div>}
            <Tip id="weather-page" title="安排变了就点「改」" style={{ marginTop: 16 }}>
              今天晚点接、不去幼儿园、要去外婆家，改一下这一天就行，提醒会按新的安排算。也可以跟「提醒」助手说一句「今天 4 点 10 分接」。
            </Tip>
          </div>
        )}
      </PullToRefresh>

      <DayPlanSheet plan={editing} open={!!editing} onClose={() => setEditing(null)} onSaved={() => void load()} />
      <LocationSheet open={locating} current={data?.location} onClose={() => setLocating(false)} onSaved={() => { Toast.show({ content: "正在取那边的天气…" }); void load(); }} />
      <WeatherStyles />
      <style jsx global>{`
        .wx-hero {
          margin: 8px 16px 0; padding: 16px; border-radius: var(--pa-radius-lg); border: 0.5px solid var(--pa-line); box-shadow: var(--pa-shadow);
          background: radial-gradient(120% 90% at 100% 0%, color-mix(in srgb, var(--tone-fg) 16%, transparent) 0%, transparent 60%),
            linear-gradient(180deg, var(--pa-surface) 0%, color-mix(in srgb, var(--tone-bg) 60%, var(--pa-surface)) 100%);
          display: flex; flex-direction: column; gap: 10px;
        }
        .wx-hero small { font-size: 12.5px; color: var(--pa-text-3); }
        .wx-hero-now { display: flex; align-items: center; gap: 14px; }
        .wx-hero-icon { font-size: 44px; line-height: 1; }
        .wx-hero-temp { font-family: var(--pa-font-display); font-size: 40px; font-weight: 700; line-height: 1.05; }
        .wx-hero-sub { font-size: 13px; color: var(--pa-text-2); margin-top: 2px; }
        .wx-hero-text { font-size: 17px; font-weight: 650; line-height: 1.5; }
        .wx-warning { display: inline-flex; align-items: center; gap: 6px; align-self: flex-start; padding: 4px 10px; border-radius: 10px; background: var(--tone-rose-bg); color: var(--tone-rose); font-size: 13.5px; font-weight: 650; }
        .wx-plan {
          appearance: none; display: flex; align-items: center; gap: 10px; width: calc(100% - 32px); margin: 12px 16px 0; padding: 12px 14px;
          border-radius: var(--pa-radius); border: 0.5px solid var(--pa-line); background: var(--pa-surface); color: var(--pa-text); text-align: left; cursor: pointer;
        }
        .wx-plan > svg:first-child { color: var(--tone-sage); flex-shrink: 0; }
        .wx-plan span { flex: 1; min-width: 0; display: flex; flex-direction: column; }
        .wx-plan b { font-size: 15px; }
        .wx-plan small { font-size: 12.5px; color: var(--pa-text-2); }
        .wx-plan em { font-style: normal; font-size: 13px; color: var(--pa-accent); }
        .wx-plan > svg:last-child { color: var(--pa-text-3); }
        .wx-chart-card { margin: 12px 16px 0; padding: 10px 8px 8px; border-radius: var(--pa-radius); background: var(--pa-surface); border: 0.5px solid var(--pa-line); }
        .wx-chart-legend { display: flex; gap: 12px; justify-content: center; font-size: 11.5px; color: var(--pa-text-3); }
        .wx-chart-legend i { display: inline-block; width: 10px; height: 10px; border-radius: 3px; margin-right: 4px; vertical-align: -1px; }
        .wx-chart-legend i[data-kind="go"] { background: var(--tone-sage); }
        .wx-chart-legend i[data-kind="back"] { background: var(--tone-rose); }
        .wx-chart-legend i[data-kind="rain"] { background: var(--tone-sky-bg); }
        .wx-parts { display: flex; flex-direction: column; gap: 10px; padding: 0 16px; }
        .wx-brief { padding: 12px 14px; border-radius: var(--pa-radius); background: var(--pa-surface); border: 0.5px solid var(--pa-line); font-size: 14.5px; line-height: 1.6; }
        .wx-brief[data-skipped="true"] { opacity: 0.6; }
        .wx-brief-head { display: flex; justify-content: space-between; margin-bottom: 4px; }
        .wx-brief-head small { color: var(--pa-text-3); }
      `}</style>
    </div>
  );
}
