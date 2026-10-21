"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import dayjs from "dayjs";
import { Toast } from "antd-mobile";
import { ChevronRight, MapPin, Search } from "lucide-react";
import { api } from "@/lib/api";
import { clothIcon, deg, hm, keyPart, resetDay, saveDay, weatherIcon, weatherToday, type DayPlan, type OutfitPart, type WeatherHour, type WeatherLocation, type WeatherToday } from "@/lib/weather";
import { Button, Segmented, Sheet } from "./ui";

/**
 * 今天的逐小时体感曲线：标出送、接、最晚到家、天黑，下雨的小时打底色。
 * 只画 6 点到 21 点，带孩子出门就在这段。
 */
export function HourlyChart({ hours, marks, sunset, big }: {
  hours: WeatherHour[];
  marks: { at: string; label: string; kind: "go" | "back" | "dark" }[];
  sunset?: string;
  big?: boolean;
}) {
  const day = hours[0] ? dayjs(hours[0].at).format("YYYY-MM-DD") : dayjs().format("YYYY-MM-DD");
  const start = dayjs(`${day} 06:00`);
  const end = dayjs(`${day} 21:00`);
  const shown = hours.filter((hour) => !dayjs(hour.at).isBefore(start) && !dayjs(hour.at).isAfter(end));
  const width = 340;
  const height = big ? 180 : 150;
  const pad = { left: 8, right: 8, top: 40, bottom: 30 };
  const values = shown.map((hour) => hour.feels);
  const min = Math.floor(Math.min(...values, 99) - 2);
  const max = Math.ceil(Math.max(...values, -99) + 2);
  const span = end.diff(start, "minute");
  const x = (at: dayjs.Dayjs) => pad.left + ((at.diff(start, "minute")) / span) * (width - pad.left - pad.right);
  const y = (value: number) => pad.top + (1 - (value - min) / Math.max(max - min, 1)) * (height - pad.top - pad.bottom);
  if (shown.length < 2) return null;
  const path = shown.map((hour, i) => `${i ? "L" : "M"}${x(dayjs(hour.at)).toFixed(1)},${y(hour.feels).toFixed(1)}`).join(" ");
  const area = `${path} L${x(dayjs(shown[shown.length - 1].at)).toFixed(1)},${height - pad.bottom} L${x(dayjs(shown[0].at)).toFixed(1)},${height - pad.bottom} Z`;
  const now = dayjs();
  const allMarks = [...marks];
  if (sunset) allMarks.push({ at: `${day} ${sunset}`, label: "天黑", kind: "dark" });
  const labelled = shown.filter((_, i) => i % 3 === 0);

  return (
    <svg className="wx-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="今天每小时的体感温度">
      <defs>
        <linearGradient id="wx-fill" x1="0" x2="0" y1="0" y2="1">
          <stop offset="0%" stopColor="var(--tone-amber)" stopOpacity="0.28" />
          <stop offset="100%" stopColor="var(--tone-amber)" stopOpacity="0" />
        </linearGradient>
      </defs>
      {shown.filter((hour) => hour.wet).map((hour) => (
        <rect key={hour.at} x={x(dayjs(hour.at))} y={pad.top - 6} width={(width - pad.left - pad.right) / (span / 60)} height={height - pad.top - pad.bottom + 6}
          fill="var(--tone-sky)" opacity="0.14" />
      ))}
      <path d={area} fill="url(#wx-fill)" />
      <path d={path} fill="none" stroke="var(--tone-amber)" strokeWidth="2.2" strokeLinejoin="round" strokeLinecap="round" />
      {labelled.map((hour) => (
        <g key={hour.at}>
          <circle cx={x(dayjs(hour.at))} cy={y(hour.feels)} r="2.6" fill="var(--tone-amber)" />
          <text x={x(dayjs(hour.at))} y={y(hour.feels) - 7} textAnchor="middle" className="wx-chart-val">{deg(hour.feels)}</text>
          <text x={x(dayjs(hour.at))} y={height - 10} textAnchor="middle" className="wx-chart-axis">{dayjs(hour.at).format("H")}点</text>
        </g>
      ))}
      {placeLabels(allMarks.map((mark) => ({ ...mark, px: x(dayjs(mark.at)) })).filter((mark) => mark.px >= pad.left && mark.px <= width - pad.right)).map((mark) => {
        const color = mark.kind === "back" ? "var(--tone-rose)" : mark.kind === "dark" ? "var(--pa-text-3)" : "var(--tone-sage)";
        return (
          <g key={mark.label + mark.at}>
            <line x1={mark.px} x2={mark.px} y1={pad.top - 10 - mark.row * 12} y2={height - pad.bottom} stroke={color} strokeWidth={mark.kind === "back" ? 2 : 1.2} strokeDasharray={mark.kind === "back" ? undefined : "3 3"} />
            <text x={mark.px} y={pad.top - 13 - mark.row * 12} textAnchor="middle" className="wx-chart-mark" fill={color}>{mark.label}</text>
          </g>
        );
      })}
      {!now.isBefore(start) && !now.isAfter(end) && dayjs(day).isSame(now, "day") && (
        <circle cx={x(now)} cy={height - pad.bottom} r="3.5" fill="var(--pa-accent)" />
      )}
    </svg>
  );
}

