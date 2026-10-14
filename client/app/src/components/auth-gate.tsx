"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { getToken } from "@/lib/api";

/** 没有令牌直接去登录页；令牌失效由接口返回 401 时统一处理。 */
export function AuthGate({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [ready, setReady] = useState(false);
  useEffect(() => {
    if (!getToken()) router.replace("/login");
    else setReady(true);
  }, [router]);
  if (!ready) return <div style={{ height: "100dvh", background: "var(--pa-bg)" }} />;
  return <>{children}</>;
}
