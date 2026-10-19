"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import dayjs from "dayjs";
import { Toast } from "antd-mobile";
import { Camera, Check, ChevronLeft, Keyboard, Mic, Send, Utensils, Volume2, X } from "lucide-react";
import { DietPhotos, RecordSheet } from "@/components/diet";
import { HoldToTalk } from "@/components/elder/hold-to-talk";
import { api } from "@/lib/api";
import { amountLabel, captureRecord, dietOverview, today } from "@/lib/diet";
import { compressImage } from "@/lib/image";
import { canRecord } from "@/lib/recorder";
import { canSpeak, speak, stopSpeaking } from "@/lib/speak";
import type { DietMeal, DietOverview, DietRecord } from "@/lib/types";

const MAX_PHOTOS = 3;

/**
 * 长辈模式的「宝宝吃饭」：今天爸爸妈妈排的（照着做）和已经记的。
 * 一餐一张卡，排的和吃了的放在一起。照着排的吃完了点一下「照这个吃完了」；不一样就拍张照、按住说一句
 * 「中午吃了半碗饭，青菜没吃」，不用选、不用填。记错了点一下改。
 */
export default function ElderDietPage() {
  const router = useRouter();
  const [day, setDay] = useState<DietOverview | null>(null);
  const [sending, setSending] = useState(false);
  const [reply, setReply] = useState("");
  const [typing, setTyping] = useState(false);
  const [voice, setVoice] = useState(false);
  const [draft, setDraft] = useState("");
  const [photos, setPhotos] = useState<{ blob: Blob; url: string }[]>([]);
  const [editing, setEditing] = useState<{ record?: DietRecord } | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const load = useCallback(() => dietOverview(today(), 1).then(setDay).catch((error) => Toast.show({ content: (error as Error).message })), []);

  useEffect(() => {
    void load();
    const supported = canRecord();
    setVoice(supported);
    setTyping(!supported);
    return () => stopSpeaking();
  }, [load]);

  // 记完以后列表会变长，等刷新完再滚到底，让回话露出来
  useLayoutEffect(() => {
    if (reply) scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [reply, day]);

  const send = async (raw: string) => {
    const text = raw.trim();
    const picked = photos;
    if (sending || (!text && !picked.length)) return;
    stopSpeaking();
    setSending(true);
    setReply("");
    try {
      const result = await captureRecord(text, picked.map((photo) => photo.blob));
      picked.forEach((photo) => URL.revokeObjectURL(photo.url));
      setPhotos([]);
      setDraft("");
      setReply(result.reply);
      if (canSpeak()) speak(result.reply);
      await load();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setSending(false);
    }
  };

  const pickPhotos = async (files: File[]) => {
    const picked: { blob: Blob; url: string }[] = [];
    for (const file of files.slice(0, Math.max(MAX_PHOTOS - photos.length, 0))) {
      try {
        const blob = await compressImage(file);
        picked.push({ blob, url: URL.createObjectURL(blob) });
      } catch (error) {
        Toast.show({ content: (error as Error).message });
      }
    }
    setPhotos((list) => [...list, ...picked]);
  };

  const plans = day?.plans ?? [];
  const records = day?.records ?? [];
  const dayPlan = plans.find((plan) => plan.meal === "day");
  // 一餐一张卡：上面是排的，下面是吃了的；没排也没记的餐不显示
  const meals = (day?.meals ?? [])
    .map((meal) => ({ meal, plan: plans.find((plan) => plan.meal === meal.key), eaten: records.filter((record) => record.meal === meal.key) }))
    .filter((item) => item.plan || item.eaten.length);
  const now = dayjs().format("HH:mm");
  const soon = dayjs().add(1, "hour").format("HH:mm");
  // 下一顿要做的：还没过点、也还没记的第一餐
  const next = meals.find((item) => item.plan && item.meal.clock >= now && !item.eaten.length)?.meal.key;

  // 第一次打开时把下一顿滚到眼前
  const scrolledRef = useRef(false);
  useEffect(() => {
    if (!day || scrolledRef.current) return;
    scrolledRef.current = true;
    if (next) document.querySelector(`[data-meal="${next}"]`)?.scrollIntoView({ block: "center" });
  }, [day, next]);

  /** 照计划吃完了：一下就记上，不用拍照、不用说。 */
  const asPlanned = async (meal: DietMeal, foods: string) => {
    if (sending) return;
    stopSpeaking();
    setSending(true);
    setReply("");
    try {
      // 还在这一餐的时段里按现在算；到了下一餐才补记的，按这一餐的时间算
      const nextClock = day?.meals.find((item) => item.clock > meal.clock)?.clock ?? "24:00";
      const at = now < nextClock ? dayjs() : dayjs(`${today()} ${meal.clock}`);
      await api.post("/diet/records", { childKey: day?.childKey, meal: meal.key, foods, amount: "all", eatenAt: at.toISOString() });
      const text = `记好了。${meal.label}：${foods}，吃完了。`;
      setReply(text);
      if (canSpeak()) speak(text);
      await load();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="el-chat" data-tone="amber">
      <header className="el-bar">
        <button className="el-back" onClick={() => router.push("/elder")} aria-label="返回">
          <ChevronLeft size={34} strokeWidth={2.6} />
          <span>返回</span>
        </button>
        <div className="el-bar-title">
          <Utensils size={26} strokeWidth={2.2} />
          {day?.childLabel ?? "宝宝"}吃饭
        </div>
        <span style={{ width: 88 }} />
      </header>

      <div className="el-msgs" ref={scrollRef}>
        {!day && <div className="el-loading"><span className="pa-btn-spin" /> 正在打开…</div>}
        {day && (
          <>
            {dayPlan && (
              <div className="el-diet-day">
                <b>今天的要求</b>
                <span>{[dayPlan.foods, dayPlan.note].filter(Boolean).join("；")}</span>
              </div>
            )}
            {meals.length === 0 ? (
              <div className="el-diet-empty">今天爸爸妈妈还没排，按平常做就行。喂完拍张照，按住下面的按钮说一句就记上了。</div>
            ) : (
              <div className="el-diet-empty">喂完了：照着排的吃完了就点「照这个吃完了」；不一样就拍张照或者说一句。记错了点一下那条改。</div>
            )}
            {meals.map(({ meal, plan, eaten }) => (
              <section key={meal.key} className="el-diet-meal" data-meal={meal.key} data-next={meal.key === next} data-done={eaten.length > 0}>
                <div className="el-diet-meal-head">
                  <b>{meal.label}</b>
                  {meal.key === next && <em>下一顿</em>}
                  {eaten.length > 0 && <em data-kind="done">记好了</em>}
                  {plan && canSpeak() && (
                    <button className="el-diet-listen" onClick={() => speak(`${meal.label}：${plan.foods}。${plan.note ? `要求：${plan.note}` : ""}`)} aria-label="读给我听">
                      <Volume2 size={24} strokeWidth={2.2} />
                    </button>
                  )}
                </div>
                {plan && (
                  <div className="el-diet-line">
                    <span className="el-diet-tag">排的</span>
                    <span>{plan.foods}{plan.note && <small>要求：{plan.note}</small>}</span>
                  </div>
                )}
                {eaten.map((record) => (
                  <button key={record.recordKey} className="el-diet-line" data-kind="eaten" onClick={() => setEditing({ record })}>
                    <span className="el-diet-tag" data-kind="eaten">吃了</span>
                    <span>
                      {record.foods}{record.amount && <>，{amountLabel(record.amount)}</>}
                      <small>{dayjs(record.eatenAt).format("HH:mm")}{record.note && ` · ${record.note}`}</small>
                      <DietPhotos record={record} size={72} />
                    </span>
                  </button>
                ))}
                {plan?.foods && !eaten.length && meal.clock <= soon && (
                  <button className="el-diet-done" onClick={() => void asPlanned(meal, plan.foods)} disabled={sending}>
                    <Check size={26} strokeWidth={2.6} /> 照这个吃完了
                  </button>
                )}
              </section>
            ))}
            {reply && <div className="el-msg" data-role="assistant"><div className="el-bubble">{reply}</div></div>}
            {sending && <div className="el-msg" data-role="assistant"><div className="el-bubble"><span className="el-thinking"><span className="pa-btn-spin" />正在记…</span></div></div>}
          </>
        )}
      </div>

      <footer className="el-input">
        {photos.length > 0 && (
          <div className="el-picked">
            {photos.map((photo, index) => (
              <div key={photo.url} className="el-picked-item">
                <img src={photo.url} alt="" />
                <button onClick={() => setPhotos((list) => list.filter((_, i) => i !== index))} aria-label="不要这张"><X size={20} strokeWidth={2.6} /></button>
              </div>
            ))}
            <span className="el-picked-hint">再说一下吃了多少</span>
          </div>
        )}
        {typing && (
          <textarea className="el-textarea" value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="比如：中午吃了半碗饭，青菜没吃" rows={2} />
        )}
        <div className="el-input-row">
          <button className="el-side-btn" onClick={() => fileRef.current?.click()} disabled={sending || photos.length >= MAX_PHOTOS} aria-label="拍照">
            <Camera size={28} strokeWidth={2.1} />
            <span>拍照</span>
          </button>
          {typing ? (
            <button className="el-send" onClick={() => void send(draft)} disabled={sending || (!draft.trim() && !photos.length)}>
              <Send size={28} strokeWidth={2.3} />
              <span>记下</span>
            </button>
          ) : (
            <HoldToTalk onText={(text) => void send(text)} disabled={sending || !day} />
          )}
          {voice && (
            <button className="el-side-btn" onClick={() => setTyping(!typing)} aria-label={typing ? "改成说话" : "改成打字"}>
              {typing ? <Mic size={28} strokeWidth={2.1} /> : <Keyboard size={28} strokeWidth={2.1} />}
              <span>{typing ? "说话" : "打字"}</span>
            </button>
          )}
        </div>
        {!typing && photos.length > 0 && (
          <button className="el-photo-only" onClick={() => void send("")} disabled={sending}>不说了，直接记照片</button>
        )}
      </footer>
      <input ref={fileRef} type="file" accept="image/*" capture="environment" hidden
        onChange={(event) => {
          const files = Array.from(event.target.files ?? []);
          event.target.value = "";
          if (files.length) void pickPhotos(files);
        }} />

      {day && <RecordSheet big target={editing} meals={day.meals} childKey={day.childKey} onClose={() => setEditing(null)} onSaved={() => void load()} />}
    </div>
  );
}
