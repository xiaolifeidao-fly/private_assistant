import type { Metadata, Viewport } from "next";
import type { ReactNode } from "react";
import "./globals.css";
import { ServiceWorker } from "@/components/service-worker";
import { withBase } from "@/lib/base-path";

export const metadata: Metadata = {
  applicationName: "家庭助理",
  title: "家庭助理",
  description: "记住家里的事，帮我想清楚怎么做、怎么说。",
  manifest: withBase("/manifest.webmanifest"),
  appleWebApp: { capable: true, statusBarStyle: "default", title: "家庭助理" },
  icons: { icon: withBase("/icons/icon-192.png"), apple: withBase("/icons/apple-touch-icon.png") },
  formatDetection: { telephone: false },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#f6f3ee" },
    { media: "(prefers-color-scheme: dark)", color: "#141311" },
  ],
  colorScheme: "light dark",
  width: "device-width",
  initialScale: 1,
  maximumScale: 1,
  userScalable: false,
  viewportFit: "cover",
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="zh-CN">
      <body>
        {children}
        <ServiceWorker />
      </body>
    </html>
  );
}
