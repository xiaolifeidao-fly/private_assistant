"use client";

import { useEffect, useState } from "react";
import dayjs from "dayjs";
import { App, Button, Drawer, Empty, Image, Input, Popconfirm, Segmented, Select, TimePicker } from "antd";
import { Pencil, Plus, Sparkles, Trash2 } from "lucide-react";
import { fetchObjectURL } from "@/lib/api";
import { amountOf, amounts, cellKey, DAY, dietApi, mealLabels, weekdays, type DietMeal, type DietPlan, type DietRecord } from "@/lib/diet";

export interface CellTarget {
  date: string;
  meal: string;
}

/** 点一个格子：改这一餐的计划，看 / 改 / 删 / 补这一餐的实际记录。 */
export function CellDrawer({ target, child, meals, plan, records, accountName, onClose, onChanged }: {
  target: CellTarget | null;
  child: string;
  meals: DietMeal[];
  plan?: DietPlan;
  records: DietRecord[];
  accountName: (key: string) => string;
  onClose: () => void;
  onChanged: (keys: string[]) => void;
}) {
  const [editing, setEditing] = useState<string | "new" | null>(null);
  useEffect(() => setEditing(null), [target?.date, target?.meal]);
  const day = target ? dayjs(target.date) : dayjs();
  const title = target ? `${day.format("M月D日")} 周${weekdays[day.day()]} · ${target.meal === DAY ? "全天要求" : mealLabels[target.meal]}` : "";

  return (
    <Drawer title={title} open={!!target} onClose={onClose} width={520} destroyOnClose>
      {target && (
        <>
          <PlanEditor key={cellKey(target.date, target.meal)} target={target} child={child} plan={plan} accountName={accountName} onChanged={onChanged} />
          {target.meal !== DAY && (
            <>
              <div className="pa-section-label" style={{ marginTop: 28 }}>
                实际吃了什么
                <span style={{ flex: 1 }} />
                {editing !== "new" && (
                  <Button size="small" icon={<Plus size={14} />} className="pa-icon-text" onClick={() => setEditing("new")}>补一条</Button>
                )}
              </div>
              {editing === "new" && (
                <RecordForm target={target} child={child} meals={meals} onDone={(keys) => { setEditing(null); if (keys) onChanged(keys); }} />
              )}
              {records.length === 0 && editing !== "new" && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="这一餐还没有记录" />}
              {records.map((record) =>
                editing === record.recordKey ? (
                  <RecordForm key={record.recordKey} target={target} child={child} meals={meals} record={record}
                    onDone={(keys) => { setEditing(null); if (keys) onChanged(keys); }} />
                ) : (
                  <RecordView key={record.recordKey} record={record} accountName={accountName} onEdit={() => setEditing(record.recordKey)} onChanged={onChanged} />
                ),
              )}
            </>
          )}
        </>
      )}
      <style jsx global>{`
        .diet-card { border: 1px solid var(--pa-line); border-radius: 12px; padding: 14px; background: var(--pa-surface); }
        .diet-card + .diet-card { margin-top: 10px; }
        .diet-card-foot { display: flex; align-items: center; gap: 8px; margin-top: 10px; font-size: 12px; color: var(--pa-text-3); flex-wrap: wrap; }
        .diet-field { margin-bottom: 12px; }
        .diet-field > label { display: block; font-size: 12.5px; color: var(--pa-text-2); font-weight: 500; margin-bottom: 6px; }
      `}</style>
    </Drawer>
  );
}

