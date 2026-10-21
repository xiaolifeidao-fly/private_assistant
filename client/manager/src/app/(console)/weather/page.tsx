"use client";

import { useEffect, useState } from "react";
import dayjs from "dayjs";
import { App, AutoComplete, Button, Col, DatePicker, Empty, Form, Input, InputNumber, Modal, Row, Segmented, Select, Space, Spin, Switch, Table, Tabs, TimePicker } from "antd";
import { AlertTriangle, MapPin, Moon, RefreshCw, RotateCcw, Send, Sparkles } from "lucide-react";
import { api, query } from "@/lib/api";
import { useData } from "@/lib/hooks";
import { Callout, Kpi, PageHeader, Panel, Pill } from "@/components/ui";
import {
  clothIcon, deg, hm, planLine, weatherIcon,
  type Advice, type Brief, type DayInput, type DayPlan, type OutfitPart, type WeatherConfig, type WeatherHour, type WeatherLocation, type WeatherStatus, type WeatherToday,
} from "@/lib/weather";

const slotOptions = [
  { value: "morning", label: "早上" },
  { value: "noon", label: "中午" },
  { value: "prepickup", label: "接之前" },
  { value: "evening", label: "晚上" },
];

export default function WeatherPage() {
  return (
    <>
      <PageHeader
        eyebrow="家庭"
        title="宝宝穿衣"
        description="按实时天气算宝宝穿什么、几点前到家，每天早 / 中 / 接之前 / 晚推给我和奶奶。数字和钟点由规则算，模型只负责说成家常话，对不上就用模板。"
      />
      <Tabs
        destroyInactiveTabPane
        items={[
          { key: "today", label: "今天", children: <TodayTab /> },
          { key: "plan", label: "送接安排", children: <PlanTab /> },
          { key: "history", label: "提醒记录", children: <HistoryTab /> },
          { key: "settings", label: "设置", children: <SettingsTab /> },
        ]}
      />
    </>
  );
}

/* ---------------------------------------------------------------- 今天 */

