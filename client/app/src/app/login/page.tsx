"use client";

import { useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Toast } from "antd-mobile";
import { Eye, EyeOff } from "lucide-react";
import { api, getToken, setToken } from "@/lib/api";
import { withBase } from "@/lib/base-path";
import { Button } from "@/components/ui";
import { fetchMe, homeOf, roleOf } from "@/lib/role";

export default function LoginPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [reveal, setReveal] = useState(false);

  // 已经登录过（比如从主屏重新打开落在登录页）就直接进去；令牌失效时接口会再送回来
  useEffect(() => {
    if (getToken()) router.replace("/");
  }, [router]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!username.trim() || !password) {
      Toast.show({ content: "请输入用户名和密码" });
      return;
    }
    setLoading(true);
    try {
      const result = await api.post<{ token: string }>("/auth/login", { username: username.trim(), password });
      setToken(result.token);
      // 奶奶的账号直接进长辈模式
      const me = await fetchMe().catch(() => null);
      router.replace(homeOf(roleOf(me)));
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login">
      <div className="login-glow" />
      <div className="login-inner pa-enter">
        <img src={withBase("/icons/icon-192.png")} alt="" width={68} height={68} className="login-icon" />
        <h1 className="login-title">家庭助理</h1>
        <p className="login-tag">记住她说过的话，想清楚再开口</p>

        <form onSubmit={submit} className="login-form">
          <label className="login-field">
            <span>用户名</span>
            <input className="pa-input" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoCapitalize="off" autoCorrect="off" placeholder="请输入用户名" />
          </label>
          <label className="login-field">
            <span>密码</span>
            <div style={{ position: "relative" }}>
              <input className="pa-input" type={reveal ? "text" : "password"} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" placeholder="请输入密码" style={{ paddingRight: 48 }} />
              <button type="button" className="login-eye" onClick={() => setReveal(!reveal)} aria-label={reveal ? "隐藏密码" : "显示密码"}>
                {reveal ? <EyeOff size={18} /> : <Eye size={18} />}
              </button>
            </div>
          </label>
          <Button type="submit" block loading={loading} style={{ marginTop: 12 }}>登录</Button>
        </form>
      </div>
      <div className="login-foot">只属于你和家人的私人空间</div>

      <style jsx>{`
        .login {
          min-height: 100dvh;
          display: flex;
          flex-direction: column;
          justify-content: center;
          padding: calc(env(safe-area-inset-top) + 24px) 28px calc(env(safe-area-inset-bottom) + 24px);
          position: relative;
          overflow: hidden;
        }
        .login-glow {
          position: absolute;
          inset: -20% -30% auto;
          height: 60%;
          background: radial-gradient(50% 50% at 50% 50%, color-mix(in srgb, var(--pa-accent) 16%, transparent), transparent 70%);
          pointer-events: none;
        }
        .login-inner { position: relative; width: 100%; max-width: 380px; margin: 0 auto; }
        .login-icon { border-radius: 18px; box-shadow: var(--pa-shadow-lg); }
        .login-title { font-family: var(--pa-font-display); font-size: 34px; font-weight: 700; margin: 22px 0 0; letter-spacing: 0.02em; }
        .login-tag { color: var(--pa-text-2); margin: 6px 0 0; font-size: 15px; }
        .login-form { margin-top: 36px; display: flex; flex-direction: column; gap: 16px; }
        .login-field { display: flex; flex-direction: column; gap: 8px; }
        .login-field > span { font-size: 13px; font-weight: 600; color: var(--pa-text-2); letter-spacing: 0.03em; }
        .login-eye {
          position: absolute;
          right: 4px;
          top: 4px;
          width: 40px;
          height: 40px;
          border: 0;
          background: none;
          color: var(--pa-text-3);
          display: grid;
          place-items: center;
          cursor: pointer;
        }
        .login-foot { position: relative; text-align: center; font-size: 12px; color: var(--pa-text-3); margin-top: 48px; letter-spacing: 0.06em; }
      `}</style>
    </div>
  );
}