/** 竖线上的字挨得太近就往上错一行，最多三行。 */
function placeLabels<T extends { px: number; label: string }>(marks: T[]): (T & { row: number })[] {
  const sorted = [...marks].sort((a, b) => a.px - b.px);
  const lastEnd = [-Infinity, -Infinity, -Infinity];
  return sorted.map((mark) => {
    const half = mark.label.length * 5.5 + 3;
    let row = lastEnd.findIndex((end) => mark.px - half > end);
    if (row < 0) row = 2;
    lastEnd[row] = mark.px + half;
    return { ...mark, row };
  });
}

/** 图上的竖线：送、接、最晚到家。 */
export function chartMarks(parts: OutfitPart[]): { at: string; label: string; kind: "go" | "back" | "dark" }[] {
  const marks: { at: string; label: string; kind: "go" | "back" | "dark" }[] = [];
  for (const part of parts) {
    if (part.skip) continue;
    marks.push({ at: part.start, label: part.kind === "drop" ? "送" : part.kind === "pickup" ? "接" : "出门", kind: "go" });
    if (part.backBy && part.kind !== "drop") marks.push({ at: part.backBy, label: `${hm(part.backBy)} 到家`, kind: "back" });
  }
  return marks;
}

/** 一段出门：穿什么、带什么、几点前到家。 */
export function PartCard({ part, big }: { part: OutfitPart; big?: boolean }) {
  return (
    <section className="wx-part" data-big={big} data-alert={part.directHome || part.skip}>
      <div className="wx-part-head">
        <b>{part.label}</b>
        <span className="pa-num">{hm(part.start)} · 体感 {deg(part.feelsMin)}{part.feelsMax - part.feelsMin >= 2 ? `–${deg(part.feelsMax)}` : ""}</span>
      </div>
      {part.skip ? (
        <div className="wx-part-back" data-kind="skip">{part.skipReason}</div>
      ) : (
        <>
          <div className="wx-clothes">
            {part.clothes.map((cloth) => <span key={cloth}><i>{clothIcon(cloth)}</i>{cloth}</span>)}
          </div>
          {part.extras.length > 0 && (
            <div className="wx-extras">带上：{part.extras.join("、")}</div>
          )}
          {part.backBy && part.kind !== "drop" && (
            <div className="wx-part-back" data-kind={part.directHome ? "direct" : part.backReason ? "early" : "ok"}>
              <span>{part.directHome ? "接到直接回，" : ""}<b className="pa-num">{hm(part.backBy)}</b> 前到家</span>
              {part.backReason && <small>{part.backReason}</small>}
              {part.playUntil && <small>天气好，想多玩可以到 {hm(part.playUntil)}</small>}
            </div>
          )}
          {part.notes.length > 0 && <div className="wx-notes">{part.notes.join("；")}</div>}
          {part.estimated && <div className="wx-notes">（离现在远，没有逐小时预报，按全天温度估的）</div>}
        </>
      )}
    </section>
  );
}

const pickupOptions = ["15:30", "15:50", "16:10", "16:30", "17:00"];