function TodayTab() {
  const { message } = App.useApp();
  const { data, loading, reload, setData } = useData(() => api.get<WeatherToday>("/weather"), []);
  const status = useData(() => api.get<WeatherStatus>("/weather/status"), []);
  const [refreshing, setRefreshing] = useState(false);

  const refresh = async () => {
    setRefreshing(true);
    try {
      setData(await api.post<WeatherToday>("/weather/refresh"));
      void status.reload();
      message.success("重新取到了");
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setRefreshing(false);
    }
  };

  if (loading && !data) return <div style={{ padding: 60, textAlign: "center" }}><Spin /></div>;
  if (!data) return <Empty description="取不到" />;
  const { snapshot, advice, evening } = data;
  const today = snapshot?.days?.find((day) => day.date === dayjs().format("YYYY-MM-DD"));
  const hours = (snapshot?.hours ?? []).filter((hour) => dayjs(hour.at).isSame(dayjs(), "day"));

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      {data.error && <Callout tone="rose" icon={<AlertTriangle size={16} />}>天气取不到：{data.error}</Callout>}
      {snapshot && advice && (
        <>
          <div className="wx-kpis">
            <Kpi label={`${snapshot.now.text} · ${snapshot.now.wind}`} value={<span>{weatherIcon(snapshot.now.text)} {deg(snapshot.now.temp)}</span>} />
            <Kpi label="体感" value={deg(snapshot.now.feels)} />
            <Kpi label={today ? `今天 · ${today.sunset} 天黑` : "今天"} value={today ? `${deg(today.low)}–${deg(today.high)}` : "—"} />
            <Kpi label="空气 AQI" value={snapshot.now.aqi || "—"} tone={snapshot.now.aqi > 150 ? "rose" : undefined} />
          </div>
          <Panel
            title={<span><MapPin size={15} style={{ verticalAlign: -2 }} /> {data.location.name} · {advice.headline}</span>}
            extra={
              <>
                <span className="pa-muted" style={{ fontSize: 12 }}>
                  {snapshot.source === "cma" ? "中国天气网" : "Open-Meteo（备用）"} · {dayjs(snapshot.fetchedAt).format("HH:mm")} 取的{snapshot.stale ? "（旧数据）" : ""}
                </span>
                <Button size="small" icon={<RefreshCw size={14} />} className="pa-icon-text" loading={refreshing} onClick={refresh}>重新取</Button>
              </>
            }
            bodyPadding
          >
            {advice.warnings.map((warning) => (
              <Callout key={warning.type} tone="rose" icon={<AlertTriangle size={16} />} style={{ marginBottom: 12 }}>
                <b>{warning.type}{warning.level}预警</b> {warning.text}
              </Callout>
            ))}
            {hours.length > 1 && (
              <>
                <HourChart hours={hours} parts={[...advice.parts]} sunset={today?.sunset} />
                <div className="pa-muted" style={{ fontSize: 12, textAlign: "right" }}>实线：体感　虚线：气温　蓝底：有雨</div>
              </>
            )}
            <div className="wx-parts">
              {advice.parts.map((part) => <PartView key={part.kind + part.start} part={part} />)}
              {evening?.night && (
                <div className="wx-part">
                  <div className="wx-part-head"><b><Moon size={14} style={{ verticalAlign: -2 }} /> 今晚睡觉</b><span>最低 {deg(evening.night.low)}</span></div>
                  <div>{evening.night.bedding}</div>
                </div>
              )}
              {evening?.parts.map((part) => <PartView key={"e" + part.kind + part.start} part={part} />)}
            </div>
            {snapshot.clothing && <div className="pa-muted" style={{ marginTop: 12, fontSize: 12.5 }}>中国天气网穿衣指数（参考）：{snapshot.clothing}</div>}
          </Panel>
        </>
      )}
      <Row gutter={16}>
        <Col xs={24} lg={14}>
          <Panel title="今天的提醒" bodyPadding>
            {data.briefs.length === 0 ? <Empty description="今天还没推" image={Empty.PRESENTED_IMAGE_SIMPLE} /> : (
              <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
                {[...data.briefs].reverse().map((brief) => <BriefView key={brief.briefKey} brief={brief} />)}
              </div>
            )}
          </Panel>
        </Col>
        <Col xs={24} lg={10}>
          <TryPanel onSent={() => void reload()} />
          <Panel title="天气接口" bodyPadding style={{ marginTop: 16 }}>
            {status.data ? (
              <div style={{ fontSize: 13, lineHeight: 1.9 }}>
                <div>正在用：{status.data.source === "cma" ? "中国天气网" : status.data.source || "—"}</div>
                <div>最近成功：{status.data.lastSuccess && !status.data.lastSuccess.startsWith("0001") ? dayjs(status.data.lastSuccess).format("M-D HH:mm") : "—"}</div>
                <div>今天请求（本进程）：{status.data.calls} 次</div>
                {status.data.lastError && (
                  <div style={{ color: "var(--tone-rose)" }}>最近出错（{dayjs(status.data.lastErrorAt).format("M-D HH:mm")}）：{status.data.lastError}</div>
                )}
              </div>
            ) : <Spin size="small" />}
          </Panel>
        </Col>
      </Row>
      <style jsx global>{`
        .wx-kpis { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
        @media (max-width: 720px) { .wx-kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
        .wx-parts { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 12px; margin-top: 12px; }
        .wx-part { border: 1px solid var(--pa-line); border-radius: 12px; padding: 12px 14px; display: flex; flex-direction: column; gap: 8px; font-size: 13.5px; }
        .wx-part[data-alert="true"] { border-color: color-mix(in srgb, var(--tone-rose) 50%, transparent); }
        .wx-part-head { display: flex; justify-content: space-between; gap: 8px; }
        .wx-part-head b { font-size: 14.5px; }
        .wx-part-head span { color: var(--pa-text-3); font-size: 12px; }
        .wx-clothes { display: flex; flex-wrap: wrap; gap: 6px; }
        .wx-clothes span { padding: 2px 8px; border-radius: 8px; background: var(--tone-amber-bg); }
        .wx-back { padding: 6px 10px; border-radius: 8px; background: var(--tone-sage-bg); }
        .wx-back[data-alert="true"] { background: var(--tone-rose-bg); }
        .wx-back small { display: block; color: var(--pa-text-2); }
        .wx-brief { border: 1px solid var(--pa-line); border-radius: 10px; padding: 10px 12px; font-size: 13.5px; line-height: 1.65; white-space: pre-wrap; }
        .wx-brief[data-skipped="true"] { opacity: 0.6; }
        .wx-brief-head { display: flex; gap: 8px; align-items: center; margin-bottom: 4px; white-space: normal; }
        .wx-chart { width: 100%; height: auto; display: block; }
        .wx-chart text { font-size: 10px; fill: var(--pa-text-3); }
        .wx-chart .wx-val { fill: var(--pa-text-2); }
      `}</style>
    </div>
  );
}

