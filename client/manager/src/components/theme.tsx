"use client";

import { useEffect, useState, type ReactNode } from "react";
import { ConfigProvider, theme as antdTheme, type ThemeConfig } from "antd";
import zhCN from "antd/locale/zh_CN";

const font = '-apple-system, BlinkMacSystemFont, "SF Pro Text", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif';

const light: ThemeConfig = {
  token: {
    colorPrimary: "#b45a37",
    colorInfo: "#427eb3",
    colorSuccess: "#4f8a63",
    colorWarning: "#b9822b",
    colorError: "#c2463a",
    colorLink: "#b45a37",
    colorText: "#1d1b18",
    colorTextSecondary: "#5f5a52",
    colorTextTertiary: "#9a948a",
    colorTextQuaternary: "#bdb7ad",
    colorBgLayout: "#f6f3ee",
    colorBgContainer: "#fffdfa",
    colorBgElevated: "#ffffff",
    colorBorder: "rgba(40, 32, 20, 0.15)",
    colorBorderSecondary: "rgba(40, 32, 20, 0.08)",
    colorFillSecondary: "rgba(40, 32, 20, 0.06)",
    colorFillTertiary: "rgba(40, 32, 20, 0.04)",
    colorFillQuaternary: "rgba(40, 32, 20, 0.025)",
    fontFamily: font,
    fontSize: 14,
    borderRadius: 9,
    borderRadiusLG: 14,
    borderRadiusSM: 7,
    controlHeight: 36,
    boxShadow: "0 1px 2px rgba(48,36,20,0.04), 0 12px 32px -12px rgba(48,36,20,0.18)",
    boxShadowSecondary: "0 2px 6px rgba(48,36,20,0.05), 0 16px 40px -12px rgba(48,36,20,0.2)",
    motionEaseOut: "cubic-bezier(0.22, 1, 0.36, 1)",
    colorBgMask: "rgba(30, 22, 12, 0.32)",
  },
  components: {
    Button: { primaryShadow: "none", defaultShadow: "none", dangerShadow: "none", fontWeight: 500 },
    Table: {
      headerBg: "transparent",
      headerSplitColor: "transparent",
      rowHoverBg: "rgba(40, 32, 20, 0.025)",
      cellPaddingBlock: 14,
      cellPaddingInline: 20,
      borderColor: "rgba(40, 32, 20, 0.07)",
    },
    Tabs: { horizontalItemPadding: "16px 0", itemColor: "#5f5a52", titleFontSize: 14 },
    Drawer: { colorBgElevated: "#fffdfa" },
    Modal: { contentBg: "#fffdfa", headerBg: "#fffdfa" },
    Tag: { defaultBg: "rgba(40, 32, 20, 0.05)" },
    Segmented: { itemSelectedBg: "#ffffff", trackBg: "rgba(40, 32, 20, 0.06)" },
    Input: { activeShadow: "0 0 0 3px rgba(180, 90, 55, 0.12)" },
    Select: { optionSelectedBg: "#f5e5db" },
    Rate: { starColor: "#d08a3c" },
  },
};

const dark: ThemeConfig = {
  algorithm: antdTheme.darkAlgorithm,
  token: {
    ...light.token,
    colorPrimary: "#e08460",
    colorLink: "#e08460",
    colorInfo: "#7eb0dd",
    colorSuccess: "#79b38a",
    colorWarning: "#d9a650",
    colorError: "#e5776b",
    colorText: "#efebe4",
    colorTextSecondary: "#b3ada3",
    colorTextTertiary: "#7d776e",
    colorTextQuaternary: "#5c574f",
    colorBgLayout: "#141311",
    colorBgContainer: "#1d1c19",
    colorBgElevated: "#262521",
    colorBorder: "rgba(255, 245, 230, 0.14)",
    colorBorderSecondary: "rgba(255, 245, 230, 0.08)",
    colorFillSecondary: "rgba(255, 245, 230, 0.08)",
    colorFillTertiary: "rgba(255, 245, 230, 0.05)",
    colorFillQuaternary: "rgba(255, 245, 230, 0.03)",
    boxShadow: "0 0 0 1px rgba(255,245,230,0.05), 0 16px 40px -12px rgba(0,0,0,0.6)",
    boxShadowSecondary: "0 0 0 1px rgba(255,245,230,0.06), 0 16px 40px -12px rgba(0,0,0,0.7)",
    colorBgMask: "rgba(0, 0, 0, 0.55)",
  },
  components: {
    ...light.components,
    Table: { ...light.components!.Table, rowHoverBg: "rgba(255, 245, 230, 0.03)", borderColor: "rgba(255, 245, 230, 0.07)" },
    Tabs: { ...light.components!.Tabs, itemColor: "#b3ada3" },
    Drawer: { colorBgElevated: "#1d1c19" },
    Modal: { contentBg: "#1d1c19", headerBg: "#1d1c19" },
    Tag: { defaultBg: "rgba(255, 245, 230, 0.06)" },
    Segmented: { itemSelectedBg: "#2e2c28", trackBg: "rgba(255, 245, 230, 0.06)" },
    Input: { activeShadow: "0 0 0 3px rgba(224, 132, 96, 0.18)" },
    Select: { optionSelectedBg: "rgba(224, 132, 96, 0.16)" },
  },
};

/** 跟随系统明暗；CSS 变量通过 <html data-theme> 切换。 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [isDark, setDark] = useState(false);
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      setDark(media.matches);
      document.documentElement.dataset.theme = media.matches ? "dark" : "light";
    };
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, []);
  return (
    <ConfigProvider locale={zhCN} button={{ autoInsertSpace: false }} wave={{ disabled: true }} theme={isDark ? dark : light}>
      {children}
    </ConfigProvider>
  );
}
