"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { App, Button, Form, Input } from "antd";
import { api, getToken, setToken } from "@/lib/api";
import { withBase } from "@/lib/base-path";

export default function LoginPage() {
  return (
    <App>
      <LoginForm />
    </App>
  );
}

function LoginForm() {
  const router = useRouter();
  const { message } = App.useApp();
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (getToken()) router.replace("/memories");
  }, [router]);

  const submit = async (values: { username: string; password: string }) => {
    setLoading(true);
    try {
      const result = await api.post<{ token: string }>("/auth/login", values);
      setToken(result.token);
      router.replace("/memories");
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login">
      <section className="login-brand">
        <div className="login-brand-top">
          <img src={withBase("/icon.png")} alt="" width={40} height={40} style={{ borderRadius: 11 }} />
          <span>家庭助理</span>
        </div>
        <blockquote className="login-quote">
          记住她说过的话，
          <br />
          想清楚再开口。
        </blockquote>
        <div className="login-brand-foot">家庭档案 · 记忆 · 人设卡 · 机器人 · 模型</div>
      </section>

      <section className="login-form-wrap">
        <div className="login-form">
          <div className="login-eyebrow">管理端</div>
          <h1 className="login-title">欢迎回来</h1>
          <p className="login-sub">登录后维护家庭档案、记忆和机器人</p>
          <Form layout="vertical" onFinish={submit} requiredMark={false} size="large" style={{ marginTop: 28 }}>
            <Form.Item name="username" label="用户名" rules={[{ required: true, message: "请输入用户名" }]}>
              <Input autoComplete="username" placeholder="用户名" />
            </Form.Item>
            <Form.Item name="password" label="密码" rules={[{ required: true, message: "请输入密码" }]}>
              <Input.Password autoComplete="current-password" placeholder="密码" />
            </Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading} style={{ height: 46, marginTop: 8, fontWeight: 600 }}>
              登录
            </Button>
          </Form>
        </div>
      </section>

      <style jsx>{`
        .login { min-height: 100vh; display: grid; grid-template-columns: minmax(0, 1.05fr) minmax(0, 1fr); }
        .login-brand {
          position: relative;
          overflow: hidden;
          padding: 40px 56px;
          display: flex;
          flex-direction: column;
          justify-content: space-between;
          color: #fbf3e8;
          background:
            radial-gradient(80% 60% at 15% 10%, rgba(255, 255, 255, 0.18), transparent 60%),
            radial-gradient(70% 70% at 100% 100%, rgba(80, 30, 10, 0.35), transparent 60%),
            linear-gradient(150deg, #c8714b 0%, #a24b2b 100%);
        }
        .login-brand-top { display: flex; align-items: center; gap: 12px; font-family: var(--pa-font-display); font-size: 20px; font-weight: 700; }
        .login-brand-top img { box-shadow: 0 6px 16px rgba(60, 20, 5, 0.3); }
        .login-quote {
          margin: 0;
          font-family: var(--pa-font-display);
          font-size: clamp(32px, 3.6vw, 52px);
          line-height: 1.45;
          font-weight: 700;
          letter-spacing: 0.04em;
        }
        .login-brand-foot { font-size: 13px; opacity: 0.75; letter-spacing: 0.12em; }
        .login-form-wrap { display: grid; place-items: center; padding: 40px 24px; background: var(--pa-bg); }
        .login-form { width: 100%; max-width: 380px; animation: pa-enter 0.5s var(--pa-ease) both; }
        .login-eyebrow { font-size: 12px; font-weight: 600; color: var(--pa-accent); letter-spacing: 0.12em; }
        .login-title { font-family: var(--pa-font-display); font-size: 32px; font-weight: 700; margin: 8px 0 0; }
        .login-sub { color: var(--pa-text-2); margin: 8px 0 0; }
        @media (max-width: 860px) {
          .login { grid-template-columns: 1fr; }
          .login-brand { display: none; }
        }
      `}</style>
    </div>
  );
}