function HourChart({ hours, parts, sunset }: { hours: WeatherHour[]; parts: OutfitPart[]; sunset?: string }) {
  const day = dayjs(hours[0].at).format("YYYY-MM-DD");
  const start = dayjs(`${day} 06:00`);
  const end = dayjs(`${day} 21:00`);
  const shown = hours.filter((hour) => !dayjs(hour.at).isBefore(start) && !dayjs(hour.at).isAfter(end));
  if (shown.length < 2) return null;
  const width = 760;
  const height = 170;
  const pad = { left: 12, right: 12, top: 44, bottom: 26 };
  const min = Math.floor(Math.min(...shown.map((hour) => Math.min(hour.feels, hour.temp))) - 2);
  const max = Math.ceil(Math.max(...shown.map((hour) => Math.max(hour.feels, hour.temp))) + 2);
  const span = end.diff(start, "minute");
  const x = (value: string) => pad.left + (dayjs(value).diff(start, "minute") / span) * (width - pad.left - pad.right);
  const y = (value: number) => pad.top + (1 - (value - min) / Math.max(max - min, 1)) * (height - pad.top - pad.bottom);
  const line = (pick: (hour: WeatherHour) => number) => shown.map((hour, i) => `${i ? "L" : "M"}${x(hour.at).toFixed(1)},${y(pick(hour)).toFixed(1)}`).join(" ");
  const marks: { at: string; label: string; color: string; solid?: boolean }[] = [];
  for (const part of parts) {
    if (part.skip) continue;
    marks.push({ at: part.start, label: part.kind === "drop" ? "送" : part.kind === "pickup" ? "接" : "出门", color: "var(--tone-sage)" });
    if (part.backBy && part.kind !== "drop") marks.push({ at: part.backBy, label: `${hm(part.backBy)} 到家`, color: "var(--tone-rose)", solid: true });
  }
  if (sunset) marks.push({ at: `${day}T${sunset}:00+08:00`, label: "天黑", color: "var(--pa-text-3)" });
  return (
    <svg className="wx-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="今天每小时的气温和体感">
      {shown.filter((hour) => hour.wet).map((hour) => (
        <rect key={hour.at} x={x(hour.at)} y={pad.top - 6} width={(width - pad.left - pad.right) / (span / 60)} height={height - pad.top - pad.bottom + 6} fill="var(--tone-sky)" opacity="0.15" />
      ))}
      <path d={line((hour) => hour.temp)} fill="none" stroke="var(--pa-text-3)" strokeWidth="1.2" strokeDasharray="4 3" />
      <path d={line((hour) => hour.feels)} fill="none" stroke="var(--tone-amber)" strokeWidth="2.2" />
      {shown.map((hour, i) => (
        <g key={hour.at}>
          <circle cx={x(hour.at)} cy={y(hour.feels)} r="2.4" fill="var(--tone-amber)" />
          {i % 2 === 0 && <text className="wx-val" x={x(hour.at)} y={y(hour.feels) - 7} textAnchor="middle">{deg(hour.feels)}</text>}
          {i % 2 === 0 && <text x={x(hour.at)} y={height - 8} textAnchor="middle">{dayjs(hour.at).format("H")}点</text>}
        </g>
      ))}
      {placeLabels(marks.map((mark) => ({ ...mark, px: x(mark.at) })).filter((mark) => mark.px >= pad.left && mark.px <= width - pad.right)).map((mark) => (
        <g key={mark.label + mark.at}>
          <line x1={mark.px} x2={mark.px} y1={pad.top - 10 - mark.row * 13} y2={height - pad.bottom} stroke={mark.color} strokeWidth={mark.solid ? 2 : 1.2} strokeDasharray={mark.solid ? undefined : "3 3"} />
          <text x={mark.px} y={pad.top - 14 - mark.row * 13} textAnchor="middle" style={{ fill: mark.color, fontWeight: 700 }}>{mark.label}</text>
        </g>
      ))}
    </svg>
  );
}

/** 竖线上的字挨得太近就往上错一行，最多三行。 */
function placeLabels<T extends { px: number; label: string }>(marks: T[]): (T & { row: number })[] {
  const lastEnd = [-Infinity, -Infinity, -Infinity];
  return [...marks].sort((a, b) => a.px - b.px).map((mark) => {
    const half = mark.label.length * 5.5 + 3;
    let row = lastEnd.findIndex((end) => mark.px - half > end);
    if (row < 0) row = 2;
    lastEnd[row] = mark.px + half;
    return { ...mark, row };
  });
}

