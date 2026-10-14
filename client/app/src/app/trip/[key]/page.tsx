"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import dayjs from "dayjs";
import { Dialog, Toast } from "antd-mobile";
import { MapPin, Plane } from "lucide-react";
import { AuthGate } from "@/components/auth-gate";
import { RichText } from "@/components/rich-text";
import { Button, Card, LoadError, Loading, NavBar, Section } from "@/components/ui";
import { api } from "@/lib/api";
import { usePlaces } from "@/lib/family";
import { upcomingDay } from "@/lib/time";
import type { Trip } from "@/lib/types";

export default function TripPage() {
  return <AuthGate><TripDetail /></AuthGate>;
}

function TripDetail() {
  const { key } = useParams<{ key: string }>();
  const router = useRouter();
  const places = usePlaces();
  const [trip, setTrip] = useState<Trip | null>(null);
  const [error, setError] = useState("");
  const load = useCallback(() => {
    setError("");
    api.get<Trip>(`/trips/${key}`).then(setTrip).catch((err) => setError((err as Error).message));
  }, [key]);
  useEffect(load, [load]);

  const cancel = async () => {
    if (!await Dialog.confirm({ title: "取消这次行程？", content: "对应的提醒也会一起取消。", confirmText: "取消行程", cancelText: "再想想" })) return;
    try {
      await api.post(`/trips/${key}/cancel`);
      Toast.show({ content: "已取消" });
      router.replace("/today");
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    }
  };

  const day = trip ? upcomingDay(trip.startAt) : null;
  const place = trip ? places.find((p) => p.placeKey === trip.placeKey) : undefined;

  return (
    <div className="pa-screen pa-page">
      <div className="pa-scroll" style={{ paddingBottom: "calc(32px + env(safe-area-inset-bottom))" }}>
        <NavBar onBack={() => router.back()} title="行前简报" />
        {!trip ? (error ? <LoadError message={error} onRetry={load} /> : <Loading />) : (
          <div className="pa-enter">
            <div className="trip-hero">
              <div className="trip-hero-icon"><Plane size={22} /></div>
              <h1 className="trip-title">{trip.title}</h1>
              <div className="trip-meta">
                {place && <span><MapPin size={13} strokeWidth={2.4} />{place.name}</span>}
                <span>{dayjs(trip.startAt).format("YYYY年M月D日 HH:mm")} 出发</span>
              </div>
              {day && day.days >= 0 && trip.status === "planned" && (
                <div className="trip-countdown">{day.days > 0 ? <><b>{day.days}</b> 天后出发</> : <b>今天出发</b>}</div>
              )}
            </div>

            <Section title="简报">
              <Card pad>
                <RichText text={trip.brief || "还没生成简报，出发前两天会自动生成。"} />
              </Card>
            </Section>

            {trip.status === "planned" && (
              <div style={{ padding: "28px 20px 0" }}>
                <Button block variant="danger" size="md" onClick={cancel}>取消行程</Button>
              </div>
            )}
          </div>
        )}
      </div>
      <style jsx global>{`
        .trip-hero {
          margin: 12px 16px 0;
          padding: 20px;
          border-radius: var(--pa-radius-lg);
          background:
            radial-gradient(100% 90% at 100% 0%, color-mix(in srgb, var(--tone-sky) 22%, transparent), transparent 65%),
            linear-gradient(160deg, var(--tone-sky-bg), var(--pa-surface));
          border: 0.5px solid var(--pa-line);
          box-shadow: var(--pa-shadow);
        }
        .trip-hero-icon { width: 44px; height: 44px; border-radius: 14px; background: var(--pa-surface); color: var(--tone-sky); display: grid; place-items: center; box-shadow: var(--pa-shadow-sm); }
        .trip-title { font-family: var(--pa-font-display); font-size: 24px; font-weight: 700; margin: 16px 0 0; line-height: 1.3; }
        .trip-meta { display: flex; flex-wrap: wrap; gap: 4px 12px; margin-top: 8px; font-size: 13.5px; color: var(--pa-text-2); }
        .trip-meta span { display: inline-flex; align-items: center; gap: 4px; }
        .trip-countdown { margin-top: 16px; padding-top: 14px; border-top: 0.5px dashed var(--pa-line-strong); font-size: 13.5px; color: var(--pa-text-2); }
        .trip-countdown b { font-size: 26px; font-weight: 700; color: var(--tone-sky); font-variant-numeric: tabular-nums; margin-right: 2px; }
      `}</style>
    </div>
  );
}