function PlanEditor({ target, child, plan, accountName, onChanged }: {
  target: CellTarget;
  child: string;
  plan?: DietPlan;
  accountName: (key: string) => string;
  onChanged: (keys: string[]) => void;
}) {
  const { message } = App.useApp();
  const [foods, setFoods] = useState(plan?.foods ?? "");
  const [note, setNote] = useState(plan?.note ?? "");
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    setFoods(plan?.foods ?? "");
    setNote(plan?.note ?? "");
  }, [plan]);
  const dirty = foods !== (plan?.foods ?? "") || note !== (plan?.note ?? "");
  const isDay = target.meal === DAY;

  const save = async (values: { foods: string; note: string }) => {
    setSaving(true);
    try {
      await dietApi.savePlan({ childKey: child, date: target.date, meal: target.meal, ...values });
      message.success(values.foods || values.note ? "计划已保存" : "这一餐的计划已取消");
      onChanged([cellKey(target.date, target.meal)]);
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <div className="pa-section-label">计划</div>
      <div className="diet-card" style={{ background: "var(--tone-amber-bg)", borderStyle: "dashed" }}>
        <div className="diet-field">
          <label>{isDay ? "这一天的总体要求" : "打算吃什么"}</label>
          <Input.TextArea value={foods} onChange={(e) => setFoods(e.target.value)} autoSize={{ minRows: 2, maxRows: 6 }}
            placeholder={isDay ? "一天一杯奶、多喝水、不给糖" : "小米粥、蒸鸡蛋、半根香蕉"} />
        </div>
        <div className="diet-field" style={{ marginBottom: 0 }}>
          <label>要求 / 注意</label>
          <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="少盐、要吃完、幼儿园" />
        </div>
        <div className="diet-card-foot">
          {plan && <span>{accountName(plan.updatedBy) || "有人"} {dayjs(plan.updatedAt).format("M月D日 HH:mm")} 改过</span>}
          <span style={{ flex: 1 }} />
          {plan && (
            <Popconfirm title="取消这一餐的计划？" onConfirm={() => save({ foods: "", note: "" })} okText="取消计划" okButtonProps={{ danger: true }}>
              <Button size="small" danger type="text">取消计划</Button>
            </Popconfirm>
          )}
          <Button size="small" type="primary" disabled={!dirty} loading={saving} onClick={() => save({ foods, note })}>保存计划</Button>
        </div>
      </div>
    </>
  );
}

function RecordView({ record, accountName, onEdit, onChanged }: {
  record: DietRecord;
  accountName: (key: string) => string;
  onEdit: () => void;
  onChanged: (keys: string[]) => void;
}) {
  const { message } = App.useApp();
  const amount = amountOf(record.amount);
  const remove = async () => {
    try {
      await dietApi.deleteRecord(record.recordKey);
      message.success("已删除");
      onChanged([]);
    } catch (error) {
      message.error((error as Error).message);
    }
  };
  return (
    <div className="diet-card">
      <div style={{ display: "flex", gap: 10, alignItems: "baseline" }}>
        <b className="pa-num" style={{ fontSize: 15 }}>{dayjs(record.eatenAt).format("HH:mm")}</b>
        {amount && <span className="pa-pill" data-tone={amount.tone}>{amount.label}</span>}
        <span style={{ flex: 1 }} />
        <Button size="small" type="text" icon={<Pencil size={14} />} onClick={onEdit} aria-label="改" />
        <Popconfirm title="删掉这条记录？" onConfirm={remove} okText="删除" okButtonProps={{ danger: true }}>
          <Button size="small" type="text" danger icon={<Trash2 size={14} />} aria-label="删除" />
        </Popconfirm>
      </div>
      <div style={{ marginTop: 6, lineHeight: 1.6 }}>{record.foods}</div>
      {record.note && <div style={{ marginTop: 4, color: "var(--pa-text-2)", fontSize: 13 }}>{record.note}</div>}
      {record.photoCount > 0 && <Photos record={record} />}
      {record.rawText && record.source !== "manual" && (
        <div className="pa-callout" data-tone="slate" style={{ marginTop: 8, fontSize: 12.5 }}>
          <Sparkles size={13} />
          <div>原话：{record.rawText}</div>
        </div>
      )}
      <div className="diet-card-foot">
        <span>{accountName(record.createdBy) || "有人"}记的</span>
        {record.updatedBy && record.updatedBy !== record.createdBy && <span>· {accountName(record.updatedBy) || "有人"}改过</span>}
      </div>
    </div>
  );
}

