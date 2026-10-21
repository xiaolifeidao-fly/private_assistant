"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import dayjs from "dayjs";
import { Toast } from "antd-mobile";
import { AlertTriangle, CalendarClock, ChevronLeft, Moon, Shirt, Volume2 } from "lucide-react";
import { DayPlanSheet, PartCard, WeatherStyles } from "@/components/weather";
import { autoRead, canSpeak, speak, stopSpeaking } from "@/lib/speak";
import { deg, hm, planLine, weatherIcon, weatherToday, type Advice, type DayPlan, type WeatherToday } from "@/lib/weather";

/** 读出来的话：一句结论，再说每一段穿什么、几点前到家。 */
function spoken(advice: Advice): string {
  const lines = [advice.headline + "。"];
  for (const part of advice.parts) {
    if (part.skip) {
      lines.push(`${part.label}：${part.skipReason}。`);
      continue;
    }
    let line = `${part.label}，穿${part.clothes.join("、")}`;
    if (part.extras.length) line += `，带上${part.extras.join("、")}`;
    if (part.backBy && part.kind !== "drop") line += `，${part.directHome ? "接到直接回，" : ""}${hm(part.backBy)}前到家`;
    lines.push(line + "。");
  }
  return lines.join("");
}

/**
 * 长辈模式的「宝宝穿什么」：大字一句结论、衣服、几点前到家，点喇叭读出来。
 * 今天安排变了（晚点接、不去幼儿园）点一下改，下一次提醒就按新的算。
 */
export default function ElderWeatherPage() {
  const router = useRouter();
  const [data, setData] = useState<WeatherToday | null>(null);
  const [editing, setEditing] = useState<DayPlan | null>(null);

  const load = useCallback(async () => {
    try {
      const result = await weatherToday();
      setData(result);
      return result;
    } catch (error) {
      Toast.show({ content: (error as Error).message });
      return null;
    }
  }, []);

  useEffect(() => {
    // 从推送点进来：打开就读一遍
    void load().then((result) => {
      if (result?.advice && canSpeak() && autoRead()) speak(spoken(result.advice));
    });
    return () => stopSpeaking();
  }, [load]);

  const advice = data?.advice;
  const snapshot = data?.snapshot;
  const evening = data?.evening;
  const tomorrowPart = evening?.parts[0];
  const today = snapshot?.days?.find((day) => day.date === dayjs().format("YYYY-MM-DD"));

  return (
    <div className="el-chat" data-tone="amber">
      <header className="el-bar">
        <button className="el-back" onClick={() => router.push("/elder")} aria-label="返回">
          <ChevronLeft size={34} strokeWidth={2.6} />
          <span>返回</span>
        </button>
        <div className="el-bar-title"><Shirt size={26} strokeWidth={2.2} />穿什么</div>
        {advice && canSpeak() ? (
          <button className="el-read-toggle" data-on="true" onClick={() => speak(spoken(advice))} aria-label="读给我听">
            <Volume2 size={28} strokeWidth={2.2} />
          </button>
        ) : <span style={{ width: 88 }} />}
      </header>

      <div className="el-msgs">
        {!data && <div className="el-loading"><span className="pa-btn-spin" /> 正在打开…</div>}
        {data?.error && <div className="el-wx-head">天气暂时查不到，出门前看一眼窗外，给宝宝备一件外套。</div>}
        {advice && snapshot && (
          <>
            <div className="el-wx-head" data-alert={advice.warnings.length > 0 || advice.parts.some((part) => part.directHome || part.skip)}>
              <div className="el-wx-now">
                <span>{weatherIcon(snapshot.now.text)}</span>
                <b className="pa-num">{deg(snapshot.now.temp)}</b>
                <small>{snapshot.now.text}<br />体感 {deg(snapshot.now.feels)}{today && <><br />今天 {deg(today.low)}–{deg(today.high)}</>}</small>
              </div>
              {advice.warnings.map((warning) => (
                <div key={warning.type} className="el-wx-warning"><AlertTriangle size={24} /> {warning.type}{warning.level}预警</div>
              ))}
              <div className="el-wx-headline">{advice.headline}</div>
            </div>

            {advice.parts.map((part) => <PartCard key={part.kind + part.start} part={part} big />)}

            {evening?.night && (
              <section className="wx-part" data-big="true">
                <div className="wx-part-head"><b><Moon size={24} style={{ verticalAlign: -3 }} /> 今晚睡觉</b><span>最低 {deg(evening.night.low)}</span></div>
                <div style={{ fontSize: 21 }}>{evening.night.bedding}</div>
              </section>
            )}
            {tomorrowPart && <PartCard part={tomorrowPart} big />}
          </>
        )}

        {data && (
          <>
            <button className="el-wx-plan" onClick={() => setEditing(data.plan)}>
              <CalendarClock size={30} strokeWidth={2.2} />
              <span><b>今天的安排变了？</b><small>{planLine(data.plan)}</small></span>
            </button>
            <button className="el-wx-plan" onClick={() => setEditing(data.tomorrow)}>
              <CalendarClock size={30} strokeWidth={2.2} />
              <span><b>明天的安排</b><small>{planLine(data.tomorrow)}</small></span>
            </button>
          </>
        )}
      </div>

      <DayPlanSheet big plan={editing} open={!!editing} onClose={() => setEditing(null)} onSaved={() => void load()} />
      <WeatherStyles />
      <style jsx global>{`
        .el-wx-head { display: flex; flex-direction: column; gap: 12px; padding: 18px; border-radius: 26px; background: var(--tone-bg); font-size: var(--el-text); line-height: 1.5; }
        .el-wx-head[data-alert="true"] { background: var(--tone-rose-bg); }
        .el-wx-now { display: flex; align-items: center; gap: 14px; }
        .el-wx-now span { font-size: 56px; line-height: 1; }
        .el-wx-now b { font-size: 56px; font-weight: 800; line-height: 1; }
        .el-wx-now small { font-size: var(--el-hint); color: var(--pa-text-2); line-height: 1.4; }
        .el-wx-headline { font-size: 28px; font-weight: 800; line-height: 1.4; }
        .el-wx-warning { display: inline-flex; align-items: center; gap: 8px; align-self: flex-start; padding: 6px 14px; border-radius: 14px; background: var(--tone-rose); color: var(--pa-surface); font-size: 21px; font-weight: 800; }
        .el-wx-plan {
          appearance: none; display: flex; align-items: center; gap: 14px; width: 100%; padding: 16px 18px; border-radius: 24px;
          border: 2px solid var(--pa-line); background: var(--pa-surface); color: var(--pa-text); text-align: left; cursor: pointer;
        }
        .el-wx-plan svg { color: var(--tone-sage); flex-shrink: 0; }
        .el-wx-plan span { display: flex; flex-direction: column; gap: 4px; }
        .el-wx-plan b { font-size: 24px; font-weight: 800; }
        .el-wx-plan small { font-size: var(--el-hint); color: var(--pa-text-2); line-height: 1.45; }
        body.elder .pa-sheet .pa-input { font-size: 20px !important; min-height: 52px; }
      `}</style>
    </div>
  );
}
