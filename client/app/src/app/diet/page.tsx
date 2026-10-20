"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import dayjs from "dayjs";
import { Dialog, TextArea, Toast } from "antd-mobile";
import { Camera, ChevronLeft, ChevronRight, ClipboardCheck, Copy, Mic, Plus, Send, Sparkles, X } from "lucide-react";
import { AuthGate } from "@/components/auth-gate";
import { DietPhotos, PlanSheet, RecordSheet } from "@/components/diet";
import { PickedPhotos } from "@/components/photos";
import { RichText } from "@/components/rich-text";
import { Button, LoadError, Loading, NavBar, Pill, Segmented, Tip } from "@/components/ui";
import { useDictation, VoiceButton } from "@/components/voice-input";
import { isSendKey } from "@/lib/keys";
import { api } from "@/lib/api";
import { amountLabel, capturePlan, captureRecord, dietOverview, mealLabels, today, type PlanTurn } from "@/lib/diet";
import { compressImage } from "@/lib/image";
import { useMe } from "@/lib/role";
import type { DietOverview, DietPlan, DietRecord, DietReview } from "@/lib/types";

const MAX_PHOTOS = 4;
const weekdays = ["日", "一", "二", "三", "四", "五", "六"];

export default function DietPage() {
  return <AuthGate><DietBoard /></AuthGate>;
}

