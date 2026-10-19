"use client";

import { useState } from "react";
import dayjs, { type Dayjs } from "dayjs";
import { App, Button, Empty, Spin } from "antd";
import { RefreshCw } from "lucide-react";
import { useData } from "@/lib/hooks";
import { amounts, DAY, dietApi, recordDate, weekdays, type DietPlan, type DietRecord } from "@/lib/diet";
import { Panel } from "@/components/ui";

// 吃了多少：同一个绿色由深到浅，没吃用玫红，不清楚用灰
const segments = [
  ...amounts.map((a) => ({
    key: a.value,
    label: a.label,
    color: a.value === "none" ? "var(--tone-rose)" : `color-mix(in srgb, var(--tone-sage) ${Math.round(30 + a.weight * 70)}%, var(--pa-surface))`,
  })),
  { key: "", label: "没说吃了多少", color: "var(--pa-line-strong)" },
];

const BAR_MAX_HEIGHT = 150;

/** 每天一根柱子：虚线框是计划了几餐，里面实心的是实际记了几餐，按吃了多少分段。 */
export function DailyChart({ days, plans, records }: { days: Dayjs[]; plans: DietPlan[]; records: DietRecord[] }) {
  const [hover, setHover] = useState<string | null>(null);
  const today = dayjs().format("YYYY-MM-DD");
  const columns = days.map((day) => {
    const date = day.format("YYYY-MM-DD");
    const dayRecords = records.filter((r) => recordDate(r) === date);
    const planned = plans.filter((p) => p.date === date && p.meal !== DAY).length;
    const counts = segments.map((s) => ({ ...s, count: dayRecords.filter((r) => (amounts.some((a) => a.value === r.amount) ? r.amount : "") === s.key).length }));
    return { date, day, planned, recorded: dayRecords.length, counts };
  });
  const max = Math.max(4, ...columns.map((c) => Math.max(c.planned, c.recorded)));
  const unit = BAR_MAX_HEIGHT / max;
  const ticks = Array.from({ length: Math.floor(max / 2) + 1 }, (_, i) => i * 2);
  const empty = columns.every((c) => !c.planned && !c.recorded);

  return (
    <div style={{ padding: "16px 20px 18px" }}>
      <div className="diet-chart-legend">
        <span><i style={{ border: "1.5px dashed var(--tone-amber)" }} />计划的餐数</span>
        {segments.map((s) => <span key={s.key}><i style={{ background: s.color }} />{s.label}</span>)}
      </div>
      {empty ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="这周还没有计划和记录" />
      ) : (
        <div className="diet-chart">
          <div className="diet-chart-axis">
            {ticks.map((tick) => (
              <span key={tick} style={{ bottom: tick * unit }} className="pa-num">{tick}</span>
            ))}
          </div>
          <div className="diet-chart-plot" style={{ height: BAR_MAX_HEIGHT }}>
            {ticks.map((tick) => <div key={tick} className="diet-chart-grid" style={{ bottom: tick * unit }} />)}
            {columns.map((column) => (
              <div key={column.date} className="diet-chart-col" onMouseEnter={() => setHover(column.date)} onMouseLeave={() => setHover(null)} data-active={hover === column.date}>
                <div className="diet-chart-bars">
                  {column.planned > 0 && <div className="diet-chart-plan" style={{ height: column.planned * unit }} />}
                  <div className="diet-chart-stack">
                    {column.counts.filter((s) => s.count).map((s) => (
                      <div key={s.key} style={{ height: s.count * unit - 2, background: s.color }} />
                    ))}
                  </div>
                </div>
                {hover === column.date && (
                  <div className="diet-chart-tip">
                    <b>{column.day.format("M月D日")} 周{weekdays[column.day.day()]}</b>
                    <div>计划 {column.planned} 餐 · 记了 {column.recorded} 条</div>
                    {column.counts.filter((s) => s.count).map((s) => (
                      <div key={s.key} className="diet-chart-tip-row"><i style={{ background: s.color }} />{s.label}<span className="pa-num">{s.count}</span></div>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
          <div />
          <div className="diet-chart-labels">
            {columns.map((column) => (
              <span key={column.date} data-today={column.date === today}>
                周{weekdays[column.day.day()]}
                <small className="pa-num">{column.recorded}/{column.planned}</small>
              </span>
            ))}
          </div>
        </div>
      )}
      <style jsx global>{`
        .diet-chart-legend { display: flex; flex-wrap: wrap; gap: 6px 14px; font-size: 12px; color: var(--pa-text-2); margin-bottom: 14px; }
        .diet-chart-legend span { display: inline-flex; align-items: center; gap: 6px; }
        .diet-chart-legend i { width: 11px; height: 11px; border-radius: 3px; display: inline-block; }
        .diet-chart { display: grid; grid-template-columns: 24px 1fr; column-gap: 8px; }
        .diet-chart-axis { position: relative; font-size: 11px; color: var(--pa-text-3); }
        .diet-chart-axis span { position: absolute; right: 0; transform: translateY(50%); }
        .diet-chart-plot { position: relative; display: grid; grid-template-columns: repeat(7, 1fr); border-bottom: 1px solid var(--pa-line-strong); }
        .diet-chart-grid { position: absolute; left: 0; right: 0; height: 0; border-top: 1px solid var(--pa-line); }
        .diet-chart-col { position: relative; display: flex; justify-content: center; align-items: flex-end; height: 100%; cursor: default; }
        .diet-chart-col[data-active="true"] { background: var(--pa-fill); }
        .diet-chart-bars { position: relative; width: 34px; height: 100%; display: flex; align-items: flex-end; justify-content: center; }
        .diet-chart-plan { position: absolute; bottom: 0; left: -5px; right: -5px; border: 1.5px dashed var(--tone-amber); border-bottom: none; border-radius: 6px 6px 0 0; }
        .diet-chart-stack { width: 22px; display: flex; flex-direction: column-reverse; gap: 2px; position: relative; }
        .diet-chart-stack > div:last-child { border-radius: 4px 4px 0 0; }
        .diet-chart-tip {
          position: absolute; bottom: calc(100% + 6px); left: 50%; transform: translateX(-50%); z-index: 5; min-width: 150px;
          background: var(--pa-elevated); border: 1px solid var(--pa-line-strong); border-radius: 10px; box-shadow: var(--pa-shadow);
          padding: 9px 11px; font-size: 12px; color: var(--pa-text-2); pointer-events: none; white-space: nowrap;
        }
        .diet-chart-tip b { color: var(--pa-text); display: block; margin-bottom: 3px; }
        .diet-chart-tip-row { display: flex; align-items: center; gap: 6px; margin-top: 3px; }
        .diet-chart-tip-row i { width: 9px; height: 9px; border-radius: 2px; }
        .diet-chart-tip-row span { margin-left: auto; padding-left: 12px; color: var(--pa-text); font-weight: 600; }
        .diet-chart-labels { display: grid; grid-template-columns: repeat(7, 1fr); text-align: center; font-size: 12px; color: var(--pa-text-2); padding-top: 6px; }
        .diet-chart-labels span[data-today="true"] { color: var(--pa-accent); font-weight: 600; }
        .diet-chart-labels small { display: block; color: var(--pa-text-3); font-size: 11px; }
      `}</style>
    </div>
  );
}

/** 这一周的对照点评（每周日晚上自动写，也可以现在生成）。 */
export function ReviewCard({ child, weekStart }: { child: string; weekStart: string }) {
  const { message } = App.useApp();
  const { data, loading, setData } = useData(() => dietApi.review(child, weekStart), [child, weekStart], { silent: true });
  const [running, setRunning] = useState(false);
  const review = data?.review;
  const generate = async () => {
    setRunning(true);
    try {
      const result = await dietApi.createReview(child, weekStart);
      setData({ review: result });
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setRunning(false);
    }
  };
  return (
    <Panel
      style={{ marginTop: 16 }}
      title={<>这周对照 {review && <span className="pa-muted" style={{ fontWeight: 400, fontSize: 12.5 }}>{dayjs(review.updatedAt).format("M月D日 HH:mm")} 写的</span>}</>}
      extra={
        <Button size="small" icon={<RefreshCw size={14} />} className="pa-icon-text" loading={running} onClick={generate}>
          {review ? "重新对照" : "现在对照一下"}
        </Button>
      }
    >
      <div className="pa-panel-body">
        {loading ? (
          <Spin />
        ) : running ? (
          <div className="pa-muted">模型正在看这周的计划和实际…</div>
        ) : review ? (
          <div className="pa-pre" style={{ lineHeight: 1.85, fontSize: 14 }}>{review.content}</div>
        ) : (
          <div className="pa-muted">这周还没有对照。每周日晚上 8 点后会自动写（这周记录少于 3 条不写），也可以现在生成。</div>
        )}
      </div>
    </Panel>
  );
}