function PartView({ part }: { part: OutfitPart }) {
  return (
    <div className="wx-part" data-alert={part.directHome || part.skip}>
      <div className="wx-part-head">
        <b>{part.label}</b>
        <span>{hm(part.start)} · 体感 {deg(part.feelsMin)}{part.feelsMax - part.feelsMin >= 2 ? `–${deg(part.feelsMax)}` : ""} · {part.level.label}（{part.layers} 层）</span>
      </div>
      {part.skip ? <div className="wx-back" data-alert="true">{part.skipReason}</div> : (
        <>
          <div className="wx-clothes">{part.clothes.map((cloth) => <span key={cloth}>{clothIcon(cloth)} {cloth}</span>)}</div>
          {part.extras.length > 0 && <div className="pa-muted">带上：{part.extras.join("、")}</div>}
          {part.backBy && part.kind !== "drop" && (
            <div className="wx-back" data-alert={part.directHome || !!part.backReason}>
              {part.directHome ? "接到直接回，" : ""}<b>{hm(part.backBy)}</b> 前到家
              {part.backReason && <small>{part.backReason}</small>}
              {part.playUntil && <small>天气好，想多玩可以到 {hm(part.playUntil)}</small>}
            </div>
          )}
          {part.notes.length > 0 && <div className="pa-muted" style={{ fontSize: 12.5 }}>{part.notes.join("；")}</div>}
          {part.estimated && <div className="pa-muted" style={{ fontSize: 12 }}>没有逐小时预报，按全天温度估的</div>}
        </>
      )}
    </div>
  );
}

function BriefView({ brief, showDate }: { brief: Brief; showDate?: boolean }) {
  return (
    <div className="wx-brief" data-skipped={brief.source === "skipped"}>
      <div className="wx-brief-head">
        <b>{showDate ? `${brief.date} ` : ""}{brief.slotLabel}</b>
        <span className="pa-muted" style={{ fontSize: 12 }}>{dayjs(brief.createdAt).format("HH:mm")}</span>
        {brief.source === "llm" && <Pill tone="plum"><Sparkles size={11} /> 模型写的</Pill>}
        {brief.source === "template" && <Pill tone="slate">模板</Pill>}
        {brief.source === "skipped" && <Pill tone="slate">没变化，没推</Pill>}
        {brief.source !== "skipped" && <span className="pa-muted" style={{ fontSize: 12 }}>推给 {brief.pushedTo.length} 个账号</span>}
      </div>
      {brief.content}
    </div>
  );
}

/** 试一下：选时段和日期先预览（不存不推），满意了再推送。 */
function TryPanel({ onSent }: { onSent: () => void }) {
  const { message, modal } = App.useApp();
  const [slot, setSlot] = useState("morning");
  const [date, setDate] = useState(dayjs());
  const [preview, setPreview] = useState<Brief | null>(null);
  const [busy, setBusy] = useState("");

  const run = async () => {
    setBusy("preview");
    try {
      setPreview(await api.post<Brief>("/weather/preview", { slot, date: date.format("YYYY-MM-DD") }));
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setBusy("");
    }
  };
  const send = () => {
    modal.confirm({
      title: "现在推送？",
      content: "会重新生成这个时段的提醒（覆盖已有的），推到我和奶奶的手机上。",
      okText: "推送",
      onOk: async () => {
        setBusy("send");
        try {
          const brief = await api.post<Brief>("/weather/send", { slot, date: date.format("YYYY-MM-DD") });
          message.success(`推给了 ${brief.pushedTo.length} 个账号`);
          setPreview(brief);
          onSent();
        } catch (error) {
          message.error((error as Error).message);
        } finally {
          setBusy("");
        }
      },
    });
  };

  return (
    <Panel title="试一下" bodyPadding>
      <Space wrap>
        <Segmented options={slotOptions} value={slot} onChange={(value) => setSlot(value as string)} />
        <DatePicker value={date} onChange={(value) => value && setDate(value)} allowClear={false} disabledDate={(value) => value.isBefore(dayjs(), "day") || value.isAfter(dayjs().add(6, "day"))} />
      </Space>
      <Space style={{ marginTop: 12 }}>
        <Button loading={busy === "preview"} onClick={run}>预览</Button>
        <Button type="primary" icon={<Send size={14} />} className="pa-icon-text" loading={busy === "send"} onClick={send}>现在推送</Button>
      </Space>
      {preview && (
        <div style={{ marginTop: 12 }}>
          <div style={{ fontWeight: 650, marginBottom: 6 }}>{preview.headline}</div>
          <BriefView brief={preview} />
        </div>
      )}
    </Panel>
  );
}

/* ---------------------------------------------------------------- 送接安排 */