/** 宝宝吃饭（我和老婆用）：一周的计划和实际对照着看，计划、实际都能说一句 / 拍张照交给模型整理。 */
function DietBoard() {
  const router = useRouter();
  const { role, ready } = useMe();
  const [day, setDay] = useState(today());
  const [week, setWeek] = useState<DietOverview | null>(null);
  const [error, setError] = useState("");
  const [planTarget, setPlanTarget] = useState<{ date: string; meal: string; plan?: DietPlan } | null>(null);
  const [recordTarget, setRecordTarget] = useState<{ record?: DietRecord; meal?: string; date?: string } | null>(null);
  const [mode, setMode] = useState<"record" | "plan">("record");
  const [text, setText] = useState("");
  const [photos, setPhotos] = useState<{ blob: Blob; url: string }[]>([]);
  const [sending, setSending] = useState(false);
  const dictation = useDictation({ value: text, onChange: setText, disabled: sending });
  // 两边各自一段对话；改计划时把前面几轮带给模型，能接着说「再把周五也这样」
  const [threads, setThreads] = useState<Record<"record" | "plan", PlanTurn[]>>({ record: [], plan: [] });
  const thread = threads[mode];
  const threadEnd = useRef<HTMLDivElement>(null);
  useEffect(() => {
    threadEnd.current?.scrollIntoView({ block: "end" });
  }, [thread.length, sending]);
  const [review, setReview] = useState<DietReview | null>(null);
  const [reviewing, setReviewing] = useState(false);

  useEffect(() => {
    if (ready && role === "elder") router.replace("/elder/diet");
  }, [ready, role, router]);

  // 一周从周一算
  const weekStart = useMemo(() => {
    const date = dayjs(day);
    return date.subtract((date.day() + 6) % 7, "day").format("YYYY-MM-DD");
  }, [day]);

  const load = useCallback(() => {
    setError("");
    api.get<{ review: DietReview | null }>(`/diet/review?date=${weekStart}`).then((r) => setReview(r.review)).catch(() => setReview(null));
    return dietOverview(weekStart, 7)
      .then(setWeek)
      .catch((err) => setError((err as Error).message));
  }, [weekStart]);

  // 这周的计划对照实际：周日晚上自动生成并推送，也可以现在就让模型看一遍
  const makeReview = async () => {
    setReviewing(true);
    try {
      setReview(await api.post<DietReview>("/diet/review", { date: weekStart }));
    } catch (err) {
      Toast.show({ content: (err as Error).message });
    } finally {
      setReviewing(false);
    }
  };
  useEffect(() => {
    void load();
  }, [load]);

  const canPlan = !!week?.canPlan;
  useEffect(() => {
    if (!canPlan) setMode("record");
  }, [canPlan]);

  const plans = week?.plans.filter((plan) => plan.date === day) ?? [];
  const records = week?.records.filter((record) => dayjs(record.eatenAt).format("YYYY-MM-DD") === day) ?? [];
  const dayPlan = plans.find((plan) => plan.meal === "day");

  const pick = async (files: File[]) => {
    const picked: { blob: Blob; url: string }[] = [];
    for (const file of files.slice(0, Math.max(MAX_PHOTOS - photos.length, 0))) {
      try {
        const blob = await compressImage(file);
        picked.push({ blob, url: URL.createObjectURL(blob) });
      } catch (err) {
        Toast.show({ content: (err as Error).message });
      }
    }
    setPhotos((list) => [...list, ...picked]);
  };

  const send = async (raw = text) => {
    const spoken = raw.trim();
    if (sending || (!spoken && !photos.length)) return;
    setSending(true);
    const asked: PlanTurn = { role: "parent", text: spoken || `（拍了 ${photos.length} 张照片）` };
    const history = threads[mode];
    setThreads((all) => ({ ...all, [mode]: [...all[mode], asked] }));
    try {
      const blobs = photos.map((photo) => photo.blob);
      const result = mode === "plan" ? await capturePlan(spoken, blobs, day, 7, "", history) : await captureRecord(spoken, blobs);
      setThreads((all) => ({ ...all, [mode]: [...all[mode], { role: "assistant", text: result.reply }] }));
      setText("");
      photos.forEach((photo) => URL.revokeObjectURL(photo.url));
      setPhotos([]);
      // 记的是别的日子（比如改了下周的计划），跳过去看
      if (mode === "record") {
        const first = (result as { records: DietRecord[] }).records[0];
        if (first) setDay(dayjs(first.eatenAt).format("YYYY-MM-DD"));
      }
      await load();
    } catch (err) {
      // 没发成：撤回这一句，话留在输入框里
      setThreads((all) => ({ ...all, [mode]: all[mode].filter((turn) => turn !== asked) }));
      Toast.show({ content: (err as Error).message });
    } finally {
      setSending(false);
    }
  };

  const copyLastWeek = async () => {
    const ok = await Dialog.confirm({ content: "把上周的计划照搬到这周？这周已经排了的餐会被覆盖。", confirmText: "照搬", cancelText: "算了" });
    if (!ok) return;
    try {
      const { count } = await api.post<{ count: number }>("/diet/plans/copy", {
        from: dayjs(weekStart).subtract(7, "day").format("YYYY-MM-DD"), to: weekStart, days: 7,
      });
      Toast.show({ content: count ? `抄了 ${count} 餐` : "上周没有计划" });
      await load();
    } catch (err) {
      Toast.show({ content: (err as Error).message });
    }
  };

  const days = Array.from({ length: 7 }, (_, index) => dayjs(weekStart).add(index, "day"));

  return (
    <div className="pa-screen pa-page">
      <div className="pa-scroll" style={{ paddingBottom: "calc(190px + env(safe-area-inset-bottom))" }}>
        <NavBar onBack={() => router.back()} title={`${week?.childLabel ?? "宝宝"}吃饭`} subtitle="计划和实际对照着看" />

        <div className="diet-week">
          <button className="pa-icon-btn" onClick={() => setDay(dayjs(day).subtract(7, "day").format("YYYY-MM-DD"))} aria-label="上一周"><ChevronLeft size={20} /></button>
          {days.map((date) => {
            const key = date.format("YYYY-MM-DD");
            const planned = week?.plans.some((plan) => plan.date === key);
            const eaten = week?.records.some((record) => dayjs(record.eatenAt).format("YYYY-MM-DD") === key);
            return (
              <button key={key} className="diet-day" data-active={key === day} data-today={key === today()} onClick={() => setDay(key)}>
                <small>{key === today() ? "今天" : `周${weekdays[date.day()]}`}</small>
                <b className="pa-num">{date.date()}</b>
                <span className="diet-dots">
                  <i data-on={planned} data-kind="plan" />
                  <i data-on={eaten} data-kind="eat" />
                </span>
              </button>
            );
          })}
          <button className="pa-icon-btn" onClick={() => setDay(dayjs(day).add(7, "day").format("YYYY-MM-DD"))} aria-label="下一周"><ChevronRight size={20} /></button>
        </div>

        <Tip id="diet-board" title="计划和实际都可以说一句、拍张照" style={{ marginTop: 12 }}>
          下面选「记实际」或「改计划」再说。比如「中午吃了半碗面条，青菜没吃」「下周每天早上一个鸡蛋，周三晚饭吃鱼」「按她最近吃的排一下下周」，幼儿园食谱拍照也行。奶奶那边在长辈模式里拍照、说一句就能记。
        </Tip>

        {!week ? (error ? <LoadError message={error} onRetry={() => void load()} /> : <Loading />) : (
          <div className="pa-enter">
            {(dayPlan || canPlan) && (
              <button className="diet-dayplan" onClick={() => canPlan && setPlanTarget({ date: day, meal: "day", plan: dayPlan })} disabled={!canPlan}>
                <span className="diet-tag" data-kind="plan">全天要求</span>
                <span className="diet-dayplan-text">{dayPlan ? [dayPlan.foods, dayPlan.note].filter(Boolean).join("；") : "点这里写这一天的总体要求"}</span>
              </button>
            )}

            {week.meals.map((meal) => {
              const plan = plans.find((item) => item.meal === meal.key);
              const eaten = records.filter((record) => record.meal === meal.key);
              if (!plan && !eaten.length && !canPlan && day > today()) return null;
              return (
                <section key={meal.key} className="diet-meal">
                  <div className="diet-meal-head">
                    <b>{meal.label}</b>
                    <small className="pa-num">{meal.clock}</small>
                  </div>
                  <button className="diet-plan" onClick={() => canPlan && setPlanTarget({ date: day, meal: meal.key, plan })} disabled={!canPlan} data-empty={!plan}>
                    <span className="diet-tag" data-kind="plan">计划</span>
                    <span className="diet-plan-text">
                      {plan ? plan.foods || "（没写吃什么）" : canPlan ? "还没计划，点一下写" : "没计划"}
                      {plan?.note && <small>要求：{plan.note}</small>}
                    </span>
                  </button>
                  {eaten.map((record) => (
                    <button key={record.recordKey} className="diet-record" onClick={() => setRecordTarget({ record })}>
                      <span className="diet-tag" data-kind="eat">实际</span>
                      <span className="diet-plan-text">
                        <span>{record.foods}{record.amount && <Pill tone={record.amount === "none" || record.amount === "little" ? "rose" : "sage"}>{amountLabel(record.amount)}</Pill>}</span>
                        <small>
                          {dayjs(record.eatenAt).format("HH:mm")}
                          {record.source === "capture" && <> · <Sparkles size={11} style={{ verticalAlign: -1 }} /> 自动整理</>}
                          {record.note && <> · {record.note}</>}
                        </small>
                        <DietPhotos record={record} size={48} />
                      </span>
                    </button>
                  ))}
                  {day <= today() && (
                    <button className="diet-add" onClick={() => setRecordTarget({ meal: meal.key, date: day })}>
                      <Plus size={14} strokeWidth={2.4} /> 记实际吃了什么
                    </button>
                  )}
                </section>
              );
            })}

            {(review || canPlan) && (
              <section className="diet-meal diet-review">
                <div className="diet-meal-head">
                  <b><ClipboardCheck size={16} style={{ verticalAlign: -2, marginRight: 4 }} />这周对照</b>
                  {review && <small>{dayjs(review.updatedAt).format("M月D日 HH:mm")}</small>}
                </div>
                {review ? (
                  <div style={{ marginTop: 8, fontSize: 14.5 }}><RichText text={review.content} /></div>
                ) : (
                  <div style={{ marginTop: 6, fontSize: 13.5, color: "var(--pa-text-3)", lineHeight: 1.6 }}>
                    每周日晚上会把这一周的计划和实际交给助理对照一遍，推到你手机上。也可以现在就看。
                  </div>
                )}
                {canPlan && (
                  <div style={{ marginTop: 10 }}>
                    <Button size="sm" variant="soft" loading={reviewing} onClick={() => void makeReview()}>{review ? "重新对照" : "现在对照一下"}</Button>
                  </div>
                )}
              </section>
            )}

            {canPlan && (
              <div style={{ padding: "18px 16px 0", display: "flex", justifyContent: "center" }}>
                <button className="diet-add" onClick={() => void copyLastWeek()}><Copy size={14} /> 把上周的计划抄到这周</button>
              </div>
            )}
          </div>
        )}
      </div>

      <footer className="diet-composer">
        {thread.length > 0 && (
          <div className="diet-thread">
            <button className="diet-thread-clear" onClick={() => setThreads((all) => ({ ...all, [mode]: [] }))} aria-label="重新开始">
              <X size={13} /> 重新开始
            </button>
            {thread.map((turn, index) => (
              <div key={index} className="diet-bubble" data-role={turn.role}>{turn.text}</div>
            ))}
            {sending && <div className="diet-bubble" data-role="assistant"><span className="pa-btn-spin" /></div>}
            <div ref={threadEnd} />
          </div>
        )}
        {canPlan && (
          <Segmented options={[{ value: "record", label: "记实际" }, { value: "plan", label: "改计划" }]} value={mode} onChange={(value) => setMode(value as "record" | "plan")} />
        )}
        <PickedPhotos urls={photos.map((photo) => photo.url)} onRemove={(index) => setPhotos((list) => {
          URL.revokeObjectURL(list[index].url);
          return list.filter((_, i) => i !== index);
        })} />
        <div className="diet-input">
          <label className="diet-tool" data-disabled={photos.length >= MAX_PHOTOS || sending} aria-label="拍照">
            <Camera size={20} />
            <input type="file" accept="image/*" hidden disabled={photos.length >= MAX_PHOTOS || sending}
              onChange={(event) => { void pick(Array.from(event.target.files ?? [])); event.target.value = ""; }} />
          </label>
          <div className="pa-textarea-wrap diet-text" {...dictation.holdProps}>
            <TextArea value={text} onChange={setText} rows={1} autoSize={{ minRows: 1, maxRows: 4 }}
              readOnly={dictation.busy}
              onKeyDown={(event) => { if (isSendKey(event) && !dictation.busy) { event.preventDefault(); void send(); } }}
              placeholder={mode === "plan" ? "下周每天早上一个鸡蛋，周三晚饭吃鱼…" : "中午吃了半碗面条，青菜没吃…"} />
          </div>
          <VoiceButton className="diet-tool" dictation={dictation} disabled={sending}>
            <Mic size={20} />
          </VoiceButton>
          {dictation.overlay}
          <button className="diet-send" onClick={() => void send()} disabled={sending || dictation.busy || (!text.trim() && !photos.length)} aria-label="发送">
            {sending ? <span className="pa-btn-spin" /> : <Send size={18} />}
          </button>
        </div>
      </footer>

      {week && (
        <>
          <PlanSheet target={planTarget} childKey={week.childKey} onClose={() => setPlanTarget(null)} onSaved={() => void load()} />
          <RecordSheet target={recordTarget} meals={week.meals} childKey={week.childKey} onClose={() => setRecordTarget(null)} onSaved={() => void load()} />
        </>
      )}

      <style jsx global>{`
        .diet-week { display: flex; align-items: center; gap: 2px; padding: 4px 8px 0; }
        .diet-day {
          appearance: none; flex: 1; min-width: 0; border: 0; background: transparent; border-radius: 14px; padding: 6px 0;
          display: flex; flex-direction: column; align-items: center; gap: 2px; color: var(--pa-text); cursor: pointer;
        }
        .diet-day small { font-size: 11px; color: var(--pa-text-3); }
        .diet-day b { font-size: 17px; font-weight: 650; }
        .diet-day[data-active="true"] { background: var(--pa-accent); color: var(--pa-accent-ink); }
        .diet-day[data-active="true"] small { color: inherit; opacity: 0.8; }
        .diet-dots { display: flex; gap: 3px; height: 5px; }
        .diet-dots i { width: 5px; height: 5px; border-radius: 3px; background: transparent; }
        .diet-dots i[data-on="true"][data-kind="plan"] { background: var(--tone-sky); }
        .diet-dots i[data-on="true"][data-kind="eat"] { background: var(--tone-sage); }
        .diet-dayplan, .diet-plan, .diet-record {
          appearance: none; display: flex; gap: 10px; align-items: flex-start; width: 100%; text-align: left;
          border: 0; background: transparent; color: var(--pa-text); padding: 8px 0; cursor: pointer; font-size: 15px; line-height: 1.55;
        }
        .diet-dayplan { width: calc(100% - 32px); margin: 14px 16px 0; padding: 12px 14px; border-radius: var(--pa-radius); background: var(--pa-surface); border: 0.5px solid var(--pa-line); }
        .diet-dayplan:disabled, .diet-plan:disabled { cursor: default; opacity: 1; }
        .diet-dayplan-text { flex: 1; color: var(--pa-text-2); }
        .diet-meal { margin: 12px 16px 0; padding: 12px 14px 8px; border-radius: var(--pa-radius); background: var(--pa-surface); border: 0.5px solid var(--pa-line); box-shadow: var(--pa-shadow-sm); }
        .diet-meal-head { display: flex; align-items: baseline; gap: 8px; }
        .diet-meal-head b { font-size: 16.5px; font-weight: 700; }
        .diet-meal-head small { font-size: 12px; color: var(--pa-text-3); }
        .diet-plan[data-empty="true"] .diet-plan-text { color: var(--pa-text-3); }
        .diet-record { border-top: 0.5px solid var(--pa-line); }
        .diet-plan-text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; word-break: break-word; }
        .diet-plan-text > span { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 6px; }
        .diet-plan-text small { font-size: 12.5px; color: var(--pa-text-3); }
        .diet-tag { flex-shrink: 0; font-size: 11.5px; font-weight: 650; padding: 2px 7px; border-radius: 8px; margin-top: 2px; }
        .diet-tag[data-kind="plan"] { background: var(--tone-sky-bg); color: var(--tone-sky); }
        .diet-tag[data-kind="eat"] { background: var(--tone-sage-bg); color: var(--tone-sage); }
        .diet-add {
          appearance: none; border: 0; background: transparent; color: var(--pa-text-3); font-size: 13px; padding: 6px 0;
          display: inline-flex; align-items: center; gap: 4px; cursor: pointer;
        }
        .diet-composer {
          position: fixed; left: 0; right: 0; bottom: 0; z-index: 20; padding: 10px 12px calc(10px + env(safe-area-inset-bottom));
          background: color-mix(in srgb, var(--pa-bg) 92%, transparent); backdrop-filter: blur(16px); -webkit-backdrop-filter: blur(16px);
          border-top: 0.5px solid var(--pa-line); display: flex; flex-direction: column; gap: 8px;
        }
        .diet-thread { max-height: 34vh; overflow-y: auto; display: flex; flex-direction: column; gap: 6px; position: relative; }
        .diet-thread-clear {
          appearance: none; align-self: center; border: 0; background: transparent; color: var(--pa-text-3); font-size: 12px;
          display: inline-flex; align-items: center; gap: 2px; padding: 0 0 2px; cursor: pointer;
        }
        .diet-bubble { max-width: 86%; font-size: 14px; line-height: 1.55; padding: 8px 12px; border-radius: 14px; white-space: pre-wrap; word-break: break-word; }
        .diet-bubble[data-role="assistant"] { align-self: flex-start; background: var(--pa-surface); border: 0.5px solid var(--pa-line); }
        .diet-bubble[data-role="parent"] { align-self: flex-end; background: var(--pa-accent); color: var(--pa-accent-ink); }
        .diet-input { display: flex; align-items: flex-end; gap: 8px; }
        .diet-text { flex: 1; min-width: 0; padding-top: 8px; padding-bottom: 8px; }
        .diet-tool, .diet-send {
          appearance: none; flex-shrink: 0; width: 42px; height: 42px; border-radius: 21px; border: 0.5px solid var(--pa-line);
          background: var(--pa-surface); color: var(--pa-text-2); display: grid; place-items: center; cursor: pointer;
        }
        .diet-tool[data-disabled="true"] { opacity: 0.4; pointer-events: none; }
        .diet-send { background: var(--pa-accent); color: var(--pa-accent-ink); border: 0; }
        .diet-send:disabled { opacity: 0.4; }
      `}</style>
    </div>
  );
}
