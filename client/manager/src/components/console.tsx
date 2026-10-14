"use client";

import { createContext, useContext } from "react";
import type { Person, Place, Principal } from "@/lib/types";

export interface ConsoleContextValue {
  me: Principal;
  persons: Person[];
  places: Place[];
  reloadFamily: () => Promise<void>;
  /** 记忆状态变了之后刷新侧边栏上的待确认数。 */
  refreshPending: () => void;
  personName: (key: string) => string;
  placeName: (key: string) => string;
}

export const ConsoleContext = createContext<ConsoleContextValue | null>(null);

export function useConsole(): ConsoleContextValue {
  const value = useContext(ConsoleContext);
  if (!value) throw new Error("useConsole 必须在控制台布局内使用");
  return value;
}