function PlanTab() {
  const { message } = App.useApp();
  const [from, setFrom] = useState(dayjs());
  const { data, loading, reload } = useData(() => api.get<{ items: DayPlan[] }>(`/weather/days${query({ from: from.format("YYYY-MM-DD"), days: 14 })}`), [from.format("YYYY-MM-DD")]);
  const [editing, setEditing] = useState<DayPlan | null>(null);

  const reset = async (plan: DayPlan) => {
    try {
      await api.del(`/weather/day?date=${plan.date}`);
      message.success("恢复成平常的安排了");
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  return (
    <Panel
      title="接下来两周"
      extra={<DatePicker value={from} onChange={(value) => value && setFrom(value)} allowClear={false} disabledDate={(value) => value.isBefore(dayjs(), "day")} />}
    >
      <Table<DayPlan>
        rowKey="date"
        loading={loading}
        dataSource={data?.items ?? []}
        pagination={false}
        size="middle"
        columns={[
          { title: "日期", dataIndex: "date", width: 140, render: (value: string) => `${dayjs(value).format("M月D日")} 周${"日一二三四五六"[dayjs(value).day()]}` },
          { title: "安排", render: (_, plan) => <span>{planLine(plan)}{plan.note && <span className="pa-muted"> · {plan.note}</span>}</span> },
          { title: "", width: 110, render: (_, plan) => plan.override ? <Pill tone="amber">临时改过</Pill> : <span className="pa-muted">平常</span> },
          {
            title: "", width: 170, render: (_, plan) => (
              <Space>
                <Button size="small" onClick={() => setEditing(plan)}>改这天</Button>
                {plan.override && <Button size="small" type="text" onClick={() => void reset(plan)}>恢复平常</Button>}
              </Space>
            ),
          },
        ]}
      />
      <DayModal plan={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); void reload(); }} />
    </Panel>
  );
}

const timeFormat = "HH:mm";
const toTime = (value?: string) => (value ? dayjs(`2000-01-01 ${value}`) : null);

