"use client";

import { useEffect, useMemo, useState } from "react";
import dayjs from "dayjs";
import { App, Button, DatePicker, Popconfirm, Select, Tooltip } from "antd";
import { ChevronLeft, ChevronRight, Copy, Sparkles } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import type { Account } from "@/lib/types";
import { amountOf, cellKey, DAY, dietApi, mealLabels, recordDate, weekdays, weekStart, type DietRecord } from "@/lib/diet";
import { useConsole } from "@/components/console";
import { Kpi, PageHeader, Panel } from "@/components/ui";
import { CellDrawer, type CellTarget } from "./cell-drawer";
import { DietAssistant } from "./assistant";
import { DailyChart, ReviewCard } from "./charts";

/** 可能是孩子的人：有生日且不到 16 岁，或者就是家里的女儿。 */
function childCandidates(persons: ReturnType<typeof useConsole>["persons"]) {
  const list = persons.filter((p) => p.personKey === "daughter" || (p.birthday && dayjs().diff(dayjs(p.birthday), "year") < 16));
  return list.length ? list : persons;
}

export default function DietPage() {
  const { persons, personName } = useConsole();
  const { message } = App.useApp();
  const children = useMemo(() => childCandidates(persons), [persons]);
  const [child, setChild] = useState(() => (children.some((p) => p.personKey === "daughter") ? "daughter" : children[0]?.personKey ?? ""));
  const [week, setWeek] = useState(() => weekStart(dayjs()));
  const [target, setTarget] = useState<CellTarget | null>(null);
  const [flash, setFlash] = useState<Set<string>>(new Set());
  const from = week.format("YYYY-MM-DD");

  const { data, loading, reload } = useData(() => dietApi.overview(child, from, 7), [child, from]);
  const { data: accounts } = useData(
    () => Promise.all(["app", "manager"].map((realm) => api.get<Account[]>(`/accounts?realm=${realm}`).catch(() => [] as Account[]))).then((lists) => lists.flat()),
    [],
    { silent: true },
  );
  const accountName = (key: string) => {
    const account = accounts?.find((a) => a.accountKey === key);
    return account ? account.displayName || account.username : "";
  };

  // 改过的格子亮一下
  useEffect(() => {
    if (!flash.size) return;
    const timer = window.setTimeout(() => setFlash(new Set()), 2600);
    return () => window.clearTimeout(timer);
  }, [flash]);

  const days = useMemo(() => Array.from({ length: 7 }, (_, i) => week.add(i, "day")), [week]);
  const rows = useMemo(() => [{ key: DAY, label: "全天要求", clock: "" }, ...(data?.meals ?? [])], [data]);
  const plans = useMemo(() => new Map((data?.plans ?? []).map((p) => [cellKey(p.date, p.meal), p])), [data]);
  const records = useMemo(() => {
    const map = new Map<string, DietRecord[]>();
    for (const record of data?.records ?? []) {
      const key = cellKey(recordDate(record), record.meal);
      map.set(key, [...(map.get(key) ?? []), record]);
    }
    return map;
  }, [data]);

  const stats = useMemo(() => {
    const mealPlans = (data?.plans ?? []).filter((p) => p.meal !== DAY);
    const followed = mealPlans.filter((p) => records.has(cellKey(p.date, p.meal))).length;
    const rated = (data?.records ?? []).filter((r) => amountOf(r.amount));
    const good = rated.filter((r) => r.amount === "all" || r.amount === "most").length;
    const pastPlans = mealPlans.filter((p) => !dayjs(p.date).isAfter(dayjs(), "day"));
    const pastFollowed = pastPlans.filter((p) => records.has(cellKey(p.date, p.meal))).length;
    return {
      planned: mealPlans.length,
      followed,
      recorded: data?.records.length ?? 0,
      coverage: pastPlans.length ? Math.round((pastFollowed / pastPlans.length) * 100) : null,
      good: rated.length ? Math.round((good / rated.length) * 100) : null,
    };
  }, [data, records]);

  const today = dayjs().format("YYYY-MM-DD");
  const isThisWeek = week.isSame(weekStart(dayjs()), "day");

  const copyLastWeek = async () => {
    try {
      const { count } = await dietApi.copyPlans({ childKey: child, from: week.subtract(7, "day").format("YYYY-MM-DD"), to: from, days: 7 });
      message.success(count ? `抄过来 ${count} 餐` : "上周没有计划");
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  const changed = (keys: string[]) => {
    setFlash(new Set(keys));
    void reload();
  };

  return (
    <>
      <PageHeader
        eyebrow="家庭"
        title="宝宝饮食"
        description="爸爸妈妈排的计划和实际吃的放在一起对照。点格子手动改；右边说一句话（或拍张食谱）交给模型改计划、记实际。"
        actions={
          <>
            {children.length > 1 && (
              <Select value={child} onChange={setChild} style={{ width: 140 }} options={children.map((p) => ({ value: p.personKey, label: personName(p.personKey) }))} />
            )}
            <div className="diet-weeknav">
              <Button type="text" size="small" icon={<ChevronLeft size={16} />} onClick={() => setWeek(week.subtract(7, "day"))} aria-label="上一周" />
              <DatePicker
                picker="week"
                variant="borderless"
                allowClear={false}
                value={week}
                onChange={(value) => value && setWeek(weekStart(value))}
                format={() => `${week.format("M月D日")} – ${week.add(6, "day").format("M月D日")}`}
                style={{ width: 160 }}
              />
              <Button type="text" size="small" icon={<ChevronRight size={16} />} onClick={() => setWeek(week.add(7, "day"))} aria-label="下一周" />
            </div>
            {!isThisWeek && <Button onClick={() => setWeek(weekStart(dayjs()))}>回到本周</Button>}
            <Popconfirm title="把上一周的计划抄到这周？" description="这周已有计划的餐会被覆盖。" onConfirm={copyLastWeek} okText="抄过来">
              <Button icon={<Copy size={15} />} className="pa-icon-text">抄上周计划</Button>
            </Popconfirm>
          </>
        }
      />

      <div className="pa-kpis">
        <Kpi label="这周计划的餐" value={stats.planned} />
        <Kpi label="实际记录" value={stats.recorded} tone="sky" />
        <Kpi label="计划的餐有记录（到今天）" value={stats.coverage === null ? "—" : `${stats.coverage}%`} tone="sage" />
        <Kpi label="吃完 / 吃了大半" value={stats.good === null ? "—" : `${stats.good}%`} tone="clay" />
      </div>

      <div className="diet-layout">
        <div style={{ minWidth: 0 }}>
          <Panel
            title={<>一周对照 <span className="pa-muted" style={{ fontWeight: 400, fontSize: 12.5 }}>上面是计划，下面是实际</span></>}
            extra={
              <div className="diet-legend">
                <span><i className="diet-legend-plan" />计划</span>
                <span><i className="diet-legend-record" />实际</span>
                <span><i className="diet-legend-miss" />排了没记</span>
              </div>
            }
          >
            <div className="diet-board-wrap">
              <div className="diet-board" data-loading={loading}>
                <div className="diet-corner" />
                {days.map((day) => {
                  const date = day.format("YYYY-MM-DD");
                  return (
                    <div key={date} className="diet-dayhead" data-today={date === today}>
                      <span>周{weekdays[day.day()]}</span>
                      <b className="pa-num">{day.format("M/D")}</b>
                    </div>
                  );
                })}
                {rows.map((row) => (
                  <Row
                    key={row.key}
                    meal={row.key}
                    label={row.key === DAY ? row.label : mealLabels[row.key] ?? row.label}
                    clock={row.clock}
                    days={days}
                    today={today}
                    plans={plans}
                    records={records}
                    flash={flash}
                    onOpen={setTarget}
                  />
                ))}
              </div>
            </div>
          </Panel>

          <Panel title="每天吃了多少" style={{ marginTop: 16 }}>
            <DailyChart days={days} plans={data?.plans ?? []} records={data?.records ?? []} />
          </Panel>

          <ReviewCard child={child} weekStart={from} />
        </div>

        <DietAssistant child={child} from={from} onChanged={changed} />
      </div>

      <CellDrawer
        target={target}
        child={child}
        meals={data?.meals ?? []}
        plan={target ? plans.get(cellKey(target.date, target.meal)) : undefined}
        records={target ? records.get(cellKey(target.date, target.meal)) ?? [] : []}
        accountName={accountName}
        onClose={() => setTarget(null)}
        onChanged={(keys) => changed(keys)}
      />

      <style jsx global>{`
        .diet-weeknav { display: inline-flex; align-items: center; background: var(--pa-surface); border: 1px solid var(--pa-line); border-radius: 8px; padding: 0 2px; }
        .diet-weeknav .ant-picker-input > input { text-align: center; font-weight: 550; cursor: pointer; }
        .diet-layout { display: grid; grid-template-columns: minmax(0, 1fr) 360px; gap: 16px; align-items: start; }
        .diet-layout > .pa-panel + .pa-panel { margin-top: 0; }
        @media (max-width: 1240px) { .diet-layout { grid-template-columns: minmax(0, 1fr); } }

        .diet-legend { display: flex; gap: 14px; font-size: 12.5px; color: var(--pa-text-2); }
        .diet-legend span { display: inline-flex; align-items: center; gap: 6px; }
        .diet-legend i { width: 12px; height: 12px; border-radius: 3px; display: inline-block; }
        .diet-legend-plan { border: 1.5px dashed var(--tone-amber); background: var(--tone-amber-bg); }
        .diet-legend-record { background: var(--tone-sage-bg); border: 1.5px solid var(--tone-sage); }
        .diet-legend-miss { background: repeating-linear-gradient(135deg, var(--pa-fill-strong) 0 3px, transparent 3px 6px); border: 1px solid var(--pa-line-strong); }

        .diet-board-wrap { overflow-x: auto; }
        .diet-board { display: grid; grid-template-columns: 76px repeat(7, minmax(116px, 1fr)); min-width: 900px; transition: opacity 0.2s; }
        .diet-board[data-loading="true"] { opacity: 0.55; }
        .diet-corner, .diet-dayhead { position: sticky; top: 0; background: var(--pa-surface); z-index: 1; border-bottom: 1px solid var(--pa-line); }
        .diet-dayhead { padding: 10px 10px 8px; display: flex; flex-direction: column; gap: 1px; font-size: 12px; color: var(--pa-text-3); border-left: 1px solid var(--pa-line); }
        .diet-dayhead b { font-size: 15px; color: var(--pa-text); font-weight: 650; }
        .diet-dayhead[data-today="true"] { color: var(--pa-accent); }
        .diet-dayhead[data-today="true"] b { color: var(--pa-accent); }
        .diet-meal { padding: 10px 8px 10px 16px; border-bottom: 1px solid var(--pa-line); font-size: 13px; font-weight: 600; color: var(--pa-text-2); }
        .diet-meal small { display: block; font-weight: 400; color: var(--pa-text-3); font-size: 11.5px; margin-top: 2px; }
        .diet-cell {
          border-left: 1px solid var(--pa-line); border-bottom: 1px solid var(--pa-line); padding: 6px; min-height: 76px;
          display: flex; flex-direction: column; gap: 5px; cursor: pointer; position: relative; transition: background 0.15s;
        }
        .diet-cell:hover { background: var(--pa-fill); }
        .diet-cell[data-today="true"] { background: color-mix(in srgb, var(--pa-accent-soft) 35%, transparent); }
        .diet-cell[data-today="true"]:hover { background: color-mix(in srgb, var(--pa-accent-soft) 60%, transparent); }
        .diet-cell[data-flash="true"] { animation: diet-flash 2.4s var(--pa-ease); }
        @keyframes diet-flash { 0%, 30% { box-shadow: inset 0 0 0 2px var(--pa-accent); background: var(--pa-accent-soft); } 100% { box-shadow: inset 0 0 0 2px transparent; } }
        .diet-cell-add { margin: auto; color: var(--pa-text-3); font-size: 18px; opacity: 0; transition: opacity 0.15s; }
        .diet-cell:hover .diet-cell-add { opacity: 1; }
        .diet-row-day .diet-cell { min-height: 48px; }
        .diet-plan, .diet-rec { border-radius: 7px; padding: 5px 7px; font-size: 12.5px; line-height: 1.45; word-break: break-word; }
        .diet-plan { background: var(--tone-amber-bg); border: 1.5px dashed color-mix(in srgb, var(--tone-amber) 55%, transparent); color: var(--pa-text); }
        .diet-plan[data-miss="true"] { background: repeating-linear-gradient(135deg, var(--pa-fill-strong) 0 3px, transparent 3px 6px), var(--tone-amber-bg); }
        .diet-plan-note { color: var(--pa-text-3); font-size: 11.5px; margin-top: 2px; }
        .diet-rec { background: var(--tone-sage-bg); border-left: 3px solid var(--tone-sage); }
        .diet-rec[data-amount="none"] { background: var(--tone-rose-bg); border-left-color: var(--tone-rose); }
        .diet-rec-meta { display: flex; align-items: center; gap: 6px; font-size: 11px; color: var(--pa-text-3); margin-top: 3px; }
        .diet-meter { display: inline-flex; gap: 2px; }
        .diet-meter i { width: 6px; height: 6px; border-radius: 2px; background: var(--pa-fill-strong); }
        .diet-meter i[data-on="true"] { background: var(--tone-sage); }
        .diet-clamp { display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
      `}</style>
    </>
  );
}

function Row({ meal, label, clock, days, today, plans, records, flash, onOpen }: {
  meal: string;
  label: string;
  clock: string;
  days: dayjs.Dayjs[];
  today: string;
  plans: Map<string, { foods: string; note: string }>;
  records: Map<string, DietRecord[]>;
  flash: Set<string>;
  onOpen: (target: CellTarget) => void;
}) {
  return (
    <div style={{ display: "contents" }} className={meal === DAY ? "diet-row-day" : undefined}>
      <div className="diet-meal">
        {label}
        {clock && <small className="pa-num">{clock}</small>}
      </div>
      {days.map((day) => {
        const date = day.format("YYYY-MM-DD");
        const key = cellKey(date, meal);
        const plan = plans.get(key);
        const list = records.get(key) ?? [];
        const miss = !!plan && meal !== DAY && list.length === 0 && date < today;
        return (
          <div key={key} className="diet-cell" data-today={date === today} data-flash={flash.has(key)} onClick={() => onOpen({ date, meal })}>
            {plan && (
              <Tooltip title={miss ? "排了计划，但没有记录" : undefined}>
                <div className="diet-plan" data-miss={miss}>
                  {plan.foods && <div className="diet-clamp">{plan.foods}</div>}
                  {plan.note && <div className="diet-plan-note">{plan.note}</div>}
                </div>
              </Tooltip>
            )}
            {list.map((record) => (
              <RecordChip key={record.recordKey} record={record} />
            ))}
            {!plan && list.length === 0 && <span className="diet-cell-add">+</span>}
          </div>
        );
      })}
    </div>
  );
}

function RecordChip({ record }: { record: DietRecord }) {
  const amount = amountOf(record.amount);
  const filled = amount ? Math.round(amount.weight * 4) : 0;
  return (
    <div className="diet-rec" data-amount={record.amount}>
      <div className="diet-clamp">{record.foods || record.note}</div>
      <div className="diet-rec-meta">
        <span className="pa-num">{dayjs(record.eatenAt).format("HH:mm")}</span>
        {amount && (
          <>
            <span className="diet-meter" aria-hidden>
              {[0, 1, 2, 3].map((i) => <i key={i} data-on={i < filled} />)}
            </span>
            <span>{amount.short}</span>
          </>
        )}
        {record.photoCount > 0 && <span>· {record.photoCount} 张照片</span>}
        {record.source !== "manual" && <Sparkles size={11} aria-label="模型整理" />}
      </div>
    </div>
  );
}
