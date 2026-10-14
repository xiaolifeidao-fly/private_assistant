"use client";

import { useEffect, useState } from "react";
import { api } from "./api";
import type { MemoryType, Person, Place } from "./types";

/**
 * 家庭成员、记忆类型、地点很少变，进程内缓存一份，各页面共用。
 * 主屏 App 一开就是好几天，管理端改了档案这里也要能跟上：
 * 超过 STALE_MS 再用到时、或者显式 refreshFamily() 时重新拉，拉完通知所有在用的页面。
 */
const STALE_MS = 10 * 60_000;

interface Store<T> {
  path: string;
  value: T[];
  loadedAt: number;
  error: string;
  pending: Promise<void> | null;
  listeners: Set<(value: T[]) => void>;
}

function store<T>(path: string): Store<T> {
  return { path, value: [], loadedAt: 0, error: "", pending: null, listeners: new Set() };
}

const persons = store<Person>("/persons");
const types = store<MemoryType>("/memory-types");
const places = store<Place>("/places");

function load<T>(item: Store<T>, force = false): Promise<void> {
  if (item.pending) return item.pending;
  if (!force && item.loadedAt && Date.now() - item.loadedAt < STALE_MS) return Promise.resolve();
  item.pending = api
    .get<T[]>(item.path)
    .then((value) => {
      item.value = value;
      item.loadedAt = Date.now();
      item.error = "";
      item.listeners.forEach((listener) => listener(value));
    })
    .catch((error) => {
      // 失败了保留旧数据，下次用到再试
      item.error = (error as Error).message;
      // 换一个数组引用，让页面重渲染出错误状态
      item.listeners.forEach((listener) => listener([...item.value]));
    })
    .finally(() => {
      item.pending = null;
    });
  return item.pending;
}

function useStore<T>(item: Store<T>): T[] {
  const [value, setValue] = useState<T[]>(item.value);
  useEffect(() => {
    item.listeners.add(setValue);
    setValue(item.value);
    void load(item);
    return () => {
      item.listeners.delete(setValue);
    };
  }, [item]);
  return value;
}

export function usePersons(): Person[] {
  return useStore(persons);
}

export function useMemoryTypes(): MemoryType[] {
  return useStore(types);
}

export function usePlaces(): Place[] {
  return useStore(places);
}

/** 下拉刷新、从后台切回来时调用，强制重新拉档案。 */
export function refreshFamily(): Promise<void> {
  return Promise.all([load(persons, true), load(types, true), load(places, true)]).then(() => undefined);
}

/** 家庭成员最近一次加载失败的原因；成功过就是空串。 */
export function personsError(): string {
  return persons.error;
}

export function personLabel(persons: Person[], key: string): string {
  const person = persons.find((p) => p.personKey === key);
  if (!person) return key || "全家";
  return person.name && person.name !== person.roleLabel ? `${person.roleLabel}·${person.name}` : person.roleLabel;
}
