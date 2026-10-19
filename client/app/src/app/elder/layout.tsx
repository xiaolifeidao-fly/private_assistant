"use client";

import { useEffect, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { AuthGate } from "@/components/auth-gate";
import { useMe } from "@/lib/role";
import "./elder.css";

/** 长辈模式只给奶奶的账号；别的账号误进来送回原来的首页。 */
export default function ElderLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGate>
      <ElderShell>{children}</ElderShell>
    </AuthGate>
  );
}

function ElderShell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const { role, ready } = useMe();

  useEffect(() => {
    if (ready && role !== "elder") router.replace("/");
  }, [ready, role, router]);

  // 弹出的提示（Toast、对话框）挂在 body 上，也要跟着放大
  useEffect(() => {
    document.body.classList.add("elder");
    return () => document.body.classList.remove("elder");
  }, []);

  if (!ready || role !== "elder") return <div style={{ height: "100dvh", background: "var(--pa-bg)" }} />;
  return <div className="el-root">{children}</div>;
}
