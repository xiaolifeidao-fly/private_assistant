import type { Metadata } from "next";
import type { ReactNode } from "react";
import { AntdRegistry } from "@ant-design/nextjs-registry";
import "dayjs/locale/zh-cn";
import "./globals.css";
import { ThemeProvider } from "@/components/theme";
import { withBase } from "@/lib/base-path";

export const metadata: Metadata = {
  title: "家庭助理 · 管理端",
  description: "家庭档案、记忆、机器人与模型配置",
  icons: { icon: withBase("/icon.png") },
};

// 首帧就按系统明暗设好 data-theme，避免深色模式下先闪一下白底
const themeScript = `try{document.documentElement.dataset.theme=matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light"}catch(e){}`;

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>
        <AntdRegistry>
          <ThemeProvider>{children}</ThemeProvider>
        </AntdRegistry>
      </body>
    </html>
  );
}