/** 改某一天的送接：去不去幼儿园、几点接、几点前到家、要不要出去。只改这一天。 */
export function DayPlanSheet({ plan, open, onClose, onSaved, big }: { plan: DayPlan | null; open: boolean; onClose: () => void; onSaved: (plan: DayPlan) => void; big?: boolean }) {
  const [school, setSchool] = useState("yes");
  const [drop, setDrop] = useState("08:00");
  const [pickup, setPickup] = useState("15:50");
  const [home, setHome] = useState("16:30");
  const [outFrom, setOutFrom] = useState("");
  const [outTo, setOutTo] = useState("");
  const [outNote, setOutNote] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open || !plan) return;
    setSchool(plan.school ? "yes" : "no");
    setDrop(plan.drop || "08:00");
    setPickup(plan.pickup || "15:50");
    setHome(plan.home || "16:30");
    const first = plan.outings[0];
    setOutFrom(first?.from ?? "");
    setOutTo(first?.to ?? "");
    setOutNote(first?.note ?? "");
  }, [open, plan]);

  const save = async () => {
    if (!plan) return;
    if ((outFrom && !outTo) || (!outFrom && outTo) || (outFrom && outTo && outTo <= outFrom)) {
      Toast.show({ content: "出去的时间要填开始和结束，结束晚于开始" });
      return;
    }
    setBusy(true);
    try {
      const outings = outFrom && outTo ? [{ from: outFrom, to: outTo, note: outNote.trim() }] : school === "yes" ? [] : plan.outings;
      const saved = await saveDay({
        date: plan.date, noSchool: school === "no",
        drop: school === "yes" ? drop : "", pickup: school === "yes" ? pickup : "", home: school === "yes" ? home : "",
        outings,
      });
      Toast.show({ content: "改好了，提醒会按新的安排算" });
      onSaved(saved);
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setBusy(false);
    }
  };

  const reset = async () => {
    if (!plan) return;
    setBusy(true);
    try {
      const saved = await resetDay(plan.date);
      Toast.show({ content: "恢复成平常的安排了" });
      onSaved(saved);
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setBusy(false);
    }
  };

  const label = plan ? (dayjs(plan.date).isSame(dayjs(), "day") ? "今天" : dayjs(plan.date).format("M月D日")) : "";
  return (
    <Sheet open={open} onClose={onClose} title={`${label}的安排`}>
      <div className="pa-field" style={{ marginTop: 8 }}>
        <div className="pa-label">去不去幼儿园</div>
        <Segmented options={[{ value: "yes", label: "去" }, { value: "no", label: "不去" }]} value={school} onChange={setSchool} />
      </div>
      {school === "yes" && (
        <>
          <div className="pa-field">
            <div className="pa-label">几点接</div>
            <div className="pa-chips">
              {pickupOptions.map((value) => (
                <button key={value} type="button" className="pa-chip" data-selected={pickup === value} onClick={() => setPickup(value)}>{value}</button>
              ))}
              <input className="pa-input wx-time" type="time" value={pickup} onChange={(event) => setPickup(event.target.value)} />
            </div>
          </div>
          <div className="pa-field wx-times">
            <label>
              <span className="pa-label">几点送</span>
              <input className="pa-input wx-time" type="time" value={drop} onChange={(event) => setDrop(event.target.value)} />
            </label>
            <label>
              <span className="pa-label">平常几点前到家</span>
              <input className="pa-input wx-time" type="time" value={home} onChange={(event) => setHome(event.target.value)} />
            </label>
          </div>
        </>
      )}
      <div className="pa-field">
        <div className="pa-label">{school === "yes" ? "接完还要去别的地方？" : "要出去吗？"}<small>（不填就按{school === "yes" ? "接完回家" : "平常周末的时间"}算）</small></div>
        <div className="wx-times">
          <input className="pa-input wx-time" type="time" value={outFrom} onChange={(event) => setOutFrom(event.target.value)} aria-label="出门时间" />
          <span>到</span>
          <input className="pa-input wx-time" type="time" value={outTo} onChange={(event) => setOutTo(event.target.value)} aria-label="回来时间" />
        </div>
        <input className="pa-input" style={{ marginTop: 8 }} value={outNote} onChange={(event) => setOutNote(event.target.value)} placeholder="去哪，比如：去外婆家" maxLength={30} />
      </div>
      <div className="pa-field" style={{ display: "flex", gap: 10 }}>
        {plan?.override && <Button variant="soft" onClick={() => void reset()} disabled={busy}>恢复平常</Button>}
        <Button variant="accent" block loading={busy} onClick={() => void save()} style={big ? { minHeight: 56, fontSize: 20 } : undefined}>保存</Button>
      </div>
    </Sheet>
  );
}