function DayModal({ plan, onClose, onSaved }: { plan: DayPlan | null; onClose: () => void; onSaved: () => void }) {
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  const school = Form.useWatch("school", form);
  useEffect(() => {
    if (!plan) return;
    form.setFieldsValue({
      school: plan.school ? "yes" : "no", drop: toTime(plan.drop || "08:00"), pickup: toTime(plan.pickup || "15:50"), home: toTime(plan.home || "16:30"),
      outings: plan.outings.map((outing) => ({ range: [toTime(outing.from), toTime(outing.to)], note: outing.note })), note: plan.note,
    });
  }, [plan, form]);

  const save = async () => {
    if (!plan) return;
    const values = await form.validateFields();
    const input: DayInput = {
      date: plan.date, noSchool: values.school === "no", note: values.note ?? "",
      outings: (values.outings ?? []).filter((item: { range?: dayjs.Dayjs[] }) => item?.range?.length === 2)
        .map((item: { range: dayjs.Dayjs[]; note?: string }) => ({ from: item.range[0].format(timeFormat), to: item.range[1].format(timeFormat), note: item.note ?? "" })),
    };
    if (values.school === "yes") {
      input.drop = values.drop?.format(timeFormat);
      input.pickup = values.pickup?.format(timeFormat);
      input.home = values.home?.format(timeFormat);
    }
    setSaving(true);
    try {
      await api.put("/weather/day", input);
      message.success("改好了，提醒会按新的安排算");
      onSaved();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal open={!!plan} title={plan ? `${dayjs(plan.date).format("M月D日")}的安排` : ""} onCancel={onClose} onOk={save} confirmLoading={saving} okText="保存" destroyOnClose>
      <Form form={form} layout="vertical" requiredMark={false}>
        <Form.Item name="school" label="去不去幼儿园">
          <Segmented options={[{ value: "yes", label: "去" }, { value: "no", label: "不去" }]} />
        </Form.Item>
        {school === "yes" && (
          <Row gutter={12}>
            <Col span={8}><Form.Item name="drop" label="送"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
            <Col span={8}><Form.Item name="pickup" label="接"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
            <Col span={8}><Form.Item name="home" label="平常几点前到家"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
          </Row>
        )}
        <Form.List name="outings">
          {(fields, { add, remove }) => (
            <Form.Item label={school === "yes" ? "接完还要去别处（不填就是接完回家）" : "出去的时段（不填就按平常周末的）"}>
              {fields.map((field) => (
                <Space key={field.key} style={{ display: "flex", marginBottom: 8 }} align="baseline">
                  <Form.Item name={[field.name, "range"]} noStyle><TimePicker.RangePicker format={timeFormat} minuteStep={5} /></Form.Item>
                  <Form.Item name={[field.name, "note"]} noStyle><Input placeholder="去哪" style={{ width: 140 }} /></Form.Item>
                  <Button type="text" onClick={() => remove(field.name)}>删</Button>
                </Space>
              ))}
              <Button size="small" onClick={() => add()}>加一段</Button>
            </Form.Item>
          )}
        </Form.List>
        <Form.Item name="note" label="备注"><Input maxLength={100} /></Form.Item>
      </Form>
    </Modal>
  );
}

/* ---------------------------------------------------------------- 提醒记录 */

function HistoryTab() {
  const [range, setRange] = useState<[dayjs.Dayjs, dayjs.Dayjs]>([dayjs().subtract(6, "day"), dayjs()]);
  const { data, loading } = useData(
    () => api.get<{ items: Brief[] }>(`/weather/briefs${query({ from: range[0].format("YYYY-MM-DD"), to: range[1].format("YYYY-MM-DD") })}`),
    [range[0].format("YYYY-MM-DD"), range[1].format("YYYY-MM-DD")],
  );
  const [open, setOpen] = useState<Brief | null>(null);
  return (
    <Panel title="推过的提醒" extra={<DatePicker.RangePicker value={range} onChange={(value) => value?.[0] && value[1] && setRange([value[0], value[1]])} allowClear={false} />}>
      <Table<Brief>
        rowKey="briefKey"
        loading={loading}
        dataSource={data?.items ?? []}
        pagination={{ pageSize: 20 }}
        size="middle"
        columns={[
          { title: "时间", width: 150, render: (_, brief) => `${dayjs(brief.date).format("M月D日")} ${brief.slotLabel}` },
          { title: "结论", dataIndex: "headline", width: 280 },
          { title: "推送内容", dataIndex: "content", render: (value: string) => <span style={{ whiteSpace: "pre-wrap" }}>{value}</span> },
          { title: "", width: 120, render: (_, brief) => <Space direction="vertical" size={2}>{brief.source === "llm" ? <Pill tone="plum">模型</Pill> : brief.source === "template" ? <Pill tone="slate">模板</Pill> : <Pill tone="slate">没推</Pill>}<a onClick={() => setOpen(brief)}>看依据</a></Space> },
        ]}
      />
      <Modal open={!!open} onCancel={() => setOpen(null)} footer={null} width={760} title={open ? `${open.date} ${open.slotLabel}的依据` : ""}>
        {open?.advice ? <AdviceView advice={open.advice} /> : <Empty description="没有存依据" />}
      </Modal>
    </Panel>
  );
}

function AdviceView({ advice }: { advice: Advice }) {
  return (
    <div>
      <div style={{ marginBottom: 8 }}>
        {advice.place} · 当时 {deg(advice.now.temp)}（体感 {deg(advice.now.feels)}）{advice.now.text}
        {advice.hasRange && ` · 全天 ${deg(advice.low)}–${deg(advice.high)}，${advice.sunset} 天黑`}
      </div>
      {advice.warnings.map((warning) => <Callout key={warning.type} tone="rose" style={{ marginBottom: 8 }}>{warning.type}{warning.level}预警</Callout>)}
      <div className="wx-parts">
        {advice.parts.map((part) => <PartView key={part.kind + part.start} part={part} />)}
        {advice.night && <div className="wx-part"><b>今晚</b>最低 {deg(advice.night.low)}，{advice.night.bedding}</div>}
      </div>
    </div>
  );
}

/* ---------------------------------------------------------------- 设置 */

const roleOptions = [
  { value: "admin", label: "我（admin）" },
  { value: "member", label: "我（member）" },
  { value: "elder", label: "奶奶（长辈模式）" },
  { value: "spouse", label: "老婆" },
];
const weekdayOptions = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"].map((label, value) => ({ value, label }));

function SettingsTab() {
  const { message, modal } = App.useApp();
  const [form] = Form.useForm();
  const { data, loading, setData } = useData(() => api.get<{ config: WeatherConfig; defaults: WeatherConfig }>("/weather/config"), []);
  const [saving, setSaving] = useState(false);
  const [options, setOptions] = useState<WeatherLocation[]>([]);
  const location = Form.useWatch("location", form) as WeatherLocation | undefined;

  const fill = (config: WeatherConfig) => form.setFieldsValue({
    ...config,
    slots: { ...config.slots, morning: toTime(config.slots.morning), noon: toTime(config.slots.noon), evening: toTime(config.slots.evening) },
    school: { ...config.school, drop: toTime(config.school.drop), pickup: toTime(config.school.pickup), home: toTime(config.school.home) },
    weekend: config.weekend.map((outing) => ({ range: [toTime(outing.from), toTime(outing.to)] })),
    quiet: [toTime(config.quietFrom), toTime(config.quietTo)],
    levels: config.levels.map((level) => ({ ...level, clothes: level.clothes })),
  });
  useEffect(() => {
    if (data) fill(data.config);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data]);

  const search = async (text: string) => {
    if (!text.trim()) return;
    try {
      const result = await api.get<{ items: WeatherLocation[] }>(`/weather/locations${query({ q: text.trim() })}`);
      setOptions(result.items);
    } catch (error) {
      message.error((error as Error).message);
    }
  };

  const save = async () => {
    const values = await form.validateFields();
    const fmt = (value?: dayjs.Dayjs | null) => value?.format(timeFormat) ?? "";
    const config: WeatherConfig = {
      ...data!.config,
      ...values,
      slots: { morning: fmt(values.slots.morning), noon: fmt(values.slots.noon), evening: fmt(values.slots.evening), prePickup: values.slots.prePickup ?? 0 },
      school: { drop: fmt(values.school.drop), pickup: fmt(values.school.pickup), home: fmt(values.school.home), walk: values.school.walk },
      weekend: (values.weekend ?? []).filter((item: { range?: dayjs.Dayjs[] }) => item?.range?.length === 2)
        .map((item: { range: dayjs.Dayjs[] }) => ({ from: fmt(item.range[0]), to: fmt(item.range[1]) })),
      quietFrom: fmt(values.quiet?.[0]), quietTo: fmt(values.quiet?.[1]),
    };
    delete (config as unknown as Record<string, unknown>).quiet;
    setSaving(true);
    try {
      const result = await api.put<{ config: WeatherConfig }>("/weather/config", config);
      setData({ config: result.config, defaults: data!.defaults });
      message.success("已保存，下一次提醒就按这个算");
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const reset = () => modal.confirm({
    title: "恢复出厂设置？", content: "位置回到上海青浦，推送时间、送接时间、回家线、穿衣档位都回到默认。", okText: "恢复",
    onOk: async () => {
      const result = await api.post<{ config: WeatherConfig }>("/weather/config/reset");
      setData({ config: result.config, defaults: data!.defaults });
      message.success("已恢复");
    },
  });

  if (loading && !data) return <div style={{ padding: 60, textAlign: "center" }}><Spin /></div>;
  return (
    <Panel bodyPadding>
      <Form form={form} layout="vertical" requiredMark={false} style={{ maxWidth: 900 }}>
        <Row gutter={16}>
          <Col span={6}><Form.Item name="enabled" label="开启推送" valuePropName="checked"><Switch /></Form.Item></Col>
          <Col span={18}>
            <Form.Item label="查哪里的天气" extra={location ? `现在：${location.name}（中国天气网编码 ${location.code}，备用源用经纬度 ${location.lat?.toFixed?.(3)}, ${location.lon?.toFixed?.(3)}）` : undefined}>
              <AutoComplete
                options={options.map((item) => ({ value: item.code, label: `${item.name}（${item.code}）` }))}
                onSearch={(text) => void search(text)}
                onSelect={(code: string) => {
                  const picked = options.find((item) => item.code === code);
                  if (picked) form.setFieldValue("location", picked);
                }}
                placeholder="搜地名换地方：青浦、赵巷、老家的县名……"
              />
            </Form.Item>
            <Form.Item name="location" hidden><Input /></Form.Item>
          </Col>
        </Row>

        <div className="pa-form-group">推送时间</div>
        <Row gutter={16}>
          <Col span={6}><Form.Item name={["slots", "morning"]} label="早上"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["slots", "noon"]} label="中午"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["slots", "evening"]} label="晚上"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["slots", "prePickup"]} label="接之前再看一眼（分钟）" extra="0 = 不看；结论变了才推"><InputNumber min={0} max={180} style={{ width: "100%" }} /></Form.Item></Col>
        </Row>

        <div className="pa-form-group">平常的送接（某一天不一样在「送接安排」里改）</div>
        <Row gutter={16}>
          <Col span={6}><Form.Item name={["school", "drop"]} label="送"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["school", "pickup"]} label="接"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["school", "home"]} label="平常几点前到家"><TimePicker format={timeFormat} minuteStep={5} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["school", "walk"]} label="接到走回家要几分钟" extra="算「接到直接回」几点到家"><InputNumber min={5} max={120} style={{ width: "100%" }} /></Form.Item></Col>
        </Row>
        <Row gutter={16}>
          <Col span={12}><Form.Item name="schoolDays" label="哪几天上幼儿园"><Select mode="multiple" options={weekdayOptions} /></Form.Item></Col>
          <Col span={12}>
            <Form.List name="weekend">
              {(fields, { add, remove }) => (
                <Form.Item label="不上学的日子平常出去玩">
                  {fields.map((field) => (
                    <Space key={field.key} style={{ display: "flex", marginBottom: 8 }}>
                      <Form.Item name={[field.name, "range"]} noStyle><TimePicker.RangePicker format={timeFormat} minuteStep={5} /></Form.Item>
                      <Button type="text" onClick={() => remove(field.name)}>删</Button>
                    </Space>
                  ))}
                  <Button size="small" onClick={() => add()}>加一段</Button>
                </Form.Item>
              )}
            </Form.List>
          </Col>
        </Row>

        <div className="pa-form-group">回家线</div>
        <Row gutter={16}>
          <Col span={6}><Form.Item name={["thresholds", "backBy"]} label="体感低于（度）就回家"><InputNumber min={-10} max={25} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["thresholds", "backByInfant"]} label="不到 1 岁（度）"><InputNumber min={-10} max={25} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["thresholds", "goOut"]} label="体感低于（度）别出去玩"><InputNumber min={-20} max={20} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["thresholds", "coldMinutes"]} label="冷天在外面最多（分钟）"><InputNumber min={10} max={300} style={{ width: "100%" }} /></Form.Item></Col>
        </Row>
        <Row gutter={16}>
          <Col span={6}><Form.Item name={["thresholds", "freezeMinutes"]} label="冻天在外面最多（分钟）"><InputNumber min={10} max={300} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["thresholds", "aqiMask"]} label="AQI 超过戴口罩"><InputNumber min={50} max={500} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name={["thresholds", "aqiStay"]} label="AQI 超过别出门"><InputNumber min={50} max={500} style={{ width: "100%" }} /></Form.Item></Col>
        </Row>

        <div className="pa-form-group">穿衣档位（按体感从高到低，最冷那档兜底）</div>
        <Form.List name="levels">
          {(fields) => fields.map((field) => (
            <Row key={field.key} gutter={8} style={{ marginBottom: 4 }}>
              <Col span={3}><Form.Item name={[field.name, "label"]} label={field.name === 0 ? "名字" : undefined}><Input /></Form.Item></Col>
              <Col span={3}><Form.Item name={[field.name, "min"]} label={field.name === 0 ? "体感不低于" : undefined}><InputNumber style={{ width: "100%" }} /></Form.Item></Col>
              <Col span={2}><Form.Item name={[field.name, "layers"]} label={field.name === 0 ? "层数" : undefined}><InputNumber min={1} max={6} style={{ width: "100%" }} /></Form.Item></Col>
              <Col span={10}><Form.Item name={[field.name, "clothes"]} label={field.name === 0 ? "穿什么" : undefined}><Select mode="tags" tokenSeparators={[",", "，", "、"]} open={false} /></Form.Item></Col>
              <Col span={6}><Form.Item name={[field.name, "tip"]} label={field.name === 0 ? "补一句" : undefined}><Input /></Form.Item></Col>
              <Form.Item name={[field.name, "key"]} hidden><Input /></Form.Item>
            </Row>
          ))}
        </Form.List>

        <div className="pa-form-group">推给谁、临时提醒</div>
        <Row gutter={16}>
          <Col span={10}><Form.Item name="recipients" label="推给哪些角色的手机账号"><Select mode="multiple" options={roleOptions} /></Form.Item></Col>
          <Col span={4}><Form.Item name="alerts" label="新预警立刻推" valuePropName="checked"><Switch /></Form.Item></Col>
          <Col span={4}><Form.Item name="alertMax" label="一天最多几条"><InputNumber min={0} max={5} style={{ width: "100%" }} /></Form.Item></Col>
          <Col span={6}><Form.Item name="quiet" label="这段时间不推临时提醒"><TimePicker.RangePicker format={timeFormat} order={false} style={{ width: "100%" }} /></Form.Item></Col>
        </Row>
        <Form.Item name="childKey" hidden><Input /></Form.Item>

        <Space>
          <Button type="primary" loading={saving} onClick={save}>保存</Button>
          <Button icon={<RotateCcw size={15} />} className="pa-icon-text" onClick={reset}>恢复出厂设置</Button>
        </Space>
      </Form>
      <style jsx global>{`
        .pa-form-group { font-weight: 650; margin: 8px 0 12px; padding-top: 12px; border-top: 1px solid var(--pa-line); }
      `}</style>
    </Panel>
  );
}
