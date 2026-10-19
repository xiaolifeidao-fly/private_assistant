"use client";

import { useEffect, useState } from "react";
import dayjs from "dayjs";
import { Dialog, ImageViewer, TextArea, Toast } from "antd-mobile";
import { api, fetchObjectURL } from "@/lib/api";
import { amountOptions, mealLabels } from "@/lib/diet";
import type { DietMeal, DietPlan, DietRecord } from "@/lib/types";
import { Button, Chips, Sheet, useLatest } from "./ui";

/** 一条记录的照片：带令牌取回来再显示，点开看大图。 */
export function DietPhotos({ record, size = 56 }: { record: DietRecord; size?: number }) {
  const [urls, setUrls] = useState<string[]>([]);
  const [preview, setPreview] = useState<number | null>(null);
  useEffect(() => {
    if (!record.photoCount) return;
    let alive = true;
    const created: string[] = [];
    Promise.all(Array.from({ length: record.photoCount }, (_, index) => fetchObjectURL(`/diet/records/${record.recordKey}/photos/${index}`).catch(() => "")))
      .then((list) => {
        created.push(...list.filter(Boolean));
        if (alive) setUrls(list.filter(Boolean));
      });
    return () => {
      alive = false;
      created.forEach((url) => URL.revokeObjectURL(url));
    };
  }, [record.recordKey, record.photoCount]);
  if (!urls.length) return null;
  return (
    <div style={{ display: "flex", gap: 6, marginTop: 8 }}>
      {urls.map((url, index) => (
        <img key={url} src={url} alt="" onClick={(event) => { event.stopPropagation(); setPreview(index); }}
          style={{ width: size, height: size, borderRadius: 10, objectFit: "cover", background: "var(--pa-fill)" }} />
      ))}
      <ImageViewer.Multi getContainer={() => document.body} images={urls} visible={preview !== null} defaultIndex={preview ?? 0} onClose={() => setPreview(null)} />
    </div>
  );
}

/** 改一餐的计划（爸爸妈妈）。两项都清空就是取消这一餐的计划。 */
export function PlanSheet({ target, childKey, onClose, onSaved }: {
  target: { date: string; meal: string; plan?: DietPlan } | null;
  childKey: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const item = useLatest(target);
  const [foods, setFoods] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setFoods(target?.plan?.foods ?? "");
    setNote(target?.plan?.note ?? "");
  }, [target]);

  const save = async (clear = false) => {
    if (!item) return;
    setBusy(true);
    try {
      await api.put("/diet/plans", { childKey, date: item.date, meal: item.meal, foods: clear ? "" : foods, note: clear ? "" : note });
      Toast.show({ content: clear ? "这一餐的计划取消了" : "计划存好了" });
      onSaved();
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Sheet open={!!target} onClose={onClose} title={item ? `${dayjs(item.date).format("M月D日")} ${mealLabels[item.meal] ?? item.meal} · 计划` : "计划"}>
      {item && (
        <div style={{ paddingBottom: 8 }}>
          <div className="pa-field" style={{ marginTop: 8 }}>
            <div className="pa-label">{item.meal === "day" ? "这一天的要求" : "打算吃什么"}</div>
            <div className="pa-textarea-wrap">
              <TextArea value={foods} onChange={setFoods} rows={2} autoSize={{ minRows: 2, maxRows: 6 }}
                placeholder={item.meal === "day" ? "一天一杯奶、多喝水、不吃糖……" : "小米粥、鸡蛋羹、苹果半个……"} />
            </div>
          </div>
          <div className="pa-field">
            <div className="pa-label">要求、注意<small>可不填</small></div>
            <div className="pa-textarea-wrap">
              <TextArea value={note} onChange={setNote} rows={1} autoSize={{ minRows: 1, maxRows: 4 }} placeholder="少盐、蒸软一点、要吃完……" />
            </div>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 10, padding: "22px 20px 0" }}>
            <Button block variant="ink" loading={busy} onClick={() => void save()} disabled={!foods.trim() && !note.trim()}>保存</Button>
            {item.plan && <Button block variant="danger" onClick={() => void save(true)} disabled={busy}>取消这一餐的计划</Button>}
          </div>
        </div>
      )}
    </Sheet>
  );
}