/** 宝宝现在在哪：搜地名（镇一级也行），选一个就按那边的天气提醒。 */
export function LocationSheet({ open, current, onClose, onSaved }: { open: boolean; current?: WeatherLocation; onClose: () => void; onSaved: () => void }) {
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<WeatherLocation[]>([]);
  const [searching, setSearching] = useState(false);
  const [saving, setSaving] = useState("");

  const search = async () => {
    if (!query.trim()) return;
    setSearching(true);
    try {
      const result = await api.get<{ items: WeatherLocation[] }>(`/weather/locations?q=${encodeURIComponent(query.trim())}`);
      setItems(result.items);
      if (!result.items.length) Toast.show({ content: "没搜到，换个说法试试（比如区县名）" });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setSearching(false);
    }
  };

  const choose = async (location: WeatherLocation) => {
    setSaving(location.code);
    try {
      await api.put("/weather/location", location);
      Toast.show({ content: `以后按${location.name}的天气提醒` });
      onSaved();
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setSaving("");
    }
  };

  return (
    <Sheet open={open} onClose={onClose} title="宝宝现在在哪">
      <div className="pa-field" style={{ marginTop: 6 }}>
        {current && <div className="pa-field-hint" style={{ marginTop: 0, marginBottom: 10 }}><MapPin size={13} style={{ verticalAlign: -2 }} /> 现在按 <b>{current.name}</b> 的天气</div>}
        <div style={{ display: "flex", gap: 8 }}>
          <input className="pa-input" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="输地名，比如：青浦、赵巷、老家的县名"
            onKeyDown={(event) => { if (event.key === "Enter") void search(); }} />
          <Button variant="soft" loading={searching} onClick={() => void search()}><Search size={16} /></Button>
        </div>
      </div>
      <div style={{ padding: "10px 20px 0", display: "flex", flexDirection: "column", gap: 8 }}>
        {items.map((item) => (
          <button key={item.code} className="wx-loc" onClick={() => void choose(item)} disabled={!!saving}>
            <span>{item.name}</span>
            {saving === item.code ? <span className="pa-btn-spin" /> : <small className="pa-num">{item.code}</small>}
          </button>
        ))}
      </div>
    </Sheet>
  );
}

