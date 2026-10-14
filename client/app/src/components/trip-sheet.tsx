"use client";

import { useEffect, useState } from "react";
import dayjs from "dayjs";
import { TextArea, Toast } from "antd-mobile";
import { api } from "@/lib/api";
import { personLabel, usePersons, usePlaces } from "@/lib/family";
import { personTone } from "@/lib/tone";
import type { Trip } from "@/lib/types";
import { Button, Chips, Sheet } from "./ui";

const defaultStart = () => dayjs().add(7, "day").hour(9).minute(0).format("YYYY-MM-DDTHH:mm");

export function TripSheet({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (trip: Trip) => void }) {
  const places = usePlaces();
  const persons = usePersons();
  const [place, setPlace] = useState<string[]>([]);
  const [title, setTitle] = useState("");
  const [startAt, setStartAt] = useState(defaultStart);
  const [endAt, setEndAt] = useState("");
  const [notes, setNotes] = useState("");
  const [people, setPeople] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open && !place.length && places.length) setPlace([places.find((item) => item.kind === "hometown")?.placeKey ?? places[0].placeKey]);
  }, [open, place.length, places]);

  const save = async () => {
    if (!place[0] || !startAt) {
      Toast.show({ content: "请选目的地和出发时间" });
      return;
    }
    setSaving(true);
    try {
      const trip = await api.post<Trip>("/trips", {
        placeKey: place[0], title, startAt: dayjs(startAt).toISOString(), endAt: endAt ? dayjs(endAt).toISOString() : null, notes, people,
      });
      Toast.show({ icon: "success", content: "建好了，出发前两天会提醒" });
      // 清空表单，下次再建行程不会带着这次的内容
      setTitle("");
      setStartAt(defaultStart());
      setEndAt("");
      setNotes("");
      setPeople([]);
      onCreated(trip);
      onClose();
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Sheet open={open} onClose={onClose} title="回老家行程" height="88vh">
      <div style={{ padding: "0 20px", fontSize: 13, color: "var(--pa-text-3)" }}>出发前助理会整理一份简报：见谁、怎么称呼、规矩和雷区</div>
      <div className="pa-field">
        <div className="pa-label">去哪个家</div>
        <Chips value={place} onChange={setPlace} options={places.map((item) => ({ label: item.name, value: item.placeKey }))} />
      </div>
      <div className="pa-field">
        <div className="pa-label">行程名称<small>可选</small></div>
        <input className="pa-input" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="例如：春节回岳父母家" />
      </div>
      <div className="pa-field">
        <div className="pa-label">出发时间</div>
        <input className="pa-input" type="datetime-local" value={startAt} onChange={(e) => setStartAt(e.target.value)} />
      </div>
      <div className="pa-field">
        <div className="pa-label">回程时间<small>可选</small></div>
        <input className="pa-input" type="datetime-local" value={endAt} onChange={(e) => setEndAt(e.target.value)} />
      </div>
      <div className="pa-field">
        <div className="pa-label">这次会见到谁<small>可多选</small></div>
        <Chips
          multiple
          size="sm"
          value={people}
          onChange={setPeople}
          options={persons.filter((person) => person.personKey !== "me").map((person) => ({ label: personLabel(persons, person.personKey), value: person.personKey, tone: personTone(person.personKey, persons) }))}
        />
      </div>
      <div className="pa-field">
        <div className="pa-label">这次特别要注意的事</div>
        <div className="pa-textarea-wrap">
          <TextArea value={notes} onChange={setNotes} autoSize={{ minRows: 2, maxRows: 5 }} placeholder="例如：会见到大舅，要带孩子住两晚" />
        </div>
      </div>
      <div style={{ padding: "24px 20px 0" }}>
        <Button block variant="accent" loading={saving} onClick={save}>保存行程</Button>
      </div>
    </Sheet>
  );
}