/** 记一条 / 改一条实际吃了什么（谁都能用）。 */
export function RecordSheet({ target, meals, childKey, onClose, onSaved, big }: {
  target: { record?: DietRecord; meal?: string; date?: string } | null;
  meals: DietMeal[];
  childKey: string;
  onClose: () => void;
  onSaved: () => void;
  /** 长辈模式：字大一点 */
  big?: boolean;
}) {
  const item = useLatest(target);
  const [meal, setMeal] = useState("");
  const [foods, setFoods] = useState("");
  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setMeal(target?.record?.meal ?? target?.meal ?? "");
    setFoods(target?.record?.foods ?? "");
    setAmount(target?.record?.amount ?? "");
    setNote(target?.record?.note ?? "");
  }, [target]);

  const save = async () => {
    if (!item) return;
    setBusy(true);
    try {
      if (item.record) {
        await api.put(`/diet/records/${item.record.recordKey}`, { meal, foods, amount, note, eatenAt: item.record.eatenAt });
      } else {
        const meta = meals.find((m) => m.key === meal);
        // 补记以前某天的：按那一餐的大概时间算
        const day = item.date && item.date !== dayjs().format("YYYY-MM-DD") ? dayjs(`${item.date} ${meta?.clock ?? "12:00"}`) : dayjs();
        await api.post("/diet/records", { childKey, meal, foods, amount, note, eatenAt: day.toISOString() });
      }
      Toast.show({ content: "记好了" });
      onSaved();
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!item?.record) return;
    const ok = await Dialog.confirm({ content: "删掉这条记录？", confirmText: "删掉", cancelText: "不删" });
    if (!ok) return;
    setBusy(true);
    try {
      await api.del(`/diet/records/${item.record.recordKey}`);
      Toast.show({ content: "删掉了" });
      onSaved();
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Sheet open={!!target} onClose={onClose} title={item?.record ? "改一下" : "记一条"}>
      {item && (
        <div style={{ paddingBottom: 8, fontSize: big ? 18 : undefined }}>
          {item.record?.rawText && (
            <div style={{ margin: "4px 20px 0", padding: "10px 12px", borderRadius: 12, background: "var(--pa-fill)", fontSize: big ? 19 : 13.5, color: "var(--pa-text-2)", lineHeight: 1.6 }}>
              当时说的：{item.record.rawText}
            </div>
          )}
          <div className="pa-field" style={{ marginTop: 14 }}>
            <div className="pa-label">哪一餐</div>
            <Chips options={meals.map((m) => ({ value: m.key, label: m.label }))} value={meal ? [meal] : []} onChange={(value) => setMeal(value[0] ?? "")} />
          </div>
          <div className="pa-field">
            <div className="pa-label">吃了什么</div>
            <div className="pa-textarea-wrap">
              <TextArea value={foods} onChange={setFoods} rows={2} autoSize={{ minRows: 2, maxRows: 6 }} placeholder="米饭、番茄炒蛋、青菜……" />
            </div>
          </div>
          <div className="pa-field">
            <div className="pa-label">吃了多少</div>
            <Chips options={amountOptions} value={amount ? [amount] : []} onChange={(value) => setAmount(value[0] === amount ? "" : value[0] ?? "")} />
          </div>
          <div className="pa-field">
            <div className="pa-label">情况<small>可不填</small></div>
            <div className="pa-textarea-wrap">
              <TextArea value={note} onChange={setNote} rows={1} autoSize={{ minRows: 1, maxRows: 4 }} placeholder="不肯吃青菜、加了一块饼干……" />
            </div>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 10, padding: "22px 20px 0" }}>
            <Button block size={big ? "lg" : undefined} variant="ink" loading={busy} onClick={() => void save()} disabled={!foods.trim() && !note.trim()}>保存</Button>
            {item.record && <Button block size={big ? "lg" : undefined} variant="danger" onClick={() => void remove()} disabled={busy}>删掉这条</Button>}
          </div>
        </div>
      )}
    </Sheet>
  );
}