/** 页面共用的样式。 */
export function WeatherStyles() {
  return (
    <style jsx global>{`
      .wx-chart { width: 100%; height: auto; display: block; }
      .wx-chart-val { font-size: 10.5px; fill: var(--pa-text-2); font-variant-numeric: tabular-nums; }
      .wx-chart-axis { font-size: 10px; fill: var(--pa-text-3); }
      .wx-chart-mark { font-size: 10.5px; font-weight: 700; }
      .wx-part { padding: 12px 14px; border-radius: var(--pa-radius); background: var(--pa-surface); border: 0.5px solid var(--pa-line); box-shadow: var(--pa-shadow-sm); display: flex; flex-direction: column; gap: 8px; }
      .wx-part[data-alert="true"] { border-color: color-mix(in srgb, var(--tone-rose) 45%, transparent); }
      .wx-part-head { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
      .wx-part-head b { font-size: 16.5px; font-weight: 700; }
      .wx-part-head span { font-size: 12.5px; color: var(--pa-text-3); }
      .wx-clothes { display: flex; flex-wrap: wrap; gap: 6px; }
      .wx-clothes span { display: inline-flex; align-items: center; gap: 4px; padding: 4px 10px; border-radius: 12px; background: var(--tone-amber-bg); color: var(--pa-text); font-size: 14px; }
      .wx-clothes i { font-style: normal; }
      .wx-extras { font-size: 13.5px; color: var(--pa-text-2); }
      .wx-part-back { padding: 8px 10px; border-radius: 12px; background: var(--tone-sage-bg); font-size: 14.5px; display: flex; flex-direction: column; gap: 2px; }
      .wx-part-back b { font-size: 17px; }
      .wx-part-back small { font-size: 12.5px; color: var(--pa-text-2); }
      .wx-part-back[data-kind="direct"], .wx-part-back[data-kind="early"], .wx-part-back[data-kind="skip"] { background: var(--tone-rose-bg); color: var(--pa-text); }
      .wx-notes { font-size: 12.5px; color: var(--pa-text-3); line-height: 1.55; }
      .wx-part[data-big="true"] { padding: 16px; border-radius: 24px; border-width: 2px; gap: 12px; }
      .wx-part[data-big="true"] .wx-part-head b { font-size: 26px; font-weight: 800; }
      .wx-part[data-big="true"] .wx-part-head span { font-size: 18px; }
      .wx-part[data-big="true"] .wx-clothes span { font-size: 21px; padding: 8px 14px; border-radius: 16px; }
      .wx-part[data-big="true"] .wx-extras { font-size: 19px; }
      .wx-part[data-big="true"] .wx-part-back { font-size: 21px; padding: 12px 14px; border-radius: 18px; }
      .wx-part[data-big="true"] .wx-part-back b { font-size: 30px; }
      .wx-part[data-big="true"] .wx-part-back small { font-size: 18px; }
      .wx-part[data-big="true"] .wx-notes { font-size: 18px; }
      .wx-times { display: flex; align-items: center; gap: 10px; }
      .wx-times label { flex: 1; min-width: 0; }
      .wx-time { width: auto; min-width: 104px; }
      .wx-loc { appearance: none; display: flex; justify-content: space-between; align-items: center; gap: 8px; padding: 12px 14px; border-radius: 14px; border: 0.5px solid var(--pa-line); background: var(--pa-surface); color: var(--pa-text); font-size: 15px; text-align: left; cursor: pointer; }
      .wx-loc small { color: var(--pa-text-3); font-size: 12px; }
    `}</style>
  );
}

/** 首页的小卡片：现在几度、一句结论、穿什么，点开到 /weather。取不到天气就不显示。 */
export function WeatherCard({ refreshKey }: { refreshKey?: number }) {
  const router = useRouter();
  const [data, setData] = useState<WeatherToday | null>(null);
  useEffect(() => {
    weatherToday().then(setData).catch(() => setData(null));
  }, [refreshKey]);
  if (!data?.advice || !data.snapshot) return null;
  const { advice, snapshot } = data;
  const part = keyPart(advice) ?? data.evening?.parts[0];
  const alert = advice.warnings.length > 0 || advice.parts.some((item) => item.directHome || item.skip);
  return (
    <button className="wx-card" data-tone={alert ? "rose" : "sky"} onClick={() => router.push("/weather")}>
      <span className="wx-card-icon">{weatherIcon(snapshot.now.text)}</span>
      <span className="wx-card-main">
        <b>{advice.headline}</b>
        <small className="pa-num">
          {deg(snapshot.now.temp)} 体感 {deg(snapshot.now.feels)} · {snapshot.now.text}
          {part && !part.skip ? ` · ${part.clothes.join("+")}` : ""}
        </small>
      </span>
      <ChevronRight size={18} className="wx-card-go" />
      <style jsx>{`
        .wx-card {
          appearance: none; display: flex; align-items: center; gap: 12px; width: calc(100% - 32px); margin: 12px 16px 0; padding: 12px 14px;
          border-radius: var(--pa-radius); border: 0.5px solid var(--pa-line); text-align: left; color: var(--pa-text); cursor: pointer;
          background: linear-gradient(90deg, color-mix(in srgb, var(--tone-bg) 70%, var(--pa-surface)) 0%, var(--pa-surface) 80%);
          box-shadow: var(--pa-shadow-sm); transition: transform 0.2s var(--pa-spring);
        }
        .wx-card:active { transform: scale(0.985); }
        .wx-card-icon { font-size: 30px; line-height: 1; flex-shrink: 0; }
        .wx-card-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
        .wx-card-main b { font-size: 15px; font-weight: 650; line-height: 1.4; }
        .wx-card-main small { font-size: 12.5px; color: var(--pa-text-2); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .wx-card :global(.wx-card-go) { color: var(--pa-text-3); flex-shrink: 0; }
      `}</style>
    </button>
  );
}