function Photos({ record }: { record: DietRecord }) {
  const [urls, setUrls] = useState<string[]>([]);
  useEffect(() => {
    let alive = true;
    const created: string[] = [];
    Promise.all(Array.from({ length: record.photoCount }, (_, i) =>
      fetchObjectURL(`/diet/records/${record.recordKey}/photos/${i}`).then((url) => { created.push(url); return url; }).catch(() => "")))
      .then((list) => alive && setUrls(list.filter(Boolean)));
    return () => {
      alive = false;
      created.forEach((url) => URL.revokeObjectURL(url));
    };
  }, [record.recordKey, record.photoCount]);
  if (!urls.length) return null;
  return (
    <div style={{ display: "flex", gap: 6, marginTop: 8, flexWrap: "wrap" }}>
      <Image.PreviewGroup>
        {urls.map((url) => <Image key={url} src={url} width={76} height={76} style={{ objectFit: "cover", borderRadius: 8 }} alt="" />)}
      </Image.PreviewGroup>
    </div>
  );
}

function RecordForm({ target, child, meals, record, onDone }: {
  target: CellTarget;
  child: string;
  meals: DietMeal[];
  record?: DietRecord;
  onDone: (changed: string[] | null) => void;
}) {
  const { message } = App.useApp();
  const clock = meals.find((m) => m.key === target.meal)?.clock ?? "12:00";
  const [date, setDate] = useState(record ? dayjs(record.eatenAt).format("YYYY-MM-DD") : target.date);
  const [time, setTime] = useState(record ? dayjs(record.eatenAt) : dayjs(`${target.date} ${clock}`));
  const [meal, setMeal] = useState(record?.meal ?? target.meal);
  const [foods, setFoods] = useState(record?.foods ?? "");
  const [amount, setAmount] = useState(record?.amount ?? "");
  const [note, setNote] = useState(record?.note ?? "");
  const [saving, setSaving] = useState(false);

  const submit = async () => {
    if (!foods.trim() && !note.trim()) {
      message.warning("写一下吃了什么");
      return;
    }
    setSaving(true);
    const eatenAt = dayjs(`${date} ${time.format("HH:mm")}`).toISOString();
    const body = { childKey: child, eatenAt, meal, foods, amount, note };
    try {
      if (record) await dietApi.updateRecord(record.recordKey, body);
      else await dietApi.createRecord(body);
      message.success("记好了");
      onDone([cellKey(date, meal)]);
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="diet-card" style={{ borderColor: "var(--pa-accent)", marginBottom: 10 }}>
      <div style={{ display: "flex", gap: 8, marginBottom: 12 }}>
        <TimePicker value={time} onChange={(v) => v && setTime(v)} format="HH:mm" allowClear={false} style={{ width: 100 }} needConfirm={false} />
        <Select value={meal} onChange={setMeal} style={{ flex: 1 }} options={meals.map((m) => ({ value: m.key, label: m.label }))} />
        {record && (
          <Input type="date" value={date} onChange={(e) => e.target.value && setDate(e.target.value)} style={{ width: 150 }} />
        )}
      </div>
      <div className="diet-field">
        <label>吃了什么</label>
        <Input.TextArea value={foods} onChange={(e) => setFoods(e.target.value)} autoSize={{ minRows: 2, maxRows: 5 }} autoFocus />
      </div>
      <div className="diet-field">
        <label>吃了多少</label>
        <Segmented
          block
          value={amount || "unknown"}
          onChange={(value) => setAmount(value === "unknown" ? "" : String(value))}
          options={[...amounts.map((a) => ({ value: a.value, label: a.short })), { value: "unknown", label: "不清楚" }]}
        />
      </div>
      <div className="diet-field">
        <label>情况</label>
        <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="不爱吃青菜、边看动画边吃" />
      </div>
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
        <Button onClick={() => onDone(null)}>取消</Button>
        <Button type="primary" loading={saving} onClick={submit}>{record ? "保存" : "记下"}</Button>
      </div>
    </div>
  );
}
